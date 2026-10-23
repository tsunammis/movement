# 003: Resolve the final specification review gaps

Status: accepted direction | Owner: Stan | Date: 2026-10-22

## Decisions

1. Maintain one authoritative, versioned completion criterion set with stable IDs. Execution and investigation results assess that same set; partial results cannot establish apparent completion.
2. Retain late evidence as attempt-linked observations without changing business state or current claims. A fresh valid report explicitly reconciles it before applying lifecycle changes.
3. Separate planning from executable-work selection. Planning includes blocked and review-needed tasks. Define overdue against a fixed as_of time and distinguish earlier-deadline carryover from actual overdue status.
4. Require stable source/action/occurrence keys for automated ingestion. Reporting periods identify separate occurrences without adding an internal recurrence scheduler.
5. Allow a current check to update its next follow-up atomically. Expose last-check and pending-follow-up information so routines avoid repeated work.
6. Originally compare independent locked writers with a single local persistence service in the storage spike. **Superseded by [decision 004](004-independent-process-storage.md): independent processes are selected; no daemon comparison remains required.** Text storage and single-binary packaging are retained.

## Acceptance and limits

Stan accepted these resolutions with “Let's resolve the gaps as suggested” on 2026-10-22. Field names, validation rules, query flags and protocol details in the spec are the concrete design for review. This does not authorize product implementation or accept the full implementation plan.

## Proof

Acceptance scenarios A21-A25 cover the five behavior gaps. At the time of this decision, no runtime checks had been run. Subsequent prototype evidence and the selected process model are recorded in decision 004; current application evidence is in [implementation status](../implementation-status.md).
