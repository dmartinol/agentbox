package skills

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
)

// ListRoles reports the state of every embedded role for every harness. Roles
// are global-only, so --local returns no role rows.
func ListRoles(opts ListOptions) ([]Status, error) {
	if opts.Local {
		return nil, nil
	}
	var l layout
	resolved := true
	if opts.Home != "" {
		l = layout{home: opts.Home}
	} else if home, err := os.UserHomeDir(); err == nil {
		l = layout{home: home}
	} else {
		resolved = false
	}
	manifest := loadManifest(l.root())
	version := effectiveVersion(opts.Version)
	out := make([]Status, 0, len(roleOrder))
	for _, name := range roleOrder {
		role, err := RoleByName(name)
		if err != nil {
			return nil, err
		}
		status := Status{Name: name, Kind: "role", Version: version, Installations: []Installation{}}
		if !resolved {
			out = append(out, status)
			continue
		}
		record, managed := manifest.Roles[name]
		for _, harness := range harnesses {
			content, err := RenderRole(role, harness, "", "", opts.Version)
			if err != nil {
				return nil, err
			}
			tracked, trackedFile := record.File(harness)
			status.Installations = append(status.Installations, roleInstallation(l.rolePath(harness, name), harness, content, tracked, record.Version, managed && trackedFile))
		}
		out = append(out, status)
	}
	return out, nil
}

// roleInstallation derives the state of one harness rendering of a role from
// its sha256, using the same current|stale|modified|missing vocabulary as a
// canonical skill copy.
func roleInstallation(path, harness string, content []byte, tracked ManifestRoleFile, version string, managed bool) Installation {
	inst := Installation{Target: harness, Path: path, State: "missing"}
	body, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return inst
	case err != nil:
		inst.Present, inst.State, inst.Detail = true, "unreadable", err.Error()
		return inst
	}
	inst.Present = true
	if managed {
		inst.Version = version
	}
	switch {
	case bytes.Equal(body, content):
		inst.State = "current"
	case managed && hashHex(body) == tracked.SHA256:
		inst.State, inst.Detail = "stale", "run `agentbox skills update`"
	default:
		inst.State, inst.Detail = "modified", "differs from the managed copy; run `agentbox skills update --force`"
	}
	return inst
}
