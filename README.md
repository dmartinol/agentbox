# AgentBox

[![CI](https://github.com/dmartinol/agentbox/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/dmartinol/agentbox/actions/workflows/ci.yml)
[![Latest stable release](https://img.shields.io/github/v/release/dmartinol/agentbox?label=latest%20stable%20release)](https://github.com/dmartinol/agentbox/releases/latest)

**Sandboxed agentic development for your Git repositories.**

AgentBox lets coding agents work on your project inside local OpenShell
sandboxes backed by Podman. Give an agent a development task, follow its work
through the CLI or web GUI, and bring the changes back for review.

Use it to investigate code, implement features, fix defects, run project checks,
and review changes. Codex, Claude Code, and OpenCode use the model provider and
project environment you configure.

AgentBox is an experimental OpenShell utility, not a supported product release.

## How it works

1. Register an existing Git checkout and configure an agent profile.
2. Prepare the repository's tools and network access when needed.
3. Start an agent with a task. By default, each run has its own branch and worktree.
4. Inspect progress, logs, and diffs; attach when the agent needs input.
5. Sync the results to the local run worktree and review them before integration.

The sandbox works on a copy of the repository. Changes return through sync;
the sandbox and host checkout are not a live shared filesystem.

## Install and start

The documentation site follows `main`. The `v0.2` tag predates the repository
setup and orchestrator configuration commands in these guides. To use the
documented features, install the current source with Go 1.26+:

```sh
go install github.com/dmartinol/agentbox/cmd/agentbox@main
export PATH="$(go env GOPATH)/bin:$PATH"
agentbox doctor
agentbox gui
```

Tagged binaries are available from [GitHub Releases](https://github.com/dmartinol/agentbox/releases);
use the documentation for the matching version when installing a release.

Running agents also requires Git, Podman, the OpenShell CLI, a running
OpenShell gateway, and access to a model provider. Building agent images
requires Node.js and npm.

Follow [Getting started](https://dmartinol.github.io/agentbox/getting-started/)
to configure your first project and run. Commands below assume that setup is complete.

```sh
agentbox agent start --project demo --profile codex-gateway \
  --gateway openshell --name fix-tests \
  --prompt "Investigate and fix the failing tests. Run the relevant checks."

agentbox agent list
agentbox agent denials RUN_ID
agentbox agent diff RUN_ID
agentbox agent sync RUN_ID
```

Replace `RUN_ID` with the ID printed at startup. `agent denials` lists the
network connections the sandbox refused and can turn a selected denial into a
run-only host rule. Sync brings work into the run's local worktree;
integrating it into your main branch is a separate step.

## Agentic development lifecycle

AgentBox supports the agentic development lifecycle (ADLC): agents plan,
change, verify, and review the project in a controlled development environment.
You can direct individual runs yourself.

An optional orchestration skill coordinates an issue-to-PR workflow with
separate workers, verification, review, and gated merge. Select AgentBox
workers when those stages should run in OpenShell sandboxes.

- [ADLC guide](https://dmartinol.github.io/agentbox/adlc/)
- [Optional orchestration](https://dmartinol.github.io/agentbox/orchestration/)
- [Example prompts](https://dmartinol.github.io/agentbox/prompts/)

## Ask your coding assistant to operate AgentBox

Install the bundled host-side usage skills, then restart your assistant session:

```sh
agentbox skills install
agentbox skills list
```

Try: “Use agentbox-cli to set up AgentBox for this repository”, or
“Use agentbox-cli to show which runs need my attention.”

The bundled skills operate AgentBox from the host. Selecting your own skills
to deliver into a sandbox is a separate feature: see the
[skill selection reference](https://dmartinol.github.io/agentbox/cli-reference/skill/).

## Documentation

- [Getting started](https://dmartinol.github.io/agentbox/getting-started/) — first task and results.
- [Repository setup](https://dmartinol.github.io/agentbox/repository-setup/) — tools, checks, and allowed hosts.
- [CLI reference](https://dmartinol.github.io/agentbox/cli-reference/) — commands and options.
- [OpenShell architecture](docs/openshell-architecture.md) — runtime, policies, credentials, and sync.
- [Contributor guide](https://dmartinol.github.io/agentbox/contributing/) — build and check the project.
- [Contributor instructions](AGENTS.md) — scope, architecture boundaries, and required checks.

## License

[Apache License 2.0](LICENSE).
