package app

import (
	"context"
	"os"
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
