package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// ManifestFile is the bookkeeping file AgentBox writes into a canonical
// skills root. It records exactly which copies and links AgentBox owns so
// update and remove never touch anything else.
const ManifestFile = ".agentbox-managed.json"

// manifestVersion is the schema version of Manifest.
const manifestVersion = 1

// Manifest is the parsed managed-skills manifest of one canonical root.
type Manifest struct {
	Version int                      `json:"version"`
	Skills  map[string]ManifestSkill `json:"skills"`
}

// ManifestSkill records one managed skill: the version and sha256 of the
// rendered file AgentBox wrote, its canonical path, and the symlinks created
// for harnesses that do not read the canonical root.
type ManifestSkill struct {
	Version string   `json:"version"`
	SHA256  string   `json:"sha256"`
	Path    string   `json:"path"`
	Links   []string `json:"links"`
}

func manifestPath(root string) string { return filepath.Join(root, ManifestFile) }

func manifestExists(root string) bool {
	info, err := os.Stat(manifestPath(root))
	return err == nil && !info.IsDir()
}

// loadManifest reads the manifest of a canonical root. A missing, unreadable,
// or future-version manifest yields an empty one: install then treats
// existing copies as unmanaged and refuses to replace them without --force,
// so no user content is ever lost to a corrupt manifest.
func loadManifest(root string) *Manifest {
	m := &Manifest{Version: manifestVersion, Skills: map[string]ManifestSkill{}}
	body, err := os.ReadFile(manifestPath(root))
	if err != nil {
		return m
	}
	var parsed Manifest
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Version != manifestVersion {
		return m
	}
	if parsed.Skills == nil {
		parsed.Skills = map[string]ManifestSkill{}
	}
	return &parsed
}

// saveManifest atomically writes the manifest of a canonical root.
func saveManifest(root string, m *Manifest) error {
	m.Version = manifestVersion
	for name, record := range m.Skills {
		if record.Links == nil {
			record.Links = []string{}
			m.Skills[name] = record
		}
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
