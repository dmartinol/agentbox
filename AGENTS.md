# AgentBox Agent Instructions

This file is the contract for AI agents working in this repository. Read it
before making changes. If a proposed change conflicts with it, stop and ask the
maintainer instead of inventing a new product direction.

## Product direction

AgentBox is a deliberately small, temporary utility for configuring and running
OpenShell-backed coding agents. It has three jobs:

1. make the useful CLI operations easy to discover and execute;
2. provide a compact, consolidated view of agents across projects;
3. remain simple enough to replace with Orca once Orca supports OpenShell.

It is not a permanent fleet-management platform.

## Non-negotiable scope rules

- Do not add features merely because they might be useful after an Orca migration.
- Do not add a second orchestration model, task graph, plugin system, or
  agent-to-agent coordination layer.
- Do not add speculative Orca integration until Orca supports OpenShell.
- Keep the CLI and web GUI as the only supported user interfaces.
- Keep Fleet and Setup as the two primary GUI surfaces.
- Prefer extending an existing Setup tab over creating another top-level surface.
- Do not over-generalize provider support. Support the documented OpenShell,
  EnMaaS, OpenAI, and Anthropic flows.
- Do not embed a general-purpose terminal emulator or shell manager. GUI attach
  is one xterm.js terminal connected to the existing tmux session.
- Do not bypass the OpenShell sandbox or weaken policy enforcement to make an
  agent run.

## Architecture boundaries

- `cmd/agentbox` is the CLI entry point. Keep command parsing and presentation
  there; business logic belongs in `internal/app`.
- `internal/app` owns application behavior and must remain testable with fake
  command runners.
- `internal/gui` owns the HTTP/WebSocket API and must remain a thin boundary over
  `internal/app`.
- `internal/gui/web` owns the React GUI. It must talk to the local API rather
  than invoke CLI commands directly.
- The compiled contents of `internal/gui/web/dist` are committed so `go install`
  produces a complete binary. Rebuild and commit them whenever GUI source
  changes.
- Images, providers, profiles, policies, and run startup are documented in
  [docs/openshell-architecture.md](docs/openshell-architecture.md). Update that
  document when changing those integration points.

## GUI expectations

- Keep the UI compact and information-dense.
- Fleet is the default surface and must show all agents grouped by project.
- Setup opens as a right-side sheet, not a separate page tree.
- Logs, diffs, policies, and task output use the bottom output drawer.
- Attach uses a WebSocket to the existing tmux session and must detach without
  finishing the run.
- User-facing errors must be actionable and visible in the relevant dialog.
- Status meaning must not rely on color alone.
- Add or update Vitest/Testing Library coverage for user-visible behavior.
- Run `npm --prefix internal/gui/web run build` after GUI changes and commit the
  resulting embedded assets.

## Credential and configuration rules

- Never log, serialize, or pass a secret value.
- Store only environment-variable names such as `ENMAAS_KEY` or
  `OPENAI_API_KEY`.
- The GUI process and OpenShell gateway must be started in an environment where
  referenced credential variables exist.
- Direct `openai` and `anthropic` providers use profile mode `standard`.
- EnMaaS and other OpenAI/Anthropic-compatible gateways use profile mode
  `gateway`.
- Gateway-mode profiles require an exact model ID.
- A blank profile policy means AgentBox creates and references the default
  policy.
- Provider configuration registered with OpenShell is separate from per-run
  agent configuration. See the architecture document before changing either.

## Development workflow

Use the repository Makefile targets:

```sh
make test       # Go and frontend tests
make vet
make race
make build      # Rebuild the Go binary with the embedded GUI
make web-test
make web-build
```

Before submitting changes, run at minimum:

```sh
make test
make vet
make build
```

For GUI changes, also run:

```sh
npm --prefix internal/gui/web run test
npm --prefix internal/gui/web run build
```

For provider, profile, policy, image, run lifecycle, or attach changes, run the
race suite as well:

```sh
make race
```

## Documentation expectations

- `README.md` is the user-facing project description, installation guide, and
  usage guide.
- `docs/openshell-architecture.md` explains the OpenShell integration and the
  relationships among images, providers, profiles, policies, and runs.
- Update documentation in the same change as behavior changes.
- Keep screenshots current when the Fleet or Setup visual layout changes.
- Do not move user-facing installation or usage instructions into issue
  comments or private notes.

## Review checklist

Before declaring work complete, verify that:

- the change advances one of the three product jobs;
- no secret value appears in code, logs, tests, fixtures, or documentation;
- tests cover the changed behavior;
- the GUI remains compact and does not add a new top-level surface;
- backend logic remains in `internal/app`;
- embedded GUI assets are current;
- documentation and screenshots match the implementation;
- the working tree builds and passes the relevant test suites.
