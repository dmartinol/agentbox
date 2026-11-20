package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Checkout states reported by ProjectCheckout. They describe only local
// remote-tracking refs; deriving them never contacts a remote.
const (
	checkoutUpToDate   = "up-to-date"
	checkoutBehind     = "behind"
	checkoutAhead      = "ahead"
	checkoutDiverged   = "diverged"
	checkoutNoUpstream = "no-upstream"
	checkoutUnknown    = "unknown"
)

// ProjectCheckout reports whether a checkout is behind or ahead of its
// configured upstream. It is derived from local configuration and
// remote-tracking refs only and is never persisted on the project record.
type ProjectCheckout struct {
	State  string `json:"state"`
	Behind int    `json:"behind"`
	Ahead  int    `json:"ahead"`
	Reason string `json:"reason,omitempty"`
}

// Summary renders the human-readable state used by the setup error and the
// GUI. It intentionally carries the meaning without relying on color.
func (c ProjectCheckout) Summary() string {
	switch c.State {
	case checkoutUpToDate:
		return "up to date"
	case checkoutBehind:
		return fmt.Sprintf("%d behind", c.Behind)
	case checkoutAhead:
		return fmt.Sprintf("%d ahead", c.Ahead)
	case checkoutDiverged:
		return fmt.Sprintf("%d ahead, %d behind", c.Ahead, c.Behind)
	case checkoutNoUpstream:
		return "no upstream"
	case checkoutUnknown:
		if c.Reason != "" {
			return fmt.Sprintf("unknown (%s)", c.Reason)
		}
		return "unknown"
	default:
		if c.Reason != "" {
			return fmt.Sprintf("unknown (%s)", c.Reason)
		}
		return "unknown"
	}
}

// ProjectCheckout derives checkout freshness from local refs. Any Git
// failure is reported as an unknown state with a short reason rather than
// an error, so one stale or unusual checkout cannot fail a project list.
func (a *App) ProjectCheckout(ctx context.Context, root string) ProjectCheckout {
	checkout := ProjectCheckout{State: checkoutUnknown}
	if _, err := a.gitOutput(ctx, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err != nil {
		branch, branchErr := a.gitOutput(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
		if branchErr != nil {
			if _, headErr := a.gitOutput(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD^{commit}"); headErr != nil {
				checkout.Reason = shortCheckoutReason(headErr.Error())
				return checkout
			}
			checkout.State = checkoutNoUpstream
			checkout.Reason = "detached HEAD"
			return checkout
		}
		branch = strings.TrimSpace(branch)
		if _, headErr := a.gitOutput(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD^{commit}"); headErr != nil {
			checkout.Reason = fmt.Sprintf("branch %s has no commits", branch)
			return checkout
		}
		upstream, upstreamErr := a.gitOutput(ctx, root, "for-each-ref", "--format=%(upstream)", "refs/heads/"+branch)
		if upstreamErr != nil {
			checkout.Reason = shortCheckoutReason(upstreamErr.Error())
			return checkout
		}
		upstream = strings.TrimSpace(upstream)
		if upstream != "" {
			checkout.Reason = fmt.Sprintf("upstream %s is not available locally; fetch it", displayUpstreamRef(upstream))
			return checkout
		}
		checkout.State = checkoutNoUpstream
		return checkout
	}
	counts, err := a.gitOutput(ctx, root, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		checkout.Reason = shortCheckoutReason(err.Error())
		return checkout
	}
	ahead, behind, err := parseCheckoutCounts(counts)
	if err != nil {
		checkout.Reason = shortCheckoutReason(err.Error())
		return checkout
	}
	checkout.Ahead = ahead
	checkout.Behind = behind
	switch {
	case ahead == 0 && behind == 0:
		checkout.State = checkoutUpToDate
	case ahead == 0:
		checkout.State = checkoutBehind
	case behind == 0:
		checkout.State = checkoutAhead
	default:
		checkout.State = checkoutDiverged
	}
	return checkout
}

func displayUpstreamRef(ref string) string {
	ref = strings.TrimPrefix(ref, "refs/remotes/")
	return strings.TrimPrefix(ref, "refs/heads/")
}

func parseCheckoutCounts(out string) (ahead, behind int, err error) {
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected rev-list output %q", strings.TrimSpace(out))
	}
	ahead, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, fmt.Errorf("unexpected rev-list output %q", strings.TrimSpace(out))
	}
	behind, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, fmt.Errorf("unexpected rev-list output %q", strings.TrimSpace(out))
	}
	return ahead, behind, nil
}

func shortCheckoutReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if index := strings.IndexByte(reason, '\n'); index >= 0 {
		reason = strings.TrimSpace(reason[:index])
	}
	if reason == "" {
		return "git command failed"
	}
	return reason
}

func (a *App) ProjectUsage() (map[string][]string, error) {
	usage := make(map[string][]string)
	runs, err := a.Store.ListRuns()
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		if run.Project == "" {
			continue
		}
		usage[run.Project] = append(usage[run.Project], runDisplayName(run))
	}
	for project := range usage {
		sort.Strings(usage[project])
	}
	return usage, nil
}

func (a *App) DeleteProject(name string) error {
	if _, err := a.Store.LoadProject(name); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &NotFoundError{Resource: "project", Name: name}
		}
		return err
	}
	usage, err := a.ProjectUsage()
	if err != nil {
		return err
	}
	if dependents := usage[name]; len(dependents) > 0 {
		return &ConflictError{Resource: "project", Name: name, Reason: "runs reference it", Dependents: dependents}
	}
	return a.Store.DeleteProject(name)
}

func (a *App) RegisterProject(ctx context.Context, name, root string, worktree bool) (Project, error) {
	canonical, err := a.ProjectRoot(ctx, root)
	if err != nil {
		return Project{}, err
	}
	value := worktree
	project := Project{
		Version:  1,
		Name:     name,
		Root:     canonical,
		Worktree: &value,
	}
	if err := a.Store.SaveProject(project); err != nil {
		return Project{}, err
	}
	return project, nil
}

func (a *App) ProjectRoot(ctx context.Context, root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	body, err := a.git(ctx, absolute, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("%s is not a Git working tree: %w", absolute, err)
	}
	canonical := filepath.Clean(strings.TrimSpace(string(body)))
	if resolved, err := filepath.EvalSymlinks(canonical); err == nil {
		canonical = filepath.Clean(resolved)
	}
	if info, err := os.Stat(canonical); err != nil || !info.IsDir() {
		return "", fmt.Errorf("project root is unavailable")
	}
	return canonical, nil
}
