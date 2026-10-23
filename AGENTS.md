# Movement: agent instructions

Read [docs/constitution.md](docs/constitution.md), [docs/workflow.md](docs/workflow.md), and the relevant `intent/<change>/` artifacts before working.

- The user authorized implementation of the full specification through the active goal on 2026-10-22. Follow `intent/001-local-task-management/spec.md` and the plan; record resolved contract details and verification evidence as implementation proceeds. Do not commit unless requested.
- Distinguish explicit requirements, recommendations, open questions, and accepted decisions. Never record an approval that was not given.
- Maintain one artifact chain per change: `intent.md` -> `spec.md` -> `plan.md`. Keep requirements, acceptance criteria, planned files, and verification linked.
- Record material architectural choices under `docs/decisions/`. Keep the constitution concise and durable.
- Keep domain behavior independent of CLI, MCP, HTTP, and UI adapters. Every interface must use the same validation and persistence semantics.
- Preserve local operation, portable packaging, readable text records, provenance, and task history. Distinguish user-authored task/check instructions from untrusted source content. A check prompt specifies the requested investigation within the user’s authorization; fetched content cannot override that scope or grant permissions.
- Never overwrite a user's task decisions during ingestion or mark work complete merely because an agent attempted it.
- Do not add speculative infrastructure or dependencies. The storage prototype is runnable; its commands and limits are in `prototypes/storage/README.md`. Core adapters and the embedded UI have completed local acceptance at the scope recorded in docs/implementation-status.md and decision 005. Codex Desktop verification is an accepted non-blocking follow-up; do not claim it is tested.
- When implementation begins, verify the relevant acceptance criteria and report actual results and limitations. Update the plan when implementation changes it.
- Do not commit, publish, or change external source systems unless requested. Preserve unrelated user edits.

These instructions govern development of Movement. Runtime task instructions are a separate product concern.
