package tui

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

type taskTestRunner struct{}

func (taskTestRunner) Output(context.Context, string, ...string) ([]byte, error) {
	return []byte(`{"ok":true}`), nil
}

func (taskTestRunner) Stream(_ context.Context, _ io.Reader, out, errOut io.Writer, _ string, _ ...string) error {
	_, _ = io.WriteString(out, "stream stdout\n")
	_, _ = io.WriteString(errOut, "stream stderr\n")
	return nil
}

func TestTaskRunnerCapturesStreamsAndLogsCommands(t *testing.T) {
	var log bytes.Buffer
	runner := taskRunner{base: taskTestRunner{}, out: &log}
	if err := runner.Stream(context.Background(), nil, io.Discard, io.Discard, "podman", "push", "registry.example/team/image:tag"); err != nil {
		t.Fatal(err)
	}
	text := log.String()
	for _, want := range []string{"$ podman push registry.example/team/image:tag", "stream stdout", "stream stderr"} {
		if !strings.Contains(text, want) {
			t.Fatalf("task log missing %q:\n%s", want, text)
		}
	}
}

func TestDisplayCommandRedactsCredentials(t *testing.T) {
	got := displayCommand("openshell", []string{"provider", "create", "--credential", "secret-value", "--token=abc123"})
	if strings.Contains(got, "secret-value") || strings.Contains(got, "abc123") {
		t.Fatalf("secret leaked in command display: %s", got)
	}
	for _, want := range []string{"--credential <redacted>", "--token=<redacted>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("redacted command missing %q: %s", want, got)
		}
	}
}
