package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (a *App) ProjectUsage() (map[string][]string, error) {
	usage := make(map[string][]string)
	runs, err := a.Store.ListRuns()
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		if run.Project == "" {
			continue
		}
		usage[run.Project] = append(usage[run.Project], runDisplayName(run))
	}
	for project := range usage {
		sort.Strings(usage[project])
	}
	return usage, nil
}

func (a *App) DeleteProject(name string) error {
	if _, err := a.Store.LoadProject(name); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &NotFoundError{Resource: "project", Name: name}
		}
		return err
	}
	usage, err := a.ProjectUsage()
	if err != nil {
		return err
	}
	if dependents := usage[name]; len(dependents) > 0 {
		return &ConflictError{Resource: "project", Name: name, Reason: "runs reference it", Dependents: dependents}
	}
	return a.Store.DeleteProject(name)
}

func (a *App) RegisterProject(ctx context.Context, name, root string, worktree bool) (Project, error) {
	canonical, err := a.ProjectRoot(ctx, root)
	if err != nil {
		return Project{}, err
	}
	value := worktree
	project := Project{
		Version:  1,
		Name:     name,
		Root:     canonical,
		Worktree: &value,
	}
	if err := a.Store.SaveProject(project); err != nil {
		return Project{}, err
	}
	return project, nil
}

func (a *App) ProjectRoot(ctx context.Context, root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	body, err := a.git(ctx, absolute, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("%s is not a Git working tree: %w", absolute, err)
	}
	canonical := filepath.Clean(strings.TrimSpace(string(body)))
	if resolved, err := filepath.EvalSymlinks(canonical); err == nil {
		canonical = filepath.Clean(resolved)
	}
	if info, err := os.Stat(canonical); err != nil || !info.IsDir() {
		return "", fmt.Errorf("project root is unavailable")
	}
	return canonical, nil
}
