package app

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	podmanClockProbeTimeout  = 3 * time.Second
	podmanClockSkewThreshold = 2 * time.Second
)

func (a *App) podmanClockCheck(ctx context.Context, now func() time.Time) Check {
	probeCtx, cancel := context.WithTimeout(ctx, podmanClockProbeTimeout)
	defer cancel()

	hostBefore := now()
	output, err := a.Runner.Output(probeCtx, "podman", "machine", "ssh", "date", "-u", "+%Y-%m-%dT%H:%M:%SZ")
	hostAfter := now()
	if err != nil {
		detail := fmt.Sprintf("Podman VM clock could not be verified: %v; run `agentbox doctor`.", err)
		if probeCtx.Err() != nil {
			detail = fmt.Sprintf("Podman VM clock could not be verified: probe timed out after %s; run `agentbox doctor`.", podmanClockProbeTimeout)
		}
		return Check{Name: "podman VM clock", OK: false, State: CheckStateUnavailable, Detail: detail}
	}

	vm, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(string(output)))
	if parseErr != nil {
		return Check{
			Name:   "podman VM clock",
			OK:     false,
			State:  CheckStateUnavailable,
			Detail: fmt.Sprintf("Podman VM clock could not be verified: invalid RFC3339 output %q; run `agentbox doctor`.", strings.TrimSpace(string(output))),
		}
	}
	return classifyPodmanClock(hostBefore.Add(hostAfter.Sub(hostBefore)/2), vm)
}

func classifyPodmanClock(host, vm time.Time) Check {
	delta := host.Sub(vm)
	absoluteDelta := delta
	if absoluteDelta < 0 {
		absoluteDelta = -absoluteDelta
	}
	state := CheckStateOK
	if absoluteDelta > podmanClockSkewThreshold {
		state = CheckStateFail
	}

	remediation := "No action is required; the host and Podman VM clocks are within routine jitter."
	detail := fmt.Sprintf(
		"Host UTC: %s\nVM UTC: %s\nSigned delta (host UTC - VM UTC): %s\nRemediation: %s",
		host.UTC().Format(time.RFC3339),
		vm.UTC().Format(time.RFC3339),
		formatSignedDuration(delta),
		remediation,
	)
	if state == CheckStateFail {
		remediation = "Restart or resynchronize the Podman VM, then run `agentbox doctor`."
		if delta > 0 {
			remediation += " Newly issued OpenShell JWTs may appear future-dated."
		}
		detail = fmt.Sprintf(
			"Host UTC: %s\nVM UTC: %s\nSigned delta (host UTC - VM UTC): %s\nRemediation: %s",
			host.UTC().Format(time.RFC3339),
			vm.UTC().Format(time.RFC3339),
			formatSignedDuration(delta),
			remediation,
		)
	}
	return Check{Name: "podman VM clock", OK: state == CheckStateOK, State: state, Detail: detail}
}

func formatSignedDuration(duration time.Duration) string {
	if duration < 0 {
		return "-" + (-duration).String()
	}
	return "+" + duration.String()
}
