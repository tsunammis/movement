# Intent, specification, and plan workflow

## Basis

Anthropic's [AI-native SDLC playbook](https://claude.com/resources/articles/the-ai-native-sdlc-playbook), consulted 2026-10-19, uses versioned intent, specification, and plan artifacts with human acceptance between stages. Plans identify files, order of work, risks, and proof. Movement adopts that artifact chain; the directory layout and templates here are project choices, not an official Anthropic scaffold.

## Working agreement

Each change has `intent/<number>-<slug>/intent.md`, `spec.md`, and `plan.md`.

| Artifact | Purpose | Ready for acceptance when |
| --- | --- | --- |
| Intent | Why, who, desired outcomes, constraints, exclusions | The user recognizes the problem and scope; uncertainties are explicit |
| Spec | Observable behavior, data contracts, edge cases, acceptance criteria | Requirements are testable; material concerns are resolved or deferred explicitly |
| Plan | Files, ordered steps, dependencies, risks, verification | Each slice can be implemented and checked against the spec |

Use `draft`, `accepted`, or `superseded` status. Record the accepting person, date, and decision reference when acceptance occurs. The user is the initial product and engineering decision owner.

For this bootstrap, all three artifacts are drafted together to make the proposal reviewable. This does not mean downstream artifacts are approved. Implementation starts only after the user accepts the intended scope and plan. Document edits do not require repeated approval.

When an upstream requirement changes, review downstream artifacts and mark affected approvals for renewed review. When implementation departs from an accepted plan, update the plan with the reason. Keep artifacts in version control when the user requests commits; do not create commits solely because the playbook describes them.

## Review prompts

- Intent: Is this the right problem? What is deliberately excluded?
- Spec: Can a person and an agent predict the same outcome? What happens on retries, conflicts, incomplete context, and failure?
- Plan: What is the smallest useful slice? Which assumptions need a spike? What proves it works?

Reusable starting points live in [templates](../templates/). Requirements and acceptance scenarios in the initial spec provide the first traceability map. Later, record verification evidence alongside the plan. Git history can measure artifact lead time and rework once commits exist; no automation is installed yet.
