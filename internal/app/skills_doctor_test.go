package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmartinol/agentbox/internal/skills"
)

func TestDoctorReportsStaleUsageSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, ".agents"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := skills.Install(skills.Options{Target: "agents", Version: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	a := &App{Store: Store{Base: t.TempDir()}, Runner: &fakeRunner{}, Version: "0.2.0"}
	var found Check
	for _, check := range a.Doctor(context.Background()) {
		if check.Name == "skills" {
			found = check
		}
	}
	if found.Name == "" {
		t.Fatal("doctor did not report skills")
	}
	if found.OK || !strings.Contains(found.Detail, "stale") {
		t.Fatalf("stale check = %+v", found)
	}
	if !strings.Contains(found.Detail, "skills install --force") {
		t.Fatalf("stale check lacks the recovery command: %s", found.Detail)
	}
}

func TestDoctorReportsCurrentUsageSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, ".agents"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := skills.Install(skills.Options{Target: "agents", Version: "0.2.0"}); err != nil {
		t.Fatal(err)
	}
	a := &App{Store: Store{Base: t.TempDir()}, Runner: &fakeRunner{}, Version: "0.2.0"}
	var found Check
	for _, check := range a.Doctor(context.Background()) {
		if check.Name == "skills" {
			found = check
		}
	}
	if !found.OK || !strings.Contains(found.Detail, "up to date") {
		t.Fatalf("current check = %+v", found)
	}
}
