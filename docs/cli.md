# Movement CLI

Implemented on macOS. See [implementation status](implementation-status.md) for current adapter coverage and remaining acceptance.

## Build and initialize

From the repository root, with Go 1.27:

```sh
go build -o bin/movement ./cmd/movement
bin/movement --data-dir /path/to/tasks init --timezone Europe/Paris
```

The directory is resolved from `--data-dir`, then `MOVEMENT_DATA_DIR`, then `~/.movement`. Global flags precede the command. Initialization writes configuration and a private local API token; it preserves existing settings/token on subsequent runs. Timezone defaults to UTC when omitted.

## Capture and read

Create payload (save as `task-input.json`):

```json
{
  "title": "Prepare project update",
  "status": "ready",
  "context": {"objective": "Create a draft for review", "constraints": ["Do not send"], "notes": ""},
  "completion": {"version": 1, "criteria": [{"id": "draft", "description": "An unsent draft exists"}]}
}
```

```sh
bin/movement task create --op capture-001 --input task-input.json
bin/movement task list --json
bin/movement task get TASK_UUID --json
bin/movement task context TASK_UUID
bin/movement task history TASK_UUID --offset 0 --limit 50
```

Creation returns a receipt with task ID, revision and event ID. Repeat the same operation ID and semantic payload after an uncertain response to recover the original receipt. Do not generate a fresh operation ID for a retry. Input `-` reads JSON from stdin. Default actor is `user`; assistants should supply an explicit client label with `--actor`.

## Update and work

```sh
bin/movement task update TASK_UUID --revision 1 --op edit-001 --input patch.json
bin/movement task claim TASK_UUID --revision 2 --op claim-001 --actor assistant
```

Example patch: `{"fields":{"priority":"high"}}`. Lifecycle actions use `{"action":"complete"}`, `accept`, `dismiss`, `reopen`, `resume`, or `needs_more_work`.

Claim returns an opaque claim token. Subsequent claimed operations supply `--claim-token TOKEN` and the latest expected revision. Work reports use `task report`; checks use `task check-report`; late evidence uses `task observe`. Their input is the corresponding request-data object from the spec (for reports/checks, `{"report":{...}}`). `task renew` accepts `{"seconds":900}`; `task release` needs no payload. Claims default to 15 minutes, bounded to 30–3600 seconds.

`--override` is an explicit user override of the current execution claim. External side effects remain the assistant's responsibility. A failed Movement report is not evidence that Gmail failed to save a draft.

For a full structured command, `movement request --input request.json` accepts `app.Request`: operation_id, kind, task_id, actor, expected_revision, optional claim_token/override, and data. This is also useful for programmatic automation.

## Selection

```sh
bin/movement task list --from 2026-10-19 --to 2026-10-26 --flow all --json
bin/movement task list --view execution --json
bin/movement task list --query query.json --json
```

Date-only boundaries use the initialized timezone and an exclusive end date. RFC 3339 boundaries are also supported. JSON queries expose the full planning/execution, date, follow-up, carryover, status, pagination and assignee contract. Reuse returned `as_of` and `next_cursor` as `cursor` with unchanged filters for the next page. Without a timeframe, list includes undated tasks; timeframe queries exclude them by default.

## Errors

Errors are JSON on stderr. Exit status: 0 success; 2 invalid input; 3 not found; 4 revision/context/operation/claim/transition conflict; 5 store busy; 1 storage, initialization or other failure. Do not blindly retry stale writes. Uncommitted revision conflicts can retry with refreshed preconditions; committed operations return their original receipt before checking current preconditions.

## Data and limitations

Tasks live in `tasks/<uuid>/task.json`; Markdown history uses YAML frontmatter. Store-wide operation receipts are private text files in `receipts/`. Those records include claim result tokens so lost-response retries can recover them; back them up with the tasks. They are authoritative durable receipts, not disposable indexes. The journal commits state, event and receipt together. Repeated-ingestion receipt-only transactions preserve task history.

History/event/task records are limited to 1 MiB each. Pending journals are limited to 8 MiB. YAML rejects duplicate keys, anchors, aliases, custom tags, non-string mapping keys, and nesting beyond 32. All access is confined through `os.Root` and internal symlinks are rejected. Process coordination uses a stable lock, including reads; full scans are used initially.

File writes use sync plus macOS full-sync before publication, and directory metadata is synced. Tests establish selected process-crash recovery behavior, not an independently demonstrated hardware power-loss guarantee. Use a local filesystem and stop writers before copying a backup. A corrupt pending transaction fails closed; no automatic destructive repair or migration is implemented.


Context pagination: `task context ID --offset N --limit 10 --revision R` continues unresolved observations from the first packet. The initial call needs no revision. Follow `next_observations_offset` until absent; a concurrent change requires restarting from a fresh packet. The packet is capped at 4 MiB and 10 recent events; `recent_history_truncated` points readers to paginated history. `assessment_conflict` uses conflict exit code 4: retain delayed evidence with `observe`, then reevaluate before applying a fresh report.

## Validation and HTTP credential rotation

`movement --data-dir PATH verify` recovers pending work and validates all task/history/receipt links, returning counts as JSON. It fails on corruption and does not silently discard evidence. It is not a forensic read-only command or a lock held for a later copy.

With all HTTP servers stopped, `movement --data-dir PATH auth rotate` validates the store and replaces its HTTP credential without printing it. Read the new token explicitly with `auth token`. Restarted servers reject the old token and old browser sessions; already-running servers retain their in-memory credentials until stopped. Claim tokens and retry receipts are unchanged. Follow the complete [backup and recovery procedure](backup-and-recovery.md).
