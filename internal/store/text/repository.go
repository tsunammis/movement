//go:build darwin

// Package text provides a local, independently locked, recoverable text repository.
package text

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"time"

	"movement.local/movement/internal/app"
	"movement.local/movement/internal/contract"
	"movement.local/movement/internal/domain"
)

type Repository struct {
	Dir         string
	LockTimeout time.Duration
	Hook        func(string) error
}
type disk struct {
	root *os.Root
	hook func(string) error
}
type journal struct {
	SchemaVersion    int           `json:"schema_version"`
	Task             *domain.Task  `json:"task,omitempty"`
	Event            *domain.Event `json:"event,omitempty"`
	Receipt          app.Receipt   `json:"receipt"`
	PreviousRevision int64         `json:"previous_revision"`
	PreviousHash     string        `json:"previous_hash"`
}
type envelope struct {
	Payload  []byte `json:"payload"`
	Checksum string `json:"checksum"`
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func encode(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(b, '\n')
}
func New(dir string) *Repository { return &Repository{Dir: dir, LockTimeout: 5 * time.Second} }
func storage(err error) error {
	if err == nil {
		return nil
	}
	var d *domain.Error
	if errors.As(err, &d) {
		return err
	}
	return domain.Err("storage_error", err.Error())
}
func (d *disk) hit(name string) error {
	if d.hook != nil {
		return d.hook(name)
	}
	return nil
}
func (r *Repository) locked(ctx context.Context, fn func(*disk) error) error {
	abs, err := filepath.Abs(r.Dir)
	if err != nil {
		return storage(err)
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return storage(err)
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return storage(err)
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return storage(err)
	}
	defer root.Close()
	d := &disk{root: root, hook: r.Hook}
	// Confinement via os.Root prevents a symlink from escaping, even during path resolution.
	// Reject internal symlinks as well so lock/file identities cannot alias each other.
	if err = d.dir("runtime"); err != nil {
		return storage(err)
	}
	if err = d.safe("runtime/write.lock", true); err != nil {
		return storage(err)
	}
	f, err := root.OpenFile("runtime/write.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return storage(err)
	}
	defer f.Close()
	timeout := r.LockTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	until := time.Now().Add(timeout)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			return storage(err)
		}
		if time.Now().After(until) {
			return domain.Err("store_busy", "data directory is locked")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = d.checkStoreSchemas(); err != nil {
		return storage(err)
	}
	for _, name := range []string{"tasks", "transactions", "receipts"} {
		if err = d.dir(name); err != nil {
			return storage(err)
		}
	}
	if err = d.recover(); err != nil {
		return storage(err)
	}
	return storage(fn(d))
}
func (d *disk) safe(path string, allowMissing bool) error {
	clean := filepath.Clean(path)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("unsafe path")
	}
	parts := strings.Split(clean, string(filepath.Separator))
	for i := range parts {
		p := filepath.Join(parts[:i+1]...)
		st, e := d.root.Lstat(p)
		if os.IsNotExist(e) && allowMissing {
			return nil
		}
		if e != nil {
			return e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink forbidden: %s", p)
		}
		if i < len(parts)-1 && !st.IsDir() {
			return fmt.Errorf("non-directory path component")
		}
		if i == len(parts)-1 && !st.IsDir() && !st.Mode().IsRegular() {
			return fmt.Errorf("non-regular store entry")
		}
	}
	return nil
}
func (d *disk) dir(path string) error {
	if err := d.safe(path, true); err != nil {
		return err
	}
	err := d.root.Mkdir(path, 0700)
	if err != nil && !os.IsExist(err) {
		return err
	}
	st, e := d.root.Lstat(path)
	if e != nil {
		return e
	}
	if !st.IsDir() {
		return fmt.Errorf("expected directory %s", path)
	}
	return d.syncDir(filepath.Dir(path))
}
func (d *disk) syncDir(path string) error {
	if path != "." {
		if e := d.safe(path, false); e != nil {
			return e
		}
	}
	f, e := d.root.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	if e := d.hit("dir_sync:" + path); e != nil {
		return e
	}
	return f.Sync()
}
func (d *disk) read(path string, limit int) ([]byte, error) {
	if e := d.safe(path, false); e != nil {
		return nil, e
	}
	f, e := d.root.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Size() > int64(limit) {
		return nil, fmt.Errorf("invalid or oversized file %s", path)
	}
	b, e := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if len(b) > limit {
		return nil, fmt.Errorf("file exceeds limit")
	}
	return b, e
}
func (d *disk) write(path string, b []byte) error {
	if e := d.safe(path, true); e != nil {
		return e
	}
	f, e := d.root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return e
	}
	if e = d.hit("write:" + path); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = d.hit("file_sync:" + path)
	}
	if e == nil {
		e = f.Sync()
	}
	if e == nil {
		e = d.hit("full_sync:" + path)
	}
	if e == nil {
		_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_FULLFSYNC, 0)
		if errno != 0 {
			e = errno
		}
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
func (d *disk) remove(path string) error {
	if e := d.safe(path, true); e != nil {
		return e
	}
	if e := d.hit("remove:" + path); e != nil {
		return e
	}
	e := d.root.Remove(path)
	if os.IsNotExist(e) {
		return nil
	}
	return e
}
func (d *disk) rename(from, to string) error {
	if e := d.hit("rename:" + to); e != nil {
		return e
	}
	return d.root.Rename(from, to)
}
func receiptPath(op string) string { return "receipts/" + digest([]byte(op)) + ".json" }
func taskPath(id string) string    { return "tasks/" + id + "/task.json" }
func eventPath(ev domain.Event) string {
	return fmt.Sprintf("tasks/%s/history/%020d-%s.md", ev.TaskID, ev.Sequence, ev.Type)
}
func (d *disk) readReceipt(path string) (app.Receipt, error) {
	var result app.Receipt
	b, e := d.read(path, MaxRecordBytes)
	if e != nil {
		return result, e
	}
	if e = app.Decode(b, &result); e != nil {
		return result, e
	}
	if e = contract.Validate(reflect.TypeFor[app.Receipt](), b); e != nil {
		return result, fmt.Errorf("receipt schema: %w", e)
	}
	if result.OperationID == "" || path != receiptPath(result.OperationID) {
		return result, fmt.Errorf("receipt operation identity mismatch")
	}
	return result, nil
}
func validateReceiptTarget(r app.Receipt, b app.Bundle) error {
	if r.TaskID != b.Task.ID || r.Revision < 1 || r.Revision > int64(len(b.History)) {
		return fmt.Errorf("receipt target missing")
	}
	ev := b.History[r.Revision-1]
	if ev.EventID != r.EventID {
		return fmt.Errorf("receipt event mismatch")
	}
	if r.Existing && ev.OperationID == r.OperationID {
		return fmt.Errorf("state-changing receipt mislabeled as ingestion retry")
	}
	if !r.Existing && (ev.OperationID != r.OperationID || ev.InputDigest != r.Digest) {
		return fmt.Errorf("receipt does not match committed event")
	}
	if r.ClaimToken != "" {
		if r.Existing || ev.Type != "task_claimed" {
			return fmt.Errorf("unexpected claim receipt token")
		}
		var hash string
		if err := app.Decode(ev.Changes["claim_token_hash"], &hash); err != nil || hash != digest([]byte(r.ClaimToken)) {
			return fmt.Errorf("claim receipt does not match recorded token hash")
		}
		if b.Task.Claim != nil && b.Task.Claim.AttemptID == ev.AttemptID && digest([]byte(r.ClaimToken)) != b.Task.Claim.TokenHash {
			return fmt.Errorf("claim receipt token mismatch")
		}
	} else if !r.Existing && ev.Type == "task_claimed" {
		return fmt.Errorf("claim receipt missing token")
	}
	return nil
}
func (d *disk) receipt(op string) (app.Receipt, error) {
	r, e := d.readReceipt(receiptPath(op))
	if e != nil {
		return r, e
	}
	b, e := d.loadTask(r.TaskID)
	if e != nil {
		return r, e
	}
	return r, validateReceiptTarget(r, b)
}
func (d *disk) validateReceipts(bundles []app.Bundle) error {
	tasks := map[string]app.Bundle{}
	for _, b := range bundles {
		tasks[b.Task.ID] = b
	}
	files, e := d.entries("receipts")
	if e != nil {
		return e
	}
	receipts := map[string]app.Receipt{}
	for _, file := range files {
		r, e := d.readReceipt("receipts/" + file.Name())
		if e != nil {
			return e
		}
		b, ok := tasks[r.TaskID]
		if !ok {
			return fmt.Errorf("receipt refers to missing task")
		}
		if e = validateReceiptTarget(r, b); e != nil {
			return e
		}
		receipts[r.OperationID] = r
	}
	for _, b := range bundles {
		for _, ev := range b.History {
			r, ok := receipts[ev.OperationID]
			if !ok || r.Existing || r.TaskID != ev.TaskID || r.EventID != ev.EventID || r.Revision != ev.Revision || r.Digest != ev.InputDigest {
				return fmt.Errorf("history operation missing matching authoritative receipt")
			}
		}
	}
	return nil
}
func (d *disk) entries(path string) ([]os.DirEntry, error) {
	if e := d.safe(path, false); e != nil {
		return nil, e
	}
	f, e := d.root.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	entries, e := f.ReadDir(-1)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, e
}
func (d *disk) loadTask(id string) (app.Bundle, error) {
	var b app.Bundle
	if !domain.ValidID(id) {
		return b, fmt.Errorf("invalid task directory")
	}
	raw, e := d.read(taskPath(id), MaxRecordBytes)
	if e != nil {
		return b, e
	}
	if e = app.Decode(raw, &b.Task); e != nil {
		return b, e
	}
	if b.Task.SchemaVersion != 1 {
		return b, domain.Err("unsupported_schema", "unsupported task schema")
	}
	if e = contract.ValidateTask(raw); e != nil {
		return b, fmt.Errorf("task schema: %w", e)
	}
	if e = b.Task.Validate(); e != nil {
		return b, e
	}
	if b.Task.ID != id {
		return b, fmt.Errorf("task directory identity mismatch")
	}
	entries, e := d.entries("tasks/" + id + "/history")
	if e != nil {
		return b, e
	}
	b.History = []domain.Event{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			return b, fmt.Errorf("unexpected history entry %s", entry.Name())
		}
		raw, e := d.read("tasks/"+id+"/history/"+entry.Name(), MaxRecordBytes)
		if e != nil {
			return b, e
		}
		ev, e := parseEvent(raw)
		if e != nil {
			return b, e
		}
		if ev.SchemaVersion != 1 {
			return b, domain.Err("unsupported_schema", "unsupported history schema")
		}
		if !domain.ValidEventType(ev.Type) || ev.TaskID != id || ev.Sequence != int64(len(b.History)+1) || ev.Revision != ev.Sequence || !domain.ValidID(ev.EventID) || entry.Name() != filepath.Base(eventPath(ev)) || ev.OperationID == "" || len(ev.InputDigest) != 64 || ev.RecordedAt.IsZero() {
			return b, fmt.Errorf("history identity/order invalid")
		}
		b.History = append(b.History, ev)
	}
	if int64(len(b.History)) != b.Task.Revision || len(b.History) == 0 || b.History[len(b.History)-1].EventID != b.Task.LastEventID {
		return b, fmt.Errorf("task/history revision mismatch")
	}
	return b, nil
}
func (d *disk) snapshot() ([]app.Bundle, error) {
	entries, e := d.entries("tasks")
	if e != nil {
		return nil, e
	}
	out := []app.Bundle{}
	for _, entry := range entries {
		if !entry.IsDir() || !domain.ValidID(entry.Name()) {
			return nil, fmt.Errorf("invalid task directory entry")
		}
		b, e := d.loadTask(entry.Name())
		if e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	if e = d.validateReceipts(out); e != nil {
		return nil, e
	}
	return out, nil
}
func (r *Repository) Snapshot(ctx context.Context) (out []app.Bundle, err error) {
	err = r.locked(ctx, func(d *disk) error { var e error; out, e = d.snapshot(); return e })
	return
}
func (r *Repository) Transact(ctx context.Context, op, inputDigest string, fn func([]app.Bundle) (app.Change, error)) (result app.Receipt, err error) {
	err = r.locked(ctx, func(d *disk) error {
		receipt, e := d.receipt(op)
		if e == nil {
			if receipt.Digest != inputDigest {
				return domain.Err("operation_conflict", "operation ID was used with different semantic input")
			}
			result = receipt
			return nil
		}
		if !os.IsNotExist(e) {
			return e
		}
		all, e := d.snapshot()
		if e != nil {
			return e
		}
		change, e := fn(all)
		if e != nil {
			return e
		}
		if change.Receipt.OperationID != op || change.Receipt.Digest != inputDigest {
			return fmt.Errorf("invalid transaction receipt")
		}
		j := journal{SchemaVersion: 1, Task: change.Task, Event: change.Event, Receipt: change.Receipt}
		if j.Task != nil {
			for _, b := range all {
				if b.Task.ID == j.Task.ID {
					j.PreviousRevision = b.Task.Revision
					raw, e := d.read(taskPath(b.Task.ID), MaxRecordBytes)
					if e != nil {
						return e
					}
					j.PreviousHash = digest(raw)
				}
			}
		}
		if e = validateJournal(j); e != nil {
			return e
		}
		payload := encode(j)
		raw := encode(envelope{payload, digest(payload)})
		if len(raw) > 8*MaxRecordBytes {
			return domain.Err("invalid_input", "transaction exceeds limit")
		}
		if e = d.write("transactions/staging.tmp", raw); e != nil {
			return e
		}
		if e = d.hit("staged"); e != nil {
			return e
		}
		if e = d.rename("transactions/staging.tmp", "transactions/prepared.json"); e != nil {
			return e
		}
		if e = d.syncDir("transactions"); e != nil {
			return e
		}
		if e = d.hit("prepared"); e != nil {
			return e
		}
		if e = d.install(j); e != nil {
			return e
		}
		result = change.Receipt
		return nil
	})
	return
}
func validateJournal(j journal) error {
	r := j.Receipt
	if j.SchemaVersion != 1 {
		return domain.Err("unsupported_schema", "unsupported transaction schema")
	}
	if r.OperationID == "" || len(r.Digest) != 64 || !domain.ValidID(r.TaskID) || !domain.ValidID(r.EventID) || r.Revision < 1 {
		return fmt.Errorf("invalid journal receipt")
	}
	if (j.Task == nil) != (j.Event == nil) {
		return fmt.Errorf("incomplete journal")
	}
	if j.Task == nil {
		if !r.Existing {
			return fmt.Errorf("receipt-only transaction must reference existing task")
		}
		return nil
	}
	if e := j.Task.Validate(); e != nil {
		return e
	}
	ev := j.Event
	if !domain.ValidEventType(ev.Type) || j.PreviousRevision < 0 || j.Task.Revision != j.PreviousRevision+1 || ev.SchemaVersion != 1 || ev.TaskID != j.Task.ID || ev.EventID != j.Task.LastEventID || ev.Revision != j.Task.Revision || ev.Sequence != ev.Revision || ev.ContextVersion != j.Task.ContextVersion || ev.OperationID != r.OperationID || ev.InputDigest != r.Digest || r.TaskID != j.Task.ID || r.EventID != ev.EventID || r.Revision != ev.Revision {
		return fmt.Errorf("journal task/event/receipt mismatch")
	}
	if len(encode(j.Task)) > MaxRecordBytes {
		return domain.Err("invalid_input", "task exceeds 1 MiB")
	}
	_, e := encodeEvent(*ev)
	return e
}
func (d *disk) recover() error {
	raw, e := d.read("transactions/prepared.json", 8*MaxRecordBytes)
	if e == nil {
		var env envelope
		if e = app.DecodeWithLimit(raw, &env, 8*MaxRecordBytes); e != nil {
			return e
		}
		if digest(env.Payload) != env.Checksum {
			return fmt.Errorf("journal checksum mismatch")
		}
		if e = checkJSONSchema(env.Payload, "transaction", 8*MaxRecordBytes); e != nil {
			return e
		}
		var versions struct {
			Task  json.RawMessage `json:"task"`
			Event json.RawMessage `json:"event"`
		}
		if e = json.Unmarshal(env.Payload, &versions); e != nil {
			return e
		}
		for kind, value := range map[string]json.RawMessage{"task": versions.Task, "history": versions.Event} {
			if len(value) > 0 && string(value) != "null" {
				if e = checkJSONSchema(value, kind, MaxRecordBytes); e != nil {
					return e
				}
			}
		}
		var j journal
		if e = app.DecodeWithLimit(env.Payload, &j, 8*MaxRecordBytes); e != nil {
			return e
		}
		if e = validateJournal(j); e != nil {
			return e
		}
		if e = d.install(j); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if e = d.remove("transactions/staging.tmp"); e != nil {
		return e
	}
	return nil
}
func (d *disk) immutable(path string, b []byte) error {
	old, e := d.read(path, MaxRecordBytes)
	if e == nil {
		if !bytes.Equal(old, b) {
			return fmt.Errorf("conflicting immutable file %s", path)
		}
		if e = d.remove(path + ".tmp"); e != nil {
			return e
		}
		// An interrupted prior installation may have linked the file without
		// syncing its parent. Recovery must finish that durability step.
		return d.syncDir(filepath.Dir(path))
	}
	if !os.IsNotExist(e) {
		return e
	}
	temp := path + ".tmp"
	if e = d.remove(temp); e != nil {
		return e
	}
	if e = d.write(temp, b); e != nil {
		return e
	}
	if e = d.hit("link:" + path); e != nil {
		return e
	}
	if e = d.root.Link(temp, path); e != nil {
		return e
	}
	if e = d.remove(temp); e != nil {
		return e
	}
	return d.syncDir(filepath.Dir(path))
}
func (d *disk) install(j journal) error {
	if e := validateJournal(j); e != nil {
		return e
	}
	if j.Task != nil {
		dir := "tasks/" + j.Task.ID
		if e := d.dir(dir); e != nil {
			return e
		}
		if e := d.dir(dir + "/history"); e != nil {
			return e
		}
		old, e := d.read(taskPath(j.Task.ID), MaxRecordBytes)
		next := encode(j.Task)
		if e == nil {
			if !bytes.Equal(old, next) && digest(old) != j.PreviousHash {
				return fmt.Errorf("task changed outside transaction")
			}
		} else if !os.IsNotExist(e) || j.PreviousRevision != 0 {
			if e != nil {
				return e
			}
			return fmt.Errorf("missing original task")
		}
		event, e := encodeEvent(*j.Event)
		if e != nil {
			return e
		}
		if e = d.immutable(eventPath(*j.Event), event); e != nil {
			return e
		}
		if e = d.hit("event"); e != nil {
			return e
		}
		temp := dir + "/task.tmp"
		if e = d.remove(temp); e != nil {
			return e
		}
		if e = d.write(temp, next); e != nil {
			return e
		}
		if e = d.rename(temp, taskPath(j.Task.ID)); e != nil {
			return e
		}
		if e = d.syncDir(dir); e != nil {
			return e
		}
		if e = d.hit("task"); e != nil {
			return e
		}
	} else {
		b, e := d.loadTask(j.Receipt.TaskID)
		if e != nil {
			return e
		}
		if b.Task.Revision < j.Receipt.Revision {
			return fmt.Errorf("existing receipt refers to missing revision")
		}
	}
	if e := d.immutable(receiptPath(j.Receipt.OperationID), encode(j.Receipt)); e != nil {
		return e
	}
	if e := d.hit("receipt"); e != nil {
		return e
	}
	if _, e := d.loadTask(j.Receipt.TaskID); e != nil {
		return e
	}
	if e := d.remove("transactions/prepared.json"); e != nil {
		return e
	}
	if e := d.hit("cleanup"); e != nil {
		return e
	}
	return d.syncDir("transactions")
}
