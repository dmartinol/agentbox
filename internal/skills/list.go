package skills

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ListOptions controls List.
type ListOptions struct {
	// Version is the running binary version to compare against.
	Version string
	// Home overrides the user home directory used to resolve the global
	// canonical root.
	Home string
	// Local reads the project-scoped root ./.agents/skills instead of the
	// global one.
	Local bool
	// ProjectDir overrides the --local base directory.
	ProjectDir string
}

// Installation describes one place a harness can see an embedded skill.
// State is current, stale, modified, invalid, missing, or unreadable for the
// canonical copy; current, missing, broken-link, or foreign for links; and
// missing or legacy for the report-only Codex row.
type Installation struct {
	Target  string `json:"target"`
	Path    string `json:"path"`
	Present bool   `json:"present"`
	Version string `json:"version,omitempty"`
	State   string `json:"state"`
	Detail  string `json:"detail,omitempty"`
}

// Status combines the embedded version of a skill with the copies found on
// disk.
type Status struct {
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	Installations []Installation `json:"installations"`
	// Files reports the state of every shipped template beside SKILL.md.
	// Skills without templates omit it.
	Files []FileStatus `json:"files,omitempty"`
}

// FileStatus is the state of one shipped template installed beside SKILL.md.
// State is current, stale, modified, missing, or unreadable.
type FileStatus struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// List reports every embedded skill and the copies found on disk. States are
// derived from the sha256 of the installed file, not its version line, so a
// hand-edited copy that keeps the version line still reports as modified.
// List never writes, so it works when the home directory is read-only or
// absent.
func List(opts ListOptions) ([]Status, error) {
	var l layout
	resolved := true
	if opts.Local {
		var err error
		l, err = newLayout(Options{Local: true, ProjectDir: opts.ProjectDir})
		if err != nil {
			return nil, err
		}
	} else if opts.Home != "" {
		l = layout{home: opts.Home}
	} else if home, err := os.UserHomeDir(); err == nil {
		l = layout{home: home}
	} else {
		resolved = false
	}
	manifest := loadManifest(l.root())
	out := make([]Status, 0, len(names))
	for _, name := range names {
		content, err := Embedded(name, opts.Version)
		if err != nil {
			return nil, err
		}
		status := Status{Name: name, Version: InstalledVersion(content), Installations: []Installation{}}
		if !resolved {
			out = append(out, status)
			continue
		}
		record, managed := manifest.Skills[name]
		canonical := canonicalInstallation(l, name, content, record, managed)
		status.Installations = append(status.Installations, canonical)
		for _, harness := range linkedHarnesses {
			status.Installations = append(status.Installations, linkInstallation(l, harness, name, canonical.Version))
		}
		status.Installations = append(status.Installations, codexInstallation(l, name))
		templates, err := templateStatuses(l, name, record, managed, opts.Version)
		if err != nil {
			return nil, err
		}
		status.Files = templates
		out = append(out, status)
	}
	return out, nil
}

// canonicalInstallation derives the state of the canonical copy from the
// sha256 of its content: current when it matches the rendered skill, invalid
// when its front matter does not parse, stale when it is byte-identical to
// what the manifest recorded but older than the running binary, and modified
// in every other case.
func canonicalInstallation(l layout, name string, content []byte, record ManifestSkill, managed bool) Installation {
	path := filepath.Join(l.root(), name, "SKILL.md")
	inst := Installation{Target: "agents", Path: path, State: "missing"}
	body, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return inst
	case err != nil:
		inst.Present, inst.State, inst.Detail = true, "unreadable", err.Error()
		return inst
	}
	inst.Present = true
	inst.Version = InstalledVersion(body)
	recorded, tracked := record.File("SKILL.md")
	switch {
	case bytes.Equal(body, content):
		inst.State = "current"
	case frontMatterBroken(body):
		inst.State = "invalid"
		inst.Detail = "front matter does not parse; run `agentbox skills install --force`"
	case managed && tracked && hashHex(body) == recorded.SHA256:
		inst.State = "stale"
		inst.Detail = "run `agentbox skills update`"
	default:
		inst.State = "modified"
		inst.Detail = "differs from the managed copy; run `agentbox skills update --force`"
	}
	return inst
}

// templateStatuses derives the state of every shipped template from the
// sha256 of its installed copy, mirroring the canonical SKILL.md states.
func templateStatuses(l layout, name string, record ManifestSkill, managed bool, version string) ([]FileStatus, error) {
	var out []FileStatus
	for _, spec := range Files(name) {
		if spec.Name == "SKILL.md" {
			continue
		}
		content, err := EmbeddedFile(name, spec.Name, version)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(l.root(), name, filepath.FromSlash(spec.Name))
		status := FileStatus{Name: spec.Name, Path: path, State: "missing"}
		body, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			out = append(out, status)
			continue
		}
		if err != nil {
			status.State, status.Detail = "unreadable", err.Error()
			out = append(out, status)
			continue
		}
		recorded, tracked := record.File(spec.Name)
		switch {
		case bytes.Equal(body, content):
			status.State = "current"
		case managed && tracked && hashHex(body) == recorded.SHA256:
			status.State, status.Detail = "stale", "run `agentbox skills update`"
		default:
			status.State, status.Detail = "modified", "differs from the managed copy; run `agentbox skills update --force`"
		}
		out = append(out, status)
	}
	return out, nil
}

// linkInstallation reports whether a harness link points at the canonical
// copy. A link is current only when the skill file is reachable through it;
// a symlink that resolves nowhere is broken-link, and a real directory,
// file, or foreign symlink at the path is foreign.
func linkInstallation(l layout, harness, name, canonicalVersion string) Installation {
	linkPath := filepath.Join(l.harnessSkillsDir(harness), name)
	inst := Installation{Target: harness, Path: linkPath, State: "missing"}
	info, err := os.Lstat(linkPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return inst
	case err != nil:
		inst.Present, inst.State, inst.Detail = true, "unreadable", err.Error()
		return inst
	}
	inst.Present = true
	if info.Mode()&os.ModeSymlink == 0 {
		inst.State = "foreign"
		inst.Detail = "not a symlink to the canonical copy"
		return inst
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		inst.State, inst.Detail = "unreadable", err.Error()
		return inst
	}
	if linkTargets(linkPath, target, filepath.Join(l.root(), name)) {
		if _, err := os.Stat(filepath.Join(linkPath, "SKILL.md")); err != nil {
			inst.State = "broken-link"
			inst.Detail = "no skill behind the link; run `agentbox skills install`"
			return inst
		}
		inst.State = "current"
		inst.Version = canonicalVersion
		return inst
	}
	if _, err := os.Stat(linkPath); err != nil {
		inst.State = "broken-link"
		inst.Detail = "dangling link; run `agentbox skills install`"
		return inst
	}
	inst.State = "foreign"
	inst.Detail = "symlink points elsewhere"
	return inst
}

// codexInstallation is report-only: Codex reads the canonical
// ~/.agents/skills copy directly, so this row surfaces leftover copies in
// ~/.codex/skills from releases that wrote there.
func codexInstallation(l layout, name string) Installation {
	path := filepath.Join(l.harnessSkillsDir("codex"), name, "SKILL.md")
	inst := Installation{
		Target: "codex",
		Path:   path,
		State:  "missing",
		Detail: "codex reads the canonical .agents/skills copy directly",
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if _, statErr := os.Stat(filepath.Dir(path)); statErr == nil {
			// Something that is not a readable skill copy sits where the
			// legacy install would have put one.
			inst.Present, inst.State = true, "legacy"
			inst.Detail = "leftover content from an older AgentBox; run `agentbox skills update`"
		}
		return inst
	}
	if err != nil {
		inst.Present, inst.State, inst.Detail = true, "legacy", fmt.Sprintf("leftover copy from an older AgentBox; run `agentbox skills update` (%s)", err)
		return inst
	}
	inst.Present, inst.State = true, "legacy"
	inst.Version = InstalledVersion(body)
	inst.Detail = "leftover copy from an older AgentBox; run `agentbox skills update`"
	return inst
}

// frontMatterBroken reports whether a rendered skill file has front matter
// that strict consumers reject.
func frontMatterBroken(body []byte) bool {
	_, err := ParseFrontMatter(body)
	return err != nil
}
