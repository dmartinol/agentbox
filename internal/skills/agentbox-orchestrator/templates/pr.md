<!-- orchestrator-workflow: {"workflow":"<workflow-id>","issue":<issue>,"stage":"implement","worker":"<runtime>","worker_id":"<worker-id>","model":"<model-id>","commit":"<full-sha>","pr":<pr>} -->

<!--
Authoring budget: a human should be able to answer "do I need to read the
diff?" from At a glance + What changed + Evidence. Keep those under ~25 lines
and put file-level detail in the collapsible change list. Report facts; do not
sell the change.

Section owners: the implementer (or fixer, for a fix cycle) writes At a glance,
What changed, Reviewer guide, Changes, Evidence, AGENTS.md check, Risks, and
Follow-ups. The verifier fills only its row under Independent checks. The
reviewer fills only its row and Follow-ups. The orchestrator owns the
provenance line, the footer, and merge. Repository contract: AGENTS.md.
-->

Closes #<issue>

## At a glance

| Field | Value |
| --- | --- |
| Kind | feat / fix / docs / refactor / chore |
| Issue | `#<issue>` — what it delivers, slice `n/m` of `#<epic>` |
| Workflow | `workflow-id` · [agent roles](../blob/HEAD/README.md#agent-roles) |
| Surface | CLI `command` · Fleet · Setup → tab · API · skills · backend only |
| Behavior | one line: what a user can do or see now that they could not before |
| Size | `n` files · `n` commits · +`x`/−`y` |
| Risk | low / medium / high — one clause: what could regress |

## What changed

- **Before:** the observable behavior that motivated the issue.
- **After:** the observable behavior now.
- **Try it:** `exact command`, or a screenshot or GIF path.

## Reviewer guide

Read in this order. The full diff should only be needed for the last row.

| Look at | Why |
| --- | --- |
| `path:line` | the decision a reviewer must judge |
| `test path` | the behavior the tests pin down |

<details>
<summary>Full change list (n files)</summary>

- **Area** — behavior change
  - `path`: what changed and why

</details>

## Evidence

Results at the reviewed commit, not at the implementer's last local edit.

| Gate | Command | Result | Commit |
| --- | --- | --- | --- |
| Focused | `command` | pass | `sha` |
| Full | `make test` | pass | `sha` |
| Vet | `make vet` | pass | `sha` |
| Build | `make build` | pass | `sha` |

**Independent checks** — the orchestrator fills these in; `pending` until the
stage reports.

| Stage | Role | Result | Evidence |
| --- | --- | --- | --- |
| Verify | `agentbox-verifier` | gates pass at `sha` | comment link |
| Review | `agentbox-reviewer` | `approve` · cycle `n` · `n` open findings | verdict link |

## AGENTS.md check

| Rule | Result |
| --- | --- |
| Advances one of the three product jobs | ✅ which one |
| No new top-level surface, plugin, or orchestration layer | ✅ / `n/a` — why |
| Business logic in `internal/app`; `internal/gui` stays a thin boundary | ✅ / `n/a` |
| Docs updated; screenshots current | ✅ / `n/a` — files |
| Embedded `dist` rebuilt and committed | ✅ / `n/a` |
| No secret value in code, logs, tests, fixtures, or docs | ✅ |
| Tests cover the changed behavior | ✅ test names |
| Compact GUI, and the layout/accessibility checklist | ✅ / `n/a` — widths and themes checked |

## Risks

- **Risk** — when it shows up, and what the reviewer or user would see.
  Rollback: revert commit / revert plus manifest edit / none.

## Follow-ups

- `#<issue>` — deferred `minor`/`nit` finding or known gap. Write `none`.

## Provenance

This body describes the implementation stage; it may be written by that worker
or by the orchestrator on its behalf, and it does not imply a human author.
Template: `pr.md` beside the installed `SKILL.md`, or the repository's
`.github/orchestrator/pr.md` override when present.

---

**<img src="https://raw.githubusercontent.com/dmartinol/agentbox/main/docs/assets/agentbox-icon.svg" width="14" height="14" align="absmiddle" alt="AgentBox"> Implemented by AgentBox (<agent>)** · reviewed [<short-sha>](<repo-url>/commit/<full-sha>) · [#<pr>](<repo-url>/pull/<pr>) closes [#<issue>](<repo-url>/issues/<issue>) · took <duration>
