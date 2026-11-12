# Skills and Plugin Delivery — Requirement

Status: implemented. The integration details are documented in
[openshell-architecture.md](openshell-architecture.md) and the usage guide in
the README.

## Problem

AgentBox runs execute inside isolated OpenShell sandboxes. The prepared state
tree carries the project clone, provider/profile settings, and selected MCP
servers, but not the developer's host skills. An agent that works well on the
host therefore loses its skills once it moves into the sandbox; only skills
committed inside the project ride along.

Global MCP servers already solve the equivalent problem. Skills and plugin
skill bundles need the same treatment.

## Goals

1. A named, agent-scoped **skills entry** stored at
   `$AGENTBOX_HOME/skills/<name>.yaml`.
2. Per-run selection at agent start; no per-project or per-profile association.
3. Deliver selected `SKILL.md` folders into the agent's user skills directory
   inside the generated state tree.
4. Include a plugin only as a whole: its bundled skills, or nothing.
5. Missing host skills or plugins produce warnings and never fail the run.

## Non-goals

- No plugin scripts, apps, hooks, commands, or plugin-declared MCP servers.
- No full plugin registration in the sandbox.
- No per-skill selection inside a plugin.
- No AgentBox plugin system or general extensibility layer.
- No Orca integration.

## Data model

```yaml
version: 1
name: work-codex
agent: codex
skills: [orca-cli, orchestration]
plugins: ["browser@openai-bundled"]
```

Validation:

- `version` is `1`; `name` uses the existing lower-case identifier rule.
- `agent` is `claude`, `codex`, or `opencode`.
- At least one of `skills` or `plugins` is non-empty.
- Entries are unique within each list.
- Skill names and both `<plugin>@<marketplace>` parts are path-free tokens
  (no `/`, no leading dot, no `..`).

## Discovery

Discovery is read-only. Missing source directories produce empty lists.

| Agent | Skill roots | Plugin bundles |
| --- | --- | --- |
| claude | `$CLAUDE_CONFIG_DIR/skills` (default `~/.claude/skills`), following symlinks | `plugins/marketplaces/*/*` and `plugins/cache/*/*/*` |
| codex | `$CODEX_HOME/skills` (default `~/.codex/skills`), skipping hidden directories such as `.system` | `$CODEX_HOME/plugins/cache/<marketplace>/<plugin>/<version>/skills` |
| opencode | `$XDG_CONFIG_HOME/opencode/skills` (default `~/.config/opencode/skills`) | none |

Discovery returns skill names, frontmatter descriptions, and plugin bundled
skill counts only. Skill bodies are never returned by the API, logged, or
rendered.

## Delivery

At run start AgentBox copies the selected folders into the prepared state tree:

| Agent | Destination |
| --- | --- |
| claude | `state/home/.claude/skills/<name>/` |
| codex | `state/home/.codex/skills/<name>/` |
| opencode | `state/config/opencode/skills/<name>/` |

- Standalone skills are delivered first, then plugin skills in entry order.
- Duplicate delivered names are first-wins and produce a warning.
- Missing skills or plugins warn and continue; the run still starts.
- An entry whose agent differs from the run profile fails before sandbox
  creation with an actionable error.
- Symlinked skill directories are resolved to their target contents.
- Executable bits are preserved.
- No selection leaves the sandbox unchanged. Project-committed skills ride
  along untouched in the project clone.

## Interfaces

CLI:

```text
agentbox skill list|show|create|delete|available
agentbox skill create --agent AGENT --skills a,b --plugins p@m NAME
agentbox skill available --agent AGENT
agentbox agent start ... --skills NAME
```

HTTP:

```text
GET  /api/skills
POST /api/skills
POST /api/skills/{name}/delete
GET  /api/skills/available?agent=
```

The run record carries `skills` and `skills_warnings`, surfaced by
`agent show`, JSON output, and the GUI. The Setup **Skills** tab manages
entries; the new-agent dialog selects an entry filtered to the profile's
agent and displays post-start warnings.
