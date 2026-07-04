// Package backend is the model-agnostic layer. The binary never embeds a
// model and never assumes one; everything is reached over HTTP through the
// Backend interface. Stdlib only — no SDK dependencies. API keys come from
// environment variables, never from source or config files.
package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Msg is one turn of conversation handed to a backend.
type Msg struct {
	Role    string // "system" | "user" | "assistant" | "tool"
	Content string
}

// Backend is the only contract the rest of the binary knows about.
type Backend interface {
	Chat(messages []Msg, temperature float64, maxTokens int) (string, error)
	Health() bool
	Name() string
	MaxContext() int
}

// param reads a string parameter with a default.
func param(params map[string]string, key, def string) string {
	if v, ok := params[key]; ok && v != "" {
		return v
	}
	return def
}

// paramInt reads an integer parameter with a default.
func paramInt(params map[string]string, key string, def int) int {
	if v, ok := params[key]; ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: 300 * time.Second}
}

// openAIMessage mirrors the OpenAI-compatible chat message shape used by
// both llama.cpp's server and OpenAI-compatible endpoints.
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func toOpenAI(msgs []Msg) []openAIMessage {
	out := make([]openAIMessage, 0, len(msgs))
	for _, m := range msgs {
		role := m.Role
		if role == "tool" {
			// OpenAI tool role requires call ids; we run a plain-text
			// protocol, so tool results travel as user turns.
			role = "user"
		}
		out = append(out, openAIMessage{Role: role, Content: m.Content})
	}
	return out
}

// postJSON POSTs a JSON body and returns the raw response bytes.
func postJSON(client *http.Client, url string, headers map[string]string, body any) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("backend HTTP %d: %s", resp.StatusCode, truncate(string(data), 500))
	}
	return data, nil
}

// chatOpenAI runs an OpenAI-compatible /v1/chat/completions exchange and
// extracts the first choice's message content.
func chatOpenAI(client *http.Client, url string, headers map[string]string, model string, msgs []Msg, temperature float64, maxTokens int) (string, error) {
	body := map[string]any{
		"model":       model,
		"messages":    toOpenAI(msgs),
		"temperature": temperature,
		"max_tokens":  maxTokens,
	}
	data, err := postJSON(client, url, headers, body)
	if err != nil {
		return "", err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("backend returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
