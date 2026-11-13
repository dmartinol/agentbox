---
layout: docs
title: Project commands
nav_title: Projects
command: project
order: 4
subcommands:
  - create
  - edit
  - show
  - list
---

Projects register Git checkouts that agent runs can be started against.

## create
{: #project-create }

Register a project.

```text
agentbox project create [--root PATH] [--no-worktree] NAME
```

| Flag | Default | Description |
| --- | --- | --- |
| `--root` | `.` | Git project root to register. |
| `--no-worktree` | `false` | Run agents directly in the checkout instead of an isolated worktree. |

Projects default to worktree isolation: every run gets its own worktree under
the configured worktree root, so runs never share a checkout. `--no-worktree`
opts out and runs agents directly in the checkout. On success the command
prints the project root.

**Example**

```sh
agentbox project create --root ~/src/demo demo
```

## edit
{: #project-edit }

Edit a project's stored configuration.

```text
agentbox project edit NAME
```

Opens the project's YAML file in `$EDITOR` (default `vi`).

## show
{: #project-show }

Show one project.

```text
agentbox project show [--json] NAME
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the project record as JSON. |

Prints the project name, root, and isolation mode.

## list
{: #project-list }

List registered projects.

```text
agentbox project list [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the project records as JSON. |

Prints one line per project with its name, isolation mode (`worktree` or
`direct`), and root.
