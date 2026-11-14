---
name: agentbox-cli
description: "Operate AgentBox from a shell: start, inspect, attach to, sync, and delete OpenShell-backed coding-agent runs, and recover from sandbox policy host blocks. Use when a user asks you to run or manage an agent through the agentbox CLI."
version: "{{VERSION}}"
---

# AgentBox CLI

AgentBox starts one coding agent per OpenShell sandbox, normally in its own Git
worktree. Use the CLI for every operation; the GUI started by `agentbox gui` is
a second view of the same state, not a separate control plane.

## Ground rules

- Run `agentbox --help` or `agentbox <command> --help` before guessing a flag.
- Flags come before positional arguments:
  `agentbox agent start --profile P --project R --gateway G`, not
  `agentbox agent start R --profile P`.
- Names are lower-case identifiers matching `^[a-z][a-z0-9-]{0,62}$`.
- Never paste secrets into a command, profile, or prompt. Pass environment
  variable names only, for example `--api-key-env OPENAI_API_KEY`. The variable
  must exist in the environment running AgentBox and the gateway.
- State lives under `~/.agentbox` (override with `AGENTBOX_HOME`). Prefer
  commands over editing that tree by hand.
- `--dry-run` previews the plan for `agent start` and `policy edit`; `--json`
  produces machine-readable output for read commands.

## Orient

```sh
agentbox doctor                     # platform, git/podman/openshell/npm, gateway, skill drift
agentbox project list               # registered repositories
agentbox profile list               # reusable agent/image/provider/policy configuration
agentbox agent list                 # all runs and their status
agentbox agent show RUN_ID --json   # one run: worktree, branch, policy, and status
```

## Start a run

```sh
agentbox agent start --profile PROFILE --project PROJECT --gateway GATEWAY [flags]
```

| Flag | Meaning |
| --- | --- |
| `--gateway GW` | Required OpenShell gateway; it must report the Podman driver. |
| `--name NAME` | Human-readable run name; defaults to a generated ID. |
| `--prompt TEXT` | Task for the agent. Omit it to open an interactive session. |
| `--image`, `--mode`, `--model`, `--approval` | Override the profile for this run. |
| `--mcp ENTRY` | MCP tools selection created with `agentbox mcp`. |
| `--skills ENTRY` | Host skills selection created with `agentbox skill`. |
| `--dry-run` | Print the resolved plan without creating a sandbox. |
| `--json` | Print the run record. |

Preview first when the profile is unfamiliar:

```sh
agentbox agent start --profile codex --project demo --gateway openshell \
  --prompt "Fix the failing tests" --dry-run
```

Then start it for real and keep the printed run ID.

## Create configuration when it is missing

```sh
agentbox project create NAME --root PATH          # --no-worktree to run in place
agentbox profile create NAME --agent codex --image IMAGE --mode gateway \
  --model MODEL --provider PROVIDER --api-key-env ENV_VAR
agentbox profile show NAME --json
agentbox project show NAME --json
```

Provider configuration is separate: `agentbox provider setup` registers the
inference endpoint with OpenShell. Direct `openai` and `anthropic` providers use
profile mode `standard`; OpenAI/Anthropic-compatible gateways use mode `gateway`
and require an exact model ID.

Recipe `r4` images ship a local-work toolset: `curl`, `jq`, `rg`, `unzip`,
`less`, `git-lfs`, and `ps`. They deliberately omit `gh` and `openssh-client`.
AgentBox adds no forge credential, no git credential helper, and no generated
`.gitconfig`, so do not paste a token into a profile or prompt. Sandboxes have
no forge reachability unless the run policy is edited to allow those hosts.

## Observe a run

```sh
agentbox agent logs RUN_ID        # agent log snapshot from the sandbox
agentbox agent attach RUN_ID      # live tmux session; detach with Ctrl-b d
agentbox agent diff RUN_ID        # changes the agent made in its worktree
```

`attach` streams your terminal into the existing session; detaching does not
stop the run. `agent list` is the cheapest way to answer "what needs me?" —
look for running, failed, or awaiting-approval runs.

## Collect results

```sh
agentbox agent sync RUN_ID        # copy the sandbox branch and changes into the local worktree
agentbox agent diff RUN_ID        # review before deleting
agentbox agent delete RUN_ID      # requires a synced run and a clean review
```

`sync` refuses to overwrite a worktree that changed since the last sync. Review
the diff before merging. `delete` refuses active or unsynced runs unless you
pass `--discard`; it removes the sandbox, worktree, and run branch.

## Manage images

```sh
agentbox image list               # local images and recipe revisions
agentbox image build AGENT        # build the latest release at recipe r4
agentbox image rm REF             # delete one image; refuses profiles/active runs
agentbox image prune              # delete images nothing references
```

Refreshing at newer harness versions creates new tags; it never rewrites a
profile's `image` field. `image rm` and `image prune` name any holder that
blocks deletion.

## Recover from a sandbox host block

Symptom: the agent log shows a connection, DNS, TLS, or HTTP 403 failure for a
host, or the agent reports that a domain is blocked.

1. Read the failure:
   ```sh
   agentbox agent logs RUN_ID
   ```
2. Inspect the effective policy:
   ```sh
   agentbox policy show RUN_ID
   ```
3. Add the missing HTTPS host rule for the run's agent binaries:
   ```sh
   agentbox policy edit --allow-host api.example.com RUN_ID
   ```
   Preview with `--dry-run` first when unsure. Rules are binary-scoped and
   AgentBox adds them for the known egress binaries, including node and git.
   Use `--binary /absolute/path/to/binary` only when the block comes from a
   binary outside that set.
4. Network-only changes apply live with `openshell policy set --wait`, so retry
   the operation. Static changes are written to a pending policy for the next
   sandbox; the command prints which case applies.
5. Never weaken hard isolation, disable Landlock, or bypass the sandbox to make
   a command run.

## Clean up

```sh
agentbox agent stop RUN_ID        # stop the sandbox process, keep the run
agentbox agent delete RUN_ID      # remove sandbox, worktree, and branch
```

## Report back to the user

Include the run ID or name, the project, the agent, the worktree path, and the
current status. For failures, quote the actionable line from the log and name
the exact command that will recover it.
