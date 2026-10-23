//go:build darwin

package text

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"movement.local/movement/internal/app"
	"movement.local/movement/internal/domain"
)

func createRequest(op string) app.Request {
	b, _ := json.Marshal(app.Create{Title: "Task", Status: domain.Ready, Context: domain.Context{Objective: "Create draft"}, Completion: domain.Completion{Version: 1, Criteria: []domain.Criterion{{ID: "draft", Description: "Draft exists"}}}})
	return app.Request{OperationID: op, Kind: "create", Actor: "test", Data: b}
}
func updateRequest(id, op string, rev int64) app.Request {
	return app.Request{TaskID: id, OperationID: op, Kind: "update", Actor: "test", ExpectedRevision: &rev, Data: json.RawMessage(`{"fields":{"title":"Updated"}}`)}
}
func TestProcess(t *testing.T) {
	if os.Getenv("MOVEMENT_STORE_WORKER") == "" {
		return
	}
	dir := os.Getenv("MOVEMENT_STORE_DIR")
	r := New(dir)
	r.Hook = func(point string) error {
		if point == os.Getenv("MOVEMENT_STORE_CRASH") {
			_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		}
		return nil
	}
	s := app.Service{Repo: r}
	var req app.Request
	if os.Getenv("MOVEMENT_STORE_READ") == "1" {
		b, e := r.Snapshot(context.Background())
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		raw, _ := json.Marshal(b)
		fmt.Println(string(raw))
		os.Exit(0)
	}
	if e := json.Unmarshal([]byte(os.Getenv("MOVEMENT_STORE_REQUEST")), &req); e != nil {
		panic(e)
	}
	receipt, e := s.Execute(context.Background(), req)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	raw, _ := json.Marshal(receipt)
	fmt.Println(string(raw))
	os.Exit(0)
}
func process(dir, phase string, req *app.Request) ([]byte, error) {
	c := exec.Command(os.Args[0], "-test.run=^TestProcess$")
	c.Env = append(os.Environ(), "MOVEMENT_STORE_WORKER=1", "MOVEMENT_STORE_DIR="+dir, "MOVEMENT_STORE_CRASH="+phase)
	if req == nil {
		c.Env = append(c.Env, "MOVEMENT_STORE_READ=1")
	} else {
		b, _ := json.Marshal(req)
		c.Env = append(c.Env, "MOVEMENT_STORE_REQUEST="+string(b))
	}
	return c.CombinedOutput()
}
func mustExec(t *testing.T, s app.Service, req app.Request) app.Receipt {
	t.Helper()
	r, e := s.Execute(context.Background(), req)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestCrashRecovery(t *testing.T) {
	for _, phase := range []string{"staged", "prepared", "event", "task", "receipt", "cleanup"} {
		for _, isCreate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/create=%v", phase, isCreate), func(t *testing.T) {
				dir := t.TempDir()
				s := app.Service{Repo: New(dir)}
				req := createRequest("first")
				prior := int64(0)
				if !isCreate {
					r := mustExec(t, s, req)
					req = updateRequest(r.TaskID, "update", 1)
					prior = 1
				}
				b, e := process(dir, phase, &req)
				if e == nil || !strings.Contains(e.Error(), "killed") {
					t.Fatalf("expected SIGKILL %v %s", e, b)
				}
				out, e := New(dir).Snapshot(context.Background())
				if e != nil {
					t.Fatal(e)
				}
				expected := prior + 1
				if phase == "staged" {
					expected = prior
				}
				if expected == 0 {
					if len(out) != 0 {
						t.Fatal("unprepared create visible")
					}
				} else if len(out) != 1 || out[0].Task.Revision != expected || int64(len(out[0].History)) != expected {
					t.Fatal("state/history mismatch")
				}
				first := mustExec(t, s, req)
				second := mustExec(t, s, req)
				if first != second {
					t.Fatal("retry receipt changed")
				}
				out, e = New(dir).Snapshot(context.Background())
				if e != nil || len(out) != 1 || out[0].Task.Revision != prior+1 {
					t.Fatalf("retry duplicated or failed: %v", e)
				}
			})
		}
	}
}
func TestConcurrentWriters(t *testing.T) {
	for _, identical := range []bool{false, true} {
		t.Run(fmt.Sprint(identical), func(t *testing.T) {
			dir := t.TempDir()
			s := app.Service{Repo: New(dir)}
			created := mustExec(t, s, createRequest("create"))
			var wg sync.WaitGroup
			results := make(chan string, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					op := fmt.Sprintf("op-%d", i)
					if identical {
						op = "same"
					}
					req := updateRequest(created.TaskID, op, 1)
					b, e := process(dir, "", &req)
					if e == nil {
						results <- "ok"
					} else if strings.Contains(string(b), "revision_conflict") {
						results <- "conflict"
					} else {
						results <- fmt.Sprintf("%v %s", e, b)
					}
				}(i)
			}
			wg.Wait()
			close(results)
			success := 0
			for x := range results {
				if x == "ok" {
					success++
				} else if x != "conflict" {
					t.Fatal(x)
				}
			}
			want := 1
			if identical {
				want = 8
			}
			if success != want {
				t.Fatalf("success %d want %d", success, want)
			}
			all, e := New(dir).Snapshot(context.Background())
			if e != nil || len(all[0].History) != 2 {
				t.Fatalf("history %v %v", all, e)
			}
		})
	}
}
func TestCreateRetryGlobalScopeAndIngestion(t *testing.T) {
	dir := t.TempDir()
	s := app.Service{Repo: New(dir)}
	req := createRequest("global")
	first := mustExec(t, s, req)
	if r := mustExec(t, s, req); r != first {
		t.Fatal("create retry changed ID")
	}
	other := updateRequest(first.TaskID, "global", 1)
	if _, e := s.Execute(context.Background(), other); e == nil {
		t.Fatal("operation ID not globally scoped")
	}
	data := app.Create{Title: "discovery", Ingestion: &domain.Ingestion{Producer: "routine", ExternalKey: `["doc","id","action","once"]`}}
	req.Data, _ = json.Marshal(data)
	req.OperationID = "discover1"
	a := mustExec(t, s, req)
	req.OperationID = "discover2"
	b := mustExec(t, s, req)
	if !b.Existing || b.TaskID != a.TaskID {
		t.Fatal("ingestion duplicate")
	}
	if replay := mustExec(t, s, req); replay != b {
		t.Fatal("dedupe receipt lost")
	}
}
func TestRecoveryDiesAgain(t *testing.T) {
	dir := t.TempDir()
	req := createRequest("create")
	if _, e := process(dir, "prepared", &req); e == nil {
		t.Fatal("expected death")
	}
	if _, e := process(dir, "event", nil); e == nil {
		t.Fatal("expected recovery death")
	}
	all, e := New(dir).Snapshot(context.Background())
	if e != nil || len(all) != 1 {
		t.Fatal(e)
	}
}
func TestSymlinkEscape(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	if e := os.Symlink(outside, filepath.Join(dir, "tasks")); e != nil {
		t.Fatal(e)
	}
	_, e := New(dir).Snapshot(context.Background())
	if e == nil {
		t.Fatal("symlink accepted")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) > 0 {
		t.Fatal("wrote outside root")
	}
}
func TestHistoryCorruption(t *testing.T) {
	dir := t.TempDir()
	s := app.Service{Repo: New(dir)}
	r := mustExec(t, s, createRequest("create"))
	files, _ := filepath.Glob(filepath.Join(dir, "tasks", r.TaskID, "history", "*.md"))
	if e := os.WriteFile(files[0], []byte("bad"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := New(dir).Snapshot(context.Background()); e == nil {
		t.Fatal("silently ignored corruption")
	}
}
func TestEventParsingRejectsUnsafeYAML(t *testing.T) {
	for _, body := range []string{"key: 1\nkey: 2", "key: &x value\nother: *x", "key: !custom value", "key: 2026-10-22", "[1,2]"} {
		if _, e := parseEvent([]byte("---\n" + body + "\n---\n\nsummary\n")); e == nil {
			t.Fatalf("accepted unsafe YAML %s", body)
		}
	}
}
func TestLockTimeout(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	if _, e := r.Snapshot(context.Background()); e != nil {
		t.Fatal(e)
	}
	f, e := os.OpenFile(filepath.Join(dir, "runtime", "write.lock"), os.O_RDWR, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		t.Fatal(e)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	r.LockTimeout = 20 * time.Millisecond
	_, e = r.Snapshot(context.Background())
	if e == nil || !strings.Contains(e.Error(), "store_busy") {
		t.Fatal(e)
	}
}
