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

Policies define sandbox access. These commands inspect or edit the derived
policy associated with an existing agent run. The run record includes policy
provenance; run copies are not reusable templates.

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
agentbox policy edit [--allow-host HOST --port PORT --binary PATH] [--dry-run] RUN_ID
```

| Flag | Default | Description |
| --- | --- | --- |
| `--allow-host` | — | Add one exact host rule. Requires `--binary`. |
| `--port` | `443` | Port for `--allow-host`. |
| `--binary` | — | Exact absolute path of the binary that made the denied request. |
| `--dry-run` | `false` | Print the planned edit without running it. |

Without flags, `policy edit` opens the run policy in `$EDITOR`. Review the
effective policy before changing network or binary rules. Network rules are
binary-scoped: a rule for a stdio MCP server must name the server process
binary, not the agent binary.

Host rules are denial-scoped. List the run's denials with
`agentbox agent denials RUN_ID` (or read
`openshell logs <sandbox> --source sandbox`) and copy the host, port, and
binary from the row to allow, for example:

```sh
agentbox policy edit --allow-host api.example.com --port 443 \
  --binary /usr/bin/curl RUN_ID
```

Wildcards, directory paths, invalid ports, and secret-like values are refused.
The secret-like check exempts binary paths from its opaque-run (entropy)
heuristic, so long hyphenated paths such as
`/usr/local/uv-aarch64-unknown-linux-gnu/uv` are accepted; embedded credential
strings are still refused. If the same host and port already has a rule, only
the missing binary is added.

The edit writes to the run's own policy copy under
`$AGENTBOX_HOME/policies`, never to the reusable profile configuration. A
static change is saved as `*-pending.yaml` for the next sandbox. The CLI does
not persist the rule in `.agentbox/config.yaml`; use the GUI's **Persist in
.agentbox/config.yaml** option when the rule should apply to future runs.
