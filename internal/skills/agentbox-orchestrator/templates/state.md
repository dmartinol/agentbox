<!-- orchestrator-workflow: {"workflow":"<workflow-id>","issue":<issue>,"stage":"state","worker":"orchestrator","worker_id":"<runtime>","model":"<model-id>","commit":"<full-sha>","pr":<pr>} -->

<!-- orchestrator-state
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
  decision: <ready|ready-simple|needs-info|out-of-scope|blocked>
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
    dispatch: <split|combined|skipped-simple>
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
-->

**<img src="https://raw.githubusercontent.com/dmartinol/agentbox/main/docs/assets/agentbox-icon.svg" width="14" height="14" align="absmiddle" alt="AgentBox"> Orchestrator state: <state>** · AgentBox orchestrator {{VERSION_DISPLAY}}
