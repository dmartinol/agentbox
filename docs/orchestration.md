---
layout: docs
title: Optional orchestration
permalink: /orchestration/
nav_sections:
  - title: Install the host skills
    id: install-the-host-skills
  - title: Configure this project
    id: configure-this-project
  - title: Start the controller
    id: start-the-controller
  - title: Follow and resume work
    id: follow-and-resume-work
  - title: Detailed reference
    id: detailed-reference
---

Use orchestration when a task benefits from a planned issue-to-PR workflow
with separate workers, verification, and independent review. Individual
AgentBox runs do not require it. See [the ADLC guide]({{ '/adlc/' | relative_url }})
for the stages and execution boundaries.

## Install the host skills

Run this on the host where your coding assistant runs:

```sh
agentbox skills install
agentbox skills list
```

Restart the assistant session so it discovers the installed roles. The same
AgentBox binary should install the skills and execute their commands.

## Configure this project

Ask the host assistant:

> Use agentbox-orchestrator to configure orchestration for this project. Use AgentBox for every shell-executing development worker. Ask me to choose the available agent profiles and models. Show the effective selections and save them at project scope. Do not start an issue workflow.

The skill collects the required choices and shows the proposed configuration
before saving. Project scope is an entry in the user-owned registry at
`$AGENTBOX_HOME/orchestration.yaml`, not a file in the repository.

Confirm that all intended development stages resolve to runtime `agentbox`.
Orca and direct harness workers are separate execution choices. AgentBox
workers use a profile/model and do not expose an effort control.

Prepare [the repository environment]({{ '/repository-setup/' | relative_url }})
before dispatching AgentBox workers. The host also needs GitHub CLI access to
the target repository for the current tracker adapter.

Inspect the saved registry with:

```sh
agentbox orchestrator config validate
agentbox orchestrator config show --project /absolute/path/to/repository \
  --remote https://github.com/OWNER/REPOSITORY.git --json
```

Use your actual checkout path and origin URL. The JSON reports the effective
choices and their sources.

## Start the controller

Assign the installed `agentbox-orchestrator` role explicitly:

| Coding assistant | Assignment |
| --- | --- |
| Claude Code | Start `claude --agent agentbox-orchestrator`. |
| OpenCode | Select `agentbox-orchestrator` as the primary agent, or use `opencode run --agent agentbox-orchestrator`. |
| Codex | Ask the top-level session to use the `agentbox-orchestrator` custom agent. |

Then give the assigned controller the issue:

> Take issue 42 to a merged pull request with the orchestrator flow. Use the existing configuration.

Use this prompt when you intend to authorize the complete issue-to-merge
workflow. Without “use the existing configuration”, the skill displays the
resolved selections and requests approval before starting.

## Follow and resume work

The controller reports the stage, worker handle, worktree, commit, and
outcome. For AgentBox workers, use Fleet or the CLI to inspect their runs;
the issue and PR carry the workflow state and evidence.

> Resume the orchestrator run on issue 42. Use the existing configuration.

Durable stage artifacts live under `$AGENTBOX_HOME/orchestrator/<workflow-id>/`; a stage is not complete until its artifact exists and is non-empty, and a resumed flow re-renders the next stage spec from the persisted artifact rather than the previous worker session. It preserves the verification/review gates before merge.

## Detailed reference

- [Configuration commands and schema]({{ '/cli-reference/orchestrator/' | relative_url }})
- [Skill installation and role reference]({{ '/cli-reference/skills/' | relative_url }})
- [Full workflow contract](https://github.com/dmartinol/agentbox/blob/main/internal/skills/agentbox-orchestrator/SKILL.md)
- [Prompt cookbook]({{ '/prompts/' | relative_url }})

Role permission matrices, state schemas, finding severity rules, forge
capabilities, runtime-specific launch paths, and templates belong in these
references rather than in the project introduction.
