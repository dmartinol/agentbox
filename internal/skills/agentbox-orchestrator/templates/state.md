<!-- orchestrator-workflow: {"workflow":"<workflow-id>","issue":<issue>,"stage":"state","worker":"orchestrator","worker_id":"<runtime>","model":"<model-id>","commit":"<full-sha>","pr":<pr>} -->

<!-- orchestrator-state -->

```yaml
schema: 2
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
stages:
  - stage: plan
    runtime: <runtime>
    agent: <agent-id>
    model: <model-id>
    effort: <effort-level>
    worker_id: <id>
    worktree: <id>
    commit: <full-sha>
    pr: null
  - stage: implement
    runtime: <runtime>
    agent: <agent-id>
    model: <model-id>
    effort: <effort-level>
    worker_id: <id>
    worktree: <id>
    branch: orchestrator/<issue>-<slug>
    commit: <full-sha>
    pr: null
  - stage: verify
    runtime: <runtime>
    agent: <agent-id>
    model: <model-id>
    effort: <effort-level>
    worker_id: <id>
    worktree: <id>
    commit: <full-sha>
    pr: <number>
  - stage: review
    runtime: <runtime>
    agent: <agent-id>
    model: <model-id>
    effort: <effort-level>
    worker_id: <id>
    worktree: <id>
    commit: <full-sha>
    pr: <number>
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

Orchestrated by agentbox-orchestrator {{VERSION}} · workflow <workflow-id> · stage state · worker orchestrator · model <model-id> · commit <full-sha>
