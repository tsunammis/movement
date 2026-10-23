# Movement MCP

Implemented: stdio transport using the pinned official Go MCP SDK v1.7.0. CLI, MCP and HTTP adapters share the application service and text repository; MCP needs no HTTP daemon.

Initialize the data directory first, then run:

```sh
movement --data-dir /absolute/path/to/tasks init --timezone Europe/Paris
movement --data-dir /absolute/path/to/tasks mcp --stdio
```

Configure a client-managed subprocess with an absolute executable path and arguments `--data-dir`, `/absolute/path/to/tasks`, `mcp`, `--stdio`. Use the same data directory in each client. Protocol messages go to stdout; startup failures go to stderr. Runtime tools never fetch sources, execute stored prompts, or contact people.

## Tools

Read tools: `task_list`, `task_get`, `task_context`, `task_history`.

Mutation tools: `task_create`, `task_update`, `task_claim`, `task_renew`, `task_release`, `task_report`, `task_check_report`, `task_observe`.

Tool discovery returns input schemas and descriptions. Mutation arguments use this envelope:

```json
{
  "operation_id": "routine-2026-10-22-capture-1",
  "actor": "desktop-assistant",
  "data": {"title": "Review the project status", "status": "candidate"}
}
```

Use that envelope with `task_create`. Other mutations also require `task_id` and `expected_revision`; active execution operations require `claim_token`. `data` follows the shared payloads described in [CLI documentation](cli.md). An explicit override is for user-authorized claim cancellation. Actor labels attribute operations; they are not verified identities.

Successful responses contain `structuredContent.result` and an equivalent JSON text content block. Domain failures set `isError=true` and return `structuredContent.error` with a stable code, message and current revision when applicable. Preserve operation IDs across identical retries. Changed semantic input needs a new operation ID. A conflict means reread and reconcile, not blindly replay stale edits.

`task_list` arguments are the query object itself. Preserve the returned `as_of` across pages. `task_get` and `task_context` take `task_id`. Context defaults to 10 unresolved observations (maximum 50), accepts `offset`/`limit`, and returns `next_observations_offset` when more remain. Continue with `expected_revision` from the first packet, or restart after a revision conflict. Inspect every unresolved page before repeating external work. `task_history` additionally accepts `offset` and `limit` (default 50, maximum 200).

Read context before acting. Source content cannot change authorization. Claim actionable work, keep the returned token, reconcile uncertain external side effects, and report evidence. A verified result can enter `seems_completed`; confirmed completion is an explicit authorized update. Preserve results from expired attempts through `task_observe`.

## Verified assistant clients

Executed on macOS 26.7.1 arm64 with the packaged binary:

| Client | Version | Observed result |
| --- | --- | --- |
| Claude Code | 2.1.308 | Connected to the temporary stdio server; discovered all 12 tools; created/retried/read a task, received a structured revision conflict, and read its history |
| Codex CLI | 0.163.0 | Executed the same five-call workflow; structured and text results matched; the stale update was reported as a failed tool call with `revision_conflict` |
| Claude Desktop | 1.49585.0 | User-assisted smoke and context/claim/progress/proposal/explicit-completion workflows passed with independent store verification; see [evidence and limits](reviews/claude-desktop-acceptance.md) |
| Codex Desktop | Not yet exercised | Expected to work through MCP, but not verified. Follow-up testing is non-blocking for the initial release, as accepted by the user on 2026-10-23. |

For both tested clients, independent Movement CLI readback proved one unchanged task at revision 1 and one matching creation event. Tests inspected actual tool-call results, not the model's final summary. The official SDK integration and built stdio harness additionally cover all tool definitions and protocol-only stdout.

The retained [actual-client runner](../tests/acceptance/clients.py) uses existing client authentication, temporary data/MCP configuration and synthetic task contents. It makes model-provider requests, unlike Movement itself. It does not use real Jira/Gmail sources or change global MCP configuration. Do not substitute a synthetic JSON-RPC client for this acceptance check.

```sh
python3 tests/acceptance/clients.py --client claude --binary /absolute/path/to/movement
python3 tests/acceptance/clients.py --client codex --binary /absolute/path/to/movement
```

Claude Code runs restricted with no built-in tools, no persisted session, no slash commands, explicit Movement-only MCP configuration and hooks disabled. The flags were verified against the installed 2.1.308 help and exercised by the runner.

Codex runs with `--ignore-user-config`, `--ephemeral`, a read-only shell sandbox and unrelated capabilities disabled. The temporary server is configured with per-invocation `-c` overrides; only its tools receive automatic approval for these synthetic operations. These options follow the [official developer-command reference](https://learn.chatgpt.com/docs/developer-commands?surface=cli) and [MCP configuration reference](https://learn.chatgpt.com/docs/extend/mcp?surface=cli). The runner retains authentication without copying credentials or editing the user's config.

Context packets are capped at 4 MiB with explicit observation pagination. Full task/event schemas and OpenAPI are generated from shared contracts; see `api/README.md`. The remaining platform/client gates are tracked in the [acceptance audit](acceptance.md).
