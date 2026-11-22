package gui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentboxweb "github.com/dmartinol/agentbox/internal/gui/web"
)

func TestSPAHandlerRevalidatesAppShell(t *testing.T) {
	handler := spaHandler()
	for _, path := range []string{"/", "/index.html", "/runs/run-123"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200", path, recorder.Code)
		}
		if got := recorder.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("GET %s: Cache-Control = %q, want %q", path, got, "no-cache")
		}
		if recorder.Header().Get("ETag") == "" {
			t.Errorf("GET %s: missing ETag", path)
		}
		if !strings.Contains(recorder.Body.String(), `id="root"`) {
			t.Errorf("GET %s: response is not the app shell", path)
		}
	}
}

func TestSPAHandlerAnswersConditionalShellRequest(t *testing.T) {
	handler := spaHandler()
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	etag := first.Header().Get("ETag")

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("If-None-Match", etag)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotModified {
		t.Fatalf("conditional GET: status = %d, want 304", recorder.Code)
	}
}

func TestSPAHandlerCachesHashedAssets(t *testing.T) {
	assets, err := fs.Sub(agentboxweb.Assets(), "dist")
	if err != nil {
		t.Fatal(err)
	}
	paths, err := fs.Glob(assets, "assets/*.js")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no built javascript assets in dist: %v", err)
	}

	recorder := httptest.NewRecorder()
	spaHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/"+paths[0], nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /%s: status = %d, want 200", paths[0], recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("GET /%s: Cache-Control = %q", paths[0], got)
	}
}

func TestSPAHandlerDoesNotMaskMissingAssets(t *testing.T) {
	recorder := httptest.NewRecorder()
	spaHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/index-stale-build.js", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing asset: status = %d, want 404", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), `id="root"`) {
		t.Fatal("missing asset returned the app shell")
	}
}
