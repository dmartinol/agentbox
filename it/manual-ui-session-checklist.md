# Manual GUI agent-flow checklist

This is a manual checklist. It is not a Go test, is not compiled by `go test`,
and must not be added to CI, `make test`, or a GitHub Actions workflow. It
requires the maintainer's local projects, profiles, provider credentials, and
OpenShell gateway.

Use it to exercise the complete Fleet -> New agent -> logs -> attach -> resume
flow through the web GUI. Record the run ID, the stored configuration, and any
new failures at the end.

## Before starting

- Build the GUI with the embedded assets: `make build`.
- Start a gateway in the same environment that holds the provider credentials.
- Start the GUI: `./agentbox gui --address 127.0.0.1:8080`.
- Confirm Podman and the OpenShell gateway are usable: `./agentbox doctor`.
- Capture the reference agent configuration:

  ```sh
  ./agentbox agent list --json > /tmp/agentbox-before.json
  ```

  Read the `test` entry and use its project, profile, gateway, agent, image,
  model, mode, and approval values for the new agent. Do not copy opaque
  identifiers or credential values into this checklist.

Reference values observed on 2026-11-21 (re-read them before each session):

| Field | Value |
| --- | --- |
| Project | `agentbox` |
| Profile | `codex-enmaas` |
| Gateway | `openshell` |
| Agent | `codex` |
| Image | `localhost/openshell:repo-github-com-dmartinol-agentbox-codex-9c13a44d` |
| Image source on `test` | `repo` |
| Mode | `gateway` |
| Model | `rits/deepseek-ai/DeepSeek-V4.1-Flash-1017` |
| Approval | `unattended` |
| MCP tools | `None` |
| Skills | `None` |

## Session checklist

Use a desktop viewport first, then repeat the layout checks at 768 px and
390 px. Keep one screenshot per checkpoint when a check fails.

- [ ] **Fleet loads.** Open `http://127.0.0.1:8080`. The `test` agent is listed
  under `agentbox`. The project header shows the expected active count. The
  header exposes a readable Doctor state.
- [ ] **Doctor state is reachable from the header.** Record whether the badge
  is a control or only a label. If it is only a label, open Setup -> Advanced
  to find the failing check.
- [ ] **New agent opens.** Click `New agent`. The dialog shows Project, Profile,
  Gateway, Agent name, MCP tools, Skills, Prompt, Resolved configuration, and
  Override image, model, and approval.
- [ ] **Configuration matches `test`.** Select the same project, profile, and
  gateway. Set a unique agent name, for example `ui-basic-flow-<date>`. Expand
  Resolved configuration and record the profile image and model. Enable
  Override image, model, and approval.
- [ ] **Image selection is unambiguous.** Choose the project image whose
  reference matches the `test` image. Read the helper text and option label.
  Record whether the helper still describes a different default image.
- [ ] **Model selection is unambiguous.** With overrides enabled, record the
  initial Model value. If it is blank, capture the request payload before
  starting and confirm whether an empty string is sent. Enter the exact model
  ID from `test`.
- [ ] **Approval matches `test`.** Select `unattended - no permission prompts`.
- [ ] **MCP and Skills stay empty.** Select `None` for both.
- [ ] **Prompt 1 is submitted.** Use:

  ```text
  Reply with exactly: hello from agentbox ui smoke
  ```

  Click `Start`. The dialog closes without an error.
- [ ] **New run is visible.** The new agent appears in Fleet and becomes
  selected. Record its status and status detail. A fast prompt is expected to
  end with the agent exited and the run resumable.
- [ ] **Stored configuration matches `test`.** Run the capture command again
  and compare the new run with `test`. Required equal fields: profile, gateway,
  agent, image, mode, model, approval. Also compare `image_source`, `base_image`,
  `image_repo`, `manifest_digest`, `policy_fragment`, and
  `policy_fragment_digest`. Record any difference.
- [ ] **Logs show the prompt response.** Click `View logs`. The raw output
  contains the prompt and `hello from agentbox ui smoke`. Network denial cards
  are shown separately from raw output.
- [ ] **Network denials are actionable.** For each denial card, attempt
  `Add host` only when it is enabled. Record whether the codex binary denials
  are rejected with a secret-like-value validation error. Do not weaken policy
  manually to make the run pass.
- [ ] **Attach opens a workspace.** Click `Attach`. The agent tab and terminal
  open. If the logs drawer is still open, record whether it remains on top of
  the terminal and blocks input.
- [ ] **Terminal input works.** Click the terminal prompt, run
  `echo attach-ok && pwd`, and confirm the output. If the agent exited, the
  shell prompt must accept input.
- [ ] **Resume opens an interactive agent.** Return to Fleet and click `Resume`.
  Wait for the Codex prompt. Send:

  ```text
  In one short sentence, what is 7*6?
  ```

  Confirm the answer and that it is the same conversation.
- [ ] **Detach preserves the run.** In the terminal, press `Ctrl-b`, then `d`.
  The terminal prints a detach message. Return to Fleet; the run is still
  available.
- [ ] **Reattach resumes the same conversation.** Click `Attach` again and
  confirm the previous answer is present.
- [ ] **Resume is blocked while live.** With the interactive agent running,
  confirm `Resume` is disabled.
- [ ] **Finish is guarded.** Do not click `Finish` until the session is over.
  Verify whether a confirmation appears. Record the result. A confirmation is
  required for a destructive one-click action.
- [ ] **Completed state is understandable.** After finishing, record the status
  badge, status detail, Sync state, and the Manage hint. Confirm that the UI
  distinguishes agent exit from user finish.
- [ ] **Delete is discoverable and confirmed.** Open `Configuration`, find
  `Delete agent`, and confirm that it asks for confirmation. Do not delete the
  reference `test` agent.
- [ ] **Configuration has the full identity.** Expand
  `Identifiers and repository`. Record whether Branch, Sandbox, Worktree, Mode,
  and Image source are present.
- [ ] **Desktop layout is usable.** At 1440 px, the agent row exposes identity,
  status, sync, branch, and actions without clipping. Long image and model
  values have a copy or reveal path.
- [ ] **Tablet layout is usable.** At 768 px, no essential field is lost or
  overlapped. Record where the branch is visible.
- [ ] **Narrow layout is usable.** At 390 px, stacked rows, the detail sheet,
  New agent, and Setup remain readable. Check for truncated values without a
  reveal path, a detail sheet bleeding through to the list, and tabs wrapping
  without clipping.

## Known failures observed on 2026-11-21

Re-run these checks on each session until they are fixed.

| Severity | Check | Observed behavior |
| --- | --- | --- |
| High | Finish safeguard | `Finish` immediately ended the run with no confirmation dialog, while `Delete agent` did confirm. |
| High | Add host for codex denials | Codex binary paths produce `hosts.binaries[0]: secret-like value is not allowed (opaque run of 40+ characters)`, so `Add host` cannot add GitHub or ChatGPT hosts. |
| High | Image provenance | Choosing a specific `Project image` in New agent stored `image_source: override` and dropped `base_image`, `image_repo`, `manifest_digest`, `policy_fragment`, and `policy_fragment_digest` compared with `test` (`image_source: repo`). |
| Medium | Exited-agent status | Fleet showed `Ready` for a run whose detail said `agent exited (code 0); resume available`. The row did not show that the agent had exited. |
| Medium | Attach vs logs drawer | Attaching while the Logs drawer was open left the modal drawer over the terminal, hiding the tmux status/prompt and intercepting clicks. |
| Medium | Branch on narrow layouts | At 768 px and 390 px the branch was absent from both the row and the main detail panel; it was only inside Configuration -> Identifiers and repository. |
| Medium | Delete discoverability | The completed-run hint says `Delete removes the record and worktree`, but Delete is only inside Configuration, not in the Manage section. |
| Medium | Doctor badge | `Doctor 1 issue` is a non-interactive Badge. Clicking it does nothing; the failing check is only in Setup -> Advanced. |
| Low | Image helper copy | After choosing a non-default project image, the helper still said `Defaults to the latest repository image: ...9435438b` while `...9c13a44d` was selected. |
| Low | Resolved configuration | Resolved configuration showed the profile image (`...codex-0.170.0-r5`) even though the default run resolves to the latest repository image; with overrides enabled it also ignored the selected override values. |
| Low | Blank model override | Enabling overrides left Model blank and the create request sent `"model": ""`; the backend silently fell back to the profile model. |
| Low | Narrow detail sheet | At 390 px the fixed detail sheet used a 95%-opaque background without a scrim, so the list underneath showed through behind the sheet header. |
| Low | Truncated Setup subtitle | At 390 px the Setup description was truncated to `Projects, profiles, providers, images, gateways, MCP to...` with only a hover title. |
| Low | Branch truncation | At desktop width the branch was middle-truncated to 28 characters (`agentbox/agent...ui-basic-flow`) even though the branch column had room for the full value; `Show full` wrapped onto a second line. |

## Record

Write the session record here or in a separate issue:

| Field | Value |
| --- | --- |
| Date |  |
| Run ID |  |
| Agent name |  |
| GUI commit |  |
| Provider/model |  |
| Prompt 1 result |  |
| Prompt 2 result |  |
| Detach/reattach result |  |
| New failures |  |

## Cleanup

- Finish or delete only the run created for this checklist.
- Leave the reference `test` agent untouched.
- Do not commit generated screenshots, run transcripts, or local paths unless
  they contain no secrets and are intentionally added to the documentation.
- Do not add this file or a script derived from it to CI.
