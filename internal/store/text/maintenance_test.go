//go:build darwin

package text

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"movement.local/movement/internal/app"
)

func TestRestorePreparedStoreAndRotateCredential(t *testing.T) {
	ctx := context.Background()
	source := New(t.TempDir())
	if _, err := source.Initialize(ctx, "Europe/Paris"); err != nil {
		t.Fatal(err)
	}
	old, err := source.APIToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	svc := app.Service{Repo: source}
	created := mustExec(t, svc, createRequest("backup-create"))
	req := updateRequest(created.TaskID, "backup-update", 1)
	source.Hook = func(point string) error {
		if point == "prepared" {
			return errors.New("interrupted after preparation")
		}
		return nil
	}
	if _, err = svc.Execute(ctx, req); err == nil {
		t.Fatal("expected interrupted operation")
	}
	restored := New(t.TempDir())
	if err = os.CopyFS(restored.Dir, os.DirFS(source.Dir)); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(source.Dir, "transactions/prepared.json"))
	if err != nil {
		t.Fatal(err)
	}
	summary, err := restored.Verify(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary != (Verification{Tasks: 1, Events: 2, Receipts: 2}) {
		t.Fatalf("bad summary %+v", summary)
	}
	receipt := mustExec(t, app.Service{Repo: restored}, req)
	if receipt.Revision != 2 {
		t.Fatal(receipt)
	}
	if err = restored.RotateAPIToken(ctx); err != nil {
		t.Fatal(err)
	}
	fresh, err := restored.APIToken(ctx)
	if err != nil || fresh == old || len(fresh) != 64 {
		t.Fatalf("rotation: %v", err)
	}
	if mustExec(t, app.Service{Repo: restored}, req) != receipt {
		t.Fatal("rotation changed retry receipt")
	}
	after, err := restored.Verify(ctx)
	if err != nil || after != summary {
		t.Fatalf("rotation changed task history: %+v %v", after, err)
	}
	untouched, err := os.ReadFile(filepath.Join(source.Dir, "transactions/prepared.json"))
	if err != nil || string(untouched) != string(before) {
		t.Fatal("restore changed source")
	}
	raw, err := os.ReadFile(filepath.Join(source.Dir, "runtime/api-token"))
	if err != nil || strings.TrimSpace(string(raw)) != old {
		t.Fatal("source credential changed")
	}
	st, err := os.Stat(filepath.Join(restored.Dir, "runtime/api-token"))
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("token permissions")
	}
}

func TestCredentialRotationFailuresRemainVisibleAndRetryable(t *testing.T) {
	for _, point := range []string{"write:runtime/api-token.tmp", "file_sync:runtime/api-token.tmp", "full_sync:runtime/api-token.tmp", "rename:runtime/api-token", "dir_sync:runtime"} {
		t.Run(point, func(t *testing.T) {
			ctx := context.Background()
			repo := New(t.TempDir())
			if _, err := repo.Initialize(ctx, "UTC"); err != nil {
				t.Fatal(err)
			}
			old, _ := repo.APIToken(ctx)
			repo.Hook = func(p string) error {
				if p == point {
					return syscall.ENOSPC
				}
				return nil
			}
			if err := repo.RotateAPIToken(ctx); err == nil {
				t.Fatal("acknowledged failed rotation")
			}
			repo.Hook = nil
			current, err := repo.APIToken(ctx)
			if err != nil || len(current) != 64 {
				t.Fatalf("torn credential: %v", err)
			}
			if point != "dir_sync:runtime" && old != current {
				t.Fatal("credential changed before replacement")
			}
			if err := repo.RotateAPIToken(ctx); err != nil {
				t.Fatal(err)
			}
			fresh, _ := repo.APIToken(ctx)
			if fresh == old {
				t.Fatal("retry did not rotate")
			}
		})
	}
}

func TestRotationRejectsCorruptStoreWithoutChangingCredential(t *testing.T) {
	ctx := context.Background()
	repo := New(t.TempDir())
	if _, err := repo.Initialize(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	created := mustExec(t, app.Service{Repo: repo}, createRequest("corrupt"))
	before, _ := os.ReadFile(filepath.Join(repo.Dir, "runtime/api-token"))
	if err := os.WriteFile(filepath.Join(repo.Dir, taskPath(created.TaskID)), []byte("bad JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := repo.RotateAPIToken(ctx); err == nil {
		t.Fatal("rotated corrupt store")
	}
	after, _ := os.ReadFile(filepath.Join(repo.Dir, "runtime/api-token"))
	if string(before) != string(after) {
		t.Fatal("credential changed")
	}
}

func TestRestoredClaimReceiptSurvivesHTTPRotation(t *testing.T) {
	ctx := context.Background()
	source := New(t.TempDir())
	if _, err := source.Initialize(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	created := mustExec(t, app.Service{Repo: source}, createRequest("create-with-claim"))
	revision := int64(1)
	claim := app.Request{OperationID: "restored-claim", Kind: "claim", TaskID: created.TaskID, Actor: "agent", ExpectedRevision: &revision, Data: []byte(`{"seconds":900}`)}
	original := mustExec(t, app.Service{Repo: source}, claim)
	restored := New(t.TempDir())
	if err := os.CopyFS(restored.Dir, os.DirFS(source.Dir)); err != nil {
		t.Fatal(err)
	}
	if err := restored.RotateAPIToken(ctx); err != nil {
		t.Fatal(err)
	}
	replayed := mustExec(t, app.Service{Repo: restored}, claim)
	if replayed != original || replayed.ClaimToken == "" {
		t.Fatal("claim retry response changed")
	}
	revision = replayed.Revision
	release := app.Request{OperationID: "restored-release", Kind: "release", TaskID: created.TaskID, Actor: "agent", ExpectedRevision: &revision, ClaimToken: replayed.ClaimToken, Data: []byte(`{}`)}
	released := mustExec(t, app.Service{Repo: restored}, release)
	if released.Revision != 3 {
		t.Fatal(released)
	}
	sourceState, err := source.Snapshot(ctx)
	if err != nil || sourceState[0].Task.Claim == nil || sourceState[0].Task.Revision != 2 {
		t.Fatal("source claim changed")
	}
}
