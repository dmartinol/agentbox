package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ManifestFile is the bookkeeping file AgentBox writes into a canonical
// skills root. It records exactly which copies and links AgentBox owns so
// update and remove never touch anything else.
const ManifestFile = ".agentbox-managed.json"

// manifestVersion is the schema version of Manifest. Version 2 records one
// entry per managed file; version 1 recorded only the canonical SKILL.md.
const manifestVersion = 2

// Manifest is the parsed managed-skills manifest of one canonical root.
type Manifest struct {
	Version int                      `json:"version"`
	Skills  map[string]ManifestSkill `json:"skills"`
}

// ManifestSkill records one managed skill: the binary version that wrote it,
// the canonical SKILL.md path, the symlinks created for harnesses that do not
// read the canonical root, and one entry per managed file.
type ManifestSkill struct {
	Version string          `json:"version"`
	Path    string          `json:"path"`
	Links   []string        `json:"links"`
	Files   []ManifestEntry `json:"files"`
}

// ManifestEntry records one file AgentBox wrote for a skill: its installed
// name relative to the skill directory, the sha256 of the rendered content,
// and its absolute path.
type ManifestEntry struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Path   string `json:"path"`
}

// legacyManifestSkill is the version 1 record shape: one canonical SKILL.md
// with no template files.
type legacyManifestSkill struct {
	Version string   `json:"version"`
	SHA256  string   `json:"sha256"`
	Path    string   `json:"path"`
	Links   []string `json:"links"`
}

func manifestPath(root string) string { return filepath.Join(root, ManifestFile) }

// validManifestFileName reports whether name is a safe installed file name
// relative to one skill directory. Installed names are slash-separated and
// always stay inside that directory: the check is path-local, so an empty
// name, an absolute path, a "." or ".." element, or any traversal is refused
// before a manifest name can be joined onto a directory.
func validManifestFileName(name string) bool {
	if name == "" || strings.ContainsRune(name, 0) {
		return false
	}
	if filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
		return false
	}
	slashed := filepath.ToSlash(name)
	// A backslash is a path separator on Windows even when this manifest was
	// written on a Unix host.
	if strings.Contains(slashed, `\`) {
		return false
	}
	for _, element := range strings.Split(slashed, "/") {
		if element == "" || element == "." || element == ".." {
			return false
		}
	}
	return true
}

func manifestExists(root string) bool {
	info, err := os.Stat(manifestPath(root))
	return err == nil && !info.IsDir()
}

// loadManifest reads the manifest of a canonical root. A missing, unreadable,
// corrupt, or future-version manifest yields an empty one: install then
// treats existing copies as unmanaged and refuses to replace them without
// --force, so no user content is ever lost to a corrupt manifest. A version 1
// manifest is migrated in memory to the per-file shape: its recorded
// SKILL.md hash becomes the file entry, so an existing install is recognised
// as managed and gains the newly shipped templates on update.
func loadManifest(root string) *Manifest {
	m := &Manifest{Version: manifestVersion, Skills: map[string]ManifestSkill{}}
	body, err := os.ReadFile(manifestPath(root))
	if err != nil {
		return m
	}
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return m
	}
	switch probe.Version {
	case 1:
		var legacy struct {
			Version int                            `json:"version"`
			Skills  map[string]legacyManifestSkill `json:"skills"`
		}
		if err := json.Unmarshal(body, &legacy); err != nil {
			return m
		}
		for name, record := range legacy.Skills {
			path := record.Path
			if path == "" {
				path = filepath.Join(root, name, "SKILL.md")
			}
			m.Skills[name] = ManifestSkill{
				Version: record.Version,
				Path:    path,
				Links:   nonNil(record.Links),
				Files: []ManifestEntry{{
					Name:   "SKILL.md",
					SHA256: record.SHA256,
					Path:   path,
				}},
			}
		}
		return m
	case manifestVersion:
		var parsed Manifest
		if err := json.Unmarshal(body, &parsed); err != nil {
			return m
		}
		if parsed.Skills == nil {
			parsed.Skills = map[string]ManifestSkill{}
		}
		for name, record := range parsed.Skills {
			record.Links = nonNil(record.Links)
			if record.Path == "" {
				record.Path = filepath.Join(root, name, "SKILL.md")
			}
			files := make([]ManifestEntry, 0, len(record.Files))
			for _, file := range record.Files {
				// The manifest is user-writable, so every name is validated
				// before lifecycle code joins it onto a directory. A rejected
				// entry is treated as unmanaged and dropped.
				if !validManifestFileName(file.Name) {
					continue
				}
				if file.Path == "" {
					file.Path = filepath.Join(root, name, filepath.FromSlash(file.Name))
				}
				files = append(files, file)
			}
			record.Files = files
			parsed.Skills[name] = record
		}
		return &parsed
	}
	return m
}

// saveManifest atomically writes the manifest of a canonical root.
func saveManifest(root string, m *Manifest) error {
	m.Version = manifestVersion
	for name, record := range m.Skills {
		record.Links = nonNil(record.Links)
		if record.Files == nil {
			record.Files = []ManifestEntry{}
		}
		m.Skills[name] = record
	}
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(root, ".agentbox-managed-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(body, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), manifestPath(root))
}

// File returns the record of one managed file of the skill.
func (s ManifestSkill) File(name string) (ManifestEntry, bool) {
	for _, file := range s.Files {
		if file.Name == name {
			return file, true
		}
	}
	return ManifestEntry{}, false
}

// setFile records file in the skill, replacing an entry with the same name.
func (s *ManifestSkill) setFile(file ManifestEntry) {
	for i, existing := range s.Files {
		if existing.Name == file.Name {
			s.Files[i] = file
			return
		}
	}
	s.Files = append(s.Files, file)
}

// recordFileNames returns the file names recorded for a skill, in a stable
// order: the shipped inventory order first, then any recorded file that is no
// longer shipped, sorted by name.
func recordFileNames(skill string, record ManifestSkill) []string {
	out := fileNames(skill)
	seen := make(map[string]bool, len(out))
	for _, name := range out {
		seen[name] = true
	}
	var extra []string
	for _, file := range record.Files {
		if file.Name == "" || seen[file.Name] {
			continue
		}
		seen[file.Name] = true
		extra = append(extra, file.Name)
	}
	sort.Strings(extra)
	return append(out, extra...)
}

func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// addManifestLink records that link points at the managed copy of name.
func addManifestLink(m *Manifest, name, link string) {
	record, ok := m.Skills[name]
	if !ok {
		return
	}
	for _, existing := range record.Links {
		if existing == link {
			return
		}
	}
	record.Links = append(record.Links, link)
	m.Skills[name] = record
}

// dropManifestLink removes link from the record of name.
func dropManifestLink(m *Manifest, name, link string) {
	record, ok := m.Skills[name]
	if !ok {
		return
	}
	kept := make([]string, 0, len(record.Links))
	for _, existing := range record.Links {
		if existing != link {
			kept = append(kept, existing)
		}
	}
	record.Links = kept
	m.Skills[name] = record
}

// hashHex returns the sha256 of content as lowercase hex.
func hashHex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
