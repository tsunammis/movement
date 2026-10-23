# Final specification challenge

Date: 2026-10-23. Scope: documentation review before further implementation. Independent processes are accepted; recommendations below are review findings, not new user approvals. Existing code and test records were inspected as context; no runtime tests were rerun for this review.

## Verdict

The architecture is coherent and ready to carry forward. CLI and MCP need no daemon; HTTP serves the browser; every process shares repository rules, a stable lock, and recovery before reads/writes. Independent processes serialize storage access. Network investigations and external actions belong outside that critical section.

The workspace already contains implementation beyond the prototype. Readiness here means that the design can guide remaining work, not that implementation has yet to start or that release acceptance has passed.

## Remaining challenges

| Priority | Gap and concrete consequence | Resolution / next action | Proof |
| --- | --- | --- | --- |
| Before accepting persistence | The review found incomplete UTC normalization for source capture, delegation and nested reports. The shared service now normalizes typed inputs before hashing; equivalent instants have equivalent digests. Final adapter and persisted-file verification must retain this contract. | Apply the existing UTC requirement to every typed timestamp in state and history. Offset-equivalent retry payloads share a digest; preserve omitted/null patch distinctions. Never rewrite timestamps embedded in user prose. | Non-UTC inputs through every adapter persist UTC instants without changing their meaning. Cover nested history fields, semantic context changes and equivalent-instant retries. Service tests cover equivalent-offset create/update/check retries and nested values; full adapter/disk coverage remains part of the acceptance audit. |
| Before accepting R7 | Representative browser workflows do not prove every domain operation and relevant option is exposed. | Compare each interface-table operation and its options against the actual UI. Claim controls, structured reports, historical observation import and selection controls now have recorded implementation; audit remaining omissions rather than reimplementing them. | An operation matrix links UI controls to shared service behavior, including revision conflicts, uncertain responses and retained input. Complete keyboard-only and narrow-view walkthroughs. |
| Before claiming Desktop compatibility | Actual Claude Code and Codex CLI evidence does not establish Claude Desktop/Codex Desktop setup or routine behavior, which motivated this product. | Exercise the named Desktop apps with the packaged binary, explicit data directory and real tool discovery. Distinguish protocol compatibility from client-specific routine scheduling. Movement itself remains unscheduled. | Create/retry/read/report/history through actual Desktop tools, clean stdio and structured failures; record client versions and any routine limitations. |
| Before release | macOS-only still leaves minimum OS/CPU and browser support unspecified. | Publish a precise support matrix based on executed evidence. Do not turn a successful cross-build into a runtime-support claim. | Run packaged acceptance on every advertised combination; retain recorded macOS arm64/Chrome evidence as evidence for that combination only. |
| Before scale claims | A single lock plus full record/history validation makes store growth a potential latency issue; bounded responses do not bound scan cost. | Keep the simple topology. Measure realistic task/history volumes and contention before promising capacity. Add a derived index only if evidence warrants it; no new database or daemon is required by this review. | Record task/event counts, list/context/mutation latency and lock timeouts under concurrent processes. Establish a documented tested envelope rather than an invented performance guarantee. |

## Gates with recorded coverage

Backup/restore now has a stopped-writer procedure, authoritative receipt preservation, prepared-transaction recovery and credential rotation tests. Unsupported-version guards cover mixed stores and pending transactions without changing authoritative files. Packaging tests exercise an isolated binary without Node or external assets. Storage tests include process interruption and injected I/O failures. These are retained regression gates, not missing architecture decisions; this documentation pass did not rerun them.

Process-crash evidence still does not establish hardware power-loss durability. External draft creation still requires reconciliation after uncertain outcomes: a Movement receipt cannot deduplicate an action in Gmail. Neither limitation justifies expanding the MVP.

## Resolved concerns that should stay resolved

The spec already defines semantic context versions, pending versus due follow-ups, claim cleanup, store-wide operation identity, authoritative receipts, bounded context pagination, observation-time ordering and HTTP session security. These are no longer discovery questions. Existing evidence is indexed in the [acceptance audit](../../docs/acceptance.md); this review does not independently certify those tests.

The previous [operational review](../../docs/reviews/2026-10-22-operational-contracts.md) remains available for rationale. Its proposed limits and pre-implementation wording are historical; current spec contracts take precedence.

## Scope and next step

Do not add a scheduler, source connectors, built-in execution, recurrence generation, sync or multi-user access. The Nathan commitment and Kevin draft remain the representative workflows. Movement records attributed evidence; it cannot guarantee that an external agent read every source or obeyed a draft-only instruction.

Resolve timestamp normalization and retry semantics first, then finish the UI parity and Desktop/platform acceptance audits using the existing plan. Measure storage growth before making capacity claims. No further process-topology exploration is needed. Full MVP readiness remains conditional on the acceptance audit, not on this review alone.

## Accepted client scope update — 2026-10-23

Claude Desktop's smoke and synthetic execution-through-human-confirmation workflow passed with independent readback. The user accepted Codex Desktop testing as a non-blocking follow-up. Expected compatibility must not be labeled verified. Initial tested clients are Claude Desktop 1.49585.0, Claude Code 2.1.308 and Codex CLI 0.163.0. This supersedes the earlier requirement to exercise both Desktop applications before initial acceptance.
