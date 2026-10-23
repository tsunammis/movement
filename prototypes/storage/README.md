# Independent-process storage prototype

This is a macOS-only experiment, not Movement's application or final data schema. It has a separate Go module, no third-party dependencies, and only a tiny title/revision model. Use a fresh scratch directory, never real task data.

## Run

From this directory, with Go 1.27:

```sh
go test -count=1 ./...
go vet ./...
go build -o /tmp/movement-storage-spike .
```

Example (all flags precede the command):

```sh
scratch_dir=$(mktemp -d /tmp/movement-spike.XXXXXX)
/tmp/movement-storage-spike -dir "$scratch_dir" set demo 0 create-001 "Write an email"
/tmp/movement-storage-spike -dir "$scratch_dir" get demo
/tmp/movement-storage-spike -dir "$scratch_dir" -crash event set demo 1 update-001 "Draft saved"
# The preceding process is intentionally killed. This read completes recovery:
/tmp/movement-storage-spike -dir "$scratch_dir" get demo
# Retry the exact operation, including its original expected revision:
/tmp/movement-storage-spike -dir "$scratch_dir" set demo 1 update-001 "Draft saved"
```

The retry returns the original event receipt, even if later revisions exist. Changed input under the same operation ID conflicts. Operation IDs are task-scoped in this spike. `hold demo 1000` holds the directory lock for one second; `-timeout 40ms` changes lock acquisition timeout.

## Storage and protocol

```text
<scratch>/
  write.lock
  tasks/<id>/task.json
  tasks/<id>/history/000000001-updated.md
  transactions/staging.tmp
  transactions/prepared.json
```

All processes take an exclusive `flock` on the stable lock file, including readers. Kernel process exit releases the lock; processes never delete that lock file. Every command performs recovery under the lock before accessing tasks.

The single prepared journal contains exact task/event bytes and hashes, plus expected prior revision. Staging is flushed before atomic publication and directory sync. Once prepared, recovery rolls forward even if the original caller saw a failure or no response. Events are published without overwriting an existing event, then task JSON is replaced. Installed bytes and revision references are checked. Both destinations are synced before journal removal. A journal can safely replay after task installation but before cleanup; conflicting bytes fail closed and preserve the journal.

This simplifies the earlier proposed protocol: one pending journal for the directory, because all writers serialize, and no separate commit marker. Durable destination installation precedes journal deletion. Unprepared staging is discarded. Frontmatter uses JSON flow mappings, a valid YAML subset; the spike parses only this subset, not general YAML.

`-crash` sends SIGKILL to the process at `staged`, `prepared`, `event`, `task`, or `cleanup`. Recovery uses the same install path and can itself be interrupted. A failure after preparation means outcome is pending/uncertain, not rolled back; reread/retry through the recovery gate.

## What the tests exercise

- Creation and update killed at all five save checkpoints (ten cases).
- Twelve different writers with the same expected revision: one succeeds, eleven conflict.
- Twelve concurrent identical retries: all return success with one resulting event.
- Exact retry after later revisions and conflicting operation-ID reuse.
- Recovery interrupted again after installing history.
- Reader waiting for the same lock; bounded timeout; killed owner releases lock.
- Conflicting installed history fails closed without overwriting evidence.
- A filesystem obstruction fails task installation after history publication; reads fail until recovery can finish, then retry is deduplicated.
- Invalid task IDs cannot introduce path traversal.

## Limits

This validates process-crash recovery at selected checkpoints on the tested local filesystem. It does not prove power-loss durability: `File.Sync` and directory sync are used, but hardware flush guarantees and macOS full-sync behavior have not been established. There is no disk-full/short-write simulator or arbitrary-instruction crash coverage.

The root is caller-owned and trusted. This is not hardened against hostile symlinks or concurrent manual file edits. It is not a performance benchmark, full schema implementation, migration system, MCP server, HTTP server, or UI. All history is scanned for receipts; global serialization is intentionally simple. A corrupt pending journal blocks the whole directory, with manual repair outside this experiment.

Do not promote this code wholesale into production. Reuse the verified protocol and tests while implementing the real repository contracts, secure path handling, resource limits, and diagnostics.

## Recorded run: 2026-10-22

Environment: `go1.27.3 darwin/arm64`, local macOS scratch directories.

- `GOWORK=off GOCACHE=/tmp/movement-go-cache go test -count=1 ./...`: PASS, 3.933s, including ten crash subcases and the installation-obstruction test.
- `GOWORK=off GOCACHE=/tmp/movement-go-cache go vet ./...`: PASS.
- `GOWORK=off GOCACHE=/tmp/movement-go-cache go build -o /tmp/movement-storage-spike .`: PASS.
- Separate built-executable smoke: create revision 1, SIGKILL after revision-2 history publication, recover via a fresh get process, retry the original operation, verify exactly two events: PASS.

Outcome: independent-process coordination is viable for the tested process-crash/retry cases. No production-readiness or power-loss guarantee is claimed.
