# Movement

A local task manager for people and AI agents. Capture candidate tasks, track delegated commitments in a “waiting for” flow, choose work for a timeframe, and give an agent instructions to act or check completion. Retain results and evidence in task history.

**Current phase: MVP implemented and local acceptance complete for the documented platform/client scope. The binary includes CLI, stdio MCP, authenticated HTTP API and embedded web UI. Codex Desktop is expected to work but remains unverified as an accepted follow-up. See [acceptance evidence](docs/acceptance.md) and [installation](docs/installation.md). No release has been published.**

## Start here

1. [Project constitution](docs/constitution.md): purpose and durable constraints.
2. [Initial intent](intent/001-local-task-management/intent.md): problem, outcomes, scope.
3. [Specification](intent/001-local-task-management/spec.md): behavior and acceptance criteria.
4. [Proposed architecture](docs/architecture.md): binary, interfaces, storage, tradeoffs.
5. [Implementation plan](intent/001-local-task-management/plan.md): delivery slices and proof.
6. [Data model](docs/data-model.md): JSON state, Markdown history, and guidance contracts.
7. [Personal task example](examples/personal-email.md): Jira research followed by a Gmail draft.
8. [Delegated task example](examples/waiting-for.md): a commitment, deadline, and completion-check prompt.
9. [Open decisions](docs/decisions/README.md): recommendations to review.

[AGENTS.md](AGENTS.md) supplies repository instructions. [CLAUDE.md](CLAUDE.md) points Claude to the same instructions. The constitution is kept separately so product principles are not mixed with tool-specific setup.

## Proposed product

One Go binary per supported CPU target, initially for macOS only, containing the CLI, MCP server, HTTP API, and web UI. The UI uses React + TypeScript with Vite during development; built assets are embedded in Go, so users need no Node runtime. No hosted backend, runtime database service, or model dependency. Task records live as UTF-8 text in a user-selected directory.

External assistants own source integrations, scheduling, inference, execution, and completion checks. Movement owns task state, context, provenance, and history. An assistant can use remote sources or models; Movement itself works offline.

## Repository layout

```text
AGENTS.md                         Shared agent instructions
CLAUDE.md                         Claude entry point
docs/constitution.md              Product principles
docs/workflow.md                  Artifact workflow and review rules
docs/architecture.md              Proposed technical design
docs/decisions/                   Architecture and product decisions
intent/001-local-task-management/ Initial intent, spec, and plan
templates/                       Templates for subsequent changes
examples/                        Synthetic task format example
```

Application code is in `cmd/` and `internal/`; frontend code is in `web/`, with built-binary acceptance harnesses in `tests/acceptance/`. The isolated [storage prototype](prototypes/storage/README.md) has build/test commands; application build and usage commands are in [CLI documentation](docs/cli.md). The initial baseline is being submitted for review; merging and distribution are separate steps.

Backup and restore: follow the [stopped-store procedure](docs/backup-and-recovery.md), including validation and HTTP credential rotation before serving a restored copy.

See [local installation and runtime requirements](docs/installation.md) and [tested MCP clients](docs/mcp.md).
