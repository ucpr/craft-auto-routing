// Package craft provides a minimal client for the Craft docs Connect API
// (https://connect.craft.do/link/YOUR_CRAFT_CONNECT_LINK_ID/docs/v1).
package craft

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is a small HTTP client for the Craft Connect API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a Craft API client. baseURL must not have a trailing slash.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ListFolders returns the full folder tree for the space.
func (c *Client) ListFolders(ctx context.Context) ([]Folder, error) {
	var out foldersResponse
	if err := c.do(ctx, http.MethodGet, "/folders", nil, nil, &out); err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	return out.Items, nil
}

// ListDocumentsParams filters the ListDocuments call.
type ListDocumentsParams struct {
	// Location filters by special location: "unsorted", "trash", "templates", "daily_notes".
	Location string
	// FolderID filters by a specific folder.
	FolderID string
	// FetchMetadata requests createdAt/lastModifiedAt/link fields.
	FetchMetadata bool
}

// ListDocuments returns documents matching the given filter.
func (c *Client) ListDocuments(ctx context.Context, params ListDocumentsParams) ([]Document, error) {
	q := url.Values{}
	if params.Location != "" {
		q.Set("location", params.Location)
	}
	if params.FolderID != "" {
		q.Set("folderId", params.FolderID)
	}
	if params.FetchMetadata {
		q.Set("fetchMetadata", "true")
	}

	var out documentsResponse
	if err := c.do(ctx, http.MethodGet, "/documents", q, nil, &out); err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	return out.Items, nil
}

// GetBlocks returns the content tree for a document (or block) id.
func (c *Client) GetBlocks(ctx context.Context, id string, maxDepth int) (*Block, error) {
	q := url.Values{}
	q.Set("id", id)
	if maxDepth > 0 {
		q.Set("maxDepth", fmt.Sprintf("%d", maxDepth))
	}

	var out Block
	if err := c.do(ctx, http.MethodGet, "/blocks", q, nil, &out); err != nil {
		return nil, fmt.Errorf("get blocks for %s: %w", id, err)
	}
	return &out, nil
}

// MoveDocuments moves the given document ids into the given destination folder.
func (c *Client) MoveDocuments(ctx context.Context, documentIDs []string, folderID string) error {
	body := moveRequest{
		DocumentIDs: documentIDs,
		Destination: moveDestination{FolderID: folderID},
	}
	if err := c.do(ctx, http.MethodPut, "/documents/move", nil, body, nil); err != nil {
		return fmt.Errorf("move documents %v to folder %s: %w", documentIDs, folderID, err)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 300 {
		return fmt.Errorf("craft api %s %s: status %d: %s", method, path, resp.StatusCode, string(respBody))
	}

	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}
	return nil
}
