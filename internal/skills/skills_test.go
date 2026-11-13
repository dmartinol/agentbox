package skills

import (
	"os"
	"strings"
	"testing"
)

// TestEmbeddedSkillsParseStrictFrontMatter guards the historical defect from
// issue #30: agentbox-cli shipped an unquoted description containing a
// colon-space, which strict YAML consumers such as `npx skills` reject with
// "Nested mappings are not allowed in compact mappings".
func TestEmbeddedSkillsParseStrictFrontMatter(t *testing.T) {
	for _, name := range Names() {
		content, err := Embedded(name, "9.9.9")
		if err != nil {
			t.Fatal(err)
		}
		fm, err := ParseFrontMatter(content)
		if err != nil {
			t.Fatalf("%s front matter does not parse: %v", name, err)
		}
		if fm.Name != name {
			t.Fatalf("%s front matter name = %q", name, fm.Name)
		}
		if strings.TrimSpace(fm.Description) == "" {
			t.Fatalf("%s front matter description is empty", name)
		}
		if fm.Version != "9.9.9" {
			t.Fatalf("%s front matter version = %q", name, fm.Version)
		}
	}
}

// TestEmbeddedSkillsAreSelfContained keeps the render and hygiene contract of
// the previous suite.
func TestEmbeddedSkillsAreSelfContained(t *testing.T) {
	for _, name := range Names() {
		content, err := Embedded(name, "9.9.9")
		if err != nil {
			t.Fatal(err)
		}
		body := string(content)
		if strings.Contains(body, VersionPlaceholder) {
			t.Fatalf("%s still contains the version placeholder", name)
		}
		if got := InstalledVersion(content); got != "9.9.9" {
			t.Fatalf("%s version = %q", name, got)
		}
		for _, forbidden := range []string{"/Users/", "/home/", `C:\Users`, "sk-", "ghp_", "AKIA"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s contains %q", name, forbidden)
			}
		}
	}
}

// TestParseFrontMatterRejectsHistoricalUnquotedDescription verifies that the
// exact front matter that shipped in PR #29 fails the strict check, so the
// defect cannot silently return.
func TestParseFrontMatterRejectsHistoricalUnquotedDescription(t *testing.T) {
	raw := "---\n" +
		"name: agentbox-cli\n" +
		"description: Operate AgentBox from a shell: start, inspect, attach to, sync.\n" +
		"version: \"1.0.0\"\n" +
		"---\n"
	if _, err := ParseFrontMatter([]byte(raw)); err == nil {
		t.Fatal("parsed the historical unquoted description that strict consumers reject")
	}
}

func TestParseFrontMatterRejectsBrokenDocuments(t *testing.T) {
	cases := map[string]string{
		"missing opening fence":  "name: x\n",
		"missing closing fence":  "---\nname: x\n",
		"extra key":              "---\nname: x\ndescription: y\nversion: z\nextra: 1\n---\n",
		"missing keys":           "---\nname: x\n---\n",
		"empty":                  "---\n---\n",
		"non-scalar description": "---\nname: x\ndescription: [a, b]\nversion: v\n---\n",
	}
	for label, raw := range cases {
		if _, err := ParseFrontMatter([]byte(raw)); err == nil {
			t.Fatalf("%s: parsed a broken front matter document", label)
		}
	}
}

func TestParseFrontMatterAcceptsQuotedDescription(t *testing.T) {
	raw := "---\nname: demo\ndescription: \"Do a thing: with a colon\"\nversion: \"1.2.3\"\n---\n"
	fm, err := ParseFrontMatter([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if fm.Description != "Do a thing: with a colon" || fm.Version != "1.2.3" || fm.Name != "demo" {
		t.Fatalf("front matter = %+v", fm)
	}
}

func TestShowRejectsUnknownSkill(t *testing.T) {
	if _, err := Embedded("../agentbox-cli", "1.0.0"); err == nil {
		t.Fatal("accepted a path-like skill name")
	}
}

func TestRenderDefaultVersionIsDev(t *testing.T) {
	content, err := Embedded("agentbox-cli", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := InstalledVersion(content); got != "dev" {
		t.Fatalf("default version = %q", got)
	}
}

// mustRead reads path and fails the test on error.
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
