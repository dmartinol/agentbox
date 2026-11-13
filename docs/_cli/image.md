---
layout: docs
title: Image commands
nav_title: Images
command: image
order: 6
subcommands:
  - versions
  - build
  - list
  - push
  - pull
  - registry show
  - registry set
  - registry clear
  - registry discover
  - registry push
---

AgentBox images package one agent (`codex`, `claude`, or `opencode`) with its
runtime. Builds are revisioned and pinned to a base-image digest. Run
`agentbox image` with no arguments for the built-in usage summary.

## versions
{: #image-versions }

Print the agent version a build would use.

```text
agentbox image versions [--json] AGENT
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the agent and proposed version as JSON. |

Prints the latest official agent release proposed for a build.

**Example**

```sh
agentbox image versions codex
```

## build
{: #image-build }

Build an agent image.

```text
agentbox image build [--version VERSION] [--dry-run] AGENT
```

| Flag | Default | Description |
| --- | --- | --- |
| `--version` | proposed latest | Exact agent version to build. |
| `--dry-run` | `false` | Print the planned build without running it. |

Builds a revisioned AgentBox image for one agent. The base image is pinned to
a digest, and the result is tagged `localhost/openshell:<agent>-<version>-r<N>`.
The build result is printed as JSON.

**Examples**

```sh
agentbox image build --version 0.167.0 codex
agentbox image build --dry-run codex
```

## list
{: #image-list }

List locally built AgentBox images.

```text
agentbox image list [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the raw image listing as JSON. |

Prints each image's agent, version, recipe revision, platform, and reference.

## push
{: #image-push }

Push a local image to an explicit remote reference.

```text
agentbox image push [--dry-run] LOCAL_IMAGE REMOTE_REFERENCE
```

| Flag | Default | Description |
| --- | --- | --- |
| `--dry-run` | `false` | Print the planned push without running it. |

## pull
{: #image-pull }

Pull a remote image.

```text
agentbox image pull [--dry-run] REMOTE_REFERENCE
```

| Flag | Default | Description |
| --- | --- | --- |
| `--dry-run` | `false` | Print the planned pull without running it. |

The `registry` subcommands configure an optional external registry used by
`registry discover` and `registry push`. Running `agentbox image registry` with
no subcommand is equivalent to `registry show`. Registry operations use
Podman's existing registry login.

## registry show
{: #image-registry-show }

Print the configured external registry.

```text
agentbox image registry show [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the registry configuration as JSON. |

Prints the registry reference, or a notice when no external registry is
configured.

## registry set
{: #image-registry-set }

Configure the external registry.

```text
agentbox image registry set REGISTRY
```

## registry clear
{: #image-registry-clear }

Remove the configured external registry.

```text
agentbox image registry clear
```

## registry discover
{: #image-registry-discover }

List AgentBox images available in the configured registry.

```text
agentbox image registry discover [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the discovered images as JSON. |

Prints each image's agent, version, recipe revision, platform, and reference.

## registry push
{: #image-registry-push }

Push a local AgentBox image to the configured registry.

```text
agentbox image registry push [--dry-run] LOCAL_IMAGE
```

| Flag | Default | Description |
| --- | --- | --- |
| `--dry-run` | `false` | Print the planned push without running it. |

`LOCAL_IMAGE` must match a locally built AgentBox image reference; the remote
reference is derived from the configured registry. Fails when no registry is
configured.
