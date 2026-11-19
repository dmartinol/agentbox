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
  - title: Review the results
    id: review-the-results
  - title: Next steps
    id: next-steps
  - title: Open the web GUI
    id: open-the-web-gui
---

Run a first development task on an existing Git checkout, inspect its work,
and bring the changes back for review. The CLI and web GUI share the same state.

## Requirements

- Go 1.26+ to install from source using `go install`.
- Git, Podman, the OpenShell CLI, and a running OpenShell gateway to run agents.
- Node.js and npm only when building AgentBox agent images.

Run `agentbox doctor` to check the local setup.

## Install

Install the source version that matches this documentation. The `v0.2` tag
predates repository setup and orchestrator configuration, so use `@main` for
the features described in those guides:

```sh
go install github.com/dmartinol/agentbox/cmd/agentbox@main
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
  --image localhost/openshell:codex-REPLACE_VERSION-r5 \
  --mode gateway \
  --model exact-model-id \
  --provider my-gateway \
  --base-url https://models.example.com/v1 \
  --api-key-env MODEL_GATEWAY_KEY \
  codex-gateway
```

Replace `REPLACE_VERSION` with the version shown by `agentbox image list`. Make sure the named credential variable is available to the OpenShell gateway process. Leave `--policy` out to create and use the profile's default policy. In the GUI, the profile form shows that default or an existing legacy path read-only; **Use AgentBox default** clears a legacy reference.

New profiles default to `approval: unattended`, which runs without permission
prompts. Add `--approval prompted` when creating a profile if you want
interactive permission prompts. The default applies only at profile creation:
existing profiles and runs keep their recorded approval, and an explicit
`agent start --approval` overrides the profile for one run only.

See [the OpenShell architecture guide]({{ '/architecture/' | relative_url }}) for provider types, direct-provider profiles, gateway configuration, and policy behavior.

Before assigning a build or test task, [prepare the repository environment]({{ '/repository-setup/' | relative_url }})
if the profile image lacks the project's tools. Use that page to author and
apply `.agentbox/config.yaml`; the CLI does not infer it automatically.

## Start an agent

Start a named agent from the registered project and profile:

```sh
agentbox agent start \
  --project demo \
  --profile codex-gateway \
  --gateway openshell \
  --name docs-review \
  --prompt "Review the installation guide against the current commands and propose focused corrections."
```

Use `agentbox agent list` to see its state. `agentbox agent attach <id>` attaches to the existing tmux session; detaching does not finish the agent.

## Review the results

Replace `RUN_ID` with the ID printed at startup:

```sh
agentbox agent logs RUN_ID
agentbox agent diff RUN_ID
agentbox agent sync RUN_ID
agentbox agent show RUN_ID --json
```

The run details identify the local worktree and branch. Review the synced
changes there before integrating them into the main branch. Sync transfers
results from the sandbox copy; it does not merge your main branch. Stop an
active run with `agentbox agent stop RUN_ID` to sync and retain its work before
the sandbox stops.

## Next steps

- [Prepare project tools and network access]({{ '/repository-setup/' | relative_url }}).
- [Explore the agentic development lifecycle]({{ '/adlc/' | relative_url }}).
- [Operate AgentBox through example prompts]({{ '/prompts/' | relative_url }}).
- [Add the optional issue-to-PR workflow]({{ '/orchestration/' | relative_url }}).

## Open the web GUI

```sh
agentbox gui
```

Open the address printed by the command (default `http://127.0.0.1:8080`). Fleet shows agents grouped by project; **Setup** contains providers, profiles, projects, images, gateways, MCP tools, skills, and advanced diagnostics. Policies are managed with `agentbox policy` or the `/api/policies` endpoints; Setup shows a profile's generated default or legacy path read-only and cannot select policies. Each run's derived policy is read-only under **View policy**.
