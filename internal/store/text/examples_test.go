//go:build darwin

package text

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"movement.local/movement/internal/app"
	"movement.local/movement/internal/domain"
)

func TestExampleBundlesLoadAndRetry(t *testing.T) {
	for _, name := range []string{"basic-task", "waiting-for-task", "personal-email"} {
		t.Run(name, func(t *testing.T) {
			source := filepath.Join("../../..", "examples", name)
			ctx := context.Background()
			destination := t.TempDir()
			repo := New(destination)
			if _, e := repo.Initialize(ctx, "Europe/Paris"); e != nil {
				t.Fatal(e)
			}
			raw, e := os.ReadFile(filepath.Join(source, "task.json"))
			if e != nil {
				t.Fatal(e)
			}
			var task domain.Task
			if e = app.Decode(raw, &task); e != nil {
				t.Fatal(e)
			}
			taskDir := filepath.Join(destination, "tasks", task.ID)
			if e = os.MkdirAll(taskDir, 0700); e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(taskDir, "task.json"), raw, 0600); e != nil {
				t.Fatal(e)
			}
			for _, sub := range []string{"history", "receipts"} {
				target := filepath.Join(taskDir, sub)
				if sub == "receipts" {
					target = filepath.Join(destination, sub)
				}
				if e = os.MkdirAll(target, 0700); e != nil {
					t.Fatal(e)
				}
				files, e := os.ReadDir(filepath.Join(source, sub))
				if e != nil {
					t.Fatal(e)
				}
				for _, f := range files {
					b, e := os.ReadFile(filepath.Join(source, sub, f.Name()))
					if e != nil {
						t.Fatal(e)
					}
					if e = os.WriteFile(filepath.Join(target, f.Name()), b, 0600); e != nil {
						t.Fatal(e)
					}
				}
			}
			snapshots, e := repo.Snapshot(ctx)
			if e != nil {
				t.Fatal(e)
			}
			if len(snapshots) != 1 || len(snapshots[0].History) != int(task.Revision) {
				t.Fatal("fixture state/history mismatch")
			}
			definition, e := os.ReadFile(filepath.Join(source, "definition.json"))
			if e != nil {
				t.Fatal(e)
			}
			receipt, e := (app.Service{Repo: repo}).Execute(ctx, app.Request{Kind: "create", OperationID: "example-" + name + "-create", Actor: "example-user", Data: json.RawMessage(definition)})
			if e != nil {
				t.Fatal(e)
			}
			if receipt.TaskID != task.ID || receipt.Revision != 1 {
				t.Fatal("original creation receipt not preserved")
			}
			after, e := repo.Snapshot(ctx)
			if e != nil {
				t.Fatal(e)
			}
			if after[0].Task.Revision != task.Revision {
				t.Fatal("fixture retry changed history")
			}
		})
	}
}
