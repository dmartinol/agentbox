# Review verdict: <approve|request-changes|blocked>

<!-- orchestrator-workflow: {"workflow":"<workflow-id>","issue":<issue>,"stage":"review","worker":"<runtime>","worker_id":"<worker-id>","model":"<model-id>","commit":"<full-sha>","pr":<pr>} -->

## Verdict

One of `approve`, `request-changes`, or `blocked`, with one sentence of
reasoning.

## Findings

| Severity | Finding | Location |
| --- | --- | --- |
| blocker, major, minor, or nit | What is wrong and what evidence shows it. | Path and line at the reviewed commit. |

## Evidence at commit

- Reviewed commit: `<full-sha>`
- Gates observed: `<command>` — result.
- Worktree: fresh checkout at the pull request head, not the implementer's
  worktree.

## Required changes

Only for `request-changes`: the smallest changes that would flip the verdict.

## Provenance

The reviewer returns this artifact to the orchestrator. The orchestrator posts
it, so a review verdict always has one writer and one provenance path.

Orchestrated by agentbox-orchestrator {{VERSION}} · workflow <workflow-id> · stage review · worker <runtime>/<agent> · model <model-id> · commit <full-sha>
