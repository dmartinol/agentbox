---
layout: docs
title: Agent commands
nav_title: Agents
command: agent
order: 12
subcommands:
  - start
  - list
  - show
  - attach
  - logs
  - stop
  - sync
  - diff
  - delete
---

Agent runs execute a profile in an isolated OpenShell sandbox, with the
checkout, state, logs, and configuration kept separate per run.

## start
{: #agent-start }

Start an agent run.

```text
agentbox agent start \
  [--project NAME | --source PATH] [--profile NAME] [--gateway NAME] \
  [--name NAME] [--image IMAGE] [--mode MODE] [--model MODEL] \
  [--approval MODE] [--prompt TEXT] [--mcp NAME] [--dry-run] [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--project` | — | Registered project name. |
| `--source` | `.` | Git source checkout, when `--project` is omitted. |
| `--profile` | — | Saved profile name. |
| `--gateway` | — | OpenShell gateway name. |
| `--name` | — | Unique run name. |
| `--image` | — | Image override. |
| `--mode` | — | Mode override. |
| `--model` | — | Model override. |
| `--approval` | — | Approval override. |
| `--prompt` | — | Task prompt; empty starts the agent interactively. |
| `--mcp` | — | MCP tools entry whose agent matches the profile. |
| `--dry-run` | `false` | Preview startup. |
| `--json` | `false` | Print structured start output. |

A run needs a registered project or a source checkout, and a profile or
explicit configuration overrides. On success the command prints the run ID,
agent, and worktree path. `--dry-run` previews the startup plan and returns
JSON output. `--mcp` selects an MCP tools entry; missing server names produce
warnings on standard error, not failures.

**Examples**

```sh
agentbox agent start --profile codex --project demo --prompt "Fix the flaky test"
agentbox agent start --profile codex --project demo --mcp codex-tools
agentbox agent start --profile codex --project demo --dry-run
```

## list
{: #agent-list }

List agent runs.

```text
agentbox agent list [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the run records as JSON. |

Reconciles run states with the sandbox environment, then prints one line per
run with its name, ID, project, agent, isolation, mode, status, and worktree.

## show
{: #agent-show }

Show one run.

```text
agentbox agent show [--json] ID
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the run record as JSON. |

## attach
{: #agent-attach }

Attach to a run's live terminal session.

```text
agentbox agent attach ID
```

Detaching from the session does not finish the run.

## logs
{: #agent-logs }

Print a run's captured logs.

```text
agentbox agent logs ID
```

## stop
{: #agent-stop }

Stop a run.

```text
agentbox agent stop ID
```

Stops the run's sandbox. The run record and its results are kept.

## sync
{: #agent-sync }

Sync a run's results into its local worktree.

```text
agentbox agent sync ID
```

Copies the run's committed branch, uncommitted changes, and untracked files
from the sandbox back into the local worktree. Sync refuses to overwrite a
worktree that changed since the last sync.

## diff
{: #agent-diff }

Print a run's changes.

```text
agentbox agent diff ID
```

Prints the run's diff against its baseline commit, followed by its untracked
files.

## delete
{: #agent-delete }

Delete a run.

```text
agentbox agent delete [--discard] ID
```

| Flag | Default | Description |
| --- | --- | --- |
| `--discard` | `false` | Acknowledge discarding the agent's results. |

Deletes the run record and its sandbox. Use `--discard` to confirm deleting a
run whose results have not been synced.
