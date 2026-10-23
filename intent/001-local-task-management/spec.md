# Specification: Movement MVP

Status: implementation authorized by user goal on 2026-10-22 | Owner: Stan | Created: 2026-10-19

Upstream: [intent](intent.md). Design: [architecture](../../docs/architecture.md). Accepted choices are recorded in the decision register; remaining detailed contracts are draft.

## Requirements and behavior

| ID | Requirement |
| --- | --- |
| R1 | Store task state and history as versioned UTF-8 text in a configurable local directory |
| R2 | Capture candidate tasks with source provenance and safe retries |
| R3 | Review and edit tasks without ingestion replacing user decisions |
| R4 | Select tasks for an explicit timeframe with predictable inclusion reasons |
| R5 | Return enough context for an agent to act and coordinate work claims |
| R6 | Persist progress, blockers, results, evidence, and completion decisions |
| R7 | Expose equivalent domain operations through CLI, MCP, HTTP, and web UI |
| R8 | Package the local services and web assets in a portable executable |
| R9 | Track delegated commitments with a responsible person, deadline, and “waiting for” flow |
| R10 | Supply a completion-check prompt and record evidence-based check results independently of task execution |
| R11 | Guide external assistants through personal tasks and record resulting artifacts within the user’s action scope |

## Supported environment and access

Initial releases support macOS only. Linux and Windows are outside the initial scope; portable packaging remains a design goal. The current tested target is macOS 26.7.1 on Apple Silicon (arm64), with Chrome 155.0.8059.85 for browser acceptance. Older macOS versions and Intel are not advertised as supported without execution evidence; see the [installation support scope](../../docs/installation.md).

Use Movement CLI, MCP, HTTP API, or web UI as the normal path for reading and changing tasks. Files remain freely readable for inspection, export, and backup. Supported writes go through Movement to preserve validation, history, and recovery; direct file editing is not the normal workflow.

## Storage process model: accepted independent processes

CLI commands, stdio MCP servers, and the local HTTP server run as independent processes sharing the same application/repository code and configured data directory. CLI and MCP require no running HTTP server or storage daemon. The browser uses the HTTP server; every process accesses storage through the shared repository implementation.

All cooperating readers and writers acquire the same stable, directory-wide OS filesystem lock with a bounded wait. Never delete or replace its lock file while the store is in use. Under the lock, recover pending transactions before reading state/history or checking mutation preconditions. Process death releases the OS lock; on timeout return store_busy without starting a mutation. Each process must resolve the same canonical data-directory path. Only local filesystems are supported initially.

A state-changing mutation prepares one journal containing the next task JSON, immutable Markdown history event, and store-wide operation receipt, validates checksums and expected prior revision, then installs and syncs all three before deleting the journal and acknowledging success. Repeated ingestion with a new operation ID commits a receipt-only transaction without changing the task or adding an event. Unprepared staging is discarded; a prepared operation rolls forward after interruption. Readers through Movement cannot observe the intermediate pair. A failure after preparation can mean pending commit, not rollback: clients reread/retry with the same operation ID. Conflicting recovery data fails visibly without overwriting evidence. See the [architecture](../../docs/architecture.md) for protocol details.

This is the selected MVP topology, not an option awaiting comparison with a daemon. Independent processes still serialize repository access through one lock; this choice does not promise parallel writes. External source access and agent execution must happen outside the repository lock.

The choice applies to every adapter: an in-process mutex alone is insufficient, and HTTP must not become a hidden prerequisite for CLI or MCP. Acceptance must exercise separate OS processes against one directory, including a reader arriving after an interrupted writer. A future change to a daemon requires a new decision, not an adapter-specific persistence shortcut.

The user selected independent processes; [decision 004](../../docs/decisions/004-independent-process-storage.md) records the prototype evidence. The macOS arm64 experiment verified selected process-crash, contention, retry, and recovery cases. It does not establish power-loss durability, hostile-filesystem protection, performance at scale, or the full application schema. Production hardening must address these separately; full application acceptance is not implied by the prototype.

## Implementation contract resolutions

The implementation uses `context_version` independently of storage revision. It increments on changes to objective, constraints, notes, description, sources, flow/delegation, authoritative completion, execution guidance, or check configuration. Schedule, priority, tags, claims, and observations do not increment it. Reports must name the evaluated current context version; expected revision separately guards the write. A successful state transition out of actionable/in_progress closes any execution claim atomically. Claim renewal validates both state and attempt.

Follow-up `pending` means an unsatisfied non-null schedule, including future schedules; `due` additionally means follow_up_at <= as_of. Only a current-context accepted check whose observed_at is at or after the scheduled instant satisfies it. Future observation timestamps are invalid. Observation receipt time does not substitute for source observation time.

Operation IDs are store-wide. Resolve committed receipts before creation ID generation or current revision/claim checks. The digest hashes operation type, target when known, actor and semantic JSON input using deterministic Go JSON encoding after strict typed decoding and default normalization. Exclude expected revision, transport credentials, and claim tokens; include evaluated context versions/attempts as semantic inputs. Omitted optional fields normalize to their typed defaults, while omitted and null patch values remain distinct. Normalize every typed input timestamp to UTC before computing the digest: different RFC 3339 offsets representing the same instant are equivalent retry inputs. This includes source capture, delegation, nested report/evidence/output timestamps and next-follow-up values. Normalize typed patch values in recorded changes too; do not rewrite timestamp-like text in prose. A failed precondition creates no receipt, so clients may retry with refreshed preconditions. A committed identical operation returns its original receipt; changed semantic input conflicts. Repeated ingestion returns the existing task without adding history; its operation receipt still must be persisted in a separate receipt record under the same lock if the new operation ID has not previously committed.

## Task record

Required: `schema_version`, UUID `id`, monotonic `revision`, semantic `context_version`, nonempty `title`, `status`, `created_at`, `updated_at`, `created_by`, `description`, `context`, `completion`, `sources`, and `last_event_id`. Empty collections and empty prose are allowed; an empty title is not.

Optional delegation/check fields are defined below; `flow` defaults to `actionable` when omitted.

Optional: `priority` (low/normal/high, default normal), tags, `scheduled_for`, `due_at`, `claim`, and an ingestion identity `(producer, external_key)`. Timestamps are RFC 3339 instants with offsets, stored normalized to UTC. No inferred due date from vague prose.

Context contains `objective`, `constraints`, and `notes`. `completion` is the single authoritative success definition: a positive `version` and `criteria` entries with stable `id` and `description`. Increment its version when criteria change; retain IDs for the same criterion, and never reuse a removed ID for a different meaning. Empty criteria are allowed for capture/manual tasks but cannot support an execution claim or agent completion transition. Sources contain a nonblank provider, nonblank stable external reference or URI, nonzero capture time, and a relevant excerpt or summary bounded to 65,536 UTF-8 bytes. Empty summary prose is allowed; incomplete provenance and oversized summaries are rejected before committing state or history. References are not fetched automatically. Multiple source items can support one task; one source item can support multiple distinct tasks.

Current state lives in `tasks/<id>/task.json`. History lives in `history/<sequence>-<type>.md`, one Markdown event with YAML frontmatter per mutation. Event metadata includes schema version, task/event ID, sequence, actor, recorded time, type, operation ID, input digest, and resulting revision. Typed report data, changes, evidence, and outputs belong in frontmatter; narrative notes belong in the Markdown body. Event metadata records the operation identity and digest. A separate authoritative store-wide receipt retains the original response, including a claim token when applicable, and also supports receipt-only ingestion. Commit it through the same recovery protocol; do not duplicate history or receipts in task JSON. Preserve events for the life of the task. Actor labels identify reporting clients, not independently verified human identities. See the [data model](../../docs/data-model.md) and architecture recovery protocol.

Schema upgrades require explicit migration with backup; reject writes to newer unsupported schema versions. Under the shared lock, check config, committed task/history and prepared-transaction versions before recovery or staging cleanup. In a mixed-version store, fail the whole operation (including committed retries) with `unsupported_schema`; leave authoritative records and pending transaction files unchanged. Validate embedded task/history versions in prepared transactions too. Opening lock/runtime metadata for coordination does not authorize rewriting data. No automatic destructive migration at startup. Size limits and migration procedures must be finalized in the storage slice.

## Personal tasks and execution guidance

The user's own tasks use `flow=actionable`. An assistant acting on the user's behalf is the execution actor, not a delegate in the waiting-for flow. An optional `execution_guidance` object has `version`, `prompt`, ordered `steps`, `input_refs`, `required_capabilities`, and `expected_outputs`. Increment its version on edit. `context.objective` describes the outcome, `context.constraints` bounds actions, and `completion.criteria` defines success; `completion_check` separately describes how to verify success. Guidance may reference the criteria but should not introduce conflicting definitions. Conflicting or incomplete instructions require clarification.

Example: write an email to Kevin by an explicit Friday deadline. Read the specified Jira project, inspect its deadline and delivery risks, summarize relevant comments, and create a Gmail draft for review. Resolve Kevin's email address and the project reference before acting. Do not send the email. A source-access failure is a blocker; the assistant must not fabricate a status summary.

An external user request or routine starts the assistant. `task_context` returns guidance and required capabilities without claiming those capabilities exist in the client. After reading context and obtaining an execution claim, the assistant uses its available authorized Jira/Gmail tools and submits `task_report`. The report includes `attempt_id`, `completion_version`, `guidance_version`, `task_revision_used` (the context revision after claiming), summary, outcome, evidence, and `outputs` entries with kind, provider, stable reference, creation/observation time, and description. Guidance versions must match current instructions. Normal expected-revision and claim rules apply; a lease renewal may advance the revision without changing guidance. A changed instruction requires rereading and reassessing, not blind reuse of a prior plan.

Record a draft reference and the fact that it was saved, not sent. If draft creation cannot be confirmed, report uncertainty rather than success. Reconcile external artifacts before retrying uncertain side effects: local operation IDs deduplicate Movement reports, not Gmail actions. If the external draft exists but the report conflicts, retain its reference and reconcile the task without recreating the draft. A `completion_proposed` work report uses the same criterion-result/evidence structure and aggregate validation as a check report. It requires a nonempty current criterion set with every criterion met; partial or unknown results must be reported as progress/blocked. A validated report moves the task to `seems_completed`; it does not mark it `done`. Sending would require separate user authorization beyond this task's draft-only guidance.

No new execution endpoint is needed: create/update stores guidance, context exposes it, report records outcomes, and history exposes the evidence. The same contract is available through CLI, MCP, HTTP, and UI. Absence of execution guidance is valid for manually performed tasks and is surfaced as missing context when an agent needs it.

## Delegation and flows

`flow` is separate from lifecycle `status`: proposed values are `actionable` (default) and `waiting_for`. A delegated task can be ready to track, in progress, blocked, or done without losing its delegation metadata. “Waiting for” is a user-facing view of nonterminal tasks with that flow, not a claim that the delegate has started work. Candidate tasks stay in the inbox until accepted.

`waiting_for` requires `delegation.assignee` (a display name, with an optional stable reference), `delegation.delegated_at`, and `delegation.expected_outcome`. `due_at` is the delivery deadline. Optional `follow_up_at` is when the user wants to check again; it does not replace the deadline. A delegate is a referenced person, not another Movement account. Assigning them in Movement does not send them a message or grant access.

Moving a task between flows or changing its delegate is explicit, revision-checked, and logged. Do not derive `in_progress` from delegation alone. Execution claims are rejected for `waiting_for` tasks: an agent checking Nathan’s delivery must not become the owner of Nathan’s work. Changing flow while an execution claim is active requires release or explicit override first. Done/dismissed tasks retain their flow and delegate in history and disappear from the active waiting-for view.

Resolve natural language such as “by Friday” into a visible date, timezone, and deadline convention before saving. If the date or time is ambiguous, request clarification rather than silently choosing. The example uses an explicitly illustrative Friday at 17:00 Europe/Paris; this is not a global default.

## Completion-check instructions and observations

Any task may have `completion_check`: a `version` (incremented on edit), a user-authored `prompt`, `criterion_ids` referencing `completion.criteria`, and `source_refs`. These are distinct from instructions to perform the task. The prompt specifies where to look and how to assess completion; it does not run inside Movement. Missing check instructions are surfaced as missing context. Check configuration cannot define a second set of criteria. Unknown or duplicate criterion IDs are invalid. Partial checks are allowed but never establish overall completion: unassessed authoritative criteria are `unknown`.

Example prompt: “Open https://foo.bar/in and check whether the document contains sections A, B, C, and D. Report each section as present, absent, or unknown, with a supporting excerpt or location. Report met only if all four are present. If the page cannot be accessed or the content is incomplete, report inconclusive. Do not edit the page or contact Nathan.” This example checks section presence only. Requiring nonempty content, a particular reporting period, or authorship needs additional explicit criteria.

The external assistant reads the context packet, uses its authorized browser/connectors, and submits a check report. Movement provides storage and validation, not source fetching, an embedded prompt runner, or an automatic scheduler. Locally stored checks remain readable offline; checking an external URL requires access through the assistant.

Each check report records `completion_version`, `check_version`, `task_revision_checked`, `observed_at`, reporting actor, outcome (`met`, `not_met`, `inconclusive`), a summary, per-criterion results (`met`, `not_met`, `unknown`), and evidence references/excerpts with observation times. Movement also records receipt time. Reports map criterion IDs to results and evidence. Unknown/duplicate IDs are rejected, omitted criteria become `unknown`, and zero criteria cannot yield `met`. Movement computes the aggregate over the full authoritative criterion set: all met yields `met`; any definitively unmet criterion yields `not_met`; otherwise the result is `inconclusive`. Reject a submitted aggregate that disagrees with the computed outcome. Access failure, incomplete retrieval, and ambiguous content yield unknown criteria, never assumed absence. A met result must have evidence for every criterion. Evidence is an attributed agent observation, not independent verification by Movement.

Check reporting uses the normal operation ID and expected-revision contract, with current context/completion/check versions and current expected revision; the evaluated task revision may be older after bookkeeping-only writes. No execution claim is acquired; flow and assignee do not change on report. Lifecycle changes follow the seems-completed rules below and are committed with the check event. For an actively execution-claimed task, the existing claim-token rule still applies. A changed expected revision requires rereading before retry; changed context requires reassessment. Concurrent identical retries cannot append duplicate observations.

Check reports append `completion_checked` history events. The UI derives the latest check from history; changing the prompt or criteria makes earlier checks visibly outdated. Evidence describes the source at observation time, not its guaranteed current state. A `met` check with evidence moves an accepted open task to `seems_completed`, never directly to `done`. `not_met` and `inconclusive` leave ordinary open tasks unchanged; rechecks of `seems_completed` follow the rules below. For delegated tasks the waiting-for flow remains intact; overdue is a deadline flag, not proof that the work is incomplete. Checking sends no reminders. Scheduling future checks and sending follow-ups remain external assistant actions under user authorization.

## Follow-up after a check

A current, accepted check report may include `next_follow_up_at`: omitted leaves the schedule unchanged; null clears it; an RFC 3339 instant sets it and must be later than the server receipt time. Persist the schedule change, observation, and any lifecycle change in one event/transaction. Retry returns the prior receipt without scheduling again. Stale reports cannot reschedule a task. This field does not change `due_at`.

Context exposes `last_check` (event reference, observation/receipt times, versions and result), `follow_up_at`, and `follow_up_pending`. For a non-null follow-up instant, pending is false when a current-context accepted check was observed at or after that instant; it becomes pending again if relevant instructions/criteria change. A new future date is pending immediately and becomes due at that instant. Thus leaving an old date unchanged does not cause endless routine checks. Explicit manual checks remain possible.

Follow-up queries default to `pending_only=true` and omit satisfied follow-ups. They can select future pending dates within a requested interval for planning; the caller still decides when to run. `pending_only=false` supports reviewing prior schedules. Routines use these fields rather than assuming any past follow-up date always needs checking. Scheduling lives in the external routine; Movement does not launch agents or send reminders.

## Late observations and reconciliation

Each successful claim creates a durable `attempt_id` retained in history. Renewals retain it; a replacement claim gets a new attempt. Ordinary reports still require the current claim token and expected task revision.

If a report cannot apply because its claim expired, was replaced, or its evaluated context is stale, the assistant may submit it through `task_observe`. This explicitly appends an observation tied to a known attempt, with original context/criterion/guidance versions, observation time, evidence and output references. Local credentials authenticate access; the obsolete claim token does not authorize state changes. Validate the attempt belongs to the task. New check-only observations may instead reference the evaluated task revision and check version without an execution attempt.

Observation writes require an operation ID and current expected revision; on conflict, reread and retry the same observation. They append `observation_recorded`, increment bookkeeping revision/update the last-event pointer, and preserve business state: status, flow, delegate, claims, instructions, and dates cannot change. They are allowed on terminal tasks and bypass the active-claim token rule only for this append-only operation. They do not count as an accepted check, satisfy follow-up, or make a task seems-completed. Context displays unreconciled observations so a replacement agent can find an already-created Gmail draft.

Reconciliation explicitly references observation event IDs in a fresh, current-version work/check report. It revalidates the evidence against current criteria and uses the normal revision/claim rules; this is what may change status. The new event marks the referenced observations reconciled without rewriting them. An irrelevant observation can be acknowledged via an observation-only event with a reason, also without changing lifecycle or claims. The original report and observation submission use separate operation IDs; retries of either remain idempotent. Movement receipts do not deduplicate external side effects.

## Lifecycle and discovery

States: `candidate`, `ready`, `in_progress`, `blocked`, `seems_completed`, `done`, `dismissed`.

`seems_completed` means “Seems completed after agent investigation.” It is a nonterminal review state, distinct from confirmed `done`, and retains the task’s flow and delegate. A successful evidence-backed check moves an accepted open task into this state. Proposed transition details: reject checks on candidate or terminal tasks until accepted/reopened; no execution claims while seems-completed; explicit confirmation moves to `done`, while “needs more work” moves to `ready`. A later `not_met` check, or edits to the objective, criteria, guidance, or check instructions, move it back to `ready` with history. An `inconclusive` recheck retains the state but displays the latest uncertainty alongside the earlier evidence. These edge-case rules are draft implementation semantics; the intermediate state itself was accepted on 2026-10-22.

- Ingestion creates `candidate`; explicit user creation may create `ready`.
- Triage accepts `candidate -> ready` or rejects `candidate -> dismissed`.
- Claiming `ready -> in_progress` creates a lease; a ready task must have an objective and at least one completion criterion to be claimed.
- Progress reports retain `in_progress`; blocker reports transition to `blocked` and release the claim. Resume moves `blocked -> ready`.
- A `completion_proposed` work report records evidence and releases the claim. Proposed behavior: when it documents verification of all completion criteria, it moves the task to `seems_completed`; an unverified attempt must instead be reported as progress or blocked. A completion action confirms `seems_completed -> done`. User-authorized manual completion remains possible for other accepted open states.
- Explicit release without completion moves `in_progress -> ready`. An expired claim can be replaced atomically while retaining `in_progress`.
- Explicit reopen moves `done` or `dismissed` to `ready`; explicit dismiss is allowed for any nonterminal state and clears a claim. Every transition is logged.

The local user may override or cancel a claim through an explicit operation, with history. Normal updates while claimed require its token; overrides invalidate the old token. Completion is a deliberate operation on the task, separate from reporting an attempt. Clients may call it under user authorization; Movement does not claim to verify external approval.

Every mutation requires an operation ID; updates also require the expected revision. Identical retries return the original receipt, even after later revisions. Different input with the same operation ID returns a conflict. The ingestion identity uniquely identifies a proposed action within a producer's namespace. Repeated ingestion returns the existing task, including dismissed or done tasks, without changing it. Adding evidence is a separate revision-checked operation. Semantic duplicates from different producers are not automatically merged; title similarity is not identity.

## Stable ingestion identities

Automated ingestion supplies `producer` (a stable routine namespace shared across its sessions) and `external_key`. Define key version 1 as the canonical JSON array of `[source_provider, source_object_id, action_key, occurrence_key]`, without insignificant whitespace. Use stable source IDs or a canonical source URI, an explicit durable action code rather than a generated title, and a resolved period/date or `once`. Changes to title, assignee, or deadline do not change identity. Distinct actions on the same source have distinct action keys. Persist and reuse this mapping; do not generate a new key each run.

Example: `["document","https://foo.bar/in","fill-fortnightly","2026-10-12/2026-10-25"]`. The next period uses a different occurrence key even if the document URL is unchanged. Record the explicit reporting period in task context/check instructions as well: identity alone does not prove the content is for the correct period. The producer determines actual period boundaries; Movement does not infer or generate recurrence.

Ingestion identity is immutable after creation in MVP. Repeated keys return the existing task even after dismissal/completion. When source/action/period identity is unresolved, automated producers must resolve it before ingestion, or explicitly request manual candidate capture without a deduplication guarantee. Cross-producer semantic merging remains out of scope.

## Timeframe contract

Queries use `[from, to)` with explicit offsets. A date-only UI range converts local calendar boundaries using the configured IANA timezone, including daylight-saving transitions. Reject invalid or inverted ranges.

Queries use `view=planning` by default: statuses ready, in_progress, blocked, and seems_completed are included. Candidates and terminal tasks require explicit review filters. `flow=actionable` remains the default; use waiting_for or all, and optional delegate filters. Planning displays blocked and review-needed work rather than hiding it.

`view=execution` restricts results to actionable tasks in ready, or in_progress with an expired claim eligible for reconciliation/reclaim, with nonempty objective/criteria and no active claim. It excludes blocked, waiting_for, and seems_completed tasks regardless of explicit status filters. Return eligibility/reconciliation hints; selection is not authorization and claiming must revalidate atomically. An agent resumes its own active work through its task/claim context, not by treating it as unclaimed work.

The query fixes `as_of` once (server time by default; an explicit timestamp is allowed for reproducible planning) and returns it. Reuse it across pagination. `is_overdue` means nonterminal and `due_at < as_of`, independently of the viewed interval; equality is not overdue. Delivery selection includes schedule or deadline in `[from,to)`, actual overdue tasks when `include_overdue=true` (default), and earlier-deadline carryover (`due_at < from`) when `include_carryover=true` (default). These predicates are independent and results are deduplicated. A deadline before a future window but after as_of is carryover, not overdue. An overdue deadline within the window has both due_in_window and overdue reasons. Undated tasks are excluded unless `include_unscheduled=true` is explicit; an undated view can list them separately. Past scheduled dates alone do not imply an overdue deadline.

Return inclusion reasons `scheduled_in_window`, `due_in_window`, `overdue`, `deadline_carryover`, or `unscheduled`, plus `is_overdue`. Sort by priority descending, earliest non-null schedule/deadline, then ID. Status/flow filters still apply before these inclusion predicates.

`time_field=follow_up` substitutes follow_up_at for date selection and sorting, with reasons `follow_up_in_window`, `follow_up_overdue`, `follow_up_carryover`, and `follow_up_unscheduled`; deadline is_overdue remains separate. Missing follow-up dates are excluded by default. `pending_only=true` additionally excludes satisfied follow-ups as defined above. Waiting-for planning includes blocked and seems_completed commitments.

Lists are paginated with contract-defined default/hard limits. Pagination across concurrent writes is best-effort, not a stable multi-task snapshot. Selection itself never claims work or authorizes external actions.

## Proposed interface contract

All adapters call the same use cases. REST paths below are under `/api/v1`; names are provisional. Web UI actions map to these operations.

| Use case | CLI under `movement task` | MCP tool | HTTP |
| --- | --- | --- | --- |
| Capture | `create` | `task_create` | `POST /tasks` |
| Select / list | `list` | `task_list` | `GET /tasks` |
| Read | `get` | `task_get` | `GET /tasks/{id}` |
| Edit / transition | `update` | `task_update` | `PATCH /tasks/{id}` |
| Context packet | `context` | `task_context` | `GET /tasks/{id}/context` |
| Claim / renew / release | `claim`, `renew`, `release` | `task_claim`, `task_renew`, `task_release` | `POST /tasks/{id}/claim`, `/claim/renew`, `/claim/release` |
| Record work | `report` | `task_report` | `POST /tasks/{id}/reports` |
| Record completion check | `check-report` | `task_check_report` | `POST /tasks/{id}/checks` |
| Record late observation / acknowledge | `observe` | `task_observe` | `POST /tasks/{id}/observations` |
| Read history | `history` | `task_history` | `GET /tasks/{id}/history` |

`update` exposes explicit validated fields and actions, never arbitrary JSON paths into history. CLI supports machine-readable JSON and useful exit codes. API/MCP errors share stable codes: `invalid_input`, `not_found`, `revision_conflict`, `operation_conflict`, `invalid_transition`, `claim_conflict`, `store_busy`, `storage_error`, and `unsupported_schema`. HTTP additionally distinguishes unauthenticated access. Responses include the current revision on safe conflict responses so clients can reread and reconcile. Never blindly retry stale writes.

Context packets contain current revision and claim state, flow, delegate, deadline/follow-up time, objective, description, constraints, versioned execution guidance and required capabilities, criteria, completion-check prompt/version/source references, latest check, follow-up pending state, unreconciled observations and bounded recent reports. `task_context` returns check instructions; `task_check_report` only records observations and never fetches a URL or executes the prompt. Missing context is explicit. Full history uses a separate paginated read. Reports include summary, outcome (`progress`, `blocked`, `completion_proposed`), evidence references, and optional next steps. A stale or expired claim report cannot apply lifecycle changes; the caller can preserve its evidence through the explicit observation operation described above.

## Web experience

An inbox for candidates; a task list with timeframe/status filters and selection options for planning/execution, independent overdue/carryover inclusion, undated override, explicit evaluation time and page size; a task detail page with context, sources, and chronological history; create/edit/accept/dismiss/reopen/complete controls. Add a “Waiting for” view with person, expected outcome, delivery deadline, follow-up time, overdue flag, and latest check outcome/time. Task editing includes “How to do it” execution guidance, required inputs/capabilities, expected outputs, delegation, and “Check if done” prompt/criteria fields. Detail shows produced artifacts such as Gmail draft references and distinguishes draft creation from sending. Detail shows per-criterion evidence, access failures, and outdated checks. Show `seems_completed` with the label “Seems completed after agent investigation,” supporting evidence, and confirm-done/needs-more-work actions. Keep these tasks visible in timeframe and waiting-for views pending review; they must not be claimed as execution work. Task cards expose inclusion reasons and reconciliation hints; eligibility is relative to the returned evaluation time, not permission to execute. Pagination must not append a response from superseded filters. Provide standalone historical-report entry, including on terminal tasks, with review of the original context and evidence before append-only observation submission. Preserve entered reports across revision conflicts and uncertain responses. Show active claims and provide acquire/renew/release controls, with explicit replacement of another active attempt. Provide structured work/check report submission using the same criterion evidence, revision, context-version and claim-token contracts as other interfaces. These controls record work performed outside Movement. A “Check if done” section exposes instructions for the external agent; it must not imply the local UI runs an agent. Completing a waiting-for task removes it from that active view while preserving its evidence/history. Conflicts show the latest task and preserve unsaved input. UI assets are bundled, usable offline, keyboard accessible, and readable on a narrow viewport. No CDN or remote fonts.

## UI tooling and delivery

Accepted on 2026-10-22: React + TypeScript with Vite in a dedicated `web/` development setup. Vite supplies local development serving and hot updates; Go remains the authoritative API and task service. Use relative `/api/v1` calls, proxied to the local Go server during development and served from the same origin in the packaged application.

Node and frontend tooling are development/build dependencies only. A release builds and type-checks the frontend first, generates `internal/web/dist/`, and embeds those assets into the Go binary. A failed frontend build must fail the release rather than reuse stale assets. Runtime needs only the binary and its data directory; no Vite server, Node installation, CDN, or separate asset directory.

Development proxying must preserve authentication/session and CSRF behavior with an explicit local development origin allowance. Production stays restricted to its own local origin. UI routes, static assets, API errors, and browser refresh must behave correctly in the embedded server. Validate key user flows against both development serving and the packaged binary. Keep React state management simple initially; dependency versions, minimum Node version and supported browser targets are implementation choices to finalize before scaffolding.

## Acceptance scenarios

| ID | Proof | Requirements |
| --- | --- | --- |
| A1 | Create through MCP; read the same task through CLI, HTTP, and UI after restart | R1,R2,R7 |
| A2 | Retry identical ingestion and mutations after a lost response; observe one task and one corresponding event; changed payload with same operation ID conflicts | R2,R6 |
| A3 | Re-ingest a dismissed/completed task and a manually reprioritized task; preserve their state and edits | R3 |
| A4 | Verify overdue is included and undated excluded with default arguments; query boundaries and explicit override flags, status filters, both-date matches, timezone and DST cases; verify IDs, ordering, and reasons | R4 |
| A5 | Read context; race two claim requests; only one wins; renew/release/expire a claim and reject obsolete reports | R5 |
| A6 | Append progress, blocker, and completion evidence; distinguish proposed completion from done; persist actors and history across restart | R6 |
| A7 | Race writes from separate CLI/MCP/HTTP processes; stale revisions fail without losing accepted state or history | R1,R7 |
| A8 | Interrupt each transaction phase, including event installation before task replacement and cleanup; recover a matching task/event pair before serving reads, preserve receipts, and acknowledge only durable success | R1,R6 |
| A9 | Copy the data directory; load with a supported binary offline; UI makes no external asset requests | R1,R8 |
| A10 | Reject traversal, symlink escape, unauthenticated HTTP, hostile origins, script markup, malformed records and newer schemas without data loss | R1,R7 |
| A11 | Use a packaged binary with actual supported assistant clients; verify tool discovery, structured errors, and clean stdio | R5,R7,R8 |
| A12 | Create Nathan’s delegated fortnightly task; show it in waiting-for by person/deadline and follow-up date, exclude from default executable work, reject execution claims, and retain delegation after completion | R4,R7,R9 |
| A13 | Retrieve the saved check prompt; report all A-D present, D absent, and inaccessible page as met/not_met/inconclusive respectively with criterion evidence; preserve flow/delegate; met enters seems_completed, not_met/inconclusive retain ordinary open status | R5,R6,R10 |
| A14 | Change check criteria during a check; reject the stale result; label earlier-version results outdated; retry a committed report without duplicating its event | R1,R6,R10 |
| A15 | Run the delegation/check workflow through CLI, MCP, HTTP and UI reads; confirm seems_completed as done, retain history, and perform no source edits or unsolicited messages | R7,R9,R10 |
| A16 | Retrieve the Kevin task via each interface with guidance, constraints, inputs and deadline; using fake connectors, create a draft, report its reference and evidence, and never invoke send | R5,R6,R7,R11 |
| A17 | Missing Jira access, unresolved recipient/project, changed guidance, and an uncertain Gmail response produce a blocker/conflict or reconciliation; retries do not blindly create another draft | R3,R6,R11 |
| A18 | Parse JSON state and YAML-frontmatter Markdown events; verify matching task IDs, revision/sequence, last-event reference, receipts and structured reports after restart/recovery | R1,R6 |
| A19 | Run the React/TypeScript UI through Vite against the local Go API; verify proxy/session authentication, CSRF rejection, task edits, and hot updates without weakening production origin rules | R7 |
| A20 | From a clean checkout, type-check/build the frontend before Go; fail on frontend errors, then run the binary without Node/Vite or separate UI files and verify offline assets, navigation/refresh, and proper API/static error responses | R7,R8 |
| A21 | Define one criterion set; reject unknown/duplicate IDs and conflicting aggregates; partial/empty/stale-version check or work results cannot establish seems_completed | R5,R6,R10,R11 |
| A22 | Create a draft, expire/replace the claim, preserve its reference as a late observation without changing status/claim, then reconcile under current criteria; retries do not duplicate observations or drafts | R3,R5,R6,R11 |
| A23 | Planning includes overdue blocked tasks and completion review; execution excludes them and active claims; future-window carryover is not labelled overdue before as_of | R4,R5 |
| A24 | Repeat discovery with stable source/action/period keys, including after dismissal; get one task. Change the period and get a distinct task | R2,R3,R9 |
| A25 | Record a check and next follow-up atomically; omitted/null/future values, retries, stale reports, and satisfied old follow-ups behave as specified | R1,R4,R6,R10 |

## Readiness and remaining verification gates

The independent-process choice is accepted (decision 004). Context versions, follow-up timing, claim cleanup, authoritative retry receipts, bounded context, assessment ordering, and HTTP session behavior are specified below and in the linked contracts. They are no longer open architecture questions. Existing implementation and reported verification are tracked in [implementation status](../../docs/implementation-status.md); this specification does not declare the MVP complete.

The [final specification challenge](final-review.md) distinguishes unresolved contract details from missing acceptance evidence. The [implementation audit](../../docs/reviews/implementation-audit.md) records operation-by-operation web parity, nested UTC storage tests and the tested macOS/CPU/browser scope. Claude Desktop smoke and guided execution through human confirmation have passed with independent store readback. On 2026-10-23 the user accepted Codex Desktop verification as a non-blocking follow-up: expected to work, not verified. Initial client acceptance covers Claude Desktop 1.49585.0, Claude Code 2.1.308 and Codex CLI 0.163.0; no client-specific routine scheduler is supplied or certified by Movement. Backup/restore, unsupported-version handling and isolated packaging now have recorded coverage; retain those tests rather than treating them as unimplemented design work. Actual Desktop compatibility cannot be inferred from CLI client tests. Retain the independent-process crash and contention gates. The [acceptance audit](../../docs/acceptance.md) tracks the remaining proof. Recommendations in the review are not additional recorded user approvals.


### HTTP implementation contract

The local HTTP implementation uses the browser session and bearer contract in [HTTP documentation](../../docs/http.md): eight-hour in-memory HttpOnly/SameSite=Strict sessions, exact Host/Origin validation, CSRF tokens for cookie-authenticated mutations, and explicit Vite origin opt-in. Session restart invalidation, 1 MiB mutation bodies and structured HTTP error mappings are implementation choices within the accepted local-only scope. Browser/Vite acceptance remains required.


### Bounded context and assessment ordering

`task_context` returns at most 10 unresolved observations by default, with an explicit `unreconciled_count`, `observations_offset` and optional `next_observations_offset`. Clients may request 1–50 observations using `limit` and `offset`. Any continuation requires `expected_revision` equal to the first packet's task revision; intervening writes return `revision_conflict` rather than silently skipping observations. Restart pagination after rereading. The packet contains at most 10 recent history events, with `recent_history_truncated` indicating that full history must be read separately. Its serialized JSON is capped at 4 MiB; byte limits can produce a shorter observation page with a continuation. Durable events are never shortened or discarded. Agents must inspect all unresolved pages before deciding whether an external side effect needs repeating. MCP may represent the same bounded packet as both structured and text content.

Accepted checks and completion-proposed work reports are ordered by `observed_at` within a semantic context version. A new assessment must be strictly newer than every accepted assessment of that context. An older or equal-time assessment returns `assessment_conflict` and changes neither state nor follow-up. Committed identical retries still return their original receipt. Preserve delayed evidence through `task_observe`; after reviewing it against the current task, submit a newly observed assessment that explicitly reconciles the observation. No automatic evidence-expiry policy is introduced.

Late observations validate nested criterion evidence and output timestamps as well as top-level evidence. Their evaluated revision must exist with the reported context version, and a referenced execution attempt must have existed by that revision. Historical criteria are not reinterpreted as current criteria; applying retained evidence remains a fresh-report operation.


### Machine-readable contract and fixture artifacts

Version-1 task JSON and history-frontmatter schemas are in `api/task.schema.json` and `api/history-event.schema.json`; HTTP operations are described in `api/openapi.yaml`. Generate them with `go run ./cmd/contracts`. Runtime storage validation and MCP input descriptions share the schema builder; cross-field/temporal/transactional rules remain authoritative in the domain and service. History filenames use a 20-digit zero-padded sequence and the validated event type. Generated synthetic bundles include valid receipt digests and matching records; `go run ./cmd/examples` regenerates them through the service without external access. See the [acceptance audit](../../docs/acceptance.md) for remaining proof.


### Identity and durable receipt validation

Removing a criterion records its ID and description in the mutation event's `changes.retired_criteria`. Restoring a retired ID requires the same description; use a new ID for a different definition. Active criteria may be edited under a new completion version. This is a structural identity check, not a claim that Movement can infer whether prose has the same meaning.

Accepted work/check reports and late observations must link their evaluated task revision to the actual context version at that revision. An execution attempt must have been claimed by the evaluated revision. Supplying current version counters alongside a pre-claim or differently versioned revision is invalid input.

Every state-changing history event must have its matching authoritative receipt. Snapshot reads validate receipt/task/event links; committed retries verify their target before returning. Claim events retain a token hash in `changes.claim_token_hash` so a historical claim receipt can be checked even after its lease ends. Plaintext claim tokens remain only in private receipt files. Receipt corruption or loss fails visibly rather than being interpreted as an unseen operation.

Recovery uses strict bounded JSON parsing for both the journal envelope and payload. Write, rename, link, sync and removal failures return errors; prepared journals remain recoverable until destination durability and cleanup finish. If an immutable destination already exists with matching bytes, recovery still syncs its parent directory before acknowledging completion. Tests inject I/O errors; they do not establish real hardware power-loss guarantees.

### Backup and restore operations

Use the [stopped-store procedure](../../docs/backup-and-recovery.md): stop all CLI/MCP/HTTP users, recover/validate with `verify`, copy the entire directory, and verify the copy. `verify` may finish pending transactions; it does not hold a lock for a later filesystem copy. Restore to a separate directory and rotate its HTTP token with `auth rotate` before starting servers. Rotation preserves authoritative receipts and execution claim tokens. Existing HTTP processes retain their in-memory token and sessions until stopped; rotation is not live revocation. Reconcile external artifacts before resuming routines. Schema v1 is the initial supported format; future migrations require explicit source/target contracts and backup.
