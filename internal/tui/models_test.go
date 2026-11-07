package tui

import (
	"testing"

	"github.com/dmartinol/agentbox/internal/app"
)

func TestNewRunRowsFormatsStructuredData(t *testing.T) {
	runs := []app.Run{
		{ID: "run-1", Agent: "codex", Mode: "enmaas", Approval: "unattended", Status: "running", Synced: false, Worktree: "/tmp/worktree"},
	}
	rows := newRunRows(runs)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.ID != "run-1" || row.Agent != "codex" || row.Mode != "enmaas" || row.Approval != "unattended" || row.Status != "running" || row.Synced != "no" || row.Worktree != "/tmp/worktree" {
		t.Fatalf("unexpected row: %+v", row)
	}
}

func TestNewImageRowsParsesPodmanJSON(t *testing.T) {
	data := []byte(`[
		{
			"Id":"0123456789abcdef0123456789abcdef",
			"Names":["localhost/agentbox-codex:0.167.0-r3"],
			"Labels":{"agentbox.agent":"codex","agentbox.version":"0.167.0","agentbox.recipe":"3","agentbox.base-digest":"sha256:abcdef"},
			"Size":1048576,
			"CreatedAt":"2026-11-07T08:00:00Z"
		}
	]`)
	rows, err := newImageRows(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.ID != "0123456789ab" || row.Name != "localhost/agentbox-codex:0.167.0-r3" || row.Agent != "codex" || row.Version != "0.167.0" || row.Recipe != "3" || row.BaseDigest != "abcdef" || row.Size != "1.0 MiB" {
		t.Fatalf("unexpected row: %+v", row)
	}
}

func TestNewGatewayRowsParsesStructuredData(t *testing.T) {
	data := []byte(`[{"name":"openshell","active":true,"endpoint":"https://localhost:17670","type":"local","auth":"mtls","source":"user"}]`)
	rows, err := newGatewayRows(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "openshell" || rows[0].Active != "yes" || rows[0].Endpoint != "https://localhost:17670" || rows[0].Type != "local" || rows[0].Auth != "mtls" || rows[0].Source != "user" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestNewSummaryCountsRuns(t *testing.T) {
	runs := []app.Run{
		{Status: "running", Synced: true},
		{Status: "finished", Synced: false},
		{Status: "error", Synced: true},
	}
	s := newSummary(runs, 3, 2, 4)
	if s.Active != 1 || s.Finished != 1 || s.Failed != 1 || s.Unsynced != 1 || s.Images != 3 || s.Gateways != 2 || s.Profiles != 4 {
		t.Fatalf("unexpected summary: %+v", s)
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0:          "0 B",
		1023:       "1023 B",
		1024:       "1.0 KiB",
		1048576:    "1.0 MiB",
		1073741824: "1.0 GiB",
	}
	for input, want := range cases {
		if got := humanSize(input); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", input, got, want)
		}
	}
}
