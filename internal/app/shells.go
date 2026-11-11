package app

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Shell sessions are extra tmux sessions that share a run's sandbox,
// environment, and worktree. Detaching from the GUI leaves them running,
// exactly like the agent session.
var shellSessionName = regexp.MustCompile(`^shell-[1-9][0-9]*$`)

// shellScript mirrors the environment that the agent session gets from
// session.sh so a shell tab sees the same toolchain configuration.
const shellScript = `export HOME=/workspace/state/home; ` +
	`export XDG_CONFIG_HOME=/workspace/state/config; ` +
	`export XDG_DATA_HOME=/workspace/state/data; ` +
	`export CODEX_HOME=/workspace/state/home/.codex; ` +
	`export CLAUDE_CONFIG_DIR=/workspace/state/home/.claude; ` +
	`cd /workspace/project; exec /bin/sh`

// ValidSession reports whether a session name may be attached to.
func ValidSession(name string) bool {
	return name == agentSessionName || shellSessionName.MatchString(name)
}

const agentSessionName = "agent"

func (a *App) listShells(ctx context.Context, r Run) ([]string, error) {
	out, err := a.sandboxExec(ctx, r, "tmux", "list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, 4)
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if shellSessionName.MatchString(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// ListShells returns the extra shell sessions of a run.
func (a *App) ListShells(ctx context.Context, id string) ([]string, error) {
	r, err := a.Store.LoadRun(id)
	if err != nil {
		return nil, err
	}
	return a.listShells(ctx, r)
}

// CreateShell starts a detached shell session in the run's sandbox and
// returns its name. The shell starts in the run's worktree.
func (a *App) CreateShell(ctx context.Context, id string) (string, error) {
	r, err := a.Store.LoadRun(id)
	if err != nil {
		return "", err
	}
	existing, err := a.listShells(ctx, r)
	if err != nil {
		return "", err
	}
	used := make(map[string]bool, len(existing))
	for _, name := range existing {
		used[name] = true
	}
	name := ""
	for index := 1; index <= 99; index++ {
		candidate := "shell-" + strconv.Itoa(index)
		if !used[candidate] {
			name = candidate
			break
		}
	}
	if name == "" {
		return "", fmt.Errorf("run %s already has too many shell sessions", id)
	}
	if _, err := a.sandboxExec(ctx, r, "tmux", "new-session", "-d", "-s", name, "-c", "/workspace/project", "/bin/sh", "-c", shellScript); err != nil {
		return "", fmt.Errorf("start shell session: %w", err)
	}
	return name, nil
}

// DeleteShell stops a shell session. The agent session is never removable.
func (a *App) DeleteShell(ctx context.Context, id, name string) error {
	if !shellSessionName.MatchString(name) {
		return fmt.Errorf("invalid shell session %q", name)
	}
	r, err := a.Store.LoadRun(id)
	if err != nil {
		return err
	}
	if _, err := a.sandboxExec(ctx, r, "tmux", "kill-session", "-t", name); err != nil {
		return fmt.Errorf("stop shell session: %w", err)
	}
	return nil
}
