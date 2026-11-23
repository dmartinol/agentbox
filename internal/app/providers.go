package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// BuiltinProvider describes an inference provider whose endpoint and credential
// environment are fixed by the model vendor. Users cannot customize, create
// under another name, edit, or delete these providers.
type BuiltinProvider struct {
	Name       string
	Kind       string
	Host       string
	Credential string
	ProfileID  string
	BaseURL    string
}

var builtinProviders = []BuiltinProvider{
	{
		Name:       "openai",
		Kind:       "openai",
		Host:       "api.openai.com",
		Credential: "OPENAI_API_KEY",
		ProfileID:  "agentbox-openai",
		BaseURL:    "https://api.openai.com/v1",
	},
	{
		Name:       "anthropic",
		Kind:       "anthropic",
		Host:       "api.anthropic.com",
		Credential: "ANTHROPIC_API_KEY",
		ProfileID:  "agentbox-anthropic",
		BaseURL:    "https://api.anthropic.com/v1",
	},
}

// BuiltinProviders returns a copy of the fixed provider definitions.
func BuiltinProviders() []BuiltinProvider {
	return append([]BuiltinProvider(nil), builtinProviders...)
}

// BuiltinProviderByName returns the built-in definition for a provider name.
func BuiltinProviderByName(name string) (BuiltinProvider, bool) {
	for _, provider := range builtinProviders {
		if provider.Name == name {
			return provider, true
		}
	}
	return BuiltinProvider{}, false
}

// BuiltinProviderByKind returns the built-in definition for a provider kind.
func BuiltinProviderByKind(kind string) (BuiltinProvider, bool) {
	for _, provider := range builtinProviders {
		if provider.Kind == kind {
			return provider, true
		}
	}
	return BuiltinProvider{}, false
}

// NormalizeProviderKind maps both direct OpenShell kinds and the generated
// profile IDs that older OpenShell releases report in the provider type field.
func NormalizeProviderKind(value string) string {
	switch {
	case value == "openai", value == "anthropic":
		return value
	case strings.HasPrefix(value, "agentbox-gateway-bearer-"):
		return "gateway-bearer"
	case strings.HasPrefix(value, "agentbox-gateway-header-"):
		return "gateway-header"
	case strings.HasPrefix(value, "agentbox-openai"):
		return "openai"
	case strings.HasPrefix(value, "agentbox-anthropic"):
		return "anthropic"
	default:
		return value
	}
}

// ProviderKindMode returns the only profile mode compatible with a provider
// kind. Unknown kinds fail closed.
func ProviderKindMode(kind string) (string, error) {
	switch NormalizeProviderKind(kind) {
	case "openai", "anthropic":
		return "standard", nil
	case "gateway-bearer", "gateway-header":
		return "gateway", nil
	default:
		return "", fmt.Errorf("provider kind %q is unsupported; supported kinds are anthropic, gateway-bearer, gateway-header, and openai", kind)
	}
}

type openShellProviderRecord struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	ProviderType string `json:"provider_type"`
}

type openShellProviderList struct {
	Providers []openShellProviderRecord `json:"providers"`
}

func (a *App) providerKind(ctx context.Context, provider string, gateway string) (string, error) {
	if definition, ok := BuiltinProviderByName(provider); ok {
		return definition.Kind, nil
	}
	data, err := a.ProviderList(ctx, gateway)
	if err != nil {
		return "", fmt.Errorf("provider %q kind could not be loaded from OpenShell: %w", provider, err)
	}
	var payload openShellProviderList
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("provider %q kind could not be parsed from OpenShell: %w", provider, err)
	}
	for _, record := range payload.Providers {
		if record.Name != provider {
			continue
		}
		rawKind := record.Type
		if rawKind == "" {
			rawKind = record.ProviderType
		}
		return NormalizeProviderKind(rawKind), nil
	}
	return "", fmt.Errorf("provider %q was not found in the OpenShell provider list", provider)
}

// BuiltinProviderStatus reports whether a built-in provider is registered with
// OpenShell and whether its fixed credential environment is available.
type BuiltinProviderStatus struct {
	BuiltinProvider
	Registered        bool
	CredentialPresent bool
	Error             string
}

// EnsureBuiltinProviders registers every built-in provider that is missing for
// the gateway. Providers without their fixed credential environment are
// reported as unregistered instead of failing the whole operation, so the GUI
// can show an actionable state.
func (a *App) EnsureBuiltinProviders(ctx context.Context, gateway string) []BuiltinProviderStatus {
	definitions := BuiltinProviders()
	out := make([]BuiltinProviderStatus, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, a.ensureBuiltinProvider(ctx, definition, gateway))
	}
	return out
}

func (a *App) ensureBuiltinProvider(ctx context.Context, definition BuiltinProvider, gateway string) BuiltinProviderStatus {
	status := BuiltinProviderStatus{
		BuiltinProvider:   definition,
		CredentialPresent: os.Getenv(definition.Credential) != "",
	}
	args := []string{"provider", "get", definition.Name}
	if gateway != "" {
		args = append(args, "--gateway", gateway)
	}
	if _, err := a.Runner.Output(ctx, "openshell", args...); err == nil {
		status.Registered = true
		return status
	}
	if !status.CredentialPresent {
		return status
	}
	if err := a.ProviderSetup(ctx, definition.Kind, definition.Name, definition.Host, definition.Credential, gateway, false, false); err != nil {
		status.Error = err.Error()
		return status
	}
	status.Registered = true
	return status
}
