package app

import (
	"context"
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
