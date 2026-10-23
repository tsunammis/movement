# 004: Independent-process storage prototype

Status: independent-process direction accepted; prototype evaluated | Owner: Stan | Date: 2026-10-22

## Decision and authorization

Stan chose independent processes and requested the prototype: “I'm in favor of the Independant process. Let's build the prototype.” This supersedes the planned comparison with a single local service. It authorizes the storage experiment, not the entire product implementation.

CLI, MCP, and HTTP processes will share repository code and a stable directory-level filesystem lock. Recovery runs before reads and writes. No always-running storage daemon is required.

## Prototype

See [storage prototype](../../prototypes/storage/README.md). A separate Go module implements a minimal title/revision record, Markdown events, exact-input retry receipts, and a single prepared journal holding both target files. Destination durability precedes journal removal; a separate committed marker is unnecessary in this serialized protocol.

## Evidence and limits

Executed on macOS arm64 with Go 1.27.3. Subprocess tests cover ten SIGKILL checkpoints across creation/updates, concurrent independent writers and identical retries, a second interruption during recovery, lock timeout/owner death, read locking, conflicting event preservation, and installation failure/recovery. Results and reproducible commands are in the prototype README and implementation plan.

The experiment supports proceeding with independent processes for application-crash recovery on the tested machine. It does not prove power-loss durability, hostile-filesystem safety, disk-full behavior, scale, or all macOS versions. Those limitations remain explicit before production repository acceptance.

## Specification reaffirmation: 2026-10-23

The user requested that this choice be reflected in the spec and challenged before further implementation. Independent CLI, MCP and HTTP processes remain the selected topology. They share one repository protocol and serialize access with one stable lock; independent processes do not mean concurrent unsynchronized writers. External investigations and connector calls stay outside the storage critical section. The current [final review](../../intent/001-local-task-management/final-review.md) records remaining contract and acceptance gaps without reopening the daemon choice.
