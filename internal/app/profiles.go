package app

import (
	"errors"
	"os"
	"sort"
)

type ProfileUsage struct {
	UsedBy map[string][]string
}

func (a *App) ProfileUsage() (ProfileUsage, error) {
	runs, err := a.Store.ListRuns()
	if err != nil {
		return ProfileUsage{}, err
	}
	usage := ProfileUsage{
		UsedBy: make(map[string][]string),
	}
	for _, run := range runs {
		if run.ProfileName == "" || terminalRunStatus(run.Status) {
			continue
		}
		usage.UsedBy[run.ProfileName] = append(usage.UsedBy[run.ProfileName], runDisplayName(run))
	}
	for name := range usage.UsedBy {
		sort.Strings(usage.UsedBy[name])
	}
	return usage, nil
}

func (a *App) DeleteProfile(name string) error {
	if _, err := a.Store.LoadProfile(name); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &NotFoundError{Resource: "profile", Name: name}
		}
		return err
	}
	usage, err := a.ProfileUsage()
	if err != nil {
		return err
	}
	if dependents := usage.UsedBy[name]; len(dependents) > 0 {
		return &ConflictError{Resource: "profile", Name: name, Reason: "non-terminal runs use it", Dependents: dependents}
	}
	return a.Store.DeleteProfile(name)
}

func runDisplayName(run Run) string {
	if run.Name != "" {
		return run.Name
	}
	return run.ID
}
