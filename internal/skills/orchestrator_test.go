package skills

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// testFooterIcon is the literal AgentBox mark every provenance footer opens
// with, kept in one place so the assertions cannot drift from the templates.
const testFooterIcon = `<img src="https://raw.githubusercontent.com/dmartinol/agentbox/main/docs/assets/agentbox-icon.svg" width="14" height="14" align="absmiddle" alt="AgentBox">`

var forbiddenForgeWording = regexp.MustCompile(`(?i)\b(?:gh|credentials?|token)\b`)

// rawSkill returns the unrendered embedded file so tests can assert on the
// literal front matter syntax.
func rawSkill(t *testing.T, name string) string {
	t.Helper()
	body, err := embedded.ReadFile(name + "/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// collapsed returns the rendered orchestrator skill with whitespace collapsed
// so assertions do not depend on line wrapping.
func collapsed(t *testing.T) string {
	t.Helper()
	content, err := Embedded("agentbox-orchestrator", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(strings.Fields(string(content)), " ")
}

// wantAll fails when body is missing one of the expected fragments.
func wantAll(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestOrchestratorSkillFrontMatter(t *testing.T) {
	content, err := Embedded("agentbox-orchestrator", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	fm, err := ParseFrontMatter(content)
	if err != nil {
		t.Fatalf("front matter does not parse strictly: %v", err)
	}
	if fm.Name != "agentbox-orchestrator" {
		t.Fatalf("name = %q", fm.Name)
	}
	if fm.Version != "9.9.9" {
		t.Fatalf("version = %q", fm.Version)
	}
	if strings.TrimSpace(fm.Description) == "" {
		t.Fatal("description is empty")
	}
	if strings.ContainsAny(fm.Description, "\n\r") {
		t.Fatal("description is not a single line")
	}
	// The description must be a quoted scalar in the source file, unlike the
	// unquoted style that shipped before issue #30.
	for _, line := range strings.Split(rawSkill(t, "agentbox-orchestrator"), "\n") {
		if strings.HasPrefix(line, "description:") {
			if !strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(line, "description:")), `"`) {
				t.Fatalf("description is not quoted: %s", line)
			}
			return
		}
	}
	t.Fatal("no description line in the front matter")
}

// TestOrchestratorSkillRoleBoundary pins the MUST boundary: orchestration and
// optional capability-driven forge management, never inline stage work.
func TestOrchestratorSkillRoleBoundary(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Role boundary (MUST)",
		"Task orchestration (mandatory):** dispatch triage and every stage's worker",
		"Git and forge management (optional, capability-driven):**",
		"MUST NOT triage, plan, implement, fix, verify, review, or run retro inline",
		"`agentbox-orchestrator` role that loads this skill as its source of truth",
		"never dispatches a second orchestrator",
		"If it starts doing stage work, it stops and dispatches a worker",
		"A resumed run redispatches the stage; it never continues a previous worker's session",
	)
}

// TestOrchestratorSkillSelection covers asking once, recording every runtime
// and model selection, confirming inherited defaults, and refusing silent
// substitution.
func TestOrchestratorSkillSelection(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Runtime and model selection",
		"Ask once during triage",
		"Confirm an inherited default explicitly before recording it",
		"| Orchestrator | runtime (`orca`, `agentbox`, or `harness`), agent, model, effort |",
		"| Triage worker | runtime, agent, model, effort |",
		"| Plan worker | runtime, agent, model, effort |",
		"| Implement worker | runtime, agent, model, effort |",
		"| Fix worker | runtime, agent, model, effort |",
		"| Verify worker | runtime, agent, model, effort |",
		"| Review worker | runtime, agent, model, effort |",
		"| Retro worker | runtime, agent, model, effort |",
		"The orchestrator cannot switch its own model from inside the turn",
		"sets `blocked` with the exact reason",
		"Never fall back to a default, a cheaper model, or a different agent",
		"Reviewer independence",
		"Reasoning effort is per runtime",
		"AgentBox worker cannot apply one",
		"records `effort: not-applicable`",
		"If the user asks for a specific effort for an AgentBox worker, set `blocked` with the exact reason",
		"Never record an effort that was not applied",
	)
}

// TestOrchestratorSkillRegistryConfiguration covers the user-owned registry,
// deterministic layer precedence, approval, model discovery, and provenance.
func TestOrchestratorSkillRegistryConfiguration(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Registry-backed orchestration configuration",
		"`$AGENTBOX_HOME/orchestration.yaml`",
		"`~/.agentbox/orchestration.yaml`",
		"never contains credentials",
		"agentbox orchestrator config show --project",
		"`default` names the configuration used when no global or project layer applies",
		"The fixed stage groups are `coding` for `plan`, `implement`, and `fix`",
		"`other` for `triage`, `verify`, `review`, and `retro`",
		"Project entries match the canonical absolute `root`",
		"the CLI reports the ambiguity and never picks one",
		"Registry resolution order (later wins, field by field)",
		"Within one layer, a stage resolves `workers.default`, then its group, then `workers.stages.<stage>`",
		"`model: \"\"` clears an inherited model",
		"Workflow precedence, highest first",
		"explicit choices in the current prompt",
		"the matching registry project layer",
		"If the prompt explicitly says **“use the existing configuration”**",
		"ask for approval before dispatch",
		"If no configuration applies or required choices are missing, ask for the missing orchestration runtime",
		"`agentbox profile list --json`",
		"Orca exposes no local model catalog",
		"Never invent or silently substitute a runtime, agent, profile, or model",
		"Record the resolved configuration, the applied layers, and the per-stage sources in the state record",
		"### Saving a project-scoped configuration",
		"A configuration authored for one repository belongs in that repository's `projects[]` entry",
		"Never put a project request into `global`, into `configs[default]`",
		"Prefer `remote` for a shared project entry",
	)
}

// TestOrchestratorSkillConfigurationAuthoring covers the create/update mode:
// an explicit interview, project-scoped writes, stale-binary handling, and
// post-save scope verification.
func TestOrchestratorSkillConfigurationAuthoring(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Configuration authoring (MUST)",
		"is a configuration-authoring request, not a workflow",
		"do not treat an existing registry as permission to skip the interview",
		"The registry file itself is always the user-owned file under `$AGENTBOX_HOME`",
		"“Project scope” means a `projects[]` entry in that file, not a file inside the repository",
		"If the command is unknown, stop and ask the user to install or build the current AgentBox binary",
		"Resolve the current registry for the repository",
		"Use the result only as context and as proposed defaults labelled with their source",
		"It does not replace the interview",
		"Interview before writing",
		"orchestrator runtime, agent, model, and effort",
		"worker runtime, agent, profile (when the runtime uses one), model, and effort",
		"any `coding` or `other` group difference",
		"any per-stage override",
		"State the allowed runtime values (`orca`, `agentbox`, `harness`) and agent values (`claude`, `codex`, `opencode`)",
		"Confirm the scope, asking only when the prompt does not name one",
		"Ask for each setting, or present a table of proposed values with their sources and ask the user to confirm or change each one",
		"Do not replace the interview with a single “use these values?” question",
		"`agentbox profile list --json`",
		"Never invent a model, profile, runtime, effort, or credential value",
		"Honour the scope named in the prompt",
		"A project request MUST write a `projects[]` entry for the current repository",
		"It MUST NOT write the answers into `configs[default]`, `global`, or a named config shared with other projects",
		"If the working directory has no Git root or origin, ask which repository the entry is for and use its absolute `root`",
		"Write every confirmed answer into the project entry, including an answer that happens to match an inherited default",
		"Show the exact YAML or diff, the registry path, and the effective selections",
		"Wait for explicit confirmation, then write it",
		"Validate and re-resolve with the same `--project` and `--remote` values",
		"require `\"scope\": \"project\"` and `\"project\": \"<name>\"`",
		"check that every answered selection reports a `project:<name>` source",
		"If a value still resolves from `default` or `global`, the save is incomplete",
		"Do not report success until the scope and sources match the request",
		"Report the registry path, scope, project name, and a compact table of the saved selections",
	)
}

// TestOrchestratorSkillWorkflowApproval covers the workflow gate: an existing
// registry never replaces explicit approval, and a mis-scoped project request
// is a failure.
func TestOrchestratorSkillWorkflowApproval(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"always ask for approval before dispatch, even when the configuration resolves every required choice",
		"An existing file, a previous workflow, or a set of inherited defaults is not approval",
		"Honour a scope named in the prompt: a project request saves under `projects[]` and never in `global` or `configs[default]`",
		"A value that still resolves from `default` or `global` means the save missed the project layer",
		"Report a `default` or `global` scope for a project request as a failure, not a success",
		"A prompt that asks to create or change the registry is handled by **Configuration authoring** and does not enter this gate",
	)
}

// TestOrchestratorSkillStageDispatch covers per-stage workers, isolated
// worktrees, and fresh verify and review worktrees at the PR head.
func TestOrchestratorSkillStageDispatch(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Stage ownership and isolation",
		"| triage | worker of the selected type, tracker-blind | its own new worktree | none; it returns a disposition artifact |",
		"| plan | worker of the selected type | its own new worktree |",
		"| implement | worker of the selected type, branch `orchestrator/<issue>-<slug>` |",
		"| fix | worker of the selected type, existing issue branch |",
		"| verify | fresh worker | fresh detached worktree at the full pull request head SHA |",
		"| review | fresh worker | fresh detached worktree at the full pull request head SHA |",
		"| retro | fresh worker, tracker-blind | its own new worktree |",
		"| merge / close | orchestrator |",
		"Every stage except merge/close runs in a worker",
		"Triage, plan, implement, fix, verify, review, and retro are never performed by the orchestrator turn",
		"fresh detached worktree created at the full 40-character pull request head SHA",
		"must not share the implementer's worktree and must not see uncommitted state",
		"The fix cycle is a distinct stage. Dispatch `agentbox-fixer`, not a new implementer",
		"A rebase or force-push invalidates prior verification and review evidence",
	)
}

// TestOrchestratorSkillFeedbackFollowUps pins the flow changes made after the
// first long orchestration run: a simple-fix fast path, completed stage
// records, concurrent verify and review, and the state comment's purpose.
func TestOrchestratorSkillFeedbackFollowUps(t *testing.T) {
	wantAll(t, collapsed(t),
		"`ready-simple`",
		"`dispatch: skipped-simple`",
		"at most three files and roughly 150 changed lines",
		"no schema, API-contract, migration, or security change",
		"every successful stage ends `Completed`",
		"agentbox agent finish RUN_ID",
		"gates 4 and 6 may overlap once the pull request exists",
		"it must not post a verdict until the verify result at the reviewed commit is recorded and green",
		"single hidden `<!-- orchestrator-state",
		"followed by one visible status line",
		"no tool parses it",
		"Codex provisions its helper aliases under `$CODEX_HOME/tmp`",
		"Do not accept a hand-built patch shim as the fix",
	)
}

// TestOrchestratorSkillPinsExactHeadSHA requires verify and review workers to
// prove that HEAD is the requested commit, not merely a tree-equal snapshot.
func TestOrchestratorSkillPinsExactHeadSHA(t *testing.T) {
	for _, name := range []string{"verify-task.md", "review-task.md"} {
		content, err := EmbeddedFile("agentbox-orchestrator", name, "9.9.9")
		if err != nil {
			t.Fatal(err)
		}
		body := strings.Join(strings.Fields(string(content)), " ")
		wantAll(t, body,
			"`git rev-parse HEAD`",
			"detach or check out",
			"`git diff <reviewed-sha>..HEAD --stat`",
		)
	}

	verifyContent, err := EmbeddedFile("agentbox-orchestrator", "verify-task.md", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	verifyBody := strings.Join(strings.Fields(string(verifyContent)), " ")
	wantAll(t, verifyBody, "commit: <full-sha actually checked>")

	wantAll(t, collapsed(t),
		"An AgentBox run starts on an empty `AgentBox starting snapshot` commit with an identical tree, so the worker must compare `git rev-parse HEAD` with that SHA and detach to it before any gate; record the SHA actually checked, not only the requested one.",
	)
}

// TestOrchestratorSkillAgentBoxVerifyAndReviewUsePRHead pins the AgentBox
// dispatch rule for verify and review: a fresh detached worktree at the full
// PR head SHA passed with --source, while --project keeps the run attributed.
func TestOrchestratorSkillAgentBoxVerifyAndReviewUsePRHead(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"For verify and review, pin the checkout at the pull request head with `--source`",
		"`--project` alone would use the registered project's current checkout",
		"`--source` supplies the checkout while `--project` supplies attribution",
		"Create one fresh detached worktree per stage at the full head SHA",
		"HEAD_SHA=$(gh api repos/OWNER/REPO/pulls/PR --jq .head.sha)",
		`git -C REPO worktree add --detach VERIFY_WORKTREE "$HEAD_SHA"`,
		`git -C REPO worktree add --detach REVIEW_WORKTREE "$HEAD_SHA"`,
		`agentbox agent start --profile PROFILE --project PROJECT --source VERIFY_WORKTREE --require-head "$HEAD_SHA" --gateway GATEWAY`,
		`agentbox agent start --profile PROFILE --project PROJECT --source REVIEW_WORKTREE --require-head "$HEAD_SHA" --gateway GATEWAY`,
		"Record the same full SHA in each stage entry",
		"Keep both worktrees until the verify and review evidence is captured and the final report is delivered",
		"clean up afterwards unless the user asked to keep them",
	)
}

// TestOrchestratorSkillOrcaVerifyAndReviewUsePRHead pins the Orca dispatch
// rule: each stage gets its own fresh child worktree based on the full PR head
// SHA, never the implementer worktree or the project checkout.
func TestOrchestratorSkillOrcaVerifyAndReviewUsePRHead(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"For verify and review, start each stage in its own fresh child worktree pinned to the full pull request head SHA",
		"Never reuse the implementer's worktree or the project checkout, whose HEAD may not be the pull request head",
		`HEAD_SHA=$(gh api repos/OWNER/REPO/pulls/PR --jq .head.sha)`,
		`--worktree new-child --base-branch "$HEAD_SHA"`,
		"--name ISSUE-verify --agent AGENT --model MODEL --effort EFFORT",
		"--name ISSUE-review --agent AGENT --model MODEL --effort EFFORT",
		"creates a separate fresh worktree for each command",
		"Record the same full SHA and each worker handle in the stage entries",
		"Keep both worktrees until the verify and review evidence is captured and the final report is delivered",
		"clean up afterwards unless the user asked to keep them",
	)
	if got := strings.Count(body, `worker-start --worktree new-child --base-branch "$HEAD_SHA"`); got != 2 {
		t.Fatalf("expected one pinned worktree per verify and review stage, got %d", got)
	}
}

// TestOrchestratorSkillHarnessVerifyAndReviewUseDetachedWorktree pins the
// harness dispatch rule: a fresh detached worktree at the full PR head SHA,
// with the worker confined to that path.
func TestOrchestratorSkillHarnessVerifyAndReviewUseDetachedWorktree(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"For verify and review, create one fresh detached worktree per stage at the full pull request head SHA",
		"launch the worker with that worktree as its working root",
		"The worker spec, reads, edits, and gate commands must stay inside that path",
		"never run the worker in the implementer's worktree or the project checkout",
		`git -C REPO worktree add --detach VERIFY_WORKTREE "$HEAD_SHA"`,
		`codex exec -C VERIFY_WORKTREE --model MODEL - < VERIFY_SPEC`,
		`git -C REPO worktree add --detach REVIEW_WORKTREE "$HEAD_SHA"`,
		`codex exec -C REVIEW_WORKTREE --model MODEL - < REVIEW_SPEC`,
		"Do not add the host repo or the implementer's worktree as an additional writable root",
		"Record the same full SHA and each worker or terminal handle in the stage entries",
		"Keep both worktrees until the verify and review evidence is captured and the final report is delivered",
		"clean up afterwards unless the user asked to keep them",
	)
}

// TestOrchestratorSkillGateDispatchRule pins which gates dispatch workers and
// that pull request creation is not a worker stage.
func TestOrchestratorSkillGateDispatchRule(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"Gates 1, 2, 3, 4, 6, 7, and 10 each dispatch a worker",
		"gate 5, pull request creation, is performed by the host implementer or the orchestrator",
		"No stage work runs inline",
	)
	if strings.Contains(body, "Gates 2 through 6 each dispatch a worker") {
		t.Fatal("gate rule still claims every gate from 2 through 6 dispatches a worker")
	}
}

// TestOrchestratorSkillStageForgeMatrix pins the single stage-aware forge
// matrix: one row per stage, no worker gains tracker authority, and only a
// host implementer may open the pull request besides the orchestrator.
func TestOrchestratorSkillStageForgeMatrix(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Stage-aware forge matrix",
		"| Stage | Worker | Local commit | Remote push | Open the PR | Workflow comments / verdicts | Merge / close | Tracker labels, state comment, issue close |",
		"| triage | tracker-blind worker | no | no | no | no | no | no |",
		"| plan | worker | no | no | no | no | no | no |",
		"| implement (sandbox) | sandbox worker | yes, local only |",
		"| implement (host) | Orca child worktree or harness host worker | yes | yes, only `orchestrator/<issue>-<slug>` | yes, with `pr.md` | no | no | no |",
		"| fix (sandbox) | fixer |",
		"| fix (host) | fixer | yes | yes, only the existing pull request branch | no |",
		"| verify | fresh worker | no | no | no | no; it writes the `stage:verify` artifact handoff that the orchestrator posts |",
		"| review | fresh worker | no | no | no | no; it writes the verdict artifact handoff that the orchestrator posts |",
		"| merge / close | orchestrator | no | no | no | yes | yes | yes |",
		"| retro | tracker-blind worker | no | no | no | no | no | no |",
		"the orchestrator syncs the run into the host worktree, verifies the branch head, pushes it, and opens the pull request",
		"it may commit, push, and open the pull request itself",
		"a push somewhere other than `orchestrator/<issue>-<slug>` is a blocked outcome",
		"No worker gains tracker authority, including a capable host implementer",
		"opening the pull request is the only implement-stage forge write a host worker may perform",
		"**Triage, plan, and retro:** they perform no forge or tracker writes",
	)
	if strings.Contains(body, "## Forge capability matrix") {
		t.Fatal("forge ownership still uses the old worker-type capability matrix")
	}
	if strings.Contains(body, "orchestrator remains the only PR creator") {
		t.Fatal("forge ownership still claims the orchestrator is the only PR creator")
	}
}

// TestOrchestratorSkillLaunchTopology pins the controller/worktree/worker
// distinction and the supported direct-start paths, so no extra launcher hop
// is documented as the default.
func TestOrchestratorSkillLaunchTopology(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Launch topology and visibility (MUST)",
		"the **controller**: the agent session that runs this skill",
		"the **worktree**: an Orca-managed checkout of a branch at a path; creating one starts no agent and runs no prompt",
		"the **worker**: the agent process launched inside a worktree to run one stage",
		"Creating a worktree is preparation, not dispatch",
		"Do not start an agent and then have that agent launch a second non-interactive agent process to do the same stage",
		"That nested launch adds a hop, hides the real worker, and makes its output unobservable",
		"| Claude Code | `claude --agent agentbox-orchestrator` | one process; no launcher |",
		"| OpenCode | `opencode run --agent agentbox-orchestrator`, or select the primary agent in a session | one process; no launcher |",
		"| Codex | ask the top-level session to use the `agentbox-orchestrator` custom agent | top-level parent plus controller subagent; Codex has no top-level `--agent` switch |",
		"| Orca | `--agent` selects only the host runtime; assign the role through the started harness | runtime plus harness role assignment |",
		"Start the assigned controller directly when the harness supports it",
		"Do not add a launcher session whose only job is to spawn the controller and relay its answer",
		"keep that parent lightweight and visible",
		"hidden non-interactive process whose output exists only in a temporary file",
		"never pass `agentbox-orchestrator` as the `--agent` runtime ID",
		"assign the custom role afterwards through that harness",
		"documented limitation. Start that harness directly",
	)
}

// TestOrchestratorSkillRuntimeProfileConstraints pins the tested Orca paths
// and the rule that an unavailable profile or model is a blocker, not a silent
// downgrade.
func TestOrchestratorSkillRuntimeProfileConstraints(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"### Runtime, profile, and model constraints",
		"A profile is a harness-level setting, not an Orca runtime ID",
		"`orca orchestration worker-start` and `orca agent launch` accept `--model` and `--effort` but have no `--profile` flag",
		"They cannot apply a Codex profile",
		"`orca worktree create --agent ... --model ... --effort ...` creates the checkout and starts the agent, but it also cannot set a harness profile",
		`orca terminal create --worktree <selector> --title "<stage>" --command "codex -p PROFILE -m MODEL" --json`,
		"Record the returned terminal handle, follow it with `orca terminal read`",
		"Never drop a requested profile, model, effort, or runtime to make a worker start",
		"set `blocked` with the exact unsupported selection and the single action that unblocks the flow",
	)
}

// TestOrchestratorSkillWorkerVisibilityAndRetention pins the default visible
// worker path, the stage updates a user can follow, and retention until the
// final report.
func TestOrchestratorSkillWorkerVisibilityAndRetention(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"### Worker visibility, retention, and cleanup",
		"When Orca is selected for workers, the default worker path is an Orca-visible supervised worker or terminal",
		"not a headless `codex exec` whose only output is redirected to a temporary file",
		"Start each Orca stage with `orca orchestration worker-start ... --json` and record the returned worker handle, worktree, and branch",
		"Report a stage-start update with the stage, worktree, branch, and worker or terminal handle",
		"Report completion or a blocker with the same handle and the evidence location",
		"`orca orchestration worker-show`, `orca orchestration worker-read`, `orca terminal read`, or `orca worktree show`",
		"keep a durable log outside `/tmp`",
		"`~/.agentbox/orchestrator/<workflow_id>/<stage>.log`",
		"never make a temporary log the only way to follow the worker",
		"Retain every worker session and worktree until the stage evidence is captured and the final report is delivered",
		"Honour an explicit request to keep them",
		"clean up only after the final report",
		"never delete a branch whose commit SHA is not recorded",
	)
}

// TestOrchestratorSkillSafetyAndWorktreeCreation pins the no-bypass rule and
// the create-verification sequence that prevents duplicate worktrees after an
// ambiguous wrapper response.
func TestOrchestratorSkillSafetyAndWorktreeCreation(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"Never bypass sandbox or approval controls",
		"`--dangerously-bypass-approvals-and-sandbox`, `--yolo`, or an equivalent to make a stage run",
		"One stage is one agent process",
		"### Worktree creation verification",
		"Read the worktree path from the raw create response, not from a display or projection layer",
		"Confirm the path exists and that `orca worktree list` or `git worktree list` contains it before recording the handle",
		"If the response is missing a path or cannot be confirmed, do not issue a second create",
		"a missing field is ambiguous, not a failure",
		"If it still cannot be confirmed, set `blocked` with the exact response and the next inspection step",
		"Never retry a create with a new name or stage suffix until the first outcome is resolved. One stage gets one worktree",
	)
}

// TestOrchestratorSkillStageRoleContracts pins the complete triage, fixer, and
// retro inputs, outputs, tool limits, and stage placement.
func TestOrchestratorSkillStageRoleContracts(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Stage role contracts",
		"| triage | `agentbox-triage` |",
		"| fix | `agentbox-fixer` |",
		"| retro | `agentbox-retro` |",
		"issue title, body, labels, and relevant comments; repository scope and policy; duplicate-search results; preflight and environment checks",
		"`decision`, `reason`, `evidence`, `questions`, and `risks` covering unresolved assumptions",
		"no tracker or forge access, no shell, no writes; uses only the supplied snapshot",
		"current pull request head; relevant diff; acceptance criteria; accepted open `blocker` and `major` findings with stable ledger ids; required gates",
		"addressed finding ids with evidence, full commit SHA and changed paths, gate results, unresolved findings, and blockers",
		"own worktree only; no tracker access; no PR creation, merge, close, or verdict",
		"sanitized workflow record",
		"zero or more drafts, each with `observation`, `evidence`, `impact` (user impact or recurrence risk), `context`, `acceptance_criteria`, and `related`",
		"post-terminal only; never infers a cross-run pattern from one workflow",
		"`needs-info` leaves the issue open",
		"resumes triage after a response",
		"The fixer addresses only those findings",
		"Verification and an independent review run again on the fixer's resulting head before merge",
		"Skip it when triage rejected the issue or a preflight blocked the workflow before any work started",
		"Retro never blocks merge or close",
		"orchestrator scope-checks and deduplicates every draft before creating any follow-up",
	)
}

// TestOrchestratorSkillNoNestedWorker pins the single-agent-per-stage rule for
// workers and the no-relay-launcher rule for direct harness roles.
func TestOrchestratorSkillNoNestedWorker(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"Do not start an agent and then have that agent launch a second non-interactive agent process",
		"A worker performs the stage itself and never launches another agent or a nested non-interactive agent for the same work",
		"The worker performs the stage itself and never launches another agent or a nested non-interactive agent for the same work",
		"The terminal command is the stage agent itself; do not start a second agent inside it",
		"it is still the stage worker",
		"Never add a launcher agent whose only job is to start a stage role and relay its result",
	)
}

// TestOrchestratorSkillProvenance covers the stable workflow id, the
// machine-readable marker, the visible footer, and duplicate-verdict refusal.
func TestOrchestratorSkillProvenance(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Comment provenance",
		"Create a stable `workflow_id` at triage",
		"Every workflow comment, pull request body, and verdict carries exactly one machine-readable marker",
		"<!-- orchestrator-workflow: {",
		"\"workflow\":\"orchestrator-owner-name-35-a1b2c3\"",
		"\"stage\":\"review\"",
		"\"worker\":\"orca\"",
		"\"worker_id\":\"worker-7\"",
		"\"model\":\"model-id\"",
		"\"commit\":\"0123456789abcdef0123456789abcdef01234567\"",
		"\"pr\":42",
		"and exactly one visible footer",
		"**"+testFooterIcon+" Reviewed [0123456](https://github.com/owner/name/commit/0123456789abcdef0123456789abcdef01234567) by AgentBox reviewer (codex)** · approve · 0 findings · independence: different model · [#42](https://github.com/owner/name/pull/42) · took 16m 48s",
		"In the table below, `<icon>` is the literal mark",
		"| Triage issue body |",
		"| Plan comment |",
		"| State comment |",
		"| Pull request body |",
		"| Verify artifact |",
		"| Review verdict |",
		"| Close comment |",
		"| Retro comment |",
		"Precede the footer with `---` on its own line.",
		"EOF newline-only drift from a tracker body write is not content, while every other footer byte invalidates the verdict.",
		"Start the line with the AgentBox mark and bold the clause up to the first `·`.",
		"End with `took <duration>` on a stage artifact",
		"The marker keeps the full machine-readable record: `workflow`, `stage`,",
		"Never print a full registry model id in visible text",
		"Use the short name from the close rule, such as `codex/DeepSeek-V4.1`",
		"The full id belongs only in the marker and in the state ledger.",
		"The footer prints the skill version once, in the state footer",
		"Link every commit, issue, and pull request.",
		"The review footer states the independence basis",
		"Post exactly one footer, as the last line of the artifact.",
		"Refuse a second verdict for the same `(workflow, stage, commit)`",
		"Provenance never implies a human author. It names the agent and the commit",
		"A bot account or GitHub App that gives platform-level identity is the documented deferred alternative",
	)
}

// TestOrchestratorSkillCloseEvidenceTable pins the close comment layout: the
// stage and agent share one cell, the model is named by its short form, and
// the measured duration sits next to the result it belongs to.
func TestOrchestratorSkillCloseEvidenceTable(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"`Stage (agent/model) | Commit | Duration | Result`",
		"Fold the agent and model into the stage cell as `<stage> (<agent>/<model>)`",
		"use the model's short name — its last path segment, never the full registry id",
		"append the effort when the runtime recorded one, as in `Triage (codex/GLM5.3/high)`",
		"`rits/deepseek-ai/DeepSeek-V4.1-Flash-1017` becomes `DeepSeek-V4.1`",
		"Keep the marker's `unspecified` when the host orchestrator has no recorded model",
		"Put the whole-workflow time in the close footer",
	)
}

// TestOrchestratorSkillTemplates covers the six shipped templates and the
// explicit override precedence.
func TestOrchestratorSkillTemplates(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Templates",
		"The skill ships seven templates and installs them beside `SKILL.md`",
		"| `issue.md` |",
		"| `pr.md` |",
		"| `verdict.md` |",
		"| `state.md` |",
		"| `plan.md` |",
		"| `review-task.md` |",
		"| `verify-task.md` |",
		"Resolve a template from the first source that defines it",
		"1. an explicit user instruction for this issue;",
		"2. the repository's `.github/orchestrator/<kind>.md`;",
		"3. the template shipped with this skill.",
		"Never edit a repository's own templates",
	)
	wantAll(t, body,
		"### Prose formatting in posted artifacts",
		"GitHub renders every newline inside a paragraph or list item as a line break",
		"issue, pull request, comment, plan, verify, and verdict bodies do not",
	)
}

// markdownBlockStart matches a line that opens a new markdown block, so the
// line before it is never a soft wrap.
var markdownBlockStart = regexp.MustCompile("^\\s*(?:[-*+]\\s|\\d+\\.\\s|#{1,6}\\s|\\||```|>|<|-{3,}\\s*$)")

// proseLine is one hard-wrapped line in a template.
type proseLine struct {
	number int
	text   string
}

// hardWrappedProseLines reports lines that a renderer would turn into a line
// break in the middle of a sentence. GitHub renders a newline inside a
// paragraph or list item as a line break, so prose must stay on one source
// line; tables, headings, fenced blocks, HTML comments, and explicit hard
// breaks are exempt.
func hardWrappedProseLines(body string) []proseLine {
	lines := strings.Split(body, "\n")
	start := 0
	// A skill file opens with YAML front matter, which is structured data and
	// never prose.
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for index := 1; index < len(lines); index++ {
			if strings.TrimSpace(lines[index]) == "---" {
				start = index + 1
				break
			}
		}
	}
	var found []proseLine
	inFence, inComment := false, false
	for index, line := range lines[start:] {
		index += start
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.Contains(trimmed, "<!--") {
			inComment = true
		}
		if inComment {
			if strings.Contains(trimmed, "-->") {
				inComment = false
			}
			continue
		}
		if trimmed == "" || index+1 >= len(lines) {
			continue
		}
		next := lines[index+1]
		if strings.TrimSpace(next) == "" || markdownBlockStart.MatchString(next) {
			continue
		}
		if strings.HasSuffix(line, "  ") {
			continue
		}
		found = append(found, proseLine{number: index + 1, text: line})
	}
	return found
}

// TestSkillFilesDoNotHardWrapProse keeps every shipped skill file readable:
// anything wrapped at an editor column produces a ragged body with sentences
// broken mid-line in the browser, and an agent copying the shipped shape
// reproduces it. Every embedded skill and template is covered, so a new skill
// inherits the rule without touching this test.
func TestSkillFilesDoNotHardWrapProse(t *testing.T) {
	if len(Names()) == 0 {
		t.Fatal("no embedded skills found, the guard would pass vacuously")
	}
	for _, name := range Names() {
		for _, file := range Files(name) {
			raw, err := embedded.ReadFile(file.Source)
			if err != nil {
				t.Fatalf("%s: %v", file.Source, err)
			}
			for _, line := range hardWrappedProseLines(string(raw)) {
				t.Errorf("%s:%d is hard-wrapped prose, keep the paragraph on one line: %q", file.Source, line.number, line.text)
			}
		}
	}
}

// TestOrchestratorSkillSeverityBar pins the severity-to-action mapping that
// bounds the review gate: minor and nit findings never block merge.
func TestOrchestratorSkillSeverityBar(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"### Severity bar",
		"Only `blocker` and `major` block merge",
		"| `blocker` | Fails an acceptance criterion",
		"| `major` | A reproducible defect in the behavior this pull request changes",
		"| `minor` | A real issue with no merge risk",
		"| `nit` | Style or preference",
		"A `minor` or `nit` never blocks merge and is always deferred to a follow-up issue",
		"`Required changes` contains only the smallest changes that remove the merge risk from open `blocker` and `major` findings",
	)
}

// TestOrchestratorVerdictTemplateSeverityBar keeps the verdict artifact and
// the skill in agreement about which severities can block a merge.
func TestOrchestratorVerdictTemplateSeverityBar(t *testing.T) {
	content, err := EmbeddedFile("agentbox-orchestrator", "verdict.md", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Join(strings.Fields(string(content)), " ")
	wantAll(t, body,
		"<approve|approve-with-follow-ups|request-changes|blocked>",
		"Only `blocker` and `major` block merge",
		"`minor` and `nit` never block merge and are always follow-ups",
		"Mandatory for `approve-with-follow-ups`",
		"List one group per deferred finding or finding group and the created issue that tracks it",
		"the smallest changes that remove the merge risk from each open `blocker` or `major` finding",
	)
}

// TestOrchestratorStateTemplateFindingLedger pins the six ledger fields so a
// reviewer can carry finding identity across cycles.
func TestOrchestratorStateTemplateFindingLedger(t *testing.T) {
	content, err := EmbeddedFile("agentbox-orchestrator", "state.md", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	body := string(content)
	start := strings.Index(body, "findings:")
	end := strings.Index(body, "review_cycles:")
	if start < 0 || end <= start {
		t.Fatalf("state.md has no findings ledger block:\n%s", body)
	}
	wantAll(t, body[start:end],
		"id:",
		"severity:",
		"location:",
		"status:",
		"opened_at:",
		"closed_at:",
	)
}

// TestOrchestratorSkillFollowUps pins the fourth verdict outcome and its
// mandatory deferred-issue requirement.
func TestOrchestratorSkillFollowUps(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"Accept `approve`, `approve-with-follow-ups`, `request-changes`, or `blocked`",
		"`approve-with-follow-ups` requires the mandatory `Follow-ups` section and one created issue per deferred group before the verdict is posted",
		"the merge then proceeds immediately",
	)
}

// TestOrchestratorSkillDeltaReview pins cycle-scoped re-review so later cycles
// inspect the delta instead of re-deriving the whole change.
func TestOrchestratorSkillDeltaReview(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"Cycle 1 reviews the full pull request diff against the base",
		"Cycle N>1 reviews only `prev_head..head`, the ledger status of every prior finding, and the gates",
		"A reviewer must not re-derive the touched subsystem or review the full diff again",
		"a new finding outside the reviewed range is allowed only at `blocker` severity",
	)
}

// TestOrchestratorSkillFindingLedger pins per-finding identity across cycles
// and the gate-6 ledger check before a verdict is posted.
func TestOrchestratorSkillFindingLedger(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"`findings[]` is the finding ledger",
		"Gate 6 requires a status for every open id before a verdict is posted",
		"Require a ledger status for every open finding id before posting a verdict",
		"a `fixed` id cannot reopen without evidence at the current head",
		"Severity disputes are settled by the orchestrator against the enumerated definitions",
	)
}

// TestOrchestratorSkillVerifyIsNotAReviewer keeps verify to gates and
// observations and forbids promoting an observation into a requirement.
func TestOrchestratorSkillVerifyIsNotAReviewer(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"reports only `gates` and `observations`",
		"it never reports required changes, findings, or a verdict",
		"it is not a second reviewer",
		"MUST NOT forward a verifier observation to the reviewer as a requirement",
		"observations enter the ledger only after the reviewer classifies them",
	)
}

// TestOrchestratorSkillReviewCap pins the hard cap and its single exit
// decision, so the flow cannot drift past the cycle limit.
func TestOrchestratorSkillReviewCap(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"`review_cycles_max` (default 2) bounds request-changes cycles",
		"At the cap the orchestrator dispatches no further fix cycle",
		"it sets `blocked`, quotes the open ledger, and asks the maintainer exactly one decision",
		"merge with follow-ups, authorize one more cycle, or stop",
	)
}

// TestOrchestratorSkillReviewerDiversity pins first-pass diversity and same
// selection for consecutive cycles.
func TestOrchestratorSkillReviewerDiversity(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"use a different agent or model from the implementer for the *first* review pass only",
		"Consecutive fix cycles keep the same reviewer selection",
		"rotating models every cycle maximizes judgment variance instead of converging the review and is explicitly discouraged",
	)
}

// TestOrchestratorSkillDurableArtifacts pins the durability, delivery,
// completion, retry, and resume contract that must survive context loss.
func TestOrchestratorSkillDurableArtifacts(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Durable stage artifacts",
		"Every stage artifact has one canonical durable root: `$AGENTBOX_HOME/orchestrator/<workflow-id>/<stage>-<iteration>.md`",
		"`artifact_path` in the state record is always `orchestrator/<workflow-id>/<stage>-<iteration>.md`, interpreted relative to `$AGENTBOX_HOME`",
		"it is never a bare filename and never an absolute host path",
		"the digest is the lowercase 64-character SHA-256 hex digest of the exact persisted artifact bytes",
		"The existing `artifact` field remains reserved for the URL of a posted stage comment and is never a filesystem path or digest",
		"Plan, implement, fix, verify, and review workers write their complete stage artifact to the untracked `.agentbox-stage/<stage>-<iteration>.md` handoff in their assigned worktree and return only a completion note naming that path",
		"Triage and retro workers return their complete artifact body because they are tracker-blind and shell-less; the orchestrator persists that body as their durable artifact",
		"A stage is not complete until the canonical artifact exists and is non-empty, its digest is recorded, and the state entry records the durable location",
		"Post the stage comment or dispatch the next stage only after that ordering succeeds",
		"Redispatch that stage once as the next iteration",
		"a second violation sets `blocked` with the exact violation and one recovery action instead of prompting indefinitely",
		"require a non-empty file, recompute SHA-256, and block on any digest mismatch rather than trusting altered evidence or silently rerunning a stage recorded as complete",
		"re-render the next stage spec from the persisted artifact and state record, never from a previous worker session, chat transcript, or in-memory worker result",
		"An AgentBox worker that writes the correct non-empty handoff but stalls before its final completion note is recoverable from that recorded run, worktree, and handoff",
		"do not resume the worker merely to manufacture a note",
		"A retry is a new dispatch and therefore a new iteration; it never overwrites an earlier stage entry or artifact",
	)
}

// TestOrchestratorSkillMaterializesStageInputs pins self-contained stage
// dispatch and resume inputs without granting forge access or snapshot leakage.
func TestOrchestratorSkillMaterializesStageInputs(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"### Stage input materialization",
		"This rule applies to plan, implement, fix, verify, and review",
		"Every stage input is either inlined verbatim in the task spec for short text or copied exact bytes into `.agentbox-stage/context/<name>` in the assigned worktree for anything long",
		"the task spec names that worktree path",
		"A host-only path such as `$AGENTBOX_HOME/orchestrator/<workflow-id>/<stage>-<iteration>.md` may never be the worker's only reference",
		"the worker never reads `$AGENTBOX_HOME`",
		"A forge client, forge credential, or forge egress rule is never a stage-input mechanism",
		"A pull-request body reaches the reviewer as a materialized file, owned by #151",
		"Before dispatching review, fetch the posted pull request body, `headRefOid`, and `pr_updated_at` on the host through the tracker adapter, write the exact body bytes to `orchestrator/<workflow-id>/review-<iteration>-pr-body.md` under `$AGENTBOX_HOME`, compute its lowercase SHA-256 as `body_sha256`, compute its canonical review-body SHA-256 as `review_content_sha256`, and copy the exact bytes to `.agentbox-stage/context/pr-body.md` in the reviewer worktree.",
		"The review stage entry records non-empty `pr_body_path`, `body_sha256`, `review_content_sha256`, `headRefOid`, and `pr_updated_at`, and the task spec names `.agentbox-stage/context/pr-body.md`; a host-only path or forge client is never the reviewer's body source.",
		"Run the offline pre-review body gate against the durable canonical body and the normalized issue body before dispatch; a failure blocks dispatch with the exact field name.",
		"On review resume, resolve `pr_body_path`, require non-empty bytes, recompute SHA-256, block on mismatch, and recreate `.agentbox-stage/context/pr-body.md` from the durable copy before re-rendering the task spec.",
		"Invalidate the review verdict and rerun review at the current head when `headRefOid`, the canonical content, or any non-owned byte changes; filling the two orchestrator-owned `Independent checks` rows never invalidates it.",
		"Always reject a durable reviewed-body digest mismatch before comparing canonical content",
		"stop a legacy review stage entry without `review_content_sha256` with `pr-merge: review stage entry is missing review_content_sha256; rerun review to upgrade the digest contract`",
		"`.agentbox-stage/context/` is untracked and stays out of a run's starting snapshot (#147), as the stage-snapshot contract requires",
		"Pinned inputs are copied exact bytes and recorded with `sha256`",
		"openshell sandbox upload",
		"Never materialize a secret value, token file, or environment dump",
		"Before dispatching plan, inline or copy the normalized issue body and triage disposition into the plan worktree",
		"A re-rendered spec materializes its inputs the same way before dispatch and never points back at an old host path",
		"Path-list parity",
		"git diff --name-only <base>..HEAD",
	)
}

// TestOrchestratorSkillRefusalExemptsBodyStages pins the body-return
// exemption so handoff-stage refusal rules cannot cover tracker-blind stages.
func TestOrchestratorSkillRefusalExemptsBodyStages(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"For plan, implement, fix, verify, and review — the stages with a `.agentbox-stage/<stage>-<iteration>.md` handoff — refuse a worker result that returns an artifact body, uses the wrong handoff path, leaves the handoff empty, or otherwise violates the handoff contract",
		"Triage and retro are exempt from that body-return refusal, including a combined triage+plan dispatch: they return their complete artifact body because they are tracker-blind and shell-less; the orchestrator persists that returned body as the durable artifact and validates it before advancing.",
	)
}

// TestOrchestratorSkillStalledFollowUpQueueRecovery keeps follow-up creation
// idempotent after a queue stalls between an artifact and tracker write.
func TestOrchestratorSkillStalledFollowUpQueueRecovery(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"### Durable follow-up queue recovery",
		"Define the reviewer follow-up queue as the accepted groups in the persisted `review-<iteration>.md` verdict and the retro follow-up queue as the accepted proposals in the persisted `retro-<iteration>.md` artifact",
		"Record follow-ups already created in the finding ledger for review or `retro.created[]` for retro",
		"persist the review verdict or retro artifact before creating any follow-up",
		"record its created issue reference immediately in the finding ledger or `retro.created[]`",
		"upsert the state comment before moving to the next group or proposal",
		"validate the artifact path and digest, compare accepted groups or proposals with the recorded created issues",
		"search the tracker for the workflow marker before retrying an ambiguous create, adopt an existing match, and create exactly one issue only when no match exists",
		"block with the proposal identity and one recovery action",
		"Never rerun review or retro, and never create a duplicate issue",
	)
}

// TestOrchestratorSkillStateSchema covers the expanded record and the
// sensitive-data rules that still apply when model and effort are recorded.
func TestOrchestratorSkillStateSchema(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"<!-- orchestrator-state",
		"one visible status line",
		"renders one status line carrying only the state and the version, and hides the record",
		"must not repeat the issue, pull request, commit, or timing",
		"schema: 4",
		"workflow_id:",
		"orchestrator: {runtime:",
		"stages:",
		"stage: implement",
		"stage: verify",
		"stage: review",
		"worktree:",
		"artifact_path: orchestrator/orchestrator-owner-name-35-a1b2c3/",
		"artifact_sha256: <64-lowercase-hex-characters>",
		"review_cycles:",
		"blocked_reason:",
		"updated:",
		"The record may contain model and effort identifiers",
		"must never contain credentials, environment-variable values, account identifiers, absolute host paths, private URLs, logs, or secret values",
	)
}

// TestOrchestratorTemplateFiles verifies the embedded inventory, the sibling
// install names, and that every template renders the version and carries
// provenance.
func TestOrchestratorTemplateFiles(t *testing.T) {
	files := Files("agentbox-orchestrator")
	wantNames := []string{"SKILL.md", "issue.md", "pr.md", "verdict.md", "state.md", "plan.md", "review-task.md", "verify-task.md"}
	if len(files) != len(wantNames) {
		t.Fatalf("files = %v", files)
	}
	for i, want := range wantNames {
		if files[i].Name != want {
			t.Fatalf("file %d = %q, want %q", i, files[i].Name, want)
		}
		if files[i].Template != (want != "SKILL.md") {
			t.Fatalf("%s template flag = %v", want, files[i].Template)
		}
		if _, err := embedded.ReadFile(files[i].Source); err != nil {
			t.Fatalf("%s source %q: %v", want, files[i].Source, err)
		}
	}
	for _, name := range []string{"agentbox-cli", "agentbox-api"} {
		if got := Files(name); len(got) != 1 || got[0].Name != "SKILL.md" {
			t.Fatalf("%s inventory = %v; other skills stay single-file", name, got)
		}
	}

	headings := map[string][]string{
		"issue.md":       {"## At a glance", "## Summary", "## Current behavior", "## Proposed design", "<summary>Design detail", "## Acceptance criteria", "## Test plan", "## Risks", "## Out of scope", "## Related"},
		"pr.md":          {"## At a glance", "## What changed", "## Acceptance criteria", "## Reviewer guide", "<summary>Full change list", "## Evidence", "**Independent checks**", "Closes #<issue>", "## AGENTS.md check", "## Risks", "## Follow-ups", "## Provenance"},
		"verdict.md":     {"## Verdict", "## Severity bar", "## Review scope", "## Independence", "## Findings", "## Evidence at commit", "## Required changes", "## Follow-ups", "## Provenance"},
		"state.md":       {"<!-- orchestrator-state", "schema: 4", "workflow_id:", "configuration:", "selection_sources:", "selection_note:", "findings:", "review_cycles_max:", "tests_added:", "diff_stat:", "pr_body_path:", "body_sha256:", "review_content_sha256:", "headRefOid:", "pr_updated_at:"},
		"plan.md":        {"## Plan at a glance", "<details>", "<summary>Full plan", "## Acceptance criteria", "## Implementation slices", "## Gate list", "## Task spec for the implementer", "## Risks", "## Out of scope", "## Provenance"},
		"review-task.md": {"## Role", "## Scope", "## Severity bar", "## Checklist", "## Output", "## Provenance"},
		"verify-task.md": {"## Role", "## Scope", "## Output", "## Provenance"},
	}
	footers := map[string]string{
		"issue.md":       "**" + testFooterIcon + " Triaged by AgentBox (<agent>)**",
		"pr.md":          "**" + testFooterIcon + " Implemented by AgentBox (<agent>)** · reviewed [<short-sha>]",
		"verdict.md":     "**" + testFooterIcon + " Reviewed [<short-sha>]",
		"state.md":       "**" + testFooterIcon + " Orchestrator state: <state>**",
		"plan.md":        "**" + testFooterIcon + " Planned by AgentBox orchestrator (<agent>)** · base [<short-sha>]",
		"review-task.md": "**" + testFooterIcon + " Reviewed [<short-sha>]",
		"verify-task.md": "**" + testFooterIcon + " Verified [<short-sha>]",
	}
	for file, wants := range headings {
		content, err := EmbeddedFile("agentbox-orchestrator", file, "9.9.9")
		if err != nil {
			t.Fatal(err)
		}
		body := string(content)
		if strings.Contains(body, VersionPlaceholder) {
			t.Fatalf("%s still contains the version placeholder", file)
		}
		if strings.Contains(body, "Orchestrated by") {
			t.Fatalf("%s still prints the retired orchestrator footer", file)
		}
		if strings.Count(body, testFooterIcon) != 1 {
			t.Fatalf("%s has %d footers, want exactly one", file, strings.Count(body, testFooterIcon))
		}
		lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
		if last := lines[len(lines)-1]; !strings.HasPrefix(last, "**"+testFooterIcon) {
			t.Fatalf("%s footer is not the last line: %q", file, last)
		}
		if len(lines) < 2 || lines[len(lines)-2] != "" {
			t.Fatalf("%s footer lacks a blank line before it", file)
		}
		if file == "state.md" {
			if len(lines) >= 3 && lines[len(lines)-3] == "---" {
				t.Fatalf("state.md status line must stand alone without a --- rule")
			}
		} else if len(lines) < 3 || lines[len(lines)-3] != "---" {
			t.Fatalf("%s footer lacks its --- separator: %q", file, lines[len(lines)-3:])
		}
		wantAll(t, body, wants...)
		wantAll(t, body,
			"<!-- orchestrator-workflow: {",
			footers[file],
		)
	}
}

func TestForbiddenForgeWordingPattern(t *testing.T) {
	for _, wording := range []string{"gh", "credential", "credentials", "token"} {
		if !forbiddenForgeWording.MatchString(wording) {
			t.Fatalf("forbiddenForgeWording does not match %q", wording)
		}
	}
	if forbiddenForgeWording.MatchString("GitHub") {
		t.Fatal("forbiddenForgeWording matches GitHub")
	}
}

func TestReviewerPRBodyInputStaysLocal(t *testing.T) {
	reviewTask, err := EmbeddedFile("agentbox-orchestrator", "review-task.md", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	reviewerRole, err := RoleYAML("agentbox-reviewer")
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"review task":   string(reviewTask),
		"reviewer role": string(reviewerRole),
	} {
		if !strings.Contains(body, ".agentbox-stage/context/pr-body.md") {
			t.Fatalf("%s does not name the materialized PR body path", name)
		}
		if match := forbiddenForgeWording.FindString(body); match != "" {
			t.Fatalf("%s contains forbidden forge-client wording %q", name, match)
		}
	}
	wantAll(t, string(reviewTask),
		"echo the supplied `body_sha256` exactly as `PR body SHA-256: <body_sha256>` and the supplied `review_content_sha256` exactly as `Canonical PR body SHA-256: <review_content_sha256>`",
		"state that the orchestrator materialized and canonicalized the body, and do not recompute or invent either digest",
	)
}

// TestVersionDisplayRendersDirtyBuild covers the footer rule that a
// git-describe dirty suffix reads as "(dirty build)" instead of "-dirty",
// while a clean version is unchanged.
func TestVersionDisplayRendersDirtyBuild(t *testing.T) {
	dirty, err := EmbeddedFile("agentbox-orchestrator", "state.md", "v0.2-162-gf230faa-dirty")
	if err != nil {
		t.Fatal(err)
	}
	body := string(dirty)
	if strings.Contains(body, "-dirty") {
		t.Fatalf("dirty state footer still prints -dirty: %s", body)
	}
	wantAll(t, body, "AgentBox orchestrator v0.2-162-gf230faa (dirty build)")

	clean, err := EmbeddedFile("agentbox-orchestrator", "state.md", "v0.3.0")
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(clean), "AgentBox orchestrator v0.3.0")
}

// TestOrchestratorSkillStateRecordExampleIsClean enforces the sensitive-data
// rules on the documented examples and shipped templates.
func TestOrchestratorSkillStateRecordExampleIsClean(t *testing.T) {
	docs := []string{rawSkill(t, "agentbox-orchestrator")}
	for _, file := range Files("agentbox-orchestrator") {
		raw, err := embedded.ReadFile(file.Source)
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, string(raw))
	}
	for _, body := range docs {
		for _, forbidden := range []string{
			"/Users/", "/home/", `C:\Users`, "sk-", "ghp_", "AKIA",
			"api.openai.com", "api.anthropic.com", "gpt-", "claude-", "gemini",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("documentation contains %q", forbidden)
			}
		}
	}
}

// TestOrchestratorStateTemplateRecordsVerifyWorkerID keeps every stage entry
// able to name the worker that produced it, and pins the effort rule.
func TestOrchestratorStateTemplateRecordsVerifyWorkerID(t *testing.T) {
	content, err := EmbeddedFile("agentbox-orchestrator", "state.md", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	body := string(content)
	var block []string
	inVerify := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- stage:") {
			inVerify = trimmed == "- stage: verify"
		}
		if inVerify {
			block = append(block, trimmed)
		}
	}
	if len(block) == 0 {
		t.Fatal("state.md has no verify stage entry")
	}
	verify := strings.Join(block, "\n")
	if !strings.Contains(verify, "worker_id: <id>") {
		t.Fatalf("verify stage entry lacks worker_id:\n%s", verify)
	}
	wantAll(t, collapsed(t),
		"`effort` records what the worker actually ran with",
		"Use `not-applicable` when the selected runtime cannot apply one",
		"never record a requested effort that the worker did not apply",
	)
}

// TestOrchestratorPRTemplateProvenanceIsNeutral removes the false claim that
// only the orchestrator authors the pull request body.
func TestOrchestratorPRTemplateProvenanceIsNeutral(t *testing.T) {
	content, err := EmbeddedFile("agentbox-orchestrator", "pr.md", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	body := string(content)
	if strings.Contains(body, "authored by the orchestrator") {
		t.Fatalf("pr.md still claims orchestrator authorship:\n%s", body)
	}
	wantAll(t, strings.Join(strings.Fields(body), " "),
		"## Provenance",
		"it may be written by that worker or by the orchestrator on its behalf",
		"does not imply a human author",
	)
}

// TestOrchestratorSkillInstallsAndLists covers install, list, and show for
// the third skill, including its sibling templates.
func TestOrchestratorSkillInstallsAndLists(t *testing.T) {
	home := prepareHome(t)
	results, err := Install(Options{All: true, Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, result := range results {
		if result.Kind == "role" {
			continue
		}
		if result.Skill != "agentbox-orchestrator" {
			continue
		}
		seen = true
		if result.Target == "agents" && result.Action != "installed" {
			t.Fatalf("orchestrator canonical action = %s", result.Action)
		}
		if result.Target == "claude" && result.Action != "linked" {
			t.Fatalf("orchestrator claude action = %s", result.Action)
		}
		if result.Target == "agents" && result.File == "" {
			t.Fatalf("orchestrator canonical result lacks a file: %+v", result)
		}
	}
	if !seen {
		t.Fatal("install ignored the orchestrator skill")
	}
	dir := filepath.Join(home, ".agents", "skills", "agentbox-orchestrator")
	canonical := filepath.Join(dir, "SKILL.md")
	if body := mustRead(t, canonical); InstalledVersion(body) != "1.0.0" {
		t.Fatalf("orchestrator version = %s", InstalledVersion(body))
	}
	for _, template := range []string{"issue.md", "pr.md", "verdict.md", "state.md", "plan.md", "review-task.md", "verify-task.md"} {
		if _, err := os.Stat(filepath.Join(dir, template)); err != nil {
			t.Fatalf("%s is not installed beside SKILL.md: %v", template, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "agentbox-orchestrator", "issue.md")); err != nil {
		t.Fatalf("templates are not reachable through the claude link: %v", err)
	}
	manifest := loadManifest(filepath.Join(home, ".agents", "skills"))
	record, ok := manifest.Skills["agentbox-orchestrator"]
	if !ok || len(record.Files) != len(Files("agentbox-orchestrator")) {
		t.Fatalf("orchestrator manifest record = %+v", record)
	}

	statuses, err := List(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	var listed bool
	for _, status := range statuses {
		if status.Name != "agentbox-orchestrator" {
			continue
		}
		listed = true
		if stateOf(t, statuses, "agentbox-orchestrator", "agents").State != "current" {
			t.Fatal("orchestrator not current after install")
		}
		if len(status.Files) != 7 {
			t.Fatalf("template statuses = %+v", status.Files)
		}
		for _, file := range status.Files {
			if file.State != "current" {
				t.Fatalf("%s state = %+v", file.Name, file)
			}
		}
	}
	if !listed {
		t.Fatal("list ignored the orchestrator skill")
	}

	content, err := Embedded("agentbox-orchestrator", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "# Issue-to-PR orchestrator flow") {
		t.Fatal("show would not render the orchestrator skill body")
	}
	if _, err := Embedded("agentbox-orchestrator-extra", "1.0.0"); err == nil {
		t.Fatal("accepted a lookalike skill name")
	}
}

// TestOrchestratorSkillUsesShippedRoles keeps the skill and the embedded role
// set in sync: the stage table names only roles this binary ships, and every
// shipped role is dispatched by the skill.
func TestOrchestratorSkillUsesShippedRoles(t *testing.T) {
	body := collapsed(t)
	for _, role := range RoleNames() {
		if !strings.Contains(body, role) {
			t.Fatalf("skill does not name shipped role %q", role)
		}
	}
	referenced := regexp.MustCompile(`agentbox-(?:orchestrator|triage|planner|implementer|fixer|verifier|reviewer|retro)[a-z-]*`).FindAllString(body, -1)
	if len(referenced) == 0 {
		t.Fatal("skill does not reference any agent role")
	}
	for _, name := range referenced {
		if !ValidRoleName(name) {
			t.Fatalf("skill references unknown role %q", name)
		}
	}
	wantAll(t, body,
		"~/.codex/agents/<role>.toml",
		"~/.claude/agents/<role>.md",
		"~/.config/opencode/agents/<role>.md",
	)
}

// TestOrchestratorSkillChecksRoleAvailabilityAtTriage covers the blocked
// outcome for any role that is not current: stale copies are refreshed, and a
// harness restart is required because roles are scanned at session start.
func TestOrchestratorSkillChecksRoleAvailabilityAtTriage(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"agentbox skills list",
		"`stale` means the installed copy is byte-identical to an older AgentBox binary",
		"agentbox skills update",
		"harness restart",
		"harness agent definition",
	)
}

// TestOrchestratorSkillDispatchesTriageFixerRetro pins the stage contract added
// for tracker-blind triage, the dedicated fix cycle, and the post-terminal
// retro. The orchestrator stays the parent controller and performs every
// tracker mutation itself.
func TestOrchestratorSkillDispatchesTriageFixerRetro(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"`agentbox-triage`",
		"`agentbox-fixer`",
		"`agentbox-retro`",
		"explicitly assign the flow to a harness agent",
		"needs-info",
		"out-of-scope",
		"tracker-blind",
		"The orchestrator rechecks current tracker state and applies every label and state change itself",
		"Dispatch `agentbox-fixer`, not a new implementer",
		"only the accepted open `blocker` and `major` findings",
		"push that commit only to the existing pull request branch",
		"Deferred `minor` and `nit` findings are not fixer work",
		"review_cycles_max",
		"post-terminal",
		"Skip retro when triage",
		"zero or more candidate follow-up issue definitions",
		"Deduplicate and scope-check every proposal",
		"Retro never calls the tracker",
		"never claims a cross-run pattern",
	)
	if !ValidRoleName("agentbox-orchestrator") {
		t.Fatal("the orchestrator controller role is not installed")
	}
}

// TestOrchestratorSkillDocumentsRoleAssignment pins the exact assignment path
// for each supported harness and the Orca limitation. Orca selects a runtime,
// never the custom role name.
func TestOrchestratorSkillDocumentsRoleAssignment(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Harness agent roles",
		"~/.codex/agents/agentbox-orchestrator.toml",
		"~/.claude/agents/agentbox-orchestrator.md",
		"~/.config/opencode/agents/agentbox-orchestrator.md",
		"claude --agent agentbox-orchestrator",
		"opencode run --agent agentbox-orchestrator",
		"Ask the top-level session to use the `agentbox-orchestrator` custom agent",
		"Codex runs custom agents as subagents and has no top-level `--agent` switch",
		"`orca orchestration worker-start --agent <id>` selects only the host runtime",
		"never pass `agentbox-orchestrator` there",
		"an Orca-started session cannot select the role",
		"Start Claude directly with `--agent agentbox-orchestrator` outside Orca",
		"harness limitation, not a reason to add an Orca-specific role flag",
		"a shell-based source write remains instruction-only",
		"never starts itself",
		"never dispatches another `agentbox-orchestrator`",
		"Start a stage role directly when the harness supports it",
		"claude --agent agentbox-verifier",
		"opencode run --agent agentbox-verifier",
		"Codex dispatches the role as a subagent from a top-level session",
		"Never add a launcher agent whose only job is to start a stage role and relay its result",
	)
}

// TestOrchestratorSkillForgeReadsUseGHAPI keeps every documented forge read on
// the REST path that works in this repository, and keeps the deprecation note
// that explains why.
func TestOrchestratorSkillForgeReadsUseGHAPI(t *testing.T) {
	docs := []string{rawSkill(t, "agentbox-orchestrator")}
	for _, file := range Files("agentbox-orchestrator") {
		raw, err := embedded.ReadFile(file.Source)
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, string(raw))
	}
	for _, body := range docs {
		for _, banned := range []string{"gh pr view ", "gh issue view "} {
			if strings.Contains(body, banned) {
				t.Fatalf("documentation uses %q, which fails in this repository", banned)
			}
		}
	}
	wantAll(t, collapsed(t),
		"Forge reads use the REST API through `gh api`",
		"which is deprecated and fails in this repository",
		`gh api "repos/$OWNER/$REPO/issues/$ISSUE"`,
		`gh api --paginate "repos/$OWNER/$REPO/issues/$ISSUE/comments"`,
		`gh api "repos/$OWNER/$REPO/pulls/$PR" --jq .head.sha`,
		`gh api repos/<owner>/<repo>/pulls/<pr> --jq .head.sha`,
	)
}

// TestOrchestratorSkillStateCommentTargetValidation pins the issue-scoped
// state-comment lookup and the target check that must happen before a PATCH.
func TestOrchestratorSkillStateCommentTargetValidation(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"resolve only from `gh api --paginate repos/<owner>/<repo>/issues/<issue>/comments`",
		"never use `repos/<owner>/<repo>/issues/comments` for state-comment resolution",
		`gh api "repos/$OWNER/$REPO/issues/comments/$COMMENT_ID"`,
		"require `.issue_url` to equal `\"https://api.github.com/repos/$OWNER/$REPO/issues/$ISSUE\"` before any PATCH",
		"If a valid foreign comment id is found—a comment that exists but belongs to another issue—do not PATCH it, do not create a replacement, and leave the foreign comment untouched",
		"Exit non-zero and print an actionable error naming both issues",
		"state comment target mismatch: expected issue https://api.github.com/repos/owner/name/issues/35, resolved issue https://api.github.com/repos/owner/name/issues/36",
		"On resume or context loss, read this comment first through the same issue-scoped paginated lookup and target validation, never a repository-wide comment listing",
	)

	start := strings.Index(body, "Resolve and validate the state-comment target before every PATCH.")
	if start < 0 {
		t.Fatal("state comment recipe start anchor missing")
	}
	end := strings.Index(body, "| Link PR |")
	if end < 0 || end <= start {
		t.Fatalf("state comment recipe end anchor missing or out of order: start=%d end=%d", start, end)
	}
	recipe := body[start:end]
	lookup := strings.Index(recipe, `gh api --paginate "repos/$OWNER/$REPO/issues/$ISSUE/comments"`)
	read := strings.Index(recipe, `gh api "repos/$OWNER/$REPO/issues/comments/$COMMENT_ID"`)
	validate := strings.Index(recipe, "before any PATCH")
	patch := strings.Index(recipe, "`gh api --method PATCH")
	if lookup < 0 || read < 0 || validate < 0 || patch < 0 {
		t.Fatalf("state comment recipe fragment missing: lookup=%d read=%d validate=%d patch=%d", lookup, read, validate, patch)
	}
	if !(lookup < read && read < validate && validate < patch) {
		t.Fatalf("state comment recipe out of order: lookup=%d read=%d validate=%d patch=%d", lookup, read, validate, patch)
	}
}

// TestOrchestratorSkillNormalizeBeforePush pins the squash step, its identity,
// and the head check verify and review must report.
func TestOrchestratorSkillNormalizeBeforePush(t *testing.T) {
	wantAll(t, collapsed(t),
		"#### Normalize the branch before push",
		"one empty `AgentBox starting snapshot` commit",
		`git -C RUN_WORKTREE reset --soft "$BASE_SHA"`,
		`-c user.email="agentbox@local.invalid"`,
		`commit -C "$WRITER_SHA"`,
		"Push only `orchestrator/<issue>-<slug>`",
		"Record `synced_commit` as the worker's commit before the squash",
		"Verify and review must each report `git diff <reviewed-sha>..HEAD --stat` as empty",
	)
}

// TestOrchestratorSkillFixerRecipeStartsAtPRHead keeps the fixer on the pull
// request head instead of the project's default branch while retaining project
// attribution.
func TestOrchestratorSkillFixerRecipeStartsAtPRHead(t *testing.T) {
	wantAll(t, collapsed(t),
		"#### Fixer launch",
		"`agentbox agent start --project PROJECT` alone checks out the registered project's current branch",
		"would silently lose the implementation",
		`git -C REPO worktree add --detach FIX_WORKTREE "$HEAD_SHA"`,
		`agentbox agent start --profile PROFILE --project PROJECT --source FIX_WORKTREE --require-head "$HEAD_SHA" --gateway GATEWAY`,
	)
}

// TestOrchestratorSkillDeleteAndCleanupRecipes pins the working delete command
// and the cleanup order used at close.
func TestOrchestratorSkillDeleteAndCleanupRecipes(t *testing.T) {
	wantAll(t, collapsed(t),
		"### Cleanup after the final report",
		"`--discard` is a flag of `agentbox agent delete` and must come before the run id",
		"agentbox agent delete --discard RUN_ID",
		"1. delete every recorded run, while its worktree mapping still exists;",
		"2. remove the run and stage worktrees",
		"3. `git worktree prune`;",
		"4. delete the local branch, then the remote",
		"Never delete a branch whose commit SHA is not recorded",
	)
}

// TestOrchestratorStateTemplateDurableArtifacts proves every stage example in
// the canonical state shape records its durable location and digest.
func TestOrchestratorStateTemplateDurableArtifacts(t *testing.T) {
	content, err := EmbeddedFile("agentbox-orchestrator", "state.md", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	body := string(content)
	if !strings.Contains(body, "schema: 4") {
		t.Fatal("state template is not schema 4")
	}

	stageExamples := map[string][]string{
		"triage":    {"orchestrator/<workflow-id>/triage-1.md"},
		"plan":      {"orchestrator/<workflow-id>/plan-1.md"},
		"implement": {"orchestrator/<workflow-id>/implement-1.md"},
		"fix":       {"orchestrator/<workflow-id>/fix-1.md"},
		"verify":    {"orchestrator/<workflow-id>/verify-1.md"},
		"review":    {"orchestrator/<workflow-id>/review-1.md"},
		"retro":     {"orchestrator/<workflow-id>/retro-1.md"},
	}
	lines := strings.Split(body, "\n")
	for stage, wantPaths := range stageExamples {
		var gotPaths []string
		for index, line := range lines {
			if strings.TrimSpace(line) != "- stage: "+stage {
				continue
			}
			var artifactPath, digest, commentURL string
			for _, field := range lines[index+1:] {
				trimmed := strings.TrimSpace(field)
				if strings.HasPrefix(trimmed, "- stage: ") {
					break
				}
				if strings.HasPrefix(trimmed, "artifact_path: ") {
					artifactPath = strings.TrimPrefix(trimmed, "artifact_path: ")
				}
				if strings.HasPrefix(trimmed, "artifact_sha256: ") {
					digest = strings.TrimPrefix(trimmed, "artifact_sha256: ")
				}
				if strings.HasPrefix(trimmed, "artifact: ") {
					commentURL = strings.TrimPrefix(trimmed, "artifact: ")
				}
			}
			if artifactPath == "" || digest != "<64-lowercase-hex-characters>" {
				t.Fatalf("%s stage example lacks durable fields: path %q, digest %q", stage, artifactPath, digest)
			}
			if artifactPath != "orchestrator/<workflow-id>/"+stage+"-1.md" {
				t.Fatalf("%s artifact path = %q", stage, artifactPath)
			}
			gotPaths = append(gotPaths, artifactPath)
			if (stage == "verify" || stage == "review") != (commentURL == "<comment-url>") {
				t.Fatalf("%s artifact comment URL = %q", stage, commentURL)
			}
		}
		if len(gotPaths) != len(wantPaths) {
			t.Fatalf("%s stage examples = %v, want %v", stage, gotPaths, wantPaths)
		}
		for index, want := range wantPaths {
			if gotPaths[index] != want {
				t.Fatalf("%s stage example %d path = %q, want %q", stage, index+1, gotPaths[index], want)
			}
		}
	}
}

// TestOrchestratorPlanTemplateDurableArtifacts pins the planner's handoff and
// completion-note contract.
func TestOrchestratorPlanTemplateDurableArtifacts(t *testing.T) {
	content, err := EmbeddedFile("agentbox-orchestrator", "plan.md", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	body := string(content)
	wantAll(t, body, "The planner writes the full artifact to the untracked `.agentbox-stage/plan-<iteration>.md` and returns only a completion note naming that path; the orchestrator reads, persists, and posts it with provenance.")
	if strings.Contains(body, "The planner returns this artifact to the orchestrator.") {
		t.Fatal("plan template still tells the planner to return the artifact")
	}
}

// TestOrchestratorReviewTaskTemplateDurableArtifacts pins the reviewer's
// exact handoff and completion-note contract.
func TestOrchestratorReviewTaskTemplateDurableArtifacts(t *testing.T) {
	content, err := EmbeddedFile("agentbox-orchestrator", "review-task.md", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	body := string(content)
	wantAll(t, body,
		"You are the independent reviewer for this pull request. You do not implement, commit, push, merge, or post to the tracker. Write exactly one `verdict.md` artifact to the untracked `.agentbox-stage/review-<iteration>.md` and return only a completion note naming that path; the orchestrator reads, persists, and posts it with provenance.",
		"Render `verdict.md` with one of `approve`, `approve-with-follow-ups`, `request-changes`, or `blocked`, plus:",
		"The `.agentbox-stage/review-<iteration>.md` handoff is the verdict artifact; the orchestrator reads, persists, and posts it, so a review verdict always has one writer and one provenance path.",
	)
	for _, superseded := range []string{
		"Return exactly one `verdict.md` artifact to the orchestrator",
		"Return `verdict.md` rendered with",
		"Return this artifact to the orchestrator.",
	} {
		if strings.Contains(body, superseded) {
			t.Fatalf("review task still contains superseded instruction %q", superseded)
		}
	}

	verdictContent, err := EmbeddedFile("agentbox-orchestrator", "verdict.md", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	verdictBody := string(verdictContent)
	wantAll(t, verdictBody,
		"The `.agentbox-stage/review-<iteration>.md` handoff is the verdict artifact; the orchestrator reads, persists, and posts it with provenance.",
	)
	if strings.Contains(verdictBody, "The reviewer returns this artifact to the orchestrator.") {
		t.Fatal("verdict template still tells the reviewer to return the artifact")
	}
}

// TestOrchestratorVerifyTaskTemplateDurableArtifacts pins the verifier's
// exact handoff and completion-note contract.
func TestOrchestratorVerifyTaskTemplateDurableArtifacts(t *testing.T) {
	content, err := EmbeddedFile("agentbox-orchestrator", "verify-task.md", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	body := string(content)
	wantAll(t, body,
		"You are the verifier, not a reviewer. Rerun every gate on the pull request head and report the results. You do not edit, commit, push, merge, or post to the tracker. Write this artifact to the untracked `.agentbox-stage/verify-<iteration>.md` and return only a completion note naming that path; the orchestrator reads, persists, and posts the comment.",
		"Write exactly this artifact, ready for the orchestrator to read and post:",
		"The orchestrator reads the `.agentbox-stage/verify-<iteration>.md` handoff, persists it, and posts it as a `stage:verify` comment on the pull request with provenance and links it from the pull request's `Independent checks` row. It is evidence, not internal bookkeeping, so it must stand on its own outside the state comment.",
	)
	for _, superseded := range []string{
		"Return this artifact to the orchestrator; it posts the comment.",
		"Return exactly this artifact, ready for the orchestrator to post:",
		"The orchestrator posts this artifact as a `stage:verify` comment",
	} {
		if strings.Contains(body, superseded) {
			t.Fatalf("verify task still contains superseded instruction %q", superseded)
		}
	}
}

// TestOrchestrationDocsDurableArtifacts keeps the user-facing resume rule to
// the durable root, non-empty completion requirement, and deterministic
// re-render behavior reserved for the skill reference.
func TestOrchestrationDocsDurableArtifacts(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "..", "..", "docs", "orchestration.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	want := "Durable stage artifacts live under `$AGENTBOX_HOME/orchestrator/<workflow-id>/`; a stage is not complete until its artifact exists and is non-empty, and a resumed flow re-renders the next stage spec from the persisted artifact rather than the previous worker session. It preserves the verification/review gates before merge."
	if !strings.Contains(body, want) {
		t.Fatalf("docs missing exact durable-artifact sentence:\n%s", want)
	}
	if strings.Contains(body, "A resumed flow reads the recorded state and dispatches the required stage") {
		t.Fatal("docs still contain the superseded two-line resume explanation")
	}
	if !strings.Contains(body, "`$AGENTBOX_HOME/orchestrator/<workflow-id>/`") || strings.Contains(body, "\\`") {
		t.Fatal("docs sentence does not use literal markdown code spans")
	}
}

// TestOrchestratorStateTemplateExpressesSelectionAndIterations covers the
// schema 4 fields a run needs to record a manual selection and a writer
// iteration without reconstructing them later.
func TestOrchestratorStateTemplateExpressesSelectionAndIterations(t *testing.T) {
	content, err := EmbeddedFile("agentbox-orchestrator", "state.md", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	flat := strings.Join(strings.Fields(string(content)), " ")
	wantAll(t, flat,
		"schema: 4",
		"selection_note:",
		"iteration: 1",
		"dispatch: <split|combined>",
		"started: <RFC3339>",
		"ended: <RFC3339>",
		"synced_commit: <full-sha>",
		"artifact_path: orchestrator/<workflow-id>/",
		"artifact_sha256: <64-lowercase-hex-characters>",
		"artifact: <comment-url>",
		"tests_added:",
		"diff_stat:",
	)
	wantAll(t, collapsed(t),
		"Every dispatch appends a new `stages[]` entry and never overwrites the previous one",
		"`iteration` counts dispatches of the same stage, starting at 1",
		"They are the source of every `took` duration in a footer",
	)
	wantAll(t, collapsed(t),
		"**Workflow identity adoption.** The state record wins over every other copy of the identity",
		"post one supersession note on the issue that names both identities",
		"update the body marker in place",
		"Schema 4 adds durable `artifact_path` and `artifact_sha256`",
		"#### Reproduce a running worker's selection",
		"agentbox agent list --json",
		"copied from running agent run-84b3cbf16f23",
	)
}

// TestOrchestratorSkillWorkflowAdoptionStatedOnce keeps the adoption rule in
// one place and referenced from triage and the tracker adapter.
func TestOrchestratorSkillWorkflowAdoptionStatedOnce(t *testing.T) {
	body := rawSkill(t, "agentbox-orchestrator")
	if got := strings.Count(body, "**Workflow identity adoption.**"); got != 1 {
		t.Fatalf("adoption rule is stated %d times, want exactly one", got)
	}
	wantAll(t, collapsed(t),
		"On resume, adopt the recorded identity before anything else",
		"**Workflow identity adoption** describes",
		"adopt the recorded workflow identity before using it, as **Workflow identity adoption** requires",
	)
}

// TestOrchestratorPlanTemplateScanLayer pins the five-row scan layer and the
// collapsed detail of the plan comment.
func TestOrchestratorPlanTemplateScanLayer(t *testing.T) {
	content, err := EmbeddedFile("agentbox-orchestrator", "plan.md", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	body := string(content)
	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		if line == "## Plan at a glance" {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("plan.md has no scan layer")
	}
	var rows int
	inTable := false
	for _, line := range lines[start+1:] {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			if inTable {
				break
			}
			if trimmed == "" || trimmed == "## Plan at a glance" {
				continue
			}
			continue
		}
		inTable = true
		if strings.HasPrefix(trimmed, "| ---") || strings.HasPrefix(trimmed, "| Field | Value |") {
			continue
		}
		rows++
	}
	if rows < 1 || rows > 5 {
		t.Fatalf("plan scan layer has %d rows, want 1..5", rows)
	}
	flat := strings.Join(strings.Fields(body), " ")
	wantAll(t, flat,
		"| What |",
		"| Why |",
		"| Size |",
		"| Gates |",
		"| Risks |",
		"<summary>Full plan</summary>",
		"## Gate list",
		"A raw `npm` command inside the sandbox can fail with `EACCES`",
		"| Full | `make test` |",
	)
}

// TestOrchestratorVerdictTemplateStatesIndependence keeps the independence
// basis on the face of the verdict, not only in the footer.
func TestOrchestratorVerdictTemplateStatesIndependence(t *testing.T) {
	content, err := EmbeddedFile("agentbox-orchestrator", "verdict.md", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	flat := strings.Join(strings.Fields(string(content)), " ")
	wantAll(t, flat,
		"## Independence",
		"not only in the footer",
		"same agent and model as the implementer",
		"session and worktree isolation do not make the review independent",
		"Head check: `git diff <reviewed-sha>..HEAD --stat` is empty",
	)
}

// TestOrchestratorSkillCombinedTriageAndPlan covers the opt-in fast path and
// the guarantees it cannot skip.
func TestOrchestratorSkillCombinedTriageAndPlan(t *testing.T) {
	wantAll(t, collapsed(t),
		"**Optional combined dispatch.**",
		"triage and plan may run as one worker dispatch",
		"Record both stages with the same `started` and `ended` values",
		"`dispatch: combined`",
		"the plan still needs its scan layer, gate list, and implementer task spec",
		"Ambiguous issues keep the split.",
	)
}

// TestOrchestratorSkillPrePushDiffReviewAndGitProbe covers the orchestrator
// diff-review checklist and the real-git probe shared by verify and review.
func TestOrchestratorSkillPrePushDiffReviewAndGitProbe(t *testing.T) {
	wantAll(t, collapsed(t),
		"### Pre-push diff review (orchestrator)",
		"| Allowed files |",
		"| Secrets |",
		"| Fake vs real tests |",
		"| `dist` reproducibility |",
		"| Document accuracy |",
		"| Acceptance criterion → test |",
		"| Host-side focused run |",
		"A failing answer is fixed before push",
		"### Real-git probe (git-derived behavior)",
		"| up-to-date | local branch equals its upstream |",
		"| behind | local branch behind its upstream |",
		"| diverged | local and upstream each have unique commits |",
		"| detached | HEAD is not on a branch |",
		"| unborn | repository has no commit yet |",
		"| non-repository | the path is not inside a git worktree |",
	)
	for _, role := range []string{"agentbox-verifier", "agentbox-reviewer"} {
		instructions, err := RoleByName(role)
		if err != nil {
			t.Fatal(err)
		}
		wantAll(t, strings.Join(strings.Fields(instructions.Instructions), " "),
			"git diff <reviewed-sha>..HEAD --stat",
			"non-repository",
			"unborn",
		)
	}
}

// TestOrchestratorRetroChecksExistingWork keeps retro from proposing what is
// already at head.
func TestOrchestratorRetroChecksExistingWork(t *testing.T) {
	wantAll(t, collapsed(t),
		"the diff stat, and the tests added by the change",
		"must not already exist at head",
		"Retro checks the record's `tests_added` and `diff_stat`",
	)
	retro, err := RoleByName("agentbox-retro")
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, strings.Join(strings.Fields(retro.Instructions), " "),
		"tests_added",
		"diff stat",
		"not already present at head",
	)
}
