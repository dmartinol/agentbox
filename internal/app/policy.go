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
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	UsedBy     []string `json:"used_by,omitempty"`
	DefaultFor string   `json:"default_for,omitempty"`
}

// PolicyEditOutcome describes what a policy edit did, or would do in a dry run.
type PolicyEditOutcome string

const (
	PolicyEditNetworkPreview    PolicyEditOutcome = "network-preview"
	PolicyEditStaticPreview     PolicyEditOutcome = "static-preview"
	PolicyEditLiveApplied       PolicyEditOutcome = "live-applied"
	PolicyEditStaticPending     PolicyEditOutcome = "static-pending"
	PolicyEditNoLiveSandbox     PolicyEditOutcome = "no-live-sandbox"
	PolicyEditOpenShellRejected PolicyEditOutcome = "openshell-rejected"
)

// PolicyEditResult carries the computed policy diff and the edit outcome so
// callers can report exactly what happened.
type PolicyEditResult struct {
	Diff       string            `json:"diff,omitempty"`
	Outcome    PolicyEditOutcome `json:"outcome"`
	RolledBack bool              `json:"rolledBack"`
	PolicyPath string            `json:"policyPath,omitempty"`
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

// DefaultPolicy returns the baseline policy for an agent sandbox.
func DefaultPolicy() []byte {
	return []byte("version: 1\nfilesystem_policy:\n  include_workdir: true\n  read_only:\n    - /usr\n    - /lib\n    - /etc\n    - /proc\n    - /dev/urandom\n  read_write:\n    - /workspace\n    - /tmp\n    - /var/tmp\n    - /dev/null\n    - /dev/pts\n    - /dev/ptmx\nlandlock:\n  compatibility: hard_requirement\nprocess:\n  run_as_user: sandbox\n  run_as_group: sandbox\nnetwork_policies: {}\n")
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
	defaultFor := make(map[string]string)
	for _, profile := range profiles {
		usedBy[profile.Policy] = append(usedBy[profile.Policy], profile.Name)
		defaultFor[profile.Name] = profile.Name
	}
	out := make([]PolicyConfiguration, 0, len(entries))
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
		out = append(out, PolicyConfiguration{Name: name, Path: path, UsedBy: users, DefaultFor: defaultFor[name]})
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

func (s Store) LoadPolicyConfiguration(name string) (PolicyConfiguration, error) {
	path, err := s.PolicyConfigurationPath(name)
	if err != nil {
		return PolicyConfiguration{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return PolicyConfiguration{}, err
	}
	if _, err := ParsePolicy(b); err != nil {
		return PolicyConfiguration{}, err
	}
	users, err := s.policyUsers(path)
	if err != nil {
		return PolicyConfiguration{}, err
	}
	return PolicyConfiguration{Name: name, Path: path, UsedBy: users}, nil
}

func (s Store) LoadPolicyConfigurationContent(name string) ([]byte, error) {
	path, err := s.PolicyConfigurationPath(name)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if _, err := ParsePolicy(b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s Store) SavePolicyConfigurationContent(name string, content []byte) error {
	path, err := s.PolicyConfigurationPath(name)
	if err != nil {
		return err
	}
	if _, err := ParsePolicy(content); err != nil {
		return err
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	return atomicWrite(path, content)
}

func (s Store) DeletePolicyConfiguration(name string) error {
	path, err := s.PolicyConfigurationPath(name)
	if err != nil {
		return err
	}
	if err := ensureResourceExists(path, "policy", name); err != nil {
		return err
	}
	return os.Remove(path)
}

func (a *App) DeletePolicyConfiguration(name string) error {
	if _, err := a.Store.LoadPolicyConfiguration(name); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &NotFoundError{Resource: "policy", Name: name}
		}
		return err
	}
	path, err := a.Store.PolicyConfigurationPath(name)
	if err != nil {
		return err
	}
	runs, err := a.Store.ListRuns()
	if err != nil {
		return err
	}
	var runDependents []string
	for _, run := range runs {
		if run.Policy == path || run.PendingPolicy == path {
			runDependents = append(runDependents, runDisplayName(run))
		}
	}
	if len(runDependents) > 0 {
		sort.Strings(runDependents)
		return &ConflictError{Resource: "policy", Name: name, Reason: "runs reference it", Dependents: runDependents}
	}
	profiles, err := a.Store.ListProfiles()
	if err != nil {
		return err
	}
	var referenced, generatedDefault []string
	for _, profile := range profiles {
		if profile.Policy == path {
			referenced = append(referenced, profile.Name)
		}
		if profile.Name == name {
			generatedDefault = append(generatedDefault, profile.Name)
		}
	}
	if len(referenced) > 0 {
		sort.Strings(referenced)
		return &ConflictError{Resource: "policy", Name: name, Reason: "profiles reference it", Dependents: referenced}
	}
	if len(generatedDefault) > 0 {
		sort.Strings(generatedDefault)
		return &ConflictError{Resource: "policy", Name: name, Reason: "it is the generated default for a profile", Dependents: generatedDefault}
	}
	return a.Store.DeletePolicyConfiguration(name)
}

func (s Store) policyUsers(path string) ([]string, error) {
	profiles, err := s.ListProfiles()
	if err != nil {
		return nil, err
	}
	var users []string
	for _, profile := range profiles {
		if profile.Policy == path {
			users = append(users, profile.Name)
		}
	}
	sort.Strings(users)
	return users, nil
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

func (a *App) PolicyEdit(ctx context.Context, runID, allowHost, binary string, dry bool) (PolicyEditResult, error) {
	var result PolicyEditResult
	r, err := a.Store.LoadRun(runID)
	if err != nil {
		return result, err
	}
	if !dry && allowHost != "" {
		current, err := a.GetRun(ctx, runID)
		if err != nil {
			return result, err
		}
		if !current.Status.IsAttachable() {
			result.Outcome = PolicyEditNoLiveSandbox
			return result, &NoLiveSandboxError{}
		}
		r = current
	}
	old, err := os.ReadFile(r.Policy)
	if err != nil {
		return result, err
	}
	oldP, err := ParsePolicy(old)
	if err != nil {
		return result, err
	}
	tmp, err := os.CreateTemp(a.Store.PolicyDir(), "edit-*.yaml")
	if err != nil {
		return result, err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return result, err
	}
	if _, err := tmp.Write(old); err != nil {
		tmp.Close()
		return result, err
	}
	if err := tmp.Close(); err != nil {
		return result, err
	}
	if allowHost != "" {
		next, err := addNetworkRule(old, r.Agent, allowHost, binary)
		if err != nil {
			return result, err
		}
		if err := os.WriteFile(tmp.Name(), next, 0600); err != nil {
			return result, err
		}
	} else {
		if err := RunEditor(ctx, tmp.Name()); err != nil {
			return result, err
		}
	}
	next, err := os.ReadFile(tmp.Name())
	if err != nil {
		return result, err
	}
	newP, err := ParsePolicy(next)
	if err != nil {
		return result, err
	}
	result.Diff = simpleDiff(old, next)
	staticChanged := policyHasStaticChanges(oldP, newP)
	if dry {
		if staticChanged {
			result.Outcome = PolicyEditStaticPreview
		} else {
			result.Outcome = PolicyEditNetworkPreview
		}
		return result, nil
	}
	saved, err := a.savePolicyEdit(ctx, r, old, next)
	if saved.Outcome != "" {
		result.Outcome = saved.Outcome
		result.RolledBack = saved.RolledBack
	}
	if saved.Diff != "" {
		result.Diff = saved.Diff
	}
	result.PolicyPath = saved.PolicyPath
	return result, err
}

func addNetworkRule(old []byte, agent, host, binary string) ([]byte, error) {
	paths, err := agentEgressBinaries(agent)
	if err != nil {
		return nil, err
	}
	if binary != "" {
		paths = []string{binary}
	}
	for _, path := range paths {
		if !strings.HasPrefix(path, "/") {
			return nil, fmt.Errorf("binary must be an absolute path")
		}
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
	binaries := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		binaries = append(binaries, map[string]any{"path": path})
	}
	net[name] = map[string]any{"name": name, "endpoints": []map[string]any{{"host": host, "port": 443, "protocol": "tcp"}}, "binaries": binaries}
	root["network_policies"] = net
	return yaml.Marshal(root)
}

// agentEgressBinaries lists the executables that actually open sockets for a
// run. The supported agents are Node applications, so matching only the
// launcher path misses the node process, and git uses a separate HTTPS helper.
// A host rule must include all of them to take effect.
func agentEgressBinaries(agent string) ([]string, error) {
	agentBinary, err := AgentBinary(agent)
	if err != nil {
		return nil, err
	}
	return []string{
		agentBinary,
		"/usr/local/bin/node",
		"/usr/bin/git",
		"/usr/lib/git-core/**",
		"/usr/bin/curl",
		"/usr/bin/git-lfs",
	}, nil
}

// AgentBinary returns the launcher path for a supported agent inside its
// sandbox image.
func AgentBinary(agent string) (string, error) {
	if _, err := agentPackage(agent); err != nil {
		return "", err
	}
	return "/usr/local/bin/" + agentCommand(agent), nil
}

func PolicyForAgent(policy []byte, agent string) ([]byte, error) {
	parsed, err := ParsePolicy(policy)
	if err != nil {
		return nil, err
	}
	agentBinary, err := AgentBinary(agent)
	if err != nil {
		return nil, err
	}
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
	if run.PolicyFragment != "" {
		fragment, err := os.ReadFile(run.PolicyFragment)
		if err != nil {
			return fmt.Errorf("run policy fragment %s is unavailable; rebuild it with `agentbox repo setup`: %w", run.PolicyFragment, err)
		}
		next, err = composeRepoPolicy(next, path, fragment, run.PolicyFragment)
		if err != nil {
			return err
		}
	}
	old, err := os.ReadFile(run.Policy)
	if err != nil {
		return err
	}
	if _, err := ParsePolicy(old); err != nil {
		return err
	}
	_, err = a.savePolicyEdit(ctx, run, old, next)
	return err
}

func policyHasStaticChanges(oldPolicy, newPolicy Policy) bool {
	return !reflect.DeepEqual(oldPolicy.Filesystem, newPolicy.Filesystem) ||
		!reflect.DeepEqual(oldPolicy.Landlock, newPolicy.Landlock) ||
		!reflect.DeepEqual(oldPolicy.Process, newPolicy.Process)
}

func (a *App) savePolicyEdit(ctx context.Context, r Run, old, next []byte) (PolicyEditResult, error) {
	result := PolicyEditResult{Diff: simpleDiff(old, next)}
	oldPolicy, err := ParsePolicy(old)
	if err != nil {
		return result, err
	}
	newPolicy, err := ParsePolicy(next)
	if err != nil {
		return result, err
	}
	if policyHasStaticChanges(oldPolicy, newPolicy) {
		path := filepath.Join(a.Store.PolicyDir(), r.ID+"-pending.yaml")
		if err := atomicWrite(path, next); err != nil {
			return result, err
		}
		r.PendingPolicy = path
		result.Outcome = PolicyEditStaticPending
		result.PolicyPath = path
		return result, a.Store.SaveRun(r)
	}
	// The original remains available for rollback if OpenShell rejects the update.
	result.PolicyPath = r.Policy
	if err := atomicWrite(r.Policy, next); err != nil {
		return result, err
	}
	args := []string{"policy", "set", r.Sandbox, "--policy", r.Policy, "--wait"}
	if r.Gateway != "" {
		args = append(args, "--gateway", r.Gateway)
	}
	if _, applyErr := a.Runner.Output(ctx, "openshell", args...); applyErr != nil {
		result.Outcome = PolicyEditOpenShellRejected
		if restoreErr := atomicWrite(r.Policy, old); restoreErr != nil {
			result.RolledBack = false
			return result, fmt.Errorf("OpenShell rejected the policy update; local policy rollback failed: %w; %v", applyErr, restoreErr)
		}
		result.RolledBack = true
		return result, fmt.Errorf("OpenShell rejected the policy update; the local policy was restored: %w", applyErr)
	}
	result.Outcome = PolicyEditLiveApplied
	return result, nil
}

// RunPolicyFile returns the run-scoped policy YAML and its path. Reusable
// policy configurations are templates; each run copies one at start and the
// Allow host action edits that copy.
func (a *App) RunPolicyFile(runID string) (string, string, error) {
	r, err := a.Store.LoadRun(runID)
	if err != nil {
		return "", "", err
	}
	b, err := os.ReadFile(r.Policy)
	if err != nil {
		return r.Policy, "", err
	}
	return r.Policy, string(b), nil
}

func simpleDiff(a, b []byte) string {
	if bytes.Equal(a, b) {
		return "No policy changes.\n"
	}
	return "--- current\n+++ proposed\n-" + strings.ReplaceAll(strings.TrimSuffix(string(a), "\n"), "\n", "\n-") + "\n+" + strings.ReplaceAll(strings.TrimSuffix(string(b), "\n"), "\n", "\n+") + "\n"
}
