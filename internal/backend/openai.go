package backend

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

// OpenAI talks to any OpenAI-compatible endpoint (OpenAI itself, OpenRouter,
// vLLM, …) with Bearer auth. The key comes from OPENAI_API_KEY or, as a
// fallback, OPENROUTER_API_KEY.
type OpenAI struct {
	baseURL string
	model   string
	maxCtx  int
	client  *http.Client
}

// NewOpenAI builds the backend from params: base_url, model, max_context.
func NewOpenAI(params map[string]string) (Backend, error) {
	return &OpenAI{
		baseURL: strings.TrimRight(param(params, "base_url", "https://api.openai.com"), "/"),
		model:   param(params, "model", "gpt-4o-mini"),
		maxCtx:  paramInt(params, "max_context", 128000),
		client:  newHTTPClient(),
	}, nil
}

func (o *OpenAI) key() string {
	if k := os.Getenv("OPENAI_API_KEY"); k != "" {
		return k
	}
	return os.Getenv("OPENROUTER_API_KEY")
}

func (o *OpenAI) Chat(messages []Msg, temperature float64, maxTokens int) (string, error) {
	key := o.key()
	if key == "" {
		return "", fmt.Errorf("OPENAI_API_KEY (or OPENROUTER_API_KEY) is not set in the environment")
	}
	headers := map[string]string{"Authorization": "Bearer " + key}
	return chatOpenAI(o.client, o.baseURL+"/v1/chat/completions", headers, o.model, messages, temperature, maxTokens)
}

func (o *OpenAI) Health() bool { return o.key() != "" }

func (o *OpenAI) Name() string { return "openai" }

func (o *OpenAI) MaxContext() int { return o.maxCtx }
