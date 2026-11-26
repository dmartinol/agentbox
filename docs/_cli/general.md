---
layout: docs
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
`podman`, `openshell`, and `npm` executables, the Podman connection, the
OpenShell gateway status, and the installed usage skills.

Doctor also compares host UTC with the Podman VM clock. The signed delta is host UTC minus VM UTC, so `+27s` means the VM is 27 seconds behind and `-27s` means it is 27 seconds ahead. Differences from zero through two seconds are treated as routine jitter and reported as `ok`; differences above two seconds are reported as `fail`.

A probe timeout, Podman command failure, or invalid RFC3339 response is reported as `unavailable`; it never reports `ok` for an unverified clock. When `openshell sandbox create` fails with a `session token was issued in the future` provisioning failure or a `ContainerExited` cause and the clock check is non-OK or unavailable, the failed run detail prioritizes the clock diagnosis and the exact `agentbox doctor` command before appending as much of the original error as the status-detail limit allows.

For `doctor --json`, every check retains `ok` and adds a `state` field containing `ok`, `fail`, or `unavailable`.

The skills check
compares every installed usage skill and its templates with the running binary
and reports stale, modified, invalid, unreadable-canonical, missing-template,
unreadable-template, broken-link, and legacy states. Each problem names the
exact `agentbox skills update` or `agentbox skills install` command that repairs
it; forced replacements are backed up first. A regular file, a dangling
symlink, or a symlink to another directory at a canonical skill path is
reported as unreadable and repaired with `agentbox skills install --force`.
The repair moves the link or blocker entry itself into the backup and leaves a
symlink target untouched; `update --force` backs the blocker up but then skips
the skill. A failed check is reported without stopping the remaining checks;
use the output to decide what to install or start. After repairing an installed
agent role, restart the harness session: harnesses scan agent directories only
when a session starts, so a running session keeps the old role until then.

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
images, gateways, MCP tools, skills, and advanced diagnostics. Legacy policy
files are managed with `agentbox policy` or the `/api/policies` endpoints, and
each run's derived policy is read-only under **View policy**. The address can be changed to
expose the GUI on another interface, but the default loopback binding is the
intended mode.

**Example**

```sh
agentbox gui --address 127.0.0.1:9090
```
