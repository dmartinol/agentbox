package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const bootstrapScript = `#!/bin/sh
set -eu
export LANG=C.UTF-8
export LC_ALL=C.UTF-8
tmux new-session -d -s agent /bin/sh /workspace/state/session.sh
tmux set-option -g remain-on-exit on
tmux pipe-pane -t agent -o 'cat >> /workspace/state/agent.log'
while [ "$(tmux display-message -p -t agent '#{pane_dead}')" != 1 ]; do sleep 1; done
touch /workspace/finished
sleep 1
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
	if opt.Image != "" {
		p.Image = opt.Image
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
		return Run{}, err
	}
	policy, err := os.ReadFile(p.Policy)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Run{}, fmt.Errorf("profile %q references missing policy file %s; edit the profile or restore the policy", p.Name, p.Policy)
		}
		return Run{}, err
	}
	policy, err = PolicyForAgent(policy, p.Agent)
	if err != nil {
		return Run{}, err
	}
	if _, err := ParsePolicy(policy); err != nil {
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
	if _, err := a.Runner.Output(ctx, "podman", "image", "inspect", p.Image); err != nil {
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
	preview := Run{ID: id, Name: runName, Source: sourceRoot, Project: projectName, Gateway: opt.Gateway, Agent: p.Agent, Image: p.Image, Mode: p.Mode, Model: p.Model, Approval: p.Approval, Policy: p.Policy, Status: StatusCreating, StatusDetail: "dry run preview", WorktreeIsolated: useWorktree}
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
	r.WorktreeIsolated = useWorktree
	r.Created = time.Now().UTC()
	r.Gateway = opt.Gateway
	r.Agent = p.Agent
	r.Image = p.Image
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
	args := []string{"sandbox", "create", "--name", r.Sandbox, "--from", p.Image, "--provider", p.Provider, "--policy", r.Policy, "--detach", "--no-auto-providers", "--approval-mode", "manual", "--label", "agentbox-run=" + id, "--gateway", opt.Gateway, "--", "/bin/sh", "-c", "while [ ! -f /workspace/ready ]; do sleep 1; done; exec /bin/sh /workspace/state/bootstrap.sh"}
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

func (a *App) VerifyEffectivePolicy(ctx context.Context, r Run, p Profile) error {
	b, err := a.Runner.Output(ctx, "openshell", "policy", "get", r.Sandbox, "--full", "-o", "json", "--gateway", r.Gateway)
	if err != nil {
		return err
	}
	s := strings.ToLower(string(b))
	if r.Mode == "gateway" {
		endpoint, err := url.Parse(p.BaseURL)
		if err != nil || endpoint.Host == "" {
			return fmt.Errorf("gateway profile lacks a valid model API base URL")
		}
		if !strings.Contains(s, strings.ToLower(endpoint.Host)) {
			return fmt.Errorf("effective policy lacks gateway endpoint %q; provider not ready", endpoint.Host)
		}
		if strings.Contains(s, "api.openai.com") || strings.Contains(s, "api.anthropic.com") {
			return fmt.Errorf("effective policy allows direct model-provider egress in gateway mode")
		}
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
		if updated.Status == StatusCompleted && !updated.Synced {
			if syncErr := a.Sync(ctx, updated.ID); syncErr == nil {
				if reloaded, loadErr := a.Store.LoadRun(updated.ID); loadErr == nil {
					updated = reloaded
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
	if r.Status == StatusFailed {
		return r, nil
	}
	evidence := runEvidence{Agent: r.Agent}
	b, err := a.Runner.Output(ctx, "openshell", a.sandboxArgs(r, "sandbox", "get", r.Sandbox, "-o", "json")...)
	if err != nil {
		// A prepared record has no sandbox yet; keep Creating instead of
		// reporting the missing sandbox, which is the expected state.
		if sandboxNotFound(err) && (r.Status == StatusCreating) {
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
			probeOut, probeErr := a.sandboxExec(ctx, r, "/bin/sh", "-c", statusProbeScript)
			if probeErr != nil {
				evidence.InspectErr = probeErr.Error()
			} else if parsed, parseErr := parseRunProbe(probeOut); parseErr != nil {
				evidence.InspectErr = parseErr.Error()
			} else {
				evidence = parsed
				evidence.Phase = strings.ToLower(status.Phase)
				evidence.Agent = r.Agent
			}
		}
	}
	status, detail := classifyRunStatus(evidence, time.Now())
	if status == StatusStopped && r.Status == StatusStopped && r.StatusDetail != "" {
		// Keep the reason Stop recorded instead of the generic sandbox phase.
		detail = r.StatusDetail
	}
	r.Status, r.StatusDetail = status, detail
	return r, a.Store.SaveRun(r)
}

// GetRun returns one run with its status recomputed by a sandbox inspect.
func (a *App) GetRun(ctx context.Context, id string) (Run, error) {
	r, err := a.Store.LoadRun(id)
	if err != nil {
		return Run{}, err
	}
	return a.inspectRun(ctx, r)
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

func (a *App) Stop(ctx context.Context, id string) error {
	r, err := a.GetRun(ctx, id)
	if err != nil {
		return err
	}
	if !r.Status.IsStoppable() {
		return fmt.Errorf("run %s is %s; stop applies to Ready, Working, or Waiting for input (use `agentbox agent delete %s --discard` to discard it)", id, r.Status.Label(), id)
	}
	if _, err := a.sandboxExec(ctx, r, "tmux", "send-keys", "-t", "agent", "C-c"); err != nil {
		return err
	}
	if _, err := a.sandboxExec(ctx, r, "/bin/sh", "-c", "test -f /workspace/idle"); err == nil {
		if _, err := a.sandboxExec(ctx, r, "tmux", "send-keys", "-t", "agent", "f", "Enter"); err != nil {
			return err
		}
	}
	finished := false
	for i := 0; i < 30; i++ {
		if _, err := a.sandboxExec(ctx, r, "/bin/sh", "-c", "test -f /workspace/finished"); err == nil {
			finished = true
			break
		}
		time.Sleep(time.Second)
	}
	if !finished {
		return fmt.Errorf("agent did not stop within 30 seconds; sandbox retained")
	}
	if err := a.Sync(ctx, id); err != nil {
		return err
	}
	if _, err := a.Runner.Output(ctx, "openshell", "sandbox", "stop", r.Sandbox, "--gateway", r.Gateway); err != nil {
		return err
	}
	r, err = a.Store.LoadRun(id)
	if err != nil {
		return err
	}
	r.Status = StatusStopped
	r.StatusDetail = "stopped by user"
	r.Synced = true
	return a.Store.SaveRun(r)
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

func (a *App) Delete(ctx context.Context, id string, discard bool) error {
	r, err := a.Store.LoadRun(id)
	if err != nil {
		return err
	}
	if r.Status.IsStoppable() && !discard {
		return fmt.Errorf("run is %s; stop it or pass --discard", r.Status.Label())
	}
	if !r.Synced && !discard {
		return fmt.Errorf("run has unsynced results; sync first or pass --discard")
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
		if _, err := a.git(ctx, r.Source, "worktree", "remove", "--force", r.Worktree); err != nil {
			return fmt.Errorf("sandbox removed but worktree cleanup failed: %w", err)
		}
		if _, err := a.git(ctx, r.Source, "branch", "-D", r.Branch); err != nil {
			return fmt.Errorf("sandbox removed but branch cleanup failed: %w", err)
		}
	}
	_ = os.RemoveAll(filepath.Join(a.Store.RunsDir(), id+"-config"))
	_ = os.Remove(r.Policy)
	return os.Remove(a.Store.RunPath(id))
}
