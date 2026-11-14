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
localhost/openshell:codex-0.167.0-r4
localhost/openshell:claude-2.1.321-r4
localhost/openshell:opencode-2.2.6-r4
```

The `r4` suffix identifies the image recipe. The image contains:

- the agent CLI binary;
- Node.js and required runtime dependencies;
- a fixed local-work toolset: `git`, `tmux`, `ca-certificates`, `curl`, `jq`,
  `ripgrep` (`rg`), `unzip`, `less`, `git-lfs`, and `procps` (`ps`);
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

The recipe revision, not per-package version pinning, is the reproducibility
boundary. Rebuilding an agent at a newer harness version writes a new tag with
the same recipe revision; it never rewrites an existing profile. Use **Refresh
all agents** in Setup → Images (or `agentbox image build`) to create fresh tags
at the latest releases.

### Image lifecycle

`agentbox image rm REF` deletes one local AgentBox image. `agentbox image prune`
deletes every local AgentBox image that no profile and no non-terminal run
references, and reclaims superseded dangling images that carry the
`agentbox=true` label by image ID. Both refuse named images with holders and
name them. Deleting an image that a profile still names leaves that profile in
place; a later run fails at start with the actionable `image unavailable
locally` error. Nothing rewrites `Profile.Image` automatically.

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

Exactly one `--provider` flag is passed. A second provider would register a
second credential reference, so the sandbox create command is asserted to carry
a single provider.

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

Images ship a local-work toolset (git, git-lfs, curl, jq, ripgrep, unzip, less,
procps) and deliberately omit `gh` and `openssh-client`. AgentBox therefore adds
no forge credential, no git credential helper, and no generated `.gitconfig` to a
run: repository work inside a sandbox uses plain `git` over the run policy, and a
sandbox has no forge reachability unless the policy is edited to allow it.
Forge-capable repository access is deferred to #19.
Custom policies written by earlier builds may still list forge hosts; remove
those rules with `agentbox policy edit` to restore the default.

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
- Codex runs without its background daemon and without its nested sandbox, so
  shell network calls follow the run policy instead of Codex's default
  network-disabled sandbox.

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

Network rules are binary-scoped. A host rule added by AgentBox therefore
allows the agent launcher together with `/usr/local/bin/node`, `/usr/bin/git`,
`/usr/lib/git-core/**`, `/usr/bin/curl`, and `/usr/bin/git-lfs`.
The supported agents are Node applications, so listing only the launcher path
would miss the process that opens the socket; git reaches HTTPS through a
separate helper binary.

The default policy grants write access to `/workspace`, `/tmp`, and
`/var/tmp`.

## Run startup sequence

A run is created as follows:

1. Load the AgentBox profile.
2. Validate image, provider, model, policy, and approval settings, then decide
   whether the provider is registered with the gateway.
3. Create an isolated Git worktree or reserve a direct checkout.
4. Save the run record.
5. Create a source Git bundle.
6. Generate run-specific agent configuration on the host.
7. Create the sandbox from the selected image, inference provider, and policy.
8. Upload the source bundle.
9. Upload and extract the configuration archive into `/workspace/state`.
10. Clone the source bundle into `/workspace/project`.
11. Verify the effective OpenShell policy.
12. Mark the sandbox ready.
13. Start tmux and the agent session.
14. Probe the new run once and cache its derived status.

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
TMPDIR=/tmp
```

Agent and shell sessions set `TMPDIR=/tmp`. Codex's TUI opens a SQLite
migration database before the model starts; when `TMPDIR` is unset, SQLite
selects `/var/tmp`, which can pass a writable-access check but still be
denied by Landlock.

Agent-specific files include:

```text
/workspace/state/home/.codex/config.toml
/workspace/state/home/.codex/gateway.config.toml
/workspace/state/config/opencode/opencode.json
/workspace/state/home/.claude/settings.json
/workspace/state/home/.codex/skills/<name>/
/workspace/state/home/.claude/skills/<name>/
/workspace/state/config/opencode/skills/<name>/
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

## Skills layer

A skills entry (`$AGENTBOX_HOME/skills/<name>.yaml`) selects standalone host
skills and whole plugin skill bundles for one agent. AgentBox stores only
names; skill contents remain in the agent's own host directories:

| Agent | Standalone skill root | Plugin bundles |
| --- | --- | --- |
| claude | `$CLAUDE_CONFIG_DIR/skills` (default `~/.claude/skills`) | `plugins/marketplaces/*/*` and `plugins/cache/*/*/*` |
| codex | `$CODEX_HOME/skills` (default `~/.codex/skills`), excluding hidden dirs | `$CODEX_HOME/plugins/cache/<marketplace>/<plugin>/<version>/skills` |
| opencode | `$XDG_CONFIG_HOME/opencode/skills` (default `~/.config/opencode/skills`) | none |

At run start, when `--skills <name>` (or the GUI selection) is present,
AgentBox:

1. loads the entry and fails before sandbox creation if its agent differs
   from the profile's agent;
2. discovers the host skills and plugin bundles for that agent;
3. resolves standalone skills first, then plugin skills in entry order;
4. records missing or duplicate entries as warnings on the run
   (`skills_warnings`) and continues.

Delivery uses the generated state archive at user scope:

- claude: `home/.claude/skills/<name>/`;
- codex: `home/.codex/skills/<name>/`;
- opencode: `config/opencode/skills/<name>/`.

Skill directories are copied recursively. Symlinked skill directories are
resolved to their target contents and executable bits are preserved. Plugins
contribute only their `skills/` folders: plugin scripts, apps, hooks,
commands, and plugin-declared MCP servers are not delivered. Project-committed
skills ride along in the clone unchanged.

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
- command-line arguments;
- generated activity hooks and the status probe (derived run status).

## Attach and run status

Each run owns a tmux session named `agent`. The GUI attaches to runs through
a project workspace: the tab strip lists every agent of the project, and each
tab is a view over one run.

- Only the visible tab holds a PTY. Switching tabs closes the previous xterm
  client; tmux keeps its session alive, so the tab reattaches to the same
  session on return. Unfocused tabs are represented by their status rather
  than by a background terminal.
- While a tab reattaches, the GUI shows the cached tail of
  `/workspace/state/agent.log`, so the surface is never blank.
- An attach issued while the sandbox bootstrap is still creating the tmux
  session waits for that session (up to 30 seconds) instead of failing with
  `no sessions`, and it fails immediately once `/workspace/finished` exists.
- Runs that cannot attach (Completed, Stopped, Failed, Unreachable) open a
  read-only log snapshot instead of a terminal.
- Shell tabs start a separate tmux session (`shell-N`) in the same sandbox
  and policy, at `/workspace/project`, with the same `HOME`, `XDG_*`,
  `CODEX_HOME`, `CLAUDE_CONFIG_DIR`, and `TMPDIR` environment as the agent.
  Shells never share the `agent` session, so their terminal sizes cannot
  fight and the agent log stays untouched.
- Closing a shell tab stops its tmux session. Closing the workspace only
  detaches; it does not finish the run.

Attach transport:

- CLI attach uses `openshell sandbox exec --tty -- tmux attach`.
- Every attach and sandbox command requests `LC_ALL=C.UTF-8` and
  `LANG=C.UTF-8`. The images set the same defaults. Without a UTF-8 locale
  the tmux client runs under POSIX and renders every character it cannot
  encode as `_`, which turned typographic apostrophes in agent output into
  underscores in the console.
- GUI attach uses xterm.js over a same-origin WebSocket and selects the
  session with `?session=agent|shell-N`.
- The GUI server allocates a local PTY sized from the xterm viewport: the
  client reports its dimensions at connect (`?cols=&rows=`) and sends
  `{"type":"resize","cols":C,"rows":R}` control messages on resize, which
  the server applies to the PTY so every rendered row stays visible.
  Terminal input travels as binary frames; text frames are control
  messages.

When the agent exits, the session asks whether to keep the run ready or finish
it. Kept runs retain:

- the sandbox;
- tmux;
- `/workspace/idle`;
- the run record;
- the worktree.

Exiting an idle shell returns to the idle/finish prompt rather than finishing
the run.

## Derived run status

`Run.Status` carries one derived, read-only operational value and
`Run.StatusDetail` an optional explanation. The persisted pair is only a cache
so Fleet and `agent show` can render before an inspect; every read reconciles
it. There is no status setter in the CLI or HTTP API, and execution gates use
the derived value only.

The classification lives in `internal/app/status.go`. One probe exec per run
collects the tmux session and foreground command, the bottom of the visible
pane, `/workspace/idle`, `/workspace/finished`, `/workspace/exit-code`,
`/workspace/state/activity.json`, and the `agent.log` mtime. Precedence, highest first:

1. sandbox phase `error` → Failed;
2. sandbox phase `stopped`/`exited` → Stopped;
3. no tmux session and no finished marker → Creating;
4. sandbox phase `completed` or finished marker → Completed, with the exit code
   or "agent exited" as detail;
5. the Codex plan-approval prompt on screen → Waiting for input, "plan ready for
   review";
6. an explicit `activity.json` signal while the session is alive → Working,
   Waiting for input, or Ready (`done`/`idle`);
7. idle marker or a shell in the tmux foreground → Ready;
8. agent in the tmux foreground → Working;
9. inspect error or unparsable response → Unreachable.

Log silence never produces Waiting for input. Only an explicit signal or the
session prompt does. OpenCode has no hooks, so a run counts as Working while
`agent.log` changed within the last two minutes and Ready afterwards.

### Activity signals

`/workspace/state/activity.json` holds
`{"state":"working|waiting|done|idle","detail":"..."}`. Malformed or unknown
values are ignored, and a signal is only honoured while the agent session is
alive, so a crash cannot leave a stale Working. AgentBox generates the writer
(`activity.sh`) and the hook definitions:

- Codex: `SessionStart` → idle; `UserPromptSubmit`, `PreToolUse`, `PostToolUse`
  → working; `PermissionRequest` → waiting; `Stop`, `SessionEnd` → done;
  `Interrupt` → idle. A `PreToolUse` for the `request_user_input` tool means the
  agent stopped to ask a question, so it signals waiting instead of working;
  the matching `PostToolUse` resumes working once the answer is submitted.
- Claude Code: the same mapping plus `Notification` and `Elicitation` →
  waiting. Claude has no `Interrupt` hook and does not fire `Stop` on an
  interrupt, so an interrupted turn falls back to the session prompt.
- OpenCode: no hooks.

Codex ends a plan-mode turn with `Stop`, so the hook signal alone says done
while the TUI is really waiting for "Implement this plan?". The probe therefore
looks for that prompt in the last twelve visible pane lines and reports Waiting
for input; nothing has to clear it, because the prompt disappears as soon as
the user answers or dismisses it.

Codex ends a turn on ESC but may still deliver progress hooks for tools that
were in flight. `activity.sh` therefore records the interrupted `turn_id` in
`/workspace/state/interrupted-turn` and ignores `PreToolUse`, `PostToolUse`,
`PermissionRequest`, `Notification`, `Elicitation`, and `Stop` for that same
turn, so the run returns to Ready and stays there. `UserPromptSubmit`,
`SessionStart`, and `SessionEnd` clear the marker; signals without a hook
payload keep the earlier behaviour.

Generated configuration also removes the trust dialogs that would otherwise
look like human-in-the-loop prompts: Codex runs with
`--dangerously-bypass-hook-trust` when the pinned binary supports it and its
`config.toml` pre-trusts `/workspace/project`; Claude's `.claude.json` sets
`hasTrustDialogAccepted` and `hasTrustDialogHooksAccepted` for the same path.

### Gates

- Active (blocks a second direct-checkout run): Creating, Ready, Working,
  Waiting for input; Unreachable also blocks because the state cannot be
  verified.
- Attach and Stop: Ready, Working, Waiting for input.
- Delete: refuses those three unless `--discard`; Creating records still use the
  existing sync and diff gates.
- Sync runs when a run reaches Completed.

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
  skills/
  worktrees/
```

Set `AGENTBOX_HOME` to use another state root.
