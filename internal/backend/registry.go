package backend

import (
	"fmt"
	"sort"
	"strings"
)

// Factory builds a Backend from its config params.
type Factory func(params map[string]string) (Backend, error)

var registry = map[string]Factory{
	"llama_cpp": NewLlamaCpp,
	"anthropic": NewAnthropic,
	"openai":    NewOpenAI,
}

// New resolves backend.type from config to a live backend. Unknown types
// error with the list of valid ones — no silent fallback.
func New(btype string, params map[string]string) (Backend, error) {
	factory, ok := registry[strings.TrimSpace(btype)]
	if !ok {
		valid := make([]string, 0, len(registry))
		for k := range registry {
			valid = append(valid, k)
		}
		sort.Strings(valid)
		return nil, fmt.Errorf("unknown backend type %q (valid: %s)", btype, strings.Join(valid, ", "))
	}
	return factory(params)
}
