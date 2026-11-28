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

### CLI OpenShell smoke checks

Use the checked-in fake at
`cmd/agentbox/testdata/fake-openshell/openshell` and run `make smoke-cli` for
the no-gateway provider-list smoke path. Reviewers should use this command
instead of hand-building a stub.

### Transient sandbox network failures

Inside an OpenShell 0.1.4 sandbox, a large npm install can fail once with
`EACCES` against a `198.18.0.x` address. The sandbox log shows
`transparent_tcp_mapping_denied` and `transparent TCP mapping is expired`:
the supervisor's transparent DNS mapping expires while npm keeps reusing the
mapped address. Go module downloads retry this internally; `npm ci` does not.

`make web-install` retries `npm ci` in a fresh process up to three times, and
the new process re-resolves the registry host. A retry normally recovers. If
all three attempts fail, keep the sandbox log lines above in the report.

The target prints the selected npm cache path.
A failed attempt reports `retry=<n>/3` with the first sanitized npm diagnostic line.
An unwritable `NPM_CONFIG_CACHE` or default cache path fails immediately with that path and a remediation hint instead of entering the retry loop.

## GUI changes

Run the frontend checks and rebuild its embedded assets:

```sh
npm --prefix internal/gui/web run test
npm --prefix internal/gui/web run build
```

The Fleet and Setup screenshots are generated, not hand-edited. After a layout
change, refresh the tour with `npm --prefix internal/gui/web run tour:capture`;
the procedure and its fixtures are documented in
[Screenshot capture]({{ '/screenshot-capture/' | relative_url }}).

The Setup → Projects surface also has a browser-backed responsive check:

```sh
npm --prefix internal/gui/web run smoke:responsive
```

It renders the built GUI at roughly 390px, 768px, and 1440px and fails when
project identity or checkout state disappears, the primary action leaves the
viewport, a control loses its accessible name, or the page needs horizontal
scrolling. It is not part of `make test` because it needs a browser. Set
`SMOKE_CHROME=/path/to/chrome` when no browser is found, or make the skip fatal
with `npm --prefix internal/gui/web run smoke:responsive -- --require-browser`.
In an AgentBox sandbox the check skips by design: the image ships no browser and
the policy blocks the browser download hosts.

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
