package docmost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dapi/docmost-openviking-sync/internal/syncer"
)

// Client supports both long-lived API tokens and Docmost email/password login.
type Client struct {
	BaseURL, Token, Email, Password string
	HTTP                            *http.Client
}

func (c *Client) ListSpaces(ctx context.Context) ([]syncer.Space, error) {
	var all []syncer.Space
	for page := 1; ; page++ {
		var response struct {
			Data struct {
				Items []syncer.Space `json:"items"`
				Meta  struct {
					HasNext bool `json:"hasNextPage"`
				} `json:"meta"`
			} `json:"data"`
		}
		if err := c.post(ctx, "/spaces", map[string]any{"limit": 100, "page": page}, &response); err != nil {
			return nil, err
		}
		all = append(all, response.Data.Items...)
		if !response.Data.Meta.HasNext {
			return all, nil
		}
	}
}

func (c *Client) ListPages(ctx context.Context, space syncer.Space) ([]syncer.Page, error) {
	var out []syncer.Page
	seen := map[string]bool{}
	var walk func(string) error
	walk = func(parentID string) error {
		pages, err := c.sidebarPages(ctx, space.ID, parentID)
		if err != nil {
			return err
		}
		for _, page := range pages {
			if page.ID == "" || seen[page.ID] {
				continue
			}
			seen[page.ID] = true
			out = append(out, page)
			if err := walk(page.ID); err != nil {
				return err
			}
		}
		return nil
	}
	return out, walk("")
}
func (c *Client) GetPage(ctx context.Context, id string) (syncer.Page, error) {
	var r struct {
		Data syncer.Page `json:"data"`
	}
	if err := c.post(ctx, "/pages/info", map[string]any{"pageId": id}, &r); err != nil {
		return syncer.Page{}, err
	}
	return r.Data, nil
}
func (c *Client) Breadcrumbs(ctx context.Context, id string) ([]string, error) {
	var r struct {
		Data json.RawMessage `json:"data"`
	}
	if err := c.post(ctx, "/pages/breadcrumbs", map[string]any{"pageId": id}, &r); err != nil {
		return nil, err
	}
	var values []struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(r.Data, &values); err != nil {
		return nil, fmt.Errorf("decode breadcrumbs: %w", err)
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v.Title != "" {
			out = append(out, v.Title)
		}
	}
	return out, nil
}
func (c *Client) sidebarPages(ctx context.Context, spaceID, pageID string) ([]syncer.Page, error) {
	var r struct {
		Data struct {
			Items []syncer.Page `json:"items"`
		} `json:"data"`
	}
	if err := c.post(ctx, "/pages/sidebar-pages", map[string]any{"spaceId": spaceID, "pageId": pageID, "page": 1}, &r); err != nil {
		return nil, err
	}
	return r.Data.Items, nil
}
func (c *Client) post(ctx context.Context, endpoint string, payload, out any) error {
	if err := c.ensureAuth(ctx); err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase()+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("Docmost %s: HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode Docmost %s response: %w", endpoint, err)
	}
	return nil
}
func (c *Client) ensureAuth(ctx context.Context) error {
	if c.Token != "" {
		return nil
	}
	if c.Email == "" || c.Password == "" {
		return fmt.Errorf("Docmost credentials are missing: set token or email/password")
	}
	body, _ := json.Marshal(map[string]string{"email": c.Email, "password": c.Password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase()+"/auth/login", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("Docmost login: HTTP %d", resp.StatusCode)
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "authToken" && cookie.Value != "" {
			c.Token = cookie.Value
			return nil
		}
	}
	return fmt.Errorf("Docmost login did not return authToken cookie")
}
func (c *Client) apiBase() string {
	base := strings.TrimSuffix(c.BaseURL, "/")
	if !strings.HasSuffix(base, "/api") {
		base += "/api"
	}
	return base
}
func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}
