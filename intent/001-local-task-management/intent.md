# Intent: local task management for people and agents

Status: draft | Owner: Stan | Created: 2026-10-19 | Acceptance: pending

## Problem

Assistants can discover action items across multiple sources, but need a common place to preserve those tasks, retrieve actionable context, and report what they did. This includes commitments delegated to other people: the user needs to know who they are waiting for and how to verify delivery without doing the work themselves. The record should outlive any assistant session and remain under the user's control.

## Desired outcome

A routine in Codex Desktop, Claude Desktop, or another client identifies possible tasks and writes them to Movement through local MCP or HTTP. The user can review them from a browser or CLI. Later, an assistant asks for tasks in a timeframe, reads their context, performs authorized work, and records progress and outcomes. For delegated work, the user records the responsible person, deadline, and a “check if done” prompt. An assistant retrieves the task, checks the specified sources, and records whether the criteria are met, unmet, or could not be determined, with evidence. All interfaces observe the same persistent state and history.

Example: “Waiting for Nathan Delange to fill in the fortnightly by Friday.” Its check prompt asks the agent to inspect `https://foo.bar/in` for sections A, B, C, and D. The URL is illustrative; this proposal does not access it.

Personal task example: “Write an email to Kevin by Friday.” Saved guidance tells the assistant to inspect a specified Jira project, check its deadline, summarize comments, and create a Gmail draft for review without sending it. The assistant records the draft reference and supporting evidence. Movement stores this context; an external request or routine starts the assistant.

## Users and systems

- A single user managing their work locally.
- Desktop assistants and coding agents with access to Movement's local interfaces.
- External source systems, accessed by those assistants using their existing integrations.
- Movement's CLI, MCP server, HTTP server, embedded browser UI, and user data directory.

## Success criteria

- Capture, review, retrieve, and report work across different interfaces without losing context or duplicating retries.
- Restart Movement and recover every acknowledged change and its history.
- Move the data directory to another supported machine and read it with the matching binary, without a database service.
- Select a timeframe and receive both suitable tasks and an explanation of their selection.
- Inspect task text and history without running the application.
- List delegated commitments by person and deadline in “waiting for”, separately from work the user should execute.
- Retrieve a saved check prompt and record evidence for each criterion; an unavailable source must not be reported as proof of non-completion.

## Constraints

Local services; text storage in a chosen user directory; portable executable preferred; CLI, MCP, HTTP, and embedded web UI; no dependence on a particular assistant. See the [constitution](../../docs/constitution.md).

## Initial scope proposed

Candidate capture, user triage, task editing, timeframe queries, execution context and explicit personal-task guidance, output references, work reports, delegated commitments, completion-check prompts and results, durable history, and basic agent coordination. Single user and local filesystem first.

## Exclusions proposed

Source connectors, source polling, routine scheduling, built-in AI inference, automatic external actions, multi-user collaboration, cloud sync, mobile apps, and recurring-task generation.

## Questions carried forward

Production hardening of the selected independent-process storage, split-file recovery, completion-state edge cases, priority/scheduling details, and minimum macOS version/CPU targets. The user accepted macOS-only initial support, Movement-first access with freely readable files, overdue-included/undated-excluded timeframe defaults, and an intermediate seems-completed state on 2026-10-22. Recommendations and their consequences are tracked in [open decisions](../../docs/decisions/README.md).
