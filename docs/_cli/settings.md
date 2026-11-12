---
layout: docs
title: Settings commands
nav_title: Settings
command: settings
order: 2
subcommands:
  - show
  - set
---

Settings control where AgentBox creates isolated Git worktrees and how run
branches are named. Running `agentbox settings` without a subcommand is
equivalent to `settings show`.

## show
{: #settings-show }

Print the effective settings.

```text
agentbox settings show [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the settings as JSON. |

Prints the worktree root and branch prefix. Effective settings merge the
built-in defaults with `~/.agentbox/settings.yaml`.

**Example**

```sh
agentbox settings show
agentbox settings show --json
```

## set
{: #settings-set }

Change global settings.

```text
agentbox settings set [--worktree-root PATH] [--branch-prefix PREFIX]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--worktree-root` | current value | Root directory for isolated Git worktrees. |
| `--branch-prefix` | current value | Branch namespace for run branches. |

Omitted flags keep their current values; the command writes the resulting
settings to `~/.agentbox/settings.yaml`.

**Example**

```sh
agentbox settings set --worktree-root ~/.agentbox/worktrees --branch-prefix agentbox/
```
