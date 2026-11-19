---
layout: docs
title: AgentBox
permalink: /
nav_sections:
  - title: Develop inside a sandbox
    id: develop-inside-a-sandbox
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

## Develop inside a sandbox

- **Your project:** start from an existing Git checkout and give each run its own worktree and branch.
- **Your development environment:** declare repository tools, build checks, and required network hosts.
- **Your agent and model:** choose a configured agent profile and model provider.
- **Your view of the work:** use the CLI or web GUI to inspect progress, attach, and review diffs.
- **Your results:** sync changes back to the local run worktree before integration.

The sandbox uses a repository copy. Sync transfers results back; it does not
merge them into your main branch.

## Follow the development lifecycle

Direct individual agent runs yourself, or use the optional orchestration skill
to coordinate triage, planning, implementation, verification, independent
review, and gated merge.

![Issue-to-PR ADLC. AgentBox workers run in OpenShell sandboxes; the host controller coordinates and performs forge operations the workers cannot.]({{ '/assets/adlc.svg' | relative_url }})

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
| Give agents the project's tools and dependencies | [Repository setup]({{ '/repository-setup/' | relative_url }}) |
| Operate AgentBox through my coding assistant | [Example prompts]({{ '/prompts/' | relative_url }}) |
| Coordinate an issue-to-PR workflow | [Optional orchestration]({{ '/orchestration/' | relative_url }}) |
| Look up a command or runtime detail | [CLI reference]({{ '/cli-reference/' | relative_url }}) · [Architecture]({{ '/architecture/' | relative_url }}) |

AgentBox is an experimental local utility, not a supported product release.
