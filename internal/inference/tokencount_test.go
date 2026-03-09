package inference

import (
	"strings"
	"testing"

	"cortex/pkg/types"
)

func TestEstimateTokens_EmptyMessages(t *testing.T) {
	count := EstimateTokens(nil)
	// Just conversation overhead (3), no messages
	if count < 1 || count > 5 {
		t.Errorf("empty messages: got %d, want 1-5", count)
	}

	count = EstimateTokens([]types.ChatMessage{})
	if count < 1 || count > 5 {
		t.Errorf("empty slice: got %d, want 1-5", count)
	}
}

func TestEstimateTokens_SimpleEnglish(t *testing.T) {
	msgs := []types.ChatMessage{
		{Role: "user", Content: "Hello, world!"},
	}
	count := EstimateTokens(msgs)
	// "Hello, world!" is 4 tokens with cl100k_base.
	// With overhead: 3 (conv) + 4 (msg) + content tokens
	// Content: 2 words + some punctuation => ~3-5 content tokens
	// Total: ~10-12
	if count < 8 || count > 16 {
		t.Errorf("simple english: got %d, want 8-16", count)
	}
}

func TestEstimateTokens_LongEnglish(t *testing.T) {
	// ~100 words of English text
	paragraph := "The quick brown fox jumps over the lazy dog. " +
		"This is a longer paragraph that contains approximately one hundred words " +
		"to test the token estimation accuracy for English text. " +
		"Token estimation is important for managing context windows in large language models. " +
		"A good estimate should be within twenty percent of the actual BPE token count. " +
		"The naive approach of dividing character count by four works reasonably well " +
		"for English text but fails badly for code, CJK characters, and other non-ASCII content. " +
		"We need a better heuristic that accounts for word boundaries and punctuation."

	msgs := []types.ChatMessage{
		{Role: "user", Content: paragraph},
	}
	count := EstimateTokens(msgs)
	// ~85 words, actual BPE would be ~100-120 tokens for content
	// Plus overhead: 3 + 4 = 7
	// Total: ~107-127
	if count < 80 || count > 180 {
		t.Errorf("long english: got %d, want 80-180", count)
	}
}

func TestEstimateTokens_CJK(t *testing.T) {
	msgs := []types.ChatMessage{
		{Role: "user", Content: "你好世界"},
	}
	count := EstimateTokens(msgs)
	// "你好世界" = 4 CJK chars, each typically 1-2 tokens in BPE
	// Old naive len("你好世界")/4 = 12/4 = 3 which is too low
	// New heuristic: multiByteRatio > 0.3 → (4*2+2)/3 ≈ 3 content tokens
	// Plus overhead: 3 + 4 = 7, total ~10
	// Key property: per-character estimate should be HIGHER than English
	if count < 8 || count > 18 {
		t.Errorf("CJK: got %d, want 8-18", count)
	}

	// Verify CJK gives higher per-char estimate than English
	englishMsgs := []types.ChatMessage{
		{Role: "user", Content: "abcd"}, // 4 ASCII chars
	}
	englishCount := EstimateTokens(englishMsgs)
	// CJK count should be >= English count for same number of chars
	cjkContentTokens := count - 7      // subtract overhead
	engContentTokens := englishCount - 7 // subtract overhead
	if cjkContentTokens < engContentTokens {
		t.Errorf("CJK per-char estimate (%d) should be >= English per-char estimate (%d) for same char count",
			cjkContentTokens, engContentTokens)
	}
}

func TestEstimateTokens_Code(t *testing.T) {
	code := `func main() { fmt.Println("hello") }`
	msgs := []types.ChatMessage{
		{Role: "user", Content: code},
	}
	count := EstimateTokens(msgs)
	// Code has lots of punctuation: (, ), {, }, ., (, ", ", )
	// Should give more tokens than naive len/4 = 37/4 = 9
	// With punctuation counting, should be higher
	if count < 10 || count > 30 {
		t.Errorf("code: got %d, want 10-30", count)
	}
}

func TestEstimateTokens_MixedContent(t *testing.T) {
	msgs := []types.ChatMessage{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "What is the capital of France?"},
	}
	count := EstimateTokens(msgs)
	// 2 messages: 3 (conv) + 2*4 (msg overhead) + content tokens
	// Content1: "You are a helpful assistant." ~6-7 words => ~8-10 tokens
	// Content2: "What is the capital of France?" ~7 words => ~9-11 tokens
	// Total: ~28-32
	if count < 18 || count > 40 {
		t.Errorf("mixed content: got %d, want 18-40", count)
	}
}

func TestEstimateTokens_ToolCalls(t *testing.T) {
	msgs := []types.ChatMessage{
		{
			Role:    "assistant",
			Content: "Let me search for that.",
			ToolCalls: []types.ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					Function: types.FunctionCall{
						Name:      "search",
						Arguments: `{"query": "capital of France"}`,
					},
				},
			},
		},
	}
	count := EstimateTokens(msgs)
	// Should include: 3 (conv) + 4 (msg) + content tokens + tool call tokens
	// Content: "Let me search for that." ~6 words => ~7-9 tokens
	// ToolCall: "search" ~1 token + arguments ~8-10 tokens + 4 overhead
	// Total: ~25-35
	if count < 15 || count > 45 {
		t.Errorf("tool calls: got %d, want 15-45", count)
	}

	// Verify tool calls add to count vs same message without tool calls
	msgsNoTools := []types.ChatMessage{
		{Role: "assistant", Content: "Let me search for that."},
	}
	countNoTools := EstimateTokens(msgsNoTools)
	if count <= countNoTools {
		t.Errorf("tool call message (%d) should have more tokens than without (%d)", count, countNoTools)
	}
}

func TestEstimateTokens_NonStringContent(t *testing.T) {
	// map[string]interface{} content (e.g. multimodal content parts)
	content := map[string]interface{}{
		"type": "text",
		"text": "Hello, world!",
	}
	msgs := []types.ChatMessage{
		{Role: "user", Content: content},
	}
	count := EstimateTokens(msgs)
	// Should marshal to JSON and estimate that
	// {"text":"Hello, world!","type":"text"} is ~40 chars
	if count < 8 || count > 30 {
		t.Errorf("non-string content: got %d, want 8-30", count)
	}
}

func TestEstimateTokens_NilContent(t *testing.T) {
	msgs := []types.ChatMessage{
		{Role: "assistant", Content: nil},
	}
	// Should not panic
	count := EstimateTokens(msgs)
	// Just overhead: 3 (conv) + 4 (msg) = 7
	if count < 5 || count > 10 {
		t.Errorf("nil content: got %d, want 5-10", count)
	}
}

func TestEstimateTokens_EmptyStringContent(t *testing.T) {
	msgs := []types.ChatMessage{
		{Role: "user", Content: ""},
	}
	count := EstimateTokens(msgs)
	// Just overhead: 3 (conv) + 4 (msg) = 7
	if count < 5 || count > 10 {
		t.Errorf("empty string content: got %d, want 5-10", count)
	}
}

func TestEstimateTokens_WhitespaceHeavy(t *testing.T) {
	// Lots of whitespace — the old len/4 would over-count
	content := strings.Repeat("  \n  \t  ", 20) + "hello"
	msgs := []types.ChatMessage{
		{Role: "user", Content: content},
	}
	count := EstimateTokens(msgs)
	// Heavy whitespace should not wildly inflate the count
	// Old naive: 165/4 = 41 tokens — way too many for mostly whitespace
	if count < 7 || count > 60 {
		t.Errorf("whitespace heavy: got %d, want 7-60", count)
	}
}

func TestEstimateStringTokens_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMin int
		wantMax int
	}{
		{"empty", "", 0, 0},
		{"single char", "a", 1, 1},
		{"single CJK", "你", 1, 2},
		{"numbers", "1234567890", 1, 5},
		{"punctuation only", "!@#$%^&*()", 1, 10},
		{"mixed CJK and ASCII", "Hello你好World世界", 3, 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateStringTokens(tt.input)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("estimateStringTokens(%q) = %d, want %d-%d", tt.input, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}
