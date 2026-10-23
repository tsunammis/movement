# Implementation status

Updated: 2026-10-23. MVP implementation and local acceptance are complete for the documented platform and the client scope accepted in decision 005. Codex Desktop remains expected but unverified, as a non-blocking follow-up. Earlier entries below are chronological evidence and may describe gates that were later closed. No commit or publication has been performed.

## Completed foundation

- Root Go module: `movement.local/movement`, Go 1.27.
- `internal/domain`: typed task, guidance, criteria, events and reports; validation; lifecycle actions; evidence aggregation; planning/execution selection; carryover/overdue reasons; follow-up timing; pagination; calendar-date/DST conversion.
- `internal/app`: shared create/update/claim/renew/release/report/check/observe use cases behind a repository interface; semantic context version; completion/claim cleanup; store-wide receipt contract; ingestion preservation; late observations and reconciliation; context/history reads.
- Strict request decoding rejects unknown and duplicate JSON fields, excessive nesting and ambiguous trailing values.
- Four final review contract resolutions incorporated in the spec. Tests exercise context freshness, observed-vs-received follow-up timing, claim cleanup and receipt lookup before generated creation IDs.

Verification: `GOWORK=off GOCACHE=/tmp/movement-go-cache go test -race ./...` and `go vet ./...` passed for the domain/application packages. Application tests use a transactional in-memory test repository; they do not prove filesystem correctness. Prototype subprocess tests remain separate from the root module.

## Implemented durable repository and CLI

- `internal/store/text`: confined filesystem access, symlink rejection, bounded reads, safe YAML frontmatter, independent-process locks, prepared-journal replay and atomic logical task/event/receipt commits.
- Store-wide receipts cover creation retries, claims and receipt-only ingestion deduplication.
- `cmd/movement` and CLI adapter: initialization, all task mutations, list/get/context/history, date-only timezone conversion, structured requests and stable JSON errors.
- Root race-enabled suite and `go vet` passed. Production repository subprocess tests cover twelve crash checkpoints, contention, identical retries, interrupted recovery, corruption, symlink escape and lock timeout. A built CLI completed create/claim/report/seems_completed/confirm/history and read a copied directory successfully.

See [CLI documentation](cli.md). The earlier repository/CLI tasks below are now implemented at this scope; remaining hardening and formal contracts remain part of the full goal.

## Implemented MCP adapter

- `internal/adapters/mcp` exposes all twelve specified tools through the official Go MCP SDK v1.7.0; `movement mcp --stdio` uses the configured repository without a daemon.
- Schemas describe typed tool inputs; shared application decoding/validation preserves revision, claim and receipt semantics. Domain errors are structured MCP tool errors.
- SDK client tests use a real temporary text repository. Built-binary JSON-RPC smoke verified initialization, discovery, creation/retry, CLI readback and clean EOF with protocol-only stdout.
- Root `go test -race ./...`, `go vet ./...` and binary build pass. Actual desktop client acceptance remains pending. See [MCP documentation](mcp.md).

## Implemented local HTTP API

- `internal/adapters/http` exposes the shared task operations, config and browser session endpoints. CLI adds `serve` and explicit `auth token` access.
- Loopback-only binding, exact Host/Origin validation, bearer credentials, eight-hour in-memory sessions, CSRF, bounded JSON bodies and stable error status mapping.
- HTTP tests cover authentication, browser session/CSRF/expiry/logout, retries, revision conflicts and query/input validation against the text repository. Root race suite, vet and binary build pass.
- `scripts/http-smoke.py` passed against the built binary: real loopback listener, HTTP create, independent CLI readback and graceful shutdown. Sandbox escalation was needed for loopback binding.
- See [HTTP documentation](http.md). UI assets, OpenAPI and Vite browser checks are implemented below; full cross-interface mutation acceptance remains pending.

## Implemented embedded UI and build pipeline

- `web/`: pinned React 19.3.1, TypeScript 7.0.2 and Vite 8.3.5 with a lockfile, structured task editor, inbox/planning/waiting-for/undated views, filters, guidance, delegation, evidence/history, lifecycle confirmation and observation acknowledgement.
- Browser login uses the session/CSRF API. Conflict recovery retains unsaved input and sends only changed fields, preserving unrelated concurrent edits. Source prose is rendered as text and links are restricted to HTTP/HTTPS.
- `internal/web` embeds generated assets. Hash navigation supports direct task links/refresh; unknown static/API routes keep error responses. `scripts/build.sh` runs locked installation, type-checking, clean frontend build and CGO-disabled Go compilation in order.
- Playwright Chrome tests passed against packaged and Vite servers (2 tests, 6.6s): delegated creation, concurrent edit/revision conflict, evidence-backed seems-completed review, confirmed completion, history, session reload/logout, CSRF rejection and narrow viewport without horizontal overflow. No external requests or browser runtime errors occurred. The mobile screenshot was visually inspected.
- Frontend type-check/build and the full build script passed using cached pinned dependencies. Final root Go race tests and vet passed. A clean source copy without generated assets or dependencies built successfully; an injected TypeScript error stopped the release before changing the Go binary. See [UI documentation](ui.md).

## Agent context and evidence hardening

- Context packets now have a 4 MiB JSON bound, at most ten recent events, explicit unresolved-observation totals and continuation offsets. Default observation page size is ten (maximum fifty). Continuations require the original task revision, preventing skips when concurrent reconciliation changes the unresolved set.
- CLI, MCP, HTTP and UI expose the same pagination. The UI loads further observations and shows the count; agents are instructed to inspect every unresolved page before repeating external effects.
- Accepted checks/completion assessments must have a strictly newer observation time within their semantic context. Older/equal-time evidence returns `assessment_conflict` and can be preserved through observations. Original committed retries remain idempotent.
- Late observations validate nested evidence/output timestamps and evaluated context/revision linkage. Tests cover pagination, large evidence, delayed/equal-time assessment conflicts and fresh reconciliation. Root race tests, vet and frontend type-check/build passed. Expanded packaged/Vite browser tests also passed (2 tests, 9.7s), including twelve late observations, page continuation and acknowledgement on a completed task.

## Formal contracts and generated examples

- `internal/contract` derives structural schemas from shared Go models, including validated patch fields. `cmd/contracts` emits task/history JSON Schemas and OpenAPI; tests detect stale generation and missing operations/references. MCP tool discovery shares the schema builder.
- The repository validates stored task/event schemas and uses the specified sequence-and-event-type history filenames. This aligns an unreleased implementation detail with the spec; earlier scratch directories are not automatically migrated.
- `cmd/examples` generates synthetic bundles through the production service, with valid IDs, semantic versions, digests, history and store-wide receipts. Repository tests load every bundle and retry its original creation without changing history. No external source access or real Gmail action occurs.
- Root race tests, vet, schema generation checks, fixture readback/retry tests, documentation links and a CGO-disabled binary build passed. Packaged/Vite browser suites passed again after runtime schema enforcement (2 tests, 10.4s).
- [Acceptance audit](acceptance.md) maps all A1–A25 scenarios and non-scenario requirements to evidence and remaining proof. It is deliberately not a completion declaration.

## Domain identity and storage failure hardening

- Criterion removals retain their definitions in immutable history; a removed ID cannot be restored with a different description. Reports now validate actual evaluated revision/context linkage and require claims to predate their evaluated revision.
- Recovery now syncs parent directories for already-installed identical immutable files. This closes an interruption gap after link installation but before directory sync.
- Sixteen injected write/sync/rename/link/removal failure cases verify visible errors, old-state preservation before preparation, roll-forward afterward and idempotent retry. Persistent recovery faults block readers and retain the journal. A partial unprepared journal is discarded safely.
- Snapshot/retry paths validate authoritative receipt links. Missing/changed receipts cannot duplicate creation. Claim-token hashes in events validate historical claim receipts after release. Ambiguous or unknown-field prepared JSON is rejected without changing task state or deleting the evidence.
- Targeted tests, the full Go race suite, vet and a CGO-disabled binary build pass. Packaged/Vite browser suites also pass against this binary (2 tests, 10.2s). Generated synthetic fixtures now contain claim-token hash metadata as produced by the service; source definitions remain unchanged.

## Remaining full-scope work

- Broaden production failure/corruption validation as needed; finish backup/restore and migration contracts.
- Actual assistant client checks, broader MCP mutation acceptance, broader HTTP acceptance and OpenAPI validation against live response fixtures.
- Broader UI acceptance: personal draft/blocker/observation workflows, all needed domain actions, keyboard-only navigation, unsafe-source fixtures, HMR and additional browser coverage.
- macOS release target selection, reproducible builds, packaged-binary acceptance and full A1-A25 audit.

The goal remains active. No commits have been created. No production readiness, complete acceptance coverage, or power-loss guarantees are claimed.

## Browser execution claims

- Added claim acquire/renew/release and deliberate replacement controls using the shared HTTP mutation contract, expected revisions, operation IDs and claim tokens. Lease inputs enforce the service's 30–3600 second range. Failed claim/revision preconditions expose explicit refresh before retrying.
- Claim tokens stay in the open detail component's memory, survive ordinary refresh/renewal, and clear on task navigation, editing, reload or workspace lock. An existing token from another client can be entered explicitly; it is not persisted in browser storage.
- Extended the browser suite with actual cross-client claim contention, incorrect-token rejection, renewal/release, explicit takeover, and reload/logout clearing. Both packaged and Vite modes passed. Full browser suite: 5 passed, 1 intentionally skipped (packaged-only scenario), 15.8s. Frontend type-check/build, CGO-disabled binary build, and Go web/HTTP tests passed.
- R7 remains incomplete until structured work/check reporting is available in the UI. Backup/restore and actual-client/platform acceptance remain separate gates; this change does not imply full MVP completion.

## Structured browser work and check reports

- Added work/check report forms with per-criterion outcomes, multiple evidence entries, general evidence, produced artifacts, explicit observation times, next steps, check follow-up scheduling and selected observation reconciliation. Check outcomes derive from the criterion results and remain validated by the shared service. Work completion proposals enter review rather than confirmed done.
- Forms retain the evaluated task revision/context/instruction versions, including across refresh. Conflicts preserve input and expose current context for review. Changed instructions/attempts prevent ordinary resubmission; the original report can be retained as a late observation at a reviewed write revision. A new assessment requires reopening and reassessing current context.
- A lost-response browser test exposed a race where the next form could open before the committed revision refreshed. Report saves now refresh context before closing the form. Retrying a committed report uses the same operation ID and appends only one event.
- Frontend type-check/build, CGO-disabled binary build and Go web/HTTP tests passed. Full browser suite passed: 7 tests, 1 intentionally skipped packaged-only case, 23.0s. Additional report-specific narrow-screen/keyboard checks passed in packaged and Vite modes (2 tests, 9.1s); the retained mobile screenshot was visually inspected.
- Browser evidence includes progress with lost response/retry, blocking and claim release, insufficient-criteria rejection, artifact-backed completion proposal, stale-check rejection without losing text, preservation as observation, and current-context reconciliation. No external source or artifact is accessed by these tests.
- Remaining acceptance still includes standalone historical-observation entry, broader UI query parity, backup/restore, retained packaging checks, actual assistant clients, and the advertised platform/browser matrix. These results do not declare R7 or the full MVP complete.

## Stopped-store backup and restore

- Added `movement verify`: recovery plus complete task/event/receipt validation and summary counts. Added `movement auth rotate`: validated-store HTTP credential replacement through a private synced temporary file and atomic rename. Rotation does not print the token or change task/receipt/claim records.
- Added the stopped-writer backup/restore procedure, including complete-directory copies, source preservation, credential/session handling, pending transactions, uncertain rotation failure, external-side-effect reconciliation, and schema-v1 upgrade constraints. Running HTTP servers must be stopped; rotation is not live revocation of their cached credentials or sessions.
- Repository tests copy an interrupted prepared operation, recover the destination, replay its receipt, rotate credentials and confirm source bytes remain unchanged. Claim-receipt replay and release remain valid after copied-store HTTP rotation. Injected token write/sync/rename failures return errors and remain retryable; corrupt stores cannot silently rotate their credential.
- CLI maintenance tests passed. Packaged browser restore test passed (1 test, 3.4s): copied task/receipt equality, new token, old bearer rejection, old-session rejection and offline embedded UI rendering without external requests.
- Full `go test -race ./...` and `go vet ./...` passed, including repository suite (26.089s); the subsequently added restored-claim case passed race-enabled separately (1.837s). CGO-disabled binary build passed. Unsupported-version no-write tests, full UI parity, retained packaging acceptance and actual client/platform support still remain.

## Unsupported-schema recovery guard

- Found and fixed a startup ordering gap: recovery previously ran before config/task/history version rejection. An unrelated pending v1 transaction could therefore install before a mixed-version record was discovered.
- The repository now checks committed config/task/history versions under the shared lock before recovery or staging cleanup. Prepared transactions check their own and nested task/event versions before installation. Future additional fields do not mask the `unsupported_schema` error by reaching current typed decoding first.
- Retained tests cover six future-version locations (config, task, history, journal, nested journal task/event), mixed supported/unsupported tasks, unrelated pending recovery, and unprepared staging. Snapshot/settings/init/verify/rotate/create/committed-retry operations all reject while every file's bytes remain unchanged. Traversal-like task IDs reject both read and mutation without changing store files.
- Full Go race tests and vet passed (repository suite 31.955s). Targeted unsupported-version/traversal race tests passed (4.742s). CGO-disabled binary build passed. Packaged/Vite browser regression suite passed: 8 tests, 2 intentional skips for packaged-only scenarios, 25.4s.
- Spec, architecture, backup procedure and A10 audit now describe this enforced boundary. Remaining work includes full UI query/observation parity, retained packaging checks, actual assistant clients and the supported platform/browser matrix; full goal completion is still unproven.

## Retained release packaging acceptance

- Added `tests/acceptance/packaging.py` and its isolated Chrome harness. The test copies current source without dependencies/assets/binary, invokes `scripts/build.sh`, injects a frontend type error plus a Go-invocation sentinel, and verifies the previous executable remains intact. A subsequent clean build removes a planted stale asset.
- The test copies only the built executable into a runtime directory and deletes its temporary source/build/dependency tree. CLI and HTTP run with an empty PATH (Node undiscoverable). Tests verify task creation/readback, embedded JS/CSS, authentication, correct missing API/static responses and hash refresh.
- A separate Chrome harness logs in, opens the direct task URL, reloads and edits the task without external requests or page errors. The isolated CLI reads the edit back. Node/Playwright are test infrastructure outside the server's environment, not application runtime dependencies.
- Executed successfully with `GOWORK=off GOCACHE=/tmp/movement-go-cache GOMODCACHE=/tmp/movement-go-modcache npm_config_cache=/tmp/movement-npm-cache npm_config_offline=true python3 tests/acceptance/packaging.py`. All five stages passed. Only temporary copies were built; the working tree's binary/assets were not changed.
- Current executed environment, queried directly: macOS 26.7.1 (25G241), arm64; Go 1.27.3; Node 24.14.1; installed Chrome 155.0.8059.85. This records the test environment rather than claiming older OSes, Intel, Safari, Firefox or actual assistant clients have been validated.

## Browser planning and execution selection

- Added selection controls for planning/executable work, independent overdue and earlier-deadline carryover flags, undated inclusion, explicit RFC 3339 evaluation time and page size. Cards display inclusion reasons, eligibility at evaluation time and expired-claim reconciliation hints from the shared query response.
- Pagination keeps the returned evaluation time. A request-generation guard discards page responses after filters change; a delayed old page cannot append into a new selection.
- Added packaged/Vite acceptance fixtures for overdue blocked work, future carryover, exact inclusive/exclusive bounds, the last hour of the 25-hour Europe/Paris DST day, undated tasks and active/expired claims. Tests exercise independent flags, execution eligibility, pagination and a deliberately delayed page during a filter change.
- Frontend type-check/build, CGO-disabled binary build and Go web/HTTP tests passed. Full browser suite passed: 10 tests, 2 intentional packaged-only skips, 31.4s. Spec, UI documentation and A4/A23 evidence now include this coverage.
- Standalone historical-observation entry and final interface audit remain, along with actual assistant-client and advertised platform/browser verification. The full goal is still active.

## Actual assistant-client acceptance

- Executed the packaged binary through installed Claude Code 2.1.308 and Codex CLI 0.163.0, each with temporary MCP configuration and synthetic data. Existing authentication was used; global MCP config was not changed. Built-in/unrelated capabilities and session persistence were disabled for the test runs.
- Claude Code connected and enumerated all twelve Movement tools. Both clients made the prescribed five calls: create, identical create retry, get, deliberately stale update, history. The update returned structured `revision_conflict`; neither client retried it with a new revision. Codex's structured and text response bodies matched.
- Independent CLI readback confirmed exactly one unchanged task at revision 1 and one matching creation event. No real source connector, external artifact or personal task was used.
- Added and executed `tests/acceptance/clients.py` for both clients. Assertions use actual tool-call/result streams and persisted records, not model-reported success. Both retained runs passed. The runner fails unexpected tool calls and kills its process group on timeout.
- MCP documentation now distinguishes tested CLI clients from untested Claude Desktop/Codex Desktop setup and routines. Added `docs/installation.md` with the executed macOS/CPU/browser environment, build/runtime requirements and backup/upgrade links. These results do not certify Desktop clients or complete the full goal.

## Standalone historical observations in the UI

- Added reviewed import of an assistant's original report object on any task, including completed/dismissed tasks. It preserves evaluated context/instruction versions, original attempt, timestamps, evidence and output references. Submission is append-only and does not reopen or complete work, reschedule checks, or take over a claim.
- Original JSON is sent through strict server validation rather than being reserialized in a way that hides duplicate keys. A preview exposes the summary and evaluated revision before saving. Conflicts retain the complete text and require reviewing/adopting the current write revision.
- Retry identity survives whitespace/key-order reformatting and unsuccessful intermediate edits. A lost response followed by retry appends one observation. Browser tests confirm raw duplicate-key rejection, unchanged terminal status/priority, retained artifact references, conflict recovery, and exact event counts.
- Frontend type-check/build, CGO-disabled binary build and Go web/HTTP tests passed. Full browser suite passed (12 tests, 2 intentional skips, 35.4s); after strengthening per-payload retry retention, the affected packaged/Vite cases passed again (2 tests, 6.5s).
- The UI now exposes standalone historical evidence as well as current reports, claims and selection controls. Requirement-by-requirement final audit and the remaining platform/browser/Desktop evidence still determine full completion; no goal completion is claimed here.

## UTC normalization and retry semantics

- Typed create/update/report/check/observation timestamps now normalize to UTC before input hashing. This covers sources, delegation, nested criterion evidence, artifacts and next-follow-up values. Patch history uses the normalized values; omitted and explicit null remain distinct, and prose is preserved.
- Task normalization copies pointer-backed scheduling/delegation/claim values and source slices before changing them, avoiding mutation of a prior task snapshot.
- Service tests cover equivalent-offset create/update/check receipt replay, no duplicate history, no semantic-context increment for an offset-only edit, stored task/check/change timestamps, nested report normalization and patch presence. Domain/application/repository targeted suites passed. Full `go test -race ./...` and `go vet ./...` passed (repository race suite: 29.936s). These tests do not substitute for the remaining packaged adapter and Desktop acceptance audit.

## Built-interface UTC evidence

- Added `Acceptance.test_utc_storage_and_offset_equivalent_retries` to the retained built-binary harness. Each CLI/MCP/HTTP writer creates and updates timestamp-bearing tasks and records work, checks and historical observations. After process restart, all three readers replay all operations with different offsets and receive the original receipts.
- The test verifies unchanged semantic context for an offset-only edit, exactly six events per task, preserved timestamp-like prose, UTC readback and direct task JSON / Markdown frontmatter timestamp values. The rebuilt CGO-disabled binary passed this targeted scenario (3.877s).
- Source-level [UI parity audit](reviews/2026-10-23-ui-parity.md) found partial-check scope expansion and retained-delegation removal during unrelated edits, plus remaining capture/context/history option gaps. These remain implementation work, not accepted exclusions.

- Full rebuilt-binary interface suite passed: `python3 tests/acceptance/interfaces.py /absolute/path/to/bin/movement`, 9 tests in 51.943s. This includes real separate-process races, actual lease expiry, restart/retry, delegated/personal workflows and the new timestamp matrix. Temporary loopback listeners required sandbox escalation; all task data was synthetic and temporary.

## UI check-scope and delegation preservation

- Fixed unrelated task edits expanding partial check scope to every criterion and clearing retained delegation on actionable tasks. Criterion checkboxes now expose scope explicitly; existing selections survive unrelated edits, new criteria remain unselected for an existing check, and removed IDs are removed from check references.
- Added explicit delegation retention/removal and editable RFC 3339 delegation time. Empty optional person references are omitted, preserving the original semantic context on a priority-only edit. New delegation time is stable across save retries.
- Added packaged/Vite regressions with independent CLI readback, exact history changes, check-version assertions, keyboard criterion selection, criterion addition/removal, flow transitions and explicit delegation removal. Frontend type-check/build and CGO-disabled binary build passed.

- Full packaged/Vite browser suite passed: 14 tests, 2 intentional packaged-only skips, 39.1s. Go web and HTTP tests passed. The new editor scenario passed in both modes with independent CLI/history verification; remaining R7 gaps are listed in the UI parity audit.

## UI context diagnostics and recorded changes

- Task details now render `missing_context` from the shared context packet as actionable guidance. Diagnostics update after edits and do not imply manual tracking is blocked.
- Added a typed history-change view for field edits, lifecycle actions, cleared values and structured coordination metadata. Native details/summary supports keyboard expansion; all values render as escaped React text.
- Extended packaged/Vite regression coverage for exact missing-context membership, diagnostic updates, priority-change history, keyboard expansion, inert malicious-looking nested prose and narrow-screen layout. Both targeted cases passed (8.2s); frontend type-check/build and rebuilt embedded binary passed.

- Full packaged/Vite browser suite passed: 14 tests, 2 intentional packaged-only skips, 39.3s. Go web/HTTP tests and edited-document link validation passed. Capture/query/claim-option parity and actual Desktop compatibility remain tracked work.

## UI candidate capture and discovery identity

- Added ready/candidate capture selection, optional stable discovery identity fields, read-only identity display for existing tasks, and editable RFC 3339 source capture timestamps. Identity encoding matches the canonical server JSON array, including escaped HTML-sensitive characters. Existing identities remain immutable.
- Packaged/Vite tests capture a candidate, accept it, manually rename/reprioritize it, correct source capture time, dismiss it and rediscover it. Independent CLI readback verifies unchanged task/history on rediscovery; a new occurrence creates a distinct candidate. Tests also cover UTC timestamp storage and identity references containing `&`, `<` and `>`.
- Targeted browser scenarios passed in both modes (2 tests, 7.4s); frontend type-check/build and rebuilt CGO-disabled binary passed.

- Full browser suite passed: 16 tests, 2 intentional packaged-only skips, 48.0s. Go web/HTTP tests and whitespace checks passed. Remaining UI query/claim/batch-acknowledgement options and Desktop compatibility are not claimed complete.

## UI batch observation acknowledgement

- Added explicit selected-observation checkboxes and batch acknowledgement using the shared operation-ID/revision contract. Selection and reason survive revision conflicts and failed responses; conflicts require refresh/review before resubmission. Pagination never selects unseen observations automatically.
- History now exposes acknowledgement references, using event sequence numbers when their records are loaded and stable event IDs otherwise.
- Packaged/Vite tests select evidence across pages, induce a concurrent edit, preserve/review selection, lose a committed response, and retry. Readback proves exactly one batch event, only the selected observations resolved, all others retained, and terminal business state/context unchanged. The targeted scenarios passed (2 tests, 11.8s). Frontend type-check/build and rebuilt embedded binary passed.

- Related structured-report and historical-import browser regressions passed in packaged/Vite modes (4 tests, 13.1s). Go web/HTTP tests and whitespace checks passed. No full-suite rerun was needed for unaffected editor/query workflows; their prior evidence remains separately recorded. Query/claim-option parity and Desktop acceptance remain unfinished.

## UI token-authorized edits and transitions

- Shared the selected task's in-memory token with its editor and lifecycle controls. A current token authorizes edits without replacing the claim; obsolete tokens remain rejected. Reload, task navigation, new capture and workspace lock clear the token, and removal of a claim clears it after refresh. No token is persisted to browser storage.
- Corrected the shared service's active-claim flow-change handling: even a valid token cannot change flow until the claim is released or explicitly overridden, matching the existing spec. Service tests verify byte-equivalent state/history after rejection and both allowed paths, using the same operation ID after the failed precondition.
- Extended browser claim coverage for token retention across priority edits, unchanged attempt metadata, active-flow rejection, obsolete-token edit rejection, logout clearing and completion with the current token without override.

- Full packaged/Vite browser suite passed: 16 tests, 2 intentional packaged-only skips, approximately 1 minute. Full Go race suite and vet passed (store suite: 36.430s), as did frontend type-check/build and whitespace checks. Query-option parity, broader keyboard/platform review and actual Desktop acceptance remain open.

## UI exact timeframe and status-set queries

- Added multiple-status selection, exact RFC 3339 boundary inputs alongside repository-timezone calendar dates, and person filtering across all views. Timeframe format changes reset boundaries explicitly.
- Incomplete windows/empty custom status sets do not fall back to unfiltered queries. Loading and invalid-query transitions clear old rows, while existing request epochs prevent late pagination responses from entering a newer result set.
- Packaged/Vite query tests verify status combinations, offset-equivalent boundaries, an exclusive upper bound and a one-microsecond extension, direct API result parity, invalid-input stale-row removal, format resets and all-view person filtering. Both targeted cases passed (8.6s); frontend type-check/build and rebuilt embedded binary passed.

- Full packaged/Vite browser suite passed: 16 tests, 2 intentional packaged-only skips, 51.8s. Go web/HTTP tests and whitespace checks passed. Final UI operation/keyboard audit, tested-support matrix, storage-size measurements and actual Desktop acceptance remain pending.

## Keyboard-only workflow and skip navigation

- Fixed the skip link changing the hash route to `#workspace`, which could close the selected task and clear its claim token. It now focuses a programmatically focusable workspace without changing the route.
- Added a packaged/Vite keyboard-only scenario using sequential Tab, Enter and native select type-ahead. No mouse clicks, test-driven focus calls or scripted field values are used for interaction. It covers login, execution claim/release, skip navigation with route/token preservation, candidate capture, acceptance, priority edit, history expansion, completion, reopening and workspace lock.
- Both scenarios passed (8.7s). Narrow-view screenshot inspection confirmed readable stacked content, visible workspace focus outline and no horizontal overflow. Tested native Chrome select type-ahead (`i` for Inbox, `h` for High); arrow/menu simulation did not change values in this headless environment, so the test does not claim that sequence was verified. Frontend type-check/build and rebuilt binary passed.

## Undated delegated work and storage baseline

- Extended undated-only filtering to all UI views, preserving the personal Undated shortcut. Switching views resets this filter explicitly. Packaged/Vite scenarios compare delegated undated results with the API, exclude dated commitments, and verify view-reset behavior.
- Targeted browser scenarios passed (2 tests, 10.0s); frontend type-check/build and rebuilt CGO-disabled binary passed.
- The independent-process measurement harness completed at 10/40 and 50/200 initial tasks/events. All operations and final task/event/receipt verification succeeded. See [measurements](reviews/storage-measurements.md) for raw evidence, method and limitations; no capacity guarantee is inferred.
- Claude Desktop's isolated test configuration is installed with an original-settings backup. Automatic approval review rejected quitting the app; restart approval remains pending. No Desktop tool-call acceptance is claimed.

- Full rebuilt packaged/Vite browser suite passed: 18 tests, 2 intentional packaged-only skips, 1.0 minute. Go web/HTTP tests passed. Measurement artifact counts were independently checked; Claude configuration readback confirms only the approved isolated server entry differs from the original. Desktop restart/test remains pending approval.

## Current requirement and packaging audit

- Inspected the specification's R1–R11, interface table and additional persistence/context/HTTP contracts; recorded evidence and remaining gaps in [implementation audit](reviews/implementation-audit.md). Spec/architecture now identify the actual tested macOS arm64 target rather than leaving platform selection ambiguous.
- Full Go race suite and vet pass (unchanged packages used the test cache). Built CLI/MCP/HTTP acceptance passes all 9 scenarios in 56.117s, including actual lease expiry and nested UTC disk checks.
- Clean-copy packaging passes all five stages, including frontend failure gating, stale-asset cleanup, isolated runtime without Node/source/assets, and actual browser navigation/edit with no external requests.
- Actual Desktop client acceptance remains unfinished. Claude's required restart is awaiting approval; no new client-setting changes were made in this audit.

## Desktop restart follow-up

The user reports restarting Claude Desktop. Native desktop access failed with `Sky Computer Use native pipe startup failed`; resetting the control session and requesting inventory produced the same error and no available app surfaces. Read-only configuration inspection found that Claude's config now exactly matches the saved original backup: the temporary Movement server entry is absent. The test fragment, prompt and binary remain available, and the isolated store still contains zero task records. No settings were changed during this follow-up, and no Desktop compatibility result is claimed. Resume by restoring desktop-control access, then reinstalling the already-authorized isolated entry and verifying that Claude loads it before executing the synthetic prompt.

## Claude Desktop connection verified

After the user installed the isolated test entry, Claude's dedicated MCP log records server startup and successful connection, an `initialize` request/response, `notifications/initialized`, and a `tools/list` request/response on 2026-10-23 at 08:49 UTC. Configuration readback confirms the intended executable and isolated data directory. This proves the Desktop transport/discovery exchange, not task execution. Native UI control still fails with `Sky Computer Use native pipe startup failed`; the synthetic tool workflow requires user interaction until desktop control is restored.

## Claude Desktop smoke workflow verified

Claude Desktop 1.49585.0 completed the user-assisted five-call synthetic workflow. Dedicated logs show discovery and five tool-call exchanges. Independent CLI verification and receipt inspection prove exactly one unchanged task at revision 1, one creation event and one receipt. The user-provided summary reports identical retry responses and `revision_conflict`; raw result bodies are not present in the log. See [evidence](reviews/claude-desktop-acceptance.md) for attribution and limits. Removed the temporary MCP configuration entry while preserving unrelated settings; did not restart Claude. Desktop context/report/routine workflows and Codex Desktop remain unverified.

## Claude Desktop guided execution verified through proposal

Independent readback verifies Project Cedar at revision 4 / `seems_completed`, no claim, four expected events, retained draft text, matching version/attempt references and evidence for both criteria. Human confirmation is intentionally pending. Added the missing outcome enumeration to the shared report schema and clarified work/check outcome sets in MCP descriptions; regenerated formal contracts. See the [Desktop evidence](reviews/claude-desktop-acceptance.md).

## Claude Desktop workflow complete through human confirmation

Independent get/history/verify checks confirm Project Cedar at `done`, revision 5, no claim, unchanged context/guidance/completion versions, five events and retained synthetic draft/evidence. Final operation `cedar-confirm-20261023-01` explicitly records `seems_completed` to `done`. Total isolated store: two tasks, six events, six receipts. Claude Desktop's user-assisted task execution and human-confirmation path now has recorded evidence; actual external integrations/routine scheduling were not exercised, and Codex Desktop remains untested.

## Local implementation acceptance closed

The user accepted Codex Desktop testing as a non-blocking follow-up in [decision 005](decisions/005-initial-client-acceptance.md). Rebuilt `bin/movement` through the actual build script with locked dependencies, frontend type-check/build and CGO disabled, including the report-outcome schema fix. Final `go test -race ./...` passed (repository suite 38.807s); `go vet ./...` passed; the rebuilt-binary CLI/MCP/HTTP harness passed all 9 scenarios in 66.874s. Retained browser evidence is 18 passes / 2 intentional packaged-only skips; the frontend is unchanged since that run. Clean-copy packaging previously passed all five stages, and the build pipeline is unchanged.

The [requirement audit](reviews/implementation-audit.md), A1–A25 matrix and actual Claude Desktop workflow cover the accepted initial scope. Supported evidence is macOS 26.7.1 arm64, Chrome 155.0.8059.85, Claude Desktop 1.49585.0, Claude Code 2.1.308 and Codex CLI 0.163.0. Larger capacity, other platforms/browsers, real external integrations and client-specific scheduling are not certified. Work remains uncommitted and unpublished.
