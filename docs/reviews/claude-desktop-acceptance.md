# Claude Desktop smoke acceptance

Executed 2026-10-23 with Claude Desktop 1.49585.0, the packaged Movement binary, and an isolated temporary data directory. The user installed the prepared MCP configuration and ran the synthetic prompt because native desktop control was unavailable.

## Evidence

- Claude's dedicated MCP log records initialization, tool-list discovery and five tool-call request/response pairs between 08:51:24 and 08:51:36 UTC.
- The user supplied Claude's result summary: create, identical create replay, get, stale revision update rejected as `revision_conflict`, then history. A preceding `tool_search` loaded schemas without external-source access.
- Independent `movement verify` returned exactly 1 task, 1 event and 1 receipt.
- Independent CLI get/history and direct receipt inspection confirm task `379c0794-afd9-4cfd-8c3e-c69089d2cfbe`, revision 1, title `Synthetic client acceptance`, status ready, and actor `claude-desktop-client-test`.
- The sole event is `065cd526-750a-4483-b073-fa7a670569be`, sequence 1, `task_created`, operation `actual-client-create`. The receipt and event share digest `135d4a4ebbc152ce5132a2b46a2fcccc3a17b7582b8e2c09ec282e6b8eb268db`.
- No stale-update receipt or mutation event exists. The title remains unchanged.

The smoke workflow passed with user-assisted client interaction and independent durable readback. The log omits argument/result bodies, so exact duplicate response equality and the returned error message are supported by the user-provided client summary, not independently extracted protocol bodies. Do not describe that summary as a captured raw transcript.

## Cleanup and limits

Removed only the temporary `movement-desktop-acceptance` configuration entry after testing, preserving other settings. No restart was performed during cleanup; a running Claude process may retain its already-loaded server until the user next quits it. Synthetic records and the original configuration backup remain in the isolated test directory.

This establishes the actual Desktop create/retry/read/conflict/history smoke workflow. Context/claim/report workflows and routine scheduling were not exercised in this Desktop run. Codex Desktop remains untested. Movement does not run a scheduler or perform external source actions.

## Guided execution through completion proposal

The user ran the Project Cedar synthetic workflow on task `5bbb0bf8-87a8-4b3a-97cb-344e3dae4ea5`. Independent CLI get/history confirms revision 4, status `seems_completed`, no active claim, context version 1, and four events: creation, claim, progress report and completion-proposed report. Both reports reference the original attempt and matching context/completion/guidance versions. The proposal retains the full synthetic unsent draft, accurate supplied facts, both criteria marked met with excerpts, and a logical `draft_text` output reference. No plaintext claim token appears in report history.

Progress was observed at 12:48:54.522083Z and recorded at 12:49:07.075557Z; the proposal was observed at 12:49:18.111164Z and recorded at 12:49:26.418303Z on 2026-10-23. The client reports obtaining timestamps from read-only task-list `as_of` values. These are approximate observation times, not proof of exact source-read timing. The user's report says only Movement tools were used; local records alone cannot prove absence of other client activity.

Human confirmation was subsequently completed and independently verified below. The test also exposed missing report outcome enumeration in tool discovery; the shared schema now enumerates work/check outcomes and MCP descriptions distinguish their permitted sets.

## Explicit human confirmation verified

The user supplied the client result after explicitly approving the synthetic draft. Independent CLI readback confirms task status `done`, revision 5, no active claim, and five history events. Final event `1cce3836-dbaa-436f-8cb0-3908614d6c92` records action `complete`, operation `cedar-confirm-20261023-01`, actor `claude-desktop-workflow`, and the transition `seems_completed` to `done` at 2026-10-23T14:13:51.1658Z. Context/completion/guidance versions remain 1. The original proposal event retains the complete synthetic unsent draft and both met criteria with evidence. Store verification reports two tasks, six events and six receipts, including the earlier smoke task.

The user-assisted Claude Desktop context/claim/progress/proposal/human-confirmation workflow has passed with independent persisted-state verification. No real Jira/Gmail integration or routine scheduling was exercised. Absence of external sending is reported by the client and bounded by the synthetic task; Movement's records do not independently audit all client activity. Codex Desktop remains a separate acceptance gate. The user reinstalled the isolated server for this workflow; the prior smoke-test cleanup does not describe its current configuration.
