package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeProviderTypeMapsOpenShellProviderKinds(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		kind    string
		mode    string
		invalid bool
	}{
		{raw: "openai", kind: "openai", mode: "standard"},
		{raw: "anthropic", kind: "anthropic", mode: "standard"},
		{raw: "gateway-bearer", kind: "gateway-bearer", mode: "gateway"},
		{raw: "gateway-header", kind: "gateway-header", mode: "gateway"},
		{raw: "agentbox-gateway-bearer-models-example-test", kind: "gateway-bearer", mode: "gateway"},
		{raw: "agentbox-gateway-header-models-example-test", kind: "gateway-header", mode: "gateway"},
		{raw: "agentbox-openai", kind: "openai", mode: "standard"},
		{raw: "agentbox-anthropic", kind: "anthropic", mode: "standard"},
		{raw: "mystery-provider", kind: "mystery-provider", invalid: true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			if got := NormalizeProviderKind(tc.raw); got != tc.kind {
				t.Fatalf("NormalizeProviderKind(%q) = %q; want %q", tc.raw, got, tc.kind)
			}
			mode, err := ProviderKindMode(tc.raw)
			if tc.invalid {
				if err == nil || !strings.Contains(err.Error(), "unsupported") {
					t.Fatalf("ProviderKindMode(%q) error = %v", tc.raw, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode != tc.mode {
				t.Fatalf("ProviderKindMode(%q) = %q; want %q", tc.raw, mode, tc.mode)
			}
		})
	}
}

func TestEnsureBuiltinProvidersCreatesMissingProviders(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "unit-openai-key")
	t.Setenv("ANTHROPIC_API_KEY", "unit-anthropic-key")
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && len(call.args) >= 2 && call.args[0] == "provider" && call.args[1] == "get" {
			return nil, errors.New("provider not found")
		}
		return nil, nil
	}}
	store := Store{Base: t.TempDir()}
	app := App{Store: store, Runner: runner}

	statuses := app.EnsureBuiltinProviders(context.Background(), "gateway-one")
	if len(statuses) != 2 {
		t.Fatalf("statuses = %#v", statuses)
	}
	created := map[string]bool{}
	for _, call := range runner.outputs {
		if call.name != "openshell" || len(call.args) < 2 || call.args[0] != "provider" || call.args[1] != "create" {
			continue
		}
		for i := 0; i+1 < len(call.args); i++ {
			if call.args[i] == "--name" {
				created[call.args[i+1]] = true
			}
		}
	}
	for _, status := range statuses {
		if !status.Registered || !status.CredentialPresent {
			t.Fatalf("status = %#v", status)
		}
		if !created[status.Name] {
			t.Fatalf("built-in %q was not created: %#v", status.Name, runner.outputs)
		}
		path := filepath.Join(store.ProfilesDir(), status.ProfileID+"-provider.yaml")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("profile for %q missing: %v", status.Name, err)
		}
	}
}

func TestEnsureBuiltinProvidersSkipsMissingCredentials(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && len(call.args) >= 2 && call.args[0] == "provider" && call.args[1] == "get" {
			return nil, errors.New("provider not found")
		}
		return nil, nil
	}}
	app := App{Store: Store{Base: t.TempDir()}, Runner: runner}

	for _, status := range app.EnsureBuiltinProviders(context.Background(), "gateway-one") {
		if status.Registered || status.CredentialPresent || status.Error != "" {
			t.Fatalf("status = %#v", status)
		}
	}
	for _, call := range runner.outputs {
		if call.name == "openshell" && len(call.args) >= 2 && call.args[0] == "provider" && call.args[1] == "create" {
			t.Fatalf("provider was created without its credential: %#v", call)
		}
	}
}

func TestBuiltinProvidersCannotBeEditedOrDeleted(t *testing.T) {
	runner := &fakeRunner{}
	app := App{Store: Store{Base: t.TempDir()}, Runner: runner}
	for _, name := range []string{"openai", "anthropic"} {
		if err := app.ProviderUpdateCredential(context.Background(), name, "OTHER_KEY", "gateway-one"); err == nil || !strings.Contains(err.Error(), "built in") {
			t.Fatalf("update %q error = %v", name, err)
		}
		if err := app.ProviderDelete(context.Background(), name, "gateway-one"); err == nil || !strings.Contains(err.Error(), "built in") {
			t.Fatalf("delete %q error = %v", name, err)
		}
	}
	if len(runner.outputs) != 0 {
		t.Fatalf("immutable provider issued OpenShell commands: %#v", runner.outputs)
	}
}

func TestProviderSetupRejectsCustomNameForBuiltinKind(t *testing.T) {
	app := App{Store: Store{Base: t.TempDir()}, Runner: &fakeRunner{}}
	err := app.ProviderSetup(context.Background(), "openai", "my-openai", "", "", "gateway-one", false, false)
	if err == nil || !strings.Contains(err.Error(), `built in as "openai"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestProviderSetupForcesBuiltinEndpointAndCredential(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "unit-secret")
	store := Store{Base: t.TempDir()}
	app := App{Store: store, Runner: &fakeRunner{}}
	if err := app.ProviderSetup(context.Background(), "openai", "openai", "evil.example.test", "EVIL_KEY", "gateway-one", false, false); err != nil {
		t.Fatal(err)
	}
	profile, err := os.ReadFile(filepath.Join(store.ProfilesDir(), "agentbox-openai-provider.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(profile), "host: api.openai.com") || !strings.Contains(string(profile), "OPENAI_API_KEY") {
		t.Fatalf("built-in provider was not pinned: %s", profile)
	}
	if strings.Contains(string(profile), "evil.example.test") || strings.Contains(string(profile), "EVIL_KEY") {
		t.Fatalf("built-in provider accepted custom values: %s", profile)
	}
}
