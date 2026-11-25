package app

import (
	"context"
	"errors"
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

func TestDirtySourceSnapshotTracksOnlyTrackedTree(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		change func(*testing.T, string)
		verify func(*testing.T, string)
	}{
		{
			name: "content modification",
			change: func(t *testing.T, source string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("changed\n"), 0600); err != nil {
					t.Fatal(err)
				}
			},
			verify: func(t *testing.T, worktree string) {
				t.Helper()
				assertFileContent(t, worktree, "tracked.txt", "changed\n")
			},
		},
		{
			name: "deletion",
			change: func(t *testing.T, source string) {
				t.Helper()
				if err := os.Remove(filepath.Join(source, "removed.txt")); err != nil {
					t.Fatal(err)
				}
			},
			verify: func(t *testing.T, worktree string) {
				t.Helper()
				assertPathAbsent(t, worktree, "removed.txt")
			},
		},
		{
			name: "staged addition",
			change: func(t *testing.T, source string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(source, "new.txt"), []byte("new\n"), 0600); err != nil {
					t.Fatal(err)
				}
				cmd(t, source, "git", "add", "new.txt")
			},
			verify: func(t *testing.T, worktree string) {
				t.Helper()
				assertFileContent(t, worktree, "new.txt", "new\n")
			},
		},
		{
			name: "staged rename",
			change: func(t *testing.T, source string) {
				t.Helper()
				cmd(t, source, "git", "mv", "old.txt", "renamed.txt")
			},
			verify: func(t *testing.T, worktree string) {
				t.Helper()
				assertPathAbsent(t, worktree, "old.txt")
				assertFileContent(t, worktree, "renamed.txt", "renamed\n")
			},
		},
		{
			name: "symlink target change",
			change: func(t *testing.T, source string) {
				t.Helper()
				if err := os.Remove(filepath.Join(source, "link.txt")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("removed.txt", filepath.Join(source, "link.txt")); err != nil {
					t.Fatal(err)
				}
			},
			verify: func(t *testing.T, worktree string) {
				t.Helper()
				link, err := os.Readlink(filepath.Join(worktree, "link.txt"))
				if err != nil || link != "removed.txt" {
					t.Fatalf("link.txt target = %q, %v; want removed.txt", link, err)
				}
			},
		},
		{
			name: "executable mode change",
			change: func(t *testing.T, source string) {
				t.Helper()
				if err := os.Chmod(filepath.Join(source, "script.sh"), 0755); err != nil {
					t.Fatal(err)
				}
			},
			verify: func(t *testing.T, worktree string) {
				t.Helper()
				info, err := os.Lstat(filepath.Join(worktree, "script.sh"))
				if err != nil || info.Mode().Perm() != 0755 {
					t.Fatalf("script.sh mode = %v, %v; want 0755", info.Mode().Perm(), err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := snapshotSourceFixture(t)
			test.change(t, source)
			runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
				if call.name == "git" {
					return OSRunner{}.Output(ctx, "git", call.args...)
				}
				return nil, nil
			}}
			base := t.TempDir()
			a := App{Store: Store{Base: filepath.Join(base, "state")}, Runner: runner}
			r, err := a.CreateWorktree(ctx, source, "run-test123")
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := filepath.EvalSymlinks(source)
			if err != nil {
				t.Fatal(err)
			}
			if r.Source != resolved || r.Baseline == "" {
				t.Fatalf("bad run: %+v", r)
			}
			if out := cmd(t, r.Worktree, "git", "status", "--porcelain"); out != "" {
				t.Fatalf("worktree not clean: %s", out)
			}
			tree := cmd(t, r.Worktree, "git", "ls-tree", "-r", "--name-only", "HEAD")
			treePaths := make(map[string]bool)
			for _, path := range strings.Split(strings.TrimSpace(tree), "\n") {
				if path != "" {
					treePaths[path] = true
				}
			}
			for _, path := range []string{".serena/state.json", ".agentbox-stage/context/pr-body.md", "local-tool.tmp", "ignored.txt"} {
				if treePaths[path] {
					t.Fatalf("untracked path %q entered selected tree: %s", path, tree)
				}
				assertPathAbsent(t, r.Worktree, path)
			}
			test.verify(t, r.Worktree)
			assertFileContent(t, source, ".agentbox-stage/context/pr-body.md", "stage handoff\n")
			assertFileContent(t, source, ".serena/state.json", "{}\n")
		})
	}
}

func snapshotSourceFixture(t *testing.T) string {
	t.Helper()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	cmd(t, source, "git", "init", "-q")
	cmd(t, source, "git", "config", "user.name", "Tester")
	cmd(t, source, "git", "config", "user.email", "test@example.invalid")
	for name, value := range map[string]string{
		"tracked.txt": "base\n",
		"removed.txt": "remove me\n",
		"old.txt":     "renamed\n",
		"script.sh":   "#!/bin/sh\n",
		".gitignore":  "ignored.txt\n",
	} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("tracked.txt", filepath.Join(source, "link.txt")); err != nil {
		t.Fatal(err)
	}
	cmd(t, source, "git", "add", ".")
	cmd(t, source, "git", "commit", "-qm", "base")
	for _, dir := range []string{".serena", ".agentbox-stage/context"} {
		if err := os.MkdirAll(filepath.Join(source, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, ".serena/state.json"), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".agentbox-stage/context/pr-body.md"), []byte("stage handoff\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "local-tool.tmp"), []byte("local\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "ignored.txt"), []byte("private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return source
}

func assertFileContent(t *testing.T, root, name, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(root, name))
	if err != nil || string(got) != want {
		t.Fatalf("%s: %v, %q; want %q", name, err, got, want)
	}
}

func assertPathAbsent(t *testing.T, root, name string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
		t.Fatalf("%s leaked into %s: %v", name, root, err)
	}
}

func TestStartFailsClosedBeforeSnapshotWhenStartingTreeCannotBePrepared(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name            string
		trackStage      bool
		change          func(*testing.T, string)
		inject          func(recordedCommand) ([]byte, error)
		wantError       string
		forbidWorktree  bool
		forbidRealApply bool
	}{
		{
			name:           "tracked stage handoff",
			trackStage:     true,
			wantError:      ".agentbox-stage/context/pr-body.md",
			forbidWorktree: true,
		},
		{
			name: "changed submodule",
			inject: func(call recordedCommand) ([]byte, error) {
				if commandHasArg(call, "--name-only") {
					return []byte("sub\x00"), nil
				}
				if commandHasArg(call, "ls-tree") {
					return []byte("160000 commit 0123456789abcdef0123456789abcdef01234567\tsub\x00"), nil
				}
				return nil, nil
			},
			wantError:      "sub",
			forbidWorktree: true,
		},
		{
			name: "apply check failure",
			change: func(t *testing.T, source string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("changed\n"), 0600); err != nil {
					t.Fatal(err)
				}
			},
			inject: func(call recordedCommand) ([]byte, error) {
				if commandHasArg(call, "apply") && commandHasArg(call, "--check") {
					return nil, errors.New("patch failed: tracked.txt")
				}
				return nil, nil
			},
			wantError:       "patch failed: tracked.txt",
			forbidRealApply: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := startingFailureSourceFixture(t, test.trackStage)
			if test.change != nil {
				test.change(t, source)
			}
			store := Store{Base: filepath.Join(t.TempDir(), "state")}
			policy, err := store.CreatePolicyConfiguration("default")
			if err != nil {
				t.Fatal(err)
			}
			profile := testProfile("codex", "gateway", "prompted")
			profile.Policy = policy.Path
			if err := store.SaveProfile(profile); err != nil {
				t.Fatal(err)
			}
			runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
				switch {
				case call.name == "openshell" && containsCommand(call.args, "gateway", "info"):
					return []byte(`{"type":"podman"}`), nil
				case call.name == "openshell" && containsCommand(call.args, "provider", "get"):
					return []byte(`{}`), nil
				case call.name == "openshell" && containsCommand(call.args, "provider", "list"):
					return []byte(`{"providers":[{"name":"unit-provider","type":"gateway-bearer"}]}`), nil
				case call.name == "podman" && containsCommand(call.args, "image", "inspect"):
					return []byte(`{}`), nil
				case call.name == "git":
					if test.inject != nil {
						if output, err := test.inject(call); err != nil || output != nil {
							return output, err
						}
					}
					return OSRunner{}.Output(ctx, "git", call.args...)
				}
				return nil, nil
			}}
			app := App{Store: store, Runner: runner}
			_, err = app.Start(ctx, StartOptions{
				Profile: profile.Name,
				Gateway: "unit-gateway",
				Source:  source,
			})
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v; want %q", err, test.wantError)
			}
			assertNoStartingSnapshotSideEffects(t, runner, store)
			for _, call := range runner.outputs {
				if test.forbidWorktree && call.name == "git" && containsCommand(call.args, "worktree", "add") {
					t.Fatalf("worktree created before failure: %#v", call)
				}
				if test.forbidRealApply && call.name == "git" && commandHasArg(call, "apply") && !commandHasArg(call, "--check") {
					t.Fatalf("non-check apply ran: %#v", call)
				}
			}
		})
	}
}

func startingFailureSourceFixture(t *testing.T, trackStage bool) string {
	t.Helper()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	cmd(t, source, "git", "init", "-q")
	cmd(t, source, "git", "config", "user.name", "Tester")
	cmd(t, source, "git", "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, source, "git", "add", "tracked.txt")
	cmd(t, source, "git", "commit", "-qm", "base")
	if err := os.MkdirAll(filepath.Join(source, ".agentbox-stage/context"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".agentbox-stage/context/pr-body.md"), []byte("stage handoff\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if trackStage {
		cmd(t, source, "git", "add", ".agentbox-stage")
	}
	return source
}

func assertNoStartingSnapshotSideEffects(t *testing.T, runner *fakeRunner, store Store) {
	t.Helper()
	for _, calls := range [][]recordedCommand{runner.outputs, runner.streams} {
		for _, call := range calls {
			if call.name == "git" && (commandHasArg(call, "--all") || commandHasArg(call, "AgentBox starting snapshot") || commandHasArg(call, "bundle")) {
				t.Fatalf("snapshot side effect ran: %#v", call)
			}
			if call.name == "openshell" && containsCommand(call.args, "sandbox", "create") {
				t.Fatalf("sandbox created before starting-tree failure: %#v", call)
			}
			if call.name == "tmux" {
				t.Fatalf("tmux ran before starting-tree failure: %#v", call)
			}
		}
	}
	runs, err := store.ListRuns()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("run records saved before starting-tree failure: %#v", runs)
	}
}

func TestAgentScriptPromptAndPermission(t *testing.T) {
	p := Profile{Version: 1, Name: "dev", Agent: "codex", Mode: "gateway", Model: "unit-model", Provider: "unit-provider", Policy: "/tmp/p", Approval: "prompted", BaseURL: "https://models.example.test/v1", APIKeyEnv: "UNIT_MODEL_KEY"}
	s, err := agentScript(p, "say 'ok'", launchInitial)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "'say '\\''ok'\\'''") {
		t.Fatalf("prompt not shell quoted: %s", s)
	}
	if strings.Contains(s, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatal("prompted mode bypasses approval")
	}
	p.Args = []string{"--yolo"}
	if _, err := agentScript(p, "", launchInitial); err == nil {
		t.Fatal("accepted bypass flag in prompted mode")
	}
}
