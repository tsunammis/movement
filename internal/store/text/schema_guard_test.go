//go:build darwin

package text

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"movement.local/movement/internal/app"
	"movement.local/movement/internal/domain"
)

func storeBytes(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		out[relative] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func futureJSON(t *testing.T, raw []byte) []byte {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	value["schema_version"] = 2
	value["future_field"] = "must remain unchanged"
	return encode(value)
}

func TestUnsupportedStoreCannotRecoverOrMutate(t *testing.T) {
	for _, kind := range []string{"config", "task", "history", "transaction", "journal-task", "journal-history"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			repo := New(dir)
			if _, err := repo.Initialize(ctx, "UTC"); err != nil {
				t.Fatal(err)
			}
			svc := app.Service{Repo: repo}
			changed := mustExec(t, svc, createRequest("supported-task"))
			other := mustExec(t, svc, createRequest("future-task"))
			repo.Hook = func(point string) error {
				if point == "prepared" {
					return errors.New("stop before installation")
				}
				return nil
			}
			if _, err := svc.Execute(ctx, updateRequest(changed.TaskID, "pending-update", 1)); err == nil {
				t.Fatal("expected interruption")
			}
			repo.Hook = nil
			var path string
			switch kind {
			case "config":
				path = "config.json"
			case "task":
				path = taskPath(other.TaskID)
			case "history":
				path = "tasks/" + other.TaskID + "/history/00000000000000000001-task_created.md"
			default:
				path = "transactions/prepared.json"
			}
			raw, err := os.ReadFile(filepath.Join(dir, path))
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "history":
				raw = []byte(strings.Replace(string(raw), `"schema_version": 1`, `"schema_version": 2`, 1))
				if !strings.Contains(string(raw), `"schema_version": 2`) {
					t.Fatalf("history schema fixture failed: %s", raw)
				}
			case "transaction", "journal-task", "journal-history":
				var env envelope
				if err = json.Unmarshal(raw, &env); err != nil {
					t.Fatal(err)
				}
				if kind == "transaction" {
					env.Payload = futureJSON(t, env.Payload)
				} else {
					var payload map[string]json.RawMessage
					if err = json.Unmarshal(env.Payload, &payload); err != nil {
						t.Fatal(err)
					}
					field := "task"
					if kind == "journal-history" {
						field = "event"
					}
					payload[field] = futureJSON(t, payload[field])
					env.Payload = encode(payload)
				}
				env.Checksum = digest(env.Payload)
				raw = encode(env)
			default:
				raw = futureJSON(t, raw)
			}
			if err = os.WriteFile(filepath.Join(dir, path), raw, 0600); err != nil {
				t.Fatal(err)
			}
			// Even cleanup of unprepared staging must not happen in an unsupported store.
			if err = os.WriteFile(filepath.Join(dir, "transactions/staging.tmp"), []byte("retain staging evidence"), 0600); err != nil {
				t.Fatal(err)
			}
			before := storeBytes(t, dir)
			calls := map[string]func() error{
				"snapshot":   func() error { _, e := repo.Snapshot(ctx); return e },
				"settings":   func() error { _, e := repo.Settings(ctx); return e },
				"initialize": func() error { _, e := repo.Initialize(ctx, "UTC"); return e },
				"verify":     func() error { _, e := repo.Verify(ctx); return e },
				"rotate":     func() error { return repo.RotateAPIToken(ctx) },
				"create":     func() error { _, e := svc.Execute(ctx, createRequest("must-not-create")); return e },
				"retry":      func() error { _, e := svc.Execute(ctx, createRequest("supported-task")); return e },
			}
			for name, call := range calls {
				err := call()
				var de *domain.Error
				if !errors.As(err, &de) || de.Code != "unsupported_schema" {
					t.Fatalf("%s: expected unsupported_schema, got %v", name, err)
				}
				if !reflect.DeepEqual(before, storeBytes(t, dir)) {
					t.Fatalf("%s changed files in unsupported store", name)
				}
			}
		})
	}
}

func TestTraversalTaskIDsNeverTouchStore(t *testing.T) {
	ctx := context.Background()
	repo := New(t.TempDir())
	if _, err := repo.Initialize(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	svc := app.Service{Repo: repo}
	mustExec(t, svc, createRequest("existing"))
	before := storeBytes(t, repo.Dir)
	for _, id := range []string{"../config.json", "../../outside", "/tmp/outside", "%2e%2e%2fconfig.json", "tasks/../config.json", "..\\outside"} {
		_, err := svc.Get(ctx, id)
		var de *domain.Error
		if !errors.As(err, &de) || de.Code != "invalid_input" {
			t.Fatalf("read %q: %v", id, err)
		}
		_, err = svc.Execute(ctx, updateRequest(id, "traversal", 1))
		if !errors.As(err, &de) || de.Code != "invalid_input" {
			t.Fatalf("write %q: %v", id, err)
		}
		if !reflect.DeepEqual(before, storeBytes(t, repo.Dir)) {
			t.Fatalf("traversal %q changed store", id)
		}
	}
}
