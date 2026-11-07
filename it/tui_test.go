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

func TestTUIKeyboardNavigation(t *testing.T) {
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

	openshell := `#!/bin/sh
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
	)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 140})
	if err != nil {
		t.Fatalf("start TUI: %v", err)
	}
	defer func() {
		_ = ptmx.Close()
		_ = cmd.Wait()
	}()

	output := make(chan string, 256)
	var latest string
	var mu sync.Mutex
	go func() {
		buf := make([]byte, 4096)
		var acc strings.Builder
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				acc.Write(buf[:n])
				mu.Lock()
				latest = acc.String()
				mu.Unlock()
				output <- acc.String()
			}
			if err != nil {
				close(output)
				return
			}
		}
	}()

	waitFor := func(t *testing.T, substrings ...string) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for {
			select {
			case text, ok := <-output:
				if !ok {
					t.Fatalf("TUI output closed before %v; output:\n%s", substrings, text)
				}
				clean := normalizeScreen(text)
				if containsAll(clean, substrings...) {
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

	send := func(keys string) {
		if _, err := ptmx.Write([]byte(keys)); err != nil {
			t.Fatalf("write keys %q: %v", keys, err)
		}
	}

	waitFor(t, "AgentBox/Dashboard")

	// Tab moves focus from the dashboard table to the quick-action buttons.
	send("\t")
	time.Sleep(100 * time.Millisecond)
	send("\r")
	waitFor(t, "AgentBox/Runs/Startrun")
	send("\x1b")
	time.Sleep(200 * time.Millisecond)
	waitFor(t, "AgentBox/Dashboard")

	// Plain letters switch between the main sections.
	send("r")
	waitFor(t, "AgentBox/Runs")
	send("i")
	waitFor(t, "AgentBox/Images")
	send("g")
	waitFor(t, "AgentBox/Gateways")
	send("p")
	waitFor(t, "AgentBox/Policies")
	send("o")
	waitFor(t, "AgentBox/Profiles")
	send("x")
	waitFor(t, "AgentBox/Doctor")
	send("d")
	waitFor(t, "AgentBox/Dashboard")

	// Tab still reaches the action bar outside the Dashboard.
	send("r")
	waitFor(t, "AgentBox/Runs")
	send("\t")
	time.Sleep(100 * time.Millisecond)
	send("\r")
	waitFor(t, "AgentBox/Runs/Startrun")
	send("\x1b")
	time.Sleep(200 * time.Millisecond)
	waitFor(t, "AgentBox/Runs")
	send("d")
	waitFor(t, "AgentBox/Dashboard")

	// Escape still goes back through navigation history.
	send("\x1b")
	time.Sleep(200 * time.Millisecond)
	waitFor(t, "AgentBox/Doctor")
	send("d")
	waitFor(t, "AgentBox/Dashboard")

	send("q")

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
