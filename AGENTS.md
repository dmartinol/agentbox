# AgentBox Project Instructions

## Product goal

AgentBox is a deliberately small, temporary utility for configuring and running
OpenShell-backed coding agents. Its purpose is to:

1. make the useful CLI operations easy to discover and execute;
2. provide a compact consolidated view of agents across projects; and
3. remain simple enough to replace with Orca once Orca supports OpenShell.

It is not a permanent fleet-management platform.

## Scope guardrails

- Do not add features because they might be useful after an Orca migration.
- Do not introduce a second orchestration model, task graph, plugin system, or
  agent-to-agent coordination layer.
- Keep the Go HTTP API as the boundary between application logic and the GUI.
- Keep the GUI compact and information-dense; avoid heavy multi-page navigation.
- Keep Fleet and Setup as the two primary GUI surfaces.
- Use the bottom output drawer for logs, diffs, policies, and task output.
- GUI attach is a required core workflow. Keep it as one xterm.js terminal
  connected to the existing tmux session; do not build a broader terminal
  emulator, multiplexer, or shell orchestration layer.
- Preserve the CLI and GUI as the only supported interfaces.
- Prefer extending an existing setup tab over adding another top-level surface.
- Do not over-generalize provider configuration; support OpenShell providers and
  the documented EnMaaS/OpenAI/Anthropic use cases.
- Avoid speculative Orca integration until Orca provides the required OpenShell
  support.

## Implementation priorities

1. Preserve simple, reliable setup and execution workflows.
2. Keep the agent fleet view readable at a glance.
3. Surface task output and errors clearly, including actionable gateway or
   dependency failures.
4. Keep tests focused on behavior and user-visible workflows.
5. Prefer small local UI primitives over adding another design system or broad
  framework abstraction.

Any proposal that expands AgentBox beyond these goals should be rejected unless
the user explicitly changes the product direction.
