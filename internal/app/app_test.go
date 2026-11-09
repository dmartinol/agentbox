package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func cmd(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	b, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, b)
	}
	return string(b)
}

func TestProfileValidationAndStorage(t *testing.T) {
	s := Store{Base: filepath.Join(t.TempDir(), "state")}
	p := Profile{Version: 1, Name: "dev", Agent: "codex", Image: "localhost/openshell-codex:1.2.3-r1", Mode: "gateway", Model: "unit-model", Provider: "unit-provider", Policy: "/tmp/policy.yaml", Approval: "prompted", BaseURL: "https://models.example.test/v1", APIKeyEnv: "UNIT_MODEL_KEY"}
	if err := s.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadProfile("dev")
	if err != nil || got.Model != p.Model {
		t.Fatalf("load: %v %+v", err, got)
	}
	info, err := os.Stat(s.ProfilePath("dev"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("profile permission: %v", err)
	}
	p.Args = []string{"--api-key=not-a-reference"}
	if err := p.Validate(); err == nil {
		t.Fatal("accepted secret-like argument")
	}
	if err := os.WriteFile(s.ProfilePath("dev"), []byte("version: 1\nname: dev\nunknown: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadProfile("dev"); err == nil {
		t.Fatal("accepted unknown profile field")
	}
}

func TestPolicyRequiresHardIsolation(t *testing.T) {
	if _, err := ParsePolicy(DefaultPolicy()); err != nil {
		t.Fatal(err)
	}
	weaker := strings.Replace(string(DefaultPolicy()), "hard_requirement", "best_effort", 1)
	if _, err := ParsePolicy([]byte(weaker)); err == nil {
		t.Fatal("accepted best-effort Landlock")
	}
}

func TestDirtySourceSnapshot(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	source := filepath.Join(base, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	cmd(t, source, "git", "init", "-q")
	cmd(t, source, "git", "config", "user.name", "Tester")
	cmd(t, source, "git", "config", "user.email", "test@example.invalid")
	for name, value := range map[string]string{"tracked.txt": "base\n", ".gitignore": "ignored.txt\n", "removed.txt": "remove me\n"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cmd(t, source, "git", "add", ".")
	cmd(t, source, "git", "commit", "-qm", "base")
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(source, "removed.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "new.txt"), []byte("new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("tracked.txt", filepath.Join(source, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "ignored.txt"), []byte("private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, source, "git", "add", "tracked.txt")
	a := App{Store: Store{Base: filepath.Join(base, "state")}, Runner: OSRunner{}}
	r, err := a.CreateWorktree(ctx, source, "run-test123")
	if err != nil {
		t.Fatal(err)
	}
	resolved, _ := filepath.EvalSymlinks(source)
	if r.Source != resolved || r.Baseline == "" {
		t.Fatalf("bad run: %+v", r)
	}
	for name, want := range map[string]string{"tracked.txt": "changed\n", "new.txt": "new\n"} {
		b, err := os.ReadFile(filepath.Join(r.Worktree, name))
		if err != nil || string(b) != want {
			t.Fatalf("%s: %v %q", name, err, b)
		}
	}
	if _, err := os.Stat(filepath.Join(r.Worktree, "ignored.txt")); !os.IsNotExist(err) {
		t.Fatalf("ignored file leaked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.Worktree, "removed.txt")); !os.IsNotExist(err) {
		t.Fatalf("removed file reappeared: %v", err)
	}
	link, err := os.Readlink(filepath.Join(r.Worktree, "link.txt"))
	if err != nil || link != "tracked.txt" {
		t.Fatalf("link: %v %s", err, link)
	}
	if out := cmd(t, source, "git", "status", "--short"); !strings.Contains(out, "tracked.txt") || !strings.Contains(out, "new.txt") {
		t.Fatalf("source changed: %s", out)
	}
	if out := cmd(t, r.Worktree, "git", "status", "--short"); out != "" {
		t.Fatalf("worktree not clean: %s", out)
	}
}

func TestAgentScriptPromptAndPermission(t *testing.T) {
	p := Profile{Version: 1, Name: "dev", Agent: "codex", Mode: "gateway", Model: "unit-model", Provider: "unit-provider", Policy: "/tmp/p", Approval: "prompted", BaseURL: "https://models.example.test/v1", APIKeyEnv: "UNIT_MODEL_KEY"}
	s, err := agentScript(p, "say 'ok'")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "'say '\\''ok'\\'''") {
		t.Fatalf("prompt not shell quoted: %s", s)
	}
	if strings.Contains(s, "bypass") {
		t.Fatal("prompted mode bypasses approval")
	}
	p.Args = []string{"--yolo"}
	if _, err := agentScript(p, ""); err == nil {
		t.Fatal("accepted bypass flag in prompted mode")
	}
}
