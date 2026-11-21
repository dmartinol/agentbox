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
localhost/openshell:codex-0.167.0-r5
localhost/openshell:claude-2.1.321-r5
localhost/openshell:opencode-2.2.6-r5
```

The `r5` suffix identifies the image recipe. The image contains:

- the agent CLI binary;
- Node.js and required runtime dependencies;
- a fixed local-work toolset: `git`, `tmux`, `ca-certificates`, `curl`, `jq`,
  `ripgrep` (`rg`), `unzip`, `less`, `git-lfs`, and `procps` (`ps`);
- OpenShell-compatible filesystem paths;
- OCI labels describing the agent, version, and recipe.

The recipe builds on a Debian trixie base. OpenShell's sandbox seccomp policy
denies the `setuid`/`setgid` family unconditionally, and GNU make before 4.4
calls `posix_spawn` with `POSIX_SPAWN_RESETIDS`, which makes glibc attempt an
identity-preserving `setuid`/`setgid` in the child and fail with `EPERM`
(`make: /bin/sh: Operation not permitted`). Trixie ships make 4.4.1, which no
longer sets that flag, so recipes and `$(shell ...)` work inside the sandbox.

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

`agentbox image rm REF` deletes the whole local AgentBox image identified by
`REF`, including every repository tag or alias attached to that image ID.
`agentbox image prune` deletes every local AgentBox image that no profile and no
non-terminal run references through any of its aliases, and reclaims superseded
dangling images that carry the `agentbox=true` label by image ID. Both refuse
images with holders and name them. Deleting an image that a profile still names
leaves that profile in place; a later run fails at start with the actionable
`image unavailable locally` error. Nothing rewrites `Profile.Image` automatically.

Holder checks live in `internal/app`. The HTTP layer maps the typed conflict to
`409` and returns a `dependents` array containing the sorted profile and run
names. The GUI disables the action with a visible reason but does not decide
whether an image is removable.

## Resource deletion guards

AgentBox keeps registrations separate from the data they refer to. The Setup GUI
uses the same application-level guard model as images:

| Resource | Removed by delete | Blocks delete | Kept by delete |
| --- | --- | --- | --- |
| Project | `projects/<name>.yaml` | any run, terminal or not | checkout, worktree, run records |
| Profile | `profiles/<name>.yaml` | a non-terminal run using it | image, policy, run records |
| Policy | `policies/<name>.yaml` | a profile reference; an existing generated default named `policies/<profile>.yaml` | profiles |
| Gateway | local OpenShell registration via `openshell gateway remove <name>` | a non-terminal run using the gateway | remote gateway, sandboxes, run records |
| MCP or Skills entry | `mcps/<name>.yaml` or `skills/<name>.yaml` | none | global/host configuration |
| Registry reference | `registry.yaml` | none | Podman images and login state |

Projects are intentionally stricter than other resources: historical run records
are enough to prevent deleting a project registration, because the Fleet view
continues to name that project. Policy guards also protect an existing generated
default even when its profile currently references another policy, so a blank
policy field can round-trip without losing the file that would be reused. After
the profile is deleted, its orphaned generated policy may be deleted.

Each guard returns a typed conflict from `internal/app`. The local API maps it
to `409` with `{"error": "...", "dependents": ["..."]}` and maps a missing
resource to `404`. React renders the list-derived reason, disables the button
with `aria-disabled`, and keeps the control keyboard focusable; it never
duplicates guard logic.

Registry clear is idempotent and removes only `registry.yaml`. It performs no
Podman logout and no image deletion. The GUI disables Clear when no saved
reference exists.

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
- `Profile.Policy`, retained for phase-1 compatibility and deprecated as a
  user-editable policy base: only its network rules participate in new runs.

### Approval defaults

New profiles created through the CLI or GUI default to
`approval: unattended`. This is a creation-time default only: AgentBox does not
migrate or rewrite existing profile approvals, and run records keep the
approval snapshot captured when the run started.

At run startup, an explicit per-run approval override takes precedence over the
profile approval. Otherwise the run uses the profile's approval. `prompted`
keeps interactive permission prompts and rejects agent permission-bypass flags.
`unattended` uses the agent's non-interactive permission flags, but AgentBox
still creates the OpenShell sandbox with its manual approval mode and verifies
the effective policy. An unattended run is refused unless the effective policy
keeps `hard_requirement` Landlock compatibility and includes
`network_policies`.

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
- The run policy derives the provider host and port from the absolute profile
  base URL. HTTPS defaults to port 443 and HTTP to port 80; no host is stored
  in the profile.
- Codex runs without its background daemon and without its nested sandbox, so
  shell network calls follow the run policy instead of Codex's default
  network-disabled sandbox.

## Repository manifest

A repository can declare its development and CI needs in a committed
`.agentbox/config.yaml` at the repository root. The manifest names four things:

1. **Image packages** — apt packages installed on top of the profile base
   image at image build time.
2. **Toolchains** — pinned HTTPS downloads with SHA-256 checksums, installed at
   image build time.
3. **Verify commands** — one-line commands that must succeed for the image
   build to pass.
4. **Hosts** — the exact runtime egress (host, port, binaries) the
   repository's tools need inside a sandbox.

The manifest is deliberately credential-free and wildcard-free. It contains no
tokens, keys, or passwords — validation rejects secret-like field names or
values anywhere in the file, including values hidden behind YAML anchors — and
hosts are exact hostnames, never `*` wildcards, CIDR blocks, schemes, or paths.
Nothing is inferred at policy-build time: what runs in the sandbox is what the
manifest declares, copied verbatim.

The file is normally authored once per repository — by an agent skill reading
the repository's build configuration, or by hand — and extended mechanically
when a new external host is needed. Everything else under `.agentbox/` stays
ignored; only `config.yaml` is committed.

Parsing and validation live in `internal/app` (`ParseRepoConfig`,
`LoadRepoConfig`). The loader reports a repository without a manifest with
`os.ErrNotExist`, distinct from validation failures, so a missing manifest and
a broken manifest are never confused.

### Schema (v1)

```yaml
schema: 1

image:
  packages:                        # apt package names, installed at build time
    - make
  toolchains:                      # pinned downloads, installed at build time
    - name: go
      url: https://go.dev/dl/go1.27.4.linux-amd64.tar.gz
      sha256: 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840
      bin: /usr/local/go/bin/go

verify:                            # image build gates; each must succeed
  - go version
  - make --version

hosts:                             # the only runtime egress; exact + binary-scoped
  - host: proxy.golang.org
    port: 443
    binaries: [/usr/local/go/bin/go]
  - host: sum.golang.org
    port: 443
    binaries: [/usr/local/go/bin/go]
  - host: registry.npmjs.org
    port: 443
    binaries: [/usr/local/bin/npm, /usr/local/bin/node]
  - host: github.com
    port: 22
    binaries: [/usr/bin/git]
```

Validation rules (every violation names the offending field):

- `schema` must be `1`.
- `image.packages` entries are plain apt package names: lowercase letters,
  digits, `+`, `-`, and `.`; URLs, version pins (`name=version`), and shell
  characters are rejected.
- `image.toolchains` entries require `name` (a lowercase identifier), `url`
  (HTTPS), `sha256` (exactly 64 hex characters, normalized to lowercase), and
  `bin` (an absolute path). Duplicate names or binary paths are rejected.
- `verify` entries are single-line, non-empty commands without leading or
  trailing whitespace or line continuations.
- `hosts` entries require an exact hostname (253 characters or fewer), a port
  from 1 to 65535, and at least one absolute binary path. OpenShell-style
  trailing `**` globs are allowed in binary paths. Duplicate host+port pairs
  are rejected; the same host on different ports is fine.
- Secret-like field names or values anywhere in the file are rejected, as are
  binary-encoded values.
- Unknown fields are rejected, so typos fail loudly instead of silently
  changing behavior.
- Every section is optional; an empty `hosts` list is valid (offline
  repositories), and a repository without a manifest keeps using profile
  images and profile policies unchanged.

AgentBox parses and validates the manifest, builds the repository environment
from it with `agentbox repo setup`, and resolves that environment at run start
(see [run image resolution](#run-image-resolution)). The remaining epic work —
full policy derivation from the gateway — is tracked in
[#42](https://github.com/dmartinol/agentbox/issues/42).

### Repository setup

`agentbox repo setup [--agent codex,claude] [--dry]` turns a validated
manifest into the two reusable artifacts runs consume:

1. **One repository image per agent**, layered on the current local base
   image for that agent (`localhost/openshell:<agent>-<version>-r<recipe>`).
   The build installs `image.packages` with apt, downloads each toolchain
   from its pinned URL after verifying its SHA-256 (archives are extracted
   under `/usr/local`, single files are installed at the declared `bin`
   path, and each `bin` directory is added to `PATH`), then runs every
   `verify` command as the sandbox user — a broken toolchain fails the
   build, not the run. Base labels are inherited, and the build also sets
   `agentbox.worker=true`, `agentbox.repo=<normalized origin remote>`,
   `agentbox.manifest-digest`, `agentbox.base=<base reference>`, and
   explicit `agentbox.agent`, `agentbox.version`, and `agentbox.recipe`
   labels so repository images never depend on label inheritance. Images
   built before the base label existed derive the same value from their
   agent, version, and recipe labels.

   OpenShell replaces the process environment when it starts a sandbox, so
   the Dockerfile `PATH` is not reliable inside agent shells. The build also
   links each toolchain binary into `/usr/local/bin` (always on `PATH`) and
   writes `/etc/profile.d/agentbox-toolchains.sh` for toolchains whose
   directory holds helper binaries. Binary-scoped network policy still
   matches the manifest's declared `bin` path because OpenShell resolves the
   link target.

2. **A policy fragment** at `$AGENTBOX_HOME/policies/repo-<repo>.yaml`,
   shared by every checkout of the repository: the `DefaultPolicy` baseline
   (filesystem, process, hard Landlock requirement) with each `hosts` entry
   copied verbatim as a binary-scoped network rule named after the host and
   port. Generation is deterministic and reruns are idempotent.

The image tag is content-addressed from the base reference plus the `image`
and `verify` sections only:
`localhost/openshell:repo-<repo>-<agent>-<hash8>`. A hosts-only manifest
change therefore keeps the previous tag and regenerates only the fragment;
changed image or verify sections produce a new tag while the previous image
remains until pruned. Rerunning setup with an unchanged manifest skips the
build entirely.

`--agent` limits the build to specific agents and defaults to the agents
present in local profiles. `--dry` validates the manifest and prints the
plan — repository identity, manifest digest, agents, and fragment path —
without running podman or writing files.

Setup never creates or edits profiles. It reports the image tag, base image,
manifest digest, and fragment path for information; run-time resolution of
the repository image and policy composition are part of run start, not
setup. The same flow is exposed to the GUI as an explicit action in
Setup → Images (Repository images) backed by `POST /api/repos/setup`,
which validates with `dry` and streams the build as a cancellable task.

### Run image resolution

Run start selects the effective image and never builds one:

- A project without `.agentbox/config.yaml` keeps the profile image and
  records `image_source: profile`.
- A project with a manifest uses the repository image whose content matches
  the manifest on a current-recipe base, preferring the newest base, and
  records `image_source: repo`. The tag hash covers the base reference, so
  resolution recomputes it for each candidate base rather than relying on
  extra image labels.
- `--image <tag>` (GUI: image override) uses that tag as-is and records
  `image_source: override`; `--image profile` forces the profile image.

The run record captures the effective image, its base image, the source, the
normalized repository identity, the manifest digest, the repository fragment
path when one was composed, and any warnings. Missing or unusable artifacts
never fail the run: it falls back to the profile image with a warning naming
`agentbox repo setup --agent <agent>`. A policy fragment that is out of date
with the manifest warns and still uses the image, so a hosts-only manifest
change does not invalidate a correct image; `agentbox repo setup` refreshes the
fragment. A repository image built on an older local base than the newest one
available warns and still runs.

Resolution reads the manifest from the selected project checkout. An isolated
run worktree carries committed and tracked changes only, so an untracked
manifest informs resolution but is not present inside the sandbox.

The GUI reads the same record. **Setup → Images** derives its
`FOUNDATIONAL`/`PROJECT <name>` badges from the worker and repository labels
and exposes the manifest digest and base reference. **New agent** matches
repository images by the project's normalized origin remote and the selected
profile's agent. Its default is **Current project image** — the `repo_status`
worker image whose tag matches the current manifest — and it sends no explicit
image override, so run start still records `image_source: repo` and resolves
the same artifact. Any other repository image is labeled as an override that
does not retain repository provenance, and **Use profile image** sends the
forced-profile opt-out. Overrides cannot start with a blank model, and the
resolved-configuration disclosure shows the effective image, model, and
approval values the start request sends. Run details render the recorded source,
base, repository, manifest digest, and warnings, and **View policy** shows the
run policy file with its repository-fragment path and digest.

## Policy layer

When a profile is created without an explicit policy, AgentBox creates and
references:

```text
$AGENTBOX_HOME/policies/<profile-name>.yaml
```

The stored file is only a generated-default marker; it may be absent at run
start and contributes nothing. A path is treated as the generated default only
when `filepath.Clean(Profile.Policy)` equals
`filepath.Clean(Store.PolicyDir()/<profile name>.yaml)`.

At run start, AgentBox derives each run policy in this order:

1. starts from `DefaultPolicy`, which owns filesystem access, the sandbox
   process user/group, and `landlock.compatibility: hard_requirement`;
2. when `Profile.Policy` is a legacy path, adds only that file's network rules;
3. when a repository image is active, unions the repository fragment's network
   rules.

AgentBox does not write the provider endpoint into the run policy. It attaches
the provider with `openshell sandbox create --provider`, and OpenShell composes
`_provider_<provider>` from the registered provider profile, including the
endpoint's host, port, path, protocol, and binary scopes. Writing the endpoint
again as a bare TCP rule makes both rules cover the same host and port with
different metadata, and OpenShell rejects the sandbox for network endpoint
ambiguity. `VerifyEffectivePolicy` still requires the composed endpoint to be
present in `openshell policy get --full` for both standard and gateway mode.

Legacy and repository rules must not re-declare the provider endpoint. AgentBox
rejects such a rule before sandbox creation and names the rule, source file,
and duplicate host:port.

Legacy and repository rules with the same canonical scope collapse to one. The
same endpoint with different binaries remains separate. Any rule with the same
name but different content is rejected. A legacy file cannot replace the
default static sections. AgentBox writes a run-specific policy, passes it to
OpenShell during sandbox creation, and verifies the effective policy before
starting the agent.

Standard mode derives its host from the built-in `openai` or `anthropic`
provider. Gateway mode derives host and port from `Profile.BaseURL`, using 443
for `https` and 80 for `http` unless the absolute URL has an explicit port.

The run records a `policy_provenance` object with its source
(`derived` or `derived+legacy`), gateway, provider, mode, host, port, the
OpenShell-composed provider rule (`_provider_<provider>`), agent binary, and
optional legacy path. Historical runs without the
object display **Not recorded (pre-#79 run)**.

The Projects API reports repository policy status read-only: manifest presence,
path, and digest; fragment path, digest, and absent/current/stale/invalid
status; manifest host rules; each agent's worker-image
current/stale/missing/unknown status; and warnings. Project registration and
setup remain the only mutation paths.

This is why a run can fail before the agent starts if the effective policy does
not allow the expected endpoint or lacks the required Landlock/network
configuration.

### Run-scoped host editing

Host remediation starts from a sandbox denial, not from a typed host. The
sandbox log emits entries such as
`NET:OPEN [MED] DENIED /usr/bin/curl(64) -> api.example.com:443` (or the
legacy `action=deny dst_host=... dst_port=... binary=...` form). AgentBox
parses those entries and surfaces the exact host, port, and binary in the
output drawer. `agentbox policy edit --allow-host HOST --port PORT
--binary /abs/path RUN_ID` and the Fleet **Add host** action both remediate
that exact tuple. The CLI requires `--binary` with `--allow-host`; the GUI
sends back the tuple from the surfaced denial, and the backend verifies that
it still appears in `openshell logs <sandbox> --source sandbox`.

A denial becomes one exact endpoint and one exact binary path. When OpenShell
reports a vendored harness path, such as a Codex child process running from
`vendor/<target>/codex`, AgentBox maps it to the canonical binary recorded in
the run's `policy_provenance.agent_binary` before validation, provided the
basenames match. The denial keeps the logged path in `observedBinary` so the
drawer and the Add-host dialog show what the sandbox reported. Globs, directory
paths, invalid ports, and secret-like values are still rejected by the same
manifest validation used for `.agentbox/config.yaml`. When a rule for
the same host and port already exists, the missing binary is appended instead
of creating a duplicate host/port entry.

The GUI endpoints are:

```http
POST /api/runs/{id}/policy/denial/preview
POST /api/runs/{id}/policy/denial
```

Both accept `{"host":"...","port":443,"binary":"/usr/bin/curl","persist":false}`.
The preview writes nothing; it returns the manifest entry, the run-policy
diff, persist availability and reason, and, with `persist: true`, the manifest
and fragment diffs. Applying uses the same `savePolicyEdit`/pending-policy
machinery as other live edits. When `persist` is selected, the backend appends
the rule to `hosts` in the repository manifest, regenerates the fragment with
`applyRepoFragment`, and never builds an image, stages, or commits. Provenance
is recorded on the run as `runtime_hosts` with `source: "run"` or
`source: "run+manifest"`; the run rule is saved first so a later persist
failure still leaves the live fix recorded.

A successful edit returns the path it wrote as `policyPath`. Reusable policy
configurations are only templates, so run copies are excluded from
`GET /api/policies`; the Setup sheet no longer has a Policies tab or fetches
policies. The profile form shows the generated default or an existing legacy
path read-only and offers **Use AgentBox default** to clear a legacy path.
`GET /api/runs/{id}/policy/content` returns that run copy as
`{"path":"...","content":"..."}` for inspection. The local policy API retains
legacy `Profile.Policy` editing and listing, but only profiles that select a
legacy file contribute its network rules; the generated default at
`policies/<profile-name>.yaml` is a marker and contributes nothing. The API no
longer offers unscoped add-host or apply actions.

The edit is allowed only when a non-dry host action reconciles the run to
Ready, Working, or Waiting. Creating, Failed, Unreachable, Stopped, Completed,
and normalized unknown statuses are rejected before any policy file changes.
Interactive edits and dry-run previews do not require a live sandbox.

Outcomes are explicit:

- a dry run returns `network-preview` or `static-preview` and never mutates the policy;
- a non-dry network-only change returns `live-applied` after OpenShell accepts it;
- a non-dry static change returns `static-pending` and saves `*-pending.yaml` for the next sandbox;
- a non-attachable run returns `no-live-sandbox` without mutating anything;
- an OpenShell rejection returns `openshell-rejected` with the computed diff.

Before applying, AgentBox writes the proposed policy locally so OpenShell can
read it. If `openshell policy set ... --wait` fails, AgentBox restores the
original local policy and reports `rolledBack: true`. If the restore also
fails, it reports `rolledBack: false` and includes both errors, so the user
knows the local policy may not match the running sandbox.

The default policy grants write access to `/workspace`, `/tmp`, and
`/var/tmp`.

## Run startup sequence

A run is created as follows:

1. Load the AgentBox profile.
2. Validate provider, model, policy, and approval settings, then decide
   whether the provider is registered with the gateway.
3. Resolve the effective image — profile, repository image, or per-run
   override — and compose the policy, recording image and policy provenance
   and any drift warnings.
4. Create an isolated Git worktree or reserve a direct checkout.
5. Save the run record.
6. Create a source Git bundle.
7. Generate run-specific agent configuration on the host.
8. Create the sandbox from the effective image, inference provider, and
   composed policy.
9. Upload the source bundle.
10. Upload and extract the configuration archive into `/workspace/state`.
11. Clone the source bundle into `/workspace/project`.
12. Verify the effective OpenShell policy.
13. Mark the sandbox ready.
14. Start tmux and the agent session.
15. Probe the new run once and cache its derived status.

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
not the agent binary. Use the denial-driven host remediation described above,
or `agentbox policy edit --allow-host ... --binary ...`, to allow an MCP
endpoint. AgentBox only edits a policy in response to that explicit action.

Setup lists the complete saved entry, including names that are no longer present
in host configuration. Editing reopens the create dialog with the name read-only,
marks unavailable current selections with a warning, and saves through the same
create endpoint. This preserves an entry whose host dependency is temporarily
missing. Deleting requires confirmation and removes only the named AgentBox
entry; the agent's global MCP configuration is retained.

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

Setup treats Skills the same way as MCP tools: the list contains all saved
skills and plugin names, Edit keeps the name read-only and preserves unavailable
selections with a warning, and Delete confirms before removing only the AgentBox
entry. Host skill folders and plugin bundles remain untouched.

### Shipped usage skills

`agentbox skills install` also writes AgentBox's own usage skills, version
matched to the installing binary, into the shared root for every detected
harness: `agentbox-cli` (run lifecycle and recovery through the CLI),
`agentbox-api` (the loopback HTTP API and where each GUI setting lives), and
`agentbox-orchestrator` (the issue-to-PR orchestrator flow, plus the harness
agent roles it dispatches). Each `SKILL.md` opens with the sample prompts that
trigger it, so an installed copy answers "how do I invoke this?":

| Skill | Trigger with a request like |
| --- | --- |
| `agentbox-cli` | "Start a codex run on the demo project" |
| `agentbox-api` | "List every agent run through the AgentBox API" |
| `agentbox-orchestrator` | "Take issue 35 to a merged pull request with the orchestrator flow" |

Because skills are version matched, a skill installed by a newer build can
instruct an agent to run a command an older `agentbox` on `PATH` does not
have. Install with the same binary that serves the commands, or upgrade with
`go install github.com/dmartinol/agentbox/cmd/agentbox@latest`.

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

When the agent CLI exits, the session wrapper writes the exit status to
`/workspace/exit-code`, signals `activity idle`, and leaves an interactive
shell in the tmux pane. Nothing prompts for terminal input, so a detached
client, a closed workspace tab, or a non-interactive launch cannot block or
finish the run. An exited agent keeps:

- the sandbox and its tmux session;
- `/workspace/state/home` (the harness session store) and the transcript;
- the run record and the worktree.

The run reads as Ready with the detail `agent exited (code N); resume
available`, and it never counts as Waiting for input. `sleep`, `resume`,
`finish`, and `delete` are explicit actions; none of them depends on terminal
input.

`bootstrap.sh` starts `session.sh initial` on the first launch. On a sandbox
start it starts `agent-entry.sh` instead, which removes the stale exit marker
and runs `session.sh resume` when the harness kept its session store, or
`session.sh fresh` (and writes `resume-fallback`) when it did not. The
bootstrap never treats pane death as completion: it only closes the tmux
session and keeps the sandbox alive.

## Derived run status

`Run.Status` carries one derived, read-only operational value and
`Run.StatusDetail` an optional explanation. The persisted pair is only a cache
so Fleet and `agent show` can render before an inspect; every read reconciles
it. There is no status setter in the CLI or HTTP API, and execution gates use
the derived value only.

The classification lives in `internal/app/status.go`. One probe exec per run
collects the tmux session and foreground command, the bottom of the visible
pane, `/workspace/idle`, `/workspace/finished`, `/workspace/exit-code`,
`/workspace/state/activity.json`, and the `agent.log` mtime. A stopped sandbox
is not probed, so a deliberate sleep is never reported as Unreachable. When the
record says AgentBox is the one that stopped the sandbox (`asleep`), the
recorded Stopped or Completed status outranks the phase: Finish closes a
sleeping run without restarting its sandbox, and a stopped sandbox cannot tell
the two apart. Resume, or any observed running sandbox, clears `asleep`.
Precedence, highest first:

1. sandbox phase `error` → Failed;
2. sandbox phase `stopped`/`exited` → Stopped;
3. sandbox phase `completed` or finished marker → Completed, with the exit code
   or "agent exited" as detail;
4. `/workspace/exit-code` → Ready, "agent exited (code N); resume available";
5. no tmux session → Creating;
6. the Codex plan-approval prompt on screen → Waiting for input, "plan ready for
   review";
7. an explicit `activity.json` signal while the session is alive → Working,
   Waiting for input, or Ready (`done`/`idle`);
8. idle marker or a shell in the tmux foreground → Ready;
9. agent in the tmux foreground → Working;
10. inspect error or unparsable response → Unreachable.

A user-issued Finish records `status_detail: "finished by user"`. The exit
marker alone cannot distinguish that from an agent exit, so Finish does not
replace the reason with an exit code, and a Completed run keeps the reason
through re-inspection.

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
- Attach: Ready, Working, Waiting for input.
- Sleep: Ready and Waiting for input. Working is refused unless `--force`,
  which interrupts the turn and waits for it to stop. Sync runs before the
  sandbox stops; a sync failure leaves the sandbox running.
- Resume: Stopped, or Ready with an exit marker. A stopped sandbox is started
  and the agent comes back through `agent-entry.sh`. Pending work is synced
  first, and a sync refusal leaves the run resumable.
- Finish: Ready, Waiting for input, or Stopped. It syncs a running agent, writes
  `/workspace/finished`, ends the tmux session, and records Completed without
  deleting the record, worktree, or transcript. Working is refused.
- Delete: refuses active runs unless `--discard`, and refuses `needs_sync` or
  `unknown` sync states. The persisted `synced` flag is not used as proof that
  newer sandbox work does not exist. Cleanup is idempotent: a worktree or branch
  that was already removed out of band is treated as clean instead of blocking
  removal of the run record.
- Reconcile syncs a run that reached Completed and an exited-but-Ready run whose
  derived sync state is `needs_sync`.

## Diff and sync

The agent works in `/workspace/project` inside the sandbox. AgentBox also owns
a local mirror worktree for each run.

- **Diff** compares the local run worktree with its baseline.
- **Sync** exports commits, patches, and untracked files from the sandbox into
  the local run worktree.
- Sync does not merge changes into the original project branch.
- Sleep, resume, and finish sync pending work before they change the sandbox
  lifecycle. A failed sync is reported and leaves the run in its prior state.

### Sync state

Each run record stores two successful-sync values in addition to the existing
`synced`, `last_synced`, and `fingerprint` fields:

- `last_successful_sync_at` is the UTC time of the last successful outbound
  sync.
- `synced_sandbox_fingerprint` is a content-aware fingerprint of the exact
  snapshot that was applied to the host worktree.

The existing fields keep their meanings: `synced` is the delete safety flag,
`last_synced` is the host branch commit SHA, and `fingerprint` is the host
worktree safety fingerprint used to refuse overwriting local changes. A failed
sync leaves the last successful timestamp and sandbox fingerprint unchanged.

Run JSON also exposes a derived, non-persisted `sync_state`:

| State | Meaning |
| --- | --- |
| `no_changes` | The sandbox is clean at the run baseline and no successful sync has been recorded. |
| `up_to_date` | The current outbound snapshot matches the last successful sync fingerprint. |
| `needs_sync` | The sandbox has outbound work and the host run worktree is unchanged. |
| `blocked` | The sandbox has outbound work but the host worktree changed, so sync will refuse to overwrite it. |
| `unknown` | The sandbox or host inspection failed; never presented as up to date. |

A run whose sandbox AgentBox put to sleep with a recorded successful sync is
`up_to_date` without a probe, because a stopped sandbox cannot produce new work
and probing it would report a correct sleep as `unknown`. Host drift is still
reported as `blocked`, and Resume clears `asleep` before the agent starts, so
later work is probed normally.

The fingerprint covers the branch, sandbox HEAD commit, the binary tracked diff,
and non-ignored untracked paths, types, modes, symlink targets, and content
hashes. The live probe reads only changed tracked content and untracked
entries; it never bundles the repository or transfers sync artifacts. Fleet
overview refreshes reuse a short-lived cache, while explicit run-detail reads
perform a fresh probe. A successful sync records the fingerprint from the exact
downloaded export before saving the run.

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
  orchestration.yaml
  worktrees/
```

Set `AGENTBOX_HOME` to use another state root.
