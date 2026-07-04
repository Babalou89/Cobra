package backend

import (
	"net/http"
	"strings"
)

// LlamaCpp talks to a local llama-server over its OpenAI-compatible
// /v1/chat/completions endpoint. No auth, no SDK.
type LlamaCpp struct {
	baseURL string
	model   string
	maxCtx  int
	client  *http.Client
}

// NewLlamaCpp builds the backend from params: base_url, model, max_context.
func NewLlamaCpp(params map[string]string) (Backend, error) {
	return &LlamaCpp{
		baseURL: strings.TrimRight(param(params, "base_url", "http://127.0.0.1:8080"), "/"),
		model:   param(params, "model", "local"),
		maxCtx:  paramInt(params, "max_context", 32768),
		client:  newHTTPClient(),
	}, nil
}

func (l *LlamaCpp) Chat(messages []Msg, temperature float64, maxTokens int) (string, error) {
	return chatOpenAI(l.client, l.baseURL+"/v1/chat/completions", nil, l.model, messages, temperature, maxTokens)
}

func (l *LlamaCpp) Health() bool {
	resp, err := l.client.Get(l.baseURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (l *LlamaCpp) Name() string { return "llama_cpp" }

func (l *LlamaCpp) MaxContext() int { return l.maxCtx }
