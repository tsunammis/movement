# Final specification challenge

Date: 2026-10-22. Scope: review before further implementation. Recommendations below are proposed, not recorded user decisions.

## Assessment

The product and independent-process architecture are coherent. Keep one shared domain service and repository, one stable store lock, and recovery before application reads and writes. No daemon comparison is needed. The existing repository contains foundation code and CLI work; this review does not claim that implementation has not started or that the MVP is complete.

The remaining risks are ambiguous operational contracts, rather than missing features. Close them at the relevant slice boundary; avoid expanding the MVP.

## Recommendations and proof

| Gap | Recommended resolution | Verification gate |
| --- | --- | --- |
| Retry responses cannot always live in task history | Treat store-wide receipts as authoritative and commit task/event/receipt together. Repeated ingestion may commit only a receipt. Back up receipts; never treat them as a disposable index. The spec, architecture and data model now agree on this existing design. | A2/A8/A18: lose the response to creation, claim and deduplicated ingestion; restart and recover the exact original receipt without another event. |
| Context can grow without bound | Set explicit byte and item limits for requests, records and responses. Paginate history and unreconciled observations. Return counts and continuation information; never silently omit evidence needed to reconcile an external action. Reject oversized writes before preparation. Recommend 1 MiB request/record limits, list/history pages of 50 by default and 200 maximum, and 10 recent reports in context, subject to checking representative fixtures. | A5/A22: a task with hundreds of observations remains readable; every observation is reachable and missing pages are explicit. Oversized input leaves state and history unchanged. |
| A valid context version does not mean evidence is recent | Define behavior for checks arriving out of observation-time order. Recommend retaining an older check as an observation instead of allowing it to replace a newer accepted assessment. Equal timestamps require an explicit reconciliation rather than guessing an order. Context should distinguish latest recorded event from latest accepted assessment and show observation time. No automatic evidence-expiry policy in MVP. | A13/A14: a delayed positive check cannot erase a newer negative finding; evidence remains available for explicit reconciliation. |
| Process-crash recovery is not a complete durability promise | State the supported guarantee as tested process-interruption recovery on a local filesystem. Inject write, rename and sync failures, including disk-full-style errors. After durable preparation, preserve the journal and fail visibly until recovery can finish. Never advertise hardware power-loss guarantees from SIGKILL tests alone. | A8/A10: failures before preparation leave the old state; failures afterward remain recoverable, never acknowledge success prematurely, and never serve an inconsistent pair. |
| Copying a directory also copies credentials and pending work | Document a stopped-writer backup: stop CLI/MCP/HTTP users of the directory, recover pending work, then copy all authoritative files. Restore into a separate directory, validate before use, and regenerate HTTP credentials/invalidate sessions before exposing the restored instance. Do not delete a live lock file. A restore is not a supported concurrent replica or sync mechanism. | A9: restore a store with receipts and history, recover an interrupted transaction, verify retry behavior, and ensure the old HTTP credential no longer authenticates the restored server. |
| Browser and release contracts are still placeholders | Before HTTP implementation, specify credential bootstrap, session expiry, CSRF and exact dev-origin behavior. Before release, name the tested macOS version/CPU, browser targets, MCP protocol and actual assistant clients. Prefer an Apple Silicon first release if that is the only hardware available for execution testing; do not equate Intel cross-compilation with validation. | A10/A11/A19/A20: unauthenticated/hostile browser requests fail; the same auth behavior works through Vite; packaged binary is exercised on every advertised platform/client combination. |

## Scope discipline

Do not add a scheduler, source connectors, built-in agent execution, recurrence generation, sync or multi-user access to close these gaps. The Nathan and Kevin examples remain the end-to-end product tests. Movement coordinates and records work; external assistants investigate and act within their existing authorization.

## Readiness

The independent-process choice is ready to carry forward. Complete the bounded-response and evidence-ordering contracts before the agent adapter slice, and the browser-auth contract before HTTP. Backup/recovery documentation and a precise support matrix are release gates. Existing A1–A25 scenarios remain necessary; the checks above sharpen their proof rather than declare them passed.


## Implementation follow-through

The bounded-context and assessment-ordering recommendations are now implemented as the detailed contracts in the spec. Tests cover 23-observation pagination, continuation conflicts after acknowledgement, multi-megabyte evidence without silent loss, delayed/equal-time checks, fresh reconciliation, and idempotent retry of an older committed assessment. This is implementation evidence, not a claim that every review gate is complete.
