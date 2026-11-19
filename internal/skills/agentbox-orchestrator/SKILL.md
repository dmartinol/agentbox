---
name: agentbox-orchestrator
description: "Drive one tracker issue end to end: dispatch tracker-blind triage, plan, implement, fix, verify, and review to isolated workers, prepare repository worker environments, run a post-terminal retro, record agent, model, and effort, enforce gates, post provenance-marked PRs and verdicts, and own merge and close. Use when asked to take an issue to a merged pull request with the orchestrator flow."
version: "{{VERSION}}"
---

# Issue-to-PR orchestrator flow

This skill drives exactly one tracker issue from triage to a merged pull
request and a closed issue. The core flow is tracker-agnostic; GitHub is the
first adapter. It is instructions only: the orchestrator follows this
document, calls the tracker adapter, and dispatches workers. It never adds
code or tooling on its own.

## Sample prompts

Requests like these select this skill:

- "Take issue 35 to a merged pull request with the orchestrator flow."
- "Run the orchestrator on issue 42."
- "Resume the orchestrator run on issue 42."
- "Let's create the orchestration config for this project." — this is the
  configuration-authoring mode (see below), not a workflow.

## Role boundary (MUST)

The orchestrator owns exactly two things:

- **Task orchestration (mandatory):** dispatch triage and every stage's
  worker, validate every returned artifact, record provenance, enforce gates,
  deadlines, and fix-cycle limits, dispatch the post-terminal retro, and own
  merge and close.
- **Git and forge management (optional, capability-driven):** only the writes
  a selected worker cannot perform itself — sync, push, pull request
  creation, comment and label posting, and the state comment. If every
  selected worker type can do these, the orchestrator performs no git
  management at all.

The orchestrator remains the parent controller. `agentbox skills install` also
writes an `agentbox-orchestrator` role that loads this skill as its source of
truth, so a user can explicitly assign the flow to a harness agent. That role
has the same boundary and never dispatches a second orchestrator. The
orchestrator MUST NOT triage, plan, implement, fix, verify, review, or run
retro inline. If it starts doing stage work, it stops and dispatches a worker.
Assembling a context packet for a worker, reading a returned artifact, and
checking tracker or forge state are orchestration, not stage work. A resumed
run redispatches the stage; it never continues a previous worker's session.

## Configuration authoring (MUST)

A prompt that asks to create, change, or inspect the orchestration
configuration — for example “let's create the orchestration config for this
project” — is a configuration-authoring request, not a workflow. Do not start
triage, do not dispatch a worker, and do not treat an existing registry as
permission to skip the interview.

The registry file itself is always the user-owned file under
`$AGENTBOX_HOME`. “Project scope” means a `projects[]` entry in that file, not
a file inside the repository and not a value in the global layer.

Authoring procedure:

1. Confirm the prerequisite. Run `agentbox orchestrator config path`. If the
   command is unknown, stop and ask the user to install or build the current
   AgentBox binary. Do not hand-edit the registry without `path`, `show`, and
   `validate`; without them the saved scope cannot be verified.
2. Resolve the current registry for the repository with the command in the
   registry section. Use the result only as context and as proposed defaults
   labelled with their source. It does not replace the interview.
3. Interview before writing. Ask one compact group of questions that covers
   every required setting: orchestrator runtime, agent, model, and effort;
   worker runtime, agent, profile (when the runtime uses one), model, and
   effort; any `coding` or `other` group difference; any per-stage override;
   and the save scope. State the allowed runtime values (`orca`, `agentbox`,
   `harness`) and agent values (`claude`, `codex`, `opencode`) rather than
   asking an open-ended question. Confirm the scope, asking only when the
   prompt does not name one. Ask for each setting, or present a table of
   proposed values with their sources and ask the user to confirm or change
   each one. Do not replace the interview with a single “use these values?”
   question. Offer AgentBox profile models discovered with
   `agentbox profile list --json` and state where each value came from. Allow
   `none` where a runtime cannot apply a setting, such as effort on an
   AgentBox worker. Never invent a model, profile, runtime, effort, or
   credential value.
4. Honour the scope named in the prompt. “For this project”, “project config”,
   or a named repository means project scope. A project request MUST write a
   `projects[]` entry for the current repository, with the answers as inline
   `orchestration` and `workers` overrides, or a named config referenced only
   by that entry. It MUST NOT write the answers into `configs[default]`,
   `global`, or a named config shared with other projects. Only an explicit
   global request may do that. If the working directory has no Git root or
   origin, ask which repository the entry is for and use its absolute `root`.
   Write every confirmed answer into the project entry, including an answer
   that happens to match an inherited default, so the saved entry records the
   user's choice instead of depending on the shared layer.
5. Show the exact YAML or diff, the registry path, and the effective
   selections. Wait for explicit confirmation, then write it.
6. Validate and re-resolve with the same `--project` and `--remote` values.
   For a project request, require `"scope": "project"` and
   `"project": "<name>"`, and check that every answered selection reports a
   `project:<name>` source. If a value still resolves from `default` or
   `global`, the save is incomplete: correct the entry and re-check. Do not
   report success until the scope and sources match the request.
7. Report the registry path, scope, project name, and a compact table of the
   saved selections. A later workflow still shows the resolved configuration
   and asks for approval unless the prompt says “use the existing
   configuration”.

## Launch topology and visibility (MUST)

Three different objects must never be conflated:

- the **controller**: the agent session that runs this skill and the
  `agentbox-orchestrator` role instructions;
- the **worktree**: an Orca-managed checkout of a branch at a path; creating
  one starts no agent and runs no prompt;
- the **worker**: the agent process launched inside a worktree to run one
  stage.

Creating a worktree is preparation, not dispatch. The stage worker is the
agent process: send it the task spec as its own prompt, `--spec-file`, or
terminal input. Do not start an agent and then have that agent launch a second
non-interactive agent process to do the same stage. That nested launch adds a
hop, hides the real worker, and makes its output unobservable. A controller
that must exist only because the host harness starts custom roles as
subagents never duplicates the stage and never performs stage work.

### Supported controller launch paths

| Harness | Start the controller | Topology |
| --- | --- | --- |
| Claude Code | `claude --agent agentbox-orchestrator` | one process; no launcher |
| OpenCode | `opencode run --agent agentbox-orchestrator`, or select the primary agent in a session | one process; no launcher |
| Codex | ask the top-level session to use the `agentbox-orchestrator` custom agent | top-level parent plus controller subagent; Codex has no top-level `--agent` switch |
| Orca | `--agent` selects only the host runtime; assign the role through the started harness | runtime plus harness role assignment |

Rules:

- Start the assigned controller directly when the harness supports it. Do not
  add a launcher session whose only job is to spawn the controller and relay
  its answer.
- When Codex requires a top-level parent to start the custom controller
  subagent, keep that parent lightweight and visible. Run it in a visible
  terminal, give it the issue and the role instruction, and let the user
  follow the parent and the controller separately. Never run that parent as a
  hidden non-interactive process whose output exists only in a temporary file.
- From Orca, never pass `agentbox-orchestrator` as the `--agent` runtime ID.
  `orca orchestration worker-start --agent <id>` and `orca agent launch
  --agent <id>` accept only a host runtime such as `codex`, `claude`, or
  `opencode`; assign the custom role afterwards through that harness.
- A harness that cannot assign the role from an Orca-started session is a
  documented limitation. Start that harness directly, or record the
  limitation and set `blocked`; never add an Orca-specific role flag or imply
  that a runtime ID selected the role.

### Runtime, profile, and model constraints

Record the tested launch path in the state record and report it to the user.
A profile is a harness-level setting, not an Orca runtime ID.

- `orca orchestration worker-start` and `orca agent launch` accept `--model`
  and `--effort` but have no `--profile` flag. They cannot apply a Codex
  profile.
- `orca worktree create --agent ... --model ... --effort ...` creates the
  checkout and starts the agent, but it also cannot set a harness profile.
- When the user requires a profile, create the worktree first, then start the
  stage agent in a visible Orca terminal whose command sets the harness
  profile and model, for example `orca terminal create --worktree <selector>
  --title "<stage>" --command "codex -p PROFILE -m MODEL" --json`. Record the
  returned terminal handle, follow it with `orca terminal read`, and drive the
  stage through that terminal.
- Never drop a requested profile, model, effort, or runtime to make a worker
  start. If no supported path can honour the selection under the runtime's
  normal sandbox and approval controls, set `blocked` with the exact
  unsupported selection and the single action that unblocks the flow.

### Worker visibility, retention, and cleanup

When Orca is selected for workers, the default worker path is an Orca-visible
supervised worker or terminal, not a headless `codex exec` whose only output
is redirected to a temporary file.

- Start each Orca stage with `orca orchestration worker-start ... --json` and
  record the returned worker handle, worktree, and branch. Start a
  profile-bound harness in a visible `orca terminal create` session and record
  its terminal handle. `worker-start` is the default because it is supervised
  and `worker-read` works even when the runtime opens a chat instead of a
  terminal.
- Report a stage-start update with the stage, worktree, branch, and worker or
  terminal handle so the user can follow the run. Report completion or a
  blocker with the same handle and the evidence location. The user must be
  able to inspect the worker with `orca orchestration worker-show`,
  `orca orchestration worker-read`, `orca terminal read`, or
  `orca worktree show`.
- If a non-interactive command is unavoidable, run it in a visible terminal
  or teeing process and keep a durable log outside `/tmp`, for example under
  `~/.agentbox/orchestrator/<workflow_id>/<stage>.log`. Report the path and
  the handle; never make a temporary log the only way to follow the worker.
- Retain every worker session and worktree until the stage evidence is
  captured and the final report is delivered. Honour an explicit request to
  keep them. Otherwise clean up only after the final report, and never delete
  a branch whose commit SHA is not recorded.

### Worktree creation verification

Creating a worktree is a mutation with an ambiguous failure mode. Verify the
raw result before recording or retrying:

- Read the worktree path from the raw create response, not from a display or
  projection layer that may rename or omit the field. Confirm the path exists
  and that `orca worktree list` or `git worktree list` contains it before
  recording the handle.
- If the response is missing a path or cannot be confirmed, do not issue a
  second create. Re-read the original raw response or list the worktrees
  first; a missing field is ambiguous, not a failure.
- Record the worktree identity and branch only after confirmation. If it still
  cannot be confirmed, set `blocked` with the exact response and the next
  inspection step.
- Never retry a create with a new name or stage suffix until the first outcome
  is resolved. One stage gets one worktree.

## Ground rules

- One issue, one writer, one branch at a time. Never run two writers on one
  issue.
- Ask for runtime and model selections once, record them, and never silently
  substitute another value.
- The orchestrator is the only actor that merges and closes, and the only
  actor that posts review verdicts, for every worker type.
- Never bypass sandbox or approval controls. Never pass
  `--dangerously-bypass-approvals-and-sandbox`, `--yolo`, or an equivalent to
  make a stage run; report a blocker when the workflow cannot proceed under
  the runtime's normal controls.
- All environment operations go through the `agentbox` CLI; never drive
  `openshell` or `podman` directly. If the CLI is missing or a documented
  command is unknown, ask the user to install the current binary
  (`go install github.com/dmartinol/agentbox/cmd/agentbox@main`) instead of
  improvising.
- One stage is one agent process. A worker performs the stage itself and never
  launches another agent or a nested non-interactive agent for the same work.
- Never bypass branch protection, required checks, or review requirements.
- On any doubt, set `blocked` with a reason instead of improvising.

## Runtime and model selection

This table governs a workflow. A prompt that asks to create or change the
registry uses **Configuration authoring** instead, which interviews every
field before any workflow starts. Otherwise, resolve the registry first (see
the next section). Ask once during triage for anything the registry and the
prompt leave open. Record every answer in the state record and keep the
answers for the life of the issue. Confirm an inherited default explicitly
before recording it; a default is never applied silently.

| Selection | Asked for | Default |
| --- | --- | --- |
| Orchestrator | runtime (`orca`, `agentbox`, or `harness`), agent, model, effort | ask; inherited runtime defaults are confirmed |
| Triage worker | runtime, agent, model, effort | inherits the orchestrator's choice after confirmation |
| Plan worker | runtime, agent, model, effort | inherits the orchestrator's choice after confirmation |
| Implement worker | runtime, agent, model, effort | inherits the plan selection after confirmation |
| Fix worker | runtime, agent, model, effort | inherits the implement selection after confirmation |
| Verify worker | runtime, agent, model, effort | a different agent or model from the implementer when one is available |
| Review worker | runtime, agent, model, effort | a different agent or model from the implementer when one is available |
| Retro worker | runtime, agent, model, effort | inherits the orchestrator's choice after confirmation |

Rules:

- The orchestrator cannot switch its own model from inside the turn. Record
  the model the session is running with, and ask the user to relaunch if a
  different orchestrator model is required.
- A selection that cannot be honoured — an unknown model, an effort the
  runtime does not support, or a worker type that is unavailable — sets
  `blocked` with the exact reason. Never fall back to a default, a cheaper
  model, or a different agent.
- Reviewer independence and diversity: use a different agent or model from the
  implementer for the *first* review pass only. Consecutive fix cycles keep the
  same reviewer selection and rely on session and worktree isolation for
  independence; rotating models every cycle maximizes judgment variance instead
  of converging the review and is explicitly discouraged. When no different
  agent or model is available, record the limitation in the state record
  instead of claiming independence.
- Changing a selection after work has started is a new explicit instruction,
  recorded as a new stage entry. It does not rewrite prior provenance.

Reasoning effort is per runtime. An Orca worker accepts `--effort`. An
AgentBox worker cannot apply one — `agentbox agent start` has no effort
control — so a stage dispatched to AgentBox records `effort: not-applicable`
after the user confirms that limitation during triage. If the user asks for a
specific effort for an AgentBox worker, set `blocked` with the exact reason.
Never record an effort that was not applied.

## Registry-backed orchestration configuration

Runtime, worker, profile, model, and effort selections can come from a
user-owned registry at `$AGENTBOX_HOME/orchestration.yaml` (default
`~/.agentbox/orchestration.yaml`). The registry is never committed to a
project repository and never contains credentials; refer to credential
environment variables by name only. Resolve it with the AgentBox CLI so the
selection is deterministic:

```sh
agentbox orchestrator config path
agentbox orchestrator config show --project "$(git rev-parse --show-toplevel)" \
  --remote "$(git remote get-url origin 2>/dev/null)" --json
```

If `agentbox orchestrator` is not a known command, the installed binary
predates this registry. Stop and ask the user to install or build the current
AgentBox binary; do not guess the path or edit the file without `show` and
`validate`.

The registry is versioned YAML:

```yaml
version: 1
default: standard

configs:
  standard:
    orchestration: {runtime: orca, agent: codex, model: <model-id>, effort: high}
    workers:
      default: {runtime: orca, agent: codex, model: <model-id>, effort: medium}
      groups:
        coding: {effort: high}
      stages:
        review: {agent: claude, model: <model-id>, effort: high}
  heavy:
    workers:
      groups:
        coding: {model: <model-id>, effort: high}

global:
  config: standard
  workers:
    groups:
      coding: {profile: enmaas}

projects:
  - name: agentbox
    root: /absolute/path/to/agentbox
    remote: github.com/owner/agentbox
    config: heavy
    workers:
      stages:
        implement: {effort: high}
```

- `default` names the configuration used when no global or project layer
  applies. It is required and must exist in `configs`.
- `orchestration` is the controller selection. `workers.default`,
  `workers.groups`, and `workers.stages` select stage workers.
- The fixed stage groups are `coding` for `plan`, `implement`, and `fix`, and
  `other` for `triage`, `verify`, `review`, and `retro`. Every stage belongs
  to exactly one group.
- A selection is `runtime` (`orca`, `agentbox`, or `harness`), `agent`
  (`claude`, `codex`, or `opencode`), and optional `profile`, `model`, and
  `effort`. `profile` is an AgentBox profile for runtime `agentbox` and a
  harness profile for `orca` or `harness`.
- Project entries match the canonical absolute `root` (the deepest matching
  root wins) or the normalized `remote` (`github.com/owner/repo`). If two
  entries match at the same specificity, or the root and remote match
  different entries, stop and ask; the CLI reports the ambiguity and never
  picks one.
- Prefer `remote` for a shared project entry: it matches every worktree of the
  repository, including an Orca-managed worktree outside the main checkout.
  Add `root` for a checkout-specific entry or when the repository has no
  remote.
- The schema is strict. Version 1 rejects unknown fields, stages, groups,
  runtimes, agents, and config references. Run
  `agentbox orchestrator config validate` after editing.
- The orchestrator is the running controller session. If the resolved
  orchestrator runtime or model differs from that session, report the
  difference and ask the user to relaunch; record only what actually runs.

Registry resolution order (later wins, field by field):

1. `configs[default]`;
2. `global.config`, when set;
3. inline `global`;
4. the matching project's `config`, when set;
5. the matching project's inline overrides.

Within one layer, a stage resolves `workers.default`, then its group, then
`workers.stages.<stage>`. The resolved output names the source of every field,
for example `global.group:coding` or `project:agentbox.stage:implement`. A
non-empty later value overrides an earlier one; `model: ""` clears an inherited
model, and `profile: ""` or `effort: ""` clears an inherited profile or effort.

### Saving a project-scoped configuration

A configuration authored for one repository belongs in that repository's
`projects[]` entry, not in `configs[default]` or `global`. A complete
project-only registry can keep the default empty, so no other project inherits
the answers:

```yaml
version: 1
default: base

configs:
  base: {}

projects:
  - name: agentbox
    remote: github.com/owner/agentbox
    orchestration:
      runtime: harness
      agent: codex
      model: <orchestrator-model-id>
      effort: high
    workers:
      default:
        runtime: orca
        agent: codex
        profile: enmaas
        model: <worker-model-id>
        effort: high
      groups:
        coding: {model: <coding-model-id>}
      stages:
        review: {agent: claude, model: <reviewer-model-id>, effort: high}
```

Name a new project entry from the repository basename and keep it unique. Set
`remote` whenever the repository has an `origin`; add `root` only for the
checkout-specific case above. A named config may hold the answers only when it
is referenced by this single project entry, and the user must ask for a shared
named config explicitly. Never put a project request into `global`, into
`configs[default]`, or into a named config another project or `global.config`
already references.

Workflow precedence, highest first:

1. explicit choices in the current prompt;
2. the matching registry project layer;
3. `.github/orchestrator.yaml` committed by the project;
4. the registry global layer;
5. the registry default configuration;
6. repository documentation and CI;
7. the defaults in this skill.

Prompt choices override the registry for that workflow only. If the prompt
explicitly says **“use the existing configuration”**, the resolved registry
configuration is approved and no second approval question is needed.

During triage:

1. If the prompt asks to create, change, or inspect the registry rather than
   run an issue through the workflow, follow **Configuration authoring** and
   stop here. The workflow does not begin until the user asks for it.
2. Resolve the registry for the repository root and remote. Show the selected
   scope and project, the default configuration name, the applied layers, the
   orchestrator selection, every stage's runtime/agent/profile/model/effort,
   and the source of inherited values.
3. Show prerequisites: the runtime CLI and agent on `PATH`, the AgentBox or
   harness profile, and the credential environment-variable names, never their
   values. For an AgentBox profile, read its model with
   `agentbox profile show --json` and treat a model that disagrees with the
   registry as a blocker. Orca exposes no local model catalog: ask for the
   exact provider model ID and treat a launch failure as a blocker. Never
   invent or silently substitute a runtime, agent, profile, or model.
4. If the prompt does not say “use the existing configuration”, always ask for
   approval before dispatch, even when the configuration resolves every
   required choice. An existing file, a previous workflow, or a set of
   inherited defaults is not approval.
5. If no configuration applies or required choices are missing, ask for the
   missing orchestration runtime, worker selections, profile, and model/effort
   values, and for the save scope when the prompt does not name it. Honour a
   scope named in the prompt: a project request saves under `projects[]` and
   never in `global` or `configs[default]`. Locate the file with
   `agentbox orchestrator config path` or the `path` field from `config show`,
   write or update the completed entry at the selected scope, then run
   `agentbox orchestrator config validate` and `config show` again.
   When the selected runtime uses an AgentBox profile, offer its model from
   `agentbox profile list --json` and state where the value came from.
6. After saving, re-resolve with the same `--project` and `--remote`. For a
   project request, require `"scope": "project"` and `"project": "<name>"`,
   and confirm that every answered selection reports a `project:<name>`
   source. A value that still resolves from `default` or `global` means the
   save missed the project layer; fix it before dispatch. Report a
   `default` or `global` scope for a project request as a failure, not a
   success.
7. Record the resolved configuration, the applied layers, and the per-stage
   sources in the state record before the first dispatch. A selection that
   cannot be honoured sets `blocked` with the exact value and the single action
   that unblocks it.

## Stage ownership and isolation

| Stage | Runs in | Worktree | Forge writes |
| --- | --- | --- | --- |
| triage | worker of the selected type, tracker-blind | its own new worktree | none; it returns a disposition artifact |
| plan | worker of the selected type | its own new worktree | none |
| implement | worker of the selected type, branch `orchestrator/<issue>-<slug>` | its own new worktree | the `implement` row of the stage-aware forge matrix |
| fix | worker of the selected type, existing issue branch | its own worktree at the current pull request head | the `fix` row of the stage-aware forge matrix |
| verify | fresh worker | fresh detached worktree at the full pull request head SHA | none |
| review | fresh worker | fresh detached worktree at the full pull request head SHA | none; it returns a verdict artifact |
| retro | fresh worker, tracker-blind | its own new worktree | none; it returns zero or more issue drafts |
| merge / close | orchestrator | not applicable | orchestrator |

Every stage except merge/close runs in a worker; the orchestrator turn only
assembles context packets, validates returned artifacts, and performs the
tracker, publication, merge, and close writes the selected worker cannot.
Triage, plan, implement, fix, verify, review, and retro are never performed by
the orchestrator turn.

- Verify and review each start from a fresh detached worktree created at the
  full 40-character pull request head SHA. They must not share the
  implementer's worktree and must not see uncommitted state; a verdict about a
  working tree that is not the pushed head is invalid.
- Every stage worker runs in its own worktree and is the only agent process in
  that checkout for the stage. A stage spec never tells a worker to start
  another agent.
- A fix cycle dispatches a fresh fixer and then a fresh reviewer at the new
  head. Previous approvals carry forward only when the diff is unaffected.
- Cycle 1 reviews the full pull request diff against the base. Cycle N>1
  reviews only `prev_head..head`, the ledger status of every prior finding, and
  the gates. A reviewer must not re-derive the touched subsystem or review the
  full diff again; a new finding outside the reviewed range is allowed only at
  `blocker` severity.
- A rebase or force-push invalidates prior verification and review evidence
  and restarts both stages at the new head.
- The fix cycle is a distinct stage. Dispatch `agentbox-fixer`, not a new
  implementer, with only the accepted `blocker` and `major` findings and their
  ledger ids. The fixer must not expand scope or add requirements.
- Triage and retro are tracker-blind and have no shell: they never mutate the
  tracker, forge, or repository. Only the orchestrator applies their results.
- Retro is post-terminal. It runs once after merge/close for a workflow that
  reached implementation and never blocks either.
- Worktrees and worker sessions are evidence. Retain them until the stage
  evidence is captured and the final report is delivered; clean up afterwards
  unless the user asked to keep them.

## Stage-aware forge matrix

One table decides every forge write. Each row is a stage; the host/sandbox
split appears only where worker capability differs. "Workflow comments" means
comments on the issue or pull request that carry the workflow marker.

| Stage | Worker | Local commit | Remote push | Open the PR | Workflow comments / verdicts | Merge / close | Tracker labels, state comment, issue close |
| --- | --- | --- | --- | --- | --- | --- | --- |
| triage | tracker-blind worker | no | no | no | no | no | no |
| plan | worker | no | no | no | no | no | no |
| implement (sandbox) | sandbox worker | yes, local only | no; the orchestrator syncs and pushes | no; the orchestrator creates it | no | no | no |
| implement (host) | Orca child worktree or harness host worker | yes | yes, only `orchestrator/<issue>-<slug>` | yes, with `pr.md` | no | no | no |
| fix (sandbox) | fixer | yes, local only | no; the orchestrator syncs and pushes | no | no | no | no |
| fix (host) | fixer | yes | yes, only the existing pull request branch | no | no | no | no |
| verify | fresh worker | no | no | no | no; it returns gates and observations | no | no |
| review | fresh worker | no | no | no | no; it returns the verdict artifact | no | no |
| merge / close | orchestrator | no | no | no | yes | yes | yes |
| retro | tracker-blind worker | no | no | no | no | no | no |

Applying the matrix:

- **Sandbox implementer:** no forge credential or egress, so the orchestrator
  syncs the run into the host worktree, verifies the branch head, pushes it,
  and opens the pull request. The worker never needs a remote, a forge token,
  or forge egress.
- **Host implementer:** it may commit, push, and open the pull request itself.
  The orchestrator still validates the returned head SHA before review; a push
  somewhere other than `orchestrator/<issue>-<slug>` is a blocked outcome.
- **Fix stage:** the fixer may commit on the existing issue branch and, when it
  is a host worker, push that commit only to the existing pull request branch.
  A sandbox fixer leaves the commit local; the orchestrator syncs and pushes
  it. The fixer never opens a pull request, merges, closes, or accesses the
  tracker.
- **Verify and review:** they perform no git or forge writes. A reviewer
  returns its verdict artifact; the orchestrator posts it, so a verdict has
  one writer and one provenance path. If verifier evidence must reach the
  tracker, the orchestrator posts it with the correct provenance.
- **Tracker authority:** only the orchestrator applies labels, writes the
  state comment, posts workflow comments and verdicts, links the pull request,
  merges, and closes. No worker gains tracker authority, including a capable
  host implementer; opening the pull request is the only implement-stage forge
  write a host worker may perform.
- **Triage, plan, and retro:** they perform no forge or tracker writes and
  only return artifacts. Triage and retro also have no shell.

## Worker contract

A worker owns only local work inside its own isolated checkout: read, edit,
run gates, and commit on its branch. It returns one result the orchestrator
consumes: branch name, HEAD SHA, changed paths, gate output, and any blockers.
A fixer result additionally lists every addressed finding id with evidence at
the returned commit and any unresolved finding.
Its forge capability is exactly its stage row in the matrix above; a worker
that needs a write it lacks reports that instead of attempting it. The worker
performs the stage itself and never launches another agent or a nested
non-interactive agent for the same work; if it lacks a required capability, it
returns a blocker.

- Sandbox commits need an explicit identity: AgentBox configures only the host
  baseline commit today (`AgentBox <agentbox@local.invalid>`), so the task
  spec must set one and the orchestrator must preserve authorship on push.
- A worker result is advisory. The orchestrator checks the commit it actually
  received before it records evidence or starts the next stage.
- The orchestrator records the worker handle, worktree, branch, model, effort,
  and returned commit in the state record so a resumed run and the user can
  find the same worker without guessing.
- Task specs are self-contained: the worker never sees this conversation.

## Stage role contracts

The shipped harness roles implement these contracts. For every stage the
orchestrator supplies a self-contained packet, validates the returned
structure, and performs every tracker mutation. A role that cannot complete
its contract returns a blocker instead of widening its own scope.

| Stage | Role | Inputs | Outputs | Tool limits |
| --- | --- | --- | --- | --- |
| triage | `agentbox-triage` | issue title, body, labels, and relevant comments; repository scope and policy; duplicate-search results; preflight and environment checks | `decision`, `reason`, `evidence`, `questions`, and `risks` covering unresolved assumptions | no tracker or forge access, no shell, no writes; uses only the supplied snapshot |
| fix | `agentbox-fixer` | current pull request head; relevant diff; acceptance criteria; accepted open `blocker` and `major` findings with stable ledger ids; required gates | addressed finding ids with evidence, full commit SHA and changed paths, gate results, unresolved findings, and blockers | own worktree only; no tracker access; no PR creation, merge, close, or verdict; a host push is limited to the existing pull request branch |
| retro | `agentbox-retro` | sanitized workflow record: stage outcomes, state transitions, gate results, review and fix cycles, blockers, and relevant worker reports | zero or more drafts, each with `observation`, `evidence`, `impact` (user impact or recurrence risk), `context`, `acceptance_criteria`, and `related` | no tracker or forge access, no shell, no writes; post-terminal only; never infers a cross-run pattern from one workflow |

Triage details:

- `ready` means actionable, in scope, and one issue. The orchestrator
  rechecks current tracker state before applying the result.
- `needs-info` leaves the issue open, posts the specific questions with
  provenance, records the wait reason, and resumes triage after a response.
- `out-of-scope` rejects with the reported evidence; `blocked` records the
  reason and the single action that unblocks it.
- The orchestrator, not the triage role, applies every label, comment, and
  close.

Fixer details:

- Inputs are limited to the existing pull request head, the reviewed delta,
  the acceptance criteria, the accepted open `blocker` and `major` findings
  with their ledger ids, and the required gates.
- The fixer addresses only those findings. It never broadens scope, adds
  requirements, reopens a deferred finding, or treats a `minor` or `nit` as
  fixer work.
- Verification and an independent review run again on the fixer's resulting
  head before merge.

Retro details:

- Run retro once after a terminal workflow that reached implementation. Skip
  it when triage rejected the issue or a preflight blocked the workflow before
  any work started.
- Retro never blocks merge or close and never mutates the tracker. The
  orchestrator scope-checks and deduplicates every draft before creating any
  follow-up.

## Comment provenance

Create a stable `workflow_id` at triage, for example
`orchestrator-<repo>-<issue>-<short-random>`, store it in the state record,
and never regenerate it. Every workflow comment, pull request body, and
verdict carries exactly one machine-readable marker:

```html
<!-- orchestrator-workflow: {"workflow":"orchestrator-owner-name-35-a1b2c3","issue":35,"stage":"review","worker":"orca","worker_id":"worker-7","model":"model-id","commit":"0123456789abcdef0123456789abcdef01234567","pr":42} -->
```

and exactly one visible footer:

```text
Orchestrated by agentbox-orchestrator {{VERSION}} · workflow orchestrator-owner-name-35-a1b2c3 · stage review · worker orca/codex · model model-id · commit 0123456789abcdef0123456789abcdef01234567
```

- Include the stage, worker, model, and commit in every marker and footer, and
  the pull request number whenever it is known. Use `null` or
  `not-applicable` only where a value genuinely does not exist yet.
- The marker is the source of truth for resuming: read the orchestrator's own
  comments by marker to reconstruct the workflow instead of relying on prose.
- Refuse a second verdict for the same `(workflow, stage, commit)`. If a
  matching verdict already exists, report it and stop that attempt rather than
  posting a duplicate.
- Provenance never implies a human author. It names the stage, worker, model,
  and commit the content applies to.
- A bot account or GitHub App that gives platform-level identity is the
  documented deferred alternative. Until it exists, provenance is
  content-based and every action is attributed by marker and footer.

## Templates

The skill ships six templates and installs them beside `SKILL.md`:

| Template | Used for |
| --- | --- |
| `issue.md` | Normalized issue body: Summary, Current behavior, Proposed design, Acceptance criteria, Test plan, Risks, Out of scope, Related. |
| `pr.md` | Pull request body: Summary, Changes, Evidence, `Closes #N`, Risks, provenance. |
| `verdict.md` | Review verdict: outcome, severity bar, review scope, findings with ids and status, evidence at commit, required changes, follow-ups, provenance. |
| `state.md` | The state envelope and its schema. |
| `review-task.md` | Reviewer mandate: role, delta scope, severity bar, checklist, output contract. |
| `verify-task.md` | Verify mandate: role, gate-only scope, and the `gates` and `observations` output contract. |

Resolve a template from the first source that defines it:

1. an explicit user instruction for this issue;
2. the repository's `.github/orchestrator/<kind>.md`;
3. the template shipped with this skill.

State which template was used when posting the artifact. Never edit a
repository's own templates; use them as they are.

## Configuration precedence

Resolve every other setting from the first source that defines it:

1. the user's explicit instruction for this issue;
2. the matching `$AGENTBOX_HOME` registry project layer;
3. `.github/orchestrator.yaml`, when present;
4. the registry global layer;
5. the registry default configuration;
6. repository documentation and CI (AGENTS.md, CONTRIBUTING, workflow files);
7. the defaults in this skill.

`.github/orchestrator.yaml` may set `orchestrator`, worker and model
selections per stage, `review_cycles_max` (default 2), `merge_strategy`
(default `merge commit`), `poll_seconds` (default 20–30), and
`task_deadline_minutes`. The registry supplies runtime, worker, profile,
model, and effort selections with the layer order documented above. A
lower-precedence source never overrides a higher one, and an explicit user
choice always wins.

## States and labels

Exactly one `orchestrator:*` label is active at any time:

| Label | Meaning |
| --- | --- |
| `orchestrator:triage` | Being qualified: dedupe, eligibility, environment sanity. |
| `orchestrator:ready` | Accepted; plan and gates recorded; waiting for one writer. |
| `orchestrator:in-progress` | One writer owns the branch. |
| `orchestrator:in-review` | Pull request open; independent review under way. |
| `orchestrator:approved` | Review verdict recorded; merge gates satisfied. |
| `orchestrator:blocked` | Stopped; `blocked_reason` explains why. |

Transitions are add-before-remove: add the new label, confirm it is applied,
then remove the old one, so an issue is never left unlabeled and never carries
two active states. The fine-grained phase (which gate, which cycle) lives in
the state record, never in extra labels.

## State record

Maintain exactly one issue comment that starts with
`<!-- orchestrator-state -->` followed by a fenced YAML block. Upsert it in
place after every state change; never leave duplicates. On resume or context
loss, read this comment first and continue from the recorded state. The
`state.md` template is the canonical shape.

```yaml
schema: 2
workflow_id: orchestrator-owner-name-35-a1b2c3
repo: owner/name
issue: 35
state: in-review
labels: [orchestrator:in-review]
orchestrator: {runtime: orca, agent: codex, model: <model-id>, effort: <effort-level>}
configuration:
  registry: ~/.agentbox/orchestration.yaml
  default: standard
  scope: project
  project: agentbox
  sources: [default:standard, global, project:agentbox]
  selection_sources:
    orchestrator: project:agentbox
    triage: global.group:other
    plan: project:agentbox.group:coding
    implement: project:agentbox.stage:implement
    fix: project:agentbox.group:coding
    verify: global.group:other
    review: project:agentbox.stage:review
    retro: global.group:other
triage: {decision: ready, reason: <one sentence>}
stages:
  - {stage: triage, runtime: orca, agent: codex, model: <model-id>, effort: high, worker_id: worker-6, worktree: issue-35-triage, commit: not-applicable}
  - {stage: implement, runtime: orca, agent: codex, model: <model-id>, effort: high, worker_id: worker-7, worktree: issue-35-implement, branch: orchestrator/35-example-slug, commit: <full-sha>, pr: 42}
  - {stage: fix, runtime: orca, agent: codex, model: <model-id>, effort: high, worker_id: worker-8, worktree: issue-35-fix, branch: orchestrator/35-example-slug, commit: <full-sha>, pr: 42}
  - {stage: verify, runtime: orca, agent: codex, model: <model-id>, effort: medium, worker_id: worker-9, worktree: issue-35-verify, commit: <full-sha>, pr: 42}
  - {stage: review, runtime: orca, agent: codex, model: <model-id>, effort: high, worker_id: worker-10, worktree: issue-35-review, commit: <full-sha>, pr: 42}
  - {stage: retro, runtime: orca, agent: codex, model: <model-id>, effort: medium, worker_id: worker-11, worktree: issue-35-retro, commit: not-applicable}
gates:
  - {command: make test, result: pending, commit: <full-sha>}
findings:
  - {id: F1, severity: major, location: internal/app/run.go:42, status: open, opened_at: <full-sha>, closed_at: ""}
  - {id: F2, severity: minor, location: internal/app/run.go:88, status: "deferred #52", opened_at: <full-sha>, closed_at: <full-sha>}
review_cycles: 0
review_cycles_max: 2
retro: {proposals: 0, created: []}
blocked_reason: ""
updated: 2026-11-13T08:00:00Z
```

`configuration` records the resolved registry layers and per-stage sources.
It is optional for workflows that started before the registry existed. Record
the registry path in display form (`$AGENTBOX_HOME/orchestration.yaml` or
`~/.agentbox/orchestration.yaml`), not an expanded absolute host path.

`findings[]` is the finding ledger. Every finding gets a stable `id` that the
review verdict cites; `location` is a repository-relative `path:line`. `status`
is `open`, `fixed`, `deferred #<issue>`, or `wontfix`. Gate 6 requires a status
for every open id before a verdict is posted. A new finding gets a new id, and
a `fixed` id cannot reopen without evidence at the current head. Severity
disputes are settled by the orchestrator against the enumerated definitions in
`verdict.md` and recorded here, so a finding cannot change class between
cycles.

The record may contain model and effort identifiers. It must never contain
credentials, environment-variable values, account identifiers, absolute host
paths, private URLs, logs, or secret values. Values are short identifiers,
tracker references, and gate results only.

`effort` records what the worker actually ran with. Use `not-applicable` when
the selected runtime cannot apply one, such as an AgentBox worker; never
record a requested effort that the worker did not apply.

`worker_id` is the user-visible worker or terminal handle, and `worktree` is
the checkout the stage ran in. Record both for every stage so the user can
follow the run and a resumed orchestrator can find the same worker. A
temporary log path is never the only recorded location.

## Workflow gates

Advance one gate at a time; record the result in the state record before
moving on. Gates 1, 2, 3, 4, 6, 7, and 10 each dispatch a worker; gate 5, pull
request creation, is performed by the host implementer or the orchestrator. No
stage work runs inline.

1. **Triage.** Resolve the registry, ask once for anything it and the prompt
   leave open, confirm inherited defaults, show the resolved selections and
   prerequisites, and obtain approval unless the prompt says “use the existing
   configuration”. Never skip that approval because a registry exists or
   because the values are inherited. A prompt that asks to create or change
   the registry is handled by **Configuration authoring** and does not enter
   this gate. Create the `workflow_id` and write the state record.
   Gather a tracker-blind triage snapshot: issue title, body, labels, and
   relevant comments; repository scope and policy; relevant duplicate-search
   results; and environment sanity (`gh auth status`, `git`, and the selected
   orchestrator and worker tools). Dispatch `agentbox-triage` with that
   snapshot; it returns a structured disposition and never calls the tracker
   or forge.
   - Confirm every role in the stage table is installed for each selected
     harness (`agentbox skills list` reports `current` or `stale`). A missing
     role sets `blocked` with "restart the harness session after
     `agentbox skills install`"; harnesses scan agent directories only at
     session start.
   - Record the tested launch path and the worker visibility method for every
     stage. When a runtime cannot honour the selected profile, model, or
     effort under its normal controls, set `blocked` with the exact
     unsupported selection before dispatching any worker.
   - Validate the disposition. `ready` accepts to `ready`. `out-of-scope`
     rejects with the reported evidence and closes. `blocked` sets `blocked`
     with the reason and the single unblocking action. `needs-info` keeps the
     issue open at `orchestrator:triage`, posts the clarification questions
     with provenance, records the wait reason in `blocked_reason`, and stops;
     rerun triage after a response without adding another workflow label.
   - The orchestrator rechecks current tracker state and applies every label
     and state change itself. The triage role never does.
2. **Plan.** Dispatch a plan worker in its own worktree as a visible Orca
   worker or terminal to write measurable acceptance criteria and the gate
   list — the commands that must pass.
   Normalize the issue with `issue.md`. Define the branch
   `orchestrator/<issue>-<slug>`, the implement worktree, and a
   self-contained task spec for the next worker. The orchestrator posts the
   plan with provenance; it does not write it.
3. **Implement.** Dispatch exactly one writer of the selected type in its own
   worktree with the task spec, as a visible Orca worker or terminal. The
   orchestrator does not edit the branch while a writer owns it.
4. **Verify.** After the worker finishes and the branch head is known,
   dispatch a fresh verify worker in a fresh worktree checked out at the pull
   request head, as a visible Orca worker or terminal. It reruns every gate on
   that commit and returns only `gates`
   and `observations`; it never returns required changes, findings, or a
   verdict, and it is not a second reviewer. Worker-reported results are
   advisory; only evidence at the recorded commit counts. Any gate failure
   returns the issue to `in-progress` with one writer. The orchestrator MUST
   NOT forward a verifier observation to the reviewer as a requirement;
   observations enter the ledger only after the reviewer classifies them, and
   only at their real severity.
5. **Pull request.** A capable host implementer may open it; otherwise the
   orchestrator pushes the sandbox result and opens it. Use a Conventional
   Commits title and the `pr.md` body: Summary, Changes, Evidence (gate
   results and SHAs), `Closes #<issue>`, Risks, and the marker and footer.
   Wait for checks to finish before review.
6. **Independent review.** Dispatch a reviewer that is not the writer, in its
   own worktree at the pull request head, as a visible Orca worker or
   terminal, using the selection recorded for the first pass; later cycles
   reuse that reviewer selection. It returns
   `verdict.md` because one GitHub identity cannot approve its own pull
   request. The orchestrator posts the verdict with provenance and refuses a
   second verdict for the same `(workflow, stage, commit)`. Code changes after
   verification restart gate 4.
   - Cycle 1 reviews the full diff against the base; cycle N>1 reviews
     `prev_head..head`, the ledger status of every prior finding, and the
     gates, as defined in Stage ownership and isolation.
   - Apply the severity bar: only `blocker` and `major` block merge. A
     `minor` or `nit` never blocks merge and is always a follow-up.
   - Accept `approve`, `approve-with-follow-ups`, `request-changes`, or
     `blocked`. `approve-with-follow-ups` requires the mandatory `Follow-ups`
     section and one created issue per deferred group before the verdict is
     posted; the merge then proceeds immediately.
   - Require a ledger status for every open finding id before posting a
     verdict. New findings get new ids, and a `fixed` id cannot reopen without
     evidence at the current head.
   - `review_cycles_max` (default 2) bounds request-changes cycles. At the cap
     the orchestrator dispatches no further fix cycle: it sets `blocked`,
     quotes the open ledger, and asks the maintainer exactly one decision —
     merge with follow-ups, authorize one more cycle, or stop.
7. **Fix cycle.** When review returns `request-changes` and the cycle budget
   remains, dispatch a fresh `agentbox-fixer` in its own worktree at the
   current pull request head, as a visible Orca worker or terminal, with only
   the accepted open `blocker` and `major` findings, their stable ids, the
   acceptance criteria, and the required gates. The fixer works only those
   findings and never expands scope, adds
   requirements, opens a PR, or accesses the tracker. A host fixer may push
   its commit only to the existing pull request branch; the orchestrator syncs
   and pushes a sandbox fixer's commit. Increment `review_cycles`, record the
   fix stage, then rerun gate 4 (verify) and gate 6 (review) on the new head.
   Deferred `minor` and `nit` findings are not fixer work.
8. **Merge.** The orchestrator verifies the head SHA matches the verified and
   reviewed commit, all checks are green, and branch protection is satisfied.
   Any rebase or force-push after verification invalidates the evidence: rerun
   gates and review. Use the repository's merge strategy; prefer a merge
   commit when unspecified.
9. **Close.** Confirm `Closes #<issue>` closed the issue, comment the outcome
   with provenance, and remove all `orchestrator:*` labels. Capture the final
   evidence and deliver the report before cleanup, then remove worktrees,
   runs, and sessions unless the user asked to keep them.
10. **Retro.** After a terminal outcome, and only when the workflow reached
    implementation, dispatch `agentbox-retro` once with a sanitized record:
    stage outcomes, state transitions, gate results, review and fix cycles,
    blockers, and relevant worker reports. It is tracker-blind and returns
    zero or more candidate follow-up issue definitions. Skip retro when triage
    rejected the issue or preflight blocked it before any work started, and
    never let retro delay or reopen merge and close. Deduplicate and
    scope-check every proposal against open issues before creating anything;
    create accepted follow-ups only through the tracker adapter, with
    provenance. Retro never calls the tracker and never claims a cross-run
    pattern from one workflow.

### Severity bar

Only `blocker` and `major` block merge. The response to a finding is fixed by
its severity; a reviewer never chooses how hard a finding blocks.

| Severity | Definition | Action |
| --- | --- | --- |
| `blocker` | Fails an acceptance criterion, loses or corrupts data, exposes a secret, weakens the sandbox or policy, leaves a gate or CI red, or reports false provenance. | Blocks merge. |
| `major` | A reproducible defect in the behavior this pull request changes. | Blocks merge. |
| `minor` | A real issue with no merge risk. | Follow-up only; never blocks. |
| `nit` | Style or preference. | Follow-up only; never blocks. |

A `minor` or `nit` never blocks merge and is always deferred to a follow-up
issue. `Required changes` contains only the smallest changes that remove the
merge risk from open `blocker` and `major` findings.

## Worker runtimes

Dispatch a self-contained task spec to a worker of the selected type. Start
the worker agent itself; never wrap a stage in another agent that launches it.
Record the runtime, agent, model, worker identifier or terminal handle,
worktree, and commit in the state record for every stage, plus the effort
actually applied: a concrete level for Orca, and `not-applicable` for
AgentBox, which has no effort control.

### Repository worker environment (one-time)

AgentBox workers run the repository's own tools, so before the first AgentBox
worker is dispatched to a repository, ensure the repository has a worker
environment: a committed `.agentbox/config.yaml`, validated and applied.
Check for it during triage alongside the other prerequisites.

- If the manifest exists, the environment is ready. Refresh it after manifest
  changes by rerunning `agentbox repo setup`.
- If it is absent, ask the user. With approval, perform the one-time
  initialization as preparation — the same standing as worktree creation, not
  stage work — exactly as the `agentbox-cli` skill documents: author the
  manifest from repository evidence (CI-pinned toolchains, Makefile tools and
  targets, module and package registries, the git remote), validate with
  `agentbox repo setup --dry`, fix every reported error, then apply with
  `agentbox repo setup`. Committing the manifest is a normal repository
  change the user reviews.
- If `agentbox repo` is an unknown command, the installed binary predates
  repository manifests: ask the user to install the current AgentBox
  (`go install github.com/dmartinol/agentbox/cmd/agentbox@main`) and retry.
- Never create or edit a profile for this: runs started for the project use
  the repository image automatically, and setup reports references for
  information only. Report the printed image tag and fragment path to the
  user.

### AgentBox worker loop

```sh
agentbox agent start --profile PROFILE --project PROJECT --gateway GATEWAY \
  --name ISSUE-SLUG --model MODEL --prompt "<full task spec>" \
  --approval unattended
```

Name each stage run `issue-<number>-<slug>`. AgentBox derives the run branch
from its worktree settings, so the generated branch follows
`<branch_prefix>/<project>/issue-<number>-<slug>` (default prefix `agentbox`);
the orchestrator pushes the synced result to the pull request branch
`orchestrator/<issue>-<slug>`.

Poll `agentbox agent show RUN_ID --json` every 20–30 seconds with a per-task
deadline. On completion or stall: `agentbox agent diff RUN_ID` and
`agentbox agent logs RUN_ID` to review the changes and the outcome, then
`agentbox agent sync RUN_ID` so the commits land in the host run worktree. The
orchestrator verifies the synced branch, pushes it from the host, opens the
pull request, and keeps the run and its worktree until the stage evidence is
captured and the final report is delivered. Honor an explicit request to keep
the run; otherwise delete it with `agentbox agent delete RUN_ID` after the
final report. Never reuse a run for a second task.

When a worker must be interrupted — a stall past its deadline or a user stop —
use `agentbox agent stop RUN_ID`. Stop interrupts the agent, syncs the changes
into the host worktree before the sandbox stops, and keeps the run, branch,
and worktree, so the stage resumes from the synced branch in a fresh dispatch.
Never `delete --discard` a run whose changes were not reviewed.

A worker blocked by a sandbox host denial is recovered with the run's **Add
host** remediation — `agentbox policy edit --allow-host HOST RUN_ID`, previewed
with `--dry-run` — never by hand-editing policies. When the repository has a
manifest, also append the host to `.agentbox/config.yaml` and commit it, so
later runs start with the host allowed.

For verify and review, never dispatch with `--project`: that uses the
registered project's current checkout, whose HEAD may not be the pull request
head. Create one fresh detached worktree per stage at the full head SHA and
pass it with `--source`:

```sh
HEAD_SHA=$(gh pr view PR --json headRefOid --jq .headRefOid)
git -C REPO worktree add --detach VERIFY_WORKTREE "$HEAD_SHA"
agentbox agent start --profile PROFILE --source VERIFY_WORKTREE --gateway GATEWAY \
  --name ISSUE-verify --model MODEL --prompt "<full verify task spec>" \
  --approval unattended

git -C REPO worktree add --detach REVIEW_WORKTREE "$HEAD_SHA"
agentbox agent start --profile PROFILE --source REVIEW_WORKTREE --gateway GATEWAY \
  --name ISSUE-review --model MODEL --prompt "<full review task spec>" \
  --approval unattended
```

Record the same full SHA in each stage entry. Keep both worktrees until the
verify and review evidence is captured and the final report is delivered;
clean up afterwards unless the user asked to keep them.

### Orca worker

Use an Orca-visible supervised worker by default. `--spec-file -` sends the
stage spec to the worker as its own task; the worker is the stage agent and
must not launch another agent.

```sh
orca orchestration worker-start --worktree new-child --agent AGENT \
  --model MODEL --effort EFFORT --spec-file - --keep-open --json
```

Record the returned worker handle, worktree, and branch, and report the handle
to the user at stage start and completion. Follow it with `worker-show`,
`worker-read`, and `worker-list` under a per-stage deadline. `--keep-open`
retains the session; keep the worktree until the stage evidence is captured
and the final report is delivered.

An Orca child worktree is a host process sharing host credentials, so a host
implementer may commit, push, and open the pull request itself; the
orchestrator validates the returned head SHA before review and still owns the
merge. When the user requires a harness profile, `worker-start` cannot apply
it; use the visible terminal path in "Profile-bound Orca workers" below
instead of dropping the profile.

For verify and review, start each stage in its own fresh child worktree pinned
to the full pull request head SHA. Never reuse the implementer's worktree or
the project checkout, whose HEAD may not be the pull request head:

```sh
HEAD_SHA=$(gh pr view PR --json headRefOid --jq .headRefOid)
orca orchestration worker-start --worktree new-child --base-branch "$HEAD_SHA" \
  --name ISSUE-verify --agent AGENT --model MODEL --effort EFFORT \
  --spec-file - --keep-open --json
orca orchestration worker-start --worktree new-child --base-branch "$HEAD_SHA" \
  --name ISSUE-review --agent AGENT --model MODEL --effort EFFORT \
  --spec-file - --keep-open --json
```

`--worktree new-child` with `--base-branch "$HEAD_SHA"` creates a separate
fresh worktree for each command, so neither stage sees uncommitted state.
Record the same full SHA and each worker handle in the stage entries. Keep
both worktrees until the verify and review evidence is captured and the final
report is delivered; clean up afterwards unless the user asked to keep them.

#### Profile-bound Orca workers

`orca orchestration worker-start`, `orca agent launch`, and
`orca worktree create --agent` have no `--profile` flag. When the user
requires a harness profile, create the checkout and then start the stage agent
in a visible terminal:

```sh
orca worktree create --repo REPO --name ISSUE-verify \
  --base-branch "$HEAD_SHA" --json
orca terminal create --worktree name:ISSUE-verify --title ISSUE-verify \
  --command "codex -p PROFILE -m MODEL" --json
```

Record the terminal handle, follow it with `orca terminal read`, and send the
rendered stage spec with `orca terminal send`. The terminal command is the
stage agent itself; do not start a second agent inside it. A non-interactive
harness command inside that same visible terminal is allowed when the stage
needs one, for example `codex -p PROFILE -m MODEL exec - < SPEC`; it is still
the stage worker, and its output stays in the terminal or a durable log
outside `/tmp`. If the selected harness has no supported profile-bound path
under its normal sandbox and approval controls, set `blocked` with the exact
unsupported selection. Never use `--dangerously-bypass-approvals-and-sandbox`
or an equivalent to make the worker start.

### Harness agent roles

`agentbox skills install` also writes one harness agent definition per stage
plus the `agentbox-orchestrator` controller role. Dispatch the named stage role
instead of pasting the stage contract into an ad-hoc prompt; the role carries
the tool limits and stage rules for its harness.

| Stage | Role |
| --- | --- |
| triage | `agentbox-triage` |
| plan | `agentbox-planner` |
| implement | `agentbox-implementer` |
| fix | `agentbox-fixer` |
| verify | `agentbox-verifier` |
| review | `agentbox-reviewer` |
| retro | `agentbox-retro` |

Triage and retro render without a shell: Claude omits `Bash`, OpenCode denies
the `shell` action, and Codex carries the rule as instruction text with a
reported limitation, so neither role can reach the tracker or forge. The
fixer has a shell and can commit, but only a host implementer or the
orchestrator opens the pull request, and only the orchestrator writes to the
tracker.

| Harness | Agent file | Invocation |
| --- | --- | --- |
| Codex | `~/.codex/agents/<role>.toml` | select the role as the subagent, or pass the role name in the spawn request |
| Claude Code | `~/.claude/agents/<role>.md` | pass `<role>` as the subagent type |
| OpenCode | `~/.config/opencode/agents/<role>.md` | select the subagent `<role>` |

Start a stage role directly when the harness supports it: Claude runs
`claude --agent agentbox-verifier` (or the role for the stage), and OpenCode
runs `opencode run --agent agentbox-verifier` or selects that subagent. Codex
dispatches the role as a subagent from a top-level session, because Codex has
no top-level `--agent` switch. Never add a launcher agent whose only job is to
start a stage role and relay its result.

The controller role is not a stage. A user assigns it explicitly, it never
starts itself, and it never dispatches another `agentbox-orchestrator`:

| Harness | Agent file | Explicit assignment |
| --- | --- | --- |
| Codex | `~/.codex/agents/agentbox-orchestrator.toml` | Ask the top-level session to use the `agentbox-orchestrator` custom agent. Codex runs custom agents as subagents and has no top-level `--agent` switch. |
| Claude Code | `~/.claude/agents/agentbox-orchestrator.md` | Start the session with `claude --agent agentbox-orchestrator`. |
| OpenCode | `~/.config/opencode/agents/agentbox-orchestrator.md` | Select the primary agent `agentbox-orchestrator` in the session, or run `opencode run --agent agentbox-orchestrator`. |

From Orca, `orca orchestration worker-start --agent <id>` selects only the
host runtime (`codex`, `claude`, or `opencode`); never pass
`agentbox-orchestrator` there. After the worker starts, assign the role through
its harness:

- Codex: ask the started session to use the `agentbox-orchestrator` custom
  agent.
- OpenCode: switch the primary agent to `agentbox-orchestrator`.
- Claude Code: an Orca-started session cannot select the role because Orca
  starts the interactive session without `--agent`. Start Claude directly with
  `--agent agentbox-orchestrator` outside Orca to use the role. This is a
  harness limitation, not a reason to add an Orca-specific role flag.

The controller keeps a shell and subagent dispatch and omits source-edit
tools: Claude gets `Read`, `Grep`, `Glob`, `Bash`, and `Agent` but not `Edit`
or `Write`; OpenCode runs it as a primary agent with `edit` denied and `shell`
and `subagent` available. Those shells are needed for tracker, forge, and
dispatch commands, so a shell-based source write remains instruction-only
there. Codex cannot narrow sandbox, write, or subagent tools from an agent
file, so the no-edit and single-orchestrator rules are instruction text with a
reported limitation.

A harness subagent is a host process sharing host credentials, so it may
commit, push, and open the pull request itself. Supply a model override where
the harness supports one; otherwise record the declared model and set
`blocked` when the requested model cannot be honoured.

Codex agent files cannot narrow sandbox or approval capabilities, so the
read-only verifier and reviewer rules are instruction text there, not
enforced capability. Treat them as prompt-level restrictions.

When the recorded stage selection differs from the role's installed model or
effort, render a copy for that worker instead of editing the installed file:

```sh
agentbox skills roles render agentbox-reviewer --harness codex \
  --model MODEL --effort EFFORT > /tmp/ISSUE-reviewer.toml
```

For verify and review, create one fresh detached worktree per stage at the
full pull request head SHA and launch the worker with that worktree as its
working root. The worker spec, reads, edits, and gate commands must stay
inside that path; never run the worker in the implementer's worktree or the
project checkout:

```sh
HEAD_SHA=$(gh pr view PR --json headRefOid --jq .headRefOid)
git -C REPO worktree add --detach VERIFY_WORKTREE "$HEAD_SHA"
orca orchestration worker-start --worktree path:VERIFY_WORKTREE --agent codex \
  --model MODEL --effort EFFORT --spec-file - --keep-open --json
git -C REPO worktree add --detach REVIEW_WORKTREE "$HEAD_SHA"
orca orchestration worker-start --worktree path:REVIEW_WORKTREE --agent codex \
  --model MODEL --effort EFFORT --spec-file - --keep-open --json
```

For a harness role that must be selected directly, start it in a visible
terminal in the same worktree instead, for example
`orca terminal create --worktree path:VERIFY_WORKTREE --command "claude
--agent agentbox-verifier" --json`. Substitute the harness's own role
invocation where it is not Claude.

A non-interactive harness command is a fallback, not the default. If one is
unavoidable, run it in a visible terminal and keep its output there or in a
durable log outside `/tmp`:

```sh
codex exec -C VERIFY_WORKTREE --model MODEL - < VERIFY_SPEC
codex exec -C REVIEW_WORKTREE --model MODEL - < REVIEW_SPEC
```

Do not add the host repo or the implementer's worktree as an additional
writable root. Record the same full SHA and each worker or terminal handle in
the stage entries. Keep both worktrees until the verify and review evidence is
captured and the final report is delivered; clean up afterwards unless the
user asked to keep them.

## Tracker adapter contract

The core flow touches the tracker only through seven operations: read issue,
list ready, create issue, set labels, comment/upsert state, link PR, close.
Adapters implement exactly these; the flow never invents tracker calls.

### GitHub adapter

| Operation | Command |
| --- | --- |
| Read issue | `gh issue view <number> --json number,title,body,labels,state,comments` |
| List ready | `gh issue list --state open --label orchestrator:ready --json number,title` |
| Create issue | `gh issue create --title "<title>" --body-file <spec>` |
| Set labels | `gh issue edit <number> --add-label <new>` then `gh issue edit <number> --remove-label <old>` |
| Upsert state | find the `<!-- orchestrator-state -->` comment in the issue comments, then `gh api --method PATCH repos/{owner}/{repo}/issues/comments/<id> -F body=@<file>`; create with `gh issue comment <number> --body-file <file>` when absent |
| Link PR | `gh pr create --base <base> --head <branch> --title "<title>" --body-file <file>` with `Closes #<issue>` in the body |
| Close | `gh issue close <number> --comment "<outcome>"` |

Read the pushed head before review with `gh pr view <pr> --json headRefOid`,
and find existing verdicts by searching comments for the
`orchestrator-workflow:` marker before posting a new one.

Create the labels before first use:

```sh
gh label create orchestrator:triage    --color fbca04 --description "Being qualified for the orchestrator flow"
gh label create orchestrator:ready     --color 0e8a16 --description "Accepted; plan recorded; waiting for one writer"
gh label create orchestrator:in-progress --color 1d76db --description "One writer owns the branch"
gh label create orchestrator:in-review  --color 5319e7 --description "Pull request open; independent review under way"
gh label create orchestrator:approved   --color 006b75 --description "Review verdict recorded; merge gates satisfied"
gh label create orchestrator:blocked    --color b60205 --description "Stopped; see blocked_reason in the state record"
```

Repository conventions: the issue body follows `issue.md`. Checks:
`gh pr checks <pr>`; merge: `gh pr merge <pr> --merge` (or the repository's
strategy).

### Jira and Linear slots

No adapter is implemented. When the tracker is Jira or Linear, delegate all
seven operations to the existing Orca skills `orca-jira` and `orca-linear`,
and keep the same states, gates, provenance, and state record. Never call
tracker APIs directly for them.

## Reporting back

Report the issue number and state, the `workflow_id`, the recorded orchestrator
runtime/agent/model/effort, the triage disposition, each stage's worker and
commit, the tested launch path, the worker or terminal handle and worktree the
user can inspect, the branch and PR, gate results with commit SHAs, review
verdicts with the open finding ledger and cycle count, each fix cycle's
addressed finding ids, any retro proposals and created follow-ups, and the
merge SHA. State whether worktrees and sessions were retained or cleaned up.
For stops, quote `blocked_reason`, the open ledger, the unsupported selection
when a runtime, profile, model, or effort could not be honoured, and the single
next action that unblocks the flow; at `review_cycles_max` that action is
exactly one maintainer decision, not an open-ended instruction to keep
iterating.
