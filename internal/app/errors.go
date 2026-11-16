package app

import (
	"fmt"
	"sort"
	"strings"
)

type ConflictError struct {
	Resource   string
	Name       string
	Reason     string
	Dependents []string
}

func (e *ConflictError) Error() string {
	dependents := append([]string(nil), e.Dependents...)
	sort.Strings(dependents)
	parts := []string{fmt.Sprintf("%s %q", e.Resource, e.Name)}
	if e.Reason != "" {
		parts = append(parts, e.Reason)
	}
	if len(dependents) > 0 {
		parts = append(parts, "dependents: "+strings.Join(dependents, ", "))
	}
	return strings.Join(parts, "; ")
}

type NotFoundError struct {
	Resource string
	Name     string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s %q not found", e.Resource, e.Name)
}

// NoLiveSandboxError reports that a host policy edit cannot be applied because
// the run has no live sandbox to update. It maps to HTTP 409.
type NoLiveSandboxError struct{}

func (e *NoLiveSandboxError) Error() string {
	return "Allow host is available only for Ready, Working, and Waiting agents."
}
