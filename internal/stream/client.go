// Package stream wraps the small slice of the Cloudflare Stream API that
// mediaflow needs: provisioning Direct Creator Upload URLs, fetching
// post-ingest metadata, and deleting videos.
package stream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiBase = "https://api.cloudflare.com/client/v4"

type Client struct {
	accountID string
	apiToken  string
	http      *http.Client
}

func NewClient(accountID, apiToken string) *Client {
	return &Client{
		accountID: accountID,
		apiToken:  apiToken,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

// Configured reports whether the credentials needed for any Stream call are present.
func (c *Client) Configured() bool {
	return c != nil && c.accountID != "" && c.apiToken != ""
}

type DirectUploadRequest struct {
	MaxDurationSeconds int               `json:"maxDurationSeconds"`
	Expiry             string            `json:"expiry,omitempty"`
	Meta               map[string]string `json:"meta,omitempty"`
	RequireSignedURLs  bool              `json:"requireSignedURLs,omitempty"`
	AllowedOrigins     []string          `json:"allowedOrigins,omitempty"`
}

type DirectUploadResult struct {
	UploadURL string
	UID       string
}

// CreateDirectUpload provisions a one-time upload URL plus a stable video UID.
// The URL accepts plain POST for ≤200MB or TUS for larger files.
func (c *Client) CreateDirectUpload(ctx context.Context, req DirectUploadRequest) (*DirectUploadResult, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("stream client not configured (missing STREAM_ACCOUNT_ID or STREAM_API_TOKEN)")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal direct_upload request: %w", err)
	}

	url := fmt.Sprintf("%s/accounts/%s/stream/direct_upload", apiBase, c.accountID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("stream direct_upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("stream direct_upload status %d: %s", resp.StatusCode, string(raw))
	}

	var parsed struct {
		Result struct {
			UploadURL string `json:"uploadURL"`
			UID       string `json:"uid"`
		} `json:"result"`
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode direct_upload response: %w", err)
	}
	if !parsed.Success || parsed.Result.UploadURL == "" || parsed.Result.UID == "" {
		return nil, fmt.Errorf("stream direct_upload returned empty result")
	}
	return &DirectUploadResult{
		UploadURL: parsed.Result.UploadURL,
		UID:       parsed.Result.UID,
	}, nil
}

// VideoDetails is the slice of GET /stream/{uid} we need for post-upload validation.
type VideoDetails struct {
	UID            string
	ReadyToStream  bool
	StatusState    string
	DurationSec    float64
	Width          int
	Height         int
	InputCodec     string
}

func (c *Client) GetVideo(ctx context.Context, uid string) (*VideoDetails, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("stream client not configured")
	}

	url := fmt.Sprintf("%s/accounts/%s/stream/%s", apiBase, c.accountID, uid)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("stream get video: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrVideoNotFound
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("stream get video status %d: %s", resp.StatusCode, string(raw))
	}

	var parsed struct {
		Result struct {
			UID           string  `json:"uid"`
			ReadyToStream bool    `json:"readyToStream"`
			Duration      float64 `json:"duration"`
			Status        struct {
				State string `json:"state"`
			} `json:"status"`
			Input struct {
				Width  int `json:"width"`
				Height int `json:"height"`
			} `json:"input"`
			Meta map[string]string `json:"meta"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode get video response: %w", err)
	}

	return &VideoDetails{
		UID:           parsed.Result.UID,
		ReadyToStream: parsed.Result.ReadyToStream,
		StatusState:   parsed.Result.Status.State,
		DurationSec:   parsed.Result.Duration,
		Width:         parsed.Result.Input.Width,
		Height:        parsed.Result.Input.Height,
	}, nil
}

func (c *Client) DeleteVideo(ctx context.Context, uid string) error {
	if !c.Configured() {
		return fmt.Errorf("stream client not configured")
	}

	url := fmt.Sprintf("%s/accounts/%s/stream/%s", apiBase, c.accountID, uid)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("stream delete video: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stream delete video status %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

var ErrVideoNotFound = fmt.Errorf("stream video not found")
