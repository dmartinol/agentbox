---
layout: docs
title: Policy commands
nav_title: Policies
command: policy
order: 9
subcommands:
  - show
  - edit
---

Policies define sandbox access. These commands inspect or edit the policy
associated with an existing agent run.

## show
{: #policy-show }

Show a run's policy layers.

```text
agentbox policy show [--json] RUN_ID
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the policy layers as JSON. |

Prints the run's `base`, `effective`, and `global` policy layers.

## edit
{: #policy-edit }

Edit a run's policy.

```text
agentbox policy edit [--allow-host HOST] [--binary PATH] [--dry-run] RUN_ID
```

| Flag | Default | Description |
| --- | --- | --- |
| `--allow-host` | — | Add an HTTPS host rule. |
| `--binary` | — | Override the default binary set with one binary path. |
| `--dry-run` | `false` | Print the planned edit without running it. |

Review the effective policy before changing network or binary rules. Network
rules are binary-scoped: a rule for a stdio MCP server must name the server
process binary, not the agent binary.

Without `--binary`, a host rule allows the agent launcher plus the binaries
that actually open the socket: `/usr/local/bin/node`, `/usr/bin/git`, and
`/usr/lib/git-core/**`. The supported agents are Node applications, so a rule
that lists only the launcher path does not cover the connecting process.
