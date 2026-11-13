package skills

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Options controls Install, Update, and Remove.
type Options struct {
	// Agents selects harnesses explicitly: claude, codex, or opencode.
	// Claude and OpenCode receive a link to the canonical copy; Codex is
	// reporting-only (see the Codex duplicate rule in skills.go).
	Agents []string
	// Target is the deprecated --target alias. It maps agents, codex,
	// claude, opencode, and all onto the same destinations as Agents.
	Target string
	// All selects every harness regardless of directory detection.
	All bool
	// Local puts the canonical root at ./.agents/skills relative to
	// ProjectDir (the working directory when empty) and links into
	// project-scoped harness directories.
	Local bool
	// ProjectDir overrides the --local base directory. It is primarily a
	// test hook; the CLI resolves the working directory instead.
	ProjectDir string
	// Dir overrides the destination root entirely. Every shipped skill file
	// is written as a plain copy beneath it: no links, no manifest, no
	// lifecycle.
	Dir string
	// Force replaces modified copies and foreign destinations, moving the
	// previous content into a timestamped backup first.
	Force bool
	// DryRun reports the planned actions without touching the filesystem.
	DryRun bool
	// Version is written into the skill front matter.
	Version string
	// Home overrides the user home directory used to resolve destinations.
	Home string
}

// Result describes one planned or completed action. Actions are stable
// strings: installed, updated, unchanged, linked, migrated, removed, and
// skipped, with would- prefixed counterparts in dry runs.
type Result struct {
	Skill string `json:"skill"`
	// File is the installed path of the acted-on file relative to the skill
	// directory: SKILL.md, or a template such as issue.md. It is empty when
	// the result describes a link rather than a file.
	File   string `json:"file,omitempty"`
	Target string `json:"target"`
	Path   string `json:"path"`
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
}

// layout resolves every filesystem location of one install scope: the user
// home directory for global installs or the project directory for --local.
type layout struct {
	home  string
	local bool
}

func newLayout(opts Options) (layout, error) {
	if opts.Local {
		base := opts.ProjectDir
		if base == "" {
			wd, err := os.Getwd()
			if err != nil {
				return layout{}, fmt.Errorf("resolve working directory: %w", err)
			}
			base = wd
		}
		abs, err := filepath.Abs(base)
		if err != nil {
			return layout{}, fmt.Errorf("resolve project directory: %w", err)
		}
		return layout{home: abs, local: true}, nil
	}
	home := opts.Home
	if home == "" {
		resolved, err := os.UserHomeDir()
		if err != nil {
			return layout{}, fmt.Errorf("resolve home directory (use --dir): %w", err)
		}
		home = resolved
	}
	return layout{home: home}, nil
}

// root is the canonical skills root every harness ultimately reads.
func (l layout) root() string { return filepath.Join(l.home, ".agents", "skills") }

// backups is where --force preserves replaced or removed content.
func (l layout) backups() string {
	return filepath.Join(l.home, ".agents", ".agentbox-skill-backups")
}

// harnessParent is the directory whose presence marks a harness as detected.
func (l layout) harnessParent(harness string) string {
	switch harness {
	case "claude":
		return filepath.Join(l.home, ".claude")
	case "codex":
		return filepath.Join(l.home, ".codex")
	case "opencode":
		if l.local {
			return filepath.Join(l.home, ".opencode")
		}
		return filepath.Join(l.home, ".config", "opencode")
	}
	return ""
}

// harnessSkillsDir is the skills directory a harness reads. The Codex value
// is the legacy location used for reporting and migration only; AgentBox
// never writes there.
func (l layout) harnessSkillsDir(harness string) string {
	parent := l.harnessParent(harness)
	if parent == "" {
		return ""
	}
	return filepath.Join(parent, "skills")
}

// detectHarnesses returns the harnesses whose parent directories exist. Codex
// detection affects reporting and migration only, never link creation.
func (l layout) detectHarnesses() []string {
	var out []string
	for _, harness := range harnesses {
		if info, err := os.Stat(l.harnessParent(harness)); err == nil && info.IsDir() {
			out = append(out, harness)
		}
	}
	return out
}

// harnessForPath labels a recorded link with the harness it belongs to.
func (l layout) harnessForPath(path string) string {
	for _, harness := range harnesses {
		dir := l.harnessSkillsDir(harness)
		if dir == "" {
			continue
		}
		if path == dir || strings.HasPrefix(path, dir+string(filepath.Separator)) {
			return harness
		}
	}
	return "link"
}

// backupSet lazily creates one timestamped backup directory per command so
// every forced replacement of a single invocation lands together.
type backupSet struct {
	root  string
	stamp string
}

func newBackupSet(l layout) *backupSet { return &backupSet{root: l.backups()} }

func (b *backupSet) move(path, label string) error {
	if b.stamp == "" {
		b.stamp = time.Now().UTC().Format("20060102-150405")
	}
	dst := filepath.Join(b.root, b.stamp, label)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.Rename(path, dst)
}

// Install writes one canonical copy of every embedded skill, links it for
// the selected harnesses that do not read the canonical root, records
// everything in the managed manifest, and migrates byte-identical legacy
// copies out of ~/.codex/skills. Modified copies and foreign destinations
// are refused unless Force is set; forced replacements are backed up first.
func Install(opts Options) ([]Result, error) {
	return runLifecycle(opts, modeInstall)
}

// Update refreshes managed copies that AgentBox installed previously
// (current or stale) and recreates their links. Modified copies are skipped
// unless Force is set, and missing copies are reported instead of created;
// run Install for those.
func Update(opts Options) ([]Result, error) {
	return runLifecycle(opts, modeUpdate)
}

type lifecycleMode int

const (
	modeInstall lifecycleMode = iota
	modeUpdate
)

func runLifecycle(opts Options, mode lifecycleMode) ([]Result, error) {
	if opts.Dir != "" {
		if mode == modeUpdate {
			return nil, fmt.Errorf("--dir is only supported by install")
		}
		if opts.Local {
			return nil, fmt.Errorf("--dir cannot be combined with --local")
		}
		if len(opts.Agents) > 0 || opts.Target != "" || opts.All {
			return nil, fmt.Errorf("--dir cannot be combined with --agent, --target, or --all")
		}
		return installDir(opts)
	}
	if mode == modeUpdate && opts.Target != "" {
		return nil, fmt.Errorf("--target is not supported by update; use --agent")
	}
	l, err := newLayout(opts)
	if err != nil {
		return nil, err
	}
	agents, err := resolveAgents(opts, l)
	if err != nil {
		return nil, err
	}
	manifestExisted := manifestExists(l.root())
	manifest := loadManifest(l.root())
	bk := newBackupSet(l)
	var results []Result
	var failures []error
	for _, name := range names {
		files, err := renderFiles(name, opts.Version)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		skillContent := files[0].content
		canonicalDir := filepath.Join(l.root(), name)
		written, ours, err := ensureCanonical(l, manifest, name, files, opts, mode, bk)
		results = append(results, written...)
		if err != nil {
			failures = append(failures, err)
		}
		if !ours {
			// The canonical copy is missing or not ours; linking harnesses
			// to it would expose content AgentBox cannot vouch for.
			continue
		}
		for _, harness := range linkedHarnesses {
			if !containsString(agents, harness) {
				continue
			}
			result, _, err := ensureLink(l, manifest, harness, name, canonicalDir, skillContent, opts, bk)
			results = append(results, result)
			if err != nil {
				failures = append(failures, err)
			}
		}
		if containsString(agents, "codex") {
			result, reported, err := migrateLegacyCodex(l, name, skillContent, opts, bk)
			if reported {
				results = append(results, result)
			}
			if err != nil {
				failures = append(failures, err)
			}
		}
	}
	if !opts.DryRun && (manifestExisted || len(manifest.Skills) > 0) {
		if err := saveManifest(l.root(), manifest); err != nil {
			failures = append(failures, fmt.Errorf("write manifest: %w", err))
		}
	}
	return results, errors.Join(failures...)
}

// resolveAgents determines which harnesses a command operates on: --all,
// then an explicit --agent list, then the deprecated --target alias, and
// finally the harnesses detected on this machine.
func resolveAgents(opts Options, l layout) ([]string, error) {
	if opts.All {
		if len(opts.Agents) > 0 || opts.Target != "" {
			return nil, fmt.Errorf("--all cannot be combined with --agent or --target")
		}
		return append([]string(nil), harnesses...), nil
	}
	if len(opts.Agents) > 0 && opts.Target != "" {
		return nil, fmt.Errorf("use --agent or the deprecated --target, not both")
	}
	if len(opts.Agents) > 0 {
		return normalizeAgents(opts.Agents)
	}
	if opts.Target != "" {
		return agentsFromTarget(opts.Target)
	}
	return l.detectHarnesses(), nil
}

// normalizeAgents validates a harness selection and returns it in a stable
// order without duplicates.
func normalizeAgents(in []string) ([]string, error) {
	seen := make(map[string]bool, len(in))
	for _, agent := range in {
		if !validHarness(agent) {
			return nil, fmt.Errorf("agent %q must be claude, codex, or opencode", agent)
		}
		seen[agent] = true
	}
	var out []string
	for _, harness := range harnesses {
		if seen[harness] {
			out = append(out, harness)
		}
	}
	return out, nil
}

// agentsFromTarget maps the deprecated --target values onto harness
// selections. agents selects no harness: only the canonical copy is written,
// which is what reading ~/.agents/skills already provides.
func agentsFromTarget(target string) ([]string, error) {
	switch target {
	case "":
		return nil, nil
	case "agents":
		return nil, nil
	case "codex":
		return []string{"codex"}, nil
	case "claude":
		return []string{"claude"}, nil
	case "opencode":
		return []string{"opencode"}, nil
	case "all":
		return append([]string(nil), harnesses...), nil
	}
	return nil, fmt.Errorf("target must be agents, codex, claude, opencode, or all")
}

// renderedFile is one embedded skill file with its version substituted.
type renderedFile struct {
	spec    SkillFile
	content []byte
}

// renderFiles renders every file of one skill in inventory order.
func renderFiles(name, version string) ([]renderedFile, error) {
	files := Files(name)
	out := make([]renderedFile, 0, len(files))
	for _, file := range files {
		content, err := EmbeddedFile(name, file.Name, version)
		if err != nil {
			return nil, err
		}
		out = append(out, renderedFile{spec: file, content: content})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("skill %q has no embedded files", name)
	}
	return out, nil
}

// ensureCanonical writes or refreshes every managed file of one skill and
// reports whether the canonical SKILL.md is provably AgentBox's. Per-file
// results let install, update, and remove report templates independently of
// SKILL.md. Stale files that match the manifest are refreshed in place;
// anything else that differs is refused unless Force is set, and forced
// replacements are backed up first.
func ensureCanonical(l layout, manifest *Manifest, name string, files []renderedFile, opts Options, mode lifecycleMode, bk *backupSet) ([]Result, bool, error) {
	dir := filepath.Join(l.root(), name)
	skillPath := filepath.Join(dir, "SKILL.md")
	record, hasRecord := manifest.Skills[name]
	result := func(action, detail string) Result {
		return Result{Skill: name, File: "SKILL.md", Target: "agents", Path: skillPath, Action: action, Detail: detail}
	}

	// The canonical skill directory must be a real directory AgentBox owns.
	if info, err := os.Lstat(dir); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
		kind := "file"
		if info.Mode()&os.ModeSymlink != 0 {
			kind = "symlink"
		}
		if !opts.Force {
			r := result("skipped", fmt.Sprintf("canonical path is a %s; use --force to replace it", kind))
			if opts.DryRun {
				return []Result{r}, false, nil
			}
			return []Result{r}, false, fmt.Errorf("refusing to replace %s %s; use --force", kind, dir)
		}
		if opts.DryRun {
			return []Result{result("would-install", fmt.Sprintf("replace the %s at the canonical path (backed up first)", kind))}, true, nil
		}
		if err := bk.move(dir, filepath.Join("canonical", name)); err != nil {
			r := result("skipped", err.Error())
			return []Result{r}, false, err
		}
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		r := result("skipped", err.Error())
		return []Result{r}, false, err
	}

	// Update never creates a missing skill; install does. Report the missing
	// SKILL.md once rather than a row per template.
	if mode == modeUpdate {
		if _, err := os.Stat(skillPath); errors.Is(err, fs.ErrNotExist) {
			return []Result{result("skipped", "not installed; run `agentbox skills install`")}, false, nil
		}
	}

	next := ManifestSkill{Version: record.Version, Path: skillPath, Links: nonNil(record.Links)}
	skillOwned := false
	recordsFile := false
	var results []Result
	var failures []error
	for i, file := range files {
		// A template is only added when SKILL.md itself is provably ours, so
		// install never litters a directory it refused to manage.
		allowCreate := i == 0 || skillOwned
		res, owned, err := ensureCanonicalFile(dir, name, file.spec, file.content, record, hasRecord, allowCreate, opts, bk)
		results = append(results, res)
		if err != nil {
			failures = append(failures, err)
		}
		if i == 0 {
			skillOwned = owned
		}
		if owned {
			next.setFile(ManifestEntry{Name: file.spec.Name, SHA256: hashHex(file.content), Path: res.Path})
			recordsFile = true
			continue
		}
		if existing, ok := record.File(file.spec.Name); ok {
			next.setFile(existing)
			recordsFile = true
		}
	}
	// Files recorded by an older install that this binary no longer ships stay
	// managed so remove can still clean them up.
	shipped := make(map[string]bool, len(files))
	for _, file := range files {
		shipped[file.spec.Name] = true
	}
	for _, file := range record.Files {
		if !shipped[file.Name] {
			next.setFile(file)
			recordsFile = true
		}
	}
	if skillOwned {
		next.Version = effectiveVersion(opts.Version)
	}
	if hasRecord || recordsFile {
		manifest.Skills[name] = next
	}
	return results, skillOwned, errors.Join(failures...)
}

// ensureCanonicalFile writes one file of a skill. owned reports whether the
// on-disk file now matches the embedded content.
func ensureCanonicalFile(dir, skill string, spec SkillFile, content []byte, record ManifestSkill, hasRecord, allowCreate bool, opts Options, bk *backupSet) (Result, bool, error) {
	path := filepath.Join(dir, filepath.FromSlash(spec.Name))
	res := Result{Skill: skill, File: spec.Name, Target: "agents", Path: path}
	tracked, trackedFile := ManifestEntry{}, false
	if hasRecord {
		tracked, trackedFile = record.File(spec.Name)
	}
	existing, err := os.ReadFile(path)
	switch {
	case err == nil && bytes.Equal(existing, content):
		res.Action = "unchanged"
		return res, true, nil
	case err == nil:
		if trackedFile && hashHex(existing) == tracked.SHA256 {
			// Byte-identical to what AgentBox installed before: stale.
			if opts.DryRun {
				res.Action, res.Detail = "would-update", "refresh from version "+record.Version
				return res, true, nil
			}
			if err := writeFile(path, content); err != nil {
				res.Action, res.Detail = "skipped", err.Error()
				return res, false, err
			}
			res.Action, res.Detail = "updated", "refreshed from version "+record.Version
			return res, true, nil
		}
		// Modified by hand or installed by an older release without a
		// manifest: preserve it unless --force is set.
		if !opts.Force {
			res.Action = "skipped"
			res.Detail = "existing file differs from the embedded skill; use --force to overwrite"
			if opts.DryRun {
				return res, false, nil
			}
			return res, false, fmt.Errorf("refusing to overwrite %s; use --force", path)
		}
		if opts.DryRun {
			res.Action, res.Detail = "would-update", "replace the modified copy (backed up first)"
			return res, true, nil
		}
		if err := bk.move(path, filepath.Join("canonical", skill, filepath.FromSlash(spec.Name))); err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		if err := writeFile(path, content); err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		res.Action, res.Detail = "updated", "replaced the modified copy (backed up)"
		return res, true, nil
	case errors.Is(err, fs.ErrNotExist):
		if !allowCreate {
			res.Action, res.Detail = "skipped", "SKILL.md differs; use --force to install this file"
			if spec.Name != "SKILL.md" {
				res.Detail = "SKILL.md is not managed; use --force to install this file"
			}
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
	default:
		res.Action, res.Detail = "skipped", err.Error()
		return res, false, err
	}
}

// ensureLink points harness's skill directory at the canonical copy with a
// relative symlink. A byte-identical legacy copy at the link path is
// replaced; anything else that differs is a foreign destination and is
// skipped unless Force is set. Dangling symlinks are always repaired.
func ensureLink(l layout, manifest *Manifest, harness, name, canonicalDir string, content []byte, opts Options, bk *backupSet) (Result, bool, error) {
	linkPath := filepath.Join(l.harnessSkillsDir(harness), name)
	target, err := filepath.Rel(filepath.Dir(linkPath), canonicalDir)
	if err != nil {
		target = canonicalDir
	}
	res := Result{Skill: name, Target: harness, Path: linkPath}
	fail := func(detail string, args ...any) (Result, bool, error) {
		res.Action = "skipped"
		res.Detail = fmt.Sprintf(detail, args...)
		if opts.DryRun {
			return res, false, nil
		}
		return res, false, fmt.Errorf("refusing to replace %s; use --force", linkPath)
	}
	info, err := os.Lstat(linkPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if opts.DryRun {
			res.Action = "would-link"
			return res, true, nil
		}
		if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		if err := os.Symlink(target, linkPath); err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		res.Action = "linked"
		addManifestLink(manifest, name, linkPath)
		return res, true, nil
	case err != nil:
		res.Action, res.Detail = "skipped", err.Error()
		return res, false, err
	case info.Mode()&os.ModeSymlink != 0:
		current, err := os.Readlink(linkPath)
		if err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		if linkTargets(linkPath, current, canonicalDir) {
			res.Action = "unchanged"
			addManifestLink(manifest, name, linkPath)
			return res, true, nil
		}
		if _, err := os.Stat(linkPath); err != nil && errors.Is(err, fs.ErrNotExist) {
			// A dangling link serves nothing; repair it without --force.
			if opts.DryRun {
				res.Action, res.Detail = "would-link", "replace the dangling link"
				return res, true, nil
			}
			if err := replaceLink(linkPath, target); err != nil {
				res.Action, res.Detail = "skipped", err.Error()
				return res, false, err
			}
			res.Action, res.Detail = "linked", "replaced the dangling link"
			addManifestLink(manifest, name, linkPath)
			return res, true, nil
		}
		if !opts.Force {
			return fail("symlink points elsewhere; use --force to replace it")
		}
		if opts.DryRun {
			res.Action, res.Detail = "would-link", "replace the foreign symlink"
			return res, true, nil
		}
		if err := replaceLink(linkPath, target); err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		res.Action, res.Detail = "linked", "replaced the foreign symlink"
		addManifestLink(manifest, name, linkPath)
		return res, true, nil
	case info.IsDir():
		body, readErr := os.ReadFile(filepath.Join(linkPath, "SKILL.md"))
		if readErr == nil && singleFileDir(linkPath) && bytes.Equal(body, content) {
			// A byte-identical copy written by an older AgentBox release.
			if opts.DryRun {
				res.Action, res.Detail = "would-migrate", "replace the legacy copy with a link"
				return res, true, nil
			}
			if err := os.RemoveAll(linkPath); err != nil {
				res.Action, res.Detail = "skipped", err.Error()
				return res, false, err
			}
			if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
				res.Action, res.Detail = "skipped", err.Error()
				return res, false, err
			}
			if err := os.Symlink(target, linkPath); err != nil {
				res.Action, res.Detail = "skipped", err.Error()
				return res, false, err
			}
			res.Action, res.Detail = "migrated", "replaced the legacy copy with a link"
			addManifestLink(manifest, name, linkPath)
			return res, true, nil
		}
		if !opts.Force {
			return fail("existing directory differs from the embedded skill; use --force to replace it")
		}
		if opts.DryRun {
			res.Action, res.Detail = "would-link", "replace the existing directory (backed up first)"
			return res, true, nil
		}
		if err := bk.move(linkPath, filepath.Join("links", harness, name)); err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		if err := os.Symlink(target, linkPath); err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		res.Action, res.Detail = "linked", "replaced the existing directory (backed up)"
		addManifestLink(manifest, name, linkPath)
		return res, true, nil
	default:
		if !opts.Force {
			return fail("existing file differs from the embedded skill; use --force to replace it")
		}
		if opts.DryRun {
			res.Action, res.Detail = "would-link", "replace the existing file (backed up first)"
			return res, true, nil
		}
		if err := bk.move(linkPath, filepath.Join("links", harness, name)); err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		if err := os.Symlink(target, linkPath); err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, false, err
		}
		res.Action, res.Detail = "linked", "replaced the existing file (backed up)"
		addManifestLink(manifest, name, linkPath)
		return res, true, nil
	}
}

// migrateLegacyCodex removes copies that older AgentBox releases wrote into
// ~/.codex/skills. Codex also scans the canonical ~/.agents/skills root, so
// these copies register every skill twice. Only directories that contain
// nothing but a byte-identical SKILL.md are removed; anything else is
// reported and requires Force (with a backup).
func migrateLegacyCodex(l layout, name string, content []byte, opts Options, bk *backupSet) (Result, bool, error) {
	dir := filepath.Join(l.harnessSkillsDir("codex"), name)
	res := Result{Skill: name, Target: "codex", Path: filepath.Join(dir, "SKILL.md")}
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return res, false, nil
	}
	if err != nil {
		res.Action, res.Detail = "skipped", err.Error()
		return res, true, err
	}
	body, readErr := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if readErr == nil && info.IsDir() && singleFileDir(dir) && bytes.Equal(body, content) {
		if opts.DryRun {
			res.Action, res.Detail = "would-migrate", "remove the byte-identical legacy copy"
			return res, true, nil
		}
		if err := os.RemoveAll(dir); err != nil {
			res.Action, res.Detail = "skipped", err.Error()
			return res, true, err
		}
		res.Action, res.Detail = "migrated", "removed the byte-identical legacy copy"
		return res, true, nil
	}
	if !opts.Force {
		res.Action = "skipped"
		res.Detail = "legacy copy differs from the embedded skill; use --force to remove it"
		if opts.DryRun {
			return res, true, nil
		}
		return res, true, fmt.Errorf("legacy copy %s differs from the embedded skill; use --force to remove it", dir)
	}
	if opts.DryRun {
		res.Action, res.Detail = "would-migrate", "remove the differing legacy copy (backed up first)"
		return res, true, nil
	}
	if err := bk.move(dir, filepath.Join("codex", name)); err != nil {
		res.Action, res.Detail = "skipped", err.Error()
		return res, true, err
	}
	res.Action, res.Detail = "migrated", "removed the differing legacy copy (backed up)"
	return res, true, nil
}

// Remove deletes only the copies and links recorded in the managed manifest.
// A modified canonical copy is refused unless Force is set; forced removals
// are backed up first, and foreign files or directories at recorded paths are
// always left alone. With Agents set, only the links of those harnesses are
// removed and the canonical copies stay.
func Remove(opts Options) ([]Result, error) {
	if opts.Dir != "" {
		return nil, fmt.Errorf("--dir is only supported by install")
	}
	if opts.Target != "" {
		return nil, fmt.Errorf("--target is not supported by remove; use --agent")
	}
	l, err := newLayout(opts)
	if err != nil {
		return nil, err
	}
	filter, full, err := resolveRemoveFilter(opts)
	if err != nil {
		return nil, err
	}
	manifest := loadManifest(l.root())
	if len(manifest.Skills) == 0 {
		return nil, fmt.Errorf("no managed skills recorded in %s; nothing to remove", manifestPath(l.root()))
	}
	bk := newBackupSet(l)
	var results []Result
	var failures []error
	for _, name := range names {
		record, ok := manifest.Skills[name]
		if !ok {
			continue
		}
		if full {
			for _, link := range sortedCopy(record.Links) {
				res, dropped, err := removeLink(l, name, link, opts)
				results = append(results, res)
				if err != nil {
					failures = append(failures, err)
				}
				if dropped {
					dropManifestLink(manifest, name, link)
				}
			}
			removedResults, removed, err := removeCanonical(l, manifest, name, opts, bk)
			results = append(results, removedResults...)
			if err != nil {
				failures = append(failures, err)
			}
			if removed {
				delete(manifest.Skills, name)
			}
			continue
		}
		for _, harness := range filter {
			linkPath := filepath.Join(l.harnessSkillsDir(harness), name)
			if !containsString(record.Links, linkPath) {
				detail := fmt.Sprintf("no %s link recorded", harness)
				if harness == "codex" {
					detail = "codex reads the canonical copy directly; there is no codex link to remove"
				}
				results = append(results, Result{Skill: name, Target: harness, Path: linkPath, Action: "skipped", Detail: detail})
				continue
			}
			res, dropped, err := removeLink(l, name, linkPath, opts)
			results = append(results, res)
			if err != nil {
				failures = append(failures, err)
			}
			if dropped {
				dropManifestLink(manifest, name, linkPath)
			}
		}
	}
	if !opts.DryRun {
		if len(manifest.Skills) == 0 {
			if err := os.Remove(manifestPath(l.root())); err != nil && !errors.Is(err, fs.ErrNotExist) {
				failures = append(failures, fmt.Errorf("remove manifest: %w", err))
			}
		} else if err := saveManifest(l.root(), manifest); err != nil {
			failures = append(failures, fmt.Errorf("write manifest: %w", err))
		}
	}
	return results, errors.Join(failures...)
}

// resolveRemoveFilter decides between a full remove (every recorded link and
// copy) and a link-only remove for the selected harnesses.
func resolveRemoveFilter(opts Options) ([]string, bool, error) {
	if opts.All {
		if len(opts.Agents) > 0 {
			return nil, false, fmt.Errorf("--all cannot be combined with --agent")
		}
		return nil, true, nil
	}
	if len(opts.Agents) > 0 {
		agents, err := normalizeAgents(opts.Agents)
		if err != nil {
			return nil, false, err
		}
		return agents, false, nil
	}
	return nil, true, nil
}

// removeCanonical deletes every managed file of one skill when it still
// matches what AgentBox recorded or rendered. Modified managed files require
// Force and are backed up first. Files at shipped paths that AgentBox never
// recorded are foreign and are always left alone. removed reports whether the
// skill's SKILL.md is gone, which lets Remove drop the manifest record.
func removeCanonical(l layout, manifest *Manifest, name string, opts Options, bk *backupSet) ([]Result, bool, error) {
	dir := filepath.Join(l.root(), name)
	record := manifest.Skills[name]
	var results []Result
	var failures []error
	removed := true
	for _, file := range recordFileNames(name, record) {
		res, gone, err := removeCanonicalFile(dir, name, file, record, opts, bk)
		results = append(results, res)
		if err != nil {
			failures = append(failures, err)
		}
		if !gone {
			removed = false
		}
	}
	// Drop the skill directory when it is now empty; foreign content in it
	// keeps the directory and is left alone.
	if removed && !opts.DryRun {
		_ = os.Remove(dir)
	}
	return results, removed, errors.Join(failures...)
}

// removeCanonicalFile deletes one canonical file of a skill. gone reports
// whether the path no longer holds a copy AgentBox must keep.
func removeCanonicalFile(dir, skill, file string, record ManifestSkill, opts Options, bk *backupSet) (Result, bool, error) {
	path := filepath.Join(dir, filepath.FromSlash(file))
	res := Result{Skill: skill, File: file, Target: "agents", Path: path}
	tracked, trackedFile := record.File(file)
	body, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		res.Action, res.Detail = "missing", "canonical copy is already gone"
		return res, true, nil
	case err != nil:
		res.Action, res.Detail = "skipped", err.Error()
		return res, false, err
	}
	rendered, renderErr := EmbeddedFile(skill, file, opts.Version)
	ours := (trackedFile && hashHex(body) == tracked.SHA256) || (renderErr == nil && bytes.Equal(body, rendered))
	if !ours {
		if !trackedFile && file != "SKILL.md" {
			// A user file at a shipped template path was never managed; keep it.
			res.Action, res.Detail = "skipped", "differs from the managed copy; left alone"
			return res, true, nil
		}
		if !opts.Force {
			res.Action, res.Detail = "skipped", "copy differs from the managed manifest; use --force to remove it"
			if opts.DryRun {
				return res, false, nil
			}
			return res, false, fmt.Errorf("refusing to remove modified copy %s; use --force", path)
		}
		if opts.DryRun {
			res.Action, res.Detail = "would-remove", "modified copy; backed up first"
			return res, false, nil
		}
		if err := bk.move(path, filepath.Join("canonical", skill, filepath.FromSlash(file))); err != nil {
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
	if err := os.Remove(path); err != nil {
		res.Action, res.Detail = "skipped", err.Error()
		return res, false, err
	}
	res.Action = "removed"
	return res, true, nil
}

// removeLink deletes one recorded symlink when it still points at the
// canonical copy. Anything else at the path is foreign and left alone.
func removeLink(l layout, name, link string, opts Options) (Result, bool, error) {
	res := Result{Skill: name, Target: l.harnessForPath(link), Path: link}
	info, err := os.Lstat(link)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		res.Action, res.Detail = "missing", "link is already gone"
		return res, true, nil
	case err != nil:
		res.Action, res.Detail = "skipped", err.Error()
		return res, false, err
	case info.Mode()&os.ModeSymlink == 0:
		res.Action, res.Detail = "skipped", "not a symlink anymore; left alone"
		return res, true, nil
	}
	current, err := os.Readlink(link)
	if err != nil {
		res.Action, res.Detail = "skipped", err.Error()
		return res, false, err
	}
	if !linkTargets(link, current, filepath.Join(l.root(), name)) {
		res.Action, res.Detail = "skipped", "points elsewhere; left alone"
		return res, true, nil
	}
	if opts.DryRun {
		res.Action = "would-remove"
		return res, false, nil
	}
	if err := os.Remove(link); err != nil {
		res.Action, res.Detail = "skipped", err.Error()
		return res, false, err
	}
	res.Action = "removed"
	return res, true, nil
}

// installDir writes plain copies of every shipped skill file under an
// explicit directory. Templates land beside SKILL.md. It creates no links and
// no manifest: the directory is the user's to manage.
func installDir(opts Options) ([]Result, error) {
	if home := opts.Home; home != "" {
		if err := refuseCodexDestination(opts.Dir, home); err != nil {
			return nil, err
		}
	} else if resolved, err := os.UserHomeDir(); err == nil {
		if err := refuseCodexDestination(opts.Dir, resolved); err != nil {
			return nil, err
		}
	}
	var results []Result
	var failures []error
	for _, name := range names {
		files, err := renderFiles(name, opts.Version)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, file := range files {
			path := filepath.Join(opts.Dir, name, filepath.FromSlash(file.spec.Name))
			result, err := installDirOne(name, file.spec.Name, path, file.content, opts)
			results = append(results, result)
			if err != nil {
				failures = append(failures, err)
			}
		}
	}
	return results, errors.Join(failures...)
}

// refuseCodexDestination rejects a --dir inside ~/.codex/skills: Codex reads
// the canonical ~/.agents/skills root as well, so a copy there would register
// every skill twice in a Codex session.
func refuseCodexDestination(dir, home string) error {
	codexRoot := filepath.Join(home, ".codex", "skills")
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	rel, err := filepath.Rel(codexRoot, absDir)
	if err != nil {
		return nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	return fmt.Errorf("--dir %s is inside %s; Codex reads ~/.agents/skills directly, so a copy there would register the skill twice", dir, codexRoot)
}

func installDirOne(name, file, path string, content []byte, opts Options) (Result, error) {
	result := Result{Skill: name, File: file, Target: "dir", Path: path}
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
			result.Action = "would-update"
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
	if err := writeFile(path, content); err != nil {
		result.Action = "skipped"
		result.Detail = err.Error()
		return result, err
	}
	result.Action = "installed"
	return result, nil
}

// writeFile creates the parent directories and writes content to path.
func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

// linkTargets reports whether the symlink at linkPath with the given target
// resolves to canonicalDir.
func linkTargets(linkPath, target, canonicalDir string) bool {
	resolved := target
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(linkPath), resolved)
	}
	return filepath.Clean(resolved) == filepath.Clean(canonicalDir)
}

// replaceLink removes the current entry at linkPath and points it at target.
func replaceLink(linkPath, target string) error {
	if err := os.Remove(linkPath); err != nil {
		return err
	}
	return os.Symlink(target, linkPath)
}

// singleFileDir reports whether dir contains exactly one entry named
// SKILL.md. Directories with any other content are foreign even when their
// SKILL.md matches, because removing them would destroy that content.
func singleFileDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "SKILL.md" {
		return false
	}
	return true
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
