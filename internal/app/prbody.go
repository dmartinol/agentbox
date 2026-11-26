package app

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	independentChecksHeader    = "| Stage | Role | Result | Evidence |"
	independentChecksSeparator = "| --- | --- | --- | --- |"
)

var acceptanceCriterionPattern = regexp.MustCompile(`^\s*-\s+\[[ xX]\]\s+(.*)$`)
var commitReferencePattern = regexp.MustCompile(`(?i)(?:/commit/([0-9a-f]{40})(?:$|[^0-9a-f])|(?:^|[^0-9a-f])([0-9a-f]{40})(?:$|[^0-9a-f]))`)

type independentCheckRow struct {
	stage    string
	role     string
	result   string
	evidence string
}

func ValidateReviewPRBody(prBody, issueBody string, issueNumber int) error {
	if !hasProvenanceMarker(prBody) {
		return fmt.Errorf("missing provenance marker")
	}
	if _, err := parseIndependentChecks(prBody); err != nil {
		return err
	}
	issueCriteria, err := acceptanceCriteria(issueBody, "issue")
	if err != nil {
		return err
	}
	prCriteria, err := acceptanceCriteria(prBody, "pull request")
	if err != nil {
		return err
	}
	for index := 0; index < len(issueCriteria) || index < len(prCriteria); index++ {
		issueCriterion := ""
		if index < len(issueCriteria) {
			issueCriterion = issueCriteria[index]
		}
		prCriterion := ""
		if index < len(prCriteria) {
			prCriterion = prCriteria[index]
		}
		if issueCriterion != prCriterion {
			unmatched := prCriterion
			if unmatched == "" {
				unmatched = issueCriterion
			}
			return fmt.Errorf("acceptance criteria mismatch: first unmatched criterion: %s", unmatched)
		}
	}
	if !hasClosesLine(prBody, issueNumber) {
		return fmt.Errorf("missing Closes #%d line", issueNumber)
	}
	return nil
}

func ValidateReviewChecks(prBody, mergedHeadSHA string) error {
	rows, err := parseIndependentChecks(prBody)
	if err != nil {
		return err
	}
	for _, stage := range []string{"Verify", "Review"} {
		row := rows[stage]
		if normalizeTableCell(row.result) == "pending" {
			return fmt.Errorf("Independent checks %s row: Result is pending", stage)
		}
		refs := commitReferences(row.evidence)
		if !isFullCommitSHA(mergedHeadSHA) && containsExactToken(row.evidence, mergedHeadSHA) {
			refs = append(refs, mergedHeadSHA)
		}
		hasMergedHead := false
		for _, ref := range refs {
			if strings.EqualFold(ref, mergedHeadSHA) {
				hasMergedHead = true
				break
			}
		}
		if !hasMergedHead {
			return fmt.Errorf("Independent checks %s row: Evidence does not contain merged head SHA %s", stage, mergedHeadSHA)
		}
		for _, ref := range refs {
			if !strings.EqualFold(ref, mergedHeadSHA) {
				return fmt.Errorf("Independent checks %s row: Evidence links commit %s instead of merged head SHA %s", stage, ref, mergedHeadSHA)
			}
		}
	}
	return nil
}

func commitReferences(evidence string) []string {
	var refs []string
	seen := make(map[string]struct{})
	add := func(ref string) {
		key := strings.ToLower(ref)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		refs = append(refs, ref)
	}
	for _, match := range commitReferencePattern.FindAllStringSubmatch(evidence, -1) {
		ref := match[1]
		if ref == "" {
			ref = match[2]
		}
		add(ref)
	}
	return refs
}

func isFullCommitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, char := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return false
		}
	}
	return true
}

func containsExactToken(evidence, value string) bool {
	for _, field := range strings.Fields(evidence) {
		if strings.EqualFold(strings.Trim(field, "`[]()<>/.,;:"), value) {
			return true
		}
	}
	return false
}

func hasProvenanceMarker(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "<!-- orchestrator-workflow:") && strings.HasSuffix(line, "-->") {
			return true
		}
	}
	return false
}

func parseIndependentChecks(body string) (map[string]independentCheckRow, error) {
	lines := strings.Split(body, "\n")
	labelIndex := -1
	for index, line := range lines {
		if strings.Contains(line, "**Independent checks**") {
			labelIndex = index
			break
		}
	}
	if labelIndex < 0 {
		return nil, fmt.Errorf("missing Independent checks table")
	}

	index := labelIndex + 1
	for index < len(lines) && strings.TrimSpace(lines[index]) == "" {
		index++
	}
	if index >= len(lines) || lines[index] != independentChecksHeader {
		return nil, fmt.Errorf("invalid Independent checks table shape: expected %s", independentChecksHeader)
	}
	index++
	if index >= len(lines) || lines[index] != independentChecksSeparator {
		return nil, fmt.Errorf("invalid Independent checks table shape: expected %s", independentChecksHeader)
	}
	index++

	var rows []independentCheckRow
	for index < len(lines) && strings.TrimSpace(lines[index]) != "" {
		cells, ok := splitTableRow(lines[index])
		if !ok || len(cells) != 4 {
			return nil, fmt.Errorf("invalid Independent checks table shape: expected %s", independentChecksHeader)
		}
		row := independentCheckRow{
			stage:    cells[0],
			role:     cells[1],
			result:   cells[2],
			evidence: cells[3],
		}
		if row.stage != "Verify" && row.stage != "Review" {
			return nil, fmt.Errorf("invalid Independent checks table shape: expected %s", independentChecksHeader)
		}
		rows = append(rows, row)
		index++
	}
	if len(rows) > 2 {
		return nil, fmt.Errorf("invalid Independent checks table shape: expected %s", independentChecksHeader)
	}

	parsed := make(map[string]independentCheckRow)
	for _, row := range rows {
		parsed[row.stage] = row
	}
	for _, stage := range []string{"Verify", "Review"} {
		if _, ok := parsed[stage]; !ok {
			return nil, fmt.Errorf("missing Independent checks %s row", stage)
		}
	}
	return parsed, nil
}

func splitTableRow(line string) ([]string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return nil, false
	}
	cells := strings.Split(trimmed[1:len(trimmed)-1], "|")
	for index := range cells {
		cells[index] = strings.TrimSpace(cells[index])
	}
	return cells, true
}

func acceptanceCriteria(body, source string) ([]string, error) {
	var criteria []string
	inSection := false
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "## Acceptance criteria" {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(strings.TrimSpace(line), "## ") {
			break
		}
		if !inSection {
			continue
		}
		match := acceptanceCriterionPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		criteria = append(criteria, normalizeCriterion(match[1]))
	}
	if len(criteria) == 0 {
		return nil, fmt.Errorf("missing %s acceptance criteria list", source)
	}
	return criteria, nil
}

func normalizeCriterion(criterion string) string {
	return strings.ToLower(strings.Join(strings.Fields(criterion), " "))
}

func normalizeTableCell(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func hasClosesLine(body string, issueNumber int) bool {
	want := fmt.Sprintf("Closes #%d", issueNumber)
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimRight(line, " ") == want {
			return true
		}
	}
	return false
}
