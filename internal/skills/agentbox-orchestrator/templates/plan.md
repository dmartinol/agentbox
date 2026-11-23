<!-- orchestrator-workflow: {"workflow":"<workflow-id>","issue":<issue>,"stage":"plan","worker":"<runtime>","worker_id":"<worker-id>","model":"<model-id>","commit":null,"pr":null} -->

<!--
Authoring budget: the scan layer under "Plan at a glance" is five lines or
fewer — one per row. A reader must be able to decide whether to open the
collapsible plan from those five rows alone. Everything longer than that goes
inside the collapsed block.

Section owners: `agentbox-planner` writes this comment. The orchestrator posts
it and never rewrites the plan. Repository contract: AGENTS.md.
-->

## Plan at a glance

| Field | Value |
| --- | --- |
| What | one line: the change this plan delivers |
| Why | one line: the acceptance criterion or user problem it serves |
| Size | ~`n` files · ~`n` lines · `n` slices |
| Gates | `make test`, `make vet`, `make build` — the exact commands |
| Risks | one line: what could regress and how it would show up |

<details>
<summary>Full plan</summary>

## Acceptance criteria

- [ ] measurable criterion, phrased so a reviewer can decide it against evidence at a commit
- [ ] the test or observed behavior that proves it

## Implementation slices

1. Slice — files, behavior, and the test that pins it.
2. Slice — files, behavior, and the test that pins it.

## Gate list

Use the repository's Makefile targets rather than the raw tool invocation. A raw `npm` command inside the sandbox can fail with `EACCES` on the cache; `make test` and `make build` already wrap it with a writable cache.

| Gate | Command | Why it must pass |
| --- | --- | --- |
| Focused | `make test` with the package filter | catches the changed behavior |
| Full | `make test` | no regression outside the package |
| Vet | `make vet` | static checks |
| Build | `make build` | the shipped binary compiles |

Add `make race`, `make web-test`, or `make web-build` when the change touches provider, profile, policy, image, run lifecycle, attach, or GUI source. When a target does not exist, name the closest Makefile target and only then the raw command.

## Task spec for the implementer

Self-contained: the implementer never sees this conversation.

- **Issue:** `#<issue>` — one line.
- **Branch:** `orchestrator/<issue>-<slug>`; worktree: `<worktree-id>`.
- **Files it may touch:** `path`, `path`, and the tests that cover them.
- **Tests to add or update:** `TestName` — the behavior it pins.
- **Gates to run:** the gate list above, on the final commit.
- **Out of scope:** the thing this plan deliberately does not change.
- **Commit identity:** `AgentBox <agentbox@local.invalid>`; the orchestrator preserves authorship on push.

## Risks

- **Risk** — when it shows up, and what the reviewer or user would see.

## Out of scope

- The thing a reader might expect and will not get, and where it is tracked.

</details>

## Provenance

The planner writes the full artifact to the untracked `.agentbox-stage/plan-<iteration>.md` and returns only a completion note naming that path; the orchestrator reads, persists, and posts it with provenance.

---

**<img src="https://raw.githubusercontent.com/dmartinol/agentbox/main/docs/assets/agentbox-icon.svg" width="14" height="14" align="absmiddle" alt="AgentBox"> Planned by AgentBox orchestrator (<agent>)** · base [<short-sha>](<repo-url>/commit/<full-sha>) · [#<issue>](<repo-url>/issues/<issue>) · took <duration>
