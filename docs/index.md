---
layout: docs
title: AgentBox
permalink: /
nav_sections:
  - title: Build the agent environment
    id: build-the-agent-environment
  - title: Configure the project
    id: configure-the-project
  - title: Follow the development lifecycle
    id: follow-the-development-lifecycle
  - title: Choose your next step
    id: choose-your-next-step
---

**Sandboxed agentic development for your Git repositories.**

Give a coding agent a task on your project. AgentBox runs it inside a local
OpenShell sandbox backed by Podman, keeps its work separate by default, and
brings the results back for review.

Use Codex, Claude Code, or OpenCode to investigate code, implement a feature,
fix a defect, run checks, or review a change.

[Start your first task]({{ '/getting-started/' | relative_url }}) ·
[Explore the ADLC]({{ '/adlc/' | relative_url }}) ·
[Copy an example prompt]({{ '/prompts/' | relative_url }})

<p align="center">
  <img src="{{ '/screenshots/flow.gif' | relative_url }}" alt="Animated AgentBox tour: monitor the fleet, configure profiles and projects in Setup, inspect a sandbox network denial, and add the blocked host to the run policy" width="100%">
</p>

## Build the agent environment

Build a revisioned OpenShell image for the harness you use: Codex, Claude Code,
or OpenCode. AgentBox runs the selected harness inside a local sandbox backed
by Podman.

- **Harness image:** build or update an image for a selected coding harness.
- **Project configuration:** describe the repository's tools, pinned
  toolchains, setup checks, and network hosts in `.agentbox/config.yaml`.
- **Sandboxed runs:** start agents against an isolated copy of the Git project,
  then inspect and sync their work for review.

![Choose Codex, Claude Code, or OpenCode, build a revisioned OpenShell image for it, then run it in a Podman-backed sandbox.]({{ '/assets/harness-images.svg' | relative_url }})

## Configure the project

AgentBox uses project configuration to build a repository image on top of the
selected harness image and generate the matching network policy. When a run
hits a blocked host, AgentBox surfaces the denial and lets you add the host to
the run policy; you can also save that rule in the project configuration for
future runs.

The manifest is authored from repository and CI evidence, then reviewed by
you. AgentBox validates it and builds the image and policy from the declared
requirements. See [Project configuration]({{ '/repository-setup/' | relative_url }})
for the workflow.

![Repository and CI evidence informs project configuration; AgentBox builds a repository image and network policy, with an explicit add-host flow for surfaced denials.]({{ '/assets/project-configuration.svg' | relative_url }})

## Use it across the development lifecycle

Choose a configured agent profile and model provider. Use the CLI or web GUI to
inspect progress, attach, and review diffs. Sync results to the local run
worktree before integration.

The sandbox uses a repository copy. Sync transfers results back; it does not
merge them into your main branch.

## Follow the development lifecycle

Direct individual agent runs yourself, or use the optional orchestration skill
to coordinate triage, planning, implementation, verification, independent
review, and gated merge. Configure the worker runtime and harness by supported
stage to choose which work runs in AgentBox's OpenShell sandbox.

![Issue-to-PR ADLC. Review branches to merge or a visible fix loop through verification, CI, and review. Restricted triage and retrospective roles use their configured runtime.]({{ '/assets/adlc.svg' | relative_url }})

The image shows AgentBox selected for development workers. Triage and
retrospective are restricted worker roles and use their configured runtime.
The host controller coordinates and handles forge operations unavailable to a
sandbox worker. Other worker runtimes do not inherit AgentBox's OpenShell
isolation.

[Read the ADLC guide]({{ '/adlc/' | relative_url }}) ·
[Set up optional orchestration]({{ '/orchestration/' | relative_url }})

## Choose your next step

| I want to… | Start here |
| --- | --- |
| Run my first development task | [Getting started]({{ '/getting-started/' | relative_url }}) |
| Configure the project's image and network access | [Project configuration]({{ '/repository-setup/' | relative_url }}) |
| Operate AgentBox through my coding assistant | [Example prompts]({{ '/prompts/' | relative_url }}) |
| Coordinate an issue-to-PR workflow | [Optional orchestration]({{ '/orchestration/' | relative_url }}) |
| Look up a command or runtime detail | [CLI reference]({{ '/cli-reference/' | relative_url }}) · [Architecture]({{ '/architecture/' | relative_url }}) |

AgentBox is an experimental local utility, not a supported product release.
