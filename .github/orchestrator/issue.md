<!-- orchestrator-workflow: {"workflow":"<workflow-id>","issue":<issue>,"stage":"triage","worker":"orchestrator","worker_id":"<runtime>","model":"<model-id>","commit":null,"pr":null} -->

<!--
Authoring budget: a reader should understand this issue in 30 seconds from
At a glance + Current behavior + Acceptance criteria. Keep those under ~25
lines and put design detail in the collapsible block.

Section owners: `agentbox-triage` writes At a glance, Summary, and Current
behavior. `agentbox-planner` writes Proposed design, Acceptance criteria, Test
plan, Risks, and Out of scope, and must keep the criteria measurable.
`agentbox-implementer` and `agentbox-reviewer` tick criteria; they never
rewrite them. Repository contract: AGENTS.md.
-->

## At a glance

| Field | Value |
| --- | --- |
| Epic | `#epic` — name (`n` of `m`) |
| Depends on | `#N` · `none` |
| Blocks | `#N` · `none` |
| Roles | `agentbox-triage` → `agentbox-planner` → `agentbox-implementer` · [agent roles](README.md#agent-roles) |
| Surface | Fleet · Setup → tab · CLI `command` · API · skills · backend |
| Change | one line: the behavior, not the files |
| Risk | low / medium / high — one clause |
| Size | ~`n` files · ~`n` lines |
| Open questions | `none`, or the one decision needed before implement and its owner |

## Summary

- **Who is affected:** the persona and situation.
- **What is wrong or missing:** the gap, in one sentence.
- **Why now:** what breaks, costs, or blocks without this.

## Current behavior

| Situation | Today | Wanted |
| --- | --- | --- |
| user action or command | observed result or file reference, or `design-only: no code yet` | what should happen |

## Proposed design

One short paragraph in plain language: the smallest shape that works.

```text
before → after, data flow, or derivation (5 to 10 lines)
```

<details>
<summary>Design detail</summary>

### Area

- Decision, the module that changes, and the data that moves.

### Area

- Deliberately excluded behavior, and where it lives instead.

### Decisions

| Decision | Chosen | Rejected alternative | Why |
| --- | --- | --- | --- |
| question | choice | alternative | constraint from AGENTS.md or the architecture |

</details>

## Acceptance criteria

Group by surface; every box must be observable and must appear in the gate
list below.

### Surface

- [ ] Observable result, including the empty, loading, and error states when
  the surface has them.
- [ ] Observable result.

## Test plan

| Gate | Command | Proves |
| --- | --- | --- |
| Focused | `command` | acceptance criteria it covers |
| Full | `make test` | no regressions |
| Vet / build | `make vet`, `make build` | what they catch here |

## Risks

| Risk | Mitigation |
| --- | --- |
| failure mode or migration concern | guard, test, or explicit acceptance |

## Out of scope

- Adjacent work this issue deliberately does not include, with the issue that
  owns it when one exists.

## Related

- `#N` — why it matters here.
- `AGENTS.md` — the sections that constrain this work: Scope rules,
  Architecture boundaries, GUI expectations, Layout and accessibility
  checklist, Credential and configuration rules.
- `docs/file.md` — what must be updated with the code.

Template `.github/orchestrator/issue.md`.

Orchestrated by agentbox-orchestrator v0.2-103-gbf51d49 · workflow <workflow-id> · stage triage · worker <runtime>/<agent> · model <model-id> · commit not-applicable
