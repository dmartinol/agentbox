---
layout: docs
title: Orchestrator commands
nav_title: Orchestrator
command: orchestrator
order: 13
subcommands:
  - config path
  - config show
  - config validate
  - gate pr-body
  - gate pr-merge
---

`agentbox orchestrator config` reads the user-owned registry that selects the
controller and per-stage worker runtimes, profiles, models, and efforts for the
shipped `agentbox-orchestrator` skill. The registry lives outside project
repositories at `$AGENTBOX_HOME/orchestration.yaml` (default
`~/.agentbox/orchestration.yaml`).

The registry never contains credentials. Refer to credential environment
variables by name only, and start the GUI, gateway, and agent processes in an
environment where those variables exist.

`agentbox orchestrator gate` runs the offline pull request gates the skill calls
before dispatch and before merge. Both gates read files from the host, print a
single confirmation line on success, and exit non-zero on the first failing
check.

The shipped skill is the consumer of these commands. It resolves the registry
with `config show --json`, reports the selected configuration and inherited
sources, shows prerequisites, and waits for approval when the resolved
configuration is complete, unless the prompt explicitly says **“use the
existing configuration.”** If the registry is absent or leaves required
choices open, the skill asks for the missing values and saves the completed
configuration at the selected scope.

## Creating or updating a configuration
{: #orchestrator-config-authoring }

A prompt that asks to create or change the configuration — for example “let's
create the orchestration config for this project” — is a configuration
authoring request, not a workflow. The shipped skill interviews for every
required setting and saves the answers at the scope the prompt names.

The registry file itself is always the user-owned file under
`$AGENTBOX_HOME`. “Project scope” means a `projects[]` entry in that file, not
a file inside the repository and not a value in the global layer. Write every
confirmed answer into the project entry, even when it matches an inherited
default, so the entry records the choice instead of depending on the shared
layer.

- **Project scope** is used for “for this project”, “project config”, or a
  named repository. Save the answers under a `projects[]` entry as inline
  `orchestration` and `workers` overrides. Do not put them in `global`,
  `configs[default]`, or a named config another project or `global.config`
  already references.
- **Global scope** is written only when the user asks for it. Save it under
  `global`, or as the named default.

A project-only registry can keep the default empty, so no other repository
inherits the answers:

```yaml
version: 1
default: base

configs:
  base: {}

projects:
  - name: agentbox
    remote: github.com/owner/agentbox
    orchestration:
      runtime: harness
      agent: codex
      model: <orchestrator-model-id>
      effort: high
    workers:
      default:
        runtime: orca
        agent: codex
        profile: enmaas
        model: <worker-model-id>
        effort: high
      groups:
        coding: {model: <coding-model-id>}
      stages:
        review: {agent: claude, model: <reviewer-model-id>, effort: high}
```

Set `remote` when the repository has an origin, so every worktree matches; an
Orca-managed worktree usually sits outside the main checkout. Add `root` only
for a checkout-specific entry or when the repository has no remote.

After writing, validate and re-resolve with the same `--project` and
`--remote`. A project request is complete only when the output reports
`"scope": "project"` and `"project": "<name>"`, and every answered selection
reports a `project:<name>` source. A value that still resolves from `default`
or `global` means the project layer missed it; fix the entry instead of
reporting success.

The `agentbox` binary must include `agentbox orchestrator`. If the command is
unknown, install or build the current AgentBox binary before editing the
registry. The skill stops rather than guessing the path or skipping
validation.

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
ambiguous registry is an error. When `--project` or `--remote` is supplied and
no project entry matches, the result keeps the `default` or `global` scope and
adds a warning that names the unmatched root and remote. A project request
must treat that warning as a failure rather than accepting the fallback.

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

## gate pr-body
{: #orchestrator-gate-pr-body }

Check a pull request body against its issue and, when the reviewed head is
known, against the diff at that head.

```text
agentbox orchestrator gate pr-body --pr-body PATH --issue-body PATH --issue N [--head SHA [--base REF [--repo DIR]]]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--pr-body` | — | Pull request body file. Required. |
| `--issue-body` | — | Issue body file whose acceptance criteria must match. Required. |
| `--issue` | — | Issue number the body must close with `Closes #N`. Required. |
| `--head` | — | Reviewed head SHA. Enables the Evidence row check. |
| `--base` | — | Base ref of the reviewed diff. Requires `--head` and enables the Size row check. |
| `--repo` | `.` | Checkout used for `git diff --numstat <base>..<head>`. |

The gate always checks the provenance marker, the acceptance criteria list, and
the `Closes` line. With `--head`, every data row of the `## Evidence` table must
name that head in its Commit cell as the full 40-character SHA or an unambiguous
prefix of at least seven hexadecimal characters; a stale or foreign commit
fails. With `--base` as well, the `Size` row must agree with
`git diff --numstat <base>..<head>` on the file count, insertions, and
deletions. The commit count in that row is not checked. The two
`Independent checks` rows are never read by these two checks.

The gate prints `pr-body: valid` and exits 0 on success, or prints the first
failing field and exits non-zero.

**Example**

```sh
agentbox orchestrator gate pr-body --pr-body /tmp/pr-body.md \
  --issue-body /tmp/issue-body.md --issue 215 \
  --head "$(git rev-parse HEAD)" --base origin/main
```

## gate pr-merge
{: #orchestrator-gate-pr-merge }

Check the body about to merge against the reviewed body and the merge head.

```text
agentbox orchestrator gate pr-merge --pr-body PATH --head SHA [--reviewed-body PATH --reviewed-body-sha256 HEX64]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--pr-body` | — | Pull request body file. Required. |
| `--head` | — | Merge head SHA. Required; must be the full 40-character commit SHA. |
| `--reviewed-body` | — | Durable copy of the reviewed body, taken when review was dispatched. |
| `--reviewed-body-sha256` | — | Exact lowercase SHA-256 of the reviewed body file. |

The gate requires the reviewed body and its digest together. It rejects a
digest mismatch, and it compares the canonical forms so only the two
`Independent checks` rows may change between the reviewed body and the current
one. Every `## Evidence` row must name the merge head, and both
`Independent checks` rows must be filled and link only that head.

The gate prints `pr-merge: valid` and exits 0 on success, or prints the first
failing check and exits non-zero.

**Example**

```sh
agentbox orchestrator gate pr-merge --pr-body /tmp/pr-body.md \
  --head 0b4deea8f5fbb846a8419d21db28e73c81ddc065 \
  --reviewed-body ~/.agentbox/orchestrator/workflow/review-1-pr-body.md \
  --reviewed-body-sha256 "$(shasum -a 256 /tmp/reviewed.md | cut -d' ' -f1)"
```
