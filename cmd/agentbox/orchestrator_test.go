package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmartinol/agentbox/internal/app"
)

func TestOrchestratorGate(t *testing.T) {
	dir := t.TempDir()
	prBodyPath := filepath.Join(dir, "pr-body.md")
	invalidPRBodyPath := filepath.Join(dir, "invalid-pr-body.md")
	issueBodyPath := filepath.Join(dir, "issue-body.md")
	if err := os.WriteFile(prBodyPath, []byte(validOrchestratorGatePRBody()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(invalidPRBodyPath, []byte(strings.Replace(validOrchestratorGatePRBody(), "<!-- orchestrator-workflow: {\"workflow\":\"workflow\"} -->\n\n", "", 1)), 0o600); err != nil {
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
			args:    []string{"gate", "pr-merge", "--pr-body", prBodyPath, "--head", "abc123"},
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

func TestOrchestratorGateReviewedBody(t *testing.T) {
	dir := t.TempDir()
	currentBody := validOrchestratorGatePRBody()
	reviewedBody := strings.ReplaceAll(currentBody, "| abc123 |", "| pending |")
	reviewedBody = strings.Replace(reviewedBody, "gates pass at abc123", "pending", 1)
	reviewedBody = strings.Replace(reviewedBody, "approve", "pending", 1)

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
			args: []string{"gate", "pr-merge", "--pr-body", currentPath, "--head", "abc123", "--reviewed-body", reviewedPath, "--reviewed-body-sha256", reviewedSHA},
		},
		{
			name: "extra trailing current newline",
			args: []string{"gate", "pr-merge", "--pr-body", currentExtraNewlinePath, "--head", "abc123", "--reviewed-body", reviewedPath, "--reviewed-body-sha256", reviewedSHA},
		},
		{
			name:      "tampered reviewed file",
			args:      []string{"gate", "pr-merge", "--pr-body", currentPath, "--head", "abc123", "--reviewed-body", tamperedPath, "--reviewed-body-sha256", reviewedSHA},
			wantError: "pr-merge: reviewed body digest mismatch",
		},
		{
			name:      "canonical mismatch",
			args:      []string{"gate", "pr-merge", "--pr-body", currentPath, "--head", "abc123", "--reviewed-body", mismatchPath, "--reviewed-body-sha256", mismatchSHA},
			wantError: "pr-merge: body differs at headings and prose",
		},
		{
			name:      "legacy review stage entry",
			args:      []string{"gate", "pr-merge", "--pr-body", currentPath, "--head", "abc123", "--reviewed-body", reviewedPath},
			wantError: "pr-merge: review stage entry is missing review_content_sha256; rerun review to upgrade the digest contract",
		},
		{
			name:      "invalid digest",
			args:      []string{"gate", "pr-merge", "--pr-body", currentPath, "--head", "abc123", "--reviewed-body", reviewedPath, "--reviewed-body-sha256", strings.ToUpper(reviewedSHA)},
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
	return `<!-- orchestrator-workflow: {"workflow":"workflow"} -->

Closes #151

## Acceptance criteria

- [ ] Add the gate.

**Independent checks**

| Stage | Role | Result | Evidence |
| --- | --- | --- | --- |
| Verify | agentbox-verifier | gates pass at abc123 | abc123 |
| Review | agentbox-reviewer | approve | abc123 |
`
}
