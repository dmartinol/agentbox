package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skills is a named selection of standalone host skills and whole plugin
// skill bundles for one agent. AgentBox stores only names; skill contents
// stay in the agent's own host directories until run start.
type Skills struct {
	Version int      `yaml:"version" json:"version"`
	Name    string   `yaml:"name" json:"name"`
	Agent   string   `yaml:"agent" json:"agent"`
	Skills  []string `yaml:"skills,omitempty" json:"skills"`
	Plugins []string `yaml:"plugins,omitempty" json:"plugins"`
}

// normalized guarantees that the JSON API always emits arrays instead of null
// for the two selection lists, so clients can safely iterate and count them.
func (s Skills) normalized() Skills {
	if s.Skills == nil {
		s.Skills = []string{}
	}
	if s.Plugins == nil {
		s.Plugins = []string{}
	}
	return s
}

// skillToken allows path-free names such as orca-cli and browser@openai-bundled
// parts. A leading dot is rejected, so ".." cannot pass as a token.
var skillToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func (s Skills) Validate() error {
	if s.Version != 1 || !safeName.MatchString(s.Name) {
		return fmt.Errorf("skills entry needs version 1 and a lower-case name")
	}
	if s.Agent != "claude" && s.Agent != "codex" && s.Agent != "opencode" {
		return fmt.Errorf("agent must be claude, codex, or opencode")
	}
	if len(s.Skills) == 0 && len(s.Plugins) == 0 {
		return fmt.Errorf("skills entry needs at least one skill or plugin")
	}
	seen := make(map[string]struct{}, len(s.Skills)+len(s.Plugins))
	for _, skill := range s.Skills {
		if !skillToken.MatchString(skill) {
			return fmt.Errorf("skill name %q must be a path-free token using letters, digits, dots, hyphens, or underscores", skill)
		}
		if _, ok := seen[skill]; ok {
			return fmt.Errorf("entry %q is listed twice", skill)
		}
		seen[skill] = struct{}{}
	}
	for _, plugin := range s.Plugins {
		parts := strings.Split(plugin, "@")
		if len(parts) != 2 || !skillToken.MatchString(parts[0]) || !skillToken.MatchString(parts[1]) {
			return fmt.Errorf("plugin %q must use the path-free <plugin>@<marketplace> form", plugin)
		}
		if _, ok := seen[plugin]; ok {
			return fmt.Errorf("entry %q is listed twice", plugin)
		}
		seen[plugin] = struct{}{}
	}
	return nil
}

// SkillInfo is one host skill exposed for selection. Body contents are never
// returned, logged, or rendered.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PluginInfo is one host plugin skill bundle exposed for selection.
type PluginInfo struct {
	ID          string `json:"id"`
	Plugin      string `json:"plugin"`
	Marketplace string `json:"marketplace"`
	Skills      int    `json:"skills"`
}

// AvailableSkills is the read-only discovery result for one agent.
type AvailableSkills struct {
	Agent   string       `json:"agent"`
	Skills  []SkillInfo  `json:"skills"`
	Plugins []PluginInfo `json:"plugins"`
}

type skillSource struct {
	Name string
	Path string
}

type pluginSource struct {
	ID          string
	Plugin      string
	Marketplace string
	Skills      []skillSource
}

type skillCatalog struct {
	Skills  []skillSource
	Plugins []pluginSource
}

// SkillsSelection is the resolved outcome of a skills entry for one run.
type SkillsSelection struct {
	Entry          Skills
	Sources        []skillSource
	MissingSkills  []string
	MissingPlugins []string
	EmptyPlugins   []string
	Duplicates     []string
}

func (s SkillsSelection) Warnings() []string {
	var warnings []string
	missingSkills := append([]string(nil), s.MissingSkills...)
	sort.Strings(missingSkills)
	for _, name := range missingSkills {
		warnings = append(warnings, fmt.Sprintf("skill %q from entry %q is not available for %s on this host; starting without it", name, s.Entry.Name, s.Entry.Agent))
	}
	missingPlugins := append([]string(nil), s.MissingPlugins...)
	sort.Strings(missingPlugins)
	for _, name := range missingPlugins {
		warnings = append(warnings, fmt.Sprintf("plugin %q from entry %q is not available for %s on this host; starting without it", name, s.Entry.Name, s.Entry.Agent))
	}
	for _, name := range s.EmptyPlugins {
		warnings = append(warnings, fmt.Sprintf("plugin %q from entry %q contains no skills; nothing to deliver", name, s.Entry.Name))
	}
	for _, name := range s.Duplicates {
		warnings = append(warnings, fmt.Sprintf("skill %q from entry %q is provided more than once; delivering the first copy", name, s.Entry.Name))
	}
	return warnings
}

func (s Store) SkillsDir() string            { return filepath.Join(s.Base, "skills") }
func (s Store) SkillPath(name string) string { return filepath.Join(s.SkillsDir(), name+".yaml") }

func (s Store) SaveSkills(entry Skills) error {
	entry = entry.normalized()
	if err := entry.Validate(); err != nil {
		return err
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	b, err := yaml.Marshal(entry)
	if err != nil {
		return err
	}
	return atomicWrite(s.SkillPath(entry.Name), b)
}

func (s Store) LoadSkills(name string) (Skills, error) {
	if !safeName.MatchString(name) {
		return Skills{}, fmt.Errorf("invalid skills entry name")
	}
	b, err := os.ReadFile(s.SkillPath(name))
	if err != nil {
		return Skills{}, err
	}
	var entry Skills
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&entry); err != nil {
		return entry, err
	}
	if entry.Name != name {
		return entry, fmt.Errorf("skills filename and name disagree")
	}
	entry = entry.normalized()
	return entry, entry.Validate()
}

func (s Store) ListSkills() ([]Skills, error) {
	entries, err := os.ReadDir(s.SkillsDir())
	if errors.Is(err, os.ErrNotExist) {
		return []Skills{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]Skills, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		entry, err := s.LoadSkills(strings.TrimSuffix(e.Name(), ".yaml"))
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s Store) DeleteSkills(name string) error {
	if !safeName.MatchString(name) {
		return fmt.Errorf("invalid skills entry name")
	}
	if err := os.Remove(s.SkillPath(name)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("skills entry %q not found", name)
		}
		return err
	}
	return nil
}

// DiscoverSkills lists the standalone skills and plugin skill bundles visible
// to an agent on this host. Missing source directories yield empty lists.
func (a *App) DiscoverSkills(agent string) (AvailableSkills, error) {
	catalog, err := discoverSkillCatalog(agent)
	if err != nil {
		return AvailableSkills{}, err
	}
	out := AvailableSkills{Agent: agent, Skills: []SkillInfo{}, Plugins: []PluginInfo{}}
	for _, source := range catalog.Skills {
		out.Skills = append(out.Skills, SkillInfo{Name: source.Name, Description: readSkillDescription(filepath.Join(source.Path, "SKILL.md"))})
	}
	for _, plugin := range catalog.Plugins {
		out.Plugins = append(out.Plugins, PluginInfo{ID: plugin.ID, Plugin: plugin.Plugin, Marketplace: plugin.Marketplace, Skills: len(plugin.Skills)})
	}
	return out, nil
}

// ResolveSkillsSelection loads a skills entry, enforces agent compatibility
// with the run profile, and resolves selected skills and plugin bundles to
// host directories. Missing entries produce warnings, not errors.
func (a *App) ResolveSkillsSelection(name, agent string) (SkillsSelection, error) {
	entry, err := a.Store.LoadSkills(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SkillsSelection{}, fmt.Errorf("skills entry %q not found; create it from the Skills page or with `agentbox skill create`", name)
		}
		return SkillsSelection{}, err
	}
	if entry.Agent != agent {
		return SkillsSelection{}, fmt.Errorf("skills entry %q selects agent %q but the run profile uses %q", name, entry.Agent, agent)
	}
	catalog, err := discoverSkillCatalog(agent)
	if err != nil {
		return SkillsSelection{}, err
	}
	skills := make(map[string]skillSource, len(catalog.Skills))
	for _, source := range catalog.Skills {
		skills[source.Name] = source
	}
	plugins := make(map[string]pluginSource, len(catalog.Plugins))
	for _, plugin := range catalog.Plugins {
		plugins[plugin.ID] = plugin
	}
	selection := SkillsSelection{Entry: entry}
	seen := make(map[string]struct{})
	add := func(source skillSource) {
		if _, ok := seen[source.Name]; ok {
			selection.Duplicates = append(selection.Duplicates, source.Name)
			return
		}
		seen[source.Name] = struct{}{}
		selection.Sources = append(selection.Sources, source)
	}
	for _, want := range entry.Skills {
		source, ok := skills[want]
		if !ok {
			selection.MissingSkills = append(selection.MissingSkills, want)
			continue
		}
		add(source)
	}
	for _, want := range entry.Plugins {
		plugin, ok := plugins[want]
		if !ok {
			selection.MissingPlugins = append(selection.MissingPlugins, want)
			continue
		}
		if len(plugin.Skills) == 0 {
			selection.EmptyPlugins = append(selection.EmptyPlugins, want)
			continue
		}
		for _, source := range plugin.Skills {
			add(source)
		}
	}
	return selection, nil
}

func discoverSkillCatalog(agent string) (skillCatalog, error) {
	base, err := agentSkillBase(agent)
	if err != nil {
		return skillCatalog{}, err
	}
	catalog := skillCatalog{}
	for _, source := range readSkillSources(filepath.Join(base, "skills"), agent == "codex") {
		catalog.Skills = append(catalog.Skills, source)
	}
	switch agent {
	case "claude":
		catalog.Plugins, err = discoverClaudePlugins(base)
	case "codex":
		catalog.Plugins, err = discoverCodexPlugins(base)
	case "opencode":
		catalog.Plugins = []pluginSource{}
	}
	if err != nil {
		return skillCatalog{}, err
	}
	sort.Slice(catalog.Skills, func(i, j int) bool { return catalog.Skills[i].Name < catalog.Skills[j].Name })
	sort.Slice(catalog.Plugins, func(i, j int) bool { return catalog.Plugins[i].ID < catalog.Plugins[j].ID })
	return catalog, nil
}

func agentSkillBase(agent string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch agent {
	case "claude":
		if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
			return dir, nil
		}
		return filepath.Join(home, ".claude"), nil
	case "codex":
		if dir := os.Getenv("CODEX_HOME"); dir != "" {
			return dir, nil
		}
		return filepath.Join(home, ".codex"), nil
	case "opencode":
		dir := os.Getenv("XDG_CONFIG_HOME")
		if dir == "" {
			dir = filepath.Join(home, ".config")
		}
		return filepath.Join(dir, "opencode"), nil
	}
	return "", fmt.Errorf("agent must be claude, codex, or opencode")
}

func readSkillSources(root string, skipHidden bool) []skillSource {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []skillSource
	seen := make(map[string]struct{})
	for _, entry := range entries {
		if skipHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		if info, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil || info.IsDir() {
			continue
		}
		if _, ok := seen[entry.Name()]; ok {
			continue
		}
		seen[entry.Name()] = struct{}{}
		out = append(out, skillSource{Name: entry.Name(), Path: path})
	}
	return out
}

func discoverClaudePlugins(base string) ([]pluginSource, error) {
	plugins := map[string]*pluginSource{}
	add := func(marketplace, plugin, dir string) {
		id := plugin + "@" + marketplace
		target := plugins[id]
		if target == nil {
			target = &pluginSource{ID: id, Plugin: plugin, Marketplace: marketplace, Skills: []skillSource{}}
			plugins[id] = target
		}
		seen := make(map[string]struct{}, len(target.Skills))
		for _, skill := range target.Skills {
			seen[skill.Name] = struct{}{}
		}
		for _, skill := range readSkillSources(filepath.Join(dir, "skills"), false) {
			if _, ok := seen[skill.Name]; ok {
				continue
			}
			seen[skill.Name] = struct{}{}
			target.Skills = append(target.Skills, skill)
		}
	}

	marketplacesRoot := filepath.Join(base, "plugins", "marketplaces")
	if entries, err := os.ReadDir(marketplacesRoot); err == nil {
		for _, marketplace := range entries {
			if !marketplace.IsDir() {
				continue
			}
			marketplaceDir := filepath.Join(marketplacesRoot, marketplace.Name())
			children, err := os.ReadDir(marketplaceDir)
			if err != nil {
				return nil, err
			}
			for _, plugin := range children {
				if plugin.IsDir() {
					add(marketplace.Name(), plugin.Name(), filepath.Join(marketplaceDir, plugin.Name()))
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	cacheRoot := filepath.Join(base, "plugins", "cache")
	if entries, err := os.ReadDir(cacheRoot); err == nil {
		for _, marketplace := range entries {
			if !marketplace.IsDir() {
				continue
			}
			marketplaceDir := filepath.Join(cacheRoot, marketplace.Name())
			pluginsInMarketplace, err := os.ReadDir(marketplaceDir)
			if err != nil {
				return nil, err
			}
			for _, plugin := range pluginsInMarketplace {
				if !plugin.IsDir() {
					continue
				}
				pluginDir := filepath.Join(marketplaceDir, plugin.Name())
				versions, err := os.ReadDir(pluginDir)
				if err != nil {
					return nil, err
				}
				for _, version := range versions {
					if version.IsDir() {
						add(marketplace.Name(), plugin.Name(), filepath.Join(pluginDir, version.Name()))
					}
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	return sortedPluginSources(plugins), nil
}

func discoverCodexPlugins(base string) ([]pluginSource, error) {
	plugins := map[string]*pluginSource{}
	cacheRoot := filepath.Join(base, "plugins", "cache")
	entries, err := os.ReadDir(cacheRoot)
	if errors.Is(err, os.ErrNotExist) {
		return []pluginSource{}, nil
	}
	if err != nil {
		return nil, err
	}
	for _, marketplace := range entries {
		if !marketplace.IsDir() {
			continue
		}
		marketplaceDir := filepath.Join(cacheRoot, marketplace.Name())
		pluginsInMarketplace, err := os.ReadDir(marketplaceDir)
		if err != nil {
			return nil, err
		}
		for _, plugin := range pluginsInMarketplace {
			if !plugin.IsDir() {
				continue
			}
			pluginDir := filepath.Join(marketplaceDir, plugin.Name())
			versions, err := os.ReadDir(pluginDir)
			if err != nil {
				return nil, err
			}
			id := plugin.Name() + "@" + marketplace.Name()
			target := plugins[id]
			if target == nil {
				target = &pluginSource{ID: id, Plugin: plugin.Name(), Marketplace: marketplace.Name(), Skills: []skillSource{}}
				plugins[id] = target
			}
			seen := make(map[string]struct{}, len(target.Skills))
			for _, skill := range target.Skills {
				seen[skill.Name] = struct{}{}
			}
			for _, version := range versions {
				if !version.IsDir() {
					continue
				}
				for _, skill := range readSkillSources(filepath.Join(pluginDir, version.Name(), "skills"), false) {
					if _, ok := seen[skill.Name]; ok {
						continue
					}
					seen[skill.Name] = struct{}{}
					target.Skills = append(target.Skills, skill)
				}
			}
		}
	}
	return sortedPluginSources(plugins), nil
}

func sortedPluginSources(plugins map[string]*pluginSource) []pluginSource {
	out := make([]pluginSource, 0, len(plugins))
	for _, plugin := range plugins {
		sort.Slice(plugin.Skills, func(i, j int) bool { return plugin.Skills[i].Name < plugin.Skills[j].Name })
		out = append(out, *plugin)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// readSkillDescription extracts only the description field from a SKILL.md
// YAML frontmatter block. The body is never returned.
func readSkillDescription(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimPrefix(string(body), "\ufeff"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return ""
	}
	var meta struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.Description)
}

func skillDeliveryDir(agent, stateDir string) string {
	switch agent {
	case "claude":
		return filepath.Join(stateDir, "home", ".claude", "skills")
	case "codex":
		return filepath.Join(stateDir, "home", ".codex", "skills")
	case "opencode":
		return filepath.Join(stateDir, "config", "opencode", "skills")
	}
	return ""
}

func deliverSkills(stateDir string, selection *SkillsSelection) error {
	if selection == nil || len(selection.Sources) == 0 {
		return nil
	}
	root := skillDeliveryDir(selection.Entry.Agent, stateDir)
	if root == "" {
		return fmt.Errorf("agent %q has no skills directory", selection.Entry.Agent)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	for _, source := range selection.Sources {
		if err := copySkillTree(source.Path, filepath.Join(root, source.Name)); err != nil {
			return fmt.Errorf("deliver skill %q: %w", source.Name, err)
		}
	}
	return nil
}

func copySkillTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()|0700); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		entryInfo, err := os.Stat(srcPath)
		if err != nil {
			return err
		}
		if entryInfo.IsDir() {
			if err := copySkillTree(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		if err := copySkillFile(srcPath, dstPath, entryInfo.Mode()); err != nil {
			return err
		}
	}
	return os.Chmod(dst, info.Mode().Perm())
}

func copySkillFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode.Perm())
}
