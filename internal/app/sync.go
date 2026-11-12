package app

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) sandboxArgs(r Run, parts ...string) []string {
	args := append([]string{}, parts...)
	if r.Gateway != "" {
		args = append(args, "--gateway", r.Gateway)
	}
	return args
}

// sandboxLocale is the UTF-8 locale requested for sandbox commands. The
// AgentBox images ship no locale, so processes default to POSIX. tmux
// replaces anything it cannot encode with "_" when its client runs under a
// non-UTF-8 locale, which turned every typographic apostrophe in agent output
// into "_" in the console.
const sandboxLocale = "C.UTF-8"

func sandboxLocaleArgs() []string {
	return []string{"--env", "LC_ALL=" + sandboxLocale, "--env", "LANG=" + sandboxLocale}
}

func (a *App) sandboxExec(ctx context.Context, r Run, command ...string) ([]byte, error) {
	args := []string{"sandbox", "exec", "-n", r.Sandbox, "--no-login-shell"}
	args = append(args, sandboxLocaleArgs()...)
	args = append(args, "--")
	args = append(args, command...)
	if r.Gateway != "" {
		args = append([]string{"--gateway", r.Gateway}, args...)
	}
	return a.Runner.Output(ctx, "openshell", args...)
}

func (a *App) download(ctx context.Context, r Run, remote, local string) error {
	args := []string{"sandbox", "download", r.Sandbox, remote, local}
	if r.Gateway != "" {
		args = append([]string{"--gateway", r.Gateway}, args...)
	}
	_, err := a.Runner.Output(ctx, "openshell", args...)
	return err
}

func (a *App) upload(ctx context.Context, r Run, local, remote string) error {
	args := []string{"sandbox", "upload", r.Sandbox, local, remote, "--no-git-ignore"}
	if r.Gateway != "" {
		args = append([]string{"--gateway", r.Gateway}, args...)
	}
	_, err := a.Runner.Output(ctx, "openshell", args...)
	return err
}

func (a *App) Sync(ctx context.Context, id string) (err error) {
	r, err := a.Store.LoadRun(id)
	if err != nil {
		return err
	}
	fp, err := a.WorktreeFingerprint(ctx, r)
	if err != nil {
		return err
	}
	if fp != r.Fingerprint {
		return fmt.Errorf("local run worktree changed since the last sync; refusing to overwrite it")
	}
	stage, err := os.MkdirTemp(a.Store.RunsDir(), id+"-sync-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	remote := "set -eu; git -C /workspace/project bundle create /workspace/export.bundle " + shellQuote(r.Branch) + "; git -C /workspace/project diff HEAD --binary --full-index > /workspace/export.patch; git -C /workspace/project ls-files --others --exclude-standard -z > /workspace/export.list; tar -C /workspace/project --null -T /workspace/export.list -cf /workspace/export.tar"
	if _, err := a.sandboxExec(ctx, r, "/bin/sh", "-c", remote); err != nil {
		return err
	}
	bundle := filepath.Join(stage, "export.bundle")
	patch := filepath.Join(stage, "export.patch")
	archive := filepath.Join(stage, "export.tar")
	for _, pair := range [][2]string{{"/workspace/export.bundle", bundle}, {"/workspace/export.patch", patch}, {"/workspace/export.tar", archive}} {
		if err := a.download(ctx, r, pair[0], pair[1]); err != nil {
			return err
		}
	}
	newPatch, err := os.ReadFile(patch)
	if err != nil {
		return err
	}
	if err := validateTar(archive); err != nil {
		return err
	}
	oldHeadB, err := a.git(ctx, r.Worktree, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	oldHead := strings.TrimSpace(string(oldHeadB))
	oldPatchPath := filepath.Join(a.Store.RunsDir(), id+"-mirror.patch")
	oldTarPath := filepath.Join(a.Store.RunsDir(), id+"-mirror.tar")
	oldPatch, _ := os.ReadFile(oldPatchPath)
	_, oldTarErr := os.Stat(oldTarPath)
	mutated := false
	defer func() {
		if err == nil || !mutated {
			return
		}
		_, resetErr := a.git(ctx, r.Worktree, "reset", "--hard", oldHead)
		if resetErr == nil && len(oldPatch) > 0 {
			_ = atomicWrite(filepath.Join(stage, "restore.patch"), oldPatch)
			_, resetErr = a.git(ctx, r.Worktree, "apply", "--binary", filepath.Join(stage, "restore.patch"))
		}
		if resetErr == nil && oldTarErr == nil {
			resetErr = extractTar(oldTarPath, r.Worktree)
		}
		if resetErr != nil {
			err = fmt.Errorf("%w; rollback failed: %v", err, resetErr)
		}
	}()
	if len(oldPatch) > 0 {
		if _, err = a.git(ctx, r.Worktree, "apply", "--reverse", "--binary", oldPatchPath); err != nil {
			return err
		}
		mutated = true
	}
	if oldTarErr == nil {
		if err = removeTarEntries(oldTarPath, r.Worktree); err != nil {
			return err
		}
		mutated = true
	}
	if _, err = a.git(ctx, r.Worktree, "fetch", bundle, r.Branch); err != nil {
		return err
	}
	if _, err = a.git(ctx, r.Worktree, "merge", "--ff-only", "FETCH_HEAD"); err != nil {
		return err
	}
	mutated = true
	if len(newPatch) > 0 {
		if _, err = a.git(ctx, r.Worktree, "apply", "--check", "--binary", patch); err != nil {
			return err
		}
		if _, err = a.git(ctx, r.Worktree, "apply", "--binary", patch); err != nil {
			return err
		}
	}
	if err = extractTar(archive, r.Worktree); err != nil {
		return err
	}
	r.Fingerprint, err = a.WorktreeFingerprint(ctx, r)
	if err != nil {
		return err
	}
	headB, err := a.git(ctx, r.Worktree, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	r.LastSynced = strings.TrimSpace(string(headB))
	r.Synced = true
	if err = atomicWrite(oldPatchPath, newPatch); err != nil {
		return err
	}
	archiveBytes, readErr := os.ReadFile(archive)
	if readErr != nil {
		return readErr
	}
	if err = atomicWrite(oldTarPath, archiveBytes); err != nil {
		return err
	}
	return a.Store.SaveRun(r)
}

func validateTar(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if !safeArchivePath(h.Name) {
			return fmt.Errorf("unsafe archive path %q", h.Name)
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA && h.Typeflag != tar.TypeSymlink {
			return fmt.Errorf("unsupported archive entry %q", h.Name)
		}
		if h.Typeflag == tar.TypeSymlink && !safeArchiveSymlink(h.Name, h.Linkname) {
			return fmt.Errorf("unsafe archive symlink %q -> %q", h.Name, h.Linkname)
		}
	}
}

func safeArchivePath(p string) bool {
	return p != "" && p != "." && !filepath.IsAbs(p) && !strings.HasPrefix(filepath.Clean(p), "..") && !strings.ContainsRune(p, 0)
}

func safeArchiveSymlink(name, target string) bool {
	if target == "" || filepath.IsAbs(target) || strings.ContainsRune(target, 0) {
		return false
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(name), target))
	return resolved != ".." && !strings.HasPrefix(resolved, ".."+string(filepath.Separator))
}

func extractTar(path, dst string) error {
	if err := validateTar(path); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dst, h.Name)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("untracked path already exists: %s", h.Name)
		}
		if h.Typeflag == tar.TypeSymlink {
			if err := os.Symlink(h.Linkname, target); err != nil {
				return err
			}
		} else {
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(h.Mode)&0777)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
}

func removeTarEntries(path, dst string) error {
	if err := validateTar(path); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(dst, h.Name)); err != nil {
			return err
		}
	}
}

func tarPaths(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	var out []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, h.Name)
	}
}

func samePatch(a, b []byte) bool { return bytes.Equal(a, b) }
