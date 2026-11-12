---
layout: cli
title: General commands
nav_title: General
command: general
order: 1
subcommands:
  - version
  - doctor
  - gui
---

## version
{: #general-version }

Print the AgentBox version and build metadata.

```text
agentbox version
agentbox --version
agentbox -version
```

`agentbox version` is equivalent to the two flag forms. A release build prints
the version, commit, and build date. A development build may print `dev`, or the
module version recorded by `go install`.

**Example**

```sh
agentbox version
```

## doctor
{: #general-doctor }

Check local dependencies and OpenShell connectivity.

```text
agentbox doctor [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the checks as a JSON array instead of a table. |

The checks cover the platform (the spike targets `darwin/arm64`), the `git`,
`podman`, `openshell`, and `npm` executables, the Podman connection, and the
OpenShell gateway status. A failed check is reported without stopping the
remaining checks; use the output to decide what to install or start.

**Examples**

```sh
agentbox doctor
agentbox doctor --json
```

**JSON shape**

```json
[
  {
    "name": "gateway",
    "ok": true,
    "detail": "..."
  }
]
```

## gui
{: #general-gui }

Start the local web GUI.

```text
agentbox gui [--address HOST:PORT]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--address` | `127.0.0.1:8080` | HTTP listen address for the GUI. |

The GUI talks to the same local state and OpenShell environment as the CLI.
Fleet is the default surface; **Setup** contains providers, profiles, projects,
images, gateways, policies, and diagnostics. The address can be changed to
expose the GUI on another interface, but the default loopback binding is the
intended mode.

**Example**

```sh
agentbox gui --address 127.0.0.1:9090
```
