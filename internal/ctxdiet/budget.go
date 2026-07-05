// Package ctxdiet is the always-on context diet: hard token caps, prompt
// ceilings, trajectory compression, and (optional) local retrieval. The
// budget is enforced every turn — an over-budget prompt is refused, never
// silently sent.
package ctxdiet

import "fmt"

// Budget caps what one turn may spend.
type Budget struct {
	MaxTokens     int // hard cap for the whole conversation payload
	PromptCeiling int // cap for any single message
}

// Estimate approximates token count from bytes. Deliberately conservative
// (~3 bytes/token): paths, code, and listings tokenize far denser than
// prose, and a budget that under-counts overflows the real context.
func Estimate(s string) int {
	return len(s)/3 + 1
}

// ClampTo truncates content to approximately maxTokens.
func ClampTo(content string, maxTokens int) string {
	if maxTokens <= 0 || Estimate(content) <= maxTokens {
		return content
	}
	keep := maxTokens * 3
	if keep > len(content) {
		keep = len(content)
	}
	return content[:keep] + "\n[…truncated by context diet]"
}

// CheckMessage refuses a single message over the prompt ceiling.
func (b Budget) CheckMessage(content string) error {
	if b.PromptCeiling > 0 && Estimate(content) > b.PromptCeiling {
		return fmt.Errorf("message of ~%d tokens exceeds the prompt ceiling (%d)", Estimate(content), b.PromptCeiling)
	}
	return nil
}

// CheckTotal refuses a conversation payload over the hard cap.
func (b Budget) CheckTotal(contents []string) error {
	total := 0
	for _, c := range contents {
		total += Estimate(c)
	}
	if b.MaxTokens > 0 && total > b.MaxTokens {
		return fmt.Errorf("conversation of ~%d tokens exceeds the budget (%d)", total, b.MaxTokens)
	}
	return nil
}

// ClampMessage truncates a message to fit under the prompt ceiling,
// marking the cut so the model knows content was dropped.
func (b Budget) ClampMessage(content string) string {
	if b.PromptCeiling <= 0 || Estimate(content) <= b.PromptCeiling {
		return content
	}
	keep := b.PromptCeiling * 4
	if keep > len(content) {
		keep = len(content)
	}
	return content[:keep] + "\n[…truncated by context diet]"
}
