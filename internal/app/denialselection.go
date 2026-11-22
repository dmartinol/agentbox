package app

import (
	"fmt"
	"strconv"
	"strings"
)

// DenialSelection pairs one denial from a run's listing with the row the
// selection named, so a caller can echo the row while applying the rule.
type DenialSelection struct {
	Row     int           `json:"row"`
	Request DenialRequest `json:"request"`
}

// ResolveDenialSelection maps an interactive selection onto the denials it
// names. Input is "all" or comma-separated row numbers counting from one, as
// printed by the listing. Rows resolve against the given listing, so a
// selection cannot drift to a different denial between listing and apply.
// Duplicate rows collapse, invalid denials are refused, and empty input
// selects nothing.
func ResolveDenialSelection(input string, denials []PolicyDenial) ([]DenialSelection, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}
	if strings.EqualFold(input, "all") {
		selections := make([]DenialSelection, 0)
		for i, denial := range denials {
			if !denial.Valid {
				continue
			}
			selections = append(selections, DenialSelection{
				Row:     i + 1,
				Request: DenialRequest{Host: denial.Host, Port: denial.Port, Binary: denial.Binary},
			})
		}
		if len(selections) == 0 {
			return nil, &ValidationError{Message: "No denial can be turned into a host rule."}
		}
		return selections, nil
	}
	seen := make(map[int]bool)
	selections := make([]DenialSelection, 0)
	for _, part := range strings.Split(input, ",") {
		part = strings.TrimSpace(part)
		row, err := strconv.Atoi(part)
		if err != nil || row < 1 {
			return nil, &ValidationError{Message: fmt.Sprintf("%q is not a row number; use numbers like 1,2 or all.", part)}
		}
		if row > len(denials) {
			return nil, &ValidationError{Message: fmt.Sprintf("Row %d does not exist; the listing has %d denial(s).", row, len(denials))}
		}
		if seen[row] {
			continue
		}
		seen[row] = true
		denial := denials[row-1]
		if !denial.Valid {
			return nil, &ValidationError{Message: fmt.Sprintf("Row %d (%s:%d) cannot be allowed: %s", row, denial.Host, denial.Port, denial.InvalidReason)}
		}
		selections = append(selections, DenialSelection{
			Row:     row,
			Request: DenialRequest{Host: denial.Host, Port: denial.Port, Binary: denial.Binary},
		})
	}
	return selections, nil
}
