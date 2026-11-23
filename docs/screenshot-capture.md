---
layout: docs
title: Screenshot capture
permalink: /screenshot-capture/
nav_sections:
  - title: What the tour shows
    id: what-the-tour-shows
  - title: Prerequisites
    id: prerequisites
  - title: Capture
    id: capture
  - title: Update the scenes
    id: update-the-scenes
  - title: Check the result
    id: check-the-result
---

`docs/screenshots/flow.gif` is the animated product tour embedded by the README
and the documentation home page. It is captured from the built GUI with
deterministic fixtures, so it can be regenerated whenever the Fleet or Setup
layout changes.

## What the tour shows

The capture renders eight stills, each held about one second:

| Step | Scene | What it proves |
| --- | --- | --- |
| 01 | Fleet | Agents grouped by project with status, sync state, and branch. |
| 02 | Setup → Profiles | The current Setup sheet, whose tabs no longer include Policies. |
| 03 | Setup → Projects | Project registration with checkout and isolation state. |
| 04 | New agent dialog | Starting a run from a project, profile, and task prompt. |
| 05 | Project workspace | The agent tab bar, its live terminal, and the sandbox branch. |
| 06 | Logs drawer | A structured network denial above the raw agent output. |
| 07 | Add host dialog | The scoped host, port, binary, and run policy diff. |
| 08 | Applied result | The confirmation that the rule reached the live run. |

Each frame is 1440×768 of application plus a 132px caption strip, for a final
1440×900 GIF at 5 frames per second.

## Prerequisites

- Node.js 22.6 or later, for TypeScript type stripping.
- Google Chrome or Chromium. The script finds a local Chrome; set
  `TOUR_CHROME=/path/to/chrome` when it is installed elsewhere.
- `ffmpeg` on `PATH`, used for palette generation and GIF encoding.
- Installed frontend dependencies: `make web-install`.

The capture mocks `/api/**` with Playwright route interception and stubs the
attach WebSocket, so it needs no OpenShell gateway, project checkout, or live
agent. The fixtures live in
[`internal/gui/web/tests/fixtures.ts`](https://github.com/dmartinol/agentbox/blob/main/internal/gui/web/tests/fixtures.ts).

## Capture

```sh
npm --prefix internal/gui/web run tour:capture
```

The npm script builds the GUI into `internal/gui/web/dist` and then runs
[`internal/gui/web/scripts/capture-tour.mjs`](https://github.com/dmartinol/agentbox/blob/main/internal/gui/web/scripts/capture-tour.mjs).
The script serves the built assets on an ephemeral local port, drives the real
components through the eight scenes, writes the frames to a temporary
directory, and replaces `docs/screenshots/flow.gif`.

Optional environment variables:

| Variable | Purpose |
| --- | --- |
| `TOUR_CHROME` | Chrome or Chromium executable to launch. |
| `TOUR_OUT` | Output GIF path; default `docs/screenshots/flow.gif`. |
| `TOUR_KEEP_FRAMES` | Keep the temporary frames and print their path. |

## Update the scenes

Scenes are declared in order in `capture-tour.mjs` with `captureApp` and
`scene`. Each scene sets the UI state through normal Playwright interactions
(clicking Setup, filling the New agent form, typing in the workspace terminal,
selecting an agent, opening logs, opening the Add host dialog), then composes
the application frame with its caption strip. The workspace scene replies to
terminal input from `installTerminalMock`; extend that stub when the scene
needs more output.

When the GUI changes:

1. Update `appHeight` only if the tour needs a different application aspect
   ratio.
2. Change the captions or clicks in the scene block that moved.
3. Update the fixture data or the mocked API responses when a panel needs new
   fields.
4. Rerun `npm --prefix internal/gui/web run tour:capture` and review every
   scene in the result.

## Check the result

- Open the rendered README and documentation home page and confirm the
  animation plays and every scene shows the current layout.
- Confirm the GIF is still 1440×900 at 5 fps and roughly 9 seconds long.
- Keep the file near its previous size so the README stays cheap to load;
  prefer fewer palette colors over dropping scenes.
