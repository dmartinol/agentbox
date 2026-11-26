package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestPodmanClockCheckReportsEqualClocksAndRoutineJitter(t *testing.T) {
	host := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	for _, delta := range []time.Duration{0, time.Second, 2 * time.Second} {
		check := classifyPodmanClock(host, host.Add(-delta))
		if !check.OK || check.State != CheckStateOK {
			t.Fatalf("delta %s check = %+v, want ok", delta, check)
		}
	}
	check := classifyPodmanClock(host, host.Add(-2100*time.Millisecond))
	if check.OK || check.State != CheckStateFail {
		t.Fatalf("2.1s check = %+v, want fail", check)
	}
}

func TestPodmanClockCheckReportsBehindAndAheadVM(t *testing.T) {
	host := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return host }
	cases := []struct {
		name       string
		offset     time.Duration
		wantSigned string
		future     bool
	}{
		{name: "27 seconds behind", offset: 27 * time.Second, wantSigned: "+27s", future: true},
		{name: "43 seconds behind", offset: 43 * time.Second, wantSigned: "+43s", future: true},
		{name: "27 seconds ahead", offset: -27 * time.Second, wantSigned: "-27s", future: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{output: func(recordedCommand) ([]byte, error) {
				return []byte(host.Add(-tc.offset).Format(time.RFC3339) + "\n"), nil
			}}
			check := (&App{Runner: runner}).podmanClockCheck(context.Background(), now)
			if check.OK || check.State != CheckStateFail {
				t.Fatalf("check = %+v, want fail", check)
			}
			if !strings.Contains(check.Detail, "Host UTC: "+host.Format(time.RFC3339)) ||
				!strings.Contains(check.Detail, "VM UTC: "+host.Add(-tc.offset).Format(time.RFC3339)) ||
				!strings.Contains(check.Detail, "Signed delta (host UTC - VM UTC): "+tc.wantSigned) {
				t.Fatalf("check detail lacks clock values: %q", check.Detail)
			}
			if !strings.Contains(check.Detail, "Remediation: Restart or resynchronize the Podman VM, then run `agentbox doctor`.") {
				t.Fatalf("check detail lacks remediation: %q", check.Detail)
			}
			if strings.Contains(strings.ToLower(check.Detail), "newly issued openshell jwts may appear future-dated") != tc.future {
				t.Fatalf("check detail future-token wording = %q", check.Detail)
			}
		})
	}
}

func TestPodmanClockCheckReportsUnavailableProbe(t *testing.T) {
	host := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return host }
	runner := &fakeRunner{output: func(recordedCommand) ([]byte, error) {
		return nil, errors.New("machine is not running")
	}}
	commandError := (&App{Runner: runner}).podmanClockCheck(context.Background(), now)
	if commandError.OK || commandError.State != CheckStateUnavailable {
		t.Fatalf("command error check = %+v, want unavailable", commandError)
	}
	if !strings.Contains(commandError.Detail, "Podman VM clock could not be verified") ||
		!strings.Contains(commandError.Detail, "machine is not running") ||
		!strings.Contains(commandError.Detail, "agentbox doctor") {
		t.Fatalf("command error detail = %q", commandError.Detail)
	}

	runner = &fakeRunner{output: func(recordedCommand) ([]byte, error) {
		return []byte("not a timestamp\n"), nil
	}}
	parseError := (&App{Runner: runner}).podmanClockCheck(context.Background(), now)
	if parseError.OK || parseError.State != CheckStateUnavailable {
		t.Fatalf("parse error check = %+v, want unavailable", parseError)
	}
	if !strings.Contains(parseError.Detail, "Podman VM clock could not be verified") ||
		!strings.Contains(parseError.Detail, "invalid RFC3339 output") ||
		!strings.Contains(parseError.Detail, "agentbox doctor") {
		t.Fatalf("parse error detail = %q", parseError.Detail)
	}
	if commandError.Detail == parseError.Detail {
		t.Fatal("command and parse error details are not distinct")
	}
}

type clockBlockingRunner struct {
	commands []recordedCommand
}

func (runner *clockBlockingRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	runner.commands = append(runner.commands, recordedCommand{name: name, args: append([]string(nil), args...)})
	<-ctx.Done()
	return nil, ctx.Err()
}

func (runner *clockBlockingRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func TestPodmanClockProbeIsBounded(t *testing.T) {
	runner := &clockBlockingRunner{}
	started := time.Now()
	check := (&App{Runner: runner}).podmanClockCheck(context.Background(), time.Now)
	elapsed := time.Since(started)
	if check.OK || check.State != CheckStateUnavailable {
		t.Fatalf("check = %+v, want unavailable", check)
	}
	if !strings.Contains(check.Detail, "probe timed out after 3s") || !strings.Contains(check.Detail, "agentbox doctor") {
		t.Fatalf("timeout detail = %q", check.Detail)
	}
	if elapsed < podmanClockProbeTimeout || elapsed > 2*podmanClockProbeTimeout {
		t.Fatalf("probe elapsed %s, want bounded near %s", elapsed, podmanClockProbeTimeout)
	}
	want := recordedCommand{name: "podman", args: []string{"machine", "ssh", "date", "-u", "+%Y-%m-%dT%H:%M:%SZ"}}
	if len(runner.commands) != 1 || runner.commands[0].name != want.name || strings.Join(runner.commands[0].args, " ") != strings.Join(want.args, " ") {
		t.Fatalf("probe command = %#v, want %#v", runner.commands, want)
	}
}

func TestDoctorIncludesPodmanVMClockCheck(t *testing.T) {
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if call.name == "podman" && commandHasArg(call, "machine") {
			return []byte(time.Now().UTC().Format(time.RFC3339)), nil
		}
		return []byte("ok"), nil
	}}
	checks := (&App{Runner: runner}).Doctor(context.Background())
	found := false
	for _, check := range checks {
		if check.Name != "podman VM clock" {
			if check.State != CheckStateOK && check.State != CheckStateFail && check.State != CheckStateUnavailable {
				t.Fatalf("check %q has unnormalized state %q", check.Name, check.State)
			}
			continue
		}
		found = true
		if check.State != CheckStateOK || !check.OK {
			t.Fatalf("podman VM clock check = %+v, want ok", check)
		}
	}
	if !found {
		t.Fatalf("doctor checks = %#v, want podman VM clock", checks)
	}
}
