package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// OrchestrationRegistryVersion is the only registry schema this binary reads.
const OrchestrationRegistryVersion = 1

// orchestrationStages is the stable order of dispatcher stages.
var orchestrationStages = []string{"triage", "plan", "implement", "fix", "verify", "review", "retro"}

// orchestrationStageGroups maps every stage to its fixed configuration group.
var orchestrationStageGroups = map[string]string{
	"triage":    "other",
	"plan":      "coding",
	"implement": "coding",
	"fix":       "coding",
	"verify":    "other",
	"review":    "other",
	"retro":     "other",
}

var orchestrationGroupNames = map[string]bool{"coding": true, "other": true}

// OrchestrationStageNames returns the configured stage names in stable order.
func OrchestrationStageNames() []string {
	out := make([]string, len(orchestrationStages))
	copy(out, orchestrationStages)
	return out
}

// OrchestrationProjectRef identifies the repository a registry is resolved for.
// Root is the canonical checkout root; Remote is an optional Git remote URL.
type OrchestrationProjectRef struct {
	Root   string
	Remote string
}

// OrchestrationOverride is one optional layer of runtime selection. A nil
// field is inherited; a non-nil empty string explicitly clears an inherited
// value.
type OrchestrationOverride struct {
	Runtime *string `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	Agent   *string `yaml:"agent,omitempty" json:"agent,omitempty"`
	Profile *string `yaml:"profile,omitempty" json:"profile,omitempty"`
	Model   *string `yaml:"model,omitempty" json:"model,omitempty"`
	Effort  *string `yaml:"effort,omitempty" json:"effort,omitempty"`
}

// OrchestrationWorkerConfig selects workers for all stages, a stage group, or
// one stage. Later fields override earlier ones within the same layer.
type OrchestrationWorkerConfig struct {
	Default *OrchestrationOverride           `yaml:"default,omitempty" json:"default,omitempty"`
	Groups  map[string]OrchestrationOverride `yaml:"groups,omitempty" json:"groups,omitempty"`
	Stages  map[string]OrchestrationOverride `yaml:"stages,omitempty" json:"stages,omitempty"`
}

// OrchestrationConfig is one named configuration.
type OrchestrationConfig struct {
	Orchestration *OrchestrationOverride     `yaml:"orchestration,omitempty" json:"orchestration,omitempty"`
	Workers       *OrchestrationWorkerConfig `yaml:"workers,omitempty" json:"workers,omitempty"`
}

// OrchestrationScope is a selectable named configuration plus inline overrides.
type OrchestrationScope struct {
	Config        *string                    `yaml:"config,omitempty" json:"config,omitempty"`
	Orchestration *OrchestrationOverride     `yaml:"orchestration,omitempty" json:"orchestration,omitempty"`
	Workers       *OrchestrationWorkerConfig `yaml:"workers,omitempty" json:"workers,omitempty"`
}

// OrchestrationProject is a repository-scoped configuration layer.
type OrchestrationProject struct {
	Name          string                     `yaml:"name" json:"name"`
	Root          string                     `yaml:"root,omitempty" json:"root,omitempty"`
	Remote        string                     `yaml:"remote,omitempty" json:"remote,omitempty"`
	Config        *string                    `yaml:"config,omitempty" json:"config,omitempty"`
	Orchestration *OrchestrationOverride     `yaml:"orchestration,omitempty" json:"orchestration,omitempty"`
	Workers       *OrchestrationWorkerConfig `yaml:"workers,omitempty" json:"workers,omitempty"`
}

// OrchestrationRegistry is the versioned, user-owned YAML registry.
type OrchestrationRegistry struct {
	Version  int                            `yaml:"version" json:"version"`
	Default  string                         `yaml:"default" json:"default"`
	Configs  map[string]OrchestrationConfig `yaml:"configs" json:"configs"`
	Global   *OrchestrationScope            `yaml:"global,omitempty" json:"global,omitempty"`
	Projects []OrchestrationProject         `yaml:"projects,omitempty" json:"projects,omitempty"`
}

// ResolvedOrchestrationSelection is one effective runtime selection and the
// source of each field.
type ResolvedOrchestrationSelection struct {
	Runtime string            `json:"runtime,omitempty"`
	Agent   string            `json:"agent,omitempty"`
	Profile string            `json:"profile,omitempty"`
	Model   string            `json:"model,omitempty"`
	Effort  string            `json:"effort,omitempty"`
	Source  string            `json:"source,omitempty"`
	Sources map[string]string `json:"sources,omitempty"`
}

// ResolvedOrchestration is the deterministic resolution of a registry for one
// repository. Scope is none, default, global, or project.
type ResolvedOrchestration struct {
	Path         string                                    `json:"path"`
	Present      bool                                      `json:"present"`
	Version      int                                       `json:"version,omitempty"`
	Default      string                                    `json:"default,omitempty"`
	Scope        string                                    `json:"scope"`
	Project      string                                    `json:"project,omitempty"`
	Sources      []string                                  `json:"sources,omitempty"`
	Orchestrator ResolvedOrchestrationSelection            `json:"orchestrator"`
	Stages       map[string]ResolvedOrchestrationSelection `json:"stages"`
	Warnings     []string                                  `json:"warnings,omitempty"`
}

// OrchestrationRegistryPath returns the registry path for an AgentBox state
// base directory.
func OrchestrationRegistryPath(base string) string {
	return filepath.Join(base, "orchestration.yaml")
}

// LoadOrchestrationRegistry reads and validates a registry. A missing file
// returns (nil, nil), which callers treat as no applicable configuration.
func LoadOrchestrationRegistry(path string) (*OrchestrationRegistry, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read orchestration registry %s: %w", path, err)
	}
	var registry OrchestrationRegistry
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	if err := decoder.Decode(&registry); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("orchestration registry %s is empty", path)
		}
		return nil, fmt.Errorf("orchestration registry %s is not valid YAML: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("orchestration registry %s must contain exactly one YAML document", path)
		}
		return nil, fmt.Errorf("orchestration registry %s has trailing content: %w", path, err)
	}
	if err := registry.Validate(); err != nil {
		return nil, fmt.Errorf("orchestration registry %s: %w", path, err)
	}
	return &registry, nil
}

// ResolveOrchestrationFile loads and resolves a registry. A missing file is a
// present=false result, not an error.
func ResolveOrchestrationFile(path string, ref OrchestrationProjectRef) (ResolvedOrchestration, error) {
	registry, err := LoadOrchestrationRegistry(path)
	if err != nil {
		return ResolvedOrchestration{}, err
	}
	return ResolveOrchestration(registry, path, ref)
}

// ResolveOrchestration resolves the registry for ref. It never guesses an
// ambiguous project match and returns an actionable error instead.
func ResolveOrchestration(registry *OrchestrationRegistry, path string, ref OrchestrationProjectRef) (ResolvedOrchestration, error) {
	if registry == nil {
		return ResolvedOrchestration{Path: path, Scope: "none", Stages: emptyStageSelections()}, nil
	}
	if err := registry.Validate(); err != nil {
		return ResolvedOrchestration{}, fmt.Errorf("orchestration registry %s: %w", path, err)
	}
	out := ResolvedOrchestration{
		Path:    path,
		Present: true,
		Version: registry.Version,
		Default: registry.Default,
		Scope:   "default",
		Stages:  emptyStageSelections(),
	}

	var layers []orchestrationLayer
	defaultConfig := registry.Configs[registry.Default]
	layers = append(layers, orchestrationLayer{id: "default:" + registry.Default, config: &defaultConfig})
	if registry.Global != nil {
		out.Scope = "global"
		if registry.Global.Config != nil {
			config := registry.Configs[*registry.Global.Config]
			layers = append(layers, orchestrationLayer{id: "global.config:" + *registry.Global.Config, config: &config})
		}
		if registry.Global.Orchestration != nil || registry.Global.Workers != nil {
			layers = append(layers, orchestrationLayer{
				id: "global",
				config: &OrchestrationConfig{
					Orchestration: registry.Global.Orchestration,
					Workers:       registry.Global.Workers,
				},
			})
		}
	}
	project, err := matchOrchestrationProject(registry, ref)
	if err != nil {
		return ResolvedOrchestration{}, err
	}
	if project != nil {
		out.Scope = "project"
		out.Project = project.Name
		if project.Config != nil {
			config := registry.Configs[*project.Config]
			layers = append(layers, orchestrationLayer{id: "project:" + project.Name + ".config:" + *project.Config, config: &config})
		}
		if project.Orchestration != nil || project.Workers != nil {
			layers = append(layers, orchestrationLayer{
				id: "project:" + project.Name,
				config: &OrchestrationConfig{
					Orchestration: project.Orchestration,
					Workers:       project.Workers,
				},
			})
		}
	}

	for _, layer := range layers {
		out.Sources = append(out.Sources, layer.id)
		if layer.config.Orchestration != nil {
			layer.config.Orchestration.apply(&out.Orchestrator, layer.id)
		}
		for _, stage := range orchestrationStages {
			selection := out.Stages[stage]
			applyWorkerSelection(&selection, stage, layer.config.Workers, layer.id)
			out.Stages[stage] = selection
		}
	}
	out.Warnings = orchestrationWarnings(out)
	if project == nil && (strings.TrimSpace(ref.Root) != "" || strings.TrimSpace(ref.Remote) != "") {
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"no project entry matched root %q or remote %q; resolved from the %s layer",
			strings.TrimSpace(ref.Root), strings.TrimSpace(ref.Remote), out.Scope))
	}
	return out, nil
}

type orchestrationLayer struct {
	id     string
	config *OrchestrationConfig
}

func emptyStageSelections() map[string]ResolvedOrchestrationSelection {
	out := make(map[string]ResolvedOrchestrationSelection, len(orchestrationStages))
	for _, stage := range orchestrationStages {
		out[stage] = ResolvedOrchestrationSelection{}
	}
	return out
}

func applyWorkerSelection(target *ResolvedOrchestrationSelection, stage string, workers *OrchestrationWorkerConfig, layer string) {
	if workers == nil {
		return
	}
	if workers.Default != nil {
		workers.Default.apply(target, layer+".default")
	}
	group := orchestrationStageGroups[stage]
	if override, ok := workers.Groups[group]; ok {
		override.apply(target, layer+".group:"+group)
	}
	if override, ok := workers.Stages[stage]; ok {
		override.apply(target, layer+".stage:"+stage)
	}
}

func (override OrchestrationOverride) apply(target *ResolvedOrchestrationSelection, source string) {
	changed := false
	if override.Runtime != nil {
		target.Runtime = strings.TrimSpace(*override.Runtime)
		target.setSource("runtime", source)
		changed = true
	}
	if override.Agent != nil {
		target.Agent = strings.TrimSpace(*override.Agent)
		target.setSource("agent", source)
		changed = true
	}
	if override.Profile != nil {
		target.Profile = strings.TrimSpace(*override.Profile)
		target.setSource("profile", source)
		changed = true
	}
	if override.Model != nil {
		target.Model = strings.TrimSpace(*override.Model)
		target.setSource("model", source)
		changed = true
	}
	if override.Effort != nil {
		target.Effort = strings.TrimSpace(*override.Effort)
		target.setSource("effort", source)
		changed = true
	}
	if changed {
		target.Source = source
	}
}

func (selection *ResolvedOrchestrationSelection) setSource(field, source string) {
	if selection.Sources == nil {
		selection.Sources = map[string]string{}
	}
	selection.Sources[field] = source
}

func orchestrationWarnings(resolved ResolvedOrchestration) []string {
	var warnings []string
	check := func(label string, selection ResolvedOrchestrationSelection) {
		if selection.Runtime == "agentbox" {
			if selection.Profile == "" {
				warnings = append(warnings, fmt.Sprintf("%s runs on agentbox but selects no profile", label))
			}
			if selection.Effort != "" {
				warnings = append(warnings, fmt.Sprintf("%s selects effort %q but AgentBox workers cannot apply an effort", label, selection.Effort))
			}
		}
		if selection.Runtime == "" && (selection.Agent != "" || selection.Profile != "" || selection.Model != "" || selection.Effort != "") {
			warnings = append(warnings, fmt.Sprintf("%s has worker settings but no runtime selection", label))
		}
	}
	check("orchestrator", resolved.Orchestrator)
	for _, stage := range orchestrationStages {
		check("stage "+stage, resolved.Stages[stage])
	}
	return warnings
}

func matchOrchestrationProject(registry *OrchestrationRegistry, ref OrchestrationProjectRef) (*OrchestrationProject, error) {
	root, err := cleanOrchestrationRefRoot(ref.Root)
	if err != nil {
		return nil, err
	}
	remote := normalizeGitRemote(ref.Remote)
	var rootMatches []projectMatch
	var remoteMatches []projectMatch
	for index := range registry.Projects {
		project := &registry.Projects[index]
		if project.Root != "" && root != "" {
			projectRoot, err := cleanOrchestrationRoot(project.Root)
			if err != nil {
				return nil, fmt.Errorf("orchestration project %q root: %w", project.Name, err)
			}
			if root == projectRoot || strings.HasPrefix(root, projectRoot+string(filepath.Separator)) {
				rootMatches = append(rootMatches, projectMatch{project: project, depth: len(projectRoot)})
			}
		}
		if remote != "" && project.Remote != "" && normalizeGitRemote(project.Remote) == remote {
			remoteMatches = append(remoteMatches, projectMatch{project: project})
		}
	}
	bestRoot := mostSpecificProject(rootMatches)
	if len(rootMatches) > 1 && countDeepest(rootMatches, bestRoot.depth) > 1 {
		return nil, ambiguousProjectError("root "+root, rootMatches)
	}
	bestRemote := mostSpecificProject(remoteMatches)
	if len(remoteMatches) > 1 {
		return nil, ambiguousProjectError("remote "+remote, remoteMatches)
	}
	if bestRoot.project != nil && bestRemote.project != nil && bestRoot.project != bestRemote.project {
		return nil, fmt.Errorf("ambiguous orchestration project match: root %s selects %q but remote %s selects %q; make the roots and remotes unique or remove one entry", root, bestRoot.project.Name, remote, bestRemote.project.Name)
	}
	if bestRoot.project != nil {
		return bestRoot.project, nil
	}
	return bestRemote.project, nil
}

type projectMatch struct {
	project *OrchestrationProject
	depth   int
}

func mostSpecificProject(matches []projectMatch) projectMatch {
	var best projectMatch
	for _, match := range matches {
		if match.project == nil {
			continue
		}
		if best.project == nil || match.depth > best.depth {
			best = match
		}
	}
	return best
}

func countDeepest(matches []projectMatch, depth int) int {
	count := 0
	for _, match := range matches {
		if match.depth == depth {
			count++
		}
	}
	return count
}

func ambiguousProjectError(subject string, matches []projectMatch) error {
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match.project.Name)
	}
	sort.Strings(names)
	return fmt.Errorf("ambiguous orchestration project match for %s: %s; make the project roots and remotes unique or remove one entry", subject, strings.Join(names, ", "))
}

func cleanOrchestrationRefRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", nil
	}
	root = expandOrchestrationHome(root)
	if !filepath.IsAbs(root) {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return "", fmt.Errorf("resolve project root %q: %w", root, err)
		}
		root = absolute
	}
	return filepath.Clean(root), nil
}

func cleanOrchestrationRoot(root string) (string, error) {
	expanded := expandOrchestrationHome(strings.TrimSpace(root))
	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("root %q must be an absolute path", root)
	}
	return filepath.Clean(expanded), nil
}

func expandOrchestrationHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// normalizeGitRemote reduces common Git remote forms to
// host/owner/repo for deterministic comparison.
func normalizeGitRemote(remote string) string {
	remote = strings.TrimSpace(remote)
	remote = strings.TrimSuffix(remote, "/")
	remote = strings.TrimSuffix(remote, ".git")
	for _, prefix := range []string{"ssh://", "git://", "https://", "http://"} {
		remote = strings.TrimPrefix(remote, prefix)
	}
	if at := strings.Index(remote, "@"); at >= 0 && !strings.Contains(remote[:at], "/") {
		remote = remote[at+1:]
	}
	remote = strings.Replace(remote, ":", "/", 1)
	return strings.ToLower(remote)
}

// Validate checks the versioned schema strictly. Unknown fields are rejected
// by the decoder; this method rejects unknown stages, groups, runtimes, agents,
// and dangling config references.
func (registry *OrchestrationRegistry) Validate() error {
	if registry.Version != OrchestrationRegistryVersion {
		return fmt.Errorf("version must be %d", OrchestrationRegistryVersion)
	}
	if strings.TrimSpace(registry.Default) == "" {
		return fmt.Errorf("default must name a configuration")
	}
	if !safeName.MatchString(registry.Default) {
		return fmt.Errorf("default %q must be a lower-case identifier", registry.Default)
	}
	if len(registry.Configs) == 0 {
		return fmt.Errorf("configs must define at least one configuration")
	}
	for _, name := range sortedConfigNames(registry.Configs) {
		if !safeName.MatchString(name) {
			return fmt.Errorf("config name %q must be a lower-case identifier", name)
		}
		if err := registry.Configs[name].validate("configs." + name); err != nil {
			return err
		}
	}
	if _, ok := registry.Configs[registry.Default]; !ok {
		return fmt.Errorf("default %q is not defined in configs", registry.Default)
	}
	if registry.Global != nil {
		if err := registry.Global.validate(registry.Configs, "global"); err != nil {
			return err
		}
	}
	names := map[string]bool{}
	for index, project := range registry.Projects {
		path := fmt.Sprintf("projects[%d]", index)
		if !safeName.MatchString(project.Name) {
			return fmt.Errorf("%s.name %q must be a lower-case identifier", path, project.Name)
		}
		if names[project.Name] {
			return fmt.Errorf("project name %q is defined twice", project.Name)
		}
		names[project.Name] = true
		if strings.TrimSpace(project.Root) == "" && strings.TrimSpace(project.Remote) == "" {
			return fmt.Errorf("%s needs root or remote", path)
		}
		if project.Root != "" {
			if _, err := cleanOrchestrationRoot(project.Root); err != nil {
				return fmt.Errorf("%s.root: %w", path, err)
			}
		}
		if remote := strings.TrimSpace(project.Remote); remote != "" {
			if orchestrationRemoteHasCredentials(remote) {
				return fmt.Errorf("%s.remote must not contain credentials; use a remote URL without userinfo", path)
			}
			if hasOrchestrationControl(remote) {
				return fmt.Errorf("%s.remote must not contain control characters", path)
			}
		}
		if project.Config != nil {
			if err := validateConfigRef(registry.Configs, *project.Config, path+".config"); err != nil {
				return err
			}
		}
		if err := validateOverride(project.Orchestration, path+".orchestration"); err != nil {
			return err
		}
		if err := validateWorkers(project.Workers, path+".workers"); err != nil {
			return err
		}
	}
	return nil
}

func (scope *OrchestrationScope) validate(configs map[string]OrchestrationConfig, path string) error {
	if scope.Config != nil {
		if err := validateConfigRef(configs, *scope.Config, path+".config"); err != nil {
			return err
		}
	}
	if err := validateOverride(scope.Orchestration, path+".orchestration"); err != nil {
		return err
	}
	return validateWorkers(scope.Workers, path+".workers")
}

func (config OrchestrationConfig) validate(path string) error {
	if err := validateOverride(config.Orchestration, path+".orchestration"); err != nil {
		return err
	}
	return validateWorkers(config.Workers, path+".workers")
}

func validateConfigRef(configs map[string]OrchestrationConfig, name, path string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("%s names an empty configuration", path)
	}
	if !safeName.MatchString(trimmed) {
		return fmt.Errorf("%s %q must be a lower-case identifier", path, name)
	}
	if _, ok := configs[trimmed]; !ok {
		return fmt.Errorf("%s %q is not defined in configs", path, name)
	}
	return nil
}

func validateWorkers(workers *OrchestrationWorkerConfig, path string) error {
	if workers == nil {
		return nil
	}
	if err := validateOverride(workers.Default, path+".default"); err != nil {
		return err
	}
	for _, group := range sortedOverrideKeys(workers.Groups) {
		if !orchestrationGroupNames[group] {
			return fmt.Errorf("%s.groups.%s is not a stage group (use coding or other)", path, group)
		}
		override := workers.Groups[group]
		if err := validateOverride(&override, path+".groups."+group); err != nil {
			return err
		}
	}
	for _, stage := range sortedOverrideKeys(workers.Stages) {
		if _, ok := orchestrationStageGroups[stage]; !ok {
			return fmt.Errorf("%s.stages.%s is not a stage (use %s)", path, stage, strings.Join(orchestrationStages, ", "))
		}
		override := workers.Stages[stage]
		if err := validateOverride(&override, path+".stages."+stage); err != nil {
			return err
		}
	}
	return nil
}

func validateOverride(override *OrchestrationOverride, path string) error {
	if override == nil {
		return nil
	}
	if override.Runtime != nil {
		value := strings.TrimSpace(*override.Runtime)
		if value != "" && value != "orca" && value != "agentbox" && value != "harness" {
			return fmt.Errorf("%s.runtime %q must be orca, agentbox, or harness", path, *override.Runtime)
		}
	}
	if override.Agent != nil {
		value := strings.TrimSpace(*override.Agent)
		if value != "" && value != "claude" && value != "codex" && value != "opencode" {
			return fmt.Errorf("%s.agent %q must be claude, codex, or opencode", path, *override.Agent)
		}
	}
	if override.Profile != nil {
		value := strings.TrimSpace(*override.Profile)
		if value != "" && !safeName.MatchString(value) {
			return fmt.Errorf("%s.profile %q must be a lower-case identifier", path, *override.Profile)
		}
	}
	for _, field := range []struct {
		name  string
		value *string
	}{{"model", override.Model}, {"effort", override.Effort}} {
		if field.value == nil {
			continue
		}
		trimmed := strings.TrimSpace(*field.value)
		if strings.ContainsAny(trimmed, " \t\r\n") || hasOrchestrationControl(trimmed) {
			return fmt.Errorf("%s.%s %q must not contain whitespace or control characters", path, field.name, *field.value)
		}
	}
	return nil
}

func sortedConfigNames(configs map[string]OrchestrationConfig) []string {
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedOverrideKeys(overrides map[string]OrchestrationOverride) []string {
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func hasOrchestrationControl(value string) bool {
	return strings.ContainsFunc(value, unicode.IsControl)
}

func orchestrationRemoteHasCredentials(remote string) bool {
	if !strings.Contains(remote, "://") {
		return false
	}
	parsed, err := url.Parse(remote)
	if err != nil || parsed.User == nil {
		return false
	}
	_, hasPassword := parsed.User.Password()
	return hasPassword
}
