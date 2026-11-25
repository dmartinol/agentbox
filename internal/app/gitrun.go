package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	all := append([]string{"-C", dir}, args...)
	return a.Runner.Output(ctx, "git", all...)
}

func (a *App) CreateWorktree(ctx context.Context, source, id string) (Run, error) {
	return a.CreateWorktreeAt(ctx, source, id, filepath.Join(a.Store.WorktreesDir(), id), "agentbox/"+id)
}

func (a *App) CreateWorktreeAt(ctx context.Context, source, id, worktree, branch string) (Run, error) {
	if err := a.Store.Ensure(); err != nil {
		return Run{}, err
	}
	rootB, err := a.git(ctx, source, "rev-parse", "--show-toplevel")
	if err != nil {
		return Run{}, err
	}
	root := strings.TrimSpace(string(rootB))
	headB, err := a.git(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return Run{}, err
	}
	head := strings.TrimSpace(string(headB))
	trackedStage, err := a.git(ctx, root, "ls-files", "-z", "--", ".agentbox-stage")
	if err != nil {
		return Run{}, err
	}
	if paths := splitNUL(trackedStage); len(paths) > 0 {
		return Run{}, fmt.Errorf("run %s cannot start: %s is tracked; .agentbox-stage/ is reserved for stage handoffs and cannot be tracked", id, paths[0])
	}
	changed, err := a.git(ctx, root, "diff", "--name-only", "-z", "--ignore-submodules=none", "--no-ext-diff", "HEAD")
	if err != nil {
		return Run{}, err
	}
	for _, path := range splitNUL(changed) {
		treeEntry, err := a.git(ctx, root, "ls-tree", "-z", "--full-name", "HEAD", "--", path)
		if err != nil {
			return Run{}, err
		}
		indexEntry, err := a.git(ctx, root, "ls-files", "-s", "-z", "--", path)
		if err != nil {
			return Run{}, err
		}
		treeFields := strings.Fields(string(treeEntry))
		indexFields := strings.Fields(string(indexEntry))
		if (len(treeFields) > 1 && (treeFields[0] == "160000" || treeFields[1] == "commit")) ||
			(len(indexFields) > 0 && indexFields[0] == "160000") {
			return Run{}, fmt.Errorf("run %s cannot start: changed Git submodule %q cannot be reproduced by the starting snapshot patch", id, path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(worktree), 0700); err != nil {
		return Run{}, err
	}
	if _, err := os.Stat(worktree); err == nil {
		return Run{}, fmt.Errorf("run worktree already exists: %s", worktree)
	} else if !os.IsNotExist(err) {
		return Run{}, err
	}
	if _, err := a.git(ctx, root, "worktree", "add", "-b", branch, worktree, head); err != nil {
		return Run{}, err
	}
	patchPath := runArtifactPath(a.Store, id, runArtifactInitialPatch)
	rollback := func() {
		_, _ = a.git(ctx, root, "worktree", "remove", "--force", worktree)
		_, _ = a.git(ctx, root, "branch", "-D", branch)
		_ = os.Remove(patchPath)
	}
	patch, err := a.git(ctx, root, "diff", "HEAD", "--binary", "--full-index", "--no-ext-diff", "--ignore-submodules=none")
	if err != nil {
		rollback()
		return Run{}, err
	}
	if len(patch) > 0 {
		if err := atomicWrite(patchPath, patch); err != nil {
			rollback()
			return Run{}, err
		}
		if _, err := a.git(ctx, worktree, "apply", "--check", "--binary", patchPath); err != nil {
			rollback()
			return Run{}, fmt.Errorf("prepare starting snapshot for run %s: %w", id, err)
		}
		if _, err := a.git(ctx, worktree, "apply", "--binary", patchPath); err != nil {
			rollback()
			return Run{}, fmt.Errorf("prepare starting snapshot for run %s: %w", id, err)
		}
	}
	if _, err := a.git(ctx, worktree, "add", "--all"); err != nil {
		rollback()
		return Run{}, err
	}
	// The baseline commit distinguishes pre-existing edits from agent changes.
	if _, err := a.git(ctx, worktree, "-c", "user.name=AgentBox", "-c", "user.email=agentbox@local.invalid", "commit", "--allow-empty", "-m", "AgentBox starting snapshot"); err != nil {
		rollback()
		return Run{}, err
	}
	baselineB, err := a.git(ctx, worktree, "rev-parse", "HEAD")
	if err != nil {
		rollback()
		return Run{}, err
	}
	r := Run{ID: id, Source: root, Worktree: worktree, Branch: branch, Baseline: strings.TrimSpace(string(baselineB)), LastSynced: strings.TrimSpace(string(baselineB)), Sandbox: id, Status: StatusCreating, Synced: true}
	fp, err := a.WorktreeFingerprint(ctx, r)
	if err != nil {
		rollback()
		return Run{}, err
	}
	r.Fingerprint = fp
	return r, nil
}

func (a *App) CreateDirectRun(ctx context.Context, source, id string) (Run, error) {
	root, err := a.ProjectRoot(ctx, source)
	if err != nil {
		return Run{}, err
	}
	status, err := a.git(ctx, root, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return Run{}, err
	}
	if len(strings.TrimSpace(string(status))) > 0 {
		return Run{}, fmt.Errorf("direct worktree mode requires a clean checkout; commit, stash, or remove local changes first")
	}
	headB, err := a.git(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return Run{}, err
	}
	branchB, err := a.git(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return Run{}, fmt.Errorf("direct worktree mode requires an attached branch: %w", err)
	}
	head := strings.TrimSpace(string(headB))
	branch := strings.TrimSpace(string(branchB))
	r := Run{
		ID:               id,
		Source:           root,
		Worktree:         root,
		Branch:           branch,
		Baseline:         head,
		LastSynced:       head,
		Sandbox:          id,
		Status:           StatusCreating,
		Synced:           true,
		WorktreeIsolated: false,
	}
	fp, err := a.WorktreeFingerprint(ctx, r)
	if err != nil {
		return Run{}, err
	}
	r.Fingerprint = fp
	return r, nil
}

func splitNUL(b []byte) []string {
	var out []string
	for _, s := range strings.Split(string(b), "\x00") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (a *App) WorktreeFingerprint(ctx context.Context, r Run) (string, error) {
	diff, err := a.git(ctx, r.Worktree, "diff", "HEAD", "--binary", "--full-index")
	if err != nil {
		return "", err
	}
	list, err := a.git(ctx, r.Worktree, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write(diff)
	h.Write(list)
	for _, name := range splitNUL(list) {
		info, err := os.Lstat(filepath.Join(r.Worktree, name))
		if err != nil {
			return "", err
		}
		h.Write([]byte(name))
		h.Write([]byte(info.Mode().String()))
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(filepath.Join(r.Worktree, name))
			if err != nil {
				return "", err
			}
			h.Write([]byte(target))
		} else {
			f, err := os.Open(filepath.Join(r.Worktree, name))
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(h, f); err != nil {
				f.Close()
				return "", err
			}
			f.Close()
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (a *App) MakeBundle(ctx context.Context, r Run) (string, error) {
	path := runArtifactPath(a.Store, r.ID, runArtifactSourceBundle)
	if _, err := a.git(ctx, r.Worktree, "bundle", "create", path, r.Branch); err != nil {
		return "", err
	}
	return path, nil
}

func (a *App) Diff(ctx context.Context, r Run) ([]byte, error) {
	b, err := a.git(ctx, r.Worktree, "diff", r.Baseline, "--binary")
	if err != nil {
		return nil, err
	}
	untracked, err := a.git(ctx, r.Worktree, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	if len(untracked) > 0 {
		b = append(b, []byte("\nUntracked files:\n"+string(untracked))...)
	}
	return b, nil
}
