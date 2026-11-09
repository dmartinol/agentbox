package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/dmartinol/agentbox/internal/app"
)

func captureStdout(t *testing.T, action func() error) (string, error) {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = write
	callErr := action()
	_ = write.Close()
	os.Stdout = previous
	defer read.Close()
	body, err := io.ReadAll(read)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes.TrimSpace(body)), callErr
}

func TestDoctorJSONIsParseable(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	output, err := captureStdout(t, func() error { return dispatch(context.Background(), a, []string{"doctor", "--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var checks []app.Check
	if err := json.Unmarshal([]byte(output), &checks); err != nil || len(checks) < 5 {
		t.Fatalf("doctor output is not a checks JSON array: %s (%v)", output, err)
	}
}

func TestImageBuildPreviewReturnsRecipeAndExactVersionAsJSON(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"image", "build", "--dry-run", "--version", "0.167.0", "codex"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var record app.ImageRecord
	if err := json.Unmarshal([]byte(output), &record); err != nil {
		t.Fatalf("image preview output is invalid JSON: %s (%v)", output, err)
	}
	if record.Agent != "codex" || record.Version != "0.167.0" || record.Tag != "localhost/openshell-codex:0.167.0-r3" {
		t.Fatalf("unexpected image preview: %+v", record)
	}
}

type appfakeRunner struct{}

func (*appfakeRunner) Output(context.Context, string, ...string) ([]byte, error) {
	return []byte(`{}`), nil
}
func (*appfakeRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func TestVersionOutputIsParseable(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	output, err := captureStdout(t, func() error { return dispatch(context.Background(), a, []string{"--version"}) })
	if err != nil {
		t.Fatal(err)
	}
	if output == "" {
		t.Fatal("version output is empty")
	}
}

func TestImageRegistryShowSetClear(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	if err := dispatch(context.Background(), a, []string{"image", "registry", "set", "quay.io/team/agentbox"}); err != nil {
		t.Fatal(err)
	}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"image", "registry", "show", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var registry app.Registry
	if err := json.Unmarshal([]byte(output), &registry); err != nil || registry.Reference != "quay.io/team/agentbox" {
		t.Fatalf("registry output = %s, %v", output, err)
	}
	if err := dispatch(context.Background(), a, []string{"image", "registry", "clear"}); err != nil {
		t.Fatal(err)
	}
	registry, err = a.Registry()
	if err != nil || registry.Reference != "" {
		t.Fatalf("registry after clear = %+v, %v", registry, err)
	}
}

func TestImageBuildHelpReturnsFlagHelp(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	err := dispatch(context.Background(), a, []string{"image", "build", "--help"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("image build --help error = %v; want flag.ErrHelp", err)
	}
}

func TestImageHelpDoesNotRunCommand(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	output, err := captureStdout(t, func() error { return dispatch(context.Background(), a, []string{"image", "--help"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "image build") {
		t.Fatalf("image help output = %q", output)
	}
}
