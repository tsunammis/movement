# Backup, restore and recovery

Movement v1 supports one local filesystem directory on one machine. A backup is a stopped-store copy, not a live replica or sync mechanism. Task JSON, Markdown history, configuration and operation receipts are authoritative. Receipts contain original retry responses and may contain claim tokens; protect the complete copy as private data.

## Create a backup

1. Stop every process using this directory: HTTP servers, client-managed MCP subprocesses and routines that launch CLI commands. Wait for running commands to finish. Closing a browser does not stop its HTTP server. Keep all writers stopped until copying finishes.
2. Run `verify` to recover any prepared operation and validate task/history/receipt consistency. It prints task, event and receipt counts and fails visibly on corrupt or unsupported records. It does not repair corruption by discarding records.
3. Copy the entire directory into a new, private backup path. Include hidden files, `config.json`, `tasks/`, `receipts/`, `transactions/` and `runtime/`. Do not copy only the task files. Do not delete or replace a live `runtime/write.lock`.
4. Verify the copy and record its counts. Resume the original processes only after copying completes.

For example, after stopping all processes, with a **nonexistent** backup destination:

```sh
movement --data-dir /absolute/path/tasks verify
umask 077
cp -R /absolute/path/tasks /absolute/path/tasks-backup
movement --data-dir /absolute/path/tasks-backup verify
```

A `verify` result is a point-in-time check, not a lock held for the later copy. Copying while other processes resume writing is unsupported. Store backups outside live cloud-sync folders and keep at least one prior known-good copy.

## Restore

1. Keep all processes for the restored directory stopped. Copy a backup into a new directory; preserve the backup itself. Never merge a backup into a running or nonempty store.
2. Run `verify` on the restored directory. Recovery may finish a prepared transaction in the copy before validating it. If validation fails, preserve the failed copy and investigate; do not remove receipts/events to force it open.
3. Rotate the restored HTTP credential, then obtain the new token. Rotation validates the store, writes a private random token using a synced temporary file and atomic replacement, and does not modify task history or operation receipts.
4. Start the restored HTTP/MCP clients with its explicit directory. Log in with the new token. Browser sessions belong to a server process and do not survive restart; the old bearer token must not authenticate the newly started restored server.
5. Select one store for ongoing work and point routines at it. Reconcile in-flight external actions before resuming them: an email draft may have been created after the backup. Restoring a directory does not roll back Gmail, Jira, or any other external system.

With a **nonexistent** restored destination:

```sh
umask 077
cp -R /absolute/path/tasks-backup /absolute/path/tasks-restored
movement --data-dir /absolute/path/tasks-restored verify
movement --data-dir /absolute/path/tasks-restored auth rotate
movement --data-dir /absolute/path/tasks-restored auth token
movement --data-dir /absolute/path/tasks-restored serve
```

Stop HTTP servers **before** rotation. A running server holds its token and sessions in memory until it exits; `auth rotate` does not remotely terminate it. If rotation reports a write/sync error, the credential may already have changed. Keep servers stopped, rerun rotation, then read the current token; do not infer success from a partial write or reuse the previously printed token.

Claim tokens are distinct from the HTTP credential. Rotation deliberately preserves them in authoritative receipts so old operations still return the original response. Restored claims retain their original expiration; renewal, release, replacement and late reports use the normal task rules. Review history, unresolved observations and external artifacts before taking over an attempt. A backup may lack the most recent external action receipt; local retry IDs alone cannot deduplicate that external action.

## Interrupted operations and corruption

All normal Movement reads/writes recover under the shared filesystem lock. Unprepared staging is discarded; prepared transactions roll forward. `verify` performs the same recovery and then validates every task, event and receipt. It may therefore change a store with pending work. For forensic inspection, preserve a byte-for-byte copy first and run Movement against a separate working copy.

If disk space or I/O errors prevent recovery, Movement fails visibly and keeps the prepared transaction. Restore available space or filesystem access and retry against the same directory. Do not delete the journal or immutable events. After an uncertain mutation response, retry the identical operation ID and semantic payload to retrieve its original receipt.

These guarantees cover the tested process-interruption and injected-I/O cases on local macOS storage. They do not establish hardware power-loss protection, tamper-proof history, or correctness of copying live/synced directories.

## Schema versions and upgrades

The initial supported format is schema version 1. Older development scratch stores are not advertised as an upgrade source. There is no automatic destructive migration or generic downgrade command. Unsupported records must fail visibly; never hand-edit a version number to bypass validation.

Before replacing the executable, stop its processes and take a verified backup. A future format migration must name the supported source/target versions, require a backup, and provide explicit conversion and verification instructions. Keep the previous executable with its compatible backup. Before recovery or staging cleanup, Movement checks stored config/task/history versions and prepared-transaction versions, including embedded task/history records. Unsupported versions return `unsupported_schema` before authoritative files change. Tests cover mixed directories, additional future fields and an unrelated pending supported transaction; even existing-operation retries are refused until the store is opened with a compatible binary. Lock/runtime metadata may still be opened to coordinate the check.
