//go:build darwin

package text

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"movement.local/movement/internal/app"
	"movement.local/movement/internal/domain"
)

func TestIOFailuresPreserveRecoverableState(t *testing.T) {
	cases := []struct {
		name, operation string
		occurrence      int
		prepared        bool
	}{
		{"disk-full-staging", "write:transactions/staging.tmp", 1, false},
		{"staging-sync", "file_sync:transactions/staging.tmp", 1, false},
		{"staging-full-sync", "full_sync:transactions/staging.tmp", 1, false},
		{"prepare-rename", "rename:transactions/prepared.json", 1, false},
		{"prepare-directory-sync", "dir_sync:transactions", 1, true},
		{"event-write", "write:EVENT.tmp", 1, true},
		{"event-link", "link:EVENT", 1, true},
		{"event-directory-sync", "dir_sync:TASK/history", 1, true},
		{"task-write", "write:TASK/task.tmp", 1, true},
		{"task-rename", "rename:TASK/task.json", 1, true},
		{"task-directory-sync", "dir_sync:TASK", 2, true}, // history mkdir syncs TASK first
		{"receipt-write", "write:RECEIPT.tmp", 1, true},
		{"receipt-link", "link:RECEIPT", 1, true},
		{"receipt-directory-sync", "dir_sync:receipts", 1, true},
		{"journal-removal", "remove:transactions/prepared.json", 1, true},
		{"cleanup-directory-sync", "dir_sync:transactions", 2, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			base := New(dir)
			svc := app.Service{Repo: base}
			created := mustExec(t, svc, createRequest("create"))
			req := updateRequest(created.TaskID, "update", 1)
			taskDir := "tasks/" + created.TaskID
			operation := strings.NewReplacer("EVENT", taskDir+"/history/00000000000000000002-task_updated.md", "TASK", taskDir, "RECEIPT", receiptPath("update")).Replace(tc.operation)
			calls := 0
			repo := New(dir)
			repo.Hook = func(point string) error {
				if point == operation {
					calls++
					if calls == tc.occurrence {
						return syscall.ENOSPC
					}
				}
				return nil
			}
			_, err := (app.Service{Repo: repo}).Execute(ctx, req)
			var de *domain.Error
			if !errors.As(err, &de) || de.Code != "storage_error" {
				t.Fatalf("expected visible storage failure: %v", err)
			}
			if calls < tc.occurrence {
				t.Fatalf("failure point not reached: %s", operation)
			}
			// A fresh process-equivalent repository recovers before returning data.
			out, err := New(dir).Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			want := int64(1)
			if tc.prepared {
				want = 2
			}
			if len(out) != 1 || out[0].Task.Revision != want || int64(len(out[0].History)) != want {
				t.Fatalf("inconsistent recovery at %s", operation)
			}
			first := mustExec(t, svc, req)
			retry := mustExec(t, svc, req)
			if first != retry || first.Revision != 2 {
				t.Fatal("retry lost original response")
			}
			out, err = base.Snapshot(ctx)
			if err != nil || out[0].Task.Revision != 2 || len(out[0].History) != 2 {
				t.Fatalf("duplicated update: %v", err)
			}
		})
	}
}

func TestPersistentRecoveryFailureBlocksReaders(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	svc := app.Service{Repo: New(dir)}
	c := mustExec(t, svc, createRequest("create"))
	req := updateRequest(c.TaskID, "update", 1)
	repo := New(dir)
	repo.Hook = func(point string) error {
		if point == "rename:tasks/"+c.TaskID+"/task.json" {
			return syscall.EIO
		}
		return nil
	}
	_, err := (app.Service{Repo: repo}).Execute(ctx, req)
	if err == nil {
		t.Fatal("acknowledged failed write")
	}
	for i := 0; i < 2; i++ {
		if _, err = repo.Snapshot(ctx); err == nil {
			t.Fatal("served partial state during failed recovery")
		}
	}
	if _, err = os.Stat(filepath.Join(dir, "transactions/prepared.json")); err != nil {
		t.Fatal("lost pending transaction")
	}
	if b, err := New(dir).Snapshot(ctx); err != nil || b[0].Task.Revision != 2 {
		t.Fatalf("could not recover: %v", err)
	}
}

func TestRecoveryResyncsPreviouslyLinkedFiles(t *testing.T) {
	for _, sub := range []string{"history", "receipts"} {
		t.Run(sub, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			svc := app.Service{Repo: New(dir)}
			c := mustExec(t, svc, createRequest("create"))
			req := updateRequest(c.TaskID, "update", 1)
			target := "receipts"
			if sub == "history" {
				target = "tasks/" + c.TaskID + "/history"
			}
			repo := New(dir)
			repo.Hook = func(point string) error {
				if point == "dir_sync:"+target {
					return syscall.EIO
				}
				return nil
			}
			if _, err := (app.Service{Repo: repo}).Execute(ctx, req); err == nil {
				t.Fatal("sync failure ignored")
			}
			if _, err := repo.Snapshot(ctx); err == nil {
				t.Fatal("recovery skipped directory sync for an existing immutable file")
			}
			if _, err := New(dir).Snapshot(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShortUnpreparedJournalIsDiscarded(t *testing.T) {
	dir := t.TempDir()
	svc := app.Service{Repo: New(dir)}
	c := mustExec(t, svc, createRequest("create"))
	if err := os.WriteFile(filepath.Join(dir, "transactions/staging.tmp"), []byte(`{"payload":"partial`), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := New(dir).Snapshot(context.Background())
	if err != nil || len(b) != 1 || b[0].Task.ID != c.TaskID || b[0].Task.Revision != 1 {
		t.Fatalf("partial staging affected committed data: %v", err)
	}
	if _, err = os.Stat(filepath.Join(dir, "transactions/staging.tmp")); !os.IsNotExist(err) {
		t.Fatal(fmt.Sprint("staging not cleaned: ", err))
	}
}
