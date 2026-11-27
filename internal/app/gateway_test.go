package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestGatewayConnectUsesExplicitRegistrationAndLocalFlag(t *testing.T) {
	runner := &fakeRunner{}
	app := App{Runner: runner}
	if err := app.GatewayConnect(context.Background(), "https://localhost:17670", "openshell", false); err != nil {
		t.Fatal(err)
	}
	if len(runner.outputs) != 1 {
		t.Fatalf("unexpected command count: %#v", runner.outputs)
	}
	want := []string{"gateway", "add", "https://localhost:17670", "--name", "openshell", "--local"}
	if !reflect.DeepEqual(runner.outputs[0].args, want) {
		t.Fatalf("gateway command = %#v; want %#v", runner.outputs[0].args, want)
	}
	if err := app.GatewayConnect(context.Background(), "https://gateway.example", "not a name", false); err == nil {
		t.Fatal("invalid gateway name was accepted")
	}
}

func TestGatewayConnectPreviewDoesNotCallOpenShell(t *testing.T) {
	runner := &fakeRunner{}
	app := App{Runner: runner}
	if err := app.GatewayConnect(context.Background(), "https://gateway.example", "team-gateway", true); err != nil {
		t.Fatal(err)
	}
	if len(runner.outputs) != 0 || len(runner.streams) != 0 {
		t.Fatalf("gateway preview invoked external commands: %#v %#v", runner.outputs, runner.streams)
	}
}

func TestGatewayStartCommandFailsClearlyWithoutSupportedDependency(t *testing.T) {
	command, args, err := gatewayStartCommand("darwin", true)
	if err != nil || command != "brew" || !reflect.DeepEqual(args, []string{"services", "start", "openshell"}) {
		t.Fatalf("gateway start command = %q %#v, %v", command, args, err)
	}
	if _, _, err := gatewayStartCommand("linux", true); err == nil || !strings.Contains(err.Error(), "macOS only") {
		t.Fatalf("unsupported platform error = %v", err)
	}
	if _, _, err := gatewayStartCommand("darwin", false); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("missing service error = %v", err)
	}
}

func TestProviderListUsesExplicitGateway(t *testing.T) {
	runner := &fakeRunner{output: func(recordedCommand) ([]byte, error) {
		return []byte(`{"providers":[]}`), nil
	}}
	app := App{Runner: runner}
	if _, err := app.ProviderList(context.Background(), "openshell"); err != nil {
		t.Fatal(err)
	}
	if len(runner.outputs) != 1 {
		t.Fatalf("unexpected command count: %#v", runner.outputs)
	}
	want := []string{"provider", "list", "-o", "json", "--gateway", "openshell"}
	if !reflect.DeepEqual(runner.outputs[0].args, want) {
		t.Fatalf("provider list command = %#v; want %#v", runner.outputs[0].args, want)
	}
}

func TestGatewayRemoveLifecycle(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(Run{ID: "run-live123", Name: "live-run", Gateway: "gateway-one", Status: StatusWorking}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{output: func(recordedCommand) ([]byte, error) {
		return []byte("Available Commands:\n  remove Remove a gateway\n"), nil
	}}
	app := App{Store: store, Runner: runner}
	err := app.GatewayRemove(context.Background(), "gateway-one")
	var conflict *ConflictError
	if err == nil || !errors.As(err, &conflict) || !reflect.DeepEqual(conflict.Dependents, []string{"live-run"}) {
		t.Fatalf("active guard = %v", err)
	}
	if len(runner.outputs) != 0 {
		t.Fatalf("blocked gateway issued command: %#v", runner.outputs)
	}

	if err := store.SaveRun(Run{ID: "run-live123", Status: StatusCompleted}); err != nil {
		t.Fatal(err)
	}
	if err := app.GatewayRemove(context.Background(), "gateway-one"); err != nil {
		t.Fatal(err)
	}
	if len(runner.outputs) != 2 || runner.outputs[0].name != "openshell" || !reflect.DeepEqual(runner.outputs[0].args, []string{"gateway", "--help"}) || runner.outputs[1].name != "openshell" || !reflect.DeepEqual(runner.outputs[1].args, []string{"gateway", "remove", "gateway-one"}) {
		t.Fatalf("commands = %#v", runner.outputs)
	}
}

func TestGatewayRemoveCapabilityPreflight(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		output func(recordedCommand) ([]byte, error)
	}{
		{
			name: "help does not list remove",
			output: func(recordedCommand) ([]byte, error) {
				return []byte("Available Commands:\n  list List gateways\n"), nil
			},
		},
		{
			name:   "help fails",
			output: func(recordedCommand) ([]byte, error) { return nil, errors.New("openshell unavailable") },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeRunner{output: test.output}
			app := App{Store: store, Runner: runner}
			err := app.GatewayRemove(context.Background(), "gateway-one")
			var unsupported *UnsupportedCapabilityError
			if err == nil || !errors.As(err, &unsupported) || unsupported.Command != "openshell gateway remove" {
				t.Fatalf("error = %v; want unsupported gateway remove capability", err)
			}
			for _, call := range runner.outputs {
				if call.name == "openshell" && len(call.args) > 1 && call.args[0] == "gateway" && call.args[1] == "remove" {
					t.Fatalf("gateway removal was invoked: %#v", call)
				}
			}
		})
	}
}
