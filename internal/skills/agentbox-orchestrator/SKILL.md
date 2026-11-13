---
name: agentbox-orchestrator
description: "Drive one tracker issue end to end: triage, plan, implement with one writer, verify on the synced commit, pull request, independent review, gated merge, and close. Use when asked to take an issue to a merged pull request with the orchestrator flow."
version: "{{VERSION}}"
---

# Issue-to-PR orchestrator flow

This skill drives exactly one tracker issue from triage to a merged pull
request and a closed issue. The core flow is tracker-agnostic; GitHub is the
first adapter. It is instructions only: the orchestrator follows this document,
calls the tracker adapter, and supervises workers. It never adds code or
tooling on its own.

## Ground rules

- One issue, one writer, one branch at a time. Never run two writers on one
  issue.
- Ask for runtime selections once, record them, and never silently fall back.
- The orchestrator host performs tracker and remote writes (`gh`, `git push`)
  until sandbox images ship GitHub tooling. Workers deliver changes; they do
  not touch the tracker or the remote.
- Never bypass branch protection, required checks, or review requirements.
- On any doubt, set `blocked` with a reason instead of improvising.

## Runtime selection

Ask the user once during triage, record both answers in the state record, and
keep them for the life of the issue. If a selection becomes unavailable, set
`blocked` and ask again; do not switch runtimes on your own.

| Choice | Options | Default |
| --- | --- | --- |
| Orchestrator | `orca` (recommended: supervision, ask/reply, decision gates) or `harness` (native subagents of the current harness) | ask |
| Worker | `agentbox` (recommended: one sandboxed run per task), `orca` (child worktree), or `harness` (review-only or sequential subagent) | ask |

## Configuration precedence

Resolve every setting from the first source that defines it:

1. the user's explicit instruction for this issue;
2. `.github/orchestrator.yaml`, when present;
3. repository documentation and CI (AGENTS.md, CONTRIBUTING, workflow files);
4. the defaults in this skill.

`.github/orchestrator.yaml` may set `orchestrator`, `worker`,
`review_cycles_max` (default 3), `merge_strategy` (default `merge commit`),
`poll_seconds` (default 20–30), and `task_deadline_minutes`. A lower-precedence
source never overrides a higher one, and an explicit user choice always wins.

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
loss, read this comment first and continue from the recorded state.

```yaml
schema: 1
repo: owner/name
issue: 32
state: in-review            # triage|ready|in-progress|in-review|approved|blocked
labels: [orchestrator:in-review]
orchestrator: orca          # orca|harness
worker: agentbox            # agentbox|orca|harness
worker_id: <run or worktree identifier>
branch: orchestrator/32-github-issue-feature-flow
pr: 45
commit: <full SHA of the verified head>
gates:
  - command: make test
    result: pass            # pass|fail|pending
    commit: <full SHA the gate ran on>
review_cycles: 1
blocked_reason: ""          # required when state is blocked, else empty
updated: 2026-11-13T08:00:00Z
```

Never record logs, secrets, account IDs, absolute host paths, private URLs, or
model and provider names. Values are short identifiers, tracker references,
and gate results only.

## Workflow gates

Advance one gate at a time; record the result in the state record before
moving on.

1. **Triage.** Search for duplicate open issues, check eligibility (in scope
   for this repository, actionable, one issue), and run environment sanity
   (`gh auth status`, `git`, and the selected orchestrator and worker tools).
   Accept to `ready`, reject with a comment and close, or set `blocked`.
2. **Plan.** Write measurable acceptance criteria and the gate list — the
   commands that must pass — into the issue. Define the branch
   `orchestrator/<issue>-<slug>`, the worktree, and a self-contained task spec
   for the worker. The task spec carries all context the worker needs;
   never assume the worker sees this conversation.
3. **Implement.** Start exactly one writer in its own worktree with the task
   spec. The orchestrator does not edit the branch while a writer owns it.
4. **Verify.** After the worker finishes and the branch is synced, the
   orchestrator reruns every gate on the synced commit. Worker-reported
   results are advisory; only the orchestrator's run on the recorded commit
   counts. Any failure returns the issue to `in-progress` with one writer.
5. **Pull request.** Open the PR with a Conventional Commits title and a body
   containing Summary, Changes, Evidence (gate results and SHAs),
   `Closes #<issue>`, and Risks. Wait for checks to finish before review.
6. **Independent review.** A separate reviewer that is not the writer
   reviews the PR at its head and records the verdict as a comment, because
   one GitHub identity cannot approve its own PR. Default 3 cycles of
   review, address, re-review. Code changes after verification restart
   gate 4; unchanged approvals carry to the new head only when the diff is
   unaffected.
7. **Merge.** Verify the head SHA matches the verified and reviewed commit,
   all checks are green, and branch protection is satisfied. Any rebase or
   force-push after verification invalidates the evidence: rerun gates and
   review. Use the repository's merge strategy; prefer a merge commit when
   unspecified.
8. **Close.** Confirm `Closes #<issue>` closed the issue, comment the
   outcome, remove all `orchestrator:*` labels, and clean up worktrees and
   runs.

## Worker runtimes

### AgentBox worker loop (recommended)

```sh
agentbox agent start --profile PROFILE --project PROJECT --gateway GATEWAY \
  --name ISSUE-SLUG --prompt "<full task spec>" --approval unattended
```

Poll `agentbox agent show RUN_ID --json` every 20–30 seconds with a
per-task deadline. On completion or stall: `agentbox agent logs RUN_ID` to
read the outcome, `agentbox agent diff RUN_ID` to review the changes, then
`agentbox agent sync RUN_ID` to bring the branch into the local worktree for
verification. After the verified branch is pushed and reviewed, delete the
run with `agentbox agent delete RUN_ID`; never reuse a run for a second task.

### Orca worker

Dispatch the task spec to an Orca child worktree, wait for the worker's
result, and sync that worktree's branch for verification. Same rules: one
writer, own worktree, no tracker access.

### Harness worker

Use the current harness's subagent for small, self-contained tasks
(review-only or strictly sequential execution). The orchestrator still owns
gates, the state record, and all tracker writes.

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

Create the labels before first use:

```sh
gh label create orchestrator:triage    --color fbca04 --description "Being qualified for the orchestrator flow"
gh label create orchestrator:ready     --color 0e8a16 --description "Accepted; plan recorded; waiting for one writer"
gh label create orchestrator:in-progress --color 1d76db --description "One writer owns the branch"
gh label create orchestrator:in-review  --color 5319e7 --description "Pull request open; independent review under way"
gh label create orchestrator:approved   --color 006b75 --description "Review verdict recorded; merge gates satisfied"
gh label create orchestrator:blocked    --color b60205 --description "Stopped; see blocked_reason in the state record"
```

Repository conventions: the issue body carries `## Context`,
`## Acceptance criteria`, `## Out of scope`, and `## Notes` sections; triage
normalizes missing sections before `ready`. Checks: `gh pr checks <pr>`;
merge: `gh pr merge <pr> --merge` (or the repository's strategy).

### Jira and Linear slots

No adapter is implemented. When the tracker is Jira or Linear, delegate all
seven operations to the existing Orca skills `orca-jira` and `orca-linear`,
and keep the same states, gates, and state record. Never call tracker APIs
directly for them.

## Reporting back

Report the issue number and state, the recorded orchestrator and worker, the
branch and PR, gate results with commit SHAs, review verdicts and cycle
count, and the merge SHA. For stops, quote `blocked_reason` and the single
next action that unblocks the flow.
