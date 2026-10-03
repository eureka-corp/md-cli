// Package mdshare talks to the mdreader share API and prepares what it uploads.
package mdshare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultBaseURL = "https://md.erk.im"

// File is one markdown document in a share.
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Share is the server's description of a share. ExpiresAt and TTL are nil for
// a share that never expires.
type Share struct {
	ID        string     `json:"id"`
	URL       string     `json:"url"`
	Title     string     `json:"title,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt"`
	TTL       *int64     `json:"ttl,omitempty"`
	FileCount int        `json:"fileCount"`
}

// Expiry is what to ask the server for: a TTL, or Keep for no expiry. The
// zero value lets the server decide (default on create, unchanged on update).
type Expiry struct {
	TTL  time.Duration
	Keep bool
}

// APIError is the error body returned on every non-2xx response.
type APIError struct {
	Status  int    `json:"-"`
	Message string `json:"error"`
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Limit   int64  `json:"limit,omitempty"`
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = fmt.Sprintf("unexpected response (HTTP %d)", e.Status)
	}
	switch {
	case e.Status == http.StatusUnauthorized:
		msg += " — check MD_TOKEN or run 'md setup'"
	case e.Code == "keep_requires_owner":
		msg += " — use an API key from https://md.erk.im/settings (md setup)"
	}
	return msg
}

// IsNotFound reports whether err is the API's 404.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// Create uploads files as a new share.
func (c *Client) Create(ctx context.Context, files []File, exp Expiry) (*Share, error) {
	return c.sendFiles(ctx, http.MethodPost, "/api/shares", files, exp)
}

// Update replaces the files of an existing share, keeping its URL. A zero
// Expiry keeps the current expiry.
func (c *Client) Update(ctx context.Context, id string, files []File, exp Expiry) (*Share, error) {
	return c.sendFiles(ctx, http.MethodPut, sharePath(id), files, exp)
}

func (c *Client) sendFiles(ctx context.Context, method, path string, files []File, exp Expiry) (*Share, error) {
	body := map[string]any{"files": files}
	if exp.Keep {
		body["keep"] = true
	} else if exp.TTL > 0 {
		body["ttl"] = int64(exp.TTL.Seconds())
	}
	var share Share
	if err := c.do(ctx, method, path, body, &share); err != nil {
		return nil, err
	}
	return &share, nil
}

// Patch is a change to an existing share. Set at most one of Extend, TTL and
// Keep. A non-nil Title renames; an empty Title resets it.
type Patch struct {
	Extend time.Duration
	TTL    time.Duration
	Keep   bool
	Title  *string
}

func (c *Client) Patch(ctx context.Context, id string, p Patch) (*Share, error) {
	body := map[string]any{}
	switch {
	case p.Keep:
		body["keep"] = true
	case p.TTL > 0:
		body["ttl"] = int64(p.TTL.Seconds())
	case p.Extend > 0:
		body["extend"] = int64(p.Extend.Seconds())
	}
	if p.Title != nil {
		if *p.Title == "" {
			body["title"] = nil
		} else {
			body["title"] = *p.Title
		}
	}
	var share Share
	if err := c.do(ctx, http.MethodPatch, sharePath(id), body, &share); err != nil {
		return nil, err
	}
	return &share, nil
}

// List returns the shares owned by the token's account.
func (c *Client) List(ctx context.Context) ([]Share, error) {
	var out struct {
		Shares []Share `json:"shares"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/me/shares", nil, &out); err != nil {
		return nil, err
	}
	return out.Shares, nil
}

// Verify checks that the token is accepted.
func (c *Client) Verify(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/api/shares", nil, nil)
}

// Delete removes a share.
func (c *Client) Delete(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, sharePath(id), nil, nil)
}

func sharePath(id string) string { return "/api/shares/" + url.PathEscape(id) }

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &APIError{}
		_ = json.Unmarshal(data, apiErr)
		apiErr.Status = resp.StatusCode
		return apiErr
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// ShareID extracts the share id from a URL or a bare id.
func ShareID(s string) string {
	s = strings.TrimSpace(s)
	if u, err := url.Parse(s); err == nil && u.Path != "" {
		s = u.Path
	}
	s = strings.TrimRight(s, "/")
	return s[strings.LastIndex(s, "/")+1:]
}
