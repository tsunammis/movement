# Decision register

Unless noted below, entries are proposed, not accepted. Stan is the initial decision owner. Once accepted, create a numbered decision record from [the template](../../templates/decision.md), with date, rationale, alternatives, and consequences.

| ID | Decision | Recommendation | Tradeoff / alternative | Needed before |
| --- | --- | --- | --- | --- |
| D1 | Packaging and language | Go modular monolith, embedded HTML/JS | Small runtime footprint; separate binaries per OS/architecture | Slice 1 |
| D2 | Authoritative text format | Task JSON plus separate Markdown events with YAML frontmatter | Readable narrative history; requires recoverable multi-file writes | Slice 1 |
| D3 | Concurrency | Accepted: independent processes with shared filesystem locking | Production tests cover selected process-crash and I/O recovery cases; hardware power loss is not certified | Slice 1 |
| D4 | Task triage and completion | Accepted: distinct seems_completed state after agent investigation; confirmed done remains separate | Preserves user decisions; adds review steps; allow agent actions when user-authorized | Slice 1 |
| D5 | Timeframe | Accepted: schedule/deadline window plus overdue by default; exclude undated by default | Predictable filtering; no automatic workload planning | Slice 1 |
| D6 | Supported systems | Accepted: macOS only initially | Tested target: macOS 26.7.1 arm64; other versions/CPUs are unverified and other OSes deferred | Slice 1 |
| D7 | Editing files | Accepted: Movement interfaces are the normal path; files freely readable; supported writes through Movement | Preserves history and conflict guarantees; arbitrary live edits are unsupported | Slice 1 |
| D8 | MCP transport | stdio first; Streamable HTTP later if needed | Easy client-owned process; client compatibility must be tested | Slice 2 |
| D9 | Execution boundary | Assistants own source access, scheduling, and actions | Keeps Movement small; external tools control their own permissions | Slice 2 |
| D11 | Delegation model | Separate `waiting_for` flow from lifecycle status; store assignee and expected outcome | Tracks commitments without requiring multi-user accounts | Slice 1 |
| D12 | Completion verification | External assistant runs a saved prompt and records criterion evidence; explicit completion remains separate | Supports arbitrary sources; results depend on access and observation time | Slice 2 |
| D10 | Browser UI | Accepted: Vite + TypeScript + React; built assets embedded in Go | Dedicated development/build tooling; no extra runtime dependency | Slice 4 |

D4 (intermediate completion state), D5, D6, and D7 were accepted by Stan on 2026-10-22 in the discussion resolving the four spec questions. See [decision record 001](001-product-defaults.md). Triage and completion edge cases remain draft where not explicitly covered by that decision.

D10 was accepted by Stan on 2026-10-22. See [decision record 002](002-ui-development.md). This supersedes the earlier plain HTML/small-JavaScript proposal.

The final review resolutions are recorded in [decision 003](003-spec-review-resolutions.md): shared criteria, late observations, planning/execution selection, stable ingestion identity, and follow-up scheduling. D3 was subsequently resolved in [decision 004](004-independent-process-storage.md); the user selected independent processes and authorized the prototype.

D2 now reflects the storage direction requested for this documentation revision on 2026-10-20; recovery mechanics are implemented and tested under the subsequently authorized implementation plan.

Explicit project constraints such as local operation and text ownership are recorded in the constitution. This register addresses choices within those constraints.

Initial client acceptance scope was accepted on 2026-10-23 in [decision 005](005-initial-client-acceptance.md): Codex Desktop testing is a non-blocking follow-up, with compatibility expected but unverified.
