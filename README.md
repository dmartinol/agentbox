# AgentBox

AgentBox is a local CLI and TUI for running multiple coding agents in isolated OpenShell sandboxes. It manages OpenShell-backed images, gateway policies, providers, and concurrent agent runs while keeping each run’s checkout, state, logs, and configuration separate.

This repository is an autonomous OpenShell spike. It is not a supported product release.

## Status

The current implementation provides:

- `doctor` checks for Go, Git, Podman, OpenShell, and gateway connectivity.
- `image` commands for version discovery, builds, listing, pushing, and pulling.
- `gateway` commands for listing, connecting, starting, and inspecting gateways.
- `provider` setup for OpenShell inference providers.
- `policy` commands for showing and editing sandbox policies.
- `run` commands for start, list, show, attach, logs, stop, sync, diff, and delete.
- A structured, tview-based terminal UI.

Unit tests cover the core behavior with temporary repositories and fake command runners. Live gateway, registry, policy-enforcement, and real-agent validation are not yet exercised.

## Install

### Go install

Requires Go 1.26 or newer:

```sh
go install github.com/dmartinol/agentbox/cmd/agentbox@latest
```

Ensure Go's binary directory is on `PATH`:

```sh
export PATH="$(go env GOPATH)/bin:$PATH"
```

### GitHub Releases

Download the archive for the current platform from:

```text
https://github.com/dmartinol/agentbox/releases
```

Release archives include the `agentbox` binary, README, and license, plus a `checksums.txt` file for verification.

### First run

```sh
agentbox doctor
agentbox tui
```

## Live integration tests

The default test suite uses fake external commands. Live OpenShell checks are opt-in:

```sh
make test-live
```

Optional variables:

```sh
AGENTBOX_LIVE_GATEWAY=openshell
AGENTBOX_LIVE_SOURCE=/path/to/a/git/repo
AGENTBOX_LIVE_PROFILE=my-profile
AGENTBOX_LIVE_RUN=run-abc123
```

The live suite checks:

- `doctor` dependencies and gateway connectivity
- Podman exposure from the active gateway
- at least one AgentBox image and r3 metadata for all built-in agents
- real profile validation and a launch preview when `AGENTBOX_LIVE_PROFILE` is set
- run state, base/effective policy, and loaded policy revision when `AGENTBOX_LIVE_RUN` is set
- live policy allow/deny behavior when a sandbox network binary is available
- a complete real run lifecycle when `AGENTBOX_LIVE_NEW_RUN=1` is set
- concurrent runs when `AGENTBOX_LIVE_CONCURRENT_PROFILES` is set
- registry push/pull when `AGENTBOX_LIVE_LOCAL_IMAGE` and `AGENTBOX_LIVE_REGISTRY_REF` are set
- a secret scan of the live state directory when `gitleaks` is installed

See [IT01-REPORT.md](IT01-REPORT.md) for the current live feasibility result.

## Runtime prerequisites
## Runtime prerequisites

The installed binary still requires:

- Git
- Podman
- OpenShell CLI
- An OpenShell gateway
- For image builds: Node.js and npm

## Build from source

```sh
make build
```

This creates an `agentbox` binary in the repository root.

## Test

```sh
make test
make vet
make race
```

`go test ./...` also runs a PTY-driven TUI integration test covering the layered pages/content/actions navigation model.

## Usage

Run the doctor command to inspect local prerequisites:

```sh
./agentbox doctor
./agentbox doctor --json
```

Discover the latest supported Codex version:

```sh
./agentbox image versions codex
```

Build an image with a pinned version:

```sh
./agentbox image build --version 0.167.0 codex
```

Image build help and command discovery:

```sh
./agentbox image --help
./agentbox image build --help
```

If `--version` is omitted, AgentBox discovers and uses the latest compatible release for the selected agent. The build pins the Node base image by digest and tags the result as `localhost/openshell:<agent>-<version>-r3`.

Configure an external image registry:

```sh
./agentbox image registry set quay.io/dmartino/openshell
./agentbox image registry discover
./agentbox image registry discover --json
./agentbox image registry push localhost/openshell:codex-0.167.0-r3
./agentbox image registry show --json
./agentbox image registry clear
```

The registry setting is stored in the private AgentBox state directory. Discovery and push/pull use Podman's existing registry login. In the TUI, open **Images → Registry** to set the registry and discover tags.

### Registry image model

One registry repository holds all AgentBox agent images. The agent, version, recipe, and architecture are encoded in the tag:

```text
<registry>/<namespace>/<project>:<agent>-<version>-r<recipe>-<os>-<arch>
```

Example:

```text
quay.io/dmartino/openshell:codex-0.167.0-r3-linux-arm64
quay.io/dmartino/openshell:claude-2.1.319-r3-linux-arm64
quay.io/dmartino/openshell:opencode-2.2.6-r3-linux-arm64
```

The `-<os>-<arch>` suffix is used for single-architecture images. A future multi-architecture manifest may publish the architecture-free tag as an additional alias.

Local builds use the same agent-in-tag convention:

```text
localhost/openshell:codex-0.167.0-r3
```

AgentBox derives the remote reference from the local image metadata and OCI labels. In **Images → Push**, the local image is selected from a dropdown and the remote reference is derived read-only from the configured registry.

Community consumers can discover all agents in one repository:

```sh
./agentbox image registry discover --json
```

The JSON result includes `agent`, `version`, `recipe`, `os`, `arch`, `digest` when available, and the full `reference` field.

Start a run:

```sh
./agentbox run start --profile dev --gateway openshell --source .
```

Open the TUI:

```sh
./agentbox tui
```

## Terminal UI

AgentBox uses [tview](https://github.com/rivo/tview) for the terminal UI.

Keyboard navigation is available throughout the TUI:

- `Tab` moves inward through the UI layers: pages → page content → action buttons
- `Shift+Tab` moves outward through the same layers
- `Esc` moves back one layer: buttons → page content → pages
- Arrow keys move among the elements in the current layer
- `Enter` selects or activates the focused element
- The header shows the breadcrumb, history affordances, and the current focus layer
- The focused layer is color-coded: pages are orange, content is blue, and actions are green
- The navigation sidebar groups sections by purpose: overview, workloads, infrastructure, configuration, and diagnostics

The TUI is organized around structured widgets rather than raw JSON output:

- Tables for runs, images, gateways, profiles, and doctor checks
- Forms for creating profiles, starting runs, building images, connecting gateways, setting up providers, and editing policies
- Start-run forms that show the selected profile's runtime values read-only; image, mode, model, and approval overrides are disabled until **Override profile** is enabled and start at `<None>`
- Tree views for nested gateway and policy data
- Modal dialogs for destructive actions
- Direct calls to the internal application layer instead of shelling out to the CLI binary
- Autocomplete suggestions for saved profiles, local images, gateways, providers, policies, run IDs, and models
- EnMaaS model discovery from the configured endpoint when `ENMAAS_KEY` is available

To start it:

```sh
./agentbox tui
```

## Attach behavior

The Runs page selects the first run by default, so the action bar can attach immediately. Attaching opens the run's persistent tmux session. When the agent exits, the session asks whether to keep the run idle for later reattachment or finish it. Keeping it idle leaves an interactive shell in the run checkout, with instructions for restarting the agent. Choosing finish writes the finished marker and closes the tmux session so the attaching CLI or TUI returns.

## State location

AgentBox stores profiles, run records, worktrees, policies, registry settings, and generated configuration outside this repository.

On macOS:

```text
~/Library/Application Support/agentbox
```

On Linux:

```text
$XDG_STATE_HOME/agentbox
```

Set `AGENTBOX_HOME` to use a private state directory for testing.

## Repository layout

```text
cmd/agentbox     CLI entry point
internal/app     Core application logic
internal/tui     tview-based terminal user interface
.github          CI workflow
Makefile         Build and test targets
```

## License

This project is licensed under the [Apache License 2.0](LICENSE).
