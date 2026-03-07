package inference

import (
	"encoding/json"
	"unicode/utf8"

	"forge/pkg/types"
)

// EstimateTokens provides a more accurate token count estimate than naive len/4.
// It accounts for:
// - Per-message overhead (role, delimiters): ~4 tokens each
// - Whitespace tokenization (spaces often merge with adjacent tokens)
// - Non-ASCII characters (CJK often = 1-2 tokens per char, not char/4)
// - Code/punctuation (often 1 token per symbol)
// - Conversation overhead: ~3 tokens
//
// This is still an estimate, but typically within 20% of actual BPE counts
// for common LLM tokenizers (cl100k_base, llama).
func EstimateTokens(messages []types.ChatMessage) int {
	total := 3 // conversation overhead (priming)
	for _, msg := range messages {
		total += 4 // per-message overhead: <|role|>, content, <|end|>
		total += estimateContentTokens(msg.Content)
		for _, tc := range msg.ToolCalls {
			total += estimateStringTokens(tc.Function.Name) + estimateStringTokens(tc.Function.Arguments) + 4
		}
	}
	if total < 1 {
		total = 1
	}
	return total
}

func estimateContentTokens(content interface{}) int {
	switch v := content.(type) {
	case string:
		return estimateStringTokens(v)
	case nil:
		return 0
	default:
		data, _ := json.Marshal(v)
		return estimateStringTokens(string(data))
	}
}

func estimateStringTokens(s string) int {
	if len(s) == 0 {
		return 0
	}

	tokens := 0
	runeCount := utf8.RuneCountInString(s)
	byteCount := len(s)

	// For ASCII-heavy text (English, code): ~3.5-4 chars per token
	// For CJK/emoji text: ~1-2 chars per token
	// Heuristic: weight by ratio of multi-byte to single-byte chars
	multiByte := byteCount - runeCount // extra bytes from multi-byte chars

	if runeCount == 0 {
		return 0
	}

	multiByteRatio := float64(multiByte) / float64(byteCount)

	if multiByteRatio > 0.3 {
		// Heavy non-ASCII (CJK, emoji, etc): ~1.5 runes per token
		tokens = (runeCount*2 + 2) / 3
	} else {
		// Mostly ASCII: count words + punctuation
		// Words ≈ tokens, punctuation often separate tokens
		words := 1
		punctuation := 0
		inWhitespace := false
		for _, r := range s {
			if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
				if !inWhitespace {
					words++
					inWhitespace = true
				}
			} else {
				inWhitespace = false
				if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
					punctuation++
				}
			}
		}
		// Subwords: long words get split. Rough: 1.3 tokens per word average
		tokens = words*13/10 + punctuation/2
		// But also use char-based estimate as floor
		asciiRunes := runeCount - multiByte // approximate single-byte rune count
		charEstimate := asciiRunes / 4
		if charEstimate > tokens {
			tokens = charEstimate
		}
	}

	if tokens < 1 && runeCount > 0 {
		tokens = 1
	}
	return tokens
}
