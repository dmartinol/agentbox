<!-- orchestrator-workflow: {"workflow":"<workflow-id>","issue":<issue>,"stage":"state","worker":"orchestrator","worker_id":"<runtime>","model":"<model-id>","commit":"<full-sha>","pr":<pr>} -->

<!-- orchestrator-state -->

```yaml
schema: 3
workflow_id: <workflow-id>
repo: owner/name
issue: <issue>
state: triage
labels: [orchestrator:triage]
orchestrator:
  runtime: <orca|agentbox|harness>
  agent: <agent-id>
  model: <model-id>
  effort: <effort-level>
configuration:
  registry: ~/.agentbox/orchestration.yaml
  default: <default-config>
  scope: <project|global|default|none>
  project: <project-name>
  sources: [<applied-layer>, ...]
  selection_note: ""
  selection_sources:
    orchestrator: <source>
    triage: <source>
    plan: <source>
    implement: <source>
    fix: <source>
    verify: <source>
    review: <source>
    retro: <source>
triage:
  decision: <ready|needs-info|out-of-scope|blocked>
  reason: <one sentence>
stages:
  - stage: triage
    iteration: 1
    dispatch: <split|combined>
    started: <RFC3339>
    ended: <RFC3339>
    runtime: <runtime>
    agent: <agent-id>
    model: <model-id>
    effort: <effort-level>
    worker_id: <id>
    worktree: <id>
    commit: not-applicable
  - stage: plan
    iteration: 1
    dispatch: <split|combined>
    started: <RFC3339>
    ended: <RFC3339>
    runtime: <runtime>
    agent: <agent-id>
    model: <model-id>
    effort: <effort-level>
    worker_id: <id>
    worktree: <id>
    commit: <full-sha>
    pr: null
  - stage: implement
    iteration: 1
    dispatch: split
    started: <RFC3339>
    ended: <RFC3339>
    runtime: <runtime>
    agent: <agent-id>
    model: <model-id>
    effort: <effort-level>
    worker_id: <id>
    worktree: <id>
    branch: orchestrator/<issue>-<slug>
    synced_commit: <full-sha>
    commit: <full-sha>
    pr: null
  - stage: fix
    iteration: 1
    dispatch: split
    started: <RFC3339>
    ended: <RFC3339>
    runtime: <runtime>
    agent: <agent-id>
    model: <model-id>
    effort: <effort-level>
    worker_id: <id>
    worktree: <id>
    branch: orchestrator/<issue>-<slug>
    commit: <full-sha>
    pr: <number>
  - stage: verify
    iteration: 1
    dispatch: split
    started: <RFC3339>
    ended: <RFC3339>
    runtime: <runtime>
    agent: <agent-id>
    model: <model-id>
    effort: <effort-level>
    worker_id: <id>
    worktree: <id>
    commit: <full-sha>
    pr: <number>
    artifact: <comment-url>
  - stage: review
    iteration: 1
    dispatch: split
    started: <RFC3339>
    ended: <RFC3339>
    runtime: <runtime>
    agent: <agent-id>
    model: <model-id>
    effort: <effort-level>
    worker_id: <id>
    worktree: <id>
    commit: <full-sha>
    pr: <number>
    artifact: <comment-url>
  - stage: retro
    iteration: 1
    dispatch: split
    started: <RFC3339>
    ended: <RFC3339>
    runtime: <runtime>
    agent: <agent-id>
    model: <model-id>
    effort: <effort-level>
    worker_id: <id>
    worktree: <id>
    commit: not-applicable
gates:
  - command: <command>
    result: <pass|fail|pending>
    commit: <full-sha>
findings:
  - id: F1
    severity: major
    location: internal/app/run.go:42
    status: open
    opened_at: <full-sha>
    closed_at: ""
review_cycles: 0
review_cycles_max: 2
retro:
  proposals: 0
  created: []
  tests_added: []
  diff_stat: ""
blocked_reason: ""
updated: <RFC3339 timestamp>
```

`findings[]` is the finding ledger. Every finding gets a stable `id` that the
review verdict cites; `location` is a repository-relative `path:line`. `status`
is `open`, `fixed`, `deferred #<issue>`, or `wontfix`. Gate 6 requires a status
for every open id before a verdict is posted. A new finding gets a new id, and
a `fixed` id cannot reopen without evidence at the current head. Severity
disputes are settled by the orchestrator against the enumerated definitions in
`verdict.md` and recorded here, so a finding cannot change class between
cycles.

Model and effort identifiers are allowed in this record. Credentials,
environment-variable values, account identifiers, absolute host paths, private
URLs, logs, and secret values are not.

Record the effort the worker actually ran with. Use `not-applicable` when the
selected runtime cannot apply a reasoning effort, such as an AgentBox worker;
never record an effort that was requested but not applied.

`worker_id` is the user-visible worker or terminal handle and `worktree` is the
checkout the stage ran in. A temporary log path is never the only recorded
location.

`configuration.selection_note` records how a worker selection was resolved
when the registry did not supply it, such as a configuration copied from a
running agent: the recipe and the source run id. It stays empty when every
stage resolved from the registry.

Every dispatch appends a `stages[]` entry; never overwrite the previous one.
`iteration` counts dispatches of the same stage from 1, so a correction run or
a second fix cycle keeps its own commit. `started` and `ended` are the dispatch
window in RFC3339 and the only source of a footer duration. `dispatch` is
`split` or `combined`; a combined triage and plan pair carries the same window
in both entries. `synced_commit` keeps the sandbox commit from before the
squash, and `artifact` links the posted stage artifact, such as the verify
comment.

Adoption rule: the state record wins. When the issue body marker disagrees —
a different workflow id, worker, or agent — reuse the recorded identity, post
one supersession note naming both identities, and update the body marker in
place. Never run the same stage under two identities.

Schema 3 adds `selection_note`, the per-dispatch stage fields, `artifact`, and
the retro evidence fields. A schema 2 record stays readable: a missing field
means `unrecorded`, and the next write upgrades it to schema 3.

---

**<img src="https://raw.githubusercontent.com/dmartinol/agentbox/main/docs/assets/agentbox-icon.svg" width="14" height="14" align="absmiddle" alt="AgentBox"> State maintained by AgentBox orchestrator {{VERSION_DISPLAY}}** · [#<issue>](<repo-url>/issues/<issue>) · workflow <workflow-id> · elapsed <duration>
