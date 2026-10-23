//go:build darwin

// A deliberately small storage experiment, not Movement's application repository.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Task struct {
	ID        string `json:"id"`
	Revision  int    `json:"revision"`
	Title     string `json:"title"`
	LastEvent string `json:"last_event_id"`
}
type Event struct {
	ID        string `json:"event_id"`
	TaskID    string `json:"task_id"`
	Revision  int    `json:"revision"`
	Operation string `json:"operation_id"`
	Digest    string `json:"input_digest"`
	Title     string `json:"title"`
}
type Journal struct {
	Task       Task   `json:"task"`
	Event      Event  `json:"event"`
	Previous   int    `json:"previous_revision"`
	TaskBytes  []byte `json:"task_bytes"`
	EventBytes []byte `json:"event_bytes"`
	TaskHash   string `json:"task_hash"`
	EventHash  string `json:"event_hash"`
}
type Store struct {
	root, crash string
	timeout     time.Duration
}

var safeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}$`)

func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func encode(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(b, '\n')
}
func eventBytes(e Event) []byte {
	// JSON flow mappings are a subset of YAML. No YAML dependency in this spike.
	b, _ := json.Marshal(e)
	return []byte("---\n" + string(b) + "\n---\n\nUpdated task title.\n")
}
func parseEvent(b []byte) (Event, error) {
	var e Event
	parts := strings.SplitN(string(b), "---\n", 3)
	if len(parts) != 3 || parts[0] != "" {
		return e, errors.New("invalid event frontmatter")
	}
	err := json.Unmarshal([]byte(parts[1]), &e)
	return e, err
}
func (s Store) path(p ...string) string { return filepath.Join(append([]string{s.root}, p...)...) }
func syncDir(p string) error {
	f, e := os.Open(p)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func mkdir(p string) error {
	if err := os.Mkdir(p, 0700); err != nil {
		if !os.IsExist(err) {
			return err
		}
		st, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe directory")
		}
		return nil
	}
	return syncDir(filepath.Dir(p))
}
func writeSync(p string, b []byte) error {
	f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
func (s Store) hit(point string) {
	if s.crash == point {
		_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		select {}
	}
}
func (s Store) locked(fn func() error) error {
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return err
	}
	// Caller-owned trusted temporary root only. This is not a hostile-filesystem implementation.
	f, e := os.OpenFile(s.path("write.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	deadline := time.Now().Add(s.timeout)
	for {
		e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if e == nil {
			break
		}
		if e != syscall.EWOULDBLOCK && e != syscall.EAGAIN {
			return e
		}
		if time.Now().After(deadline) {
			return errors.New("store_busy")
		}
		time.Sleep(5 * time.Millisecond)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	for _, dir := range []string{"tasks", "transactions"} {
		if e = mkdir(s.path(dir)); e != nil {
			return e
		}
	}
	if e = s.recover(); e != nil {
		return e
	}
	return fn()
}
func (s Store) recover() error {
	b, e := os.ReadFile(s.path("transactions", "prepared.json"))
	if e == nil {
		var j Journal
		if e = json.Unmarshal(b, &j); e != nil {
			return fmt.Errorf("corrupt journal: %w", e)
		}
		if e = s.install(j); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	// Temporary files are never committed; prepared journal contains all replay bytes.
	if e = os.Remove(s.path("transactions", "staging.tmp")); e != nil && !os.IsNotExist(e) {
		return e
	}
	return nil
}
func (s Store) read(id string) (Task, error) {
	var t Task
	b, e := os.ReadFile(s.path("tasks", id, "task.json"))
	if e != nil {
		return t, e
	}
	if e = json.Unmarshal(b, &t); e != nil {
		return t, e
	}
	if t.ID != id || t.Revision < 1 {
		return t, errors.New("corrupt task identity/revision")
	}
	return t, nil
}
func (s Store) events(id string) ([]Event, error) {
	entries, e := os.ReadDir(s.path("tasks", id, "history"))
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Event{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		b, e := os.ReadFile(s.path("tasks", id, "history", entry.Name()))
		if e != nil {
			return nil, e
		}
		ev, e := parseEvent(b)
		if e != nil {
			return nil, e
		}
		if ev.TaskID != id || ev.Revision != len(out)+1 || entry.Name() != fmt.Sprintf("%09d-updated.md", ev.Revision) || ev.ID != fmt.Sprintf("%s-%09d", id, ev.Revision) {
			return nil, errors.New("corrupt history sequence/identity")
		}
		out = append(out, ev)
	}
	return out, nil
}
func (s Store) validate(id string) (Task, []Event, error) {
	t, e := s.read(id)
	if os.IsNotExist(e) {
		t = Task{ID: id}
	} else if e != nil {
		return t, nil, e
	}
	events, e := s.events(id)
	if e != nil {
		return t, nil, e
	}
	if len(events) != t.Revision {
		return t, nil, errors.New("state/history mismatch")
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		if t.LastEvent != last.ID || t.Title != last.Title {
			return t, nil, errors.New("state/history mismatch")
		}
	}
	return t, events, nil
}
func (s Store) install(j Journal) error {
	if !safeID.MatchString(j.Task.ID) || j.Task.Revision != j.Previous+1 || j.Previous < 0 || j.Event.TaskID != j.Task.ID || j.Event.Revision != j.Task.Revision || j.Event.ID != j.Task.LastEvent || j.Event.ID != fmt.Sprintf("%s-%09d", j.Task.ID, j.Task.Revision) || j.Event.Title != j.Task.Title || hash(j.TaskBytes) != j.TaskHash || hash(j.EventBytes) != j.EventHash || !bytes.Equal(j.TaskBytes, encode(j.Task)) || !bytes.Equal(j.EventBytes, eventBytes(j.Event)) {
		return errors.New("corrupt journal validation")
	}
	dir := s.path("tasks", j.Task.ID)
	if e := mkdir(dir); e != nil {
		return e
	}
	if e := mkdir(filepath.Join(dir, "history")); e != nil {
		return e
	}
	current, e := s.read(j.Task.ID)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	if current.Revision != j.Previous && current.Revision != j.Task.Revision {
		return errors.New("unexpected revision during recovery")
	}
	if current.Revision == j.Task.Revision {
		b, e := os.ReadFile(filepath.Join(dir, "task.json"))
		if e != nil {
			return e
		}
		if !bytes.Equal(b, j.TaskBytes) {
			return errors.New("conflicting recovered task")
		}
	}
	eventPath := filepath.Join(dir, "history", fmt.Sprintf("%09d-updated.md", j.Task.Revision))
	if b, e := os.ReadFile(eventPath); e == nil {
		if !bytes.Equal(b, j.EventBytes) {
			return errors.New("conflicting existing event")
		}
	} else if os.IsNotExist(e) {
		tmp := filepath.Join(dir, "history", "event.tmp")
		if e = os.Remove(tmp); e != nil && !os.IsNotExist(e) {
			return e
		}
		if e = writeSync(tmp, j.EventBytes); e != nil {
			return e
		}
		if e = os.Link(tmp, eventPath); e != nil {
			return e
		} // Atomic no-overwrite publication.
		if e = os.Remove(tmp); e != nil {
			return e
		}
	} else {
		return e
	}
	if e = syncDir(filepath.Join(dir, "history")); e != nil {
		return e
	}
	s.hit("event")
	tmp := filepath.Join(dir, "task.tmp")
	if e = os.Remove(tmp); e != nil && !os.IsNotExist(e) {
		return e
	}
	if e = writeSync(tmp, j.TaskBytes); e != nil {
		return e
	}
	if e = os.Rename(tmp, filepath.Join(dir, "task.json")); e != nil {
		return e
	}
	if e = syncDir(dir); e != nil {
		return e
	}
	s.hit("task")
	if _, _, e = s.validate(j.Task.ID); e != nil {
		return e
	}
	// No separate commit marker: destinations are durable before journal removal.
	if e = os.Remove(s.path("transactions", "prepared.json")); e != nil {
		return e
	}
	s.hit("cleanup")
	return syncDir(s.path("transactions"))
}
func (s Store) set(id, title, op string, expected int) (Event, error) {
	var result Event
	e := s.locked(func() error {
		t, events, e := s.validate(id)
		if e != nil {
			return e
		}
		// Expected revision participates in input identity; retries reuse original inputs.
		digest := hash(encode(struct {
			ID, Title string
			Expected  int
		}{id, title, expected}))
		for _, ev := range events {
			if ev.Operation == op {
				if ev.Digest != digest {
					return errors.New("operation_conflict")
				}
				result = ev
				return nil
			}
		}
		if t.Revision != expected {
			return errors.New("revision_conflict")
		}
		t.Revision++
		t.Title = title
		t.LastEvent = fmt.Sprintf("%s-%09d", id, t.Revision)
		result = Event{t.LastEvent, id, t.Revision, op, digest, title}
		j := Journal{Task: t, Event: result, Previous: expected, TaskBytes: encode(t), EventBytes: eventBytes(result)}
		j.TaskHash = hash(j.TaskBytes)
		j.EventHash = hash(j.EventBytes)
		stage := s.path("transactions", "staging.tmp")
		if e = writeSync(stage, encode(j)); e != nil {
			return e
		}
		s.hit("staged")
		if e = os.Rename(stage, s.path("transactions", "prepared.json")); e != nil {
			return e
		}
		if e = syncDir(s.path("transactions")); e != nil {
			return e
		}
		s.hit("prepared")
		return s.install(j)
	})
	return result, e
}
func run(args []string) error {
	fs := flag.NewFlagSet("storage-spike", flag.ContinueOnError)
	dir := fs.String("dir", "", "required scratch data directory")
	crash := fs.String("crash", "", "SIGKILL at staged, prepared, event, task, cleanup")
	timeout := fs.Duration("timeout", 2*time.Second, "lock timeout")
	if e := fs.Parse(args); e != nil {
		return e
	}
	a := fs.Args()
	if *dir == "" || len(a) < 2 || !safeID.MatchString(a[1]) {
		return errors.New("usage: storage-spike -dir DIR [-crash PHASE] set ID EXPECTED_REV OP TITLE | get ID | hold ID MILLISECONDS")
	}
	if *crash != "" && !map[string]bool{"staged": true, "prepared": true, "event": true, "task": true, "cleanup": true}[*crash] {
		return errors.New("unknown crash phase")
	}
	s := Store{*dir, *crash, *timeout}
	switch a[0] {
	case "set":
		if len(a) != 5 || a[3] == "" {
			return errors.New("set requires ID REV OP TITLE")
		}
		rev, e := strconv.Atoi(a[2])
		if e != nil || rev < 0 {
			return errors.New("invalid revision")
		}
		ev, e := s.set(a[1], a[4], a[3], rev)
		if e != nil {
			return e
		}
		fmt.Print(string(encode(ev)))
	case "get":
		return s.locked(func() error {
			t, _, e := s.validate(a[1])
			if e != nil {
				return e
			}
			if t.Revision == 0 {
				return errors.New("not_found")
			}
			fmt.Print(string(encode(t)))
			return nil
		})
	case "hold":
		if len(a) != 3 {
			return errors.New("hold requires ID MILLISECONDS")
		}
		ms, e := strconv.Atoi(a[2])
		if e != nil || ms < 0 {
			return errors.New("invalid hold time")
		}
		return s.locked(func() error { fmt.Println("locked"); time.Sleep(time.Duration(ms) * time.Millisecond); return nil })
	default:
		return errors.New("unknown command")
	}
	return nil
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
