package app

import "testing"

func TestImageArtifactRemoteNaming(t *testing.T) {
	img := ImageArtifact{Agent: "codex", Version: "0.167.0", Recipe: 3, OS: "linux", Arch: "arm64"}
	if got := img.RemoteTag(); got != "codex-0.167.0-r3-linux-arm64" {
		t.Fatalf("RemoteTag = %q", got)
	}
	if got := RemoteReference("quay.io/dmartino/openshell", img); got != "quay.io/dmartino/openshell:codex-0.167.0-r3-linux-arm64" {
		t.Fatalf("RemoteReference = %q", got)
	}
	parsed, ok := ParseRemoteTag("codex-0.167.0-r3-linux-arm64")
	if !ok || parsed.Agent != "codex" || parsed.Version != "0.167.0" || parsed.Recipe != 3 || parsed.OS != "linux" || parsed.Arch != "arm64" {
		t.Fatalf("parsed = %+v, %v", parsed, ok)
	}
	parsed, ok = ParseRemoteTag("claude-2.1.319-r3")
	if !ok || parsed.Agent != "claude" || parsed.OS != "" || parsed.Arch != "" {
		t.Fatalf("architecture-free tag parsed = %+v, %v", parsed, ok)
	}
	if _, ok := ParseRemoteTag("v1.0.0"); ok {
		t.Fatal("unrelated tag was accepted")
	}
}
