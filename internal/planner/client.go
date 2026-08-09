package planner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client wraps an OpenAI-compatible /v1/chat/completions endpoint for the
// planner role. Works with llama.cpp's built-in server.
type Client struct {
	BaseURL    string // e.g. "http://127.0.0.1:8080"
	Model      string // e.g. "local"
	HTTPClient *http.Client
}

// NewClient returns a planner Client targeting a local llama-server.
func NewClient(baseURL, apiKey, model string) *Client {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8080"
	}
	if model == "" {
		model = "local"
	}
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatReq struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *apiError `json:"error,omitempty"`
}

type apiError struct {
	Message string `json:"message"`
}

// Chat sends a single request and returns the text response.
// Returns empty string and nil error on any failure (graceful degradation).
func (c *Client) Chat(system, user string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 2000
	}

	body := chatReq{
		Model:       c.Model,
		Messages:    []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: user}},
		MaxTokens:   maxTokens,
		Temperature: 0.3,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("planner: marshal request: %w", err)
	}

	url := c.BaseURL + "/v1/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("planner: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("planner: http call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("planner: read response: %w", err)
	}

	var cr chatResp
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return "", fmt.Errorf("planner: parse response: %w (body: %s)", err, truncate(string(respBody), 200))
	}

	if cr.Error != nil {
		return "", fmt.Errorf("planner: api error: %s", cr.Error.Message)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("planner: http %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("planner: no choices in response")
	}

	return cr.Choices[0].Message.Content, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
