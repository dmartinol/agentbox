# Review verdict: <approve|approve-with-follow-ups|request-changes|blocked>

<!-- orchestrator-workflow: {"workflow":"<workflow-id>","issue":<issue>,"stage":"review","worker":"<runtime>","worker_id":"<worker-id>","model":"<model-id>","commit":"<full-sha>","pr":<pr>} -->

## Verdict

One of `approve`, `approve-with-follow-ups`, `request-changes`, or `blocked`, with one sentence of reasoning.

## Severity bar

Only `blocker` and `major` block merge. `minor` and `nit` never block merge and are always follow-ups.

| Severity | Definition | Action |
| --- | --- | --- |
| `blocker` | Fails an acceptance criterion, loses or corrupts data, exposes a secret, weakens the sandbox or policy, leaves a gate or CI red, or reports false provenance. | Blocks merge. |
| `major` | A reproducible defect in the behavior this pull request changes. | Blocks merge. |
| `minor` | A real issue with no merge risk. | Follow-up only; never blocks. |
| `nit` | Style or preference. | Follow-up only; never blocks. |

## Review scope

- Cycle 1: the full diff against the base.
- Cycle 2 and later: `prev_head..head`, the ledger status of every prior finding, and the gates. Do not re-derive the touched subsystem or review the full diff again. A new finding outside the reviewed range is allowed only at `blocker` severity.

## Independence

State the basis on the face of the verdict, not only in the footer:

- `different agent or model` — name the implementer and the reviewer selections.
- `same agent and model as the implementer` — a recorded limitation. Say so plainly; session and worktree isolation do not make the review independent.

## Findings

| Id | Severity | Finding | Location | Status |
| --- | --- | --- | --- | --- |
| `F<n>`, matching `findings[].id` in the state record | blocker, major, minor, or nit | What is wrong and what evidence shows it. | Path and line at the reviewed commit. | open, fixed, `deferred #<issue>`, or wontfix |

## Evidence at commit

- Reviewed commit: `<full-sha>`
- Reviewed range: `base..<full-sha>` for cycle 1, `prev_head..<full-sha>` for later cycles.
- Head check: `git diff <reviewed-sha>..HEAD --stat` is empty.
- Gates observed: `<command>` — result.
- Worktree: fresh checkout at the pull request head, not the implementer's worktree.
- Real-git probe: the observed result per state, when the change derives behavior from git state.

## Required changes

Only for `request-changes`: the smallest changes that remove the merge risk from each open `blocker` or `major` finding. Never list a `minor` or `nit` finding here.

## Follow-ups

Mandatory for `approve-with-follow-ups`; otherwise `none`. List one group per deferred finding or finding group and the created issue that tracks it:

| Findings | Issue |
| --- | --- |
| `F<n>` | #<issue> |

## Provenance

The `.agentbox-stage/review-<iteration>.md` handoff is the verdict artifact; the orchestrator reads, persists, and posts it with provenance. A review verdict always has one writer and one provenance path.

---

**<img src="https://raw.githubusercontent.com/dmartinol/agentbox/main/docs/assets/agentbox-icon.svg" width="14" height="14" align="absmiddle" alt="AgentBox"> Reviewed [<short-sha>](<repo-url>/commit/<full-sha>) by AgentBox reviewer (<agent>)** · <verdict> · <n> findings · independence: <basis> · [#<pr>](<repo-url>/pull/<pr>) · took <duration>
