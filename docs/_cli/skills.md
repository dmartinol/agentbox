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

Three skills are shipped:

| Skill | Covers |
| --- | --- |
| `agentbox-cli` | Starting, inspecting, attaching to, syncing, and deleting runs; project, profile, and policy basics; the blocked-host recovery path. |
| `agentbox-api` | The local HTTP API at `http://127.0.0.1:8080/api/...` with `curl`, plus a map of the Fleet and Setup surfaces. |
| `agentbox-orchestrator` | The issue-to-PR orchestrator flow: triage, a selected worker per stage with the agent, model, and effort it actually applied (`not-applicable` for AgentBox workers), isolated verify and review worktrees detached at the PR head, a capability-based forge matrix, provenance-marked comments and PRs, a severity-gated finding ledger with delta re-review and a bounded cycle cap, gated merge, and close, with a GitHub tracker adapter. It ships `issue.md`, `pr.md`, `verdict.md`, `state.md`, `review-task.md`, and `verify-task.md` templates. |

## Layout: one canonical copy plus links
{: #skills-layout }

Every command manages a single canonical copy of each skill. Complex skills
ship several files, installed together as siblings:

| Piece | Location |
| --- | --- |
| Canonical copy | `~/.agents/skills/<name>/` (or `./.agents/skills/<name>/` with `--local`), containing `SKILL.md` and any shipped templates such as `issue.md`, `pr.md`, `verdict.md`, `state.md`, `review-task.md`, and `verify-task.md` |
| Claude link | `~/.claude/skills/<name>` → `../../.agents/skills/<name>` (or `./.claude/skills/<name>` with `--local`) |
| OpenCode link | `~/.config/opencode/skills/<name>` → `../../../.agents/skills/<name>` (or `./.opencode/skills/<name>` with `--local`) |
| Manifest | `~/.agents/skills/.agentbox-managed.json` (or `./.agents/skills/.agentbox-managed.json`) |
| Backups | `~/.agents/.agentbox-skill-backups/<timestamp>/` (or `./.agents/.agentbox-skill-backups/<timestamp>/`); `--dir` installs back up under `<dir>/.agentbox-skill-backups/<timestamp>/` |

Claude and OpenCode do not read the shared `.agents/skills` root, so they get
a relative symlink to the canonical copy. **Nothing is ever written into
`~/.codex/skills`**: Codex scans `~/.agents/skills` directly, so a copy or
link there would register every skill twice in a Codex session. Selecting
Codex only enables reporting and the legacy migration described below.

The manifest records, per skill, the version, the canonical `SKILL.md` path,
the links created, and one entry per managed file with its installed relative
path, absolute path, and rendered `sha256`. Version 2 manifests use that
per-file shape; a version 1 manifest is migrated in place on the next
`install` or `update`, keeping the recorded `SKILL.md` hash so an existing
install is recognised rather than treated as modified. `update` and `remove`
only touch paths recorded there, and every forced replacement or removal first
moves the previous content into the timestamped backup directory at the same
relative path.

`agentbox-orchestrator` ships `issue.md`, `pr.md`, `verdict.md`, `state.md`,
`review-task.md`, and `verify-task.md` from `templates/` in the source tree and
installs them beside its `SKILL.md`. The other skills remain single-file.

### Relationship to `npx skills`
{: #skills-npx }

The canonical layout is the shared `.agents/skills` convention that
[`npx skills`](https://skills.sh) also reads:

- `npx skills list -g` (and `npx skills list` for a `--local` install) lists
  the AgentBox skills with `Source: untracked` and no parse warnings.
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
| `--target` | — | Deprecated alias for `--agent`. `agents` writes only the canonical copies; `codex`, `claude`, `opencode` select one harness; `all` selects all three. |
| `--all` | `false` | Install for every harness regardless of directory detection. |
| `--local` | `false` | Put the canonical root at `./.agents/skills` for the current project and link into `./.claude/skills` and `./.opencode/skills`. |
| `--dir` | — | Escape hatch: write plain copies of every shipped skill file under this directory with no links, manifest, or lifecycle. Refused inside `~/.codex/skills`. With `--force`, a replaced file is first moved to `<dir>/.agentbox-skill-backups/<timestamp>/`; an unreadable path is refused and left in place. |
| `--force` | `false` | Replace modified copies, unreadable managed files, and foreign destinations, backing up the previous content first. |
| `--dry-run` | `false` | Report the planned changes without touching the filesystem. |
| `--json` | `false` | Print the result list as JSON. |

A harness counts as detected when its parent directory exists (`~/.claude`,
`~/.config/opencode`, `~/.codex`). Detected defaults never create a harness
directory; an explicit `--agent` or `--all` does. The canonical
`~/.agents/skills` root is always created by `install`.

`install` is idempotent: unchanged copies and correct links are reported as
`unchanged`, one result per file. Stale files that still match the manifest
are refreshed in place. A modified file — anything that differs from what the
manifest recorded and from the embedded skill — is preserved and refused
unless `--force` replaces it after a backup. The backup rule also covers
`--dir` installs, where the previous file lands under
`<dir>/.agentbox-skill-backups/<timestamp>/`. The same rule protects foreign
destinations: an existing real directory or foreign symlink at a link path is
reported and skipped unless `--force`. In the managed layout, a path that
exists but cannot be read is reported as `unreadable`; `--force` moves it into
the backup and writes the shipped file. The same applies when a regular file or
symlink occupies the canonical skill directory itself: without `--force` it is
refused, and with `--force` it is backed up and the directory is recreated. A
`--dir` install refuses an unreadable path without touching it.

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

`update` refreshes files that are `current` or `stale` and skips a skill whose
`SKILL.md` is `missing` or whose canonical skill path is a regular file or
symlink instead of a directory (run `install --force` for those). For an
installed skill, `update` also adds files this binary ships but the install
predates, so a version 1 install gains the orchestrator templates. Modified
files are skipped unless `--force` replaces them after a backup; unreadable
files inside a real skill directory are repaired the same way. A blocker at the
canonical skill path itself cannot be repaired by `update --force`: it moves
the blocker into the backup and then skips the skill because `SKILL.md` is
gone, so use `agentbox skills install --force`. With `--force`, a symlink at
the canonical skill path is moved into the backup as the link entry itself;
its target is never changed. Dangling harness links are repaired without
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

`remove` deletes only manifest-recorded paths, including every managed
template. A recorded template is removed only when it matches the hash in the
recorded manifest; a recorded `SKILL.md` is also removed when it matches the
rendered copy embedded in the running binary. A recorded file that matches
neither is refused unless `--force` removes it after a backup; the manifest
record is kept while such a file remains. Foreign files and directories
sitting at recorded link paths, and files at shipped template paths that
AgentBox never recorded, are always left alone, even when such a template file
is byte-identical to the shipped template. When the manifest runs empty, the
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
| `agents` | `unreadable` | The path exists but cannot be read, for example because of permissions, because it is not a file, or because the skill directory entry is a symlink, dangling or pointing elsewhere. Repair it with `agentbox skills install --force`; the link or blocker entry itself is backed up and a symlink target is left untouched. |
| `claude`, `opencode` | `current` | Link points at the canonical copy and a skill file is reachable through it. |
| `claude`, `opencode` | `broken-link` | Symlink resolves nowhere or has no skill behind it. |
| `claude`, `opencode` | `foreign` | A real directory, file, or foreign symlink occupies the link path. |
| `claude`, `opencode` | `missing` | No link. |
| `claude`, `opencode` | `unreadable` | The link path exists but cannot be inspected. |
| `codex` | `legacy` | Leftover copy in `~/.codex/skills` from an older release. |
| `codex` | `missing` | No leftover copy (the normal state; Codex reads the canonical root). |

Skills that ship templates also report one `file` row per template, derived
from the same hashes:

| Row | State | Meaning |
| --- | --- | --- |
| `file` | `current` | Byte-identical to the shipped template of this binary. |
| `file` | `stale` | Byte-identical to the manifest record, but the binary is newer. |
| `file` | `modified` | Differs from both the manifest and the shipped template. |
| `file` | `missing` | No installed template at that path. |
| `file` | `unreadable` | The template path exists but cannot be read; `--force` replaces it after a backup. |

The `agents` row still describes `SKILL.md`, so existing `SKILL.md` and link
rows keep their meaning.

`list` only reads, so it works when the home directory is read-only or
unavailable.

## show
{: #skills-show }

Print one embedded skill to standard output with the version substituted.

```text
agentbox skills show NAME [--json]
```

`--json` prints `{name, version, description, content}` instead of the raw
markdown. Valid names are `agentbox-cli`, `agentbox-api`, and
`agentbox-orchestrator`. `show` performs no writes and does not need a home
directory.

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
`codex`), or `dir` for `--dir` installs. `file` names the acted-on file
relative to the skill directory, such as `SKILL.md` or `issue.md`; it is
omitted on link results. `action` is one of `installed`, `updated`,
`unchanged`, `linked`, `migrated`, `removed`, or `skipped`, with `would-`
prefixed counterparts in dry runs. `list` prints the status tree described
above, including a `files` array of template states for skills that ship
templates, and `show` prints the single-skill object.

`agentbox doctor` reports the same states: stale copies, modified copies,
invalid front matter, unreadable canonical paths, missing and unreadable
template files, broken links, and legacy Codex copies each come with the exact
command that fixes them. An unreadable canonical path is a path that exists but
cannot be read as a skill directory, such as a regular file, a dangling
symlink, or a symlink to another directory at `~/.agents/skills/<name>`; the
doctor recommends `agentbox skills install --force`, which backs up the link or
blocker entry itself, recreates the skill directory, and leaves the symlink
target untouched. When a template problem is reported without a usable
canonical `SKILL.md`, the advice uses `agentbox skills install`, or `agentbox
skills install --force` for modified and unreadable templates, because
`update` skips a missing or unreadable skill.
