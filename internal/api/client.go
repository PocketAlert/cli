package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

var ErrNotAuthenticated = errors.New("not authenticated: run `pocketalert auth` or set POCKETALERT_API_KEY")

type Client struct {
	BaseURL   string
	APIKey    string
	UserAgent string
	HTTP      *http.Client
}

func New(baseURL, apiKey, userAgent string) *Client {
	return &Client{
		BaseURL:   baseURL,
		APIKey:    apiKey,
		UserAgent: userAgent,
		HTTP:      &http.Client{Timeout: 60 * time.Second},
	}
}

// Error is a non-2xx API response. Raw keeps the body for --json output.
type Error struct {
	Status     int
	Message    string
	Details    string
	Limit      int
	Used       int
	ResetsAt   string
	UpgradeURL string
	Raw        []byte
}

func (e *Error) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	if e.Details != "" {
		msg += ": " + e.Details
	}
	switch {
	case e.Status == http.StatusTooManyRequests && e.Limit > 0:
		msg = fmt.Sprintf("daily message limit reached (%d/%d), resets at %s", e.Used, e.Limit, e.ResetsAt)
		if e.UpgradeURL != "" {
			msg += "\nUpgrade: " + e.UpgradeURL
		}
	case e.Status == http.StatusUnauthorized:
		msg += " — check your API key (`pocketalert auth`)"
	}
	return fmt.Sprintf("API error %d: %s", e.Status, msg)
}

func parseError(status int, body []byte) *Error {
	e := &Error{Status: status, Raw: body}
	var payload struct {
		Error      string `json:"error"`
		Message    string `json:"message"`
		Details    string `json:"details"`
		Limit      int    `json:"limit"`
		Used       int    `json:"used"`
		ResetsAt   string `json:"resets_at"`
		UpgradeURL string `json:"upgrade_url"`
	}
	if json.Unmarshal(body, &payload) == nil {
		e.Message = payload.Error
		if e.Message == "" {
			e.Message = payload.Message
		}
		e.Details = payload.Details
		e.Limit = payload.Limit
		e.Used = payload.Used
		e.ResetsAt = payload.ResetsAt
		e.UpgradeURL = payload.UpgradeURL
	} else if len(body) > 0 && len(body) < 512 {
		e.Message = string(bytes.TrimSpace(body))
	}
	return e
}

// Do sends a JSON request and decodes the response into out (when non-nil).
// It returns the raw body so callers can print it verbatim with --json.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	return c.send(ctx, method, path, query, reader, "application/json", out)
}

// Multipart posts form fields plus one file, used for image uploads.
func (c *Client) Multipart(ctx context.Context, path string, fields map[string]string, fileField, filePath string, out any) ([]byte, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	part, err := w.CreateFormFile(fileField, filepath.Base(filePath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, fmt.Errorf("read %s: %w", filePath, err)
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return c.send(ctx, http.MethodPost, path, nil, &buf, w.FormDataContentType(), out)
}

func (c *Client) send(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string, out any) ([]byte, error) {
	if c.APIKey == "" {
		return nil, ErrNotAuthenticated
	}
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Token", c.APIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return raw, parseError(resp.StatusCode, raw)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return raw, fmt.Errorf("decode response: %w", err)
		}
	}
	return raw, nil
}
