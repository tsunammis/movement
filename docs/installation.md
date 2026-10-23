# Build and run Movement locally

The validated environment is macOS 26.7.1 on Apple Silicon (arm64). Other macOS versions and Intel have not yet been execution-tested. Linux and Windows are outside the initial release scope. This is an unpublished local release candidate; see the [acceptance audit](acceptance.md) for executed evidence, tested scope and deferred follow-ups.

## Build

Development needs Go 1.27 and Node 24.14.1 or later with npm. The lockfile pins frontend dependencies. From the repository root:

```sh
./scripts/build.sh
```

The build installs locked frontend dependencies, type-checks and builds the UI, then compiles `bin/movement` with CGO disabled. Frontend errors prevent Go compilation. A clean checkout cannot compile the embedded package until the frontend assets exist.

The resulting executable includes the CLI, stdio MCP server, HTTP server and all web assets. Copy the executable wherever you want to run it; the runtime needs no Go, Node, Vite, database server, source checkout, or separate UI directory. The retained packaging test verifies this with an isolated runtime and real browser.

## Initialize and open the UI

Choose one explicit, private local data directory; do not use a network filesystem or a live cloud-sync folder. Run these with the built executable or an absolute path to its installed copy:

```sh
bin/movement --data-dir /absolute/path/to/tasks init --timezone Europe/Paris
bin/movement --data-dir /absolute/path/to/tasks auth token
bin/movement --data-dir /absolute/path/to/tasks serve
```

Open the loopback URL printed by `serve` and enter the token. Initialization preserves an existing directory's settings and token. The token is exchanged for a browser session; treat it as private. Chrome 155.0.8059.85 is the browser used in executed acceptance; Safari/Firefox compilation targets are not runtime compatibility evidence.

CLI and MCP work independently without `serve` running:

```sh
bin/movement --data-dir /absolute/path/to/tasks task list --json
bin/movement --data-dir /absolute/path/to/tasks mcp --stdio
```

Configure assistant clients to launch the same executable with the same explicit data directory. [MCP setup and tested clients](mcp.md) records the exact tested CLI versions and outstanding Desktop checks. The MCP server never executes stored instructions or accesses external sources; the assistant performs authorized work with its own tools.

## Backup and upgrade

Before replacing the executable, stop its processes and take a [verified stopped-store backup](backup-and-recovery.md). `verify` may recover prepared work. Do not hand-edit schema versions or delete receipts to open an unsupported store. Restored HTTP servers require credential rotation before use; external artifacts and in-flight claims need reconciliation.
