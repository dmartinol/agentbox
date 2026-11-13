---
layout: docs
title: Usage skill commands
nav_title: Usage skills
command: skills
order: 2
subcommands:
  - install
  - update
  - remove
  - list
  - show
---

`agentbox skills` installs AgentBox's own usage skills for host agents and
keeps them current. The skills are embedded in the binary, carry the running
version in their front matter, and are version-matched so an agent can tell
whether its instructions are stale. No network access, registry, or package
manager is involved.

This command is separate from [`agentbox skill`]({{ '/cli-reference/skill/' | relative_url }}),
which selects host skills and plugin bundles to deliver **into** an agent's
sandbox. `agentbox skills` documents how to drive AgentBox itself.

Two skills are shipped:

| Skill | Covers |
| --- | --- |
| `agentbox-cli` | Starting, inspecting, attaching to, syncing, and deleting runs; project, profile, and policy basics; the blocked-host recovery path. |
| `agentbox-api` | The local HTTP API at `http://127.0.0.1:8080/api/...` with `curl`, plus a map of the Fleet and Setup surfaces. |

## Layout: one canonical copy plus links
{: #skills-layout }

Every command manages a single canonical copy of each skill:

| Piece | Location |
| --- | --- |
| Canonical copy | `~/.agents/skills/<name>/SKILL.md` (or `./.agents/skills/<name>/SKILL.md` with `--local`) |
| Claude link | `~/.claude/skills/<name>` → `../../.agents/skills/<name>` (or `./.claude/skills/<name>` with `--local`) |
| OpenCode link | `~/.config/opencode/skills/<name>` → `../../../.agents/skills/<name>` (or `./.opencode/skills/<name>` with `--local`) |
| Manifest | `~/.agents/skills/.agentbox-managed.json` (or `./.agents/skills/.agentbox-managed.json`) |
| Backups | `~/.agents/.agentbox-skill-backups/<timestamp>/` (or `./.agents/.agentbox-skill-backups/<timestamp>/`) |

Claude and OpenCode do not read the shared `.agents/skills` root, so they get
a relative symlink to the canonical copy. **Nothing is ever written into
`~/.codex/skills`**: Codex scans `~/.agents/skills` directly, so a copy or
link there would register every skill twice in a Codex session. Selecting
Codex only enables reporting and the legacy migration described below.

The manifest records, per skill, the version, the `sha256` of the rendered
file, the canonical path, and the links created. `update` and `remove` only
touch paths recorded there, and every forced replacement or removal first
moves the previous content into the timestamped backup directory.

### Relationship to `npx skills`
{: #skills-npx }

The canonical layout is the shared `.agents/skills` convention that
[`npx skills`](https://skills.sh) also reads:

- `npx skills list -g` (and `npx skills list` for a `--local` install) lists
  both AgentBox skills with `Source: untracked` and no parse warnings.
- Because the skills are untracked, `npx skills update` cannot update them;
  run `agentbox skills update` after upgrading AgentBox.
- `npx skills remove` can delete them, and `agentbox skills remove` removes
  exactly the copies and links the manifest records.

## install
{: #skills-install }

Install the embedded skills and create the links for the selected harnesses.

```text
agentbox skills install [--agent a,b|--target T] [--all] [--local] [--dir PATH] [--force] [--dry-run] [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--agent` | detected | Comma-separated harnesses to install for: `claude`, `codex`, `opencode`. Claude and OpenCode receive a link; Codex is reporting-only. |
| `--target` | — | Deprecated alias for `--agent`. `agents` writes only the canonical copy; `codex`, `claude`, `opencode` select one harness; `all` selects all three. |
| `--all` | `false` | Install for every harness regardless of directory detection. |
| `--local` | `false` | Put the canonical root at `./.agents/skills` for the current project and link into `./.claude/skills` and `./.opencode/skills`. |
| `--dir` | — | Escape hatch: write plain copies of both skills under this directory with no links, manifest, or lifecycle. Refused inside `~/.codex/skills`. |
| `--force` | `false` | Replace modified copies and foreign destinations, backing up the previous content first. |
| `--dry-run` | `false` | Report the planned changes without touching the filesystem. |
| `--json` | `false` | Print the result list as JSON. |

A harness counts as detected when its parent directory exists (`~/.claude`,
`~/.config/opencode`, `~/.codex`). Detected defaults never create a harness
directory; an explicit `--agent` or `--all` does. The canonical
`~/.agents/skills` root is always created by `install`.

`install` is idempotent: unchanged copies and correct links are reported as
`unchanged`. Stale copies that still match the manifest are refreshed in
place. A modified copy — anything that differs from what the manifest
recorded and from the embedded skill — is preserved and refused unless
`--force` replaces it after a backup. The same rule protects foreign
destinations: an existing real directory or foreign symlink at a link path is
reported and skipped unless `--force`.

On the first `install` or `update`, AgentBox also migrates copies that older
releases wrote into `~/.codex/skills/agentbox-*`: directories containing
nothing but a byte-identical `SKILL.md` are removed; anything else is
reported and left alone unless `--force` removes it after a backup.

**Examples**

```sh
agentbox skills install
agentbox skills install --agent claude,opencode
agentbox skills install --target codex        # deprecated; canonical copy + Codex reporting
agentbox skills install --local
agentbox skills install --dir ./project/.skills
agentbox skills install --force
```

## update
{: #skills-update }

Refresh the managed copies after upgrading AgentBox, and recreate their
links.

```text
agentbox skills update [--agent a,b] [--all] [--local] [--force] [--dry-run] [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--agent` | detected | Comma-separated harnesses to relink for. |
| `--all` | `false` | Relink for every harness regardless of detection. |
| `--local` | `false` | Update the project-scoped `./.agents/skills` root. |
| `--force` | `false` | Replace modified copies after a backup. |
| `--dry-run` | `false` | Report the planned changes without touching the filesystem. |
| `--json` | `false` | Print the result list as JSON. |

`update` refreshes copies that are `current` or `stale` and skips copies that
are `missing` (run `install` for those). Modified copies are skipped unless
`--force` replaces them after a backup. Dangling links are repaired without
`--force`, and the legacy `~/.codex/skills` migration runs on `update` as
well.

## remove
{: #skills-remove }

Delete the copies and links recorded in the manifest.

```text
agentbox skills remove [--agent a,b] [--all] [--local] [--force] [--dry-run] [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--agent` | — | Remove only the links of these harnesses; the canonical copies and the other links stay. |
| `--all` | `false` | Remove every recorded copy and link (same as the default). |
| `--local` | `false` | Remove from the project-scoped `./.agents/skills` root. |
| `--force` | `false` | Remove modified copies after a backup. |
| `--dry-run` | `false` | Report the planned removals without touching the filesystem. |
| `--json` | `false` | Print the result list as JSON. |

`remove` deletes only manifest-recorded paths. A canonical copy that differs
from both the manifest hash and the embedded skill is refused unless
`--force` removes it after a backup. Foreign files and directories sitting at
recorded link paths are always left alone. When the manifest runs empty, the
manifest file itself is removed. `remove --agent codex` is a no-op by design:
Codex reads the canonical copy directly, so there is no Codex link to remove.

## list
{: #skills-list }

Show the embedded version of each skill and the state of every copy on disk.

```text
agentbox skills list [--local] [--json]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--local` | `false` | Read the project-scoped `./.agents/skills` root. |
| `--json` | `false` | Print the status as JSON. |

States are derived from the `sha256` of the installed file, not its version
line, so a hand-edited copy that keeps the version line still reports as
`modified`:

| Row | State | Meaning |
| --- | --- | --- |
| `agents` | `current` | Byte-identical to the embedded skill of this binary. |
| `agents` | `stale` | Byte-identical to the manifest record, but the binary is newer. |
| `agents` | `modified` | Differs from both the manifest and the embedded skill. |
| `agents` | `invalid` | Front matter does not parse as strict YAML. |
| `agents` | `missing` | No canonical copy. |
| `claude`, `opencode` | `current` | Link points at the canonical copy and a skill file is reachable through it. |
| `claude`, `opencode` | `broken-link` | Symlink resolves nowhere or has no skill behind it. |
| `claude`, `opencode` | `foreign` | A real directory, file, or foreign symlink occupies the link path. |
| `claude`, `opencode` | `missing` | No link. |
| `codex` | `legacy` | Leftover copy in `~/.codex/skills` from an older release. |
| `codex` | `missing` | No leftover copy (the normal state; Codex reads the canonical root). |

`list` only reads, so it works when the home directory is read-only or
unavailable.

## show
{: #skills-show }

Print one embedded skill to standard output with the version substituted.

```text
agentbox skills show NAME [--json]
```

`--json` prints `{name, version, description, content}` instead of the raw
markdown. Valid names are `agentbox-cli` and `agentbox-api`. `show` performs
no writes and does not need a home directory.

**Example**

```sh
agentbox skills show agentbox-cli
agentbox skills show --json agentbox-api
```

## JSON output
{: #skills-json }

`install`, `update`, and `remove` print a stable array of actions:

```json
[
  {
    "skill": "agentbox-cli",
    "target": "claude",
    "path": "/home/user/.claude/skills/agentbox-cli",
    "action": "linked",
    "detail": ""
  }
]
```

`target` is `agents` (canonical copy), a harness name (`claude`, `opencode`,
`codex`), or `dir` for `--dir` installs. `action` is one of `installed`,
`updated`, `unchanged`, `linked`, `migrated`, `removed`, or `skipped`, with
`would-` prefixed counterparts in dry runs. `list` prints the status tree
described above, and `show` prints the single-skill object.

`agentbox doctor` reports the same states: stale copies, modified copies,
invalid front matter, broken links, and legacy Codex copies each come with
the exact command that fixes them.
