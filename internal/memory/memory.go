// Package memory is the optional local memory store: a JSONL file searched
// with TF-IDF. Off by default; enabled only via config. Local disk only —
// no network, no external service.
package memory

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Entry is one remembered snippet.
type Entry struct {
	Time time.Time `json:"time"`
	Text string    `json:"text"`
}

// Store is a JSONL-backed memory with TF-IDF search.
type Store struct {
	Path string
}

// Open returns a store bound to path (created lazily on first Add).
func Open(path string) *Store {
	return &Store{Path: path}
}

// Add appends one entry.
func (s *Store) Add(text string) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(Entry{Time: time.Now(), Text: text})
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

func (s *Store) load() []Entry {
	f, err := os.Open(s.Path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Text != "" {
			out = append(out, e)
		}
	}
	return out
}

var tokenPat = regexp.MustCompile(`[a-zA-Z0-9_]+`)

func tokenize(s string) []string {
	return tokenPat.FindAllString(strings.ToLower(s), -1)
}

// Search returns the k entries most similar to query by TF-IDF cosine
// score. Satisfies the ctxdiet.Retriever interface.
func (s *Store) Search(query string, k int) []string {
	entries := s.load()
	if len(entries) == 0 || k <= 0 {
		return nil
	}
	docs := make([][]string, len(entries))
	df := map[string]int{}
	for i, e := range entries {
		docs[i] = tokenize(e.Text)
		seen := map[string]bool{}
		for _, t := range docs[i] {
			if !seen[t] {
				seen[t] = true
				df[t]++
			}
		}
	}
	n := float64(len(entries))
	qTokens := tokenize(query)
	type scored struct {
		idx   int
		score float64
	}
	var results []scored
	for i, doc := range docs {
		tf := map[string]float64{}
		for _, t := range doc {
			tf[t]++
		}
		var score float64
		for _, q := range qTokens {
			if tf[q] == 0 {
				continue
			}
			idf := math.Log(1 + n/float64(1+df[q]))
			score += (tf[q] / float64(len(doc)+1)) * idf
		}
		if score > 0 {
			results = append(results, scored{i, score})
		}
	}
	sort.Slice(results, func(a, b int) bool { return results[a].score > results[b].score })
	if len(results) > k {
		results = results[:k]
	}
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, entries[r.idx].Text)
	}
	return out
}
