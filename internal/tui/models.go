package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dmartinol/agentbox/internal/app"
)

type runRow struct {
	ID        string
	Name      string
	Project   string
	Agent     string
	Mode      string
	Approval  string
	Isolation string
	Status    string
	Synced    string
	Worktree  string
}

func newRunRows(runs []app.Run) []runRow {
	out := make([]runRow, 0, len(runs))
	for _, r := range runs {
		synced := "no"
		if r.Synced {
			synced = "yes"
		}
		isolation := "direct"
		if r.Isolated() {
			isolation = "worktree"
		}
		name := r.Name
		if name == "" {
			name = r.ID
		}
		out = append(out, runRow{
			ID:        r.ID,
			Name:      name,
			Project:   r.Project,
			Agent:     r.Agent,
			Mode:      r.Mode,
			Approval:  r.Approval,
			Isolation: isolation,
			Status:    r.Status,
			Synced:    synced,
			Worktree:  r.Worktree,
		})
	}
	return out
}

type imageRecord struct {
	ID        string            `json:"Id"`
	Names     []string          `json:"Names"`
	Tag       string            `json:"Tag"`
	Labels    map[string]string `json:"Labels"`
	Size      int64             `json:"Size"`
	CreatedAt string            `json:"CreatedAt"`
	Digest    string            `json:"Digest"`
}

type imageRow struct {
	ID         string
	Name       string
	Agent      string
	Version    string
	Recipe     string
	BaseDigest string
	Size       string
	Created    string
}

func newImageRows(data []byte) ([]imageRow, error) {
	var records []imageRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	out := make([]imageRow, 0, len(records))
	for _, rec := range records {
		name := strings.Join(rec.Names, ", ")
		if name == "" && rec.Tag != "" {
			name = rec.Tag
		}
		label := func(name string) string {
			if v, ok := rec.Labels[name]; ok {
				return v
			}
			for _, prefix := range []string{"agentbox.", "ai-sandbox."} {
				if v, ok := rec.Labels[prefix+name]; ok {
					return v
				}
			}
			return ""
		}
		out = append(out, imageRow{
			ID:         shortDigest(rec.ID),
			Name:       name,
			Agent:      label("agent"),
			Version:    label("version"),
			Recipe:     label("recipe"),
			BaseDigest: shortDigest(label("base-digest")),
			Size:       humanSize(rec.Size),
			Created:    rec.CreatedAt,
		})
	}
	return out, nil
}

type gatewayRecord struct {
	Name     string `json:"name"`
	Active   bool   `json:"active"`
	Endpoint string `json:"endpoint"`
	Type     string `json:"type"`
	Auth     string `json:"auth"`
	Source   string `json:"source"`
}

type gatewayRow struct {
	Name     string
	Active   string
	Endpoint string
	Type     string
	Auth     string
	Source   string
}

func newGatewayRows(data []byte) ([]gatewayRow, error) {
	var records []gatewayRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	out := make([]gatewayRow, 0, len(records))
	for _, rec := range records {
		active := "no"
		if rec.Active {
			active = "yes"
		}
		out = append(out, gatewayRow{
			Name:     rec.Name,
			Active:   active,
			Endpoint: rec.Endpoint,
			Type:     rec.Type,
			Auth:     rec.Auth,
			Source:   rec.Source,
		})
	}
	return out, nil
}

type projectRow struct {
	Name      string
	Root      string
	Isolation string
}

func newProjectRows(projects []app.Project) []projectRow {
	out := make([]projectRow, 0, len(projects))
	for _, p := range projects {
		isolation := "worktree"
		if !p.UseWorktree() {
			isolation = "direct"
		}
		out = append(out, projectRow{Name: p.Name, Root: p.Root, Isolation: isolation})
	}
	return out
}

type policyRow struct {
	Name   string
	Path   string
	UsedBy string
}

func newPolicyRows(policies []app.PolicyConfiguration) []policyRow {
	out := make([]policyRow, 0, len(policies))
	for _, policy := range policies {
		usedBy := strings.Join(policy.UsedBy, ", ")
		if usedBy == "" {
			usedBy = "-"
		}
		out = append(out, policyRow{Name: policy.Name, Path: policy.Path, UsedBy: usedBy})
	}
	return out
}

type profileRow struct {
	Name     string
	Agent    string
	Mode     string
	Model    string
	Provider string
	Approval string
}

func newProfileRows(profiles []app.Profile) []profileRow {
	out := make([]profileRow, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, profileRow{
			Name:     p.Name,
			Agent:    p.Agent,
			Mode:     p.Mode,
			Model:    p.Model,
			Provider: p.Provider,
			Approval: p.Approval,
		})
	}
	return out
}

type doctorRow struct {
	Name   string
	Status string
	Detail string
}

func newDoctorRows(checks []app.Check) []doctorRow {
	out := make([]doctorRow, 0, len(checks))
	for _, c := range checks {
		status := "FAIL"
		if c.OK {
			status = "OK"
		}
		out = append(out, doctorRow{Name: c.Name, Status: status, Detail: c.Detail})
	}
	return out
}

type summary struct {
	Active   int
	Finished int
	Failed   int
	Unsynced int
	Images   int
	Gateways int
	Profiles int
	Projects int
}

func newSummary(runs []app.Run, images int, gateways int, profiles int, projects int) summary {
	var s summary
	for _, r := range runs {
		switch strings.ToLower(r.Status) {
		case "running", "created", "prepared":
			s.Active++
		case "finished":
			s.Finished++
		case "error", "create-failed", "policy-blocked", "unreadable", "unreachable":
			s.Failed++
		}
		if !r.Synced {
			s.Unsynced++
		}
	}
	s.Images = images
	s.Gateways = gateways
	s.Profiles = profiles
	s.Projects = projects
	return s
}

func shortDigest(s string) string {
	if s == "" {
		return "-"
	}
	s = strings.TrimPrefix(s, "sha256:")
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func sortedJSONKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func itoa(i int) string { return strconv.Itoa(i) }
