package openviking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	BaseURL, APIKey string
	HTTP            *http.Client
}

func (c Client) Write(ctx context.Context, uri, content string) error {
	err := c.request(ctx, http.MethodPost, "/api/v1/content/write", map[string]any{"uri": uri, "content": content, "mode": "create", "wait": true})
	if status(err) != http.StatusConflict {
		return err
	}
	return c.request(ctx, http.MethodPost, "/api/v1/content/write", map[string]any{"uri": uri, "content": content, "mode": "replace", "wait": true})
}
func (c Client) Delete(ctx context.Context, uri string) error {
	return c.request(ctx, http.MethodDelete, "/api/v1/fs?uri="+url.QueryEscape(uri), nil)
}
func (c Client) request(ctx context.Context, method, path string, payload any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.APIKey != "" {
		req.Header.Set("X-API-Key", c.APIKey)
	}
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
		return httpError{status: resp.StatusCode, message: fmt.Sprintf("OpenViking %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(data)))}
	}
	return nil
}

type httpError struct {
	status  int
	message string
}

func (e httpError) Error() string { return e.message }
func status(err error) int {
	if e, ok := err.(httpError); ok {
		return e.status
	}
	return 0
}
func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}
