package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// TestValidManifestFileName pins the path-local rules applied to every
// manifest file name before lifecycle code joins it onto a directory.
func TestValidManifestFileName(t *testing.T) {
	for _, name := range []string{"SKILL.md", "issue.md", "nested/file.md"} {
		if !validManifestFileName(name) {
			t.Fatalf("%q was rejected", name)
		}
	}
	for _, name := range []string{
		"",
		".",
		"..",
		"/etc/passwd",
		"../outside.md",
		"../../outside.md",
		"a/../../outside.md",
		"a//b.md",
		"./issue.md",
		"a/./b.md",
		"dir/",
		`..\outside.md`,
		`C:\outside.md`,
	} {
		if validManifestFileName(name) {
			t.Fatalf("%q was accepted", name)
		}
	}
}

// TestLoadManifestDropsUnsafeFileNames rejects unsafe names on load so no
// later lifecycle step can ever join them onto a directory.
func TestLoadManifestDropsUnsafeFileNames(t *testing.T) {
	root := t.TempDir()
	body := `{"version":2,"skills":{"agentbox-cli":{"version":"1.0.0","path":"` +
		filepath.Join(root, "agentbox-cli", "SKILL.md") + `","links":[],"files":[` +
		`{"name":"SKILL.md","sha256":"aaaaaaaa","path":""},` +
		`{"name":"../../outside.md","sha256":"bbbbbbbb","path":""},` +
		`{"name":"/absolute.md","sha256":"cccccccc","path":""},` +
		`{"name":"..","sha256":"dddddddd","path":""}]}}}`
	if err := os.WriteFile(manifestPath(root), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	record := loadManifest(root).Skills["agentbox-cli"]
	if len(record.Files) != 1 || record.Files[0].Name != "SKILL.md" {
		t.Fatalf("loaded files = %+v", record.Files)
	}
	if record.Files[0].Path != filepath.Join(root, "agentbox-cli", "SKILL.md") {
		t.Fatalf("valid entry path = %q", record.Files[0].Path)
	}
}

// TestRemoveNeverJoinsTraversalFileName proves a crafted manifest entry
// cannot delete a file outside the skill directory.
func TestRemoveNeverJoinsTraversalFileName(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".agents", "skills")
	outside := filepath.Join(root, "outside.md")
	if err := os.WriteFile(outside, []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := loadManifest(root)
	record := manifest.Skills["agentbox-cli"]
	record.Files = append(record.Files, ManifestEntry{Name: "../outside.md", SHA256: hashHex([]byte("keep me\n"))})
	manifest.Skills["agentbox-cli"] = record
	if err := saveManifest(root, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove(Options{Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	if body := mustRead(t, outside); string(body) != "keep me\n" {
		t.Fatalf("remove touched a file outside the skill directory: %q", body)
	}
	if _, err := os.Stat(filepath.Join(root, "agentbox-cli", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("remove stopped managing the skill")
	}
}
