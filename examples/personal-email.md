# Personal task: draft an email to Kevin

Illustrative only: no Jira query, Gmail draft, email, or real task has been created. Kevin's address and the project reference are placeholders. Friday 23 October 2026 at 17:00 Europe/Paris is an example deadline, not an inferred commitment.

The user owns this task in the `actionable` flow. Claude acts on the user's behalf; this does not turn the task into a delegated waiting-for commitment.

## How to do it

1. Read the specified Jira project's status and relevant issues.
2. Check the delivery deadline and identify documented risks.
3. Summarize relevant recent comments, retaining source references.
4. Draft an email to the resolved Kevin covering progress, deadline, risks and next steps.
5. Save it as a Gmail draft for user review. Do not send it.
6. Record the draft reference and source evidence in Movement.

The task stores these instructions separately from its completion check, which asks the assistant to verify the recorded draft exists, addresses the intended recipient, contains the requested summary, and remains unsent. Missing access or unresolved inputs block execution; they are not permission to invent a project update.

## Assistant handoff

A user request or external routine starts Claude. It retrieves the context from Movement, verifies required capabilities and input references, claims the task, reads the latest context, performs the work through its own integrations, and submits a work report. Movement stores the report and exposes it in history. It does not start Claude, connect to Jira/Gmail, or send mail.

The [example task](personal-email/task.json) shows the snapshot after a synthetic evidence-backed completion proposal. Its history contains [creation](personal-email/history/00000000000000000001-task_created.md), [claim](personal-email/history/00000000000000000002-task_claimed.md), and [work report](personal-email/history/00000000000000000003-work_reported.md). It enters `seems_completed` pending confirmed completion; the work-report event releases the claim. The draft and evidence references are intentionally synthetic.

If draft creation succeeds but reporting fails, the assistant retains the draft ID. If the execution claim has expired or been replaced, it records a late observation tied to the original attempt, without changing task status or the replacement claim. A current authorized agent then reconciles that observation against the authoritative completion criteria and submits a fresh report, rather than creating another draft. If it cannot determine whether Gmail saved the draft, it reports uncertainty and checks the external system before retrying creation.
