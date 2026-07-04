package ctxdiet

// Retriever is anything that can pull relevant snippets for a query.
// The local memory store satisfies this when enabled.
type Retriever interface {
	Search(query string, k int) []string
}

// Retrieve is a strict no-op unless memory is enabled in config. Retrieval
// is local-only by design — there is no external memory service.
func Retrieve(r Retriever, enabled bool, query string, k int) []string {
	if !enabled || r == nil {
		return nil
	}
	return r.Search(query, k)
}
