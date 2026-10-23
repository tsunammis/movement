//go:build darwin

package text

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"movement.local/movement/internal/app"
	"movement.local/movement/internal/domain"
)

func TestReceiptCorruptionCannotDuplicateOperations(t *testing.T) {
	for _, kind := range []string{"missing", "changed-digest", "wrong-event", "wrong-task", "wrong-existing-flag", "unexpected-token"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			repo := New(dir)
			svc := app.Service{Repo: repo}
			req := createRequest("create")
			c := mustExec(t, svc, req)
			taskFile := filepath.Join(dir, taskPath(c.TaskID))
			before, err := os.ReadFile(taskFile)
			if err != nil {
				t.Fatal(err)
			}
			receiptFile := filepath.Join(dir, receiptPath("create"))
			changed := c
			switch kind {
			case "missing":
				err = os.Remove(receiptFile)
			case "changed-digest":
				changed.Digest = digest([]byte("other"))
			case "wrong-event":
				changed.EventID = domain.NewID()
			case "wrong-task":
				changed.TaskID = domain.NewID()
			case "wrong-existing-flag":
				changed.Existing = true
			case "unexpected-token":
				changed.ClaimToken = "unexpected"
			}
			if err != nil {
				t.Fatal(err)
			}
			if kind != "missing" {
				if err = os.WriteFile(receiptFile, encode(changed), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = repo.Snapshot(context.Background()); err == nil {
				t.Fatal("corrupt receipt accepted by snapshot")
			}
			if _, err = svc.Execute(context.Background(), req); err == nil {
				t.Fatal("corrupt receipt retry succeeded")
			}
			after, err := os.ReadFile(taskFile)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("retry overwrote task")
			}
			tasks, err := os.ReadDir(filepath.Join(dir, "tasks"))
			if err != nil || len(tasks) != 1 {
				t.Fatal("retry duplicated task")
			}
		})
	}
}
func TestExpiredClaimReceiptStillHasVerifiedToken(t *testing.T) {
	dir := t.TempDir()
	repo := New(dir)
	svc := app.Service{Repo: repo}
	c := mustExec(t, svc, createRequest("create"))
	rev := int64(1)
	req := app.Request{Kind: "claim", TaskID: c.TaskID, OperationID: "claim", Actor: "agent", ExpectedRevision: &rev, Data: json.RawMessage(`{}`)}
	claim := mustExec(t, svc, req)
	rev = 2
	mustExec(t, svc, app.Request{Kind: "release", TaskID: c.TaskID, OperationID: "release", Actor: "agent", ExpectedRevision: &rev, ClaimToken: claim.ClaimToken, Data: json.RawMessage(`{}`)})
	claim.ClaimToken = domain.NewID() + domain.NewID()
	if err := os.WriteFile(filepath.Join(dir, receiptPath("claim")), encode(claim), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Execute(context.Background(), req); err == nil {
		t.Fatal("returned altered historical claim token")
	}
}
func TestPreparedJournalRejectsAmbiguousJSON(t *testing.T) {
	for _, kind := range []string{"trailing-envelope", "unknown-payload-field", "duplicate-payload-field"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			repo := New(dir)
			svc := app.Service{Repo: repo}
			c := mustExec(t, svc, createRequest("create"))
			before, err := os.ReadFile(filepath.Join(dir, taskPath(c.TaskID)))
			if err != nil {
				t.Fatal(err)
			}
			repo.Hook = func(point string) error {
				if point == "prepared" {
					return syscall.EIO
				}
				return nil
			}
			if _, err = svc.Execute(context.Background(), updateRequest(c.TaskID, "update", 1)); err == nil {
				t.Fatal("fault not injected")
			}
			path := filepath.Join(dir, "transactions/prepared.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "trailing-envelope" {
				raw = append(raw, []byte(` {}`)...)
			} else {
				var env envelope
				if err = json.Unmarshal(raw, &env); err != nil {
					t.Fatal(err)
				}
				prefix := []byte(`{"unknown":true,`)
				if kind == "duplicate-payload-field" {
					prefix = []byte(`{"schema_version":1,`)
				}
				env.Payload = append(prefix, env.Payload[1:]...)
				env.Checksum = digest(env.Payload)
				raw = encode(env)
			}
			if err = os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = New(dir).Snapshot(context.Background()); err == nil {
				t.Fatal("malformed prepared journal accepted")
			}
			retained, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(retained, raw) {
				t.Fatal("malformed journal evidence removed")
			}
			after, err := os.ReadFile(filepath.Join(dir, taskPath(c.TaskID)))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("malformed journal changed task")
			}
		})
	}
}
