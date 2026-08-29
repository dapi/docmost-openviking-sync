//go:build integration

package docmost

import (
	"context"
	"os"
	"testing"
)

// Run explicitly with credentials loaded into the environment:
// go test -tags=integration ./internal/docmost
func TestIntegrationReadDocmost(t *testing.T) {
	c := &Client{BaseURL: os.Getenv("DOCMOST_API_URL"), Token: os.Getenv("DOCMOST_API_TOKEN"), Email: os.Getenv("DOCMOST_EMAIL"), Password: os.Getenv("DOCMOST_PASSWORD")}
	if c.BaseURL == "" || (c.Token == "" && (c.Email == "" || c.Password == "")) {
		t.Skip("Docmost credentials are not configured")
	}
	spaces, err := c.ListSpaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(spaces) == 0 {
		t.Fatal("expected at least one accessible Docmost space")
	}
	pages, err := c.ListPages(context.Background(), spaces[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		return
	}
	if _, err := c.GetPage(context.Background(), pages[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Breadcrumbs(context.Background(), pages[0].ID); err != nil {
		t.Fatal(err)
	}
}
