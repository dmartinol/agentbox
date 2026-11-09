package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestFetchModelIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization header = %q", got)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"zeta"},{"id":"alpha"},{"id":""}]}`))
	}))
	defer server.Close()

	ids, err := fetchModelIDs(context.Background(), server.Client(), server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alpha", "zeta"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("model IDs = %#v; want %#v", ids, want)
	}
}

func TestEnMaaSMoelsWithoutKeyIsEmpty(t *testing.T) {
	t.Setenv("ENMAAS_KEY", "")
	app := App{}
	ids, err := app.EnMaaSMoels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("model IDs without credential = %#v", ids)
	}
}
