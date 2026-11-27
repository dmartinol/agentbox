package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmartinol/agentbox/internal/app"
)

const orchestratorGateHeadSHA = "0123456789abcdef0123456789abcdef01234567"

func TestOrchestratorGate(t *testing.T) {
	dir := t.TempDir()
	prBodyPath := filepath.Join(dir, "pr-body.md")
	invalidPRBodyPath := filepath.Join(dir, "invalid-pr-body.md")
	issueBodyPath := filepath.Join(dir, "issue-body.md")
	if err := os.WriteFile(prBodyPath, []byte(validOrchestratorGatePRBody()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(invalidPRBodyPath, []byte(strings.Replace(validOrchestratorGatePRBody(), "<!-- orchestrator-workflow: {\"workflow\":\"workflow\",\"commit\":\""+orchestratorGateHeadSHA+"\"} -->\n\n", "", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(issueBodyPath, []byte("## Acceptance criteria\n\n- [ ] Add the gate.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		args      []string
		wantOut   string
		wantError string
	}{
		{
			name:    "pr-body success",
			args:    []string{"gate", "pr-body", "--pr-body", prBodyPath, "--issue-body", issueBodyPath, "--issue", "151"},
			wantOut: "pr-body: valid",
		},
		{
			name:    "pr-merge success",
			args:    []string{"gate", "pr-merge", "--pr-body", prBodyPath, "--head", orchestratorGateHeadSHA},
			wantOut: "pr-merge: valid",
		},
		{
			name:      "invalid pr-body",
			args:      []string{"gate", "pr-body", "--pr-body", invalidPRBodyPath, "--issue-body", issueBodyPath, "--issue", "151"},
			wantError: "missing provenance marker",
		},
		{
			name:      "missing pr-body input flag",
			args:      []string{"gate", "pr-body", "--issue-body", issueBodyPath, "--issue", "151"},
			wantError: "orchestrator gate pr-body requires --pr-body",
		},
		{
			name:      "missing issue-body input flag",
			args:      []string{"gate", "pr-body", "--pr-body", prBodyPath, "--issue", "151"},
			wantError: "orchestrator gate pr-body requires --issue-body",
		},
		{
			name:      "missing issue number flag",
			args:      []string{"gate", "pr-body", "--pr-body", prBodyPath, "--issue-body", issueBodyPath},
			wantError: "orchestrator gate pr-body requires --issue",
		},
		{
			name:      "missing pr-merge head flag",
			args:      []string{"gate", "pr-merge", "--pr-body", prBodyPath},
			wantError: "orchestrator gate pr-merge requires --head",
		},
		{
			name:      "unknown gate",
			args:      []string{"gate", "unknown"},
			wantError: `unknown orchestrator gate command "unknown"`,
		},
		{
			name:      "positional argument",
			args:      []string{"gate", "pr-body", "--pr-body", prBodyPath, "--issue-body", issueBodyPath, "--issue", "151", "extra"},
			wantError: "orchestrator gate pr-body takes no positional arguments",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a := &app.App{Store: app.Store{Base: t.TempDir()}}
			output, err := captureStdout(t, func() error {
				return orchestratorCmd(a, test.args)
			})
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("orchestratorCmd() error = %v", err)
				}
				if output != test.wantOut {
					t.Fatalf("output = %q, want %q", output, test.wantOut)
				}
				return
			}
			if err == nil || err.Error() != test.wantError {
				t.Fatalf("orchestratorCmd() error = %v, want %q", err, test.wantError)
			}
			if output != "" {
				t.Fatalf("failure output = %q, want empty", output)
			}
		})
	}
}

type orchestratorGateRunner struct {
	output   []byte
	commands [][]string
}

func (runner *orchestratorGateRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	runner.commands = append(runner.commands, append([]string{name}, args...))
	return runner.output, nil
}

func (runner *orchestratorGateRunner) Stream(_ context.Context, _ io.Reader, _, _ io.Writer, _ string, _ ...string) error {
	return nil
}

func TestOrchestratorGatePRBodyReviewedHead(t *testing.T) {
	dir := t.TempDir()
	prBodyPath := filepath.Join(dir, "pr-body.md")
	issueBodyPath := filepath.Join(dir, "issue-body.md")
	if err := os.WriteFile(prBodyPath, []byte(validOrchestratorGatePRBody()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(issueBodyPath, []byte("## Acceptance criteria\n\n- [ ] Add the gate.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &orchestratorGateRunner{output: []byte("1\t2\tfile.txt\n")}
	application := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: runner}
	output, err := captureStdout(t, func() error {
		return orchestratorCmd(application, []string{
			"gate", "pr-body", "--pr-body", prBodyPath, "--issue-body", issueBodyPath, "--issue", "151",
			"--head", orchestratorGateHeadSHA, "--base", "main", "--repo", "/repo",
		})
	})
	if err != nil {
		t.Fatalf("orchestratorCmd() error = %v", err)
	}
	if output != "pr-body: valid" {
		t.Fatalf("output = %q", output)
	}
	wantCommand := []string{"git", "-C", "/repo", "diff", "--numstat", "main.." + orchestratorGateHeadSHA}
	if len(runner.commands) != 1 || strings.Join(runner.commands[0], " ") != strings.Join(wantCommand, " ") {
		t.Fatalf("DiffStat command = %#v, want %#v", runner.commands, wantCommand)
	}

	stale := strings.Replace(validOrchestratorGatePRBody(), "| Focused | go test ./... | pass | "+orchestratorGateHeadSHA+" |", "| Focused | go test ./... | pass | fedcba9876543210fedcba9876543210fedcba98 |", 1)
	if err := os.WriteFile(prBodyPath, []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	err = orchestratorCmd(application, []string{
		"gate", "pr-body", "--pr-body", prBodyPath, "--issue-body", issueBodyPath, "--issue", "151",
		"--head", orchestratorGateHeadSHA,
	})
	wantError := "Evidence Focused row: Commit \"fedcba9876543210fedcba9876543210fedcba98\" does not name reviewed head " + orchestratorGateHeadSHA + "; use the full SHA or an unambiguous prefix of at least 7 hexadecimal characters"
	if err == nil || err.Error() != wantError {
		t.Fatalf("orchestratorCmd() error = %v, want %q", err, wantError)
	}

	err = orchestratorCmd(application, []string{
		"gate", "pr-body", "--pr-body", prBodyPath, "--issue-body", issueBodyPath, "--issue", "151", "--base", "main",
	})
	if err == nil || err.Error() != "orchestrator gate pr-body --base also requires --head" {
		t.Fatalf("orchestratorCmd() base-without-head error = %v", err)
	}
}

func TestOrchestratorGateDocsNameTheGateFlags(t *testing.T) {
	docs, err := os.ReadFile(filepath.Join("..", "..", "docs", "_cli", "orchestrator.md"))
	if err != nil {
		t.Fatal(err)
	}
	flat := strings.Join(strings.Fields(string(docs)), " ")
	for _, want := range []string{
		"- gate pr-body",
		"- gate pr-merge",
		"## gate pr-body",
		"## gate pr-merge",
		"| `--head` |",
		"| `--base` |",
		"| `--repo` |",
		"| `--reviewed-body` |",
		"| `--reviewed-body-sha256` |",
	} {
		if !strings.Contains(flat, want) {
			t.Fatalf("docs/_cli/orchestrator.md is missing %q", want)
		}
	}
}

func TestOrchestratorGateReviewedBody(t *testing.T) {
	dir := t.TempDir()
	currentBody := validOrchestratorGatePRBody()
	reviewedBody := strings.Replace(currentBody, "| Verify | agentbox-verifier | gates pass at "+orchestratorGateHeadSHA+" | "+orchestratorGateHeadSHA+" |", "| Verify | agentbox-verifier | pending | pending |", 1)
	reviewedBody = strings.Replace(reviewedBody, "| Review | agentbox-reviewer | approve | "+orchestratorGateHeadSHA+" |", "| Review | agentbox-reviewer | pending | pending |", 1)

	currentPath := filepath.Join(dir, "current.md")
	currentExtraNewlinePath := filepath.Join(dir, "current-extra-newline.md")
	reviewedPath := filepath.Join(dir, "reviewed.md")
	tamperedPath := filepath.Join(dir, "tampered.md")
	mismatchPath := filepath.Join(dir, "mismatch.md")
	if err := os.WriteFile(currentPath, []byte(currentBody), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(currentExtraNewlinePath, []byte(currentBody+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reviewedPath, []byte(reviewedBody), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tamperedPath, []byte(reviewedBody+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mismatchBody := strings.Replace(reviewedBody, "## Acceptance criteria", "## Changed criteria", 1)
	if err := os.WriteFile(mismatchPath, []byte(mismatchBody), 0o600); err != nil {
		t.Fatal(err)
	}

	reviewedDigest := sha256.Sum256([]byte(reviewedBody))
	reviewedSHA := hex.EncodeToString(reviewedDigest[:])
	mismatchDigest := sha256.Sum256([]byte(mismatchBody))
	mismatchSHA := hex.EncodeToString(mismatchDigest[:])
	tests := []struct {
		name      string
		args      []string
		wantError string
	}{
		{
			name: "match",
			args: []string{"gate", "pr-merge", "--pr-body", currentPath, "--head", orchestratorGateHeadSHA, "--reviewed-body", reviewedPath, "--reviewed-body-sha256", reviewedSHA},
		},
		{
			name: "extra trailing current newline",
			args: []string{"gate", "pr-merge", "--pr-body", currentExtraNewlinePath, "--head", orchestratorGateHeadSHA, "--reviewed-body", reviewedPath, "--reviewed-body-sha256", reviewedSHA},
		},
		{
			name:      "tampered reviewed file",
			args:      []string{"gate", "pr-merge", "--pr-body", currentPath, "--head", orchestratorGateHeadSHA, "--reviewed-body", tamperedPath, "--reviewed-body-sha256", reviewedSHA},
			wantError: "pr-merge: reviewed body digest mismatch",
		},
		{
			name:      "canonical mismatch",
			args:      []string{"gate", "pr-merge", "--pr-body", currentPath, "--head", orchestratorGateHeadSHA, "--reviewed-body", mismatchPath, "--reviewed-body-sha256", mismatchSHA},
			wantError: "pr-merge: body differs at headings and prose",
		},
		{
			name:      "legacy review stage entry",
			args:      []string{"gate", "pr-merge", "--pr-body", currentPath, "--head", orchestratorGateHeadSHA, "--reviewed-body", reviewedPath},
			wantError: "pr-merge: review stage entry is missing review_content_sha256; rerun review to upgrade the digest contract",
		},
		{
			name:      "invalid digest",
			args:      []string{"gate", "pr-merge", "--pr-body", currentPath, "--head", orchestratorGateHeadSHA, "--reviewed-body", reviewedPath, "--reviewed-body-sha256", strings.ToUpper(reviewedSHA)},
			wantError: "orchestrator gate pr-merge requires --reviewed-body-sha256 to be 64 lowercase hexadecimal characters",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a := &app.App{Store: app.Store{Base: t.TempDir()}}
			output, err := captureStdout(t, func() error {
				return orchestratorCmd(a, test.args)
			})
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("orchestratorCmd() error = %v", err)
				}
				if output != "pr-merge: valid" {
					t.Fatalf("output = %q", output)
				}
				return
			}
			if err == nil || err.Error() != test.wantError {
				t.Fatalf("orchestratorCmd() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func validOrchestratorGatePRBody() string {
	body := `<!-- orchestrator-workflow: {"workflow":"workflow","commit":"<head>"} -->

Closes #151

## Acceptance criteria

- [ ] Add the gate.

## At a glance

| Field | Value |
| --- | --- |
| Size | 1 files · 1 commits · +1/−2 |

## Evidence

| Gate | Command | Result | Commit |
| --- | --- | --- | --- |
| Focused | go test ./... | pass | <head> |
| Full | make test | pass | <head> |

**Independent checks**

| Stage | Role | Result | Evidence |
| --- | --- | --- | --- |
| Verify | agentbox-verifier | gates pass at <head> | <head> |
| Review | agentbox-reviewer | approve | <head> |
`
	return strings.ReplaceAll(body, "<head>", orchestratorGateHeadSHA)
}
