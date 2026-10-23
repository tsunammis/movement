# 002: Vite, TypeScript, and React for the UI

Status: accepted | Owner: Stan | Date: 2026-10-22

## Context and decision

Movement's task lists, filters, editing, history, and completion review justify a dedicated UI development setup. Use Vite + TypeScript + React in `web/`, replacing the earlier plain HTML/small-JavaScript proposal.

During development, Vite serves the frontend and proxies `/api` to the local Go API. During release builds, type-check and build the frontend into `internal/web/dist/`, then embed those assets in Go. The installed application remains one binary with no Node or Vite runtime dependency.

## Consequences and validation

Pin frontend dependencies and commit the chosen lockfile. Keep React state management simple initially. The Go application service remains authoritative for task behavior. Generated assets are not committed and must be rebuilt before release compilation.

Validate sessions, origin checks and CSRF through the development proxy; do not relax packaged security defaults. Exercise offline assets, navigation, task editing, and errors against the packaged binary as well as the development server. A19-A20 in the specification cover these checks.

## Acceptance

Accepted by Stan on 2026-10-22 with “perfect, please update the spec accordingly,” following the Vite + TypeScript + React recommendation. This accepts the tooling direction, not implementation of the UI or the full implementation plan. Versions and browser targets remain to be selected.
