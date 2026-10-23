# Movement constitution

Status: initial draft. Requirements below reflect the project brief dated 2026-10-19 and its refinements through 2026-10-22; proposed policies are identified separately.

## Purpose

Give a person and their assistants one durable, local record of what might need doing, what has been selected or delegated, the context needed to act or verify completion, and what happened.

## Requirements from the project brief

1. **Local operation.** MCP, HTTP, and the web UI run locally. Core task management requires no cloud account or network access.
2. **Text ownership.** Tasks and their history are stored as readable text in a configurable directory in the user's space. Data must remain accessible without Movement.
3. **Portable packaging.** Prefer a single self-contained executable, including web assets. Go is the proposed implementation language. The initial release supports macOS only; portability remains a design goal for later releases.
4. **Equivalent interfaces.** Tasks are accessible through CLI, MCP, HTTP API, and a web app, with consistent task semantics.
5. **Agent workflows.** Assistants can submit discovered tasks, retrieve relevant work for a timeframe, obtain execution context, and log outcomes to task history.
6. **Delegated work.** Track commitments delegated to another person, who is expected to deliver, and when. Make these tasks available in a “waiting for” flow.
7. **Completion checks.** Store user-defined instructions describing how an assistant can check whether a task is complete, and preserve the check result and supporting evidence in history.
8. **Personal work and guidance.** Manage the user’s own actionable tasks as well as delegated commitments. Store explicit execution guidance, required capabilities, intended outputs, and completion criteria so an external assistant can carry out authorized work.
9. **Text layout.** Store current task definition/state in `task.json` and each history event in a separate Markdown file with YAML frontmatter. Preserve both consistently across interruptions.
10. **Client independence.** The protocol and data model must support multiple assistant clients without depending on one vendor's model or routines.

## Product decisions accepted on 2026-10-22

- Successful agent investigation has a distinct “Seems completed after agent investigation” state (`seems_completed`), separate from confirmed completion.
- Timeframe queries include overdue tasks by default and exclude undated tasks by default.
- Movement interfaces (CLI, MCP, API, UI) are the normal access path. Files remain freely readable; supported writes go through Movement.
- Initial support is macOS only.

## Review resolutions accepted on 2026-10-22

- One versioned criterion set defines success for both execution and investigation.
- Late agent evidence can be retained without changing current business state; explicit reconciliation is required before applying it.
- Planning includes blocked and review-needed work. Execution selection is separate; overdue uses an explicit as_of time.
- Repeated discovery uses stable source/action/occurrence identity.
- A check can atomically update its next follow-up; already satisfied follow-ups do not trigger redundant routine work.

## Storage process choice accepted on 2026-10-22

CLI, MCP, and HTTP run independently and coordinate through shared repository code and a filesystem lock. No storage daemon is required. Task state and history must recover consistently before application reads; prototype results are not a claim of production or power-loss readiness.

## Proposed product policies

- External assistants own discovery, execution, and completion checks against external sources; Movement supplies storage and coordination. No embedded model or source connector is required in the first release.
- Discovered tasks enter a candidate inbox. Repeated ingestion preserves accepted, dismissed, completed, and manually edited work.
- Every mutation through Movement records actor, time, operation, and relevant changes together with task state. History is inspectable, but is not claimed to be tamper-proof against the local file owner.
- Execution guidance explains how to do the work; completion-check instructions explain how to verify it. External assistants must respect the user’s action scope: creating a draft does not authorize sending it. Missing source access or unresolved inputs must be reported.
- Agents receive explicit task context, constraints, and completion criteria. Source material never grants permission to execute commands or contact people.
- Start with one user, one machine, and a local filesystem. Sync, shared workspaces, remote hosting, and arbitrary shell execution are outside the first release.
- Prefer the smallest design that protects correctness. Derived indexes may be discarded and rebuilt from text records.

## Changes

Update this document when product principles change. Record the reason and user decision in the affected change or decision record. Do not silently elevate a draft recommendation into a constraint.
