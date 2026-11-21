package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const bootstrapScript = `#!/bin/sh
set -eu
export LANG=C.UTF-8
export LC_ALL=C.UTF-8
if [ -f /workspace/finished ]; then
  exec sleep infinity
fi
if [ -f /workspace/state/agent-started ]; then
  # A stopped or restarted sandbox keeps /workspace/state, so the agent comes
  # back through the continue-or-fresh entry point instead of replaying the
  # original task prompt.
  tmux new-session -d -s agent /bin/sh /workspace/state/agent-entry.sh
else
  touch /workspace/state/agent-started
  tmux new-session -d -s agent /bin/sh /workspace/state/session.sh initial
fi
# The first tmux client command starts the server, so remain-on-exit can only
# be set after new-session; on a fresh sandbox set-option before it fails with
# "error connecting" and ends the run.
tmux set-option -g remain-on-exit on
tmux pipe-pane -t agent -o 'cat >> /workspace/state/agent.log'
# Finish or resume may remove the session while this loop is running, so poll
# the session before the pane instead of using a command substitution that
# would keep failing forever once it is gone.
while tmux has-session -t agent 2>/dev/null; do
  if [ "$(tmux display-message -p -t agent '#{pane_dead}' 2>/dev/null || echo 1)" = 1 ]; then
    break
  fi
  sleep 1
done
tmux kill-session -t agent 2>/dev/null || true
exec sleep infinity
`

type StartOptions struct {
	Profile  string
	Project  string
	Name     string
	Source   string
	Gateway  string
	Image    string
	Model    string
	Mode     string
	Approval string
	Prompt   string
	MCP      string
	Skills   string
	Dry      bool
}

func (a *App) Start(ctx context.Context, opt StartOptions) (Run, error) {
	p, err := a.Store.LoadProfile(opt.Profile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Run{}, fmt.Errorf("profile %q not found; create it from the Profiles page or with `agentbox profile create`", opt.Profile)
		}
		return Run{}, err
	}
	override := strings.TrimSpace(opt.Image)
	forceProfileImage := false
	if override == "profile" {
		override, forceProfileImage = "", true
	}
	if override != "" {
		p.Image = override
	}
	if opt.Model != "" {
		p.Model = opt.Model
	}
	if opt.Mode != "" {
		p.Mode = opt.Mode
	}
	if opt.Approval != "" {
		p.Approval = opt.Approval
	}
	if err := p.Validate(); err != nil {
		return Run{}, fmt.Errorf("profile %q is invalid: %w", p.Name, err)
	}
	if _, err := deriveProviderEndpoint(p); err != nil {
		return Run{}, err
	}
	legacyNetwork, legacyPath, err := a.legacyPolicyNetwork(p)
	if err != nil {
		return Run{}, err
	}
	var selection *MCPSelection
	if opt.MCP != "" {
		resolved, err := a.ResolveMCPSelection(opt.MCP, p.Agent)
		if err != nil {
			return Run{}, err
		}
		selection = &resolved
	}
	var skillSelection *SkillsSelection
	if opt.Skills != "" {
		resolved, err := a.ResolveSkillsSelection(opt.Skills, p.Agent)
		if err != nil {
			return Run{}, err
		}
		skillSelection = &resolved
	}
	if opt.Source == "" {
		opt.Source = "."
	}
	projectName := ""
	useWorktree := true
	if opt.Project != "" {
		project, err := a.Store.LoadProject(opt.Project)
		if err != nil {
			return Run{}, fmt.Errorf("project %q not found; create it from the Projects page or with `agentbox project create`", opt.Project)
		}
		projectName = project.Name
		opt.Source = project.Root
		useWorktree = project.UseWorktree()
	}
	sourceRoot, err := a.ProjectRoot(ctx, opt.Source)
	if err != nil {
		return Run{}, err
	}
	if projectName == "" {
		if project, ok, err := a.Store.ProjectRoot(sourceRoot); err != nil {
			return Run{}, err
		} else if ok {
			projectName = project.Name
			useWorktree = project.UseWorktree()
		}
	}
	if err := a.validateRunName(projectName, opt.Name); err != nil {
		return Run{}, err
	}
	if !useWorktree {
		if err := a.requireDirectRunAvailable(ctx, sourceRoot); err != nil {
			return Run{}, err
		}
	}
	if opt.Gateway == "" {
		return Run{}, fmt.Errorf("explicit gateway name required")
	}
	gw, err := a.GatewayInfo(ctx, opt.Gateway)
	if err != nil {
		return Run{}, err
	}
	if !strings.Contains(strings.ToLower(string(gw)), "podman") {
		return Run{}, fmt.Errorf("selected gateway does not report the Podman driver")
	}
	if definition, ok := BuiltinProviderByName(p.Provider); ok {
		status := a.ensureBuiltinProvider(ctx, definition, opt.Gateway)
		if !status.Registered {
			if !status.CredentialPresent {
				return Run{}, fmt.Errorf("provider %q is built in but %s is not set in the environment where AgentBox and the OpenShell gateway run", p.Provider, definition.Credential)
			}
			detail := status.Error
			if detail == "" {
				detail = "not registered with OpenShell"
			}
			return Run{}, fmt.Errorf("provider %q is built in but could not be registered: %s", p.Provider, detail)
		}
	}
	if _, err := a.Runner.Output(ctx, "openshell", "provider", "get", p.Provider, "--gateway", opt.Gateway); err != nil {
		return Run{}, fmt.Errorf("provider %q unavailable: %w", p.Provider, err)
	}
	runImage := RunImage{Image: p.Image, BaseImage: p.Image, Source: ImageSourceProfile}
	switch {
	case override != "":
		runImage = RunImage{Image: override, Source: ImageSourceOverride}
	case !forceProfileImage:
		runImage = a.ResolveRunImage(ctx, p, sourceRoot)
	}
	policy, provenance, err := deriveRunPolicy(p, opt.Gateway, legacyNetwork, legacyPath, runImage)
	if err != nil {
		return Run{}, err
	}
	if _, err := a.Runner.Output(ctx, "podman", "image", "inspect", runImage.Image); err != nil {
		return Run{}, fmt.Errorf("image unavailable locally: %w", err)
	}
	id, err := NewID()
	if err != nil {
		return Run{}, err
	}
	runName := strings.TrimSpace(opt.Name)
	if runName == "" {
		runName = id
	}
	preview := Run{ID: id, Name: runName, Source: sourceRoot, Project: projectName, ProfileName: p.Name, Gateway: opt.Gateway, Agent: p.Agent, Image: runImage.Image, BaseImage: runImage.BaseImage, ImageSource: string(runImage.Source), ImageRepo: runImage.Repo, ManifestDigest: runImage.ManifestDigest, ImageWarnings: runImage.Warnings, PolicyProvenance: provenance, Mode: p.Mode, Model: p.Model, Approval: p.Approval, Policy: p.Policy, Status: StatusCreating, StatusDetail: "dry run preview", WorktreeIsolated: useWorktree, SyncState: SyncStateUnknown}
	if runImage.Source == ImageSourceRepo {
		preview.PolicyFragment = runImage.FragmentPath
		preview.PolicyFragmentDigest = runImage.FragmentDigest
	}
	if selection != nil {
		preview.MCP = selection.Entry.Name
		preview.MCPWarnings = selection.Warnings()
	}
	if skillSelection != nil {
		preview.Skills = skillSelection.Entry.Name
		preview.SkillsWarnings = skillSelection.Warnings()
	}
	if opt.Dry {
		return preview, nil
	}
	var r Run
	if useWorktree {
		worktree, branch, err := a.WorktreeLocation(projectName, runName, id)
		if err != nil {
			return Run{}, err
		}
		r, err = a.CreateWorktreeAt(ctx, sourceRoot, id, worktree, branch)
		if err != nil {
			return Run{}, err
		}
	} else {
		r, err = a.CreateDirectRun(ctx, sourceRoot, id)
		if err != nil {
			return Run{}, err
		}
	}
	r.Name = runName
	r.Project = projectName
	r.ProfileName = p.Name
	r.WorktreeIsolated = useWorktree
	r.Created = time.Now().UTC()
	r.Gateway = opt.Gateway
	r.Agent = p.Agent
	r.Image = runImage.Image
	r.BaseImage = runImage.BaseImage
	r.ImageSource = string(runImage.Source)
	r.ImageRepo = runImage.Repo
	r.ManifestDigest = runImage.ManifestDigest
	r.ImageWarnings = runImage.Warnings
	r.PolicyProvenance = provenance
	if runImage.Source == ImageSourceRepo {
		r.PolicyFragment = runImage.FragmentPath
		r.PolicyFragmentDigest = runImage.FragmentDigest
	}
	r.Mode = p.Mode
	r.Model = p.Model
	r.Approval = p.Approval
	r.Policy = filepath.Join(a.Store.PolicyDir(), id+".yaml")
	if selection != nil {
		r.MCP = selection.Entry.Name
		r.MCPWarnings = selection.Warnings()
	}
	if skillSelection != nil {
		r.Skills = skillSelection.Entry.Name
		r.SkillsWarnings = skillSelection.Warnings()
	}
	if err := atomicWrite(r.Policy, policy); err != nil {
		return r, err
	}
	if err := a.Store.SaveRun(r); err != nil {
		return r, err
	}
	failRun := func(err error) (Run, error) {
		r.Status = StatusFailed
		r.StatusDetail = truncateDetail(err.Error())
		_ = a.Store.SaveRun(r)
		return r, err
	}
	bundle, err := a.MakeBundle(ctx, r)
	if err != nil {
		return failRun(err)
	}
	config, err := a.PrepareAgent(ctx, r, p, opt.Prompt, selection, skillSelection)
	if err != nil {
		return failRun(err)
	}
	bootstrap := []byte(bootstrapScript)
	bootPath := filepath.Join(config, "bootstrap.sh")
	if err := os.WriteFile(bootPath, bootstrap, 0700); err != nil {
		return failRun(err)
	}
	args := sandboxCreateArgs(r, p, opt.Gateway)
	if _, err := a.Runner.Output(ctx, "openshell", args...); err != nil {
		return failRun(err)
	}
	r.Status = StatusCreating
	r.StatusDetail = "sandbox created; starting agent"
	r.Synced = false
	if err := a.Store.SaveRun(r); err != nil {
		return r, err
	}
	if err := a.upload(ctx, r, bundle, "/workspace/source.bundle"); err != nil {
		return failRun(err)
	}
	tarPath := filepath.Join(a.Store.RunsDir(), id+"-config.tar")
	if _, err := a.Runner.Output(ctx, "tar", "-C", config, "-cf", tarPath, "."); err != nil {
		return failRun(err)
	}
	if err := a.upload(ctx, r, tarPath, "/workspace/config.tar"); err != nil {
		return failRun(err)
	}
	prep := "set -eu; mkdir -p /workspace/state; tar -xf /workspace/config.tar -C /workspace/state; git clone /workspace/source.bundle /workspace/project; git -C /workspace/project checkout " + shellQuote(r.Branch)
	if _, err := a.sandboxExec(ctx, r, "/bin/sh", "-c", prep); err != nil {
		return failRun(err)
	}
	if err := a.VerifyEffectivePolicy(ctx, r, p); err != nil {
		r.Status = StatusFailed
		r.StatusDetail = "policy check failed: " + err.Error()
		_ = a.Store.SaveRun(r)
		return r, err
	}
	if _, err := a.sandboxExec(ctx, r, "/bin/sh", "-c", "touch /workspace/ready"); err != nil {
		return failRun(err)
	}
	r.Status = StatusReady
	updated, err := a.inspectRun(ctx, r)
	if err != nil {
		return r, err
	}
	return updated, nil
}

func sandboxCreateArgs(r Run, p Profile, gateway string) []string {
	args := []string{"sandbox", "create", "--name", r.Sandbox, "--from", r.Image, "--provider", p.Provider}
	args = append(args, "--policy", r.Policy, "--detach", "--no-auto-providers", "--approval-mode", "manual", "--label", "agentbox-run="+r.ID, "--gateway", gateway, "--", "/bin/sh", "-c", "while [ ! -f /workspace/ready ]; do sleep 1; done; exec /bin/sh /workspace/state/bootstrap.sh")
	return args
}

func (a *App) VerifyEffectivePolicy(ctx context.Context, r Run, p Profile) error {
	b, err := a.Runner.Output(ctx, "openshell", "policy", "get", r.Sandbox, "--full", "-o", "json", "--gateway", r.Gateway)
	if err != nil {
		return err
	}
	s := strings.ToLower(string(b))
	endpoint, err := deriveProviderEndpoint(p)
	if err != nil {
		return err
	}
	// OpenShell wraps the policy document in an envelope with metadata such as
	// active_version and policy_source. Older releases returned the document
	// at the top level, so accept both shapes.
	var envelope struct {
		Policy *Policy `yaml:"policy"`
	}
	if err := yaml.Unmarshal(b, &envelope); err != nil {
		return fmt.Errorf("effective policy could not be parsed: %w", err)
	}
	var effective Policy
	if envelope.Policy != nil {
		effective = *envelope.Policy
	} else if err := yaml.Unmarshal(b, &effective); err != nil {
		return fmt.Errorf("effective policy could not be parsed: %w", err)
	}
	endpointAllowed := false
	for _, rule := range effective.Network {
		for _, candidate := range rule.Endpoints {
			if strings.EqualFold(candidate.Host, endpoint.host) && candidate.Port == endpoint.port {
				endpointAllowed = true
			}
		}
	}
	if !endpointAllowed {
		return fmt.Errorf("effective policy lacks provider endpoint %q; provider %q is not ready", fmt.Sprintf("%s:%d", endpoint.host, endpoint.port), p.Provider)
	}
	if p.Mode == "gateway" && (strings.Contains(s, "api.openai.com") || strings.Contains(s, "api.anthropic.com")) {
		return fmt.Errorf("effective policy allows direct model-provider egress in gateway mode")
	}
	if r.Approval == "unattended" {
		if !strings.Contains(s, "hard_requirement") {
			return fmt.Errorf("unattended run lacks hard Landlock requirement")
		}
		if !strings.Contains(s, "network_policies") {
			return fmt.Errorf("unattended run lacks verified network policy")
		}
	}
	return nil
}

func (a *App) ListAndReconcile(ctx context.Context) ([]Run, error) {
	runs, err := a.Store.ListRuns()
	if err != nil {
		return nil, err
	}
	for i := range runs {
		updated, err := a.inspectRun(ctx, runs[i])
		if err != nil {
			return nil, err
		}
		needsSync := updated.Status == StatusCompleted && !updated.Synced
		if updated.Status == StatusReady && updated.Exited && updated.SyncState == SyncStateNeedsSync {
			needsSync = true
		}
		if needsSync {
			// Reconcile retries an observed exit on every read. A failed sync
			// stays visible through the derived sync state instead of being
			// swallowed or reported as up to date.
			if syncErr := a.Sync(ctx, updated.ID); syncErr == nil {
				if reloaded, loadErr := a.Store.LoadRun(updated.ID); loadErr == nil {
					updated = reloaded
					updated.SyncState = a.deriveSyncState(ctx, updated, false)
				}
			}
		}
		runs[i] = updated
	}
	return runs, nil
}

// inspectRun derives the authoritative status for one run. Failed runs keep
// their terminal failure until the record is deleted; every other run is
// probed so the cache never outlives the sandbox.
func (a *App) inspectRun(ctx context.Context, r Run) (Run, error) {
	return a.inspectRunWithSync(ctx, r, false)
}

func (a *App) inspectRunWithSync(ctx context.Context, r Run, freshSyncProbe bool) (Run, error) {
	if r.Status == StatusFailed {
		r.SyncState = SyncStateUnknown
		return r, nil
	}
	evidence := runEvidence{Agent: r.Agent}
	probed := false
	b, err := a.Runner.Output(ctx, "openshell", a.sandboxArgs(r, "sandbox", "get", r.Sandbox, "-o", "json")...)
	if err != nil {
		// A prepared record has no sandbox yet; keep Creating instead of
		// reporting the missing sandbox, which is the expected state.
		if sandboxNotFound(err) && (r.Status == StatusCreating) {
			r.SyncState = SyncStateUnknown
			return r, nil
		}
		evidence.InspectErr = err.Error()
	} else {
		var status struct {
			Phase string `json:"phase"`
		}
		if unmarshalErr := json.Unmarshal(b, &status); unmarshalErr != nil {
			evidence.Unreadable = true
		} else {
			evidence.Phase = strings.ToLower(status.Phase)
			switch evidence.Phase {
			case "stopped", "exited", "error":
				// A stopped sandbox cannot run the probe; probing it would
				// turn a deliberate stop into Unreachable.
				evidence.Agent = r.Agent
			default:
				probeOut, probeErr := a.sandboxExec(ctx, r, "/bin/sh", "-c", statusProbeScript)
				if probeErr != nil {
					evidence.InspectErr = probeErr.Error()
				} else if parsed, parseErr := parseRunProbe(probeOut); parseErr != nil {
					evidence.InspectErr = parseErr.Error()
				} else {
					evidence = parsed
					evidence.Phase = strings.ToLower(status.Phase)
					evidence.Agent = r.Agent
					probed = true
				}
			}
		}
	}
	status, detail := classifyRunStatus(evidence, time.Now())
	if r.Asleep && status == StatusStopped && (r.Status == StatusStopped || r.Status == StatusCompleted) {
		// AgentBox stopped this sandbox itself, and a stopped sandbox reports
		// nothing else. The record is authoritative: Finish closes a sleeping
		// run without restarting the sandbox, so the phase alone cannot tell
		// Stopped from Completed.
		status, detail = r.Status, r.StatusDetail
	}
	if probed {
		// The exit marker is part of the probe evidence, so only a successful
		// probe may replace the cached value. A running sandbox also proves the
		// run is awake, even when it was started outside AgentBox.
		r.Exited = evidence.ExitCode != nil
		r.ExitCode = evidence.ExitCode
		r.Asleep = false
	}
	r.Status, r.StatusDetail = status, detail
	r.SyncState = a.deriveSyncState(ctx, r, freshSyncProbe)
	return r, a.Store.SaveRun(r)
}

// GetRun returns one run with its status and sync state recomputed by a fresh
// sandbox inspect. Explicit detail reads bypass the bounded overview cache.
func (a *App) GetRun(ctx context.Context, id string) (Run, error) {
	r, err := a.Store.LoadRun(id)
	if err != nil {
		return Run{}, err
	}
	return a.inspectRunWithSync(ctx, r, true)
}

// GetRunWithSyncState returns one stored run with its derived sync state. It
// deliberately skips the status inspect so callers that just changed lifecycle
// state, such as Sleep, can report that state without overwriting it.
func (a *App) GetRunWithSyncState(ctx context.Context, id string) (Run, error) {
	r, err := a.Store.LoadRun(id)
	if err != nil {
		return Run{}, err
	}
	r.SyncState = a.deriveSyncState(ctx, r, true)
	return r, nil
}

func (a *App) requireDirectRunAvailable(ctx context.Context, root string) error {
	runs, err := a.Store.ListRuns()
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.Isolated() || filepath.Clean(run.Source) != root {
			continue
		}
		updated, err := a.inspectRun(ctx, run)
		if err != nil {
			return err
		}
		switch updated.Status {
		case StatusCreating, StatusReady, StatusWorking, StatusWaiting:
			return fmt.Errorf("direct worktree mode already has active run %s (%s); wait for it to finish or enable worktree isolation", updated.ID, updated.Status.Label())
		case StatusUnreachable:
			// Never start a second direct run on a checkout we cannot verify.
			return fmt.Errorf("cannot verify that direct run %s is idle: %s; restore the gateway or enable worktree isolation", updated.ID, updated.StatusDetail)
		}
	}
	return nil
}

func (a *App) Attach(ctx context.Context, id string) error {
	r, err := a.Store.LoadRun(id)
	if err != nil {
		return err
	}
	if err := a.waitForAttachSession(ctx, r, agentSessionName); err != nil {
		return err
	}
	// The CLI attaches the user's real terminal, so the process streams
	// straight through: OpenShell detects the local terminal and sizes the
	// remote pty from it. Routing through the fixed-size pty bridge instead
	// renders tmux at an arbitrary size on top of the user's terminal.
	return a.Runner.Stream(ctx, os.Stdin, os.Stdout, os.Stderr, "openshell", attachArgs(r, agentSessionName)...)
}

const agentTerminalColumns = 120
const agentTerminalRows = 36

// DefaultTerminalSize is the fallback attach size when the client does not
// report its viewport.
var DefaultTerminalSize = TerminalSize{Columns: agentTerminalColumns, Rows: agentTerminalRows}

func attachArgs(r Run, session string) []string {
	args := []string{"sandbox", "exec", "-n", r.Sandbox, "--tty"}
	args = append(args, sandboxLocaleArgs()...)
	args = append(args, "--", "tmux", "attach", "-t", session)
	if r.Gateway != "" {
		args = append([]string{"--gateway", r.Gateway}, args...)
	}
	return args
}

const (
	attachSessionWait  = 30 * time.Second
	attachPollInterval = 250 * time.Millisecond
)

// waitForAttachSession blocks until the tmux session exists. The GUI attaches
// as soon as a run reports running, which is slightly before the sandbox
// bootstrap creates the tmux session; without the wait the attach fails with
// "no sessions" and the terminal stays dead.
func (a *App) waitForAttachSession(ctx context.Context, r Run, session string) error {
	if r.Status != StatusCreating && !r.Status.IsAttachable() {
		detail := r.StatusDetail
		if detail != "" {
			detail = ": " + detail
		}
		return fmt.Errorf("run %s is %s%s; attach requires a live sandbox", r.ID, r.Status.Label(), detail)
	}
	deadline := time.Now().Add(attachSessionWait)
	for {
		_, err := a.sandboxExec(ctx, r, "tmux", "has-session", "-t", session)
		if err == nil {
			return nil
		}
		if !sessionMissing(err) {
			return err
		}
		output, err := a.sandboxExec(ctx, r, "/bin/sh", "-c", "if test -f /workspace/finished; then printf ended; fi")
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(output)) == "ended" {
			return fmt.Errorf("session %q has ended", session)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q did not start", session)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(attachPollInterval):
		}
	}
}

func sessionMissing(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no sessions") ||
		strings.Contains(message, "no such session") ||
		strings.Contains(message, "session not found") ||
		strings.Contains(message, "no server running")
}

// AttachStream attaches the run's agent session.
func (a *App) AttachStream(ctx context.Context, id string, in io.Reader, out, errOut io.Writer, size TerminalSize, resizes <-chan TerminalSize) error {
	return a.AttachSessionStream(ctx, id, agentSessionName, in, out, errOut, size, resizes)
}

// AttachSessionStream attaches a tmux session inside the run's sandbox.
// Shell sessions share the run's worktree and sandbox policy.
func (a *App) AttachSessionStream(ctx context.Context, id, session string, in io.Reader, out, errOut io.Writer, size TerminalSize, resizes <-chan TerminalSize) error {
	if !ValidSession(session) {
		return fmt.Errorf("invalid session %q", session)
	}
	r, err := a.Store.LoadRun(id)
	if err != nil {
		return err
	}
	if err := a.waitForAttachSession(ctx, r, session); err != nil {
		return err
	}
	if !size.Valid() {
		size = DefaultTerminalSize
	}
	if terminalRunner, ok := a.Runner.(TerminalCommandRunner); ok {
		return terminalRunner.StreamTerminal(ctx, in, out, errOut, size.Columns, size.Rows, resizes, "openshell", attachArgs(r, session)...)
	}
	return a.Runner.Stream(ctx, in, out, errOut, "openshell", attachArgs(r, session)...)
}

func (a *App) Logs(ctx context.Context, id string) ([]byte, error) {
	r, err := a.Store.LoadRun(id)
	if err != nil {
		return nil, err
	}
	return a.sandboxExec(ctx, r, "/bin/sh", "-c", "cat /workspace/state/agent.log 2>/dev/null || true")
}

// TailLogs returns the last lines of the agent log. Unfocused workspace tabs
// render this snapshot instead of holding a live terminal.
func (a *App) TailLogs(ctx context.Context, id string, lines int) ([]byte, error) {
	if lines <= 0 {
		return a.Logs(ctx, id)
	}
	if lines > 2000 {
		lines = 2000
	}
	r, err := a.Store.LoadRun(id)
	if err != nil {
		return nil, err
	}
	return a.sandboxExec(ctx, r, "/bin/sh", "-c", fmt.Sprintf("tail -n %d /workspace/state/agent.log 2>/dev/null || true", lines))
}

// Sleep syncs outstanding work and stops the sandbox while keeping the run,
// worktree, and transcript. It replaces the old terminal Stop.
func (a *App) Sleep(ctx context.Context, id string, force bool) error {
	r, err := a.GetRun(ctx, id)
	if err != nil {
		return err
	}
	switch r.Status {
	case StatusReady, StatusWaiting:
	case StatusWorking:
		if !force {
			return fmt.Errorf("run %s is Working; wait for the turn to finish, or rerun with --force to interrupt it", id)
		}
		if _, err := a.sandboxExec(ctx, r, "tmux", "send-keys", "-t", agentSessionName, "C-c"); err != nil {
			return err
		}
		if err := a.waitForStatus(ctx, id, func(r Run) bool { return r.Status != StatusWorking }); err != nil {
			return fmt.Errorf("agent did not stop; the sandbox is still running: %w", err)
		}
	default:
		return fmt.Errorf("run %s is %s; sleep applies to Ready, Working (with --force), or Waiting for input; remove it with `agentbox agent delete --discard %s`", id, r.Status.Label(), id)
	}
	if err := a.Sync(ctx, id); err != nil {
		return fmt.Errorf("sync before sleep failed; the sandbox is still running: %w", err)
	}
	r, err = a.Store.LoadRun(id)
	if err != nil {
		return err
	}
	if _, err := a.Runner.Output(ctx, "openshell", "sandbox", "stop", r.Sandbox, "--gateway", r.Gateway); err != nil {
		return err
	}
	r.Status = StatusStopped
	r.StatusDetail = "asleep; resume available"
	r.Synced = true
	r.Asleep = true
	return a.Store.SaveRun(r)
}

// Stop is the deprecated name for Sleep.
func (a *App) Stop(ctx context.Context, id string) error {
	return a.Sleep(ctx, id, false)
}

// Resume starts a stopped sandbox or relaunches an exited agent in the same
// sandbox and worktree. It never discards the run or the transcript.
func (a *App) Resume(ctx context.Context, id string) error {
	r, err := a.GetRun(ctx, id)
	if err != nil {
		return err
	}
	switch r.Status {
	case StatusStopped:
		if _, err := a.Runner.Output(ctx, "openshell", "sandbox", "start", r.Sandbox, "--gateway", r.Gateway); err != nil {
			return err
		}
		r.Asleep = false
		if err := a.Store.SaveRun(r); err != nil {
			return err
		}
		if err := a.requireResumeEntry(ctx, r); err != nil {
			// A record created before resumable runs cannot continue its
			// conversation; its bootstrap replays the original task prompt.
			// Stop that session and put the sandbox back to sleep.
			_, _ = a.sandboxExec(ctx, r, "/bin/sh", "-c", "tmux kill-session -t agent 2>/dev/null || true")
			if _, stopErr := a.Runner.Output(ctx, "openshell", "sandbox", "stop", r.Sandbox, "--gateway", r.Gateway); stopErr == nil {
				r.Asleep = true
				_ = a.Store.SaveRun(r)
			}
			return err
		}
		if err := a.waitForStatus(ctx, id, func(r Run) bool {
			return !r.Exited && (r.Status == StatusWorking || r.Status == StatusWaiting || r.Status == StatusReady)
		}); err != nil {
			return fmt.Errorf("sandbox started, but the agent did not resume: %w", err)
		}
		return nil
	case StatusReady:
		if !r.Exited {
			return fmt.Errorf("run %s is Ready and its agent is still running; nothing to resume", id)
		}
		if err := a.requireResumeEntry(ctx, r); err != nil {
			return err
		}
	case StatusCreating:
		return fmt.Errorf("run %s is still Creating; wait for the sandbox to start", id)
	default:
		return fmt.Errorf("run %s is %s; resume applies to Stopped runs and Ready runs whose agent exited", id, r.Status.Label())
	}
	switch r.SyncState {
	case SyncStateNeedsSync, SyncStateBlocked:
		// Pending work, or a worktree that drifted from the last sync, is
		// resolved first. Sync itself refuses to overwrite host edits, and a
		// refusal leaves the run resumable.
		if err := a.Sync(ctx, id); err != nil {
			return fmt.Errorf("sync before resume failed; the run stays resumable: %w", err)
		}
	}
	launch := "if tmux has-session -t agent 2>/dev/null; then tmux send-keys -t agent '/bin/sh /workspace/state/agent-entry.sh' Enter; else tmux new-session -d -s agent /bin/sh /workspace/state/agent-entry.sh; fi"
	if _, err := a.sandboxExec(ctx, r, "/bin/sh", "-c", launch); err != nil {
		return err
	}
	if err := a.waitForStatus(ctx, id, func(r Run) bool {
		return !r.Exited && (r.Status == StatusWorking || r.Status == StatusWaiting || r.Status == StatusReady)
	}); err != nil {
		return fmt.Errorf("agent did not resume: %w", err)
	}
	return nil
}

// requireResumeEntry verifies the sandbox carries the continue-or-fresh entry
// point written by PrepareAgent. Runs created by older AgentBox versions only
// hold the original launcher and would replay the task prompt on resume.
func (a *App) requireResumeEntry(ctx context.Context, r Run) error {
	if _, err := a.sandboxExec(ctx, r, "/bin/sh", "-c", "test -f /workspace/state/agent-entry.sh"); err != nil {
		return fmt.Errorf("run %s was created before resumable runs and cannot continue its conversation; finish it or delete it with --discard", r.ID)
	}
	return nil
}

// Finish ends a running run deliberately: it syncs outstanding work, marks the
// run Completed, and keeps the record, worktree, and transcript until delete.
func (a *App) Finish(ctx context.Context, id string) error {
	r, err := a.GetRun(ctx, id)
	if err != nil {
		return err
	}
	switch r.Status {
	case StatusReady, StatusWaiting:
		if err := a.Sync(ctx, id); err != nil {
			return fmt.Errorf("sync before finish failed; the run was not finished: %w", err)
		}
		if _, err := a.sandboxExec(ctx, r, "/bin/sh", "-c", "touch /workspace/finished; tmux kill-session -t agent 2>/dev/null || true"); err != nil {
			return err
		}
	case StatusStopped:
		// A sleeping sandbox has no live agent and nothing to sync: sleep
		// already synced the work. Finish only ends the record.
	case StatusWorking:
		return fmt.Errorf("run %s is Working; wait for the turn to finish or sleep with --force first", id)
	case StatusCompleted:
		return fmt.Errorf("run %s is already Completed", id)
	default:
		return fmt.Errorf("run %s is %s; finish applies to a running run", id, r.Status.Label())
	}
	r, err = a.Store.LoadRun(id)
	if err != nil {
		return err
	}
	wasStopped := r.Status == StatusStopped
	r.Status = StatusCompleted
	r.StatusDetail = "finished by user"
	if !wasStopped && r.Exited && r.ExitCode != nil {
		r.StatusDetail = fmt.Sprintf("exit code %d", *r.ExitCode)
	}
	if !wasStopped {
		r.Synced = true
	}
	return a.Store.SaveRun(r)
}

const lifecycleStatusWait = 30 * time.Second

// waitForStatus polls the derived status until done reports true. Lifecycle
// gates use it instead of sleeping for a fixed time.
func (a *App) waitForStatus(ctx context.Context, id string, done func(Run) bool) error {
	deadline := time.Now().Add(lifecycleStatusWait)
	for {
		r, err := a.GetRun(ctx, id)
		if err != nil {
			return err
		}
		if done(r) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s", lifecycleStatusWait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

const (
	sandboxDeleteTimeout      = 2 * time.Minute
	sandboxDeletePollInterval = 2 * time.Second
)

func (a *App) deleteSandboxAndWait(ctx context.Context, r Run) error {
	_, err := a.Runner.Output(ctx, "openshell", "sandbox", "delete", r.Sandbox, "--gateway", r.Gateway)
	if err != nil {
		if sandboxNotFound(err) {
			return nil
		}
		return err
	}
	return a.waitForSandboxDeletedWith(ctx, r, sandboxDeleteTimeout, sandboxDeletePollInterval)
}

func (a *App) waitForSandboxDeletedWith(ctx context.Context, r Run, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		_, err := a.Runner.Output(ctx, "openshell", "sandbox", "get", r.Sandbox, "-o", "json", "--gateway", r.Gateway)
		if err != nil && sandboxNotFound(err) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("sandbox %s deletion still pending after %s; run record retained", r.Sandbox, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func sandboxNotFound(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "sandbox not found") ||
		strings.Contains(message, "was not found") ||
		strings.Contains(message, "not found")
}

// removeRunWorktree removes the run's worktree. Delete must be repeatable:
// when the worktree is no longer registered with the source repository
// (removed out of band, or by an earlier partially completed delete), git
// refuses with "is not a working tree" and the run record could never be
// deleted. Clear the stale metadata and drop any leftover directory
// directly so deletion still completes.
func (a *App) removeRunWorktree(ctx context.Context, r Run) error {
	_, err := a.git(ctx, r.Source, "worktree", "remove", "--force", r.Worktree)
	if err == nil {
		return nil
	}
	if !worktreeAlreadyGone(err) {
		return err
	}
	if _, err := a.git(ctx, r.Source, "worktree", "prune"); err != nil {
		return err
	}
	return removeLeftoverWorktree(r)
}

// removeRunBranch removes the run's branch, treating an already-deleted
// branch as success so a repeated delete cannot get stuck on it either.
func (a *App) removeRunBranch(ctx context.Context, r Run) error {
	_, err := a.git(ctx, r.Source, "branch", "-D", r.Branch)
	if err == nil || branchAlreadyGone(err) {
		return nil
	}
	return err
}

// worktreeAlreadyGone reports whether a failed worktree removal means the
// worktree is no longer registered with its repository.
func worktreeAlreadyGone(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "is not a working tree") ||
		strings.Contains(message, "does not exist")
}

// branchAlreadyGone reports whether a failed branch deletion means the
// branch no longer exists.
func branchAlreadyGone(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "branch") && strings.Contains(message, "not found")
}

// removeLeftoverWorktree deletes a worktree directory that git no longer
// tracks. Only a path that still looks like a linked worktree of the run's
// source repository is removed, so an unrelated directory recorded in a run
// file is never deleted.
func removeLeftoverWorktree(r Run) error {
	link := filepath.Join(r.Worktree, ".git")
	info, err := os.Lstat(link)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// A .git directory is a main checkout, never a linked worktree.
	if info.IsDir() {
		return nil
	}
	// Linked worktrees point back at their repository through a "gitdir:" file.
	body, err := os.ReadFile(link)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.TrimSpace(string(body)), "gitdir: "+filepath.Join(r.Source, ".git")) {
		return nil
	}
	return os.RemoveAll(r.Worktree)
}

func (a *App) Delete(ctx context.Context, id string, discard bool) error {
	r, err := a.GetRunWithSyncState(ctx, id)
	if err != nil {
		return err
	}
	if r.Status.IsAttachable() && !discard {
		return fmt.Errorf("run is %s; sleep it or pass --discard", r.Status.Label())
	}
	if !discard {
		// Gate on the derived sync state. The persisted Synced flag only
		// records that a sync succeeded once, not whether newer sandbox work
		// exists.
		switch r.SyncState {
		case SyncStateNeedsSync:
			return fmt.Errorf("run has unsynced results; sync first or pass --discard")
		case SyncStateUnknown:
			return fmt.Errorf("cannot verify that the run is synced; restore the sandbox or pass --discard")
		}
	}
	if !discard {
		diff, err := a.Diff(ctx, r)
		if err != nil {
			return err
		}
		if len(diff) > 0 {
			return fmt.Errorf("run has review changes; pass --discard to acknowledge deletion")
		}
	}
	// Prepared records never had a sandbox; every other record is attempted
	// so a partially created sandbox is not orphaned.
	if !(r.Status == StatusCreating && r.Synced) {
		if err := a.deleteSandboxAndWait(ctx, r); err != nil {
			return err
		}
	}
	if r.Isolated() {
		if err := a.removeRunWorktree(ctx, r); err != nil {
			return fmt.Errorf("sandbox removed but worktree cleanup failed: %w", err)
		}
		if err := a.removeRunBranch(ctx, r); err != nil {
			return fmt.Errorf("sandbox removed but branch cleanup failed: %w", err)
		}
	}
	_ = os.RemoveAll(filepath.Join(a.Store.RunsDir(), id+"-config"))
	_ = os.Remove(r.Policy)
	a.invalidateSyncProbe(id)
	return os.Remove(a.Store.RunPath(id))
}
