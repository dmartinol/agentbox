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
	rollback := func() {
		_, _ = a.git(ctx, root, "worktree", "remove", "--force", worktree)
		_, _ = a.git(ctx, root, "branch", "-D", branch)
	}
	patch, err := a.git(ctx, root, "diff", "HEAD", "--binary", "--full-index")
	if err != nil {
		rollback()
		return Run{}, err
	}
	if len(patch) > 0 {
		patchPath := filepath.Join(a.Store.RunsDir(), id+"-initial.patch")
		if err := atomicWrite(patchPath, patch); err != nil {
			rollback()
			return Run{}, err
		}
		if _, err := a.git(ctx, worktree, "apply", "--binary", patchPath); err != nil {
			rollback()
			return Run{}, err
		}
	}
	untracked, err := a.git(ctx, root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		rollback()
		return Run{}, err
	}
	for _, p := range splitNUL(untracked) {
		if err := copySnapshotEntry(filepath.Join(root, p), filepath.Join(worktree, p)); err != nil {
			rollback()
			return Run{}, err
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

func copySnapshotEntry(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported untracked file type: %s", src)
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
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
	path := filepath.Join(a.Store.RunsDir(), r.ID+"-source.bundle")
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
