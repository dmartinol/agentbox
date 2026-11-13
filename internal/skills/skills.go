// Package skills embeds AgentBox's own usage skills and installs them into the
// skill directories of host coding agents. The content is version-matched to
// the running binary so an agent can tell whether its instructions are stale.
package skills

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// VersionPlaceholder is replaced with the running binary version at render
// time. The embedded files keep the placeholder so installed copies can be
// compared against the binary that wrote them.
const VersionPlaceholder = "{{VERSION}}"

//go:embed agentbox-cli/SKILL.md agentbox-api/SKILL.md
var embedded embed.FS

var names = []string{"agentbox-cli", "agentbox-api"}

var targets = []string{"agents", "codex", "claude", "opencode"}

// Names returns the embedded skill names in a stable order.
func Names() []string {
	out := make([]string, len(names))
	copy(out, names)
	return out
}

// Targets returns the supported built-in install targets in a stable order.
func Targets() []string {
	out := make([]string, len(targets))
	copy(out, targets)
	return out
}

// ValidName reports whether name identifies an embedded skill.
func ValidName(name string) bool {
	for _, candidate := range names {
		if candidate == name {
			return true
		}
	}
	return false
}

// Embedded returns the rendered content of one embedded skill.
func Embedded(name, version string) ([]byte, error) {
	if !ValidName(name) {
		return nil, fmt.Errorf("unknown skill %q (available: %s)", name, strings.Join(names, ", "))
	}
	raw, err := embedded.ReadFile(name + "/SKILL.md")
	if err != nil {
		return nil, err
	}
	return render(raw, version), nil
}

func render(raw []byte, version string) []byte {
	if version == "" {
		version = "dev"
	}
	return bytes.ReplaceAll(raw, []byte(VersionPlaceholder), []byte(version))
}

// InstalledVersion extracts the version recorded in a rendered SKILL.md. It
// returns an empty string when the file has no front matter version line.
func InstalledVersion(content []byte) string {
	lines := strings.Split(strings.TrimPrefix(string(content), "\ufeff"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		if strings.HasPrefix(line, "version:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "version:")), `"'`)
		}
	}
	return ""
}

// Options controls Install.
type Options struct {
	// Target is agents, codex, claude, opencode, or all. Empty means all.
	Target string
	// Dir overrides the target root entirely. It cannot be combined with
	// Target all because both skills would land twice in the same directory.
	Dir string
	// Force overwrites files whose content differs from the embedded skill.
	Force bool
	// DryRun reports the planned writes without touching the filesystem.
	DryRun bool
	// Version is written into the skill front matter.
	Version string
	// Home overrides the user home directory used to resolve targets.
	Home string
}

// Result describes one planned or completed write.
type Result struct {
	Skill  string `json:"skill"`
	Target string `json:"target"`
	Path   string `json:"path"`
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
}

// Installation describes one installed copy of a skill.
type Installation struct {
	Target  string `json:"target"`
	Path    string `json:"path"`
	Present bool   `json:"present"`
	Version string `json:"version,omitempty"`
	State   string `json:"state"`
	Error   string `json:"error,omitempty"`
}

// Status combines the embedded version of a skill with the copies found on
// disk. State is current, stale, missing, or unreadable.
type Status struct {
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	Installations []Installation `json:"installations"`
}

type destination struct {
	target string
	root   string
	custom bool
}

// Install copies the embedded skills to the selected targets. A missing
// built-in parent directory is reported and skipped instead of created; an
// explicit Dir is created. Differing files are preserved unless Force is set,
// and that refusal is returned as an error after every other write is done.
func Install(opts Options) ([]Result, error) {
	target := opts.Target
	dests, err := destinations(target, opts.Dir, opts.Home)
	if err != nil {
		return nil, err
	}
	var results []Result
	var failures []error
	for _, dest := range dests {
		for _, name := range names {
			content, err := Embedded(name, opts.Version)
			if err != nil {
				failures = append(failures, err)
				continue
			}
			path := filepath.Join(dest.root, name, "SKILL.md")
			result, err := installOne(dest, name, path, content, opts)
			results = append(results, result)
			if err != nil {
				failures = append(failures, err)
			}
		}
	}
	return results, errors.Join(failures...)
}

func destinations(target, dir, home string) ([]destination, error) {
	if target != "" && target != "agents" && target != "codex" && target != "claude" && target != "opencode" && target != "all" {
		return nil, fmt.Errorf("target must be agents, codex, claude, opencode, or all")
	}
	if dir != "" {
		if target == "all" {
			return nil, fmt.Errorf("--dir writes both skills once; do not combine it with --target all")
		}
		label := target
		if label == "" {
			label = "dir"
		}
		return []destination{{target: label, root: dir, custom: true}}, nil
	}
	if target == "" {
		target = "all"
	}
	if home == "" {
		resolved, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home directory (use --dir): %w", err)
		}
		home = resolved
	}
	selected := targets
	if target != "all" {
		selected = []string{target}
	}
	dests := make([]destination, 0, len(selected))
	for _, name := range selected {
		root, err := targetRoot(name, home)
		if err != nil {
			return nil, err
		}
		dests = append(dests, destination{target: name, root: root})
	}
	return dests, nil
}

func targetRoot(target, home string) (string, error) {
	switch target {
	case "agents":
		return filepath.Join(home, ".agents", "skills"), nil
	case "codex":
		return filepath.Join(home, ".codex", "skills"), nil
	case "claude":
		return filepath.Join(home, ".claude", "skills"), nil
	case "opencode":
		return filepath.Join(home, ".config", "opencode", "skills"), nil
	}
	return "", fmt.Errorf("unknown target %q", target)
}

func installOne(dest destination, name, path string, content []byte, opts Options) (Result, error) {
	result := Result{Skill: name, Target: dest.target, Path: path}
	if !dest.custom {
		if info, err := os.Stat(filepath.Dir(dest.root)); err != nil || !info.IsDir() {
			result.Action = "skipped"
			result.Detail = fmt.Sprintf("%s does not exist", filepath.Dir(dest.root))
			return result, nil
		}
	}
	existing, err := os.ReadFile(path)
	switch {
	case err == nil && bytes.Equal(existing, content):
		result.Action = "unchanged"
		return result, nil
	case err == nil:
		if !opts.Force {
			result.Action = "skipped"
			result.Detail = "existing file differs from the embedded skill; use --force to overwrite"
			if opts.DryRun {
				return result, nil
			}
			return result, fmt.Errorf("refusing to overwrite %s; use --force", path)
		}
		if opts.DryRun {
			result.Action = "would-overwrite"
			return result, nil
		}
		result.Detail = "replaced existing file"
	case errors.Is(err, fs.ErrNotExist):
		if opts.DryRun {
			result.Action = "would-install"
			return result, nil
		}
	default:
		result.Action = "skipped"
		result.Detail = err.Error()
		return result, err
	}
	if !opts.DryRun {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			result.Action = "skipped"
			result.Detail = err.Error()
			return result, err
		}
		if err := os.WriteFile(path, content, 0644); err != nil {
			result.Action = "skipped"
			result.Detail = err.Error()
			return result, err
		}
	}
	result.Action = "installed"
	return result, nil
}

// ListOptions controls List.
type ListOptions struct {
	// Version is the running binary version to compare against.
	Version string
	// Home overrides the user home directory used to resolve targets.
	Home string
}

// List reports every embedded skill and the copies found on disk. It never
// writes, so it works when the home directory is read-only or absent.
func List(opts ListOptions) ([]Status, error) {
	home := opts.Home
	if home == "" {
		if resolved, err := os.UserHomeDir(); err == nil {
			home = resolved
		}
	}
	out := make([]Status, 0, len(names))
	for _, name := range names {
		content, err := Embedded(name, opts.Version)
		if err != nil {
			return nil, err
		}
		version := InstalledVersion(content)
		status := Status{Name: name, Version: version, Installations: []Installation{}}
		for _, target := range targets {
			root, err := targetRoot(target, home)
			if err != nil || home == "" {
				continue
			}
			path := filepath.Join(root, name, "SKILL.md")
			installed, err := os.ReadFile(path)
			installation := Installation{Target: target, Path: path, State: "missing"}
			switch {
			case errors.Is(err, fs.ErrNotExist):
			case err != nil:
				installation.State = "unreadable"
				installation.Error = err.Error()
			default:
				installation.Present = true
				installation.Version = InstalledVersion(installed)
				if installation.Version == version {
					installation.State = "current"
				} else {
					installation.State = "stale"
				}
			}
			status.Installations = append(status.Installations, installation)
		}
		out = append(out, status)
	}
	return out, nil
}
