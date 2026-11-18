---
layout: docs
title: Orchestrator configuration commands
nav_title: Orchestrator configuration
command: orchestrator
order: 13
subcommands:
  - config path
  - config show
  - config validate
---

`agentbox orchestrator config` reads the user-owned registry that selects the
controller and per-stage worker runtimes, profiles, models, and efforts for the
shipped `agentbox-orchestrator` skill. The registry lives outside project
repositories at `$AGENTBOX_HOME/orchestration.yaml` (default
`~/.agentbox/orchestration.yaml`).

The registry never contains credentials. Refer to credential environment
variables by name only, and start the GUI, gateway, and agent processes in an
environment where those variables exist.

The shipped skill is the consumer of these commands. It resolves the registry
with `config show --json`, reports the selected configuration and inherited
sources, shows prerequisites, and waits for approval when the resolved
configuration is complete, unless the prompt explicitly says **“use the
existing configuration.”** If the registry is absent or leaves required
choices open, the skill asks for the missing values and saves the completed
configuration at the selected scope.

## Schema
{: #orchestrator-config-schema }

The schema is versioned and strict. Version 1 rejects unknown fields, unknown
stages, unknown groups, unknown runtimes or agents, and references to missing
configurations.

```yaml
version: 1
default: standard

configs:
  standard:
    orchestration:
      runtime: orca
      agent: codex
      model: <orchestrator-model-id>
      effort: high
    workers:
      default:
        runtime: orca
        agent: codex
        model: <worker-model-id>
        effort: medium
      groups:
        coding:
          effort: high
      stages:
        review:
          agent: claude
          model: <reviewer-model-id>
          effort: high
  heavy:
    workers:
      groups:
        coding:
          model: <heavy-coding-model-id>
          effort: high

global:
  config: standard
  workers:
    groups:
      coding:
        profile: enmaas

projects:
  - name: agentbox
    root: /absolute/path/to/agentbox
    remote: github.com/owner/agentbox
    config: heavy
    workers:
      stages:
        implement:
          effort: high
```

A complete example is in
[`docs/examples/orchestration.yaml`]({{ '/examples/orchestration.yaml' | relative_url }}).
Replace every `<...>` placeholder with a model ID available to the selected
runtime, profile, or provider. A placeholder is documentation, not a suggested
model.

| Field | Meaning |
| --- | --- |
| `version` | Schema version. Must be `1`. |
| `default` | Named configuration used when no `global` or project layer applies. Required, and must exist in `configs`. |
| `configs.<name>` | Named controller and worker selections. Names are lower-case identifiers. |
| `global` | Optional global layer. `config` selects a named configuration; inline `orchestration` and `workers` override it. |
| `projects[]` | Optional repository layers. Each entry needs a unique `name` and a `root`, a `remote`, or both. |
| `orchestration` | Controller selection: `runtime`, `agent`, and optional `profile`, `model`, and `effort`. |
| `workers.default` | Worker selection inherited by every stage in this layer. |
| `workers.groups` | Selections for the fixed stage groups `coding` and `other`. |
| `workers.stages` | Selections for one stage, applied after the group. |

A selection accepts:

| Key | Values |
| --- | --- |
| `runtime` | `orca`, `agentbox`, or `harness`. |
| `agent` | `claude`, `codex`, or `opencode`. |
| `profile` | An AgentBox profile for runtime `agentbox`; a harness profile for `orca` or `harness`. |
| `model` | An exact model ID. Empty clears an inherited model. |
| `effort` | A reasoning-effort value supported by the selected runtime. |

The stage groups are fixed:

| Group | Stages |
| --- | --- |
| `coding` | `plan`, `implement`, `fix` |
| `other` | `triage`, `verify`, `review`, `retro` |

Every stage belongs to exactly one group. A stage entry overrides its group and
group defaults, and a later registry layer overrides an earlier one field by
field.

## Resolution
{: #orchestrator-config-resolution }

Later layers win, field by field:

1. `configs[default]`;
2. `global.config`, when set;
3. inline `global`;
4. the matching project's `config`, when set;
5. the matching project's inline overrides.

Within one layer, a stage resolves `workers.default`, then its group, then
`workers.stages.<stage>`. The resolved output records the source of every
field, for example `global.group:coding` or
`project:agentbox.stage:implement`.

The workflow adds these higher-precedence sources:

1. explicit choices in the current prompt;
2. the matching registry project layer;
3. the project's committed `.github/orchestrator.yaml`;
4. the registry global layer;
5. the registry default configuration;
6. repository documentation and CI;
7. the defaults in the shipped skill.

Prompt choices override the registry for that workflow only.

### Project matching
{: #orchestrator-config-project-matching }

Project entries match the canonical absolute `root` or the normalized
`remote`. A root matches an exact checkout or any path beneath it; the deepest
matching root wins. A remote is normalized to `host/owner/repo`, so
`git@github.com:owner/repo.git`, `https://github.com/owner/repo.git`, and
`ssh://git@github.com/owner/repo` are the same remote.

The resolver never guesses. It returns an error when two entries match at the
same root depth, when more than one entry matches the remote, or when the root
and remote match different entries. Make the roots and remotes unique, or
remove the conflicting entry.

## Prerequisites
{: #orchestrator-config-prerequisites }

The shipped skill reports these before dispatch and treats an unmet
requirement as a blocker instead of substituting another runtime, agent,
profile, or model.

| Runtime | Prerequisites | Model source |
| --- | --- | --- |
| `orca` | The `orca` CLI and the selected agent runtime. A requested harness profile uses the visible `orca terminal create` path because `worker-start` has no profile flag. | Orca exposes no local model catalog. Ask for the exact provider model ID and treat a launch failure as a blocker. |
| `agentbox` | The `agentbox` CLI, an existing AgentBox profile, and the OpenShell gateway that profile uses. AgentBox workers cannot apply a reasoning effort. | `agentbox profile list --json` and `agentbox profile show --json` report the profile model. A registry model that disagrees with the profile is a blocker. |
| `harness` | The selected harness CLI (`claude`, `codex`, or `opencode`) and any harness profile it needs. | Ask for the exact model ID; the supported model set depends on the harness and its profile. |

Credentials stay in the environment. Record only environment-variable names
such as `ENMAAS_KEY` or `OPENAI_API_KEY`; never write a credential value into
the registry, state record, logs, or documentation.

## config path
{: #orchestrator-config-path }

Print the registry path for the current AgentBox state root.

```text
agentbox orchestrator config path
```

`AGENTBOX_HOME` changes the path. The default is
`~/.agentbox/orchestration.yaml`.

**Example**

```sh
agentbox orchestrator config path
```

## config show
{: #orchestrator-config-show }

Resolve the registry for a repository and print the effective selections and
their sources.

```text
agentbox orchestrator config show [--file PATH] [--project ROOT] [--remote URL] [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--file` | registry path | Registry file to resolve. |
| `--project` | — | Repository root used to match a project entry. |
| `--remote` | — | Git remote URL used to match a project entry. |
| `--json` | `false` | Print the resolved result as JSON. |

A missing registry is not an error: the result reports `present: false` and
`scope: none`, and the skill asks for the missing choices. A malformed or
ambiguous registry is an error.

**Examples**

```sh
agentbox orchestrator config show --project "$(git rev-parse --show-toplevel)" \
  --remote "$(git remote get-url origin 2>/dev/null)" --json
agentbox orchestrator config show
```

**JSON shape**

```json
{
  "path": "/Users/example/.agentbox/orchestration.yaml",
  "present": true,
  "version": 1,
  "default": "standard",
  "scope": "project",
  "project": "agentbox",
  "sources": ["default:standard", "global", "project:agentbox"],
  "orchestrator": {
    "runtime": "orca",
    "agent": "codex",
    "model": "<orchestrator-model-id>",
    "effort": "high",
    "source": "project:agentbox",
    "sources": {"model": "project:agentbox", "runtime": "global"}
  },
  "stages": {
    "implement": {
      "runtime": "orca",
      "agent": "codex",
      "profile": "enmaas",
      "model": "<heavy-coding-model-id>",
      "effort": "high",
      "source": "project:agentbox.stage:implement",
      "sources": {"model": "project:agentbox.group:coding"}
    }
  },
  "warnings": []
}
```

## config validate
{: #orchestrator-config-validate }

Validate the schema without resolving a project.

```text
agentbox orchestrator config validate [--file PATH] [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--file` | registry path | Registry file to validate. |
| `--json` | `false` | Print `{"path": ..., "valid": true}` as JSON. |

A missing file is an error so a save step cannot silently validate the wrong
path. Invalid YAML, an unknown field, an unknown stage or group, an unsupported
runtime or agent, and a missing configuration reference fail with the exact
path in the registry.

**Example**

```sh
agentbox orchestrator config validate
agentbox orchestrator config validate --file ./orchestration.yaml --json
```
