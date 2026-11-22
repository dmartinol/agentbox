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
  - denials
  - sleep
  - resume
  - finish
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
run with its name, ID, project, agent, isolation, mode, derived status (with its
detail when present), and worktree.

Status is derived and read-only: `creating`, `ready`, `working`, `waiting`,
`failed`, `unreachable`, `stopped`, or `completed`.

## show
{: #agent-show }

Show one run.

```text
agentbox agent show [--json] ID
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the run record as JSON. |

Inspects the sandbox before printing, so the status and detail reflect the
current state. The JSON record includes `status`, `status_detail`, and the
derived `sync_state` (`no_changes`, `up_to_date`, `needs_sync`, `blocked`, or
`unknown`).

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

## denials
{: #agent-denials }

Show a run's network denials and allow them interactively.

```text
agentbox agent denials [--json] ID
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the denials as JSON, using the same schema as the run logs API. |

Reads the run's sandbox policy log and prints one row per denied network
connection: host, port, calling binary, and deny reason. Rows that cannot be
turned into a host rule are marked `invalid` with the reason.

When the run has at least one valid denial and standard input is a terminal,
the command ends with one prompt:

```text
Allow which? (e.g. 1,2 · all · Enter to skip):
```

Answer with row numbers (`1`, `1,3`) or `all` to add run-only host rules for
the selected denials through the same verified flow as the Fleet **Add
host** action. Each rule prints one outcome line (`live-applied`,
`static-pending`, `no-live-sandbox`, or `openshell-rejected`), and the
command exits non-zero when any rule failed. Empty input or end of file
skips.

The rules apply to the current run only. Piped input and `--json` never
prompt: they print the listing and exit. To allow a host without a denial
listing, or to persist a rule for future runs, use
`agentbox policy edit --allow-host` or the GUI's persist option.

## sleep
{: #agent-sleep }

Sync a run's work and stop its sandbox while keeping the run resumable.

```text
agentbox agent sleep [--force] ID
```

| Flag | Default | Description |
| --- | --- | --- |
| `--force` | `false` | Interrupt a Working agent before sleeping. |

Syncs pending work first; a sync failure leaves the sandbox running. A
successful sleep records `stopped` with the detail `asleep; resume available`
and keeps the record, worktree, transcript, and harness session store.

## resume
{: #agent-resume }

Resume a sleeping run or continue an agent that exited.

```text
agentbox agent resume [--no-attach] ID
```

| Flag | Default | Description |
| --- | --- | --- |
| `--no-attach` | `false` | Do not attach after the agent resumes. |

Starts the sandbox when the run is `stopped`, otherwise relaunches the agent in
the existing sandbox. The agent continues its last conversation; when the
harness session store is missing, AgentBox starts a fresh session in the same
worktree and reports that context could not be restored. Pending work is
synced first, and a sync failure leaves the run resumable.

## finish
{: #agent-finish }

End a run deliberately.

```text
agentbox agent finish ID
```

Syncs outstanding work, ends the agent session, and records `completed`. The
run keeps its record, worktree, and transcript, but it cannot be resumed. A
sleeping run is marked completed without restarting its sandbox. Finish refuses
while the agent is `working`; sleep with `--force` first.

## stop
{: #agent-stop }

Deprecated alias for `sleep`.

```text
agentbox agent stop ID
```

Prints a deprecation warning and behaves like `agentbox agent sleep ID`.

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
run whose results have not been synced. The flag must come before the run id:
`agentbox agent delete --discard RUN_ID`. A flag written after the id is read
as a second argument, and the command then waits for confirmation.

Without `--discard`, delete refuses an active run and any run whose derived
`sync_state` is `needs_sync` or `unknown`.
