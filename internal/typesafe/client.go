// Package typesafe is a minimal client for the TypeSafe SystemOne evaluation
// API (https://docs.typesafe.ai), used here with the "jev" model family to
// classify text against a set of choices.
package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.typesafe.ai"

// Client is a small HTTP client for the TypeSafe SystemOne API.
type Client struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the default TypeSafe API base URL.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) { c.baseURL = baseURL }
}

// NewClient creates a TypeSafe API client. model is typically "jev-latest".
func NewClient(apiKey, model string, opts ...Option) *Client {
	c := &Client{
		baseURL: defaultBaseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Classify asks the model to pick the single best-matching key from options
// (key -> human-readable description) for the given state text.
func (c *Client) Classify(ctx context.Context, questionKey, instructions, state string, options map[string]string) (ClassifyResult, error) {
	if len(options) == 0 {
		return ClassifyResult{}, fmt.Errorf("classify: no options provided")
	}

	reqBody := systemOneRequest{
		State: state,
		Model: c.model,
		Questions: map[string]choiceQuestion{
			questionKey: {
				Type:         "choice",
				Instructions: instructions,
				Criteria:     options,
			},
		},
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return ClassifyResult{}, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/systemone", bytes.NewReader(b))
	if err != nil {
		return ClassifyResult{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ClassifyResult{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ClassifyResult{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		return ClassifyResult{}, fmt.Errorf("typesafe api: status %d: %s", resp.StatusCode, string(respBody))
	}

	var out systemOneResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return ClassifyResult{}, fmt.Errorf("decode response: %w", err)
	}

	answer, ok := out.Answers[questionKey]
	if !ok {
		return ClassifyResult{}, fmt.Errorf("typesafe api: missing answer for question %q", questionKey)
	}

	return ClassifyResult{
		Choice:        answer.Choice,
		Confidence:    answer.Confidence,
		Probabilities: answer.Probabilities,
	}, nil
}
