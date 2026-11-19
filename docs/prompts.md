---
layout: docs
title: Example prompts
permalink: /prompts/
nav_sections:
  - title: Before you start
    id: before-you-start
  - title: Set up and operate AgentBox
    id: set-up-and-operate-agentbox
  - title: Coordinate a development workflow
    id: coordinate-a-development-workflow
  - title: Use the local API
    id: use-the-local-api
  - title: Tasks for the sandboxed agent
    id: tasks-for-the-sandboxed-agent
---

Copy a prompt into your coding assistant and replace the example project,
profile, issue, or run name. Explicitly naming the skill makes your intended
workflow clear; discovery and invocation controls depend on the assistant.

## Before you start

```sh
agentbox skills install
agentbox skills list
```

Restart your host assistant session. These skills help an assistant operate
AgentBox from the host; installing them does not launch an agent or configure
your model provider.

## Set up and operate AgentBox

Use these in your **host assistant** with `agentbox-cli` available.

| Goal | Copy this prompt | Expected result |
| --- | --- | --- |
| Prepare a project | Use agentbox-cli to set up AgentBox for this repository. Derive its tools, verification commands, and required hosts from the repository and CI. | A proposed repository manifest, validation, and repository setup. |
| Start a focused change | Use agentbox-cli to start a Codex run on project demo with profile codex-gateway and gateway openshell. Ask it to fix the failing parser tests and run the relevant checks. | A sandbox run and an inspectable run ID. |
| Inspect progress | Use agentbox-cli to show which agents are running and which need my input. | A status summary with actionable blockers. |
| Review results | Use agentbox-cli to show what run RUN_ID changed, then sync its results. | A diff and results in the local run worktree. |
| Recover a network block | Use agentbox-cli to investigate run RUN_ID's connection failure to api.example.com. Use the surfaced denial to propose the exact allowed host, port, and binary. | An evidence-based policy change proposal. |
| Add missing tools | Use agentbox-cli to update this repository's AgentBox environment: its checks need Go and make. Use versions supported by the repository and CI. | A manifest update and refreshed repository environment. |
| Keep work while stopping | Use agentbox-cli to stop run RUN_ID and keep its work. | Changes synced before stopping; run, worktree, and branch retained. |

## Coordinate a development workflow

Use these in the **host assistant** with `agentbox-orchestrator` available.
Assign the controller role as described in [Optional orchestration]({{ '/orchestration/' | relative_url }}).

| Goal | Copy this prompt | Expected result |
| --- | --- | --- |
| Configure the sandboxed workflow | Use agentbox-orchestrator to configure orchestration for this project. Use AgentBox for every shell-executing development worker. Ask me to choose available profiles and models, and save at project scope. Do not start a workflow. | Configuration interview, proposed changes, and validated project selections. |
| Inspect selections | Use agentbox-orchestrator to inspect the resolved orchestration configuration for this project. Show which stages use AgentBox sandboxes and where each selection comes from. Do not start a workflow. | Effective selections and their sources. |
| Run an issue | Take issue 42 to a merged pull request with the orchestrator flow. | Resolved configuration for approval, then the staged issue-to-merge workflow. |
| Reuse approved configuration | Take issue 42 to a merged pull request with the orchestrator flow. Use the existing configuration. | The same workflow using saved selections without the configuration approval pause. |
| Resume | Resume the orchestrator run on issue 42. Use the existing configuration. | Recorded state loaded and the required stage redispatched. |

The issue-to-merge prompts authorize implementation, tracker/PR updates, and
merge when the workflow gates pass. Use the configuration and inspection
prompts first when you only want to prepare the workflow.

## Use the local API

Use these in your **host assistant** with `agentbox-api` available and
`agentbox gui` running.

| Goal | Copy this prompt |
| --- | --- |
| Inspect runs | Use agentbox-api to list every agent run through the local AgentBox API and summarize their status. |
| Automate a run | Use agentbox-api to write a curl example that starts a run for project demo with profile codex-gateway and gateway openshell. |
| Find a setting | Use agentbox-api to explain where I change the gateway setting in the web UI. |

## Tasks for the sandboxed agent

After starting or attaching to a run, give **the agent inside the sandbox** a
development task directly. These are task examples, not AgentBox skill triggers:

- “Explain how this repository validates configuration. Identify the relevant files.”
- “Reproduce the parser failure, implement a focused fix, and run the relevant tests.”
- “Add validation for empty project names and cover the behavior with tests.”
- “Review this branch for regressions. Report actionable findings with file references.”
- “Update the installation guide to match the current commands.”

Selecting additional skills to copy into the sandbox uses
[`agentbox skill`]({{ '/cli-reference/skill/' | relative_url }}), distinct from
the host-side `agentbox skills install` command.
