---
layout: docs
title: Skill commands
nav_title: Skills
command: skill
order: 10
subcommands:
  - list
  - show
  - create
  - delete
  - available
---

A skills entry selects standalone host skills and whole plugin skill bundles
for one agent. AgentBox stores only names; skill folders stay in the agent's
own host directories until run start. Selected `SKILL.md` folders are copied
into the agent's user skills directory inside the sandbox. A plugin is included
or excluded as a whole, and only its bundled skills are delivered.

## list
{: #skill-list }

List skills entries.

```text
agentbox skill list [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the entries as JSON. |

## show
{: #skill-show }

Show one skills entry.

```text
agentbox skill show [--json] NAME
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the entry as JSON. |

## create
{: #skill-create }

Create a skills entry.

```text
agentbox skill create --agent AGENT [--skills NAME[,NAME...]] [--plugins PLUGIN@MARKETPLACE[,...]] NAME
```

| Flag | Default | Description |
| --- | --- | --- |
| `--agent` | — | Required. Agent the entry selects skills for: `claude`, `codex`, or `opencode`. |
| `--skills` | — | Comma-separated standalone skill names. |
| `--plugins` | — | Comma-separated `plugin@marketplace` bundles. |

At least one skill or plugin is required.

**Example**

```sh
agentbox skill create --agent codex --skills orca-cli,orchestration \
  --plugins browser@openai-bundled work-codex
```

## delete
{: #skill-delete }

Delete a skills entry.

```text
agentbox skill delete NAME
```

Deleting an entry removes only the selection; host skill folders are unchanged.

## available
{: #skill-available }

List the skills and plugin bundles available for selection.

```text
agentbox skill available --agent AGENT [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--agent` | — | Required. Agent to discover skills for. |
| `--json` | `false` | Print names, frontmatter descriptions, and plugin bundled-skill counts as JSON. |

Use this list to find valid values for `skill create` and
`agent start --skills`.
