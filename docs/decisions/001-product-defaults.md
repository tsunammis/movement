# 001: Completion review, timeframe defaults, access, and initial platform

Status: accepted | Owner: Stan | Date: 2026-10-22

## Decision

Stan resolved four questions from the specification review:

1. Add a state meaning “seems completed after agent investigation.” The internal name is `seems_completed`; it is separate from confirmed `done`.
2. Include overdue tasks in timeframe queries by default. Exclude undated tasks by default.
3. Use Movement interfaces as the normal access path. Files remain freely readable, while supported writes go through Movement.
4. Support macOS only initially. Other operating systems are deferred.

## Consequences

Task state, history, UI, and every adapter must distinguish apparent completion from confirmed completion. Evidence and status transitions are saved together. Waiting-for tasks retain their delegate and flow while awaiting confirmation. Overdue work remains visible in time-based planning; undated work is available separately.

The specification proposes detailed rules for rechecks, invalidating old evidence, execution reports, and review-state filtering. Those mechanics remain draft rather than being attributed to the user's explicit decision. Minimum macOS version and CPU targets also remain open.

## Acceptance

Accepted by Stan in the 2026-10-22 conversation responding to the four numbered specification questions. This accepts these product choices, not the entire specification or implementation plan. No implementation is authorized by this record.

## Validation

The [specification](../../intent/001-local-task-management/spec.md) covers default query behavior and evidence-backed status changes; the [plan](../../intent/001-local-task-management/plan.md) must validate macOS persistence and packaged-binary behavior. Runtime validation is pending implementation.
