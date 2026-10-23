# Movement contracts

- `task.schema.json`: stored task JSON, schema version 1.
- `history-event.schema.json`: the machine fields in Markdown event YAML frontmatter.
- `openapi.yaml`: OpenAPI 3.1 document for the authenticated local HTTP API.

Regenerate with `go run ./cmd/contracts`. A test compares checked-in artifacts with generator output. Schemas are derived from the domain/application Go structs with explicit enum, identity and editable-field constraints. MCP input discovery uses the same schema builder. The repository validates stored task/event structure with this builder and separately checks domain rules and task/history/receipt consistency.

JSON Schema describes structure; it does not replace cross-field or transactional validation. Revision equality, lifecycle transitions, claim tokens, report aggregates, observed-time ordering, semantic context versions, byte limits, source identity and filesystem recovery are enforced by the shared service/repository. Length constraints in the service are UTF-8 byte limits; JSON Schema string lengths count characters. Unknown fields are rejected. Nullable arrays reflect currently supported Go JSON representations; application defaults normalize core task collections.

History frontmatter uses a bounded safe subset of YAML: mappings with string keys, ordinary sequences/scalars, no duplicate keys, aliases, anchors, custom tags or multiple documents. Timestamps are quoted strings. The Markdown body must equal the recorded summary after trimming surrounding whitespace. Both the event ID and its sequence are validated; filenames use a zero-padded 20-digit sequence and a known event type, e.g. `00000000000000000001-task_created.md`.

The fixtures in `examples/` are generated using the production service and validated through the production reader. Their external evidence remains synthetic. OpenAPI generation tests cover every implemented route/operation and local schema references; end-to-end behavior is tracked in `docs/acceptance.md`.
