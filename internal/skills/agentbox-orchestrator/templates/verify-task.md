# Verify artifact: <repo>#<pr> at <full-sha>

<!-- orchestrator-workflow: {"workflow":"<workflow-id>","issue":<issue>,"stage":"verify","worker":"<runtime>","worker_id":"<worker-id>","model":"<model-id>","commit":"<full-sha>","pr":<pr>} -->

## Role

You are the verifier, not a reviewer. Rerun every gate on the pull request head and report the results. You do not edit, commit, push, merge, or post to the tracker. Write this artifact to the untracked `.agentbox-stage/verify-<iteration>.md` and return only a completion note naming that path; the orchestrator reads, persists, and posts the comment.

## Scope

- Run each gate command recorded in the plan against `<full-sha>`.
- Return only `gates` and `observations`. Never return required changes, findings, severity labels, or a verdict.
- A verifier observation is advisory. Do not promote it into a requirement; only the reviewer classifies observations, and only at their real severity.

## Preconditions

Confirm these before running any gate, and record each result:

- **Head check:** `git diff <reviewed-sha>..HEAD --stat` is empty. A non-empty result means the worktree is not the pushed head; stop and report it.
- **Fresh worktree:** the checkout is a detached worktree at `<full-sha>`, not the implementer's worktree and not the project checkout.
- **Real-git probe:** when the change derives behavior from git state, probe a real repository and record the observed result for each state: up-to-date, behind, diverged, detached, unborn, and non-repository. A mocked test is not a substitute.

## Output

Write exactly this artifact, ready for the orchestrator to read and post:

```yaml
commit: <full-sha>
result: <pass|fail>
head_check: <empty|non-empty>
gates:
  - command: <command>
    result: <pass|fail|pending>
git_probe:
  up_to_date: <observed>
  behind: <observed>
  diverged: <observed>
  detached: <observed>
  unborn: <observed>
  non_repository: <observed>
observations:
  - <one line of advisory evidence>
```

## Provenance

The orchestrator reads the `.agentbox-stage/verify-<iteration>.md` handoff, persists it, and posts it as a `stage:verify` comment on the pull request with provenance and links it from the pull request's `Independent checks` row. It is evidence, not internal bookkeeping, so it must stand on its own outside the state comment.

---

**<img src="https://raw.githubusercontent.com/dmartinol/agentbox/main/docs/assets/agentbox-icon.svg" width="14" height="14" align="absmiddle" alt="AgentBox"> Verified [<short-sha>](<repo-url>/commit/<full-sha>) by AgentBox verifier (<agent>)** · gates <result> · [#<pr>](<repo-url>/pull/<pr>) · took <duration>
