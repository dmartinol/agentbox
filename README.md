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

## Prerequisites

- Go 1.26 or newer
- Git
- Podman
- OpenShell CLI
- An OpenShell gateway
- For image builds: Node.js and npm

## Build

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

`go test ./...` also runs a PTY-driven TUI integration test that verifies direct section navigation, back/forward history, and quitting with `q`.

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

- `F1`–`F7` switch directly between the main sections
- `Esc`, `Ctrl+[`, or `Alt+←` goes back in the navigation history
- `Ctrl+]` or `Alt+→` goes forward in the navigation history
- `Tab` moves from a table to its action bar; `Shift+Tab` moves back
- On the Dashboard, `1`, `2`, and `3` directly open Start run, Build image, and Connect gateway
- `Tab` and `Shift+Tab` move between fields and buttons in forms
- Arrow keys move within tables and tree views
- The header shows a breadcrumb and indicates when history is available
- The navigation sidebar groups sections by purpose: overview, workloads, infrastructure, configuration, and diagnostics

The TUI is organized around structured widgets rather than raw JSON output:

- Tables for runs, images, gateways, profiles, and doctor checks
- Forms for creating profiles, starting runs, building images, connecting gateways, setting up providers, and editing policies
- Tree views for nested gateway and policy data
- Modal dialogs for destructive actions
- Direct calls to the internal application layer instead of shelling out to the CLI binary

To start it:

```sh
./agentbox tui
```

## State location

AgentBox stores profiles, run records, worktrees, policies, and generated configuration outside this repository.

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
