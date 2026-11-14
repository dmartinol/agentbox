package app

import "fmt"

func containsCommand(args []string, first, second string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == first && args[i+1] == second {
			return true
		}
	}
	return false
}

// statusRunnerFor fakes one sandbox inspect and one status probe.
func statusRunnerFor(phase, probe string) *fakeRunner {
	return &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if call.name != "openshell" {
			return nil, nil
		}
		if containsCommand(call.args, "sandbox", "get") {
			return []byte(fmt.Sprintf(`{"phase":%q}`, phase)), nil
		}
		return []byte(probe), nil
	}}
}
