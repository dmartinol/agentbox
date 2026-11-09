package tui

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/rivo/tview"
)

func (ui *UI) imageNames() []string {
	images, err := ui.app.LocalImages(ui.ctx)
	if err != nil {
		return nil
	}
	var names []string
	for _, image := range images {
		if image.Reference != "" {
			names = append(names, image.Reference)
		}
	}
	return uniqueSorted(names)
}

func (ui *UI) projectNames() []string {
	projects, err := ui.app.Store.ListProjects()
	if err != nil {
		return nil
	}
	var names []string
	for _, project := range projects {
		names = append(names, project.Name)
	}
	return uniqueSorted(names)
}

func (ui *UI) profileNames() []string {
	profiles, err := ui.app.Store.ListProfiles()
	if err != nil {
		return nil
	}
	var names []string
	for _, profile := range profiles {
		names = append(names, profile.Name)
	}
	return uniqueSorted(names)
}

func (ui *UI) gatewayNames() []string {
	data, err := ui.app.GatewayList(ui.ctx)
	if err != nil {
		return nil
	}
	rows, err := newGatewayRows(data)
	if err != nil {
		return nil
	}
	var names []string
	for _, row := range rows {
		if row.Name != "" {
			names = append(names, row.Name)
		}
	}
	return uniqueSorted(names)
}

func (ui *UI) providerNames(gateway string) []string {
	data, err := ui.app.ProviderList(ui.ctx, gateway)
	if err != nil {
		return nil
	}
	var payload struct {
		Providers []struct {
			Name string `json:"name"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	var names []string
	for _, provider := range payload.Providers {
		if provider.Name != "" {
			names = append(names, provider.Name)
		}
	}
	return uniqueSorted(names)
}

func (ui *UI) modelSuggestions() []string {
	var models []string
	profiles, err := ui.app.Store.ListProfiles()
	if err == nil {
		for _, profile := range profiles {
			if profile.Model != "" {
				models = append(models, profile.Model)
			}
		}
	}
	for _, profile := range profiles {
		if profile.Mode == "gateway" && profile.BaseURL != "" && profile.APIKeyEnv != "" {
			if discovered, err := ui.app.GatewayModels(ui.ctx, profile.BaseURL, profile.APIKeyEnv); err == nil {
				models = append(models, discovered...)
			}
		}
	}
	return uniqueSorted(models)
}

func (ui *UI) policyPaths() []string {
	policies, err := ui.app.Store.ListPolicyConfigurations()
	if err != nil {
		return nil
	}
	var paths []string
	for _, policy := range policies {
		paths = append(paths, policy.Path)
	}
	return uniqueSorted(paths)
}

func (ui *UI) runIDs() []string {
	runs, err := ui.app.Store.ListRuns()
	if err != nil {
		return nil
	}
	var ids []string
	for _, run := range runs {
		ids = append(ids, run.ID)
	}
	return ids
}

func autocompleteInput(label, text string, width int, options []string, changed func(string)) *tview.InputField {
	field := tview.NewInputField().
		SetLabel(label).
		SetText(text).
		SetFieldWidth(width).
		SetChangedFunc(changed)
	if len(options) == 0 {
		return field
	}
	field.SetAutocompleteFunc(func(current string) []string {
		needle := strings.ToLower(strings.TrimSpace(current))
		var entries []string
		for _, option := range options {
			if needle == "" || strings.Contains(strings.ToLower(option), needle) {
				entries = append(entries, option)
			}
		}
		return entries
	})
	return field
}

func uniqueSorted(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func optionIndex(options []string, value string) int {
	for i, option := range options {
		if option == value {
			return i
		}
	}
	return 0
}

func (ui *UI) registryReferences() []string {
	registry, err := ui.app.Registry()
	if err != nil {
		return nil
	}
	var refs []string
	if registry.Reference != "" {
		refs = append(refs, registry.NormalizedReference())
	}
	for _, image := range ui.registryImages {
		if image.Reference != "" {
			refs = append(refs, image.Reference)
		}
	}
	return uniqueSorted(refs)
}
