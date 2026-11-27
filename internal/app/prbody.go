package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const (
	independentChecksHeader    = "| Stage | Role | Result | Evidence |"
	independentChecksSeparator = "| --- | --- | --- | --- |"
	orchestratorOwnedTableCell = " <orchestrator-owned> "
)

var acceptanceCriterionPattern = regexp.MustCompile(`^\s*-\s+\[[ xX]\]\s+(.*)$`)
var commitReferencePattern = regexp.MustCompile(`(?i)(?:/commit/([0-9a-f]{40})(?:$|[^0-9a-f])|(?:^|[^0-9a-f])([0-9a-f]{40})(?:$|[^0-9a-f]))`)
var reviewPRSizePattern = regexp.MustCompile(`(?i)^\s*(\d+)\s+files?\s+·\s+(\d+)\s+commits?\s+·\s+\+(\d+)\s*/\s*[−-]\s*(\d+)`)

func CanonicalReviewPRBody(body string) (string, error) {
	if _, err := parseIndependentChecks(body); err != nil {
		return "", err
	}
	lines := strings.Split(body, "\n")
	rowIndexes, err := independentCheckDataRowIndexes(lines)
	if err != nil {
		return "", err
	}
	for _, index := range rowIndexes {
		cells := splitRawTableRow(lines[index])
		if strings.TrimSpace(cells[0]) != "Verify" && strings.TrimSpace(cells[0]) != "Review" {
			continue
		}
		cells[2] = orchestratorOwnedTableCell
		cells[3] = orchestratorOwnedTableCell
		lines[index] = renderTableRow(lines[index], cells)
	}
	// The tracker appends a newline on every body write, so EOF newlines are not content.
	return strings.TrimRight(strings.Join(lines, "\n"), "\n"), nil
}

func CanonicalReviewPRBodySHA256(body string) (string, error) {
	canonical, err := CanonicalReviewPRBody(body)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:]), nil
}

func CompareCanonicalReviewPRBodies(reviewed, current string) error {
	reviewedCanonical, err := CanonicalReviewPRBody(reviewed)
	if err != nil {
		return err
	}
	currentCanonical, err := CanonicalReviewPRBody(current)
	if err != nil {
		return err
	}
	for _, section := range canonicalReviewPRBodySections(currentCanonical, reviewedCanonical) {
		if section.reviewed != section.current {
			return fmt.Errorf("body differs at %s", section.name)
		}
	}
	return nil
}

type canonicalReviewPRBodySection struct {
	name     string
	reviewed string
	current  string
}

func canonicalReviewPRBodySections(reviewed, current string) []canonicalReviewPRBodySection {
	reviewedSections := classifyCanonicalReviewPRBody(reviewed)
	currentSections := classifyCanonicalReviewPRBody(current)
	names := []string{"provenance marker", "headings and prose", "acceptance criteria", "other table rows", "footer"}
	sections := make([]canonicalReviewPRBodySection, 0, len(names))
	for _, name := range names {
		sections = append(sections, canonicalReviewPRBodySection{
			name:     name,
			reviewed: reviewedSections[name],
			current:  currentSections[name],
		})
	}
	return sections
}

func classifyCanonicalReviewPRBody(body string) map[string]string {
	lines := strings.Split(body, "\n")
	acceptanceStart, acceptanceEnd := acceptanceCriteriaRange(lines)
	footerStart := finalStandaloneRuleIndex(lines)
	if footerStart < 0 {
		footerStart = len(lines)
	}

	sections := make(map[string][]string)
	for index, line := range lines {
		switch {
		case isProvenanceMarkerLine(line):
			sections["provenance marker"] = append(sections["provenance marker"], line)
		case index >= acceptanceStart && index < acceptanceEnd:
			sections["acceptance criteria"] = append(sections["acceptance criteria"], line)
		case index >= footerStart:
			sections["footer"] = append(sections["footer"], line)
		case isTableShapedLine(line):
			sections["other table rows"] = append(sections["other table rows"], line)
		default:
			sections["headings and prose"] = append(sections["headings and prose"], line)
		}
	}

	rendered := make(map[string]string, len(sections))
	for name, sectionLines := range sections {
		rendered[name] = strings.Join(sectionLines, "\n")
	}
	for _, name := range []string{"provenance marker", "headings and prose", "acceptance criteria", "other table rows", "footer"} {
		if _, ok := rendered[name]; !ok {
			rendered[name] = ""
		}
	}
	return rendered
}

func acceptanceCriteriaRange(lines []string) (int, int) {
	for index, line := range lines {
		if strings.TrimSpace(line) != "## Acceptance criteria" {
			continue
		}
		end := len(lines)
		for next := index + 1; next < len(lines); next++ {
			if strings.HasPrefix(strings.TrimSpace(lines[next]), "## ") {
				end = next
				break
			}
		}
		return index, end
	}
	return len(lines), len(lines)
}

func finalStandaloneRuleIndex(lines []string) int {
	for index := len(lines) - 1; index >= 0; index-- {
		if strings.TrimSpace(lines[index]) == "---" {
			return index
		}
	}
	return -1
}

func isProvenanceMarkerLine(line string) bool {
	line = strings.TrimSuffix(line, "\r")
	return strings.HasPrefix(line, "<!-- orchestrator-workflow:") && strings.HasSuffix(line, "-->")
}

func isTableShapedLine(line string) bool {
	_, ok := splitTableRow(line)
	return ok
}

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

type DiffStat struct {
	Files      int
	Insertions int
	Deletions  int
}

func ValidateReviewPRSize(prBody string, stat DiffStat) error {
	parsed, err := reviewPRSize(prBody)
	if err != nil {
		return err
	}
	if parsed == stat {
		return nil
	}
	source := "the reviewed diff"
	if head := reviewedHeadFromProvenance(prBody); head != "" {
		source = fmt.Sprintf("reviewed head %s", head)
	}
	return fmt.Errorf(
		"PR body Size row says %s; %s has %s; refresh the Size row from git diff --stat at the reviewed head",
		formatDiffStat(parsed),
		source,
		formatDiffStat(stat),
	)
}

func ValidateReviewEvidenceAtHead(prBody, headSHA string) error {
	if !isFullCommitSHA(headSHA) {
		return fmt.Errorf("reviewed head SHA %q must be a full 40-character commit SHA", headSHA)
	}
	rows, err := evidenceTableDataRows(prBody)
	if err != nil {
		return err
	}
	for _, row := range rows {
		commit := strings.Trim(row[3], "`")
		if namesReviewedHead(commit, headSHA) {
			continue
		}
		return fmt.Errorf(
			"Evidence %s row: Commit %q does not name reviewed head %s; use the full SHA or an unambiguous prefix of at least 7 hexadecimal characters",
			row[0],
			commit,
			headSHA,
		)
	}
	return nil
}

func reviewPRSize(prBody string) (DiffStat, error) {
	inSection := false
	for _, line := range strings.Split(prBody, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## At a glance" {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if !inSection {
			continue
		}
		cells, ok := splitTableRow(line)
		if !ok || len(cells) != 2 || cells[0] != "Size" {
			continue
		}
		match := reviewPRSizePattern.FindStringSubmatch(strings.ReplaceAll(cells[1], "`", ""))
		if match == nil {
			return DiffStat{}, fmt.Errorf("invalid At a glance Size row: expected `n files · n commits · +x/−y`")
		}
		files, filesErr := strconv.Atoi(match[1])
		insertions, insertionsErr := strconv.Atoi(match[3])
		deletions, deletionsErr := strconv.Atoi(match[4])
		if filesErr != nil || insertionsErr != nil || deletionsErr != nil {
			return DiffStat{}, fmt.Errorf("invalid At a glance Size row: expected `n files · n commits · +x/−y`")
		}
		return DiffStat{Files: files, Insertions: insertions, Deletions: deletions}, nil
	}
	return DiffStat{}, fmt.Errorf("missing At a glance Size row")
}

func reviewedHeadFromProvenance(prBody string) string {
	for _, line := range strings.Split(prBody, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if !strings.HasPrefix(line, "<!-- orchestrator-workflow:") || !strings.HasSuffix(line, "-->") {
			continue
		}
		var marker struct {
			Commit string `json:"commit"`
		}
		payload := strings.TrimSuffix(strings.TrimPrefix(line, "<!-- orchestrator-workflow:"), "-->")
		if err := json.Unmarshal([]byte(payload), &marker); err != nil {
			return ""
		}
		return marker.Commit
	}
	return ""
}

func evidenceTableDataRows(prBody string) ([][]string, error) {
	const header = "| Gate | Command | Result | Commit |"
	const separator = "| --- | --- | --- | --- |"
	lines := strings.Split(prBody, "\n")
	inSection := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Evidence" {
			inSection = true
			continue
		}
		if !inSection {
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			break
		}
		if trimmed != header {
			continue
		}
		if index+1 >= len(lines) || strings.TrimSpace(lines[index+1]) != separator {
			return nil, fmt.Errorf("invalid Evidence table shape: expected %s", header)
		}
		var rows [][]string
		for _, tableLine := range lines[index+2:] {
			trimmedTableLine := strings.TrimSpace(tableLine)
			if trimmedTableLine == "" {
				break
			}
			cells, ok := splitEvidenceRow(tableLine)
			if !ok || len(cells) != 4 {
				return nil, fmt.Errorf("invalid Evidence table shape: expected %s", header)
			}
			rows = append(rows, cells)
		}
		if len(rows) == 0 {
			return nil, fmt.Errorf("Evidence table has no data rows")
		}
		return rows, nil
	}
	return nil, fmt.Errorf("missing Evidence table")
}

// splitEvidenceRow splits an Evidence table row on cell separators while
// keeping an escaped `\|` inside a cell, because gate commands commonly carry
// an alternation such as `-run 'A\|B'` that Markdown requires escaping.
func splitEvidenceRow(line string) ([]string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return nil, false
	}
	var cells []string
	var current strings.Builder
	escaped := false
	for _, char := range trimmed[1 : len(trimmed)-1] {
		switch {
		case escaped:
			if char != '|' {
				current.WriteRune('\\')
			}
			current.WriteRune(char)
			escaped = false
		case char == '\\':
			escaped = true
		case char == '|':
			cells = append(cells, strings.TrimSpace(current.String()))
			current.Reset()
		default:
			current.WriteRune(char)
		}
	}
	if escaped {
		current.WriteRune('\\')
	}
	cells = append(cells, strings.TrimSpace(current.String()))
	return cells, true
}

func namesReviewedHead(commit, headSHA string) bool {
	if len(commit) < 7 || len(commit) > len(headSHA) {
		return false
	}
	for _, char := range commit {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return false
		}
	}
	return strings.HasPrefix(strings.ToLower(headSHA), strings.ToLower(commit))
}

func formatDiffStat(stat DiffStat) string {
	return fmt.Sprintf("%d files, +%d/−%d", stat.Files, stat.Insertions, stat.Deletions)
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
	rowIndexes, err := independentCheckDataRowIndexes(lines)
	if err != nil {
		return nil, err
	}

	var rows []independentCheckRow
	for _, index := range rowIndexes {
		cells, ok := splitTableRow(lines[index])
		if !ok || len(cells) != 4 {
			return nil, fmt.Errorf("invalid Independent checks table shape: expected %s", independentChecksHeader)
		}
		rows = append(rows, independentCheckRow{
			stage:    cells[0],
			role:     cells[1],
			result:   cells[2],
			evidence: cells[3],
		})
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

func independentCheckDataRowIndexes(lines []string) ([]int, error) {
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

	var rowIndexes []int
	for index < len(lines) && strings.TrimSpace(lines[index]) != "" {
		cells, ok := splitTableRow(lines[index])
		if !ok || len(cells) != 4 {
			return nil, fmt.Errorf("invalid Independent checks table shape: expected %s", independentChecksHeader)
		}
		if cells[0] != "Verify" && cells[0] != "Review" {
			return nil, fmt.Errorf("invalid Independent checks table shape: expected %s", independentChecksHeader)
		}
		rowIndexes = append(rowIndexes, index)
		index++
	}
	if len(rowIndexes) > 2 {
		return nil, fmt.Errorf("invalid Independent checks table shape: expected %s", independentChecksHeader)
	}
	return rowIndexes, nil
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

func splitRawTableRow(line string) []string {
	core := strings.TrimSpace(line)
	return strings.Split(core[1:len(core)-1], "|")
}

func renderTableRow(line string, cells []string) string {
	leftTrimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
	prefix := line[:len(line)-len(leftTrimmed)]
	core := strings.TrimRightFunc(leftTrimmed, unicode.IsSpace)
	suffix := leftTrimmed[len(core):]
	return prefix + "|" + strings.Join(cells, "|") + "|" + suffix
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
