# Web UI and embedded builds

The React/TypeScript UI provides inbox, personal work, waiting-for, undated and all-task views; date/status/person/follow-up filtering; structured task creation/editing; guidance, criteria and delegation; evidence/history inspection; completion review and explicit lifecycle actions. Source prose is rendered as text, not raw HTML. Only HTTP/HTTPS references become links, opened without an opener or referrer.

Browser authentication uses the [local HTTP session contract](http.md). The UI keeps the CSRF token in memory and does not persist the bearer credential. A login token comes from `movement --data-dir PATH auth token`. Task links use `#task/<id>` so direct links and browser refresh work with embedded assets.

## Development

Use Node 24.14.1 or later and the pinned dependency lockfile. Node is needed only for development/builds. The installed versions are recorded in `web/package.json` and `web/package-lock.json`; setup follows the official [Vite guide](https://vite.dev/guide/) and [React documentation](https://react.dev/).

```sh
npm ci --prefix web
npm run build --prefix web
# Build Go after the generated assets exist.
go build -o bin/movement ./cmd/movement
bin/movement --data-dir /absolute/path/to/dev-tasks init --timezone Europe/Paris
bin/movement --data-dir /absolute/path/to/dev-tasks serve --dev-origin http://127.0.0.1:5173
```

In another terminal:

```sh
npm run dev --prefix web
```

Open `http://127.0.0.1:5173`. Vite proxies relative `/api` calls to `http://127.0.0.1:8484`, preserving Host, Origin, authentication and CSRF headers. Override the backend URL with `MOVEMENT_API_URL` when needed. Both servers stay on loopback. The production server does not enable the dev-origin exception by default.

## Release build

```sh
./scripts/build.sh
bin/movement --data-dir /absolute/path/to/tasks serve
```

The script runs locked dependency installation, TypeScript checking and a clean Vite build before Go compilation. Frontend failure stops the build. Generated `internal/web/dist/` assets and `web/node_modules/` are ignored. A clean checkout must build the frontend before compiling or testing Go because `go:embed` requires its inputs.

The binary embeds HTML, JavaScript and CSS, uses no CDN or remote fonts, and runs without Node or an external assets directory. The embedded handler serves known files only; missing assets and API errors are not replaced by the entry page. UI navigation uses hash routes rather than server-side path fallback.

Build targets are Safari 17, Chrome 120 and Firefox 121 or newer. These are compilation targets, not a claim that every browser has been exercised. Browser acceptance uses installed Chrome in an isolated profile. Other browser acceptance remains a release gate.

## Interaction contracts

- Selection options expose planning versus executable work, independent overdue/carryover inclusion, an explicit undated override, a reproducible RFC 3339 evaluation time and page size. Task cards show server-provided inclusion reasons and expired-attempt reconciliation hints. Eligibility is labelled at evaluation time; claiming always revalidates current state. Changing filters invalidates pending pagination responses so old pages cannot enter a new result set.
- Timeframe dates use the repository IANA timezone through the shared server date-window conversion. The end date is exclusive. Deadline/schedule editor inputs explicitly show the browser timezone.
- Editing preserves stable criterion IDs. Guidance and criteria versions are assigned by the application service.
- Saves send changed fields. A revision conflict retains unsaved input and displays the latest task; applying edits to its revision requires a separate explicit action.
- Mutation retries reuse the operation ID for identical payloads while the page remains open. Reloading during an uncertain write requires checking the task/history before resubmitting.
- Completion and dismissal ask for a deliberate confirmation. Active claims require an explicit override. Completion records a decision; it never runs an external action.
- Execution claims support acquire, renew, release and deliberate replacement through the shared API. Lease duration is 30–3600 seconds. Claim tokens remain only in memory for the selected task and are shared with its editor. Reload, navigation away, new-task capture or workspace lock clears them. Token-authorized edits preserve the claim; lifecycle transitions use the same token, while override remains explicit. A token from another client can be entered explicitly. Claim/revision conflicts require refreshing and reviewing before retrying; taking over a claim never performs external work.
- Structured work reports record progress, blockers or proposed completion; check reports compute the aggregate from per-criterion results. Both support multiple evidence entries and artifact references. Checks can leave, clear or reschedule follow-up; reports can reconcile explicitly selected late observations. These forms never fetch sources or perform external work.
- A report retains its evaluated revision, context and instruction versions while it is open, including across list/detail refresh. Conflict recovery displays the latest task before explicitly adopting its write revision. Changed context/attempts disable normal submission; the entered report can instead be preserved as a late observation. Reopen a fresh report only after reassessing current instructions. Observation timestamps are explicit RFC 3339 instants, not automatically refreshed on retries.
- Standalone historical-report entry is available even for candidate, done and dismissed tasks. Paste the original report object from the assistant, review its summary/evaluated context, then save an observation without changing business state. Original JSON reaches strict server validation, including duplicate/unknown-key checks. Revision conflicts preserve the report and require explicit review of the latest write revision. Whitespace/key-order changes alone retain retry identity after a lost response.
- History is paginated. Evidence includes its observation time, source, outcome and outdated-context indication. Late observations remain visible until reconciled or explicitly acknowledged with a reason.

## Browser tests

Build the binary first, then:

```sh
MOVEMENT_BINARY=/absolute/path/to/movement npm run test:e2e --prefix web
```

The Playwright suite creates temporary repositories and local servers, testing the packaged and Vite paths. It uses the locally installed Chrome channel and does not use an existing personal browser profile. Tests and screenshots are under `web/tests/` and ignored `web/test-results/` respectively. The implementation status records executed evidence and remaining acceptance work.

The retained release check `python3 tests/acceptance/packaging.py` builds from a clean temporary source copy, verifies frontend failure blocks Go, then removes the build tree and serves a copied executable with an empty runtime PATH. A separate real-browser harness verifies the embedded UI and CLI readback. See the [acceptance runner](../tests/acceptance/README.md) for prerequisites.

The task editor exposes the subset of authoritative criteria covered by a completion check. Existing partial checks retain their selection on unrelated edits; new criteria are not silently added to an existing check, and removed criteria are removed from its references. An actionable task can retain delegation metadata using “Keep delegation details”; unchecking it explicitly removes that metadata. Delegation timestamps are editable as RFC 3339 and unchanged values retain their precision.

Task details show the server-provided missing-context list with guidance for adding objectives, criteria, execution instructions or investigation instructions. These diagnostics do not prevent manual tracking. Each history entry with recorded changes has an expandable section showing updated field values and lifecycle/coordination metadata. Nested values remain readable as escaped text, including cleared fields; source text never becomes executable markup.

New tasks can be captured as ready work or inbox candidates. Optional repeated-discovery fields construct the stable producer/source/action/occurrence identity used by all adapters. Reusing it opens the existing task, including dismissed or completed tasks, without replacing edits or adding task history. A changed occurrence represents a new task. Existing identities are displayed read-only in the editor. Source capture times can be entered or corrected explicitly as RFC 3339 instants.

Unreconciled observations support individual or selected-batch acknowledgement with an explicit reason. Selection persists across observation pages; loading more evidence does not select it. A revision conflict preserves the selection and reason and requires refresh/review before resubmission. Uncertain-response retries reuse the same operation identity. History lists the observations covered by each acknowledgement; acknowledgement does not apply evidence to completion or change task state.

Selection supports one status or an explicit set through “Choose multiple statuses”. Selection options offer calendar-date windows in the repository timezone or exact RFC 3339 instants with offsets, plus person filtering in any view. Switching timeframe formats clears the old boundaries rather than silently converting them. Both boundaries must be supplied together; empty custom status sets and invalid filters clear previous results and show an error instead of displaying an apparently matching stale list.

The skip link moves keyboard focus to the workspace without changing the selected task URL or discarding its in-memory claim token. The retained keyboard-only browser scenario exercises login, claim/release, candidate creation, acceptance, editing, history expansion, completion/reopening and locking through sequential keyboard navigation in both packaged and Vite modes. Native selects support type-ahead in the tested Chrome environment.

The Selection options panel also exposes **Only undated tasks** in every view, including Waiting for and All tasks. The Undated shortcut selects personal undated work; switching views resets this filter to that view's default. It is separate from including undated work alongside dated tasks in a timeframe.
