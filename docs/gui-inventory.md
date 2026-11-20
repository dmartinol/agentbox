# GUI screen inventory and detail hierarchy

This document records how Fleet and Setup divide scan-level content from
on-demand detail. Keep it current when a screen changes. The general review
checklist lives in [AGENTS.md](../AGENTS.md#layout-and-accessibility-checklist).

## Shared patterns

- **Scan row**: identity, status, blockers, and one primary action.
- **Row-detail transition**: selecting an agent opens the 24rem detail panel;
  on narrow screens the same panel becomes a bottom sheet with an explicit close
  button. Do not create a second detail implementation. Opening the output
  drawer on a narrow screen dismisses the detail sheet instead of stacking two
  overlays. Full configuration opens as a read-only modal from the panel, so
  Fleet keeps one sidebar and no new route.
- **Expandable value**: long paths, IDs, references, and lists shorten in the
  middle and reveal their full value inline. Every value has a copy control and
  remains selectable.
- **Disclosure**: resolved defaults, dependency explanations, and diagnostic
  output sit behind a native `<details>`/`<summary>` pair.
- **Output drawer**: logs, diffs, policies, and task output share one bottom
  drawer with distinct loading, empty, error, and content states. ANSI control
  sequences are normalized before display.
- **Action hierarchy**: one visible primary action; secondary actions grouped by
  task; destructive actions separated and named.
- **Responsive tables**: when a table's own container is narrower than 960px it
  becomes compact cards. Every card leads with its identity and keeps the rest
  of the row's metadata inline on the same line, wrapping only when space runs
  out. Actions move to their own line. Visible field captions are omitted as
  self-evident; the same names stay attached to cells for assistive technology.
  Fleet cards therefore read as name, type, status, and sync, with the branch
  left to the detail panel. Wider containers keep the same DOM as a table, so
  there is no duplicate markup or duplicate action cluster. The container
  query, rather than a viewport width, also accounts for the Fleet detail
  sidebar and the Setup sheet.

## Fleet and agent flows

| Surface | Scan level | Detail reveal | Narrow layout |
| --- | --- | --- | --- |
| Fleet toolbar | Search, project filter, Needs You count, refresh, New agent | — | Wrapping rows; search and filter span the width |
| Project group | Project name, counts, Workspace entry | Agent rows | Wrapping header badges and actions |
| Agent row | Name, type, operational status, derived sync state, branch, primary action | Selecting the row (click, or Enter/Space on the focused row) opens the detail panel | Name plus type/status/sync on one line; sync state keeps its text and glyph; actions beneath; branch in the detail panel |
| Agent detail panel | Identity, operational status, project, harness, running model, profile, image | Image-source qualifier and drift warnings; Attach, Logs, Diff, Policy, and Configuration; Sync and Stop; sync state with last-sync time and explanation | Bottom sheet with close control; same grouped actions |
| Agent configuration | — | Read-only New agent field grid (project, profile, gateway, name, MCP, skills, image, model, approval); identifiers and repository disclosure; runtime host rules disclosure; Copy agent and Delete agent in the footer | Single-column form; footer actions wrap |
| Workspace | Project, agent tabs, shell tabs | Terminal or cached log tail | Horizontally scrollable tabs; terminal fills the surface |
| New agent | Project, profile, gateway, name, MCP, skills, prompt | Resolved image/model/provider/approval disclosure; image select defaults to the project's latest repository image with a profile-image opt-out; optional overrides | Single-column form; the image select spans the row and actions wrap |
| Logs / diff / policy drawer | Title and task status; structured **Network denials** above raw output | Denial host, port, binary, and reason with **Add host**; Add-host dialog shows the manifest entry, run-policy diff, optional persist, and manifest/fragment diffs; run policy shows textual provenance before the YAML | Full-width drawer; denial cards and dialog fields wrap and scroll |

## Setup tabs

| Tab | Scan level | Detail reveal | Narrow layout |
| --- | --- | --- | --- |
| Providers | Name, type, credential-env name, usage count | Full credential names, full usage lists, edit/delete | Compact card: identity plus inline metadata; actions beneath |
| Profiles | Name, agent, mode, model, provider | Expandable model, edit/delete; deprecated legacy-policy selector | Compact card: identity plus inline metadata; actions beneath |
| Projects | Name, checkout state, isolation | Expandable Git root; repository-details disclosure with checkout state and reason, the local-refs/no-fetch basis, and manifest, fragment, host-rule, worker-image, and warning status; edit/delete | Compact card: identity plus inline metadata; checkout and repository status wrap in readable text |
| Images | Kind badge (`FOUNDATIONAL` or `PROJECT <name>`), reference, agent, version, recipe, platform | Expandable reference, repository provenance disclosure (manifest digest, base image), delete; registry matches; repository setup (project, agents, validation plan, streamed build) | Compact card: identity plus inline metadata; actions beneath; provenance disclosure and setup form wrap with their actions |
| Gateways | Name, status, type | Expandable endpoint, inspect payload, remove | Compact card; toolbar wraps |
| MCP Tools | Name, agent, server names | Expandable server list, edit/delete | Compact card; explanatory panel below |
| Skills | Name, agent, skills, plugins | Expandable lists, role installations, edit/delete | Compact card; role table uses the same pattern |
| Advanced | Worktree settings, Doctor summary | Doctor check disclosure with complete output, action, and copy | Single-column fields; check rows stack |

## Dialog and state rules

- Create/edit dialogs keep required inputs first; resolved or advanced values
  belong in a disclosure. Destructive confirmations name the object, say what
  is retained, and stay separate from secondary buttons.
- Errors render in the dialog or drawer where the action happened.
- Empty and loading states use explicit text, not an empty pane.
- Status badges include text and a glyph; color is supplementary.
