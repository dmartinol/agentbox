//go:build !windows

package it_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestTUICompleteLayeredNavigation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TUI integration test in short mode")
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "agentbox")
	build := exec.Command("go", "build", "-o", bin, "./cmd/agentbox")
	build.Dir = root
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build agentbox: %v", err)
	}

	stateDir := filepath.Join(tmp, "state")
	stubDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stubDir, 0700); err != nil {
		t.Fatal(err)
	}
	runsDir := filepath.Join(stateDir, "runs")
	if err := os.MkdirAll(runsDir, 0700); err != nil {
		t.Fatal(err)
	}
	seedRun := `{"id":"run-seed","status":"create-failed","agent":"codex","mode":"standard","model":"test-model","approval":"prompted","sandbox":"run-seed","gateway":"openshell","worktree":"/tmp/run-seed","branch":"agentbox/run-seed"}`
	if err := os.WriteFile(filepath.Join(runsDir, "run-seed.json"), []byte(seedRun), 0600); err != nil {
		t.Fatal(err)
	}

	openshell := `#!/bin/sh
is_sandbox_exec=0
has_sandbox=0
has_exec=0
for arg in "$@"; do
  [ "$arg" = "sandbox" ] && has_sandbox=1
  [ "$arg" = "exec" ] && has_exec=1
done
if [ "$has_sandbox" = 1 ] && [ "$has_exec" = 1 ]; then
  printf '%s\n' 'ATTACH_DONE'
  exit 0
fi
case "$1" in
  status)
    printf '%s\n' '{"status":"connected","version":"test"}'
    ;;
  gateway)
    if [ "$2" = "list" ]; then
      printf '%s\n' '[{"name":"openshell","active":true,"endpoint":"https://localhost:17670","type":"local","auth":"mtls","source":"test"}]'
    else
      printf '%s\n' '{}'
    fi
    ;;
  *)
    printf '%s\n' '{}'
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(stubDir, "openshell"), []byte(openshell), 0700); err != nil {
		t.Fatal(err)
	}

	podman := `#!/bin/sh
if [ "$1" = "images" ]; then
  printf '%s\n' '[]'
else
  exit 0
fi
`
	if err := os.WriteFile(filepath.Join(stubDir, "podman"), []byte(podman), 0700); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "tui")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"AGENTBOX_HOME="+stateDir,
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TERM=xterm-256color",
		"ENMAAS_KEY=",
	)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 140})
	if err != nil {
		t.Fatalf("start TUI: %v", err)
	}
	defer func() {
		_ = ptmx.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}()

	chunks := make(chan string, 256)
	var latest string
	var mu sync.Mutex
	go func() {
		buf := make([]byte, 4096)
		var acc strings.Builder
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				acc.WriteString(chunk)
				mu.Lock()
				latest = acc.String()
				mu.Unlock()
				chunks <- chunk
			}
			if readErr != nil {
				close(chunks)
				return
			}
		}
	}()

	drain := func() {
		for {
			select {
			case <-chunks:
			default:
				return
			}
		}
	}

	send := func(keys string) {
		t.Helper()
		if _, writeErr := ptmx.Write([]byte(keys)); writeErr != nil {
			t.Fatalf("write keys %q: %v", keys, writeErr)
		}
	}

	waitFor := func(t *testing.T, substrings ...string) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		var seen strings.Builder
		for {
			select {
			case chunk, ok := <-chunks:
				if !ok {
					t.Fatalf("TUI output closed before %v; output:\n%s", substrings, seen.String())
				}
				seen.WriteString(chunk)
				if containsAll(normalizeScreen(seen.String()), substrings...) {
					return
				}
			case <-deadline:
				mu.Lock()
				text := latest
				mu.Unlock()
				t.Fatalf("timed out waiting for %v; output:\n%s", substrings, stripANSI(text))
			}
		}
	}

	press := func(t *testing.T, keys string, substrings ...string) {
		t.Helper()
		drain()
		send(keys)
		if len(substrings) != 0 {
			waitFor(t, substrings...)
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	}

	// The application starts in the page-list layer.
	waitFor(t, "AgentBox/Dashboard", "Focus:Pages")

	// Tab moves inward through all three layers.
	press(t, "\t", "Focus:Content")
	press(t, "\t", "Focus:Actions")

	// Arrow keys move among buttons without leaving the action layer.
	press(t, "\x1b[C", "Focus:Actions")
	press(t, "\x1b[D", "Focus:Actions")

	// Esc moves outward through all three layers.
	press(t, "\x1b", "Focus:Content")
	press(t, "\x1b", "Focus:Pages")

	// Visit every main section. Each non-empty page has content and action layers.
	pages := []string{
		"Runs",
		"Images",
		"Gateways",
		"Policies",
		"Profiles",
		"Doctor",
	}
	for _, page := range pages {
		press(t, "\x1b[B") // Move to the next page in the navigation list.
		press(t, "\r", "AgentBox/"+page, "Focus:Content")

		if page == "Policies" {
			// Policies is empty in this fixture and has no action bar.
			// Tab must remain in the content layer rather than getting stuck.
			press(t, "\t", "Focus:Content")
			press(t, "\x1b", "Focus:Pages")
			continue
		}

		if page == "Images" {
			press(t, "\t", "Focus:Actions")
			press(t, "\x1b[C", "Focus:Actions")
			press(t, "\x1b[C", "Focus:Actions")
			press(t, "\x1b[C", "Focus:Actions")
			press(t, "\x1b[C", "Focus:Actions")
			press(t, "\r", "AgentBox/Images/Registry", "Focus:Content")
			press(t, "\x1b", "AgentBox/Images", "Focus:Content")
			press(t, "\x1b", "Focus:Pages")
			continue
		}

		if page == "Runs" {
			press(t, "\t", "Focus:Actions")
			press(t, "\x1b[C", "Focus:Actions") // Move from Start to Attach.
			press(t, "\r", "ATTACH_DONE", "Focus:Content")
			press(t, "\x1b", "Focus:Pages")
			continue
		}

		if page == "Profiles" {
			press(t, "\t", "Focus:Actions")
			press(t, "\r", "AgentBox/Profiles/Createprofile", "Focus:Content")
			press(t, "\x1b", "AgentBox/Profiles", "Focus:Content")
			press(t, "\x1b", "Focus:Pages")
			continue
		}

		press(t, "\t", "Focus:Actions")
		press(t, "\x1b[C", "Focus:Actions")
		press(t, "\x1b[D", "Focus:Actions")
		press(t, "\x1b", "Focus:Content")
		press(t, "\x1b", "Focus:Pages")
	}

	// Open a form from the Runs action bar and return with Esc.
	press(t, "\x1b[A") // Doctor -> Profiles
	press(t, "\x1b[A") // Profiles -> Policies
	press(t, "\x1b[A") // Policies -> Gateways
	press(t, "\x1b[A") // Gateways -> Images
	press(t, "\x1b[A") // Images -> Runs
	press(t, "\r", "AgentBox/Runs", "Focus:Content")
	press(t, "\t", "Focus:Actions")
	press(t, "\x1b[C", "Focus:Actions")
	press(t, "\x1b[D", "Focus:Actions") // Return to the first button.
	press(t, "\r", "AgentBox/Runs/Startrun", "Focus:Content", "Nosavedprofilesfound")
	press(t, "\x1b", "AgentBox/Runs", "Focus:Content")
	press(t, "\x1b", "Focus:Pages")

	// The app must remain exitable from the deepest action-button layer.
	press(t, "\t", "Focus:Content")
	press(t, "\t", "Focus:Actions")
	press(t, "q")

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("TUI exited with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("TUI did not exit after q")
	}
}

func stripANSI(s string) string {
	pattern := regexp.MustCompile(`\x1b(?:\[[0-9;?]*[ -/]*[@-~]|\][^\x07]*\x07|[@-Z\\-_])`)
	return pattern.ReplaceAllString(s, "")
}

func normalizeScreen(s string) string {
	return strings.Join(strings.Fields(stripANSI(s)), "")
}

func containsAll(s string, substrings ...string) bool {
	for _, substr := range substrings {
		if !strings.Contains(s, substr) {
			return false
		}
	}
	return true
}
