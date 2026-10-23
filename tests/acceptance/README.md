# Built-binary acceptance

Build the release binary with `./scripts/build.sh`, then run:

```sh
python3 tests/acceptance/interfaces.py /absolute/path/to/movement
MOVEMENT_BINARY=/absolute/path/to/movement npm run test:e2e --prefix web
```

The Python suite uses only the standard library. It launches real independent CLI, MCP stdio and loopback HTTP processes against temporary stores. It never uses a personal task directory or real Jira/Gmail connectors. The fake connector tracks created drafts and send attempts, simulates missing access/unresolved inputs and a lost response after saving, and reconciles by an external operation identity before retrying. One test waits for a real 30-second lease to expire.

Coverage includes creation/readback after restart, equivalent structured errors, operation receipt replay across adapters, races between the three processes, claim/renew/release, completed/dismissed/manual-decision preservation, distinct recurrence identities, A–D completion checks, follow-up scheduling, stale/partial/duplicate/unknown criteria, progress/blockers, draft output evidence, overdue/carryover/boundary selection and pagination. These are deterministic workflow tests, not proof that an arbitrary external agent will follow instructions.

The browser suite uses installed Chrome in an isolated profile. It tests both packaged and Vite workflows, including task editing, revision conflicts, completion review, late-observation paging and acknowledgement. A packaged-only scenario creates via a real MCP subprocess, restarts HTTP, compares CLI/HTTP data and reads the same task through the UI. That scenario also tests malicious source text, keyboard login and outdated-evidence rendering. The Vite scenario verifies a hot stylesheet update without reload or loss of unsaved input; its unique test rule is removed afterward.

For focused reruns:

```sh
python3 tests/acceptance/interfaces.py /absolute/path/to/movement Acceptance.test_races_between_independent_adapters
MOVEMENT_BINARY=/absolute/path/to/movement npm run test:e2e --prefix web -- --project=packaged --grep MCP-created
```

Production repository subprocess crash and I/O failure tests live in `internal/store/text/`. They include structured work/check/observation payloads through each transaction phase. Run them with `go test -race ./internal/store/text` after generating frontend assets if needed for a full root test run.

Loopback listeners and isolated browser processes may need sandbox escalation. All stores are temporary and cleaned up. Backup/restore is covered by repository and packaged browser tests; release packaging has the retained test below. Actual supported assistant-client and platform acceptance remain tracked in `docs/acceptance.md`.

## Reproducible release packaging

```sh
python3 tests/acceptance/packaging.py
```

This test copies the current source into a temporary directory without `node_modules`, generated assets or a binary, and runs the actual release build script. It injects a TypeScript error and a Go-invocation sentinel to prove frontend failure prevents Go compilation and preserves the prior executable. A succeeding clean rebuild must remove a stale asset.

It then copies only the executable into an isolated runtime and deletes the temporary source/dependency/asset tree. The server and CLI run with `PATH` pointing to an empty directory. HTTP checks cover embedded assets, authentication and missing-resource errors. A separate browser harness uses installed Chrome/Playwright to log in, navigate directly, reload and edit a task; the isolated CLI reads the edit back. Browser test tooling uses Node outside the server environment; Movement itself has no Node access through PATH and no separate UI directory.

The test requires the workspace Playwright dependency (`npm ci --prefix web`) and installed Chrome. Build dependencies may be downloaded by `npm ci`, or use an already populated npm cache with `npm_config_offline=true`. Go/npm cache locations can be passed through the normal environment variables. Stores/build copies are disposable; the working tree's binary and assets are untouched. Loopback/browser execution may require sandbox escalation.

## Actual assistant clients (opt-in)

```sh
python3 tests/acceptance/clients.py --client claude --binary /absolute/path/to/movement
python3 tests/acceptance/clients.py --client codex --binary /absolute/path/to/movement
```

These tests require an installed, authenticated client and network access to its model provider; they use normal client account quota. They create only synthetic local tasks. Configuration/store directories are temporary, sessions are ephemeral, unrelated capabilities are disabled, and global MCP configuration is not changed. No external source connectors or real personal tasks are used.

The runner inspects actual client tool-call output, identical retry receipts, structured revision-conflict errors and persisted CLI task/history readback. Unexpected tool calls fail the test. Timeouts terminate the test client process group; they are failures, not compatibility passes. CLI versions differ over time, so an unsupported flag or changed event format should be investigated rather than bypassing the assertions. Desktop applications and routines require their own checks; these CLI tests do not certify them.
