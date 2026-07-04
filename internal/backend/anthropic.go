package backend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Anthropic talks to the Claude API /v1/messages endpoint using only the
// standard library. The key comes from ANTHROPIC_API_KEY in the environment.
type Anthropic struct {
	baseURL string
	model   string
	maxCtx  int
	client  *http.Client
}

// NewAnthropic builds the backend from params: base_url, model, max_context.
func NewAnthropic(params map[string]string) (Backend, error) {
	return &Anthropic{
		baseURL: strings.TrimRight(param(params, "base_url", "https://api.anthropic.com"), "/"),
		model:   param(params, "model", "claude-sonnet-5"),
		maxCtx:  paramInt(params, "max_context", 200000),
		client:  newHTTPClient(),
	}, nil
}

func (a *Anthropic) headers() (map[string]string, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set in the environment")
	}
	return map[string]string{
		"x-api-key":         key,
		"anthropic-version": "2023-06-01",
	}, nil
}

func (a *Anthropic) Chat(messages []Msg, temperature float64, maxTokens int) (string, error) {
	headers, err := a.headers()
	if err != nil {
		return "", err
	}
	// The messages API takes the system prompt as a top-level field and
	// only user/assistant roles in the messages array.
	var system string
	turns := make([]openAIMessage, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "system":
			if system != "" {
				system += "\n\n"
			}
			system += m.Content
		case "assistant":
			turns = append(turns, openAIMessage{Role: "assistant", Content: m.Content})
		default: // user, tool
			turns = append(turns, openAIMessage{Role: "user", Content: m.Content})
		}
	}
	body := map[string]any{
		"model":       a.model,
		"max_tokens":  maxTokens,
		"temperature": temperature,
		"messages":    turns,
	}
	if system != "" {
		body["system"] = system
	}
	data, err := postJSON(a.client, a.baseURL+"/v1/messages", headers, body)
	if err != nil {
		return "", err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	var sb strings.Builder
	for _, block := range out.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	if sb.Len() == 0 {
		return "", fmt.Errorf("backend returned no text content")
	}
	return sb.String(), nil
}

func (a *Anthropic) Health() bool {
	_, err := a.headers()
	return err == nil
}

func (a *Anthropic) Name() string { return "anthropic" }

func (a *Anthropic) MaxContext() int { return a.maxCtx }
