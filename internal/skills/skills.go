// Package skills embeds AgentBox's own usage skills and manages their
// lifecycle on the host. Every skill is version-matched to the running
// binary and installed as a single canonical copy under ~/.agents/skills
// (or ./.agents/skills with --local). Harnesses that do not read that shared
// root receive a relative symlink; nothing is ever written into
// ~/.codex/skills.
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

//go:embed agentbox-cli/SKILL.md agentbox-api/SKILL.md agentbox-orchestrator/SKILL.md
var embedded embed.FS

// names lists the embedded skills in a stable order.
var names = []string{"agentbox-cli", "agentbox-api", "agentbox-orchestrator"}

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
