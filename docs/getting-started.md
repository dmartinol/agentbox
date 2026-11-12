---
layout: docs
title: Getting started
permalink: /getting-started/
nav_sections:
  - title: Requirements
    id: requirements
  - title: Install
    id: install
  - title: Register a project
    id: register-a-project
  - title: Prepare an image and profile
    id: prepare-an-image-and-profile
  - title: Start an agent
    id: start-an-agent
  - title: Open the web GUI
    id: open-the-web-gui
---

This quick start covers the local CLI workflow. The same projects, profiles, and agents are available in the web GUI.

## Requirements

- Go 1.26+ to install from source using `go install`.
- Git, Podman, the OpenShell CLI, and a running OpenShell gateway to run agents.
- Node.js and npm only when building AgentBox agent images.

Run `agentbox doctor` to check the local setup.

## Install

Install the latest published version:

```sh
go install github.com/dmartinol/agentbox/cmd/agentbox@latest
```

Make sure Go's binary directory is on `PATH`, then verify the install:

```sh
agentbox version
agentbox doctor
```

Prebuilt binaries are also available from [GitHub Releases](https://github.com/dmartinol/agentbox/releases).

## Register a project

Register a Git checkout. Worktree isolation is on by default:

```sh
cd /path/to/your/repository
agentbox project create --root . demo
```

Use `--no-worktree` only when you deliberately want the agent to work directly in the checkout; direct-checkout projects require a clean checkout and allow one active agent at a time.

## Prepare an image and profile

Build an agent image (omit `--version` to discover the latest compatible release) and check its exact local reference:

```sh
agentbox image build codex
agentbox image list
```

Create a provider and profile for your model gateway. The credential option names an environment variable; it does not accept the secret itself:

```sh
agentbox provider setup \
  --type gateway-bearer \
  --host models.example.com \
  --credential MODEL_GATEWAY_KEY \
  --gateway openshell \
  my-gateway

agentbox profile create \
  --agent codex \
  --image localhost/openshell:codex-REPLACE_VERSION-r3 \
  --mode gateway \
  --model exact-model-id \
  --provider my-gateway \
  --base-url https://models.example.com/v1 \
  --api-key-env MODEL_GATEWAY_KEY \
  --approval prompted \
  codex-gateway
```

Replace `REPLACE_VERSION` with the version shown by `agentbox image list`. Make sure the named credential variable is available to the OpenShell gateway process. Leave `--policy` out to create and use the profile's default policy.

See [the OpenShell architecture guide](architecture/) for provider types, direct-provider profiles, gateway configuration, and policy behavior.

## Start an agent

Start a named agent from the registered project and profile:

```sh
agentbox agent start \
  --project demo \
  --profile codex-gateway \
  --gateway openshell \
  --name docs-review
```

Use `agentbox agent list` to see its state. `agentbox agent attach <id>` attaches to the existing tmux session; detaching does not finish the agent.

## Open the web GUI

```sh
agentbox gui
```

Open the address printed by the command (default `http://127.0.0.1:8080`). Fleet shows agents grouped by project; **Setup** contains providers, profiles, projects, images, gateways, policies, and diagnostics.
