---
layout: docs
title: AgentBox
permalink: /
nav_sections:
  - title: Start here
    id: start-here
  - title: How AgentBox fits together
    id: how-agentbox-fits-together
---

AgentBox is a deliberately small local CLI and web GUI for configuring and running coding agents in isolated [OpenShell](https://openshell.org/) sandboxes.

It helps you prepare an agent image and model profile, register a Git project, start an isolated agent, and inspect its status and changes. It is a local utility—not a hosted agent service or a permanent fleet-management platform.

<p align="center">
  <img src="{{ '/screenshots/flow.gif' | relative_url }}" alt="AgentBox tour: configure profiles, register projects, start an OpenShell-backed agent, monitor Fleet, and switch between agents and shell sessions in a project workspace" width="100%">
</p>

## Start here

- [Get started](getting-started/) — install AgentBox and start your first agent.
- [CLI reference](cli-reference/) — browse command syntax and options.
- [OpenShell architecture](architecture/) — understand images, providers, profiles, policies, and runs.

## How AgentBox fits together

- **Images** provide the agent runtime.
- **Providers** register model endpoints with OpenShell.
- **Profiles** select an agent, image, provider, model, approval mode, and policy.
- **Projects** identify Git checkouts and their worktree-isolation setting.
- **Agents** run in OpenShell sandboxes with separate state and, by default, separate Git worktrees.

Credentials stay outside AgentBox state: configuration refers to environment-variable names, not secret values. See [the architecture guide](architecture/) before changing provider, profile, policy, or startup configuration.
