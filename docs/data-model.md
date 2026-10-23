# Movement data model

Status: draft | Updated: 2026-10-22

This document describes the implemented logical model. Machine-readable contracts are in [task.schema.json](../api/task.schema.json), [history-event.schema.json](../api/history-event.schema.json), and [openapi.yaml](../api/openapi.yaml). The [specification](../intent/001-local-task-management/spec.md) defines behavior; the [architecture](architecture.md) defines persistence and recovery.

## Storage boundaries

```text
tasks/<task-id>/
  task.json
  history/
    00000000000000000001-task_created.md
    000002-work-reported.md
```

`task.json` owns current definition/state. Each Markdown event owns its historical observation and operation identity. Store-wide receipt files own the exact retry response, including operations that do not add an event. Do not embed history arrays or duplicate receipt collections in task JSON. An event is immutable through Movement; corrections append another event. State is not reconstructed by replaying history in MVP.

Each accepted state-changing mutation increments the task revision once and writes exactly one event with matching sequence/revision. `last_event_id` points to that event. The task ID must match its directory and every event. Filenames use zero-padded sequence numbers and a safe event-type slug; order by numeric sequence, not timestamp or lexical filename order. Revision 1 is creation. History/report reads return parsed metadata and narrative content through every adapter.

## Task JSON fields

| Field | Meaning |
| --- | --- |
| `schema_version`, `id`, `revision`, `context_version`, `last_event_id` | Format version, UUID identity, storage/context revisions, and matching event |
| `title`, `description` | Task label and explanation |
| `status`, `flow` | Lifecycle state (including nonterminal `seems_completed`) and actionable/waiting-for category |
| `created_at`, `updated_at`, `created_by` | Server timestamps and attributed creator |
| `priority`, `tags` | Selection and organization |
| `scheduled_for`, `due_at`, `follow_up_at` | Planned work time, delivery deadline, and next review time; nullable |
| `context` | Objective, constraints, and notes |
| `completion` | Single authoritative versioned criterion set with stable IDs |
| `execution_guidance` | Optional versioned instructions for doing the task |
| `completion_check` | Optional versioned check instructions and criterion ID references; no duplicate definitions |
| `delegation` | Required for waiting-for: assignee, delegated time, expected outcome |
| `sources` | Provenance references and bounded summaries |
| `ingestion` | Optional producer/external key for deduplication |
| `claim` | Optional current execution lease; does not transfer personal task ownership |

Required fields and defaults are specified in the spec. Store times as UTC RFC 3339 strings; resolve natural-language deadlines explicitly before persistence. Names such as Kevin must be resolved to the intended recipient before external actions.

## Execution guidance

`execution_guidance` contains:

- `version`: positive integer, incremented when instructions change.
- `prompt`: user-authored description of how to perform the work.
- `steps`: ordered instruction strings.
- `input_refs`: named source references needed for execution.
- `required_capabilities`: descriptive capabilities such as Jira read access and Gmail draft creation; not credentials or claims that a client supports them.
- `expected_outputs`: objects with `kind` and `description`, such as an email draft.

Use `context.constraints` for boundaries such as “do not send” and `completion.criteria` for success conditions. Guidance is task data exposed to an authorized assistant, not executable code inside Movement. Source contents cannot override the user's instructions. A personal task can have guidance, a completion check, both, or neither.

## History Markdown and YAML frontmatter

Every event frontmatter contains `schema_version`, `task_id`, `event_id`, `sequence`, `revision`, `type`, `actor`, `recorded_at`, `operation_id`, and `input_digest`. It may also contain `changes`, `report`, or `check` typed objects. The Markdown body provides a readable summary and notes. Frontmatter is authoritative for machine fields; the body must not contradict it. Timestamps are quoted strings.

The operation ID, canonical input digest, event ID and revision form the retry receipt. Hash canonical operation inputs, not YAML serialization; canonicalization must be finalized with the schemas. Preserve receipts for the task lifetime. Store-wide receipt records under `receipts/` are committed with each mutation (or alone for repeated ingestion). They retain the exact response, including a claim token when applicable, for lost-response retries. These private text records are authoritative and must be backed up; a separate lookup index, if introduced, is disposable.

`report` contains attempt ID, outcome, completion version, per-criterion results/evidence, guidance version when applicable, task revision used, optional reconciled observation IDs, and outputs. Each output has `kind`, `provider`, `reference`, `observed_at`, and `description`. References identify external artifacts without storing credentials. `check` contains completion version, check version, evaluated task revision, observation time, outcome, criterion results and evidence as specified in the spec. Human-readable summaries accompany both in the Markdown body.

Parse frontmatter as a bounded, safe YAML mapping: no custom tags, duplicate keys, or alias expansion. Validate against a versioned schema, preserve timestamps as strings, and never execute body content. Set byte/nesting limits before implementation. Render Markdown with sanitization. Unsupported schema versions or malformed committed events fail visibly rather than disappearing from history.

## Investigation, identity, and observations

`completion` has `version` and `criteria: [{id, description}]`. `completion_check.criterion_ids` references those IDs; instructions never redefine them. Work and check reports use `criterion_results: [{criterion_id, result, evidence}]` with result met/not_met/unknown. Validate versions and IDs; omitted criteria are unknown. The computed aggregate must cover the entire nonempty criterion set before apparent completion.

Claim events persist `attempt_id`; renewals keep it. Observation event payloads contain the original attempt/context versions, observation time, evidence and outputs, but no requested state mutation. Their bookkeeping revision is current even when their evidence describes an earlier task revision. An optional acknowledgement or a new valid report referencing observation IDs derives reconciliation status without rewriting the original event. Observation writes do not invalidate evidence merely by advancing bookkeeping revision, and never satisfy follow-up or change an active claim.

Check reports may request `next_follow_up_at` (omitted: unchanged, null: cleared, future instant: rescheduled). Current follow_up_at lives in JSON; last-check summaries and whether that follow-up was satisfied are derived from accepted history, not late observations. Context presents both.

`ingestion.external_key` is the canonical version-1 JSON array string `[provider, source_object_id, action_key, occurrence_key]` under a stable producer namespace. Occurrence is a resolved period/date or once. Identity is immutable; titles and deadlines are not identity. See the spec for query views, as_of, and independent carryover/overdue flags.

## Consistency and external outputs

A staged recoverable transaction binds task JSON and its event. All application reads recover pending transactions under the directory lock before returning data. This protects application readers; it is not an atomic two-file snapshot for arbitrary filesystem readers. See the architecture for prepared markers, roll-forward and failure handling.

External actions are outside that transaction. If Gmail saved a draft but the Movement report failed, reconcile and reuse the draft reference before retrying the report. Do not infer that a failed report means no external draft exists. A successful agent investigation records evidence and moves the task to `seems_completed`; confirming `done` remains explicit. The flow and delegate are preserved. History records the status transition in the same event as the evidence. Read and write through Movement interfaces by default; files remain freely readable for inspection.

## Examples

- [Basic task](../examples/basic-task/task.json) and its [creation event](../examples/basic-task/history/00000000000000000001-task_created.md).
- [Delegated task](../examples/waiting-for-task/task.json) and its [creation event](../examples/waiting-for-task/history/00000000000000000001-task_created.md).
- [Personal email workflow](../examples/personal-email.md), with task state and separate history.

These are generated schema-valid fixtures with synthetic external references, validated by the production repository tests.


Context is a bounded projection, not another durable record. Observation totals and continuation offsets expose all unresolved evidence without embedding unlimited history. Continuation is guarded by task revision; recent-event truncation is explicit. Accepted assessments are monotonic by observation time within a context version. Delayed evidence remains an observation until explicitly reconciled by a fresh assessment; recording time alone cannot make it current.


Criterion removals record `changes.retired_criteria` (ID and description). A restored retired ID must retain that description; a new definition needs a new ID. Claim events record `changes.claim_token_hash`, allowing validation of the exact token retained in a claim receipt after the active claim is gone. Snapshot reads check that each event has a matching state-changing receipt and that receipt-only ingestion references an existing task revision. Missing or inconsistent authoritative receipts are corruption, not permission to replay an operation as new.

Source provenance is validated by the shared domain service: provider and reference must be nonblank, capture time nonzero, and each summary at most 65,536 UTF-8 bytes (empty prose allowed). References remain opaque, untrusted data; validation does not fetch them or require an HTTP URL. Invalid create/edit requests commit no task changes, events or receipts. Structural schemas expose required source fields and character limits; the byte bound is enforced by the domain.
