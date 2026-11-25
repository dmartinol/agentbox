package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const npmStubScript = `#!/bin/sh
attempts=0
if [ -f "$NPM_ATTEMPTS_FILE" ]; then
	read attempts < "$NPM_ATTEMPTS_FILE"
fi
attempts=$((attempts + 1))
printf '%s\n' "$attempts" > "$NPM_ATTEMPTS_FILE"
printf '%s\n' "$NPM_CONFIG_CACHE" > "$NPM_CACHE_FILE"

case "$NPM_STUB_MODE" in
fail-first)
	if [ "$attempts" -eq 1 ]; then
		printf '%s\n' "npm error code EACCES" >&2
		printf '%s\n' "npm error path /npm/_cacache" >&2
		exit 1
	fi
	;;
always-fail)
	printf '%s\n' "npm error code EACCES" >&2
	printf '%s\n' "npm error path /npm/_cacache" >&2
	exit 1
	;;
esac
exit 0
`

type webInstallHarness struct {
	tempDir    string
	attempts   string
	cacheFile  string
	path       string
	helperPath string
	webDir     string
}

func newWebInstallHarness(t *testing.T) *webInstallHarness {
	t.Helper()

	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.Mkdir(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "npm"), []byte(npmStubScript), 0755); err != nil {
		t.Fatal(err)
	}

	return &webInstallHarness{
		tempDir:    tempDir,
		attempts:   filepath.Join(tempDir, "attempts"),
		cacheFile:  filepath.Join(tempDir, "observed-cache"),
		path:       binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		helperPath: filepath.Join("..", "..", "internal", "gui", "web", "scripts", "npm-ci.sh"),
		webDir:     filepath.Join("..", "..", "internal", "gui", "web"),
	}
}

func (h *webInstallHarness) run(t *testing.T, mode, cache string) (string, error) {
	t.Helper()

	command := exec.Command(h.helperPath, h.webDir)
	command.Env = []string{
		"PATH=" + h.path,
		"TMPDIR=" + h.tempDir,
		"NPM_ATTEMPTS_FILE=" + h.attempts,
		"NPM_CACHE_FILE=" + h.cacheFile,
		"NPM_STUB_MODE=" + mode,
	}
	if cache != "" {
		command.Env = append(command.Env, "NPM_CONFIG_CACHE="+cache)
	}

	output, err := command.CombinedOutput()
	return string(output), err
}

func (h *webInstallHarness) attemptCount(t *testing.T) int {
	t.Helper()

	raw, err := os.ReadFile(h.attempts)
	if err != nil {
		t.Fatal(err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestWebInstallNpmCacheHelper(t *testing.T) {
	t.Run("clean success", func(t *testing.T) {
		harness := newWebInstallHarness(t)
		output, err := harness.run(t, "clean", "")
		if err != nil {
			t.Fatalf("web-install failed: %v\n%s", err, output)
		}

		cache := filepath.Join(harness.tempDir, "agentbox-npm-cache")
		if !strings.Contains(output, "web-install: npm cache: "+cache) {
			t.Fatalf("selected cache not reported:\n%s", output)
		}
		observed, err := os.ReadFile(harness.cacheFile)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(observed)) != cache {
			t.Fatalf("npm cache = %q, want %q", strings.TrimSpace(string(observed)), cache)
		}
		if got := harness.attemptCount(t); got != 1 {
			t.Fatalf("npm attempts = %d, want 1", got)
		}
		if strings.Contains(output, "retry=") {
			t.Fatalf("clean success retried:\n%s", output)
		}
	})

	t.Run("forced retry succeeds on second attempt", func(t *testing.T) {
		harness := newWebInstallHarness(t)
		output, err := harness.run(t, "fail-first", "")
		if err != nil {
			t.Fatalf("web-install failed: %v\n%s", err, output)
		}
		if !strings.Contains(output, "retry=1/3") {
			t.Fatalf("first retry not reported:\n%s", output)
		}
		if !strings.Contains(output, "npm error code EACCES") {
			t.Fatalf("first error not reported:\n%s", output)
		}
		if got := harness.attemptCount(t); got != 2 {
			t.Fatalf("npm attempts = %d, want 2", got)
		}
	})

	t.Run("always fail reports all three attempts", func(t *testing.T) {
		harness := newWebInstallHarness(t)
		output, err := harness.run(t, "always-fail", "")
		if err == nil {
			t.Fatalf("web-install unexpectedly succeeded:\n%s", output)
		}
		if !strings.Contains(output, "retry=3/3") {
			t.Fatalf("final retry not reported:\n%s", output)
		}
		if !strings.Contains(output, "npm error code EACCES") {
			t.Fatalf("first error not reported:\n%s", output)
		}
		if got := harness.attemptCount(t); got != 3 {
			t.Fatalf("npm attempts = %d, want 3", got)
		}
	})

	t.Run("unwritable cache fails before npm", func(t *testing.T) {
		harness := newWebInstallHarness(t)
		cache := filepath.Join(harness.tempDir, "not-a-directory")
		if err := os.WriteFile(cache, []byte("not a directory"), 0400); err != nil {
			t.Fatal(err)
		}

		output, err := harness.run(t, "clean", cache)
		if err == nil {
			t.Fatalf("web-install unexpectedly succeeded:\n%s", output)
		}
		if !strings.Contains(output, "cache is not writable: "+cache) {
			t.Fatalf("unwritable cache not reported:\n%s", output)
		}
		if !strings.Contains(output, "select a writable NPM_CONFIG_CACHE") {
			t.Fatalf("remediation not reported:\n%s", output)
		}
		if strings.Contains(output, "retry=") {
			t.Fatalf("unwritable cache entered retry loop:\n%s", output)
		}
		if _, statErr := os.Stat(harness.attempts); statErr == nil {
			t.Fatalf("npm ran before cache validation:\n%s", output)
		}
	})
}
