# MCP Tools — Requirement

Status: implemented (awaiting validation before commit). Research for both
open questions is complete (see [Research findings](#research-findings)).
The integration details are documented in
[openshell-architecture.md](openshell-architecture.md) and the usage guide
in the README.

## Problem

AgentBox runs execute inside isolated OpenShell sandboxes:

- agent configuration is generated on the host and extracted into
  `/workspace/state` (`HOME`, `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, and
  `XDG_CONFIG_HOME` all point there);
- the project is cloned into `/workspace/project`.

As a result, MCP configuration committed in the project (for example a
`.mcp.json` file) reaches the agent naturally, but the user's global MCP
configuration on the host never enters the sandbox, and nothing tells the
user which MCP configuration a run will use.

AgentBox must let the user name a reusable selection of global MCP servers,
choose it when a run starts, and have exactly those servers delivered into
the sandbox alongside whatever the project itself commits.

## Goals

1. A named, agent-scoped **MCP tools** entry in AgentBox global state that
   selects a subset of the agent's own global MCP servers by name.
2. Per-run selection at agent start (run-start dialog and CLI flag); no
   per-project association.
3. Deliver the selected servers into the sandbox in the agent's native
   format. AgentBox stores selections only — never server definitions.
4. Project-local MCP files always ride along; on name conflicts, local wins.
5. Follow the repository credential rules: environment-variable names only;
   secret values are never stored, logged, serialized, or passed.

## Non-goals

- No MCP server registry, editor, or validator in AgentBox. Server
  definitions live in the agent's own configuration and are passed through.
- No format conversion between agents and no normalized cross-agent config.
- No copying, merging, or rewriting of project-local MCP files. They ride
  along in the clone; AgentBox only detects them by filename for display.
- No per-project or per-profile association with an MCP tools entry;
  selection happens at run start only.
- No MCP health checks or agent-side tool management.
- No automatic policy editing for MCP network endpoints.
- No automatic OpenShell provider/profile generation for MCP endpoints
  (the manual path is documented; automation is a possible follow-up).
- No speculative Orca integration.

## Definitions

**MCP tools entry** — a named entity stored at
`$AGENTBOX_HOME/mcps/<name>.yaml`:

```yaml
version: 1
name: work-claude
agent: claude
servers:
  - github
  - slack
```

The entry selects servers **by name**; enabling a server exposes its tools.
It contains no server definitions, no URLs, commands, or credentials. An
entry names servers explicitly — no wildcards and no "all" mode — so a run's
toolset is always deterministic.

**Global MCP configuration** — the MCP portion of the selected agent's own
user-scope configuration on the host, resolved the same way the agent CLI
resolves it:

| Agent    | Host source                                                                          | MCP portion              |
| -------- | ------------------------------------------------------------------------------------ | ------------------------ |
| claude   | `$CLAUDE_CONFIG_DIR/.claude.json` (default `~/.claude.json`)                         | `mcpServers` key         |
| codex    | `$CODEX_HOME/config.toml` (default `~/.codex/config.toml`)                           | `[mcp_servers.*]` tables |
| opencode | `$XDG_CONFIG_HOME/opencode/opencode.json(c)` (default `~/.config/opencode/opencode.json(c)`) | `mcp.servers` key |

AgentBox does not duplicate this configuration. If the source file is absent
or contains no MCP portion, no global servers are available to select.

**Local MCP configuration** — agent-native MCP files committed at the
project root, detected by filename only:

- claude: `.mcp.json`
- opencode: `opencode.json(c)` or `.opencode/opencode.json(c)`
- codex: `.codex/config.toml` exists as a project format but is loaded only
  for **trusted projects**; a fresh sandbox clone is never trusted, so for
  AgentBox runs codex has no effective local configuration.

**Local wins** — when a selected global server has the same name as a
project-local server, the project-local definition takes precedence. This is
achieved by delivering global servers at user scope and relying on each
agent's native scope precedence (verified; see FR5).

## Functional requirements

### FR1 — Entity model and storage

- Stored under `$AGENTBOX_HOME/mcps/<name>.yaml`, following the existing
  entity pattern (`profiles/`, `policies/`). `settings.yaml` is unchanged.
- Validation: name matches the existing safe-name rule; agent is `claude`,
  `codex`, or `opencode`; the server list is non-empty, unique, and contains
  plain names.
- Deleting or renaming an entry does not affect running runs (configuration
  is copied at start). Historical run records keep the name they were
  started with.

### FR2 — Global server discovery

AgentBox can enumerate the available global MCP server names for a given
agent by reading the host source in the table above. This powers entry
creation in the GUI and CLI. Discovery reads names only.

### FR3 — Selection at run start

- `agent start` accepts an MCP tools entry name (CLI `--mcp <name>`, GUI
  run-start dialog). Empty selection is the default and reproduces today's
  behavior exactly.
- The entry's agent must match the run profile's agent; a mismatch fails
  run start with an actionable error before sandbox creation.
- The effective entry name is resolved before sandbox creation and recorded
  on the run record (`agent show`, JSON output).
- No per-project or per-profile association exists.

### FR4 — Extraction and drift

When an entry is selected, AgentBox reads the host source for the run's
agent and extracts **only** the selected servers' definitions. The remainder
of the host file is never copied into a run: host agent configuration can
contain account state and credentials which must not enter the sandbox.

- Extraction is structural: definitions are copied verbatim (values
  unchanged; nothing added, removed, or validated).
- AgentBox never interprets individual server definitions.
- If a selected name no longer exists in the host source, the run **warns
  and continues** with the servers that remain. The warning names the
  missing servers and is visible in CLI start output, `--dry-run`, the JSON
  response, and the GUI run-start dialog.
- An absent host source or an empty MCP portion is not an error; the same
  warning applies.

### FR5 — Delivery

The extracted definitions are staged into the run configuration archive
(`/workspace/state`) at user scope, so native precedence yields "local
wins" (verified for all three agents; see
[Research findings](#research-findings)):

- **claude**: the selected servers are written as the `mcpServers` key of
  `$CLAUDE_CONFIG_DIR/.claude.json`. Additionally, AgentBox adds
  `enableAllProjectMcpServers: true` to the generated
  `$CLAUDE_CONFIG_DIR/settings.json` (merged with the gateway-mode settings
  AgentBox already writes; created in standard mode). Without this key,
  project `.mcp.json` servers stay at "pending approval" forever in a fresh
  clone — a cloned repository cannot approve its own servers, and only
  user-scope settings apply in untrusted folders. This makes
  "local rides along" effective and is a deliberate trust decision: the
  worktree is the user's own project clone, so its committed MCP servers
  are within the user's trust domain. It changes existing behavior: claude
  runs previously ignored project `.mcp.json` servers (pending approval);
  after this change they load.
- **codex**: the selected servers are emitted as `[mcp_servers.*]` tables in
  the generated `CODEX_HOME/config.toml` (appended after any
  gateway-generated content; created in standard mode).
- **opencode**: the selected servers are merged into the `mcp.servers` key
  of the generated `$XDG_CONFIG_HOME/opencode/opencode.json` (AgentBox
  already generates this file in gateway mode; a structural merge of the
  `mcp.servers` key is required). The nested `mcp.servers.<name>` schema is
  the one OpenCode reads and writes.

Runs without a selection deliver nothing, except the claude
`enableAllProjectMcpServers` setting, which is written for every claude run
so local files ride along as specified.

### FR6 — Credentials

- Server definitions reference environment-variable names using each
  agent's native syntax: claude expands `${VAR}` in `command`, `args`,
  `url`, `headers`, and `env` values; codex forwards names via `env_vars`
  for stdio servers and `bearer_token_env_var` / `env_http_headers` for
  HTTP servers; opencode substitutes `{env:VAR}` inside config strings.
- Values must resolve inside the sandbox. The OpenShell mechanisms are:
  - **Non-secret values** may be set with `openshell sandbox create --env
    KEY=VALUE` (repeatable). OpenShell explicitly forbids secrets here,
    and so does this requirement.
  - **Secrets** flow through OpenShell providers: the value stays in the
    gateway environment, the sandbox environment holds an
    `openshell:resolve:env:<id>_NAME` reference, and the gateway rewrites
    the credential on the wire for registered, policy-allowed endpoints.
    Provider profiles on current gateways are inference-scoped, so
    secret-requiring MCP servers need a manually created provider and
    policy rule for their endpoint today; automating that is a possible
    follow-up, out of scope here.
- AgentBox scans the extracted definitions for inline secret-like values
  and rejects run start with an actionable error naming the offending
  server and field, pointing at environment-variable references instead.
- AgentBox stores, logs, and passes only variable names. It never reads,
  holds, or forwards secret values, including via `--env`.

### FR7 — Policy

MCP endpoints are governed by the run policy like any other network access.
AgentBox never edits policies automatically.

- The existing policy rule UX (`AddPolicyNetworkRule`, live `policy set`)
  is the documented path for allowing MCP endpoint hosts.
- Network rules are **binary-scoped**: the rule must name the process that
  makes the network call. For stdio MCP servers that is the server process
  binary (for example the `node`/`npx` path inside the image), not the
  agent binary that AgentBox defaults to. The documentation must call this
  out; discovery of the effective binary per server is the user's
  responsibility in v1.

### FR8 — Unattended safety

Runs with `approval: unattended` must not block on MCP trust or enable
prompts. The selection must resolve completely before the agent starts.
For claude this is satisfied by the `enableAllProjectMcpServers` user-scope
setting (FR5); codex and opencode have no MCP approval gates at startup.

### FR9 — GUI

- Setup > MCP Tools tab (inside the existing Setup sheet, not a new
  top-level surface): create, list, and delete entries; show the available
  global server names for an agent; show detected local MCP files per
  project for context.
- Run-start dialog: MCP tools selection filtered to entries compatible with
  the chosen profile's agent, plus any drift warnings after start.
- Status meaning must not rely on color alone; errors are actionable and
  visible in the relevant dialog.

### FR10 — CLI

- `agentbox mcp list|show|create|delete` manages entries; listing global
  server names per agent is available to assist creation.
- `agentbox agent start --mcp <name>` selects the entry for that run.
- `--dry-run`, `agent show`, and JSON output display the effective selection
  and any drift warnings.

## Acceptance criteria

1. A run started with an entry selecting N servers delivers exactly those
   servers for each supported agent, and project-local files still apply;
   name conflicts resolve in favor of the local definition (claude,
   opencode; codex has no effective local configuration).
2. A run started without a selection delivers no global servers; claude
   runs still load committed `.mcp.json` servers per FR5.
3. Selecting an entry whose agent differs from the run profile's agent fails
   before sandbox creation with an actionable error.
4. Selected names missing from the host source produce a warning naming
   them, visible in CLI output, dry-run, JSON, and the GUI; the run
   proceeds with the remaining servers.
5. No secret value appears in run records, logs, config archives, or the
   GUI. Inline secret-like values in extracted definitions fail run start
   with an actionable error.
6. Unattended runs start without MCP prompts.
7. Unit tests cover entry validation, discovery, selection resolution,
   extraction, and per-agent delivery; GUI tests cover the MCP Tools tab
   and the run-start dialog; README and openshell-architecture.md are
   updated with the implementation.

## Research findings

Verified on 2026-11-11 against the host CLIs and the OpenShell 0.1.4 CLI;
host versions: claude 2.1.318 (image: 2.1.321), codex 0.167.1 (image:
0.167.0), opencode 2.0.32 host and 2.2.6 via npx (image: 2.2.6).

**Claude Code**

- User-scope MCP servers load from `$CLAUDE_CONFIG_DIR/.claude.json`
  (`mcpServers` key). Verified with `claude mcp list` under a temporary
  `CLAUDE_CONFIG_DIR`.
- Project `.mcp.json` servers are "⏸ Pending approval" in a fresh
  environment. With `enableAllProjectMcpServers: true` in user-scope
  `$CLAUDE_CONFIG_DIR/settings.json`, they load without interaction, and on
  a name conflict the **project** definition wins over user scope. The
  official docs confirm: a cloned repository cannot approve its own servers
  (project-committed approval settings are ignored in untrusted folders),
  while user-scope settings approvals apply everywhere; on conflicts the
  highest-precedence source's whole entry is used, fields are not merged.
- `${VAR}` expansion in MCP config values reads from the process
  environment; unset variables warn and load unexpanded.
- `--mcp-config <files...>` and `--strict-mcp-config` exist as an
  alternative delivery mechanism; not needed given user-scope delivery
  works. Reserved server names (for example `workspace`) are skipped.

**Codex**

- `[mcp_servers.<name>]` tables in `$CODEX_HOME/config.toml` load
  (`codex mcp list` under a temporary `CODEX_HOME`). Version matches the
  image (0.167.x).
- `.codex/config.toml` is a project-scoped config but is loaded only for
  trusted projects; fresh clones are untrusted, so it is ineffective in
  runs.
- Stdio servers support `env_vars` (forward environment variables by name
  from the Codex process environment); HTTP servers support
  `bearer_token_env_var` and `env_http_headers`.

**OpenCode**

- Global config is read from `$XDG_CONFIG_HOME/opencode/opencode.json` or
  `.jsonc`; project config requires project detection (a Git work tree) and
  wins over global on key conflicts. Verified with 2.2.6 under isolated
  `XDG_CONFIG_HOME`/`XDG_DATA_HOME`, and documented: global < repo root <
  nested project configs < system config. The system directory
  (`/etc/opencode` on Linux) outranks everything but is controlled by the
  AgentBox image.
- The MCP schema is nested: `mcp.servers.<name>` with
  `{"type": "local", "command": [...]}`; `opencode mcp add` writes this
  shape.
- `{env:VAR}` substitution is supported inside config strings.
- Caveat for host-side tooling only: `opencode mcp list` delegates to a
  managed background service that caches config; sandbox runs use
  `--standalone` and are unaffected.

**OpenShell (0.1.4)**

- `sandbox create --env KEY=VALUE` (repeatable) sets **non-secret**
  environment variables only; the CLI explicitly directs secrets to
  providers, and warns when `--env` values look like credentials.
- Secrets attach via `sandbox create --provider <name>`; providers are
  created with `provider create --name <n> --type <profile> --credential
  <ENV_NAME>`. The sandbox environment then holds
  `openshell:resolve:env:<id>_NAME` references, and the gateway rewrites
  credentials on the wire for registered, policy-allowed endpoints
  (policy enforcement modes include credential rewrite).
- Provider profiles on the current gateway are inference-scoped, but the
  profile system is general (AgentBox already generates custom gateway
  profiles), so per-MCP-host providers are mechanically possible as a
  follow-up.
- Policy network rules are host+port+protocol endpoint lists scoped to
  binary paths; `openshell policy update --add-endpoint` and AgentBox's
  existing rule editing apply them live.

## Open questions

None blocking. Deferred items (explicitly out of scope for the first
implementation):

1. Automatic OpenShell provider/profile generation for MCP endpoints that
   require secrets.
2. Auto-detection of the network-calling binary for stdio MCP servers in
   policy rules.

## Implementation plan

Stages land in order; `make test`, `make vet`, and (for run-lifecycle
changes) `make race` pass at each stage.

### Stage 1 — application layer (`internal/app`)

- `store.go`: `MCPTools` entity (`version`, `name`, `agent`, `servers`) with
  `Validate()` (safe name, agent enum, non-empty unique plain names); add
  `mcps/` to `Store.Ensure()`; store CRUD following the policy pattern
  (`List`/`Load`/`Save`/`Delete` + path helper).
- New `mcps.go`:
  - `GlobalMCPServers(agent)` — discovery; resolves the host source path
    from `CLAUDE_CONFIG_DIR` / `CODEX_HOME` / `XDG_CONFIG_HOME` (with
    `~/.claude.json`, `~/.codex/config.toml`,
    `~/.config/opencode/opencode.json`→`.jsonc` fallbacks); parses only the
    MCP portion; absent source returns an empty list, not an error. For
    claude, only the top-level `mcpServers` key counts (per-project entries
    under `projects` are ignored).
  - extraction of selected definitions (structural copy) returning the
    delivered set plus missing names for the drift warning;
  - secret scan over extracted string values (existing `secretName`
    pattern) failing with an actionable error naming server and field.
- `runs.go`: `StartOptions.MCP`; resolution before sandbox creation — load
  entry, enforce agent compatibility, extract, secret-scan; `Run` gains
  omitempty `mcp` (effective entry name) and `mcp_warnings` fields so
  `agent show` and JSON carry them.
- `agents.go`: thread the resolved selection into `PrepareAgent` delivery:
  - claude: write `home/.claude/.claude.json` (`mcpServers`) and always
    write `home/.claude/settings.json` including
    `enableAllProjectMcpServers: true` (merged with the gateway-mode `env`
    settings; standard mode currently writes no settings file);
  - codex: append `[mcp_servers.*]` tables to `home/.codex/config.toml`
    (created in standard mode);
  - opencode: merge `mcp.servers` into `config/opencode/opencode.json`
    (created in standard mode).
- Tests: new `mcps_test.go`; extend `agents_test.go` (per-agent delivery in
  both modes, settings merge) and `runs_test.go` (resolution, compat error,
  default unchanged, drift warning, secret rejection).

### Stage 2 — CLI (`cmd/agentbox/main.go`)

- `agentbox mcp list|show|create|delete` plus `agentbox mcp servers --agent
  <agent>` for discovery during creation
  (`mcp create --agent <agent> --servers <name>,<name>... <name>`).
- `agent start --mcp <name>`; `agent show` and JSON output pick up the new run
  fields automatically.
- Tests follow the existing `main_test.go` fake-runner pattern.

### Stage 3 — GUI API (`internal/gui/server.go`)

- Routes mirroring the policies pattern: `GET/POST /api/mcps`,
  `GET /api/mcps/servers?agent=...`, `POST /api/mcps/{name}/delete`.
- `startRun` accepts `mcp` and returns the run including drift warnings.
- `server_test.go` coverage for the new handlers and start payload.

### Stage 4 — web GUI (`internal/gui/web/src`)

- `types.ts` / `api.ts`: `MCPTools` type, CRUD + discovery calls, `Run.mcp`
  and `Run.mcp_warnings`, `StartRunRequest.mcp`.
- New `pages/setup/McpPanel.tsx` wired as an "MCP Tools" tab in
  `SetupSheet.tsx`: entry list, create dialog (name, agent, server
  multi-select fed by discovery), delete, and detected local MCP files per
  project for context.
- `FleetView.tsx` run-start dialog: MCP selection filtered to entries whose
  agent matches the chosen profile; surface drift warnings after start.
- Vitest/Testing Library coverage for the panel and the start form;
  `npm --prefix internal/gui/web run build` and commit the rebuilt
  `dist/`.

### Stage 5 — documentation

- `docs/openshell-architecture.md`: MCP delivery layout per agent, the env
  and credential model (providers, never `--env`), and the policy
  binary-scoping note.
- `README.md`: `agentbox mcp` usage and `agent start --mcp`.
- Update the status line of this document.

## Decisions

- Entity name: **MCP tools** (it selects servers by name; enabling a server
  exposes its tools). Identifiers stay short: `mcps/` storage, `--mcp` flag.
- Selection is per-run only; no per-project or per-profile association.
- Merge strategy is fixed: local always wins, via user-scope delivery and
  native agent precedence (verified).
- Drift (missing selected names) warns and continues; the run is not failed.
- Default with no selection: today's behavior, except that claude runs gain
  `enableAllProjectMcpServers` so committed `.mcp.json` servers finally load
  (the "local rides along" expectation).
- Global configuration source: the selected agent's own user-scope
  configuration on the host; AgentBox stores selections only.
- Credentials: environment-variable names only; secrets flow through
  OpenShell providers, never through AgentBox or `--env`.
