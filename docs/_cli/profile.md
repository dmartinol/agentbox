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
default model, legacy policy reference, and approval behavior. Profiles are
referenced by name from `agent start` and from the GUI.

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
| `--policy` | profile default | Deprecated legacy policy path. Only network rules are added to the derived run policy. |
| `--approval` | `unattended` | `prompted` or `unattended`; unattended runs without permission prompts. |
| `--base-url` | — | Gateway model API base URL. |
| `--api-key-env` | — | Environment-variable **name** holding the API key. |

Gateway-mode profiles require an exact model ID and an OpenShell provider.
If `--policy` is omitted, AgentBox creates and references the profile's
generated-default marker under the policies directory; that file need not exist
when the run starts. `--api-key-env` takes an
environment-variable name, never a key value. On success the command prints
the saved profile path.

Every run policy starts from the AgentBox default, adds a binary-scoped
provider rule, then adds network rules from a non-generated legacy policy and
the active repository fragment. Legacy files cannot replace the default
filesystem, process, or hard Landlock sections.

New profiles default to `approval: unattended`. This default applies only when
creating a profile; existing profiles keep the approval stored in their YAML,
and existing runs keep their recorded approval snapshot. Pass
`--approval prompted` to create a profile that asks for permission before
actions. At run start, `agent start --approval prompted|unattended` overrides
the selected profile for that run only. Unattended runs still require an
effective policy with `hard_requirement` Landlock compatibility and
`network_policies`; AgentBox refuses to start the agent otherwise.

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
