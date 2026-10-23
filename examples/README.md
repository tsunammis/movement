# Examples

These synthetic bundles are generated through the production application service and text repository. UUIDs, input digests, matching history and retry receipts are real generated fixture values; external source and artifact references remain synthetic. No sources were fetched and no messages or drafts were created.

- [Basic task JSON](basic-task/task.json) with [Markdown creation history](basic-task/history/00000000000000000001-task_created.md).
- [Waiting-for walkthrough](waiting-for.md), [task JSON](waiting-for-task/task.json), and [creation history](waiting-for-task/history/00000000000000000001-task_created.md).
- [Personal email walkthrough](personal-email.md), [task JSON](personal-email/task.json), and [work report](personal-email/history/00000000000000000003-work_reported.md).

The [data model](../docs/data-model.md) describes JSON state, YAML frontmatter metadata, Markdown narratives, and their consistency requirements. Each bundle includes a create payload in `definition.json`, current `task.json`, `history/`, and `receipts/`. The schema and repository tests validate them and verify creation retries return the original receipt. They omit configuration and runtime credentials and are not directories to use directly as live stores. The synthetic claim receipt has no authority outside its discarded temporary generator store.

Regenerate deliberately with `go run ./cmd/examples`. This changes generated IDs/tokens while preserving the checked-in definition payloads. No generator runs automatically during build or tests.
