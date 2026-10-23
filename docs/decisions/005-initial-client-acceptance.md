# 005: Initial client acceptance scope

Status: accepted by Stan, 2026-10-23.

## Decision

Codex Desktop testing is a non-blocking follow-up for the initial local release. Describe compatibility as **expected to work through MCP, not yet verified**. Do not infer verified Desktop behavior from Codex CLI tests.

The tested client scope is Claude Desktop 1.49585.0, Claude Code 2.1.308 and Codex CLI 0.163.0. Claude Desktop has user-assisted smoke and synthetic context/claim/progress/proposal/human-confirmation evidence with independent durable readback. Exact evidence and limitations are linked from [MCP documentation](../mcp.md).

## Rationale and consequences

The shared MCP implementation has real-client evidence from both vendors and an actual Desktop workflow. Requiring a second Desktop test need not block the initial release. Keep the test as a follow-up, preserve the distinction between expected and verified compatibility, and avoid certifying client-specific routine scheduling. Movement remains independent of a client vendor and contains no scheduler or external connectors.

This changes the acceptance gate, not the task-management implementation scope. It does not authorize commits, publication or changes to external source systems.
