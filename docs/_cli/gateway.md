---
layout: cli
title: Gateway commands
nav_title: Gateways
command: gateway
order: 6
subcommands:
  - list
  - connect
  - start
  - info
---

Gateways are OpenShell gateways that host agent sandboxes. Runs reference a
gateway by name.

## list
{: #gateway-list }

List registered gateways.

```text
agentbox gateway list
```

Queries the local OpenShell environment and prints the known gateways.

## connect
{: #gateway-connect }

Register an OpenShell gateway.

```text
agentbox gateway connect [--name NAME] [--dry-run] GATEWAY_URL
```

| Flag | Default | Description |
| --- | --- | --- |
| `--name` | — | Name to register the gateway under. |
| `--dry-run` | `false` | Print the planned registration without running it. |

**Example**

```sh
agentbox gateway connect --name office https://gateway.office.example.com
```

## start
{: #gateway-start }

Start a local OpenShell gateway.

```text
agentbox gateway start [--dry-run]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--dry-run` | `false` | Print the planned startup without running it. |

## info
{: #gateway-info }

Show gateway details.

```text
agentbox gateway info [--json] [NAME]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the raw gateway details as JSON. |

Shows details for the named gateway, or the default gateway when `NAME` is
omitted. Without `--json` the response is pretty-printed.
