package app

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
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
| Verify | agentbox-verifier | gates pass at 0123456789abcdef0123456789abcdef01234567 | 0123456789abcdef0123456789abcdef01234567 |
| Review | agentbox-reviewer | approve | 0123456789abcdef0123456789abcdef01234567 |
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
			prBody:  strings.Replace(validReviewPRBody(), "| Verify | agentbox-verifier | gates pass at 0123456789abcdef0123456789abcdef01234567 | 0123456789abcdef0123456789abcdef01234567 |", "| Verify | agentbox-verifier | gates pass at 0123456789abcdef0123456789abcdef01234567 |", 1),
			issue:   reviewIssueBody,
			wantErr: "invalid Independent checks table shape: expected | Stage | Role | Result | Evidence |",
		},
		{
			name:    "missing verify row",
			prBody:  strings.Replace(validReviewPRBody(), "| Verify | agentbox-verifier | gates pass at 0123456789abcdef0123456789abcdef01234567 | 0123456789abcdef0123456789abcdef01234567 |\n", "", 1),
			issue:   reviewIssueBody,
			wantErr: "missing Independent checks Verify row",
		},
		{
			name:    "missing review row",
			prBody:  strings.Replace(validReviewPRBody(), "| Review | agentbox-reviewer | approve | 0123456789abcdef0123456789abcdef01234567 |\n", "", 1),
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
			prBody:  "<!-- orchestrator-workflow: {\"workflow\":\"workflow\"} -->\n\nCloses #151\n\n## Summary\n\nNo criteria here.\n\n**Independent checks**\n\n| Stage | Role | Result | Evidence |\n| --- | --- | --- | --- |\n| Verify | agentbox-verifier | gates pass at 0123456789abcdef0123456789abcdef01234567 | 0123456789abcdef0123456789abcdef01234567 |\n| Review | agentbox-reviewer | approve | 0123456789abcdef0123456789abcdef01234567 |\n",
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
		{name: "bare merged head SHA", prBody: validReviewPRBody()},
		{
			name:    "verify result pending",
			prBody:  strings.Replace(validReviewPRBody(), "gates pass at 0123456789abcdef0123456789abcdef01234567", "pending", 1),
			wantErr: "Independent checks Verify row: Result is pending",
		},
		{
			name:    "review result pending",
			prBody:  strings.Replace(validReviewPRBody(), "| Review | agentbox-reviewer | approve |", "| Review | agentbox-reviewer | Pending |", 1),
			wantErr: "Independent checks Review row: Result is pending",
		},
		{
			name:    "verify evidence wrong head",
			prBody:  strings.Replace(validReviewPRBody(), "| Verify | agentbox-verifier | gates pass at 0123456789abcdef0123456789abcdef01234567 | 0123456789abcdef0123456789abcdef01234567 |", "| Verify | agentbox-verifier | gates pass at 0123456789abcdef0123456789abcdef01234567 | other-sha |", 1),
			wantErr: "Independent checks Verify row: Evidence does not contain merged head SHA 0123456789abcdef0123456789abcdef01234567",
		},
		{
			name:    "review evidence wrong head",
			prBody:  strings.Replace(validReviewPRBody(), "| Review | agentbox-reviewer | approve | 0123456789abcdef0123456789abcdef01234567 |", "| Review | agentbox-reviewer | approve | other-sha |", 1),
			wantErr: "Independent checks Review row: Evidence does not contain merged head SHA 0123456789abcdef0123456789abcdef01234567",
		},
		{
			name: "verify evidence comment and merged head commit link",
			prBody: strings.Replace(
				validReviewPRBody(),
				"| Verify | agentbox-verifier | gates pass at 0123456789abcdef0123456789abcdef01234567 | 0123456789abcdef0123456789abcdef01234567 |",
				"| Verify | agentbox-verifier | gates pass at 0123456789abcdef0123456789abcdef01234567 | [commit](https://example.test/repo/commit/0123456789abcdef0123456789abcdef01234567) · [comment](https://example.test/comment) |",
				1,
			),
		},
		{
			name: "verify evidence wrong commit link with merged head text",
			prBody: strings.Replace(
				validReviewPRBody(),
				"| Verify | agentbox-verifier | gates pass at 0123456789abcdef0123456789abcdef01234567 | 0123456789abcdef0123456789abcdef01234567 |",
				"| Verify | agentbox-verifier | gates pass at 0123456789abcdef0123456789abcdef01234567 | [commit](https://example.test/repo/commit/ffffffffffffffffffffffffffffffffffffffff) 0123456789abcdef0123456789abcdef01234567 |",
				1,
			),
			wantErr: "Independent checks Verify row: Evidence links commit ffffffffffffffffffffffffffffffffffffffff instead of merged head SHA 0123456789abcdef0123456789abcdef01234567",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateReviewChecks(test.prBody, "0123456789abcdef0123456789abcdef01234567")
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

func TestCanonicalReviewPRBody(t *testing.T) {
	base := canonicalReviewPRBodyFixture()
	rowOnlyChange := strings.Replace(base, "| Verify | verifier  | pending | pending |", "| Verify | verifier  | gates pass | [result](https://example.test/result) |", 1)
	tests := []struct {
		name    string
		current string
		equal   bool
	}{
		{name: "owned row only", current: rowOnlyChange, equal: true},
		{name: "heading", current: strings.Replace(base, "## Summary", "## Changed summary", 1), equal: false},
		{name: "provenance", current: strings.Replace(base, `"workflow":"workflow"`, `"workflow":"changed"`, 1), equal: false},
		{name: "footer", current: strings.Replace(base, "Reviewed body", "Changed body", 1), equal: false},
		{name: "closes", current: strings.Replace(base, "Closes #151", "Closes #152", 1), equal: false},
		{name: "other table cell", current: strings.Replace(base, "| Verify | verifier  | pending | pending |", "| Verify | other-role  | pending | pending |", 1), equal: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reviewedDigest, err := CanonicalReviewPRBodySHA256(base)
			if err != nil {
				t.Fatal(err)
			}
			currentDigest, err := CanonicalReviewPRBodySHA256(test.current)
			if err != nil {
				t.Fatal(err)
			}
			if (reviewedDigest == currentDigest) != test.equal {
				t.Fatalf("CanonicalReviewPRBodySHA256() equality = %v, want %v", reviewedDigest == currentDigest, test.equal)
			}
		})
	}

	canonical, err := CanonicalReviewPRBody(base)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(base, "| Verify | verifier  | pending | pending |", "| Verify | verifier  | <orchestrator-owned> | <orchestrator-owned> |", 1)
	want = strings.Replace(want, "| Review | reviewer | pending | pending |", "| Review | reviewer | <orchestrator-owned> | <orchestrator-owned> |", 1)
	want = strings.TrimRight(want, "\n")
	if canonical != want {
		t.Fatalf("CanonicalReviewPRBody() = %q, want %q", canonical, want)
	}
}

func TestCompareCanonicalReviewPRBodies(t *testing.T) {
	base := canonicalReviewPRBodyFixture()
	tests := []struct {
		name    string
		current string
		wantErr string
	}{
		{name: "provenance marker", current: strings.Replace(base, `"workflow":"workflow"`, `"workflow":"changed"`, 1), wantErr: "body differs at provenance marker"},
		{name: "headings and prose", current: strings.Replace(base, "Keep the reviewed body stable.", "Change the reviewed body.", 1), wantErr: "body differs at headings and prose"},
		{name: "acceptance criteria", current: strings.Replace(base, "- [ ] Keep the canonical digest stable.", "- [ ] Change the canonical digest.", 1), wantErr: "body differs at acceptance criteria"},
		{name: "other table rows", current: strings.Replace(base, "| Verify | verifier  | pending | pending |", "| Verify | other-role  | pending | pending |", 1), wantErr: "body differs at other table rows"},
		{name: "footer", current: strings.Replace(base, "Reviewed body", "Changed body", 1), wantErr: "body differs at footer"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := CompareCanonicalReviewPRBodies(base, test.current)
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("CompareCanonicalReviewPRBodies() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestObservedReviewBodyDigests(t *testing.T) {
	compressed := make([]byte, base64.StdEncoding.DecodedLen(len(observedReviewBodyGzipBase64)))
	compressedLength, err := base64.StdEncoding.Decode(compressed, []byte(observedReviewBodyGzipBase64))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed[:compressedLength]))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if digest := sha256.Sum256(body); hex.EncodeToString(digest[:]) != "feac9a544f2b186635d7ae5ff43579cac61d4440c0eb297cf568a1f9ec66d586" {
		t.Fatalf("observed body SHA-256 = %s", hex.EncodeToString(digest[:]))
	}
	postFill := fillObservedReviewRow(t, string(body))
	if digest := sha256.Sum256([]byte(postFill)); hex.EncodeToString(digest[:]) == "feac9a544f2b186635d7ae5ff43579cac61d4440c0eb297cf568a1f9ec66d586" {
		t.Fatal("post-fill body SHA-256 unexpectedly matches the reviewed body")
	}
	reviewedDigest, err := CanonicalReviewPRBodySHA256(string(body))
	if err != nil {
		t.Fatal(err)
	}
	postFillDigest, err := CanonicalReviewPRBodySHA256(postFill)
	if err != nil {
		t.Fatal(err)
	}
	if reviewedDigest != postFillDigest {
		t.Fatalf("canonical digest changed after filling owned cells: %s != %s", reviewedDigest, postFillDigest)
	}
	canonical, err := CanonicalReviewPRBody(string(body))
	if err != nil {
		t.Fatal(err)
	}
	idempotent, err := CanonicalReviewPRBody(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if canonical != idempotent {
		t.Fatal("CanonicalReviewPRBody() is not idempotent")
	}
	for _, trailingNewlines := range []string{"\n", "\n\n\n"} {
		trailingNewlineDigest, err := CanonicalReviewPRBodySHA256(string(body) + trailingNewlines)
		if err != nil {
			t.Fatal(err)
		}
		if trailingNewlineDigest != reviewedDigest {
			t.Fatalf("canonical digest changed after adding trailing newlines %q", trailingNewlines)
		}
	}
	trailingSpaceDigest, err := CanonicalReviewPRBodySHA256(string(body) + " ")
	if err != nil {
		t.Fatal(err)
	}
	if trailingSpaceDigest == reviewedDigest {
		t.Fatal("canonical digest ignored a trailing space")
	}
	if err := CompareCanonicalReviewPRBodies(string(body), string(body)+" "); err == nil || err.Error() != "body differs at footer" {
		t.Fatalf("CompareCanonicalReviewPRBodies() trailing-space error = %v, want body differs at footer", err)
	}
}

func fillObservedReviewRow(t *testing.T, body string) string {
	t.Helper()
	lines := strings.Split(body, "\n")
	for index, line := range lines {
		if !strings.HasPrefix(line, "| Review |") {
			continue
		}
		cells := splitRawTableRow(line)
		cells[2] = "approve · 0 findings"
		cells[3] = "review verdict at 4db7f15b22367356b9e067292dce3c9a8dbc9d84"
		lines[index] = renderTableRow(line, cells)
		return strings.Join(lines, "\n")
	}
	t.Fatal("observed body has no Review row")
	return ""
}

const observedReviewBodyGzipBase64 = `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
`

func canonicalReviewPRBodyFixture() string {
	return "<!-- orchestrator-workflow: {\"workflow\":\"workflow\"} -->\r\n" +
		"\n" +
		"Closes #151\n" +
		"\n" +
		"## Summary\n" +
		"\n" +
		"Keep the reviewed body stable.\n" +
		"\n" +
		"## Acceptance criteria\n" +
		"\n" +
		"- [ ] Keep the canonical digest stable.\n" +
		"\n" +
		"## Details\n" +
		"\n" +
		"**Independent checks**\n" +
		"\n" +
		"| Stage | Role | Result | Evidence |\n" +
		"| --- | --- | --- | --- |\n" +
		"  | Verify | verifier  | pending | pending |\r\n" +
		"| Review | reviewer | pending | pending |\n" +
		"\n" +
		"---\n" +
		"\n" +
		"Reviewed body\n"
}
