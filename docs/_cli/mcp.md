---
layout: cli
title: MCP commands
nav_title: MCP
command: mcp
order: 9
subcommands:
  - list
  - show
  - create
  - delete
  - servers
---

An MCP tools entry selects global MCP servers **by name** for one agent.
Definitions stay in the agent's own user-scope configuration on the host
(`~/.claude.json`, `~/.codex/config.toml`, or
`~/.config/opencode/opencode.json(c)`); AgentBox never stores them. Selected
servers are delivered into a run at user scope; project-local MCP files always
ride along and win name conflicts. Credentials must be environment-variable
references — inline secret-like values fail run start.

## list
{: #mcp-list }

List MCP tools entries.

```text
agentbox mcp list [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the entries as JSON. |

Prints one line per entry with its name, agent, and selected server names.

## show
{: #mcp-show }

Show one MCP tools entry.

```text
agentbox mcp show [--json] NAME
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the entry as JSON. |

## create
{: #mcp-create }

Create an MCP tools entry.

```text
agentbox mcp create --agent AGENT --servers NAME[,NAME...] NAME
```

| Flag | Default | Description |
| --- | --- | --- |
| `--agent` | — | Required. Agent the entry selects servers for: `claude`, `codex`, or `opencode`. |
| `--servers` | — | Required. Comma-separated global MCP server names. |

**Example**

```sh
agentbox mcp create --agent codex --servers github,slack codex-tools
```

## delete
{: #mcp-delete }

Delete an MCP tools entry.

```text
agentbox mcp delete NAME
```

Deleting an entry removes only the selection; the server definitions stay in
the agent's own configuration.

## servers
{: #mcp-servers }

List the global MCP servers available for selection.

```text
agentbox mcp servers --agent AGENT [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--agent` | — | Required. Agent to list global MCP server names for. |
| `--json` | `false` | Print the names as JSON. |

Use this list to find valid names for `mcp create --servers` and
`agent start --mcp`.
