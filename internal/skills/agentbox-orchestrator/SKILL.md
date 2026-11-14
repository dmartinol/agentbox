---
name: agentbox-orchestrator
description: "Drive one tracker issue end to end: triage, dispatch plan, implement, verify, and review to isolated workers with recorded agent, model, and effort, enforce gates, post provenance-marked PRs and verdicts, and own merge and close. Use when asked to take an issue to a merged pull request with the orchestrator flow."
version: "{{VERSION}}"
---

# Issue-to-PR orchestrator flow

This skill drives exactly one tracker issue from triage to a merged pull
request and a closed issue. The core flow is tracker-agnostic; GitHub is the
first adapter. It is instructions only: the orchestrator follows this
document, calls the tracker adapter, and dispatches workers. It never adds
code or tooling on its own.

## Role boundary (MUST)

The orchestrator owns exactly two things:

- **Task orchestration (mandatory):** triage, select and dispatch every
  stage's worker, record provenance, enforce gates, deadlines, and fix-cycle
  limits, and own merge and close.
- **Git and forge management (optional, capability-driven):** only the writes
  a selected worker cannot perform itself — sync, push, pull request
  creation, comment and label posting, and the state comment. If every
  selected worker type can do these, the orchestrator performs no git
  management at all.

The orchestrator MUST NOT plan, implement, verify, or review inline. If it
starts doing stage work, it stops and dispatches a worker. Planning a task
spec for a worker, reading a returned artifact, and checking forge state are
orchestration, not stage work. A resumed run redispatches the stage; it never
continues a previous worker's session.

## Ground rules

- One issue, one writer, one branch at a time. Never run two writers on one
  issue.
- Ask for runtime and model selections once, record them, and never silently
  substitute another value.
- The orchestrator is the only actor that merges and closes, and the only
  actor that posts review verdicts, for every worker type.
- Never bypass branch protection, required checks, or review requirements.
- On any doubt, set `blocked` with a reason instead of improvising.

## Runtime and model selection

Ask once during triage, record every answer in the state record, and keep the
answers for the life of the issue. Confirm an inherited default explicitly
before recording it; a default is never applied silently.

| Selection | Asked for | Default |
| --- | --- | --- |
| Orchestrator | runtime (`orca`, `agentbox`, or `harness`), agent, model, effort | ask; inherited runtime defaults are confirmed |
| Plan worker | runtime, agent, model, effort | inherits the orchestrator's choice after confirmation |
| Implement worker | runtime, agent, model, effort | inherits the plan selection after confirmation |
| Verify worker | runtime, agent, model, effort | a different agent or model from the implementer when one is available |
| Review worker | runtime, agent, model, effort | a different agent or model from the implementer when one is available |

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

## Stage ownership and isolation

| Stage | Runs in | Worktree | Forge writes |
| --- | --- | --- | --- |
| triage | orchestrator | not applicable | none |
| plan | worker of the selected type | its own new worktree | none |
| implement | worker of the selected type, branch `orchestrator/<issue>-<slug>` | its own new worktree | see the forge capability matrix |
| verify | fresh worker | fresh detached worktree at the full pull request head SHA | none |
| review | fresh worker | fresh detached worktree at the full pull request head SHA | none; it returns a verdict artifact |
| merge / close | orchestrator | not applicable | orchestrator |

Every stage except triage and merge/close runs in its own selected
worker/worktree. Plan, implement, verify, and review are never performed by
the orchestrator turn.

- Verify and review each start from a fresh detached worktree created at the
  full 40-character pull request head SHA. They must not share the
  implementer's worktree and must not see uncommitted state; a verdict about a
  working tree that is not the pushed head is invalid.
- A fix cycle dispatches a new implementer and then a new reviewer at the new
  head. Previous approvals carry forward only when the diff is unaffected.
- Cycle 1 reviews the full pull request diff against the base. Cycle N>1
  reviews only `prev_head..head`, the ledger status of every prior finding, and
  the gates. A reviewer must not re-derive the touched subsystem or review the
  full diff again; a new finding outside the reviewed range is allowed only at
  `blocker` severity.
- A rebase or force-push invalidates prior verification and review evidence
  and restarts both stages at the new head.

## Forge capability matrix

One table decides every forge write. Capability belongs to the worker type,
not to a particular stage.

| Worker type | Can commit | Can push | Can open the PR | Can post comments or verdicts | Can merge or close |
| --- | --- | --- | --- | --- | --- |
| AgentBox sandbox worker | yes, local only | no | no | no | no |
| Orca child worktree (host) | yes | yes | yes | yes, except verdicts and state | no |
| Harness subagent (host) | yes | yes | yes | yes, except verdicts and state | no |
| Orchestrator | only when a worker cannot | only for a sandbox worker | only for a sandbox worker | yes, including verdicts and state | yes |

Applying the matrix:

- **Sandbox implementer:** no forge credential or egress, so the orchestrator
  syncs the run into the host worktree, verifies the branch head, pushes it,
  and opens the pull request. The worker never needs a remote, a forge token,
  or forge egress.
- **Host implementer:** it may commit, push, and open the pull request itself.
  The orchestrator still validates the returned head SHA before review; a push
  somewhere other than `orchestrator/<issue>-<slug>` is a blocked outcome.
- **Every worker type:** merge, close, the state comment, and review verdicts
  remain orchestrator-authored. A reviewer returns its verdict artifact; the
  orchestrator posts it, so a verdict has one writer and one provenance path.
- **Plan and verify:** they perform no forge writes. If their evidence must
  reach the tracker, the orchestrator posts it with the correct provenance.

## Worker contract

A worker owns only local work inside its own isolated checkout: read, edit,
run gates, and commit on its branch. It returns one result the orchestrator
consumes: branch name, HEAD SHA, changed paths, gate output, and any blockers.
Its forge capability is exactly the row for its worker type in the matrix
above; a worker that needs a write it lacks reports that instead of attempting
it.

- Sandbox commits need an explicit identity: AgentBox configures only the host
  baseline commit today (`AgentBox <agentbox@local.invalid>`), so the task
  spec must set one and the orchestrator must preserve authorship on push.
- A worker result is advisory. The orchestrator checks the commit it actually
  received before it records evidence or starts the next stage.
- Task specs are self-contained: the worker never sees this conversation.

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
2. `.github/orchestrator.yaml`, when present;
3. repository documentation and CI (AGENTS.md, CONTRIBUTING, workflow files);
4. the defaults in this skill.

`.github/orchestrator.yaml` may set `orchestrator`, worker and model
selections per stage, `review_cycles_max` (default 2), `merge_strategy`
(default `merge commit`), `poll_seconds` (default 20–30), and
`task_deadline_minutes`. A lower-precedence source never overrides a higher
one, and an explicit user choice always wins.

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
stages:
  - {stage: implement, runtime: orca, agent: codex, model: <model-id>, effort: high, worker_id: worker-7, worktree: issue-35-implement, branch: orchestrator/35-example-slug, commit: <full-sha>, pr: 42}
  - {stage: verify, runtime: orca, agent: codex, model: <model-id>, effort: medium, worker_id: worker-9, worktree: issue-35-verify, commit: <full-sha>, pr: 42}
  - {stage: review, runtime: orca, agent: codex, model: <model-id>, effort: high, worker_id: worker-10, worktree: issue-35-review, commit: <full-sha>, pr: 42}
gates:
  - {command: make test, result: pending, commit: <full-sha>}
findings:
  - {id: F1, severity: major, location: internal/app/run.go:42, status: open, opened_at: <full-sha>, closed_at: ""}
  - {id: F2, severity: minor, location: internal/app/run.go:88, status: "deferred #52", opened_at: <full-sha>, closed_at: <full-sha>}
review_cycles: 0
review_cycles_max: 2
blocked_reason: ""
updated: 2026-11-13T08:00:00Z
```

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

## Workflow gates

Advance one gate at a time; record the result in the state record before
moving on. Gates 2, 3, 4, and 6 each dispatch a worker; gate 5, pull request
creation, is performed by the host implementer or the orchestrator. No stage
work runs inline.

1. **Triage.** Search for duplicate open issues, check eligibility (in scope
   for this repository, actionable, one issue), and run environment sanity
   (`gh auth status`, `git`, and the selected orchestrator and worker tools).
   Ask once for the orchestrator and per-stage selections, confirm inherited
   defaults, create the `workflow_id`, and write the state record. Accept to
   `ready`, reject with a comment and close, or set `blocked`.
2. **Plan.** Dispatch a plan worker in its own worktree to write measurable
   acceptance criteria and the gate list — the commands that must pass.
   Normalize the issue with `issue.md`. Define the branch
   `orchestrator/<issue>-<slug>`, the implement worktree, and a
   self-contained task spec for the next worker. The orchestrator posts the
   plan with provenance; it does not write it.
3. **Implement.** Dispatch exactly one writer of the selected type in its own
   worktree with the task spec. The orchestrator does not edit the branch
   while a writer owns it. A fix cycle dispatches a fresh implementer rather
   than continuing the old session.
4. **Verify.** After the worker finishes and the branch head is known,
   dispatch a fresh verify worker in a fresh worktree checked out at the pull
   request head. It reruns every gate on that commit and returns only `gates`
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
   own worktree at the pull request head, using the selection recorded for the
   first pass; later cycles reuse that reviewer selection. It returns
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
7. **Merge.** The orchestrator verifies the head SHA matches the verified and
   reviewed commit, all checks are green, and branch protection is satisfied.
   Any rebase or force-push after verification invalidates the evidence: rerun
   gates and review. Use the repository's merge strategy; prefer a merge
   commit when unspecified.
8. **Close.** Confirm `Closes #<issue>` closed the issue, comment the outcome
   with provenance, remove all `orchestrator:*` labels, and clean up
   worktrees and runs.

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

Dispatch a self-contained task spec to a worker of the selected type. Record
the runtime, agent, model, worker identifier, worktree, and commit in the
state record for every stage, plus the effort actually applied: a concrete
level for Orca, and `not-applicable` for AgentBox, which has no effort
control.

### AgentBox worker loop

```sh
agentbox agent start --profile PROFILE --project PROJECT --gateway GATEWAY \
  --name ISSUE-SLUG --model MODEL --prompt "<full task spec>" \
  --approval unattended
```

Poll `agentbox agent show RUN_ID --json` every 20–30 seconds with a per-task
deadline. On completion or stall: `agentbox agent diff RUN_ID` and
`agentbox agent logs RUN_ID` to review the changes and the outcome, then
`agentbox agent sync RUN_ID` so the commits land in the host run worktree. The
orchestrator verifies the synced branch, pushes it from the host, opens the
pull request, and deletes the run with `agentbox agent delete RUN_ID`; never
reuse a run for a second task.

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

Record the same full SHA in each stage entry, and remove each worktree after
the stage returns.

### Orca worker

```sh
orca orchestration worker-start --worktree new-child --agent AGENT \
  --model MODEL --effort EFFORT --spec-file - --json
```

Then use `worker-show`, `worker-read`, and wait with a per-stage deadline.
An Orca child worktree is a host process sharing host credentials, so it may
commit, push, and open the pull request itself; the orchestrator validates
the returned head SHA before review and still owns the merge.

For verify and review, start each stage in its own fresh child worktree pinned
to the full pull request head SHA. Never reuse the implementer's worktree or
the project checkout, whose HEAD may not be the pull request head:

```sh
HEAD_SHA=$(gh pr view PR --json headRefOid --jq .headRefOid)
orca orchestration worker-start --worktree new-child --base-branch "$HEAD_SHA" \
  --name ISSUE-verify --agent AGENT --model MODEL --effort EFFORT \
  --spec-file - --json
orca orchestration worker-start --worktree new-child --base-branch "$HEAD_SHA" \
  --name ISSUE-review --agent AGENT --model MODEL --effort EFFORT \
  --spec-file - --json
```

`--worktree new-child` with `--base-branch "$HEAD_SHA"` creates a separate
fresh worktree for each command, so neither stage sees uncommitted state.
Record the same full SHA in each stage entry and remove each worktree after
the stage returns.

### Harness worker

Use the current harness's subagent for a stage that needs a local worker. A
harness subagent is a host process sharing host credentials, so it may commit,
push, and open the pull request itself. Supply a model override where the
harness supports one; otherwise record the declared model and set `blocked`
when the requested model cannot be honoured.

For verify and review, create one fresh detached worktree per stage at the
full pull request head SHA and launch the worker with that worktree as its
working root. The worker spec, reads, edits, and gate commands must stay
inside that path; never run the worker in the implementer's worktree or the
project checkout:

```sh
HEAD_SHA=$(gh pr view PR --json headRefOid --jq .headRefOid)
git -C REPO worktree add --detach VERIFY_WORKTREE "$HEAD_SHA"
codex exec -C VERIFY_WORKTREE --model MODEL - < VERIFY_SPEC
git -C REPO worktree add --detach REVIEW_WORKTREE "$HEAD_SHA"
codex exec -C REVIEW_WORKTREE --model MODEL - < REVIEW_SPEC
```

Substitute the harness's own non-interactive command where it is not Codex,
and do not add the host repo or the implementer's worktree as an additional
writable root. Record the same full SHA in each stage entry and remove each
worktree after the stage returns.

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
runtime/agent/model/effort, each stage's worker and commit, the branch and PR,
gate results with commit SHAs, review verdicts with the open finding ledger and
cycle count, and the merge SHA. For stops, quote `blocked_reason`, the open
ledger, and the single next action that unblocks the flow; at
`review_cycles_max` that action is exactly one maintainer decision, not an
open-ended instruction to keep iterating.
