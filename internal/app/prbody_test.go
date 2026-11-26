package app

import (
	"strings"
	"testing"
)

const reviewIssueBody = `## Acceptance criteria

- [ ] Add the local gate.
- [x] Keep the reviewer offline.
`

func validReviewPRBody() string {
	return `<!-- orchestrator-workflow: {"workflow":"workflow"} -->

Closes #151

## Acceptance criteria

- [ ] Add   the LOCAL gate.
- [X] Keep the reviewer offline.

**Independent checks** — the orchestrator fills these in.

| Stage | Role | Result | Evidence |
| --- | --- | --- | --- |
| Verify | agentbox-verifier | gates pass at abc123 | abc123 |
| Review | agentbox-reviewer | approve | abc123 |
`
}

func TestValidateReviewPRBody(t *testing.T) {
	tests := []struct {
		name    string
		prBody  string
		issue   string
		wantErr string
	}{
		{name: "valid", prBody: validReviewPRBody(), issue: reviewIssueBody},
		{
			name:    "missing provenance marker",
			prBody:  strings.Replace(validReviewPRBody(), "<!-- orchestrator-workflow: {\"workflow\":\"workflow\"} -->\n\n", "", 1),
			issue:   reviewIssueBody,
			wantErr: "missing provenance marker",
		},
		{
			name:    "missing table",
			prBody:  strings.Replace(validReviewPRBody(), "**Independent checks** — the orchestrator fills these in.", "Independent checks", 1),
			issue:   reviewIssueBody,
			wantErr: "missing Independent checks table",
		},
		{
			name:    "invalid header",
			prBody:  strings.Replace(validReviewPRBody(), "| Stage | Role | Result | Evidence |", "| Stage | Role | Result | Notes |", 1),
			issue:   reviewIssueBody,
			wantErr: "invalid Independent checks table shape: expected | Stage | Role | Result | Evidence |",
		},
		{
			name:    "invalid separator",
			prBody:  strings.Replace(validReviewPRBody(), "| --- | --- | --- | --- |", "| --- | --- | --- |", 1),
			issue:   reviewIssueBody,
			wantErr: "invalid Independent checks table shape: expected | Stage | Role | Result | Evidence |",
		},
		{
			name:    "invalid data row",
			prBody:  strings.Replace(validReviewPRBody(), "| Verify | agentbox-verifier | gates pass at abc123 | abc123 |", "| Verify | agentbox-verifier | gates pass at abc123 |", 1),
			issue:   reviewIssueBody,
			wantErr: "invalid Independent checks table shape: expected | Stage | Role | Result | Evidence |",
		},
		{
			name:    "missing verify row",
			prBody:  strings.Replace(validReviewPRBody(), "| Verify | agentbox-verifier | gates pass at abc123 | abc123 |\n", "", 1),
			issue:   reviewIssueBody,
			wantErr: "missing Independent checks Verify row",
		},
		{
			name:    "missing review row",
			prBody:  strings.Replace(validReviewPRBody(), "| Review | agentbox-reviewer | approve | abc123 |\n", "", 1),
			issue:   reviewIssueBody,
			wantErr: "missing Independent checks Review row",
		},
		{
			name:    "missing issue criteria",
			prBody:  validReviewPRBody(),
			issue:   "## Summary\n\nNo criteria here.\n",
			wantErr: "missing issue acceptance criteria list",
		},
		{
			name:    "missing pull request criteria",
			prBody:  "<!-- orchestrator-workflow: {\"workflow\":\"workflow\"} -->\n\nCloses #151\n\n## Summary\n\nNo criteria here.\n\n**Independent checks**\n\n| Stage | Role | Result | Evidence |\n| --- | --- | --- | --- |\n| Verify | agentbox-verifier | gates pass at abc123 | abc123 |\n| Review | agentbox-reviewer | approve | abc123 |\n",
			issue:   reviewIssueBody,
			wantErr: "missing pull request acceptance criteria list",
		},
		{
			name:    "mismatched criteria",
			prBody:  strings.Replace(validReviewPRBody(), "- [ ] Add   the LOCAL gate.", "- [ ] Add a different gate", 1),
			issue:   reviewIssueBody,
			wantErr: "acceptance criteria mismatch: first unmatched criterion: add a different gate",
		},
		{
			name:    "missing closure",
			prBody:  strings.Replace(validReviewPRBody(), "Closes #151\n\n", "", 1),
			issue:   reviewIssueBody,
			wantErr: "missing Closes #151 line",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateReviewPRBody(test.prBody, test.issue, 151)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateReviewPRBody() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("ValidateReviewPRBody() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateReviewChecks(t *testing.T) {
	tests := []struct {
		name    string
		prBody  string
		wantErr string
	}{
		{name: "valid", prBody: validReviewPRBody()},
		{
			name:    "verify result pending",
			prBody:  strings.Replace(validReviewPRBody(), "gates pass at abc123", "pending", 1),
			wantErr: "Independent checks Verify row: Result is pending",
		},
		{
			name:    "review result pending",
			prBody:  strings.Replace(validReviewPRBody(), "| Review | agentbox-reviewer | approve |", "| Review | agentbox-reviewer | Pending |", 1),
			wantErr: "Independent checks Review row: Result is pending",
		},
		{
			name:    "verify evidence wrong head",
			prBody:  strings.Replace(validReviewPRBody(), "| Verify | agentbox-verifier | gates pass at abc123 | abc123 |", "| Verify | agentbox-verifier | gates pass at abc123 | other-sha |", 1),
			wantErr: "Independent checks Verify row: Evidence does not contain merged head SHA abc123",
		},
		{
			name:    "review evidence wrong head",
			prBody:  strings.Replace(validReviewPRBody(), "| Review | agentbox-reviewer | approve | abc123 |", "| Review | agentbox-reviewer | approve | other-sha |", 1),
			wantErr: "Independent checks Review row: Evidence does not contain merged head SHA abc123",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateReviewChecks(test.prBody, "abc123")
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateReviewChecks() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("ValidateReviewChecks() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}
