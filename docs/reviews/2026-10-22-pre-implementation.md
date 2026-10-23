# Final pre-implementation contract review

Status: findings, not accepted changes | 2026-10-22

Independent processes are explicitly accepted in the spec and constitution. The architecture now shows the single prepared journal used by the prototype. No new product scope is recommended. Four contract details should be settled before implementing the affected use cases.

## 1. Separate content freshness from storage revision

The spec's execution report allows task_revision_used to predate a lease renewal, but does not define how to prove that objective/constraints/input context remained unchanged. Guidance/completion versions alone do not cover those fields. Conversely, check reports reject any intervening revision, including harmless late observations.

Example: the recipient or constraints change after an agent reads context; the agent rereads only the current expected revision and submits old evidence with unchanged guidance/completion versions. The implementation has no precise semantic freshness test beyond the narrative requirement to reassess.

Recommendation: keep revision for write conflicts and add one context_version covering objective, constraints, inputs, completion criteria, execution/check instructions, and delegation/occurrence scope. Every context-changing mutation increments it. Reports identify the evaluated version; bookkeeping events and lease renewal do not change it. Stale evidence uses the observation path. Specify the exact field list and tests before model implementation.

## 2. Give follow-up scheduling unambiguous semantics

The spec says a future date becomes pending when due, but also says pending-only planning can return future pending dates. It marks a schedule satisfied using receipt time, so an observation made before a scheduled check but submitted after it can incorrectly suppress the intended investigation.

Recommendation: distinguish pending (not satisfied) from due (pending and follow_up_at <= as_of). Future pending items remain selectable for planning. Require the check's observation time to meet the scheduled instant and current context version; reject future observation timestamps. Test a check observed at 14:00 and submitted at 15:01 for a 15:00 follow-up: it must remain pending.

## 3. Define claim cleanup for every state transition

Work reports release claims, but successful check reports can move an actively claimed task to seems_completed without explicitly clearing the claim. Manual completion has the same gap. That can leave a non-executable task holding a live execution lease, or allow renewals after apparent completion.

Recommendation: an active execution claim implies actionable/in_progress. Any transition out of that combination atomically closes the attempt and clears its token. Renewal checks both current state and attempt. Preserve late evidence through observations. Add transition-table tests for successful checks, manual completion, blocking, dismissing and explicit overrides.

## 4. Finalize idempotency identity before implementing the repository API

The prototype scopes operation IDs per task and includes expected revision in the digest. The product API generates task IDs on creation, while manual creation can omit ingestion identity. An identical create retry after a lost response therefore needs receipt lookup before the caller knows the task ID. Also, conflict handling says to retry the same observation with a new expected revision, but payload changes under the same operation ID are otherwise conflicts.

Recommendation: define store-wide operation IDs and a rebuildable receipt lookup, with create lookup before ID generation. Hash the operation type, target when known, and canonical semantic input; exclude expected revision and transport/claim credentials. Check a committed receipt before current preconditions. Uncommitted conflicts can retry with fresh preconditions; committed identical operations return their original receipt. Document JSON canonicalization, omitted/default handling, and lookup precedence versus ingestion identity. Update the prototype's narrower convention when implementing the production repository.

## Readiness

The storage topology is settled. These are implementation-contract fixes, not another product discovery round. Address them in schemas, transition rules and acceptance scenarios before implementing those behaviors. macOS deployment target, payload limits and browser-session bootstrap are implementation choices to fix at their respective slice boundaries.
