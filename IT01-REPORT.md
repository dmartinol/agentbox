# IT01 — Live OpenShell Feasibility Report

Date: 2026-11-09  
Host: macOS arm64  
OpenShell: 0.1.4  
Gateway: `openshell`  
Compute driver: Podman  
AgentBox live IT commit: `1a26eb2`

## Scope

This report records the live checks for the AgentBox OpenShell spike. The deterministic unit and TUI integration suites are covered by `go test ./...`; this report covers opt-in live checks run with `AGENTBOX_LIVE=1`.

## Command used

```sh
AGENTBOX_HOME=<temporary-live-state> \
AGENTBOX_LIVE=1 \
AGENTBOX_LIVE_GATEWAY=openshell \
AGENTBOX_LIVE_PROFILE=it-codex \
AGENTBOX_LIVE_RUN=<existing-run-id> \
AGENTBOX_LIVE_SOURCE=/Users/dmartino/projects/AI/ai5/agentbox \
AGENTBOX_LIVE_NEW_RUN=1 \
AGENTBOX_LIVE_CONCURRENT_PROFILES=it-codex,it-opencode \
AGENTBOX_LIVE_TIMEOUT=5m \
go test -tags live -count=1 -v -run '^TestLive' ./it
```

Result: **PASS** for all executed checks; one registry check skipped because no registry target was configured.

## Results

| Area | Result | Evidence |
|---|---|---|
| Doctor prerequisites | PASS | Git, Podman, OpenShell, npm, Podman connection, gateway checks passed |
| Gateway connectivity | PASS | OpenShell 0.1.4 connected and authenticated |
| Gateway compute driver | PASS | `gateway info` reported the Podman compute driver |
| Image inventory | PASS | Found `localhost/openshell-claude:2.1.319-r3` and other r3 images |
| r3 image metadata | PASS | Claude, Codex, OpenCode r3 images have version, base digest, recipe, and image digest |
| Image version discovery | PASS | Exact version proposal succeeded for Claude, Codex, OpenCode |
| Profile validation | PASS | `it-codex` loaded and validated |
| Run launch preview | PASS | `run start --dry-run` accepted profile, provider, image, gateway, and policy |
| Run state and policy | PASS | Base/effective policy returned; effective policy had active version and config revision |
| Policy allow/deny | PASS | Added `example.com` for `/usr/local/bin/node`; allowed fetch returned 200, `example.org` was denied |
| Codex unattended run | PASS | EnMaaS `rits/zai-org/glm-5-3` replied `OK`; idle prompt shown; finish and sync completed |
| OpenCode unattended run | PASS | Concurrent run completed in the two-agent concurrency check |
| Concurrent runs | PARTIAL | Codex + OpenCode ran simultaneously with distinct worktrees/branches; Claude unavailable |
| Idle/reattach prompt | PASS | Agent exit offered `[y]es/[f]inish`; finish path wrote finished marker and synced |
| TUI live walkthrough | PASS | Live TUI started, navigated to Runs, moved through layers, and exited with `q` |
| Secret scan | PASS | `gitleaks detect --no-git --source <state> --redact` found no leaks |
| Registry push/pull | BLOCKED | No valid registry target configured; local Podman registries config also has a parse error |
| Claude live run | BLOCKED | Claude Code reported `Not logged in · Please run /login`; EnMaaS bearer provider did not satisfy Claude auth |
| Three concurrent agents | BLOCKED | Blocked by Claude authentication; only Codex + OpenCode could run concurrently |
| Prompted live completion | PARTIAL | Native Codex folder-trust prompt was observed; fully automated prompted completion needs interactive attach |
| Docker-backed gateway | NOT AVAILABLE | Only the installed Podman-backed gateway was available |
| Linux build/runtime | NOT RUN | Spike validation was performed on macOS arm64 |

## Findings

1. AgentBox is live-feasible for Codex and OpenCode with the existing Podman-backed OpenShell gateway and EnMaaS provider.
2. Unattended Codex execution works against EnMaaS with the tested model.
3. Live policy allow/deny enforcement works with hard Landlock and network policy revision updates.
4. The idle/reattach prompt works; finish writes the marker, syncs, and returns the attaching client.
5. TUI layered navigation remains usable against live state.
6. Claude Code is blocked until an Anthropic credential or supported native login reuse path is provided.
7. Registry push/pull remains unverified because this workstation has no usable registry target and the local Podman registries config is malformed.
8. Three-agent concurrency cannot be claimed until Claude authentication is resolved.
9. Prompted native approvals were observed, but prompted runs require an interactive attach to complete reliably.
10. Docker-backed gateway and Linux runtime remain unverified.

## Conclusion

IT01 is complete as a feasibility pass with recorded unavailable cells. It does **not** establish full three-agent acceptance.

Supported evidence today:

- macOS arm64
- Podman-backed OpenShell gateway
- Codex and OpenCode concurrent runs
- EnMaaS unattended model execution
- live network policy allow/deny
- idle/reattach and finish/sync
- live TUI navigation
- clean secret scan

Outstanding blockers:

- Claude authentication
- registry push/pull
- three-agent concurrency
- Docker gateway validation
- Linux runtime validation
