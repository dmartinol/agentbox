package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var safeName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var secretName = regexp.MustCompile(`(?i)(key|token|secret|password|credential)`)

type Profile struct {
	Version  int      `yaml:"version" json:"version"`
	Name     string   `yaml:"name" json:"name"`
	Agent    string   `yaml:"agent" json:"agent"`
	Image    string   `yaml:"image" json:"image"`
	Mode     string   `yaml:"mode" json:"mode"`
	Model    string   `yaml:"model" json:"model"`
	Provider string   `yaml:"provider" json:"provider"`
	Policy   string   `yaml:"policy" json:"policy"`
	Approval string   `yaml:"approval" json:"approval"`
	Args     []string `yaml:"args,omitempty" json:"args,omitempty"`
}

func (p Profile) Validate() error {
	if p.Version != 1 || !safeName.MatchString(p.Name) {
		return fmt.Errorf("profile needs version 1 and a lower-case name")
	}
	if p.Agent != "claude" && p.Agent != "codex" && p.Agent != "opencode" {
		return fmt.Errorf("agent must be claude, codex, or opencode")
	}
	if p.Mode != "standard" && p.Mode != "enmaas" {
		return fmt.Errorf("mode must be standard or enmaas")
	}
	if p.Approval != "prompted" && p.Approval != "unattended" {
		return fmt.Errorf("approval must be prompted or unattended")
	}
	if p.Provider == "" || !safeName.MatchString(p.Provider) {
		return fmt.Errorf("provider must name an existing OpenShell provider")
	}
	if p.Policy == "" {
		return fmt.Errorf("policy path is required")
	}
	if p.Mode == "enmaas" && p.Model == "" {
		return fmt.Errorf("enmaas mode requires an exact model ID")
	}
	for _, arg := range p.Args {
		if strings.ContainsRune(arg, 0) || secretName.MatchString(arg) {
			return fmt.Errorf("profile arguments may not contain secret-like values")
		}
	}
	return nil
}

type Run struct {
	ID            string    `json:"id"`
	Created       time.Time `json:"created"`
	Source        string    `json:"source"`
	Worktree      string    `json:"worktree"`
	Branch        string    `json:"branch"`
	Baseline      string    `json:"baseline"`
	LastSynced    string    `json:"last_synced"`
	Fingerprint   string    `json:"fingerprint,omitempty"`
	Sandbox       string    `json:"sandbox"`
	Gateway       string    `json:"gateway"`
	Agent         string    `json:"agent"`
	Image         string    `json:"image"`
	Mode          string    `json:"mode"`
	Model         string    `json:"model"`
	Approval      string    `json:"approval"`
	Status        string    `json:"status"`
	Synced        bool      `json:"synced"`
	Policy        string    `json:"policy"`
	PendingPolicy string    `json:"pending_policy,omitempty"`
}

type Store struct {
	Base string
}

func DefaultStore() (Store, error) {
	if custom := os.Getenv("AGENTBOX_HOME"); custom != "" {
		return Store{Base: custom}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Store{}, err
	}
	if runtimeGOOS() == "darwin" {
		return Store{Base: filepath.Join(home, "Library", "Application Support", "agentbox")}, nil
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(home, ".local", "state")
	}
	return Store{Base: filepath.Join(base, "agentbox")}, nil
}

func (s Store) Ensure() error {
	for _, dir := range []string{s.Base, s.ProfilesDir(), s.RunsDir(), s.WorktreesDir(), s.PolicyDir()} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0700); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) ProfilesDir() string            { return filepath.Join(s.Base, "profiles") }
func (s Store) RunsDir() string                { return filepath.Join(s.Base, "runs") }
func (s Store) WorktreesDir() string           { return filepath.Join(s.Base, "worktrees") }
func (s Store) PolicyDir() string              { return filepath.Join(s.Base, "policies") }
func (s Store) ProfilePath(name string) string { return filepath.Join(s.ProfilesDir(), name+".yaml") }
func (s Store) RunPath(id string) string       { return filepath.Join(s.RunsDir(), id+".json") }

func (s Store) SaveProfile(p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	b, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return atomicWrite(s.ProfilePath(p.Name), b)
}

func (s Store) LoadProfile(name string) (Profile, error) {
	if !safeName.MatchString(name) {
		return Profile{}, fmt.Errorf("invalid profile name")
	}
	b, err := os.ReadFile(s.ProfilePath(name))
	if err != nil {
		return Profile{}, err
	}
	return parseProfile(b, name)
}

func parseProfile(b []byte, name string) (Profile, error) {
	var p Profile
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return p, err
	}
	if p.Name != name {
		return p, fmt.Errorf("profile filename and name disagree")
	}
	return p, p.Validate()
}

func (s Store) EditProfile(ctx context.Context, name string) error {
	if _, err := s.LoadProfile(name); err != nil {
		return err
	}
	b, err := os.ReadFile(s.ProfilePath(name))
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.ProfilesDir(), "edit-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := RunEditor(ctx, f.Name()); err != nil {
		return err
	}
	next, err := os.ReadFile(f.Name())
	if err != nil {
		return err
	}
	if _, err := parseProfile(next, name); err != nil {
		return err
	}
	return atomicWrite(s.ProfilePath(name), next)
}

func (s Store) ListProfiles() ([]Profile, error) {
	entries, err := os.ReadDir(s.ProfilesDir())
	if errors.Is(err, os.ErrNotExist) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Profile
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		p, err := s.LoadProfile(strings.TrimSuffix(e.Name(), ".yaml"))
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s Store) SaveRun(r Run) error {
	if !safeName.MatchString(r.ID) {
		return fmt.Errorf("invalid run ID")
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(s.RunPath(r.ID), append(b, '\n'))
}

func (s Store) LoadRun(id string) (Run, error) {
	if !safeName.MatchString(id) {
		return Run{}, fmt.Errorf("invalid run ID")
	}
	b, err := os.ReadFile(s.RunPath(id))
	if err != nil {
		return Run{}, err
	}
	var r Run
	if err := json.Unmarshal(b, &r); err != nil {
		return r, err
	}
	if r.ID != id {
		return r, fmt.Errorf("run ID mismatch")
	}
	return r, nil
}

func (s Store) ListRuns() ([]Run, error) {
	entries, err := os.ReadDir(s.RunsDir())
	if errors.Is(err, os.ErrNotExist) {
		return []Run{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Run
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		r, err := s.LoadRun(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

func NewID() (string, error) {
	// OpenShell sandbox names currently permit at most 19 characters.
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "run-" + hex.EncodeToString(b[:]), nil
}

func atomicWrite(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".new-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func runtimeGOOS() string { return runtime.GOOS }

type Registry struct {
	Reference string `yaml:"reference" json:"reference"`
}

func (s Store) RegistryPath() string { return filepath.Join(s.Base, "registry.yaml") }

func (s Store) LoadRegistry() (Registry, error) {
	b, err := os.ReadFile(s.RegistryPath())
	if errors.Is(err, os.ErrNotExist) {
		return Registry{}, nil
	}
	if err != nil {
		return Registry{}, err
	}
	var registry Registry
	if err := yaml.Unmarshal(b, &registry); err != nil {
		return Registry{}, err
	}
	if err := registry.Validate(); err != nil {
		return Registry{}, err
	}
	return registry, nil
}

func (s Store) SaveRegistry(registry Registry) error {
	if err := registry.Validate(); err != nil {
		return err
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	b, err := yaml.Marshal(registry)
	if err != nil {
		return err
	}
	return atomicWrite(s.RegistryPath(), b)
}

func (s Store) ClearRegistry() error {
	if err := os.Remove(s.RegistryPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (r Registry) Validate() error {
	reference := strings.TrimSpace(r.Reference)
	if reference == "" {
		return fmt.Errorf("registry reference is required")
	}
	if strings.Contains(reference, "://") || strings.ContainsAny(reference, " \t\r\n") {
		return fmt.Errorf("registry reference must be an OCI repository without a scheme or spaces")
	}
	if strings.Contains(reference, "@") {
		return fmt.Errorf("registry reference must not include a digest")
	}
	if !strings.Contains(reference, "/") {
		return fmt.Errorf("registry reference must include a registry and repository path")
	}
	lastSlash := strings.LastIndex(reference, "/")
	if strings.Contains(reference[lastSlash+1:], ":") {
		return fmt.Errorf("registry reference must not include a tag")
	}
	return nil
}

func (r Registry) NormalizedReference() string {
	return strings.TrimSuffix(strings.TrimSpace(r.Reference), "/")
}
