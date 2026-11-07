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
- A small aggregate TUI.

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
cmd/agentbox     CLI and TUI entry points
internal/app     Core application logic
.github          CI workflow
Makefile         Build and test targets
```

## License

TBD
