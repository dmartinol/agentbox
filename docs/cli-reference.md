---
layout: page
title: CLI reference
permalink: /cli-reference/
---

The CLI entry point is `agentbox`. Run `agentbox --help` for the top-level command list and `agentbox version` to check the installed build. Flag details below reflect the current command parser; not every subcommand implements its own `--help` output yet.

Unless noted, flags appear before positional arguments. Commands marked `[--json]` support machine-readable output.

## General

```text
agentbox version
agentbox doctor [--json]
agentbox gui [--address HOST:PORT]
```

- `version` prints the version and build metadata.
- `doctor` checks local dependencies and the OpenShell gateway.
- `gui` starts the local web application; the default address is `127.0.0.1:8080`.

## Settings

```text
agentbox settings show [--json]
agentbox settings set [--worktree-root PATH] [--branch-prefix PREFIX]
```

Settings control where isolated worktrees are created and how their branches are named.

## Projects

```text
agentbox project create [--root PATH] [--no-worktree] NAME
agentbox project edit NAME
agentbox project show [--json] NAME
agentbox project list [--json]
```

Projects default to worktree isolation. `--no-worktree` opts out and runs an agent directly in a clean checkout.

## Profiles

```text
agentbox profile create \
  --agent AGENT --image IMAGE \
  [--mode standard|gateway] [--model MODEL] [--provider NAME] \
  [--policy PATH] [--approval prompted|unattended] \
  [--base-url URL] [--api-key-env ENV_NAME] NAME
agentbox profile edit NAME
agentbox profile show [--json] NAME
agentbox profile list [--json]
```

Supported agent values are `codex`, `claude`, and `opencode`. Gateway-mode profiles require an exact model ID and an OpenShell provider. If `--policy` is omitted, AgentBox creates and references the profile's default policy. `--api-key-env` takes an environment-variable **name**, never a key value.

## Images

```text
agentbox image versions [--json] AGENT
agentbox image build [--version VERSION] [--dry-run] AGENT
agentbox image list [--json]
agentbox image push [--dry-run] LOCAL_IMAGE REMOTE_REFERENCE
agentbox image pull [--dry-run] REMOTE_REFERENCE
agentbox image registry show [--json]
agentbox image registry set REGISTRY
agentbox image registry clear
agentbox image registry discover [--json]
agentbox image registry push [--dry-run] LOCAL_IMAGE
```

`AGENT` is `codex`, `claude`, or `opencode`. If `--version` is omitted, `image build` discovers the latest compatible release. Registry operations use Podman's existing registry login.

## Gateways and providers

```text
agentbox gateway list
agentbox gateway connect [--name NAME] [--dry-run] GATEWAY_URL
agentbox gateway start [--dry-run]
agentbox gateway info [--json] [NAME]

agentbox provider setup \
  [--type gateway-bearer|gateway-header|openai|anthropic] \
  [--host HOST] [--credential ENV_NAME] [--gateway NAME] \
  [--dry-run] [--update] NAME
```

Use `gateway-bearer` for OpenAI-compatible gateway APIs and `gateway-header` for Anthropic-compatible gateway APIs. The built-in `openai` and `anthropic` providers are for direct standard-mode profiles. The provider credential flag references an environment-variable name only.

## Policies

```text
agentbox policy show [--json] RUN_ID
agentbox policy edit [--allow-host HOST] [--binary PATH] [--dry-run] RUN_ID
```

Policies define sandbox access. These commands inspect or edit the policy associated with an agent run; review the effective policy before changing network or binary rules. Note that network rules are binary-scoped: a rule for a stdio MCP server must name the server process binary, not the agent binary. `policy edit --allow-host` allows the host for the run's agent and covers its whole executable chain (launcher, `node`, and installed package tree) because OpenShell allowlists by resolved executable path. Pass `--binary` to restrict the rule to one absolute binary path instead.

## MCP tools

```text
agentbox mcp list [--json]
agentbox mcp show [--json] NAME
agentbox mcp create --agent AGENT --servers NAME[,NAME...] NAME
agentbox mcp delete NAME
agentbox mcp servers --agent AGENT [--json]
```

An MCP tools entry selects global MCP servers **by name** for one agent. Definitions stay in the agent's own user-scope configuration on the host (`~/.claude.json`, `~/.codex/config.toml`, or `~/.config/opencode/opencode.json(c)`); AgentBox never stores them. `mcp servers` lists the names available for selection. Selected servers are delivered into a run at user scope; project-local MCP files always ride along and win name conflicts. Credentials must be environment-variable references — inline secret-like values fail run start.

## Agents

```text
agentbox agent start \
  [--project NAME | --source PATH] [--profile NAME] [--gateway NAME] \
  [--name NAME] [--image IMAGE] [--mode MODE] [--model MODEL] \
  [--approval MODE] [--prompt TEXT] [--mcp NAME] [--dry-run] [--json]
agentbox agent list [--json]
agentbox agent show [--json] ID
agentbox agent attach ID
agentbox agent logs ID
agentbox agent stop ID
agentbox agent sync ID
agentbox agent diff ID
agentbox agent delete [--discard] ID
```

`agent start` needs a registered project or source checkout and a profile (or explicit configuration overrides). `--dry-run` previews startup; `--json` prints structured start output. `--mcp` selects an MCP tools entry whose agent matches the run profile; missing server names produce warnings, not failures. `agent delete --discard` acknowledges discarding the agent's results.

## Related guides

- [Getting started](getting-started/)
- [OpenShell architecture](architecture/)
