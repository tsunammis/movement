//go:build darwin

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWorker(t *testing.T) {
	if os.Getenv("MOVEMENT_SPIKE_WORKER") != "1" {
		return
	}
	sep := 0
	for i, a := range os.Args {
		if a == "--" {
			sep = i + 1
			break
		}
	}
	if e := run(os.Args[sep:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	os.Exit(0)
}
func command(args ...string) *exec.Cmd {
	c := exec.Command(os.Args[0], append([]string{"-test.run=^TestWorker$", "--"}, args...)...)
	c.Env = append(os.Environ(), "MOVEMENT_SPIKE_WORKER=1")
	return c
}
func invoke(dir string, args ...string) ([]byte, error) {
	return command(append([]string{"-dir", dir}, args...)...).CombinedOutput()
}
func must(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	b, e := invoke(dir, args...)
	if e != nil {
		t.Fatalf("%v: %v %s", args, e, b)
	}
	return b
}
func check(t *testing.T, dir string, rev int) {
	t.Helper()
	var task Task
	if e := json.Unmarshal(must(t, dir, "get", "demo"), &task); e != nil {
		t.Fatal(e)
	}
	if task.Revision != rev {
		t.Fatalf("revision %d want %d", task.Revision, rev)
	}
	files, e := filepath.Glob(filepath.Join(dir, "tasks", "demo", "history", "*.md"))
	if e != nil || len(files) != rev {
		t.Fatalf("history count %d want %d", len(files), rev)
	}
}
func TestCrashRecovery(t *testing.T) {
	for _, phase := range []string{"staged", "prepared", "event", "task", "cleanup"} {
		for _, creation := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/create=%v", phase, creation), func(t *testing.T) {
				dir := t.TempDir()
				prior := 0
				if !creation {
					must(t, dir, "set", "demo", "0", "create", "old")
					prior = 1
				}
				b, e := invoke(dir, "-crash", phase, "set", "demo", fmt.Sprint(prior), "change", "new")
				if e == nil || !strings.Contains(e.Error(), "killed") {
					t.Fatalf("expected SIGKILL: %v %s", e, b)
				}
				expected := prior + 1
				if phase == "staged" {
					expected = prior
				}
				if expected == 0 {
					b, e = invoke(dir, "get", "demo")
					if e == nil || !strings.Contains(string(b), "not_found") {
						t.Fatalf("unprepared create visible %v %s", e, b)
					}
				} else {
					check(t, dir, expected)
				}
				first := must(t, dir, "set", "demo", fmt.Sprint(prior), "change", "new")
				second := must(t, dir, "set", "demo", fmt.Sprint(prior), "change", "new")
				if string(first) != string(second) {
					t.Fatal("receipt changed")
				}
				check(t, dir, prior+1)
				check(t, dir, prior+1)
			})
		}
	}
}
func TestConcurrentProcesses(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(fmt.Sprint(same), func(t *testing.T) {
			dir := t.TempDir()
			must(t, dir, "set", "demo", "0", "create", "old")
			const n = 12
			var wg sync.WaitGroup
			results := make(chan string, n)
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					op := fmt.Sprintf("op-%d", i)
					if same {
						op = "same"
					}
					b, e := invoke(dir, "set", "demo", "1", op, "new")
					if e == nil {
						results <- "ok"
					} else if strings.Contains(string(b), "revision_conflict") {
						results <- "conflict"
					} else {
						results <- fmt.Sprintf("unexpected %v %s", e, b)
					}
				}(i)
			}
			wg.Wait()
			close(results)
			ok, conflicts := 0, 0
			for result := range results {
				switch result {
				case "ok":
					ok++
				case "conflict":
					conflicts++
				default:
					t.Fatal(result)
				}
			}
			if same {
				if ok != n {
					t.Fatalf("retry successes %d", ok)
				}
			} else if ok != 1 || conflicts != n-1 {
				t.Fatalf("success %d conflicts %d", ok, conflicts)
			}
			check(t, dir, 2)
		})
	}
}
func TestReceiptBeforeRevision(t *testing.T) {
	dir := t.TempDir()
	first := must(t, dir, "set", "demo", "0", "first", "one")
	must(t, dir, "set", "demo", "1", "second", "two")
	if b := must(t, dir, "set", "demo", "0", "first", "one"); string(b) != string(first) {
		t.Fatal("old receipt changed")
	}
	b, e := invoke(dir, "set", "demo", "0", "first", "changed")
	if e == nil || !strings.Contains(string(b), "operation_conflict") {
		t.Fatalf("expected conflict %v %s", e, b)
	}
	check(t, dir, 2)
}
func TestRecoveryInterruptedAgain(t *testing.T) {
	dir := t.TempDir()
	must(t, dir, "set", "demo", "0", "create", "old")
	if _, e := invoke(dir, "-crash", "prepared", "set", "demo", "1", "change", "new"); e == nil {
		t.Fatal("expected crash")
	}
	if _, e := invoke(dir, "-crash", "event", "get", "demo"); e == nil {
		t.Fatal("expected recovery crash")
	}
	check(t, dir, 2)
}
func TestConflictingEventFailsClosed(t *testing.T) {
	dir := t.TempDir()
	must(t, dir, "set", "demo", "0", "create", "old")
	if _, e := invoke(dir, "-crash", "event", "set", "demo", "1", "change", "new"); e == nil {
		t.Fatal("expected crash")
	}
	p := filepath.Join(dir, "tasks", "demo", "history", "000000002-updated.md")
	if e := os.WriteFile(p, []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	b, e := invoke(dir, "get", "demo")
	if e == nil || !strings.Contains(string(b), "conflicting existing event") {
		t.Fatalf("expected corruption error %v %s", e, b)
	}
	b, _ = os.ReadFile(p)
	if string(b) != "corrupt" {
		t.Fatal("overwrote evidence")
	}
	if _, e = os.Stat(filepath.Join(dir, "transactions", "prepared.json")); e != nil {
		t.Fatal("lost journal")
	}
}
func TestLockTimeoutAndOwnerDeath(t *testing.T) {
	dir := t.TempDir()
	must(t, dir, "set", "demo", "0", "create", "old")
	c := command("-dir", dir, "hold", "demo", "10000")
	out, e := c.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = c.Process.Kill(); _ = c.Wait() }()
	sc := bufio.NewScanner(out)
	if !sc.Scan() || sc.Text() != "locked" {
		t.Fatal("no lock signal")
	}
	b, e := invoke(dir, "-timeout", "40ms", "get", "demo")
	if e == nil || !strings.Contains(string(b), "store_busy") {
		t.Fatalf("expected timeout %v %s", e, b)
	}
	_ = c.Process.Kill()
	_ = c.Wait()
	check(t, dir, 1)
}
func TestReaderWaits(t *testing.T) {
	dir := t.TempDir()
	must(t, dir, "set", "demo", "0", "create", "old")
	c := command("-dir", dir, "hold", "demo", "250")
	out, e := c.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Start(); e != nil {
		t.Fatal(e)
	}
	defer c.Wait()
	sc := bufio.NewScanner(out)
	if !sc.Scan() {
		t.Fatal("no lock signal")
	}
	start := time.Now()
	check(t, dir, 1)
	if time.Since(start) < 100*time.Millisecond {
		t.Fatal("reader bypassed lock")
	}
}
func TestInvalidTaskPath(t *testing.T) {
	dir := t.TempDir()
	b, e := invoke(dir, "set", "../escape", "0", "op", "title")
	if e == nil {
		t.Fatalf("accepted traversal %s", b)
	}
}

func TestFailedInstallationRecovers(t *testing.T) {
	dir := t.TempDir()
	must(t, dir, "set", "demo", "0", "create", "old")
	// An actual filesystem obstruction makes task replacement fail after event installation.
	obstruction := filepath.Join(dir, "tasks", "demo", "task.tmp")
	if e := os.Mkdir(obstruction, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(obstruction, "block"), []byte("block"), 0600); e != nil {
		t.Fatal(e)
	}
	if b, e := invoke(dir, "set", "demo", "1", "change", "new"); e == nil {
		t.Fatalf("acknowledged failed write %s", b)
	}
	if b, e := invoke(dir, "get", "demo"); e == nil {
		t.Fatalf("served partial state %s", b)
	}
	if e := os.RemoveAll(obstruction); e != nil {
		t.Fatal(e)
	}
	check(t, dir, 2)
	must(t, dir, "set", "demo", "1", "change", "new")
	check(t, dir, 2)
}
