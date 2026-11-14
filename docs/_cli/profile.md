---
layout: docs
title: Profile commands
nav_title: Profiles
command: profile
order: 5
subcommands:
  - create
  - edit
  - show
  - list
---

Profiles are reusable run configurations: the agent, its image, provider mode,
default model, sandbox policy, and approval behavior. Profiles are referenced
by name from `agent start` and from the GUI.

## create
{: #profile-create }

Create a profile.

```text
agentbox profile create \
  --agent AGENT --image IMAGE \
  [--mode standard|gateway] [--model MODEL] [--provider NAME] \
  [--policy PATH] [--approval prompted|unattended] \
  [--base-url URL] [--api-key-env ENV_NAME] NAME
```

| Flag | Default | Description |
| --- | --- | --- |
| `--agent` | — | Required. `codex`, `claude`, or `opencode`. |
| `--image` | — | Required. Local image reference built with `image build`. |
| `--mode` | `standard` | `standard` or `gateway`. |
| `--model` | — | Exact default model ID. |
| `--provider` | — | OpenShell provider name. |
| `--policy` | profile default | Sandbox policy path. |
| `--approval` | `prompted` | `prompted` or `unattended`. |
| `--base-url` | — | Gateway model API base URL. |
| `--api-key-env` | — | Environment-variable **name** holding the API key. |

Gateway-mode profiles require an exact model ID and an OpenShell provider.
If `--policy` is omitted, AgentBox creates and references the profile's
default policy under the policies directory. `--api-key-env` takes an
environment-variable name, never a key value. On success the command prints
the saved profile path.

**Example**

```sh
agentbox profile create --agent codex \
  --image localhost/openshell:codex-0.167.0-r4 codex
```

## edit
{: #profile-edit }

Edit a profile.

```text
agentbox profile edit NAME
```

Opens the profile's YAML file in `$EDITOR` (default `vi`).

## show
{: #profile-show }

Show one profile.

```text
agentbox profile show [--json] NAME
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the profile record as JSON. |

## list
{: #profile-list }

List saved profiles.

```text
agentbox profile list [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Print the profile records as JSON. |

Prints one line per profile with its name, agent, mode, and model.
