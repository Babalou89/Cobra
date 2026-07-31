package planner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client wraps the Anthropic Messages API for the planner role.
// Uses OneProvider as the default base URL.
type Client struct {
	BaseURL    string // e.g. "https://api.oneprovider.dev"
	APIKey     string
	Model      string // e.g. "claude-fable-5"
	HTTPClient *http.Client
}

// NewClient returns a planner Client with sensible defaults.
func NewClient(baseURL, apiKey, model string) *Client {
	if baseURL == "" {
		baseURL = "https://api.oneprovider.dev"
	}
	if model == "" {
		model = "claude-fable-5"
	}
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		HTTPClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// msg is the wire format for a single message.
type msg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatReq is the Anthropic Messages API request body.
type chatReq struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	System    string `json:"system,omitempty"`
	Messages  []msg  `json:"messages"`
}

// contentBlock is one block in the response content array.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// chatResp is the Anthropic Messages API response body.
type chatResp struct {
	Content []contentBlock `json:"content"`
	Error   *apiError      `json:"error,omitempty"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Chat sends a single messages request and returns the text response.
// Returns empty string and nil error on any failure (graceful degradation).
func (c *Client) Chat(system, user string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 2000
	}

	body := chatReq{
		Model:     c.Model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  []msg{{Role: "user", Content: user}},
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("planner: marshal request: %w", err)
	}

	url := c.BaseURL + "/v1/messages"
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("planner: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("planner: http call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("planner: read response: %w", err)
	}

	var cr chatResp
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return "", fmt.Errorf("planner: parse response: %w (body: %s)", err, truncate(string(respBody), 200))
	}

	if cr.Error != nil {
		return "", fmt.Errorf("planner: api error: %s (%s)", cr.Error.Message, cr.Error.Type)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("planner: http %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	// Extract text from the first content block of type "text".
	for _, block := range cr.Content {
		if block.Type == "text" && block.Text != "" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("planner: no text content in response")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
