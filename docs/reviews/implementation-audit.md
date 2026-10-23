# Implementation acceptance audit

This audit maps the current specification to inspected implementation and executed evidence. Claude Desktop now has user-assisted smoke and full synthetic workflow evidence. The user accepted Codex Desktop testing as a non-blocking follow-up on 2026-10-23; it is expected to work but remains unverified. The numbered A1–A25 detail is maintained in [acceptance](../acceptance.md).

## Requirement map

| Requirement | Implementation inspected | Executed evidence and conclusion |
| --- | --- | --- |
| R1 text storage | `internal/store/text`: canonical paths, confined root, OS lock, prepared journal, immutable Markdown/YAML events, JSON tasks and receipts | Repository race/crash/I/O/schema/receipt tests pass in the Go race suite. Built adapter harness verifies persisted records after restart. Implemented for the tested local macOS filesystem; hardware power loss is not established. |
| R2 capture/provenance/retries | Shared service create/ingestion, domain source validation, store-wide receipts | Adapter harness verifies occurrence identity, unchanged replay and UTC disk values; browser capture tests verify user decisions survive rediscovery. Covered. |
| R3 review/edit | Shared update/lifecycle validation, editor and transition controls | Browser conflicts/preservation, accepted/dismissed/completed rediscovery, active-token edits and explicit override tests pass. Covered. |
| R4 selection | Shared domain query, adapter query mapping, UI filters | Domain DST/boundary tests, adapter parity and browser status/instant/undated/delegate/pagination tests pass. Covered. |
| R5 context/claims | Service context pagination/budget; claim/renew/release; UI claim controls | Bounded context and stale-page tests, independent claim race, actual expiry/replacement, obsolete tokens and browser ephemeral-token workflows pass. Covered, including user-assisted Claude Desktop context/claim execution and independent stored-state verification. |
| R6 history/results | Typed reports, checks, observations, acknowledgements and lifecycle transactions | Adapter/blocker/report tests, late-observation/reconciliation scenarios and structured-payload crash recovery pass. Browser history exposes recorded changes and evidence. Covered. |
| R7 equivalent operations | CLI, MCP and HTTP share service; UI capture/query/editor/detail/ClaimControls/ReportControls/ObservationForm | Every operation in the specification interface table has a UI path in the [operation matrix](2026-10-23-ui-parity.md). Browser scenarios cover normal, stale and uncertain-response paths. Covered, including the recorded Claude Desktop smoke and guided workflow. |
| R8 self-contained executable | `scripts/build.sh`, Go embedded assets, CGO-disabled build | Clean-copy packaging test passes: frontend failure stops Go, stale assets removed, isolated binary runs without Node/source/assets, real browser login/navigation/edit makes no external requests. Covered on macOS 26.7.1 arm64 with Chrome 155.0.8059.85. |
| R9 delegation | Separate flow/delegate/deadline/follow-up fields and waiting-for view | Nathan scenarios verify filtering, no execution claims, preserved delegation after completion, and undated delegated selection. Covered. |
| R10 checks | Shared authoritative criteria and versioned check instructions; no source executor | A–D met/not-met/inconclusive, stale/partial/unknown/duplicate criteria, observation ordering, atomic follow-up and UI structured-check tests pass. Covered. |
| R11 personal agent guidance | Context guidance, capabilities, constraints, report outputs and explicit completion | Kevin fake-connector scenarios prove one unsent draft, blocker handling and reconciliation after uncertain creation. Browser displays guidance and artifact evidence. Covered with fake external connectors and actual Claude Desktop synthetic guidance/report/confirmation. No real external action is claimed. |

## Additional contracts and artifacts

- The Go race suite includes generated-contract equality against `api/task.schema.json`, `api/history-event.schema.json` and `api/openapi.yaml`, example bundle readback/retry, retired criterion identity, evaluated-revision linkage, delayed assessment rejection and offset-equivalent timestamp retries.
- Unsupported schemas block recovery/mutation without rewriting authoritative data. Corrupt receipts cannot be treated as new operations. Prepared recovery and injected I/O failures remain fail-visible; power-loss guarantees are not inferred.
- HTTP tests cover authentication, exact Host/Origin, CSRF, session expiry/restart and input limits. Vite and packaged browser tests exercise the session path; unsafe source text remains inert.
- Backup/restore tests preserve receipts and claims while rotating copied HTTP credentials. The documented stopped-store procedure requires stopping every writer before copying.
- Keyboard-only and narrow-view evidence covers the retained task workflow. The current full browser suite passes in packaged/Vite modes: 18 passed, with 2 scenarios intentionally run only against the packaged server.
- Storage measurements at 10 and 50 tasks are recorded separately. They establish small-store observations, not a maximum size or latency guarantee.

## Current verification run

- `go test -race ./...`: passed; unchanged packages used Go's test cache.
- `go vet ./...`: passed.
- `python3 tests/acceptance/interfaces.py`: 9 passed, 56.117 seconds, including a real 30-second lease expiry.
- `python3 tests/acceptance/packaging.py`: all five stages passed, including clean build, failure gating, isolated runtime and real-browser checks.
- Full rebuilt browser suite from the preceding verification pass: 18 passed, 2 intentional skips, 1.0 minute. No runtime source changes followed that run.

## Accepted client scope and follow-ups

On 2026-10-23 the user accepted initial client coverage from Claude Desktop 1.49585.0, Claude Code 2.1.308 and Codex CLI 0.163.0. Claude Desktop's user-assisted context/claim/progress/proposal/confirmation path passed, with independent readback through revision 5 / done; see [evidence](claude-desktop-acceptance.md).

Codex Desktop is expected to work through MCP but remains unverified. Its setup/tool workflow test is a non-blocking follow-up, not a claimed result. Client-specific routine scheduling is external to Movement and remains unverified. Larger stores, other macOS versions/CPUs and other browsers are not advertised as tested.

No commit or publication is implied by local implementation acceptance.

## Final rebuild verification

After the outcome schema fix and decision 005, the actual build script passed with locked dependencies, frontend type-check/build and CGO-disabled binary compilation. Full Go race suite passed (store 38.807s), vet passed, and all 9 built-interface scenarios passed in 66.874s. No UI or build-pipeline behavior changed since the recorded 18-pass browser and five-stage isolated packaging checks. R1–R11 and A1–A25 are satisfied at the accepted initial platform/client scope. Local implementation acceptance is complete; no publication or commit was performed.
