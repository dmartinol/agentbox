package skills

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// harnessAgentsDir is the global agent-definition directory a harness scans at
// session start. Roles are global-only: --local and --dir never write here.
func (l layout) harnessAgentsDir(harness string) string {
	switch harness {
	case "claude":
		return filepath.Join(l.home, ".claude", "agents")
	case "codex":
		return filepath.Join(l.home, ".codex", "agents")
	case "opencode":
		return filepath.Join(l.home, ".config", "opencode", "agents")
	}
	return ""
}

// rolePath is the installed path of one role for one harness.
func (l layout) rolePath(harness, role string) string {
	ext, ok := RoleFile(harness)
	if !ok {
		return ""
	}
	return filepath.Join(l.harnessAgentsDir(harness), role+ext)
}

// skippedRoleResults reports every role as skipped for a scope that cannot own
// a global agent definition.
func skippedRoleResults(detail string) []Result {
	out := make([]Result, 0, len(roleOrder))
	for _, name := range roleOrder {
		out = append(out, Result{Skill: name, Target: "roles", Action: "skipped", Detail: detail})
	}
	return out
}

const (
	rolesLocalSkip  = "agent roles are global-only; omit --local"
	rolesDirSkip    = "agent roles are global-only; --dir cannot install them"
	rolesUpdateSkip = "agent roles are global-only"
)

// installRoles writes or refreshes the selected harness renderings of every
// role and records them in the manifest. Update never creates a missing role.
func installRoles(l layout, agents []string, manifest *Manifest, opts Options, mode lifecycleMode, bk *backupSet) ([]Result, error) {
	if manifest.Roles == nil {
		manifest.Roles = map[string]ManifestRole{}
	}
	var results []Result
	var failures []error
	for _, name := range roleOrder {
		role, err := RoleByName(name)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		record, hasRecord := manifest.Roles[name]
		next := ManifestRole{Version: record.Version, Harnesses: map[string]ManifestRoleFile{}}
		for harness, file := range record.Harnesses {
			next.Harnesses[harness] = file
		}
		owned := false
		for _, harness := range agents {
			content, err := RenderRole(role, harness, "", "", opts.Version)
			if err != nil {
				failures = append(failures, err)
				continue
			}
			path := l.rolePath(harness, name)
			tracked, trackedFile := record.File(harness)
			res, ours, err := ensureRoleFile(name, harness, path, content, tracked, trackedFile, opts, mode, bk)
			res.Detail = roleDetail(res.Detail, harness, role)
			results = append(results, res)
			if err != nil {
				failures = append(failures, err)
			}
			if ours {
				next.Harnesses[harness] = ManifestRoleFile{Path: path, SHA256: hashHex(content)}
				owned = true
			}
		}
		if owned {
			next.Version = effectiveVersion(opts.Version)
		}
		if hasRecord || len(next.Harnesses) > 0 {
			manifest.Roles[name] = next
		}
	}
	return results, errors.Join(failures...)
}

// ensureRoleFile writes one harness rendering of a role. ours reports whether
// the file on disk now matches the embedded definition.
func ensureRoleFile(role, harness, path string, content []byte, tracked ManifestRoleFile, trackedFile bool, opts Options, mode lifecycleMode, bk *backupSet) (Result, bool, error) {
	res := Result{Skill: role, Target: harness, Path: path}
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if mode == modeUpdate {
			res.Action, res.Detail = "skipped", "not installed; run `agentbox skills install`"
			return res, false, nil
		}
		if opts.DryRun {
			res.Action = "would-install"
			return res, true, nil
		}
		if err := writeFile(path, content); err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		res.Action = "installed"
		return res, true, nil
	case err != nil:
		res.Action, res.Detail = "skipped", err.Error()
		return res, false, err
	}
	if info.IsDir() {
		return replaceRolePath(res, path, content, tracked, trackedFile, opts, bk, "existing directory at the agent path")
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		if !opts.Force {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		return replaceRolePath(res, path, content, tracked, trackedFile, opts, bk, "unreadable agent file")
	}
	if bytes.Equal(existing, content) {
		res.Action = "unchanged"
		return res, true, nil
	}
	if trackedFile && hashHex(existing) == tracked.SHA256 {
		if opts.DryRun {
			res.Action = "would-update"
			return res, true, nil
		}
		if err := writeFile(path, content); err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		res.Action, res.Detail = "updated", "refreshed a stale copy"
		return res, true, nil
	}
	return replaceRolePath(res, path, content, tracked, trackedFile, opts, bk, "existing agent file differs from the embedded role")
}

// replaceRolePath backs up and replaces a foreign path or a modified copy when
// --force is set.
func replaceRolePath(res Result, path string, content []byte, _ ManifestRoleFile, _ bool, opts Options, bk *backupSet, problem string) (Result, bool, error) {
	if !opts.Force {
		res.Action, res.Detail = "skipped", problem+"; use --force to replace it"
		if opts.DryRun {
			return res, false, nil
		}
		return res, false, fmt.Errorf("refusing to overwrite %s; use --force", path)
	}
	if opts.DryRun {
		res.Action, res.Detail = "would-update", "replace the "+problem+" (backed up first)"
		return res, true, nil
	}
	if err := bk.move(path, filepath.Join("roles", res.Target, res.Skill)); err != nil {
		res.Action, res.Detail = "skipped", err.Error()
		return res, false, err
	}
	if err := writeFile(path, content); err != nil {
		res.Action, res.Detail = "skipped", err.Error()
		return res, false, err
	}
	res.Action, res.Detail = "updated", "replaced the "+problem+" (backed up)"
	return res, true, nil
}

// roleDetail appends the Codex write-capability limitation to a result, so the
// install output never implies enforcement the harness cannot provide.
func roleDetail(detail, harness string, role Role) string {
	note := RoleWriteLimitation(harness, role)
	switch {
	case note == "":
		return detail
	case detail == "":
		return note
	default:
		return detail + "; " + note
	}
}

// removeRoles deletes manifest-owned role files. With a harness filter only
// the recordings of those harnesses are removed. Files AgentBox did not record
// are foreign and left alone.
func removeRoles(l layout, agents []string, full bool, manifest *Manifest, opts Options, bk *backupSet) ([]Result, error) {
	var results []Result
	var failures []error
	for _, name := range roleOrder {
		record, ok := manifest.Roles[name]
		if !ok {
			continue
		}
		for harness, file := range record.Harnesses {
			if !full && !containsString(agents, harness) {
				continue
			}
			res, gone, err := removeRoleFile(name, harness, file, opts, bk)
			results = append(results, res)
			if err != nil {
				failures = append(failures, err)
			}
			if gone {
				delete(record.Harnesses, harness)
			}
		}
		if len(record.Harnesses) == 0 {
			delete(manifest.Roles, name)
			continue
		}
		manifest.Roles[name] = record
	}
	return results, errors.Join(failures...)
}

// removeRoleFile deletes one recorded role file. gone reports whether the
// recording may be dropped.
func removeRoleFile(role, harness string, file ManifestRoleFile, opts Options, bk *backupSet) (Result, bool, error) {
	res := Result{Skill: role, Target: harness, Path: file.Path}
	body, err := os.ReadFile(file.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		res.Action, res.Detail = "missing", "agent file is already gone"
		return res, true, nil
	case err != nil:
		if !opts.Force {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		if opts.DryRun {
			res.Action, res.Detail = "would-remove", "unreadable agent file; backed up first"
			return res, false, nil
		}
		if err := bk.move(file.Path, filepath.Join("roles", harness, role)); err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		res.Action, res.Detail = "removed", "unreadable agent file backed up before removal"
		return res, true, nil
	}
	if hashHex(body) != file.SHA256 {
		if !opts.Force {
			res.Action, res.Detail = "skipped", "copy differs from the managed manifest; use --force to remove it"
			if opts.DryRun {
				return res, false, nil
			}
			return res, false, fmt.Errorf("refusing to remove modified agent file %s; use --force", file.Path)
		}
		if opts.DryRun {
			res.Action, res.Detail = "would-remove", "modified copy; backed up first"
			return res, false, nil
		}
		if err := bk.move(file.Path, filepath.Join("roles", harness, role)); err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		res.Action, res.Detail = "removed", "modified copy backed up before removal"
		return res, true, nil
	}
	if opts.DryRun {
		res.Action = "would-remove"
		return res, false, nil
	}
	if err := os.Remove(file.Path); err != nil {
		res.Action, res.Detail = "skipped", err.Error()
		return res, false, err
	}
	res.Action = "removed"
	return res, true, nil
}
