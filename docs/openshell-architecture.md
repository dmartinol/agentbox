# AgentBox and OpenShell Integration Architecture

This document explains how AgentBox composes OpenShell images, providers,
profiles, policies, and agent runs. It is intended for contributors and anyone
debugging why an agent starts, attaches, syncs, or fails.

## High-level model

AgentBox separates four concerns:

1. **Agent image** — the immutable runtime containing Codex, Claude Code, or
   OpenCode and their dependencies.
2. **OpenShell provider** — the registered inference endpoint, authentication
   style, credential reference, and allowed network path.
3. **AgentBox profile** — the user-selected image, agent, model, provider,
   approval mode, and policy defaults.
4. **Run** — one isolated worktree, sandbox, policy, agent configuration, and
   lifecycle record.

The image does not contain provider credentials or per-run gateway settings.
Those are attached or copied when a run starts.

## Image layer

AgentBox builds one local image per agent version:

```text
localhost/openshell:codex-0.167.0-r3
localhost/openshell:claude-2.1.321-r3
localhost/openshell:opencode-2.2.6-r3
```

The `r3` suffix identifies the image recipe. The image contains:

- the agent CLI binary;
- Node.js and required runtime dependencies;
- OpenShell-compatible filesystem paths;
- OCI labels describing the agent, version, and recipe.

The image does not contain:

- EnMaaS or OpenAI credentials;
- provider hosts;
- model IDs;
- AgentBox profiles;
- run policies;
- project source code.

This makes one image reusable across providers, profiles, projects, and runs.

## Provider layer

Provider setup registers an inference provider with OpenShell.

For a gateway provider, AgentBox generates a profile that describes:

- endpoint host and path;
- authentication style (`bearer` or header);
- credential environment-variable name;
- allowed agent binaries;
- network enforcement.

It then imports or updates the profile and creates a named OpenShell provider.
For example:

```text
agentbox-gateway-bearer-api-enmaas-devshift-net
```

The provider references a credential environment variable such as:

```text
ENMAAS_KEY
```

The secret value remains in the OpenShell gateway process environment. AgentBox
stores only the variable name.

At sandbox creation, AgentBox passes the provider name:

```text
openshell sandbox create --provider <provider-name> ...
```

OpenShell uses that registration to inject the credential reference and enforce
network access to the provider endpoint.

### Provider types

- `gateway-bearer` — OpenAI-compatible APIs such as Codex and OpenCode through
  EnMaaS or another gateway.
- `gateway-header` — Anthropic-compatible APIs such as Claude through EnMaaS.
- `openai` — built-in direct OpenAI API access; use with profile mode
  `standard`.
- `anthropic` — built-in direct Anthropic API access; use with profile mode
  `standard`.

The `openai` and `anthropic` providers are built in because their endpoint and
credential environment are fixed by the vendor. AgentBox ensures both exist for
a gateway when the provider list is loaded and before starting a run that
references one. If the matching credential environment is absent, the provider
is reported as unregistered rather than created. Built-in providers cannot use
a custom name and cannot be edited or deleted through AgentBox.

## Profile layer

An AgentBox profile selects:

- agent type;
- image;
- provider;
- `standard` or `gateway` mode;
- exact model ID for gateway mode;
- approval mode;
- policy;
- optional model API base URL and credential environment name.

### Standard mode

Standard mode uses a direct provider such as `openai` or `anthropic`.

- The model field is optional.
- If blank, the agent uses its own default model.
- AgentBox does not generate an OpenAI-compatible gateway configuration.
- Codex API-key authentication is initialized from the provider-injected
  `OPENAI_API_KEY` reference.
- Codex runs without its background daemon and without its nested sandbox
  because the OpenShell sandbox is already authoritative.

### Gateway mode

Gateway mode routes model traffic through an OpenShell-registered provider.

- An exact model ID is required.
- AgentBox generates agent-specific gateway configuration at run startup.
- The profile base URL is the exact model API endpoint, commonly ending in
  `/v1`.
- The provider host is normalized for OpenShell network policy and is not
  necessarily the same string as the profile base URL.

## Policy layer

When a profile is created without an explicit policy, AgentBox creates:

```text
$AGENTBOX_HOME/policies/<profile-name>.yaml
```

At run start, AgentBox:

1. loads the profile policy;
2. adapts agent binary paths for the selected agent;
3. writes a run-specific policy;
4. passes it to OpenShell during sandbox creation;
5. fetches and verifies the effective policy before starting the agent.

This is why a run can fail before the agent starts if the effective policy does
not allow the expected endpoint or lacks the required Landlock/network
configuration.

## Run startup sequence

A run is created as follows:

1. Load the AgentBox profile.
2. Validate image, provider, model, policy, and approval settings.
3. Create an isolated Git worktree or reserve a direct checkout.
4. Save the run record.
5. Create a source Git bundle.
6. Generate run-specific agent configuration on the host.
7. Create the sandbox from the selected image, provider, and policy.
8. Upload the source bundle.
9. Upload and extract the configuration archive into `/workspace/state`.
10. Clone the source bundle into `/workspace/project`.
11. Verify the effective OpenShell policy.
12. Mark the sandbox ready.
13. Start tmux and the agent session.

## Runtime configuration location

The generated configuration archive is extracted to:

```text
/workspace/state
```

The generated `agent.sh` sets:

```text
HOME=/workspace/state/home
XDG_CONFIG_HOME=/workspace/state/config
XDG_DATA_HOME=/workspace/state/data
CODEX_HOME=/workspace/state/home/.codex
CLAUDE_CONFIG_DIR=/workspace/state/home/.claude
```

Agent-specific files include:

```text
/workspace/state/home/.codex/config.toml
/workspace/state/home/.codex/gateway.config.toml
/workspace/state/config/opencode/opencode.json
/workspace/state/home/.claude/settings.json
/workspace/state/agent.sh
/workspace/state/session.sh
/workspace/state/bootstrap.sh
```

This configuration is copied at startup. Editing an AgentBox profile does not
retroactively change an already-running sandbox. Start a new run to use updated
profile settings.

## MCP tools layer

An MCP tools entry (`$AGENTBOX_HOME/mcps/<name>.yaml`) selects global MCP
servers **by name** for one agent. Definitions are never stored in AgentBox;
they live in the agent's own user-scope configuration on the host:

| Agent    | Host source                    | MCP portion              |
| -------- | ------------------------------ | ------------------------ |
| claude   | `$CLAUDE_CONFIG_DIR/.claude.json` | `mcpServers` key      |
| codex    | `$CODEX_HOME/config.toml`      | `[mcp_servers.*]` tables |
| opencode | `$XDG_CONFIG_HOME/opencode/opencode.json(c)` | `mcp.servers` key |

At run start, when `--mcp <name>` (or the GUI selection) is present,
AgentBox:

1. loads the entry and fails before sandbox creation if its agent differs
   from the profile's agent;
2. reads the host source and extracts only the selected server definitions
   (the remainder of the host file — account state, credentials — never
   enters the run);
3. rejects inline secret-like values with an actionable error;
4. records missing names as warnings on the run (`mcp_warnings`) and
   continues with the servers that remain.

Delivery happens through the generated configuration archive at user scope:

- claude: `home/.claude/.claude.json` gains the selected `mcpServers`, and
  the generated `home/.claude/settings.json` always sets
  `enableAllProjectMcpServers: true` — a cloned repository cannot approve
  its own `.mcp.json` servers, and only user-scope settings apply in
  untrusted folders;
- codex: `[mcp_servers.*]` tables are appended to the generated
  `home/.codex/config.toml`;
- opencode: the selected servers are merged into `mcp.servers` of the
  generated `config/opencode/opencode.json`.

Project-local MCP files ride along in the clone and win name conflicts
("local wins"): Claude Code and OpenCode project scope overrides user scope,
and Codex loads `.codex/config.toml` only for trusted projects, which a
fresh clone never is.

MCP credentials follow the provider model: definitions must reference
environment-variable names (`${NAME}`, `env_vars`, `{env:NAME}`), secret
values flow through OpenShell providers with on-wire rewrite, and AgentBox
never passes secret values — `openshell sandbox create --env` is for
non-secret variables only.

MCP endpoints are governed by the run policy like any other network access.
Network rules are binary-scoped: for a stdio MCP server the rule must name
the server process binary (for example the `node` path inside the image),
not the agent binary. Use the existing policy host-rule editing to allow
MCP endpoints; AgentBox never edits policies automatically.

## Credential resolution

A sandbox may show an environment value such as:

```text
OPENAI_API_KEY=openshell:resolve:env:<id>_OPENAI_API_KEY
```

This is not the raw secret. It is an OpenShell resolver reference. OpenShell
resolves the credential when the sandbox accesses the registered provider.

AgentBox never writes the raw secret into:

- images;
- profiles;
- generated agent configuration;
- run records;
- logs;
- command-line arguments.

## Attach and idle

Each run owns a tmux session named `agent`.

Attach connects to that session:

- CLI attach uses `openshell sandbox exec --tty -- tmux attach`.
- GUI attach uses xterm.js over a same-origin WebSocket.
- The GUI server allocates a local PTY with dimensions matching xterm.js.
- Closing the GUI terminal detaches the client but does not finish the run.

When the agent exits, the session asks whether to keep the run idle or finish
it. Idle runs retain:

- the sandbox;
- tmux;
- `/workspace/idle`;
- the run record;
- the worktree.

Exiting an idle shell returns to the idle/finish prompt rather than finishing
the run.

## Diff and sync

The agent works in `/workspace/project` inside the sandbox. AgentBox also owns
a local mirror worktree for each run.

- **Diff** compares the local run worktree with its baseline.
- **Sync** exports commits, patches, and untracked files from the sandbox into
  the local run worktree.
- Sync does not merge changes into the original project branch.
- Stop/finish performs a final sync before stopping the sandbox.

## State layout

Default AgentBox state is stored under:

```text
~/.agentbox/
  settings.yaml
  projects/
  profiles/
  runs/
  policies/
  mcps/
  worktrees/
```

Set `AGENTBOX_HOME` to use another state root.
