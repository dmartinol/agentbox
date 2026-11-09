package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func (a *App) Registry() (Registry, error) {
	return a.Store.LoadRegistry()
}

func (a *App) SetRegistry(reference string) error {
	return a.Store.SaveRegistry(Registry{Reference: reference})
}

func (a *App) ClearRegistry() error {
	return a.Store.ClearRegistry()
}

// RegistryDiscover lists tags for the configured registry repository.
// It uses Podman's existing login state and does not handle credentials.
func (a *App) RegistryDiscover(ctx context.Context) ([]string, error) {
	registry, err := a.Store.LoadRegistry()
	if err != nil {
		return nil, err
	}
	if registry.Reference == "" {
		return nil, fmt.Errorf("no external registry configured")
	}
	reference := registry.NormalizedReference()
	data, err := a.Runner.Output(ctx, "podman", "search", "--list-tags", "--format", "json", reference)
	if err != nil {
		return nil, err
	}
	images, err := parseRegistrySearch(data, reference)
	if err != nil {
		return nil, err
	}
	return images, nil
}

func parseRegistrySearch(data []byte, fallbackReference string) ([]string, error) {
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err == nil {
		var images []string
		for _, row := range rows {
			name := firstString(row, "Name", "name", "Repository", "repository")
			if name == "" {
				name = fallbackReference
			}
			tag := firstString(row, "Tag", "tag")
			image := name
			if tag != "" && !strings.HasSuffix(name, ":"+tag) {
				image = name + ":" + tag
			}
			if image != "" {
				images = append(images, image)
			}
		}
		return uniqueSortedStrings(images), nil
	}
	var stringsOnly []string
	if err := json.Unmarshal(data, &stringsOnly); err != nil {
		return nil, fmt.Errorf("unrecognized registry discovery output")
	}
	for i, image := range stringsOnly {
		if !strings.Contains(image, ":") && !strings.Contains(image, "@") {
			stringsOnly[i] = fallbackReference + ":" + image
		}
	}
	return uniqueSortedStrings(stringsOnly), nil
}

func firstString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := row[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueSortedStrings(values []string) []string {
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
