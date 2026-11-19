---
layout: docs
title: Contributor guide
permalink: /contributing/
nav_sections:
  - title: Build
    id: build
  - title: Run the standard checks
    id: run-the-standard-checks
  - title: GUI changes
    id: gui-changes
  - title: Live OpenShell checks
    id: live-openshell-checks
---

Use the repository's Makefile for development. The detailed scope and review
contract for agents and contributors is in [`AGENTS.md`](https://github.com/dmartinol/agentbox/blob/main/AGENTS.md).

## Build

```sh
make build
```

This creates `./agentbox` and embeds the committed GUI from
`internal/gui/web/dist`. A Go build does not need npm; rebuild the embedded
assets when GUI source changes.

## Run the standard checks

```sh
make test
make vet
make build
```

For changes to provider setup, profiles, policies, images, run lifecycle, or
attach behavior, also run:

```sh
make race
```

The test suite uses temporary repositories and fake command runners by
default. It does not require a live OpenShell gateway.

## GUI changes

Run the frontend checks and rebuild its embedded assets:

```sh
npm --prefix internal/gui/web run test
npm --prefix internal/gui/web run build
```

## Live OpenShell checks

Live integration checks are opt-in and use the active local OpenShell gateway:

```sh
AGENTBOX_LIVE_GATEWAY=openshell make test-live
```

Optional inputs identify a repository, profile, or existing run:

| Variable | Purpose |
| --- | --- |
| `AGENTBOX_LIVE_SOURCE` | Repository to use for live checks. |
| `AGENTBOX_LIVE_PROFILE` | Profile for validation and launch preview. |
| `AGENTBOX_LIVE_RUN` | Existing run for state and policy checks. |
| `AGENTBOX_LIVE_NEW_RUN` | Set to `1` to exercise a complete new run lifecycle. |
| `AGENTBOX_LIVE_CONCURRENT_PROFILES` | Profiles for concurrent-run checks. |
| `AGENTBOX_LIVE_LOCAL_IMAGE` and `AGENTBOX_LIVE_REGISTRY_REF` | Local image and registry reference for push/pull checks. |

The suite's test setup is in
[`it/live_test.go`](https://github.com/dmartinol/agentbox/blob/main/it/live_test.go).
It reports the feasibility result in
[`IT01-REPORT.md`](https://github.com/dmartinol/agentbox/blob/main/IT01-REPORT.md).

For runtime boundaries, credential handling, and repository manifests, see the
[OpenShell architecture guide]({{ '/architecture/' | relative_url }}).
