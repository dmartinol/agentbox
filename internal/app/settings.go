package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) EffectiveSettings() (Settings, error) {
	return a.Store.LoadSettings()
}

func (a *App) SaveGlobalSettings(root, branchPrefix string) (Settings, error) {
	root = strings.TrimSpace(root)
	if strings.HasPrefix(root, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return Settings{}, err
		}
		root = filepath.Join(home, strings.TrimPrefix(root, "~/"))
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return Settings{}, err
	}
	settings := Settings{
		Version: 1,
		Worktrees: WorktreeSettings{
			Root:         filepath.Clean(absolute),
			BranchPrefix: strings.TrimSpace(branchPrefix),
		},
	}
	if err := settings.Validate(); err != nil {
		return Settings{}, err
	}
	if err := os.MkdirAll(settings.Worktrees.Root, 0700); err != nil {
		return Settings{}, err
	}
	if err := a.Store.SaveSettings(settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func (a *App) WorktreeLocation(project, name, id string) (string, string, error) {
	settings, err := a.EffectiveSettings()
	if err != nil {
		return "", "", err
	}
	leaf := strings.TrimSpace(name)
	if leaf == "" {
		leaf = id
	}
	if !safeName.MatchString(leaf) {
		return "", "", fmt.Errorf("run name must be a lower-case identifier")
	}
	location := settings.Worktrees.Root
	branch := settings.Worktrees.BranchPrefix
	if project != "" {
		location = filepath.Join(location, project)
		branch += "/" + project
	}
	location = filepath.Join(location, leaf)
	branch += "/" + leaf
	return location, branch, nil
}

func (a *App) validateRunName(project, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if !safeName.MatchString(name) {
		return fmt.Errorf("run name must be a lower-case identifier")
	}
	runs, err := a.Store.ListRuns()
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.Project == project && run.Name == name {
			return fmt.Errorf("run name %q already exists in project %q", name, project)
		}
	}
	return nil
}
