---
layout: docs
title: Agentic development lifecycle
permalink: /adlc/
nav_sections:
  - title: Develop the checked-out project
    id: develop-the-checked-out-project
  - title: Direct a task yourself
    id: direct-a-task-yourself
  - title: Coordinate a complete workflow
    id: coordinate-a-complete-workflow
  - title: Where work runs
    id: where-work-runs
---

The agentic development lifecycle (ADLC) is the development work agents perform
on your project: understanding a task, planning a change, implementing it,
verifying the result, and reviewing it before integration.

AgentBox supplies the local sandbox environment for that work. You can direct
individual tasks or add the bundled orchestration skill for a coordinated workflow.

## Develop the checked-out project

Start with an existing Git checkout. A run normally gets a dedicated branch
and host worktree, with a separate repository copy inside its OpenShell sandbox.
The agent uses the project's configured tools and permitted network access.

Examples include implementing a feature, reproducing a defect, running tests,
refactoring a component, and updating documentation. The project being developed
can be any supported repository; it does not have to be an AI application.

## Direct a task yourself

After [initial setup]({{ '/getting-started/' | relative_url }}), start a scoped task:

```sh
agentbox agent start --project demo --profile codex-gateway \
  --gateway openshell --name add-validation \
  --prompt "Add validation for empty project names. Cover the behavior with tests and run the relevant checks."
```

Inspect the result and bring it back:

```sh
agentbox agent logs RUN_ID
agentbox agent diff RUN_ID
agentbox agent sync RUN_ID
```

Review the synced run worktree and branch before integrating the changes.
Sync refuses to overwrite a local run worktree that changed since the previous sync.
Detaching from an agent leaves it running; stopping a run syncs its work before
stopping the sandbox and retains the worktree and branch.

## Coordinate a complete workflow

The optional `agentbox-orchestrator` skill coordinates one issue through the
following stages. This is an instruction-driven workflow executed by a host
coding assistant, rather than a built-in AgentBox scheduling service. Stage
work runs in separate workers using their configured runtimes; the host
controller coordinates and owns merge and close.

![Orchestrated ADLC with a review decision, merge path, and fix loop through verification, CI, and review.]({{ '/assets/adlc.svg' | relative_url }})

| Stage | Result |
| --- | --- |
| Triage | A scoped, actionable issue or a clear reason it cannot proceed. |
| Plan | Acceptance criteria and the checks required to complete the task. |
| Implement | A focused change and its supporting tests. |
| Verify | Fresh execution of the agreed checks at the candidate commit. |
| Pull request and CI | A reviewable change with checks and recorded evidence. |
| Independent review | A separate review of the change at the recorded commit. |
| Fix, when needed | Accepted blocking findings addressed, followed by fresh verification, CI, and independent review. The diagram shows this loop. |
| Merge and close | Integration after the gates pass at the same recorded head commit. |
| Retrospective | Optional follow-up proposals from the workflow record after the terminal outcome; the skill dispatches this when implementation was reached. |

Verification and review serve different purposes: verification reruns the
agreed checks; review evaluates the change. They use separate worktrees pinned
to the candidate/PR head. A changed head requires renewed evidence before merge.

## Where work runs

| Work | Execution boundary |
| --- | --- |
| Workflow coordination | Host coding-assistant session. The controller validates worker results and owns merge and close. |
| Any stage selected with runtime `agentbox` | A separate OpenShell-backed AgentBox run. Sandbox commits are local; the host controller performs forge writes the worker cannot. |
| Triage and retrospective | Restricted worker roles receive a supplied snapshot and have no shell or tracker access. Their runtime is configured separately; sandbox isolation follows that runtime. |
| Workers selected with runtime `orca` or `harness` | The selected runtime's environment; OpenShell isolation is not implied. |
| Pull request and CI | A capable host implementer can open the pull request. Otherwise, the controller syncs and pushes the sandbox commit, then opens it; CI runs in the host forge. |

Choose AgentBox for each worker stage you want to run in an OpenShell sandbox.
The controller coordinates on the host. A worktree alone is not an OpenShell
sandbox.

Continue with [Optional orchestration]({{ '/orchestration/' | relative_url }})
or [Example prompts]({{ '/prompts/' | relative_url }}).
