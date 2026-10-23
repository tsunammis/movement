# Plan: Movement MVP

Status: implemented; local acceptance complete 2026-10-23 within decision 005 client scope | Owner: Stan | 2026-10-19

Inputs: [intent](intent.md), [spec](spec.md), [architecture](../../docs/architecture.md).

## Current baseline

All five delivery slices below are implemented. Local acceptance is complete for macOS 26.7.1 arm64, Chrome 155.0.8059.85 and the client scope in decision 005. The [acceptance audit](../../docs/acceptance.md) links the executed evidence. Codex Desktop is expected to work but remains unverified as an accepted non-blocking follow-up. The user has authorized committing this baseline and opening a PR; merging and the next UI intent are separate subsequent steps.

## Ordered delivery slices

### 1. Task storage and CLI: first useful vertical slice

Files: `go.mod`, `cmd/movement/main.go`, `internal/domain/{task,lifecycle,selection}.go`, `internal/app/{service,operations}.go`, `internal/store/text/{repository,lock,atomic,history,recovery}.go`, OS-specific lock/replacement files, `internal/config/config.go`, `internal/adapters/cli/cli.go`, colocated tests, `api/task.schema.json`, `api/history-event.schema.json`.

- Independent processes selected by the user; see decision 004 and `prototypes/storage/`. The prototype validates shared locking and recovery using independent subprocesses. No daemon comparison is required.
- Spike cross-process locking where applicable, replace-and-flush behavior, and permissions on macOS. Stop and revise the architecture if crash recovery cannot meet A8.
- Finalize task JSON and history-frontmatter schemas, event receipts, safe YAML parsing, input limits, and explicit migration strategy. Validate all example bundles.
- Implement staged manifests, durable preparation, idempotent roll-forward, reader gating, corruption detection, and interrupted cleanup. Inject failures at every phase, including between history installation and task replacement.
- Model flow separately from status; include delegate, deadline, follow-up time, and versioned check instructions and execution guidance in the schema.
- Implement init/create/get/list/update/history and selection rules, including waiting-for/person/follow-up filters, overdue included by default, and undated excluded by default. Add planning/execution views, stable as_of, distinct carryover/overdue predicates, and stable source/action/period ingestion keys. One task snapshot plus one Markdown event form a recoverable transaction; gate reads behind recovery as well as writes.
- Validate transitions, provenance, ingestion identity, revision conflicts, and retry receipts under the directory lock.
- Keep all data in a temporary test directory during verification.

Proof: A2-A4, A7-A8, A12, A18, A23-A24 and filesystem/schema portions of A10. Exercise concurrent independent processes and injected I/O failure; a mutex-only unit test is insufficient. Deliverable: useful local CLI with text persistence and history.

### 2. Agent context, work reports, and MCP

Files: `internal/app/{context,claims,reports,checks}.go`, `internal/adapters/cli/cli.go`, `internal/adapters/mcp/{server,tools}.go`, colocated tests, `tests/acceptance/mcp_test.go`, `docs/mcp.md`.

- Implement context packets, explicit missing-context behavior, leases, renew/release/override, report validation, and completion separation.
- Expose execution guidance, required capabilities, and expected outputs in context. Record guidance versions and artifact references in work reports; document external-side-effect reconciliation.
- Implement a single versioned completion criterion set, criterion-ID report validation, attempt-linked late observations and explicit reconciliation. Add atomic next-follow-up updates and satisfied-follow-up query behavior.
- Implement check reports with criterion evidence, met/not_met/inconclusive outcomes, revision/version conflicts, and history. Keep verification independent from execution claims; persist seems_completed transitions atomically with evidence and keep confirmed completion explicit. Validate rechecks, changed criteria, rejection, and non-executable review-state filtering.
- Select and pin a maintained Go MCP SDK after checking the current protocol and client support.
- Expose the spec's MCP tools over stdio, preserving errors and receipt semantics.
- Document absolute executable paths, explicit data directory, and client-specific setup only after testing it.

Proof: A5-A6, A13-A14, A21-A22, A25, A16-A17 with fake connectors, CLI/MCP parts of A1, A11 and A15, and prompt-like source content handled as data. Deliverable: an assistant can capture, select, read context, claim, and report work.

### 3. Local HTTP API

Files: `api/openapi.yaml`, `internal/adapters/http/{server,handlers,auth}.go`, `internal/config/config.go`, `cmd/movement/main.go`, colocated tests, `tests/acceptance/http_test.go`.

- Finalize browser session bootstrap and API credential storage before handlers.
- Implement loopback serving, request/response schemas, limits, authentication, origin/host validation, CSRF protection, pagination, and consistent domain errors.
- Exercise simultaneous CLI, MCP, and HTTP writes against the same directory.

Proof: HTTP portions of A1, A7, and A10. Deliverable: authenticated local API with no requirement that CLI/MCP use it.

### 4. Embedded web UI

Files: `web/{package.json,package-lock.json,tsconfig.json,vite.config.ts,index.html,src/}`, `internal/web/embed.go`, generated `internal/web/dist/`, `internal/adapters/http/server.go`, `.gitignore`, `tests/acceptance/web/`. Package manager/lockfile choice may be adjusted before scaffolding; commit the selected lockfile.

- Sketch inbox, timeframe list, and detail/history before implementation; keep the design within the accepted spec.
- Scaffold Vite + React + TypeScript in `web/`; pin toolchain/dependencies and define separate type-check, development, and build commands. Use simple React state initially.
- Proxy relative `/api` calls to Go in development, including authentication. Test exact-origin allowances, sessions and CSRF; retain packaged security defaults.
- Build assets into `internal/web/dist/` and embed them in Go; implement UI navigation fallback without masking API or static-asset errors.
- Add triage, edit, completion, claim acquire/renew/release/replacement, structured work/check reports, and conflict recovery flows using the HTTP API. Preserve evaluated context and entered report evidence across conflicts; support explicit late-observation retention and reconciliation. Include waiting-for/person/deadline views, execution-guidance/check prompt editing, draft/output references, and criterion evidence with observation times and outdated-result labels.
- Bundle assets and sanitize source/Markdown output. Include keyboard interaction and narrow-view checks.

Proof: full A1, A19-A20, browser portions of A9-A10, manual UX walkthrough in Vite and the packaged binary. Deliverable: one executable serves all runtime UI assets.

### 5. Packaging and end-to-end acceptance

Files: `scripts/build.sh` (or equivalent chosen build tool), `tests/acceptance/packaging_test.go`, `docs/{installation,backup-and-recovery}.md`, `README.md`.

- Add a reproducible release pipeline: locked frontend dependency installation, separate type-check, clean Vite production build, then Go compilation. Ignore generated assets; fail rather than reuse stale output.
- Build per accepted macOS/CPU target with no required external runtime; validate any CGO/dependency assumptions.
- Run packaged-binary acceptance on each advertised macOS/CPU target, not cross-compilation alone.
- Document backup with writers stopped, restore, upgrade constraints, local-only scope, and credential regeneration after copying data.
- Run actual client tests and the complete A1-A25 matrix. Record environment, commands, outcomes, and limitations here.

Deliverable: reproducible local release candidate. Publication is a separate user-requested action.

## Dependencies and risks

Slices are sequential: durable domain/storage precede all adapters; context/claims precede agent handoff; HTTP precedes browser UI. Biggest risks are macOS crash guarantees, client transport differences, unbounded history growth, manual file edits bypassing coordination, and unintuitive date selection. The design intentionally defers cloud sync, automatic semantic merging, and built-in execution.

## Verification and closure

The storage prototype passed independent-process crash, contention and recovery tests; production repository tests now cover the complete application schema and injected process/I/O faults. Neither establishes hardware power-loss guarantees.

Final rebuilt-binary acceptance passed all nine CLI/MCP/HTTP scenarios (66.874s); the full Go race suite and vet passed. Frontend type-check/build passed. The packaged/Vite browser suite passed 18 scenarios, with two intentionally packaged-only skips. Clean-copy packaging passed all five stages, including an isolated runtime with no Node or external UI assets. UTC normalization, equivalent-offset retries, backup/restore and unsupported-schema protection have retained tests.

Claude Desktop passed smoke and synthetic context/claim/progress/proposal/human-confirmation workflows with independent persisted-state readback. Claude Code and Codex CLI tests passed. The user accepted Codex Desktop testing as a non-blocking follow-up on 2026-10-23; keep its compatibility labeled expected but unverified. Routine scheduling and real source connectors remain external to Movement.

The [implementation audit](../../docs/reviews/implementation-audit.md) maps requirements and the [acceptance audit](../../docs/acceptance.md) maps A1–A25. Storage measurements cover 10 and 50 tasks; no larger capacity guarantee is advertised. Other platforms, browsers and hardware power-loss behavior are not certified by this evidence.

## Handoff

Commit the baseline and open a PR as requested. After merge, start a separate UI intent focused on viewing tasks and beginning personal use. This baseline already includes a functional embedded UI; the next intent should build on it. Do not merge the PR or create real tasks/routines as part of the baseline submission.
