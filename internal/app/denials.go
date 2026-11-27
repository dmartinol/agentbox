package app

import (
	"bufio"
	"bytes"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// PolicyDenial is one network connection OpenShell refused. The sandbox log is
// the authority: the entry carries the destination and the calling binary, so
// a remediation rule can be scoped without asking the user to retype them.
type PolicyDenial struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Binary         string `json:"binary"`
	ObservedBinary string `json:"observedBinary,omitempty"`
	Reason         string `json:"reason,omitempty"`
	LogLine        string `json:"logLine,omitempty"`
	Valid          bool   `json:"valid"`
	InvalidReason  string `json:"invalidReason,omitempty"`
}

// RuntimeHost records one host rule added to a live run. Source is "run" for a
// run-only fix or "run+manifest" when the same rule was persisted.
type RuntimeHost struct {
	Host    string    `json:"host"`
	Port    int       `json:"port"`
	Binary  string    `json:"binary"`
	Rule    string    `json:"rule"`
	Source  string    `json:"source"`
	AddedAt time.Time `json:"added_at"`
}

// DenialRequest names the exact denial entry being remediated. The GUI never
// accepts free-form hosts: it sends back the host, port, and binary the sandbox
// logged, and the backend verifies that tuple against current run output.
type DenialRequest struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Binary  string `json:"binary"`
	Persist bool   `json:"persist"`
}

// DenialPreview is what the Add host dialog displays before any write.
type DenialPreview struct {
	Entry            RepoHostRule `json:"entry"`
	PolicyPath       string       `json:"policyPath"`
	PolicyDiff       string       `json:"policyDiff"`
	PersistAvailable bool         `json:"persistAvailable"`
	PersistReason    string       `json:"persistReason,omitempty"`
	ManifestPath     string       `json:"manifestPath,omitempty"`
	ManifestDiff     string       `json:"manifestDiff,omitempty"`
	FragmentPath     string       `json:"fragmentPath,omitempty"`
	FragmentDiff     string       `json:"fragmentDiff,omitempty"`
}

// DenialApplyResult reports what the Add host action changed.
type DenialApplyResult struct {
	DenialPreview
	Outcome        PolicyEditOutcome `json:"outcome"`
	RolledBack     bool              `json:"rolledBack"`
	RuntimeHost    RuntimeHost       `json:"runtimeHost"`
	Persisted      bool              `json:"persisted"`
	FragmentStatus string            `json:"fragmentStatus,omitempty"`
}

var (
	// Shorthand OCSF network denial, for example:
	//   OCSF NET:OPEN [MED] DENIED /usr/bin/curl(64) -> httpbin.org:443 [reason:no matching policy]
	// openshell logs may prefix the line with cursor and source columns.
	ocsfNetworkDenial = regexp.MustCompile(`NET:(?:OPEN|REFUSE)\s+\[[A-Z]+\]\s+(?:DENIED|BLOCKED)\s+(\S+?)\((\d+)\)\s+->\s+(\S+):(\d+)`)
	denialReasonTag   = regexp.MustCompile(`\[reason:([^\]]+)\]`)
	legacyDenialField = regexp.MustCompile(`(?:^|\s)([a-z_]+)=("([^"]*)"|\S+)`)
)

// ParsePolicyDenials extracts actionable network denials from OpenShell sandbox
// output. Filesystem/Landlock denials and L7 method denials are intentionally
// ignored: they do not describe a host, port, and binary that a host rule can
// safely fix. Duplicate tuples collapse to the first occurrence.
func ParsePolicyDenials(output []byte) []PolicyDenial {
	return ParsePolicyDenialsForRun(output, "")
}

// ParsePolicyDenialsForRun also maps a vendored harness binary back to the
// canonical agent binary recorded in the run's policy provenance. OpenShell
// reports the deep vendor path for Codex child processes, and that path can
// look like an opaque credential to the manifest secret scan. The observed
// path stays on the denial so the UI can still show what the sandbox logged.
func ParsePolicyDenialsForRun(output []byte, agentBinary string) []PolicyDenial {
	seen := make(map[string]bool)
	denials := make([]PolicyDenial, 0)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		for _, denial := range parseDenialLine(line) {
			annotateDenial(&denial, agentBinary)
			key := denialKey(denial.Host, denial.Port, denial.Binary)
			if seen[key] {
				continue
			}
			seen[key] = true
			denials = append(denials, denial)
		}
	}
	return denials
}

func parseDenialLine(line string) []PolicyDenial {
	if match := ocsfNetworkDenial.FindStringSubmatch(line); match != nil {
		port, err := strconv.Atoi(match[4])
		if err != nil {
			return nil
		}
		host := strings.TrimSuffix(strings.TrimPrefix(match[3], "["), "]")
		reason := ""
		if r := denialReasonTag.FindStringSubmatch(line); r != nil {
			reason = strings.TrimSpace(r[1])
		}
		return []PolicyDenial{{
			Host:    strings.ToLower(host),
			Port:    port,
			Binary:  match[1],
			Reason:  reason,
			LogLine: line,
		}}
	}
	// Older OpenShell releases emitted key/value denial lines:
	//   action=deny dst_host=api.github.com dst_port=443 binary=/usr/bin/curl deny_reason="no matching network policy"
	fields := make(map[string]string)
	for _, match := range legacyDenialField.FindAllStringSubmatch(line, -1) {
		value := match[2]
		if match[3] != "" || strings.HasPrefix(value, `"`) {
			value = match[3]
		}
		fields[strings.ToLower(match[1])] = value
	}
	action := strings.ToLower(fields["action"])
	if action != "deny" && action != "denied" && action != "blocked" {
		return nil
	}
	port, err := strconv.Atoi(fields["dst_port"])
	if err != nil || fields["dst_host"] == "" || fields["binary"] == "" {
		return nil
	}
	return []PolicyDenial{{
		Host:    strings.ToLower(fields["dst_host"]),
		Port:    port,
		Binary:  fields["binary"],
		Reason:  strings.TrimSpace(fields["deny_reason"]),
		LogLine: line,
	}}
}

func denialKey(host string, port int, binary string) string {
	return fmt.Sprintf("%s:%d\x00%s", strings.ToLower(host), port, binary)
}

func annotateDenial(denial *PolicyDenial, agentBinary string) {
	rule := RepoHostRule{Host: denial.Host, Port: denial.Port, Binaries: []string{denial.Binary}}
	// A vendored harness path is always reported as the canonical agent
	// binary; whether the logged path happens to pass manifest validation is
	// not a reason to keep the vendor layout in the rule.
	if canonical := canonicalDenialBinary(denial.Binary, agentBinary); canonical != "" {
		rule.Binaries[0] = canonical
	}
	err := ValidateHostRule(rule)
	if rule.Binaries[0] != denial.Binary {
		denial.ObservedBinary = denial.Binary
		denial.Binary = rule.Binaries[0]
	}
	denial.Valid = err == nil
	if err != nil {
		denial.InvalidReason = err.Error()
	}
}

func canonicalDenialBinary(binary, agentBinary string) string {
	agentBinary = strings.TrimSpace(agentBinary)
	if agentBinary == "" || binary == "" || binary == agentBinary {
		return ""
	}
	if path.Base(binary) != path.Base(agentBinary) {
		return ""
	}
	return agentBinary
}
