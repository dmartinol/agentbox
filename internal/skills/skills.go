// Package skills embeds AgentBox's own usage skills and manages their
// lifecycle on the host. Every skill is version-matched to the running
// binary and installed as one canonical directory under ~/.agents/skills
// (or ./.agents/skills with --local), containing SKILL.md and any templates
// the skill ships. Harnesses that do not read that shared root receive a
// relative symlink; nothing is ever written into ~/.codex/skills.
package skills

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// VersionPlaceholder is replaced with the running binary version at render
// time. The embedded files keep the placeholder so installed copies can be
// compared against the binary that wrote them.
const VersionPlaceholder = "{{VERSION}}"

//go:embed agentbox-cli/SKILL.md agentbox-api/SKILL.md agentbox-orchestrator/SKILL.md agentbox-orchestrator/templates/issue.md agentbox-orchestrator/templates/pr.md agentbox-orchestrator/templates/verdict.md agentbox-orchestrator/templates/state.md agentbox-orchestrator/templates/review-task.md agentbox-orchestrator/templates/verify-task.md
var embedded embed.FS

// SkillFile is one file shipped with an embedded skill.
type SkillFile struct {
	// Name is the installed path relative to the skill directory, using
	// forward slashes. Templates are flattened: the shipped
	// templates/issue.md installs as issue.md beside SKILL.md.
	Name string
	// Source is the path of the file within the embedded filesystem.
	Source string
	// Template reports whether the file is a shipped template rather than
	// SKILL.md.
	Template bool
}

// skillSpec is the ordered file inventory of one embedded skill.
type skillSpec struct {
	name  string
	files []SkillFile
}

// newSkillSpec builds an inventory with SKILL.md first and the named
// templates after it, in the given order.
func newSkillSpec(name string, templates ...string) skillSpec {
	files := []SkillFile{{Name: "SKILL.md", Source: name + "/SKILL.md"}}
	for _, template := range templates {
		files = append(files, SkillFile{
			Name:     template + ".md",
			Source:   name + "/templates/" + template + ".md",
			Template: true,
		})
	}
	return skillSpec{name: name, files: files}
}

// specs is the stable ordered inventory of every embedded skill. Other skills
// remain single-file; the orchestrator skill ships its templates beside
// SKILL.md so an installed copy is self-contained.
var specs = []skillSpec{
	newSkillSpec("agentbox-cli"),
	newSkillSpec("agentbox-api"),
	newSkillSpec("agentbox-orchestrator", "issue", "pr", "verdict", "state", "review-task", "verify-task"),
}

// names lists the embedded skills in a stable order.
var names = func() []string {
	out := make([]string, 0, len(specs))
	for _, spec := range specs {
		out = append(out, spec.name)
	}
	return out
}()

// harnesses lists every supported harness in a stable order.
var harnesses = []string{"claude", "codex", "opencode"}

// linkedHarnesses lists harnesses that receive a relative symlink to the
// canonical copy because they do not read the shared .agents/skills root.
var linkedHarnesses = []string{"claude", "opencode"}

// Codex duplicate rule: Codex scans both ~/.agents/skills and ~/.codex/skills.
// AgentBox installs the canonical copy into ~/.agents/skills, so writing or
// linking anything into ~/.codex/skills would register every skill twice in a
// Codex session. AgentBox therefore never writes there: selecting Codex
// controls reporting and legacy-copy migration only. Do not re-add
// ~/.codex/skills as an install destination.

// Names returns the embedded skill names in a stable order.
func Names() []string {
	out := make([]string, len(names))
	copy(out, names)
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

// Files returns the shipped files of one embedded skill in stable order,
// with SKILL.md first.
func Files(name string) []SkillFile {
	spec, ok := lookupSpec(name)
	if !ok {
		return nil
	}
	out := make([]SkillFile, len(spec.files))
	copy(out, spec.files)
	return out
}

// lookupSpec finds the inventory of one embedded skill.
func lookupSpec(name string) (skillSpec, bool) {
	for _, spec := range specs {
		if spec.name == name {
			return spec, true
		}
	}
	return skillSpec{}, false
}

// lookupFile finds one shipped file of a skill.
func lookupFile(name, file string) (SkillFile, bool) {
	spec, ok := lookupSpec(name)
	if !ok {
		return SkillFile{}, false
	}
	for _, candidate := range spec.files {
		if candidate.Name == file {
			return candidate, true
		}
	}
	return SkillFile{}, false
}

// Embedded returns the rendered content of one embedded skill's SKILL.md.
func Embedded(name, version string) ([]byte, error) {
	return EmbeddedFile(name, "SKILL.md", version)
}

// EmbeddedFile returns the rendered content of one shipped file of an
// embedded skill. The version placeholder is substituted in every file, so
// templates can name the orchestrator version too.
func EmbeddedFile(name, file, version string) ([]byte, error) {
	if !ValidName(name) {
		return nil, fmt.Errorf("unknown skill %q (available: %s)", name, strings.Join(names, ", "))
	}
	spec, ok := lookupFile(name, file)
	if !ok {
		return nil, fmt.Errorf("unknown file %q for skill %q (available: %s)", file, name, strings.Join(fileNames(name), ", "))
	}
	raw, err := embedded.ReadFile(spec.Source)
	if err != nil {
		return nil, err
	}
	return render(raw, version), nil
}

// fileNames returns the installed names of a skill's files in stable order.
func fileNames(name string) []string {
	spec, ok := lookupSpec(name)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(spec.files))
	for _, file := range spec.files {
		out = append(out, file.Name)
	}
	return out
}

func render(raw []byte, version string) []byte {
	return bytes.ReplaceAll(raw, []byte(VersionPlaceholder), []byte(effectiveVersion(version)))
}

// effectiveVersion defaults an empty version to dev, matching what render
// writes into the front matter.
func effectiveVersion(version string) string {
	if version == "" {
		return "dev"
	}
	return version
}

// FrontMatter is the parsed SKILL.md front matter of a rendered skill.
type FrontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
}

// ParseFrontMatter parses the YAML front matter of a SKILL.md strictly: the
// document must open and close with --- fences and contain exactly the name,
// description, and version keys. Strict consumers such as `npx skills` skip
// skills whose front matter is not valid YAML, so AgentBox reports the same
// condition as invalid instead of installing an unusable skill.
func ParseFrontMatter(content []byte) (FrontMatter, error) {
	var zero FrontMatter
	lines := strings.Split(strings.TrimPrefix(string(content), "\ufeff"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return zero, fmt.Errorf("front matter must start with a --- line")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return zero, fmt.Errorf("front matter is not closed with a --- line")
	}
	var fm FrontMatter
	dec := yaml.NewDecoder(strings.NewReader(strings.Join(lines[1:end], "\n")))
	dec.KnownFields(true)
	if err := dec.Decode(&fm); err != nil {
		if errors.Is(err, io.EOF) {
			return zero, fmt.Errorf("front matter is empty")
		}
		return zero, fmt.Errorf("front matter is not valid YAML: %w", err)
	}
	if strings.TrimSpace(fm.Name) == "" || strings.TrimSpace(fm.Description) == "" || strings.TrimSpace(fm.Version) == "" {
		return zero, fmt.Errorf("front matter needs non-empty name, description, and version")
	}
	return fm, nil
}

// InstalledVersion extracts the version recorded in a rendered SKILL.md with
// a lenient line scan. It returns an empty string when no version line is
// present. It is used for reporting only; state decisions compare the sha256
// of the whole file.
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

// validHarness reports whether name is a harness AgentBox can select.
func validHarness(name string) bool {
	switch name {
	case "claude", "codex", "opencode":
		return true
	}
	return false
}

// containsString reports whether list contains value.
func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
