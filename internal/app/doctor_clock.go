package app

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	podmanClockProbeTimeout  = 3 * time.Second
	podmanClockSkewThreshold = 2 * time.Second
)

func (a *App) podmanClockCheck(ctx context.Context, now func() time.Time) Check {
	probeCtx, cancel := context.WithTimeout(ctx, podmanClockProbeTimeout)
	defer cancel()

	hostBefore := now()
	output, err := a.Runner.Output(probeCtx, "podman", "machine", "ssh", "date", "-u", "+%Y-%m-%dT%H:%M:%S.%NZ")
	hostAfter := now()
	if err != nil {
		detail := unavailablePodmanClockDetail(fmt.Sprintf("%v", err))
		if probeCtx.Err() != nil {
			detail = unavailablePodmanClockDetail(fmt.Sprintf("probe timed out after %s", podmanClockProbeTimeout))
		}
		return Check{Name: "podman VM clock", OK: false, State: CheckStateUnavailable, Detail: detail}
	}

	probeOutput := strings.TrimSpace(string(output))
	vm, parseErr := time.Parse(time.RFC3339Nano, probeOutput)
	if parseErr != nil {
		return Check{
			Name:   "podman VM clock",
			OK:     false,
			State:  CheckStateUnavailable,
			Detail: unavailablePodmanClockDetail(fmt.Sprintf("invalid RFC3339 output %q", probeOutput)),
		}
	}
	host := hostBefore.Add(hostAfter.Sub(hostBefore) / 2)
	hasFraction := strings.ContainsAny(probeOutput, ".,")
	if !hasFraction {
		host = host.Truncate(time.Second)
		vm = vm.Truncate(time.Second)
	}
	return classifyPodmanClockAtPrecision(host, vm, hasFraction)
}

func classifyPodmanClock(host, vm time.Time) Check {
	return classifyPodmanClockAtPrecision(host, vm, true)
}

func classifyPodmanClockAtPrecision(host, vm time.Time, fractional bool) Check {
	if !fractional {
		host = host.Truncate(time.Second)
		vm = vm.Truncate(time.Second)
	}
	detailLayout := time.RFC3339
	if fractional {
		detailLayout = time.RFC3339Nano
	}
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
		host.UTC().Format(detailLayout),
		vm.UTC().Format(detailLayout),
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
			host.UTC().Format(detailLayout),
			vm.UTC().Format(detailLayout),
			formatSignedDuration(delta),
			remediation,
		)
	}
	return Check{Name: "podman VM clock", OK: state == CheckStateOK, State: state, Detail: detail}
}

func unavailablePodmanClockDetail(cause string) string {
	const diagnosis = "Podman VM clock could not be verified: "
	const remediation = "; run `agentbox doctor`."
	cause = strings.ToValidUTF8(cause, "�")
	limit := statusDetailLimit - len(diagnosis) - len(remediation)
	if len(cause) > limit {
		for len(cause) > limit && !utf8.ValidString(cause[:limit]) {
			limit--
		}
		cause = cause[:limit]
	}
	return diagnosis + cause + remediation
}

func formatSignedDuration(duration time.Duration) string {
	if duration < 0 {
		return "-" + (-duration).String()
	}
	return "+" + duration.String()
}
