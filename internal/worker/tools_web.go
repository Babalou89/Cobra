package worker

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var webClient = &http.Client{Timeout: 20 * time.Second}

var tagPat = regexp.MustCompile(`<[^>]*>`)
var wsPat = regexp.MustCompile(`\s{2,}`)

func fetchText(target string) ToolResult {
	resp, err := webClient.Get(target)
	if err != nil {
		return ToolResult{OK: false, Output: err.Error()}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return ToolResult{OK: false, Output: err.Error()}
	}
	text := tagPat.ReplaceAllString(string(data), " ")
	text = wsPat.ReplaceAllString(text, " ")
	return ToolResult{OK: resp.StatusCode < 400, Output: clip(strings.TrimSpace(text), 4000)}
}

// registerWeb wires read-only web access: fetch a URL, or run a search
// against DuckDuckGo's HTML endpoint. No credentials, no POSTs.
func registerWeb(r *Registry) {
	r.Register(&Tool{
		Name:  "web_fetch",
		Usage: `{"url": "https://example.com"}`,
		Desc:  "fetch a URL and return its text content",
		Fn: func(args map[string]any) ToolResult {
			target := argString(args, "url", "")
			if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
				return ToolResult{OK: false, Output: "url must start with http:// or https://"}
			}
			return fetchText(target)
		},
	})
	r.Register(&Tool{
		Name:  "web_search",
		Usage: `{"query": "golang yaml parser"}`,
		Desc:  "run a web search and return the result text",
		Fn: func(args map[string]any) ToolResult {
			query := argString(args, "query", "")
			if strings.TrimSpace(query) == "" {
				return ToolResult{OK: false, Output: "query must not be empty"}
			}
			return fetchText("https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query))
		},
	})
}
