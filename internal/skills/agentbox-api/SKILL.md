---
name: agentbox-api
description: "Drive the local AgentBox HTTP API on 127.0.0.1:8080 with curl to manage projects, profiles, providers, images, gateways, policies, MCPs, skills, and agent runs, and map API results to the Fleet and Setup surfaces in the GUI. Use when a user wants programmatic control or needs to know where a setting lives in the web UI."
version: "{{VERSION}}"
---

# AgentBox HTTP API

`agentbox gui` serves a loopback-only JSON API at `http://127.0.0.1:8080`.
The React client and the API share the same Go application layer as the CLI, so
anything the GUI can do is scriptable here. There is no authentication: keep the
default loopback binding and never expose it on a public interface.

## Sample prompts

Requests like these select this skill:

- "List every agent run through the AgentBox API."
- "Start a run from a script, not the GUI."
- "What does the AgentBox API return for the overview?"
- "Where do I change the gateway setting in the web UI?"

## Conventions

- Send and receive JSON. Successful responses use `2xx`; failures return
  `{"error": "..."}` with a `4xx` or `5xx` status. Always check the status.
- Identifiers are the `id` of a run and the `name` of a project, profile,
  provider, gateway, policy, MCP, or skills entry.
- Names are lower-case identifiers matching `^[a-z][a-z0-9-]{0,62}$`.
- Credentials are environment-variable names, never values.
- Use `curl -fsS` so HTTP errors fail the command instead of printing a body.

```sh
BASE=http://127.0.0.1:8080
```

## Read the current state

```sh
curl -fsS "$BASE/api/overview"            # summary, project/run tree, runs, images, gateways, doctor
curl -fsS "$BASE/api/runs"                # all runs
curl -fsS "$BASE/api/runs/RUN_ID"         # one run
curl -fsS "$BASE/api/doctor"              # environment and skill-drift checks
```

`GET /api/overview` is the single call to make first: it answers "what is
running, what needs attention, and is the environment healthy?".

## Start and control a run

```sh
curl -fsS -X POST "$BASE/api/runs" -H 'Content-Type: application/json' -d '{
  "profile": "codex",
  "project": "demo",
  "gateway": "openshell",
  "name": "auth-refactor",
  "prompt": "Fix the failing tests",
  "approval": "prompted"
}'
```

The start payload accepts `profile`, `project`, `name`, `gateway`, `image`,
`mode`, `model`, `approval`, `prompt`, `mcp`, and `skills`; empty fields fall
back to the profile. `gateway` is required and must report the Podman driver.

```sh
curl -fsS -X POST "$BASE/api/runs/RUN_ID/sleep"          # sync, then stop the sandbox; run stays resumable
curl -fsS -X POST "$BASE/api/runs/RUN_ID/sleep?force=true"  # interrupt a working agent first
curl -fsS -X POST "$BASE/api/runs/RUN_ID/resume"         # start the sandbox or relaunch an exited agent
curl -fsS -X POST "$BASE/api/runs/RUN_ID/finish"         # sync, end the session, record Completed
curl -fsS -X POST "$BASE/api/runs/RUN_ID/sync"
curl -fsS -X DELETE "$BASE/api/runs/RUN_ID"
curl -fsS "$BASE/api/runs/RUN_ID/logs?tail=200"   # {"logs": "..."}
curl -fsS "$BASE/api/runs/RUN_ID/diff"            # {"diff": "..."}
curl -fsS "$BASE/api/runs/RUN_ID/policy"          # base, effective, global policy text
```

`POST /api/runs/RUN_ID/stop` is the deprecated alias for `/sleep`. An agent
CLI exit leaves the run `ready` with the detail `agent exited (code N); resume
available`; it does not finish the run.

`GET /api/runs/RUN_ID/attach` upgrades to a WebSocket for the live agent
terminal. Prefer the CLI (`agentbox agent attach RUN_ID`) for interactive use.

Extra sandbox shells (same worktree and policy as the run):

```sh
curl -fsS "$BASE/api/runs/RUN_ID/shells"
curl -fsS -X POST "$BASE/api/runs/RUN_ID/shells"                    # {"name": "..."}
curl -fsS -X POST "$BASE/api/runs/RUN_ID/shells/SHELL_NAME/delete"
```

## Fix a blocked host

The Fleet **Add host** action and the CLI share the exact-denial path. The
request carries the host, port, and binary from the surfaced denial; there is
no free-form host input.

```sh
curl -fsS -X POST "$BASE/api/runs/RUN_ID/policy/denial/preview" \
  -H 'Content-Type: application/json' \
  -d '{"host":"api.example.com","port":443,"binary":"/usr/bin/curl","persist":false}'

curl -fsS -X POST "$BASE/api/runs/RUN_ID/policy/denial" \
  -H 'Content-Type: application/json' \
  -d '{"host":"api.example.com","port":443,"binary":"/usr/bin/curl","persist":true}'
```

The preview writes nothing and returns the manifest entry, run-policy diff,
persist availability, and (with `persist: true`) the manifest and fragment
diffs. Applying patches the live run policy; with `persist: true` it also
appends the same rule to `.agentbox/config.yaml` and regenerates the fragment
without building an image or committing.

| Status | Body | Meaning |
| --- | --- | --- |
| `200` | `{"outcome":"live-applied","policyPath":"...","runtimeHost":{...},"persisted":false}` | Network rule applied to the live sandbox. |
| `400` | `{"error":"..."}` | Tuple is not a current denial, or the rule would use a wildcard, directory, invalid port, or secret-like value. |
| `409` | `{"error":"..."}` | Run is not Ready, Working, or Waiting; nothing changed. |
| `500` | `{"outcome":"openshell-rejected","error":"...","policyDiff":"...","rolledBack":true}` | OpenShell rejected the update and the local policy was restored. `rolledBack` is `false` when restore also failed. |


## Configuration endpoints

| Area | Endpoints |
| --- | --- |
| Projects | `GET|POST /api/projects` |
| Profiles | `GET|POST /api/profiles` |
| Providers | `GET|POST /api/providers`, `GET /api/providers/{name}/models`, `POST /api/providers/{name}/update`, `POST /api/providers/{name}/delete` |
| Images | `GET /api/images`, `POST /api/images/version`, `POST /api/images/build`, `POST /api/images/refresh`, `POST /api/images/delete`, `POST /api/images/prune`, `POST /api/images/pull`, `POST /api/images/push`, `GET /api/tasks/{id}`, `POST /api/tasks/{id}/stop` |
| Registry | `GET|POST /api/registry`, `POST /api/registry/discover` |
| Gateways | `GET /api/gateways`, `POST /api/gateways/connect`, `POST /api/gateways/start`, `GET /api/gateways/{name}/info` |
| Policies | `GET|POST /api/policies`, `GET|POST /api/policies/{name}`, `POST /api/policies/{name}/host`, `POST /api/policies/{name}/apply` |
| MCP servers | `GET|POST /api/mcps`, `POST /api/mcps/{name}/delete`, `GET /api/mcps/servers`, `GET /api/mcps/local` |
| Skills selections | `GET|POST /api/skills`, `POST /api/skills/{name}/delete`, `GET /api/skills/available?agent=codex` |
| Settings | `GET|POST /api/settings` |

Long-running image operations return a task ID; poll `GET /api/tasks/{id}`
until it reports completion instead of blocking on the build or pull request.

## Map results to the GUI

Fleet is the default surface: it groups every run by project and shows status,
the selected agent, and actions for logs, diff, policy, and attach. Setup opens
as a right-side sheet with tabs for providers, profiles, projects, images,
gateways, MCP servers, skills selections, and advanced diagnostics.

When a user asks "where do I change this?", answer with the Setup tab name and
the endpoint that backs it: profiles and projects are their own tabs, provider
registration is the providers tab, image build/pull/push and registry settings
are the images tab, gateway connection is the gateways tab, and the
platform/gateway/skill checks are diagnostics. Setup has no Policies tab:
legacy policy files are still listed and edited through `agentbox policy` and
the `/api/policies` endpoints, and run policies are read-only in the GUI from
Fleet → View policy. `Profile.Policy` is a deprecated phase-1 source of extra
network rules.
