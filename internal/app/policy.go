package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Policy struct {
	Version    int `yaml:"version"`
	Filesystem struct {
		IncludeWorkdir bool     `yaml:"include_workdir"`
		ReadOnly       []string `yaml:"read_only"`
		ReadWrite      []string `yaml:"read_write"`
	} `yaml:"filesystem_policy"`
	Landlock struct {
		Compatibility string `yaml:"compatibility"`
	} `yaml:"landlock"`
	Process struct {
		User  string `yaml:"run_as_user"`
		Group string `yaml:"run_as_group"`
	} `yaml:"process"`
	Network     map[string]NetworkRule `yaml:"network_policies"`
	Middlewares map[string]any         `yaml:"network_middlewares,omitempty"`
}

type NetworkRule struct {
	Name      string     `yaml:"name"`
	Endpoints []Endpoint `yaml:"endpoints"`
	Binaries  []Binary   `yaml:"binaries"`
}
type Endpoint struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Protocol string `yaml:"protocol,omitempty"`
}
type Binary struct {
	Path string `yaml:"path"`
}

type PolicyConfiguration struct {
	Name   string   `json:"name"`
	Path   string   `json:"path"`
	UsedBy []string `json:"used_by,omitempty"`
}

func ParsePolicy(b []byte) (Policy, error) {
	var p Policy
	dec := yaml.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&p); err != nil {
		return p, err
	}
	if p.Version != 1 {
		return p, fmt.Errorf("policy version must be 1")
	}
	if p.Landlock.Compatibility != "hard_requirement" {
		return p, fmt.Errorf("policy must set landlock.compatibility: hard_requirement")
	}
	if !p.Filesystem.IncludeWorkdir {
		return p, fmt.Errorf("policy must include workdir")
	}
	for name, rule := range p.Network {
		if !safeName.MatchString(name) || len(rule.Endpoints) == 0 || len(rule.Binaries) == 0 {
			return p, fmt.Errorf("invalid network rule %q", name)
		}
		for _, e := range rule.Endpoints {
			if e.Host == "" || e.Port < 1 || e.Port > 65535 {
				return p, fmt.Errorf("invalid endpoint in %q", name)
			}
		}
		for _, b := range rule.Binaries {
			if !strings.HasPrefix(b.Path, "/") {
				return p, fmt.Errorf("network binary path must be absolute")
			}
		}
	}
	return p, nil
}

func DefaultPolicy() []byte {
	return []byte("version: 1\nfilesystem_policy:\n  include_workdir: true\n  read_only:\n    - /usr\n    - /lib\n    - /etc\n    - /proc\n    - /dev/urandom\n  read_write:\n    - /workspace\n    - /tmp\n    - /dev/null\n    - /dev/pts\n    - /dev/ptmx\nlandlock:\n  compatibility: hard_requirement\nprocess:\n  run_as_user: sandbox\n  run_as_group: sandbox\nnetwork_policies: {}\n")
}

func (s Store) ListPolicyConfigurations() ([]PolicyConfiguration, error) {
	entries, err := os.ReadDir(s.PolicyDir())
	if errors.Is(err, os.ErrNotExist) {
		return []PolicyConfiguration{}, nil
	}
	if err != nil {
		return nil, err
	}
	runs, err := s.ListRuns()
	if err != nil {
		return nil, err
	}
	profiles, err := s.ListProfiles()
	if err != nil {
		return nil, err
	}
	runPolicies := make(map[string]struct{})
	for _, run := range runs {
		runPolicies[run.Policy] = struct{}{}
		if run.PendingPolicy != "" {
			runPolicies[run.PendingPolicy] = struct{}{}
		}
	}
	usedBy := make(map[string][]string)
	for _, profile := range profiles {
		usedBy[profile.Policy] = append(usedBy[profile.Policy], profile.Name)
	}
	var out []PolicyConfiguration
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".yaml")
		if !safeName.MatchString(name) {
			continue
		}
		path := filepath.Join(s.PolicyDir(), entry.Name())
		if _, ok := runPolicies[path]; ok {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if _, err := ParsePolicy(b); err != nil {
			return nil, fmt.Errorf("policy %s: %w", name, err)
		}
		users := append([]string(nil), usedBy[path]...)
		sort.Strings(users)
		out = append(out, PolicyConfiguration{Name: name, Path: path, UsedBy: users})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s Store) PolicyConfigurationPath(name string) (string, error) {
	if !safeName.MatchString(name) {
		return "", fmt.Errorf("invalid policy name")
	}
	return filepath.Join(s.PolicyDir(), name+".yaml"), nil
}

func (s Store) CreatePolicyConfiguration(name string) (PolicyConfiguration, error) {
	path, err := s.PolicyConfigurationPath(name)
	if err != nil {
		return PolicyConfiguration{}, err
	}
	if err := s.Ensure(); err != nil {
		return PolicyConfiguration{}, err
	}
	if _, err := os.Stat(path); err == nil {
		return PolicyConfiguration{}, fmt.Errorf("policy %q already exists", name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return PolicyConfiguration{}, err
	}
	if err := atomicWrite(path, DefaultPolicy()); err != nil {
		return PolicyConfiguration{}, err
	}
	return PolicyConfiguration{Name: name, Path: path}, nil
}

func (s Store) EditPolicyConfiguration(ctx context.Context, name string) error {
	path, err := s.PolicyConfigurationPath(name)
	if err != nil {
		return err
	}
	old, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := ParsePolicy(old); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.PolicyDir(), "edit-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(old); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := RunEditor(ctx, tmp.Name()); err != nil {
		return err
	}
	next, err := os.ReadFile(tmp.Name())
	if err != nil {
		return err
	}
	if _, err := ParsePolicy(next); err != nil {
		return err
	}
	return atomicWrite(path, next)
}

func (a *App) AddPolicyNetworkRule(policyName, agent, host string) error {
	path, err := a.Store.PolicyConfigurationPath(policyName)
	if err != nil {
		return err
	}
	old, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := ParsePolicy(old); err != nil {
		return err
	}
	next, err := addNetworkRule(old, agent, host, "")
	if err != nil {
		return err
	}
	return atomicWrite(path, next)
}

func (a *App) PolicyShow(ctx context.Context, runID string) (map[string]json.RawMessage, error) {
	r, err := a.Store.LoadRun(runID)
	if err != nil {
		return nil, err
	}
	out := map[string]json.RawMessage{}
	for key, args := range map[string][]string{
		"base":      {"policy", "get", r.Sandbox, "--base", "-o", "json"},
		"effective": {"policy", "get", r.Sandbox, "--full", "-o", "json"},
		"global":    {"policy", "get", "--global", "-o", "json"},
	} {
		if r.Gateway != "" {
			args = append(args, "--gateway", r.Gateway)
		}
		b, err := a.Runner.Output(ctx, "openshell", args...)
		if err != nil {
			if key == "global" {
				out[key] = json.RawMessage("null")
				continue
			}
			return nil, err
		}
		if !json.Valid(b) {
			return nil, fmt.Errorf("OpenShell returned invalid %s policy JSON", key)
		}
		out[key] = b
	}
	return out, nil
}

func (a *App) PolicyEdit(ctx context.Context, runID, allowHost, binary string, dry bool) error {
	r, err := a.Store.LoadRun(runID)
	if err != nil {
		return err
	}
	old, err := os.ReadFile(r.Policy)
	if err != nil {
		return err
	}
	oldP, err := ParsePolicy(old)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(a.Store.PolicyDir(), "edit-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(old); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if allowHost != "" {
		next, err := addNetworkRule(old, r.Agent, allowHost, binary)
		if err != nil {
			return err
		}
		if err := os.WriteFile(tmp.Name(), next, 0600); err != nil {
			return err
		}
	} else if !dry {
		if err := RunEditor(ctx, tmp.Name()); err != nil {
			return err
		}
	}
	next, err := os.ReadFile(tmp.Name())
	if err != nil {
		return err
	}
	newP, err := ParsePolicy(next)
	if err != nil {
		return err
	}
	fmt.Print(simpleDiff(old, next))
	staticChanged := policyHasStaticChanges(oldP, newP)
	if dry {
		if staticChanged {
			fmt.Println("static change: pending for a future sandbox")
		} else {
			fmt.Println("network change: apply live on save")
		}
		return nil
	}
	return a.savePolicyEdit(ctx, r, old, next)
}

func addNetworkRule(old []byte, agent, host, binary string) ([]byte, error) {
	if binary == "" {
		binary = "/usr/local/bin/" + agentCommand(agent)
	}
	if !strings.HasPrefix(binary, "/") {
		return nil, fmt.Errorf("binary must be an absolute path")
	}
	name := "host-" + strings.NewReplacer(".", "-", ":", "-").Replace(host)
	if !safeName.MatchString(name) {
		return nil, fmt.Errorf("invalid host")
	}
	var root map[string]any
	if err := yaml.Unmarshal(old, &root); err != nil {
		return nil, err
	}
	net, ok := root["network_policies"].(map[string]any)
	if !ok {
		net = map[string]any{}
	}
	net[name] = map[string]any{"name": name, "endpoints": []map[string]any{{"host": host, "port": 443, "protocol": "tcp"}}, "binaries": []map[string]any{{"path": binary}}}
	root["network_policies"] = net
	return yaml.Marshal(root)
}

func PolicyForAgent(policy []byte, agent string) ([]byte, error) {
	parsed, err := ParsePolicy(policy)
	if err != nil {
		return nil, err
	}
	if _, err := agentPackage(agent); err != nil {
		return nil, err
	}
	agentBinary := "/usr/local/bin/" + agentCommand(agent)
	for _, rule := range parsed.Network {
		for i := range rule.Binaries {
			if isAgentBinary(rule.Binaries[i].Path) {
				rule.Binaries[i].Path = agentBinary
			}
		}
	}
	return yaml.Marshal(parsed)
}

func isAgentBinary(path string) bool {
	return path == "/usr/local/bin/claude" || path == "/usr/local/bin/codex" || path == "/usr/local/bin/opencode"
}

func (a *App) ApplyPolicyConfiguration(ctx context.Context, policyName, runID string) error {
	path, err := a.Store.PolicyConfigurationPath(policyName)
	if err != nil {
		return err
	}
	config, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := ParsePolicy(config); err != nil {
		return err
	}
	run, err := a.Store.LoadRun(runID)
	if err != nil {
		return err
	}
	next, err := PolicyForAgent(config, run.Agent)
	if err != nil {
		return err
	}
	old, err := os.ReadFile(run.Policy)
	if err != nil {
		return err
	}
	if _, err := ParsePolicy(old); err != nil {
		return err
	}
	return a.savePolicyEdit(ctx, run, old, next)
}

func policyHasStaticChanges(oldPolicy, newPolicy Policy) bool {
	return !reflect.DeepEqual(oldPolicy.Filesystem, newPolicy.Filesystem) ||
		!reflect.DeepEqual(oldPolicy.Landlock, newPolicy.Landlock) ||
		!reflect.DeepEqual(oldPolicy.Process, newPolicy.Process)
}

func (a *App) savePolicyEdit(ctx context.Context, r Run, old, next []byte) error {
	oldPolicy, err := ParsePolicy(old)
	if err != nil {
		return err
	}
	newPolicy, err := ParsePolicy(next)
	if err != nil {
		return err
	}
	if policyHasStaticChanges(oldPolicy, newPolicy) {
		path := filepath.Join(a.Store.PolicyDir(), r.ID+"-pending.yaml")
		if err := atomicWrite(path, next); err != nil {
			return err
		}
		r.PendingPolicy = path
		return a.Store.SaveRun(r)
	}
	// The original remains available for rollback if OpenShell rejects the update.
	if err := atomicWrite(r.Policy, next); err != nil {
		return err
	}
	args := []string{"policy", "set", r.Sandbox, "--policy", r.Policy, "--wait"}
	if r.Gateway != "" {
		args = append(args, "--gateway", r.Gateway)
	}
	if _, err := a.Runner.Output(ctx, "openshell", args...); err != nil {
		_ = atomicWrite(r.Policy, old)
		return fmt.Errorf("policy apply failed; local policy restored: %w", err)
	}
	return nil
}

func simpleDiff(a, b []byte) string {
	if bytes.Equal(a, b) {
		return "No policy changes.\n"
	}
	return "--- current\n+++ proposed\n-" + strings.ReplaceAll(strings.TrimSuffix(string(a), "\n"), "\n", "\n-") + "\n+" + strings.ReplaceAll(strings.TrimSuffix(string(b), "\n"), "\n", "\n+") + "\n"
}
