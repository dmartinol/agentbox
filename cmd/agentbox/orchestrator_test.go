package main

import (
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
