package openviking

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteAndDelete(t *testing.T) {
	var requests []*http.Request
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		if r.Header.Get("X-API-Key") != "key" {
			t.Errorf("missing API key")
		}
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"uri":"viking://user/resources/docmost/pages/p1.md"`) {
				t.Errorf("write body: %s", body)
			}
			if !strings.Contains(string(body), `"mode":"create"`) {
				t.Errorf("create mode: %s", body)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer s.Close()
	c := Client{BaseURL: s.URL, APIKey: "key"}
	if err := c.Write(context.Background(), "viking://user/resources/docmost/pages/p1.md", "# Page"); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(context.Background(), "viking://user/resources/docmost/pages/p1.md"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[1].Method != http.MethodDelete || !strings.HasPrefix(requests[1].URL.String(), "/api/v1/fs?uri=") {
		t.Fatalf("unexpected requests")
	}
}
