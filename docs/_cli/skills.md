---
layout: docs
title: Usage skill commands
nav_title: Usage skills
command: skills
order: 2
subcommands:
  - install
  - list
  - show
---

`agentbox skills` installs AgentBox's own usage skills for host agents. The
skills are embedded in the binary, carry the running version in their front
matter, and are copied to the conventional skill directory of the selected
harness. No network access, registry, or package manager is involved.

This command is separate from [`agentbox skill`]({{ '/cli-reference/skill/' | relative_url }}),
which selects host skills and plugin bundles to deliver **into** an agent's
sandbox. `agentbox skills` documents how to drive AgentBox itself.

Two skills are shipped:

| Skill | Covers |
| --- | --- |
| `agentbox-cli` | Starting, inspecting, attaching to, syncing, and deleting runs; project, profile, and policy basics; the blocked-host recovery path. |
| `agentbox-api` | The local HTTP API at `http://127.0.0.1:8080/api/...` with `curl`, plus a map of the Fleet and Setup surfaces. |

## install
{: #skills-install }

Install the embedded skills.

```text
agentbox skills install [--target agents|codex|claude|all] [--dir PATH] [--force] [--dry-run]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--target` | `all` | Harness directory to install into. `all` installs into every built-in target whose parent directory exists. |
| `--dir` | — | Override the target root entirely. Both skills are written once beneath this directory. Cannot be combined with `--target all`. |
| `--force` | `false` | Overwrite an existing `SKILL.md` whose content differs. |
| `--dry-run` | `false` | Report the planned writes without changing the filesystem. |

Built-in targets:

| Target | Destination |
| --- | --- |
| `agents` | `~/.agents/skills/agentbox-<name>/SKILL.md` |
| `codex` | `~/.codex/skills/agentbox-<name>/SKILL.md` |
| `claude` | `~/.claude/skills/agentbox-<name>/SKILL.md` |
| `opencode` | `~/.config/opencode/skills/agentbox-<name>/SKILL.md` |

Re-running `install` is idempotent: files that already match the embedded
content are reported as `unchanged`. A differing file is never overwritten
without `--force`; the command reports the path and exits non-zero after
finishing the other writes. A built-in target whose parent directory is absent
is reported and skipped rather than created, which keeps AgentBox from adding
directories for harnesses you do not use. An explicit `--dir` is created.

**Examples**

```sh
agentbox skills install
agentbox skills install --target codex
agentbox skills install --target all --dry-run
agentbox skills install --dir ./project/.skills --force
```

## list
{: #skills-list }

Show the embedded version of each skill and the copies found on disk.

```text
agentbox skills list [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the status as JSON. |

Each installation is reported as `current`, `stale`, `missing`, or
`unreadable`. `list` only reads, so it works when the home directory is
read-only or unavailable.

## show
{: #skills-show }

Print one embedded skill to standard output with the version substituted.

```text
agentbox skills show NAME
```

Valid names are `agentbox-cli` and `agentbox-api`. `show` performs no writes and
does not need a home directory.

**Example**

```sh
agentbox skills show agentbox-cli
```

`agentbox doctor` also reports installed usage skills as `current`, `stale`, or
absent, and points at `agentbox skills install --force` when a copy is stale.
