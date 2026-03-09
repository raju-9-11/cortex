package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cortex/pkg/types"
)

// makeEvents creates a channel and sends the given events, then closes it.
func makeEvents(evts ...types.StreamEvent) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, len(evts))
	for _, e := range evts {
		ch <- e
	}
	close(ch)
	return ch
}

// contentDelta is a shorthand for building a content delta event.
func contentDelta(s string) types.StreamEvent {
	return types.StreamEvent{Type: types.EventContentDelta, Delta: s}
}

func TestRenderStream_NormalFlow(t *testing.T) {
	deltas := []string{"Hello", ", ", "world", "!", "\n"}
	evts := make([]types.StreamEvent, len(deltas))
	for i, d := range deltas {
		evts[i] = contentDelta(d)
	}

	var buf bytes.Buffer
	fullText, err := RenderStream(context.Background(), makeEvents(evts...), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantFull := "Hello, world!\n"
	if fullText != wantFull {
		t.Errorf("fullText = %q, want %q", fullText, wantFull)
	}

	output := buf.String()
	// Output should contain the text (possibly with ANSI reset at the end).
	if !strings.Contains(output, "Hello, world!") {
		t.Errorf("output missing expected text, got: %q", output)
	}
}

func TestRenderStream_BoldFormatting(t *testing.T) {
	evts := []types.StreamEvent{
		contentDelta("some "),
		contentDelta("**bold**"),
		contentDelta(" text"),
	}

	var buf bytes.Buffer
	fullText, err := RenderStream(context.Background(), makeEvents(evts...), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// fullText should be raw markdown without ANSI
	wantFull := "some bold text"
	if fullText != wantFull {
		t.Errorf("fullText = %q, want %q", fullText, wantFull)
	}

	output := buf.String()
	// Output should contain Bold and Reset ANSI codes around "bold"
	if !strings.Contains(output, Bold) {
		t.Errorf("output missing Bold ANSI code, got: %q", output)
	}
	if !strings.Contains(output, Reset) {
		t.Errorf("output missing Reset ANSI code, got: %q", output)
	}
	// Verify "bold" text is present
	if !strings.Contains(output, "bold") {
		t.Errorf("output missing 'bold' text, got: %q", output)
	}
}

func TestRenderStream_CodeBlock(t *testing.T) {
	evts := []types.StreamEvent{
		contentDelta("before\n"),
		contentDelta("```\n"),
		contentDelta("code line\n"),
		contentDelta("```\n"),
		contentDelta("after"),
	}

	var buf bytes.Buffer
	fullText, err := RenderStream(context.Background(), makeEvents(evts...), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// fullText should have raw content without ANSI
	if !strings.Contains(fullText, "code line") {
		t.Errorf("fullText missing 'code line', got: %q", fullText)
	}
	if !strings.Contains(fullText, "before") {
		t.Errorf("fullText missing 'before', got: %q", fullText)
	}
	if !strings.Contains(fullText, "after") {
		t.Errorf("fullText missing 'after', got: %q", fullText)
	}

	output := buf.String()
	// Output should contain Dim ANSI code for code block content
	if !strings.Contains(output, Dim) {
		t.Errorf("output missing Dim ANSI code, got: %q", output)
	}
}

func TestRenderStream_InlineCode(t *testing.T) {
	evts := []types.StreamEvent{
		contentDelta("use "),
		contentDelta("`fmt.Println`"),
		contentDelta(" here"),
	}

	var buf bytes.Buffer
	fullText, err := RenderStream(context.Background(), makeEvents(evts...), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantFull := "use fmt.Println here"
	if fullText != wantFull {
		t.Errorf("fullText = %q, want %q", fullText, wantFull)
	}

	output := buf.String()
	if !strings.Contains(output, Cyan) {
		t.Errorf("output missing Cyan ANSI code, got: %q", output)
	}
	if !strings.Contains(output, "fmt.Println") {
		t.Errorf("output missing 'fmt.Println', got: %q", output)
	}
}

func TestRenderStream_ToolCall(t *testing.T) {
	evts := []types.StreamEvent{
		{
			Type:     types.EventToolCallStart,
			ToolCall: &types.ToolCallEvent{ID: "1", Name: "readFile"},
		},
		{
			Type:     types.EventToolCallDelta,
			ToolCall: &types.ToolCallEvent{Arguments: `{"path":"foo.go"}`},
		},
		{
			Type: types.EventToolCallComplete,
		},
	}

	var buf bytes.Buffer
	fullText, err := RenderStream(context.Background(), makeEvents(evts...), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tool call events should NOT accumulate into fullText.
	if fullText != "" {
		t.Errorf("fullText should be empty for tool calls, got: %q", fullText)
	}

	output := buf.String()
	if !strings.Contains(output, "[Calling: readFile]") {
		t.Errorf("output missing tool call start, got: %q", output)
	}
	if !strings.Contains(output, Yellow) {
		t.Errorf("output missing Yellow ANSI code, got: %q", output)
	}
	if !strings.Contains(output, `{"path":"foo.go"}`) {
		t.Errorf("output missing tool arguments, got: %q", output)
	}
	if !strings.Contains(output, Dim) {
		t.Errorf("output missing Dim ANSI code for tool arguments, got: %q", output)
	}
	if !strings.Contains(output, "[Tool call complete]") {
		t.Errorf("output missing tool call complete, got: %q", output)
	}
	if !strings.Contains(output, Green) {
		t.Errorf("output missing Green ANSI code, got: %q", output)
	}
}

func TestRenderStream_ErrorEvent(t *testing.T) {
	testErr := errors.New("something broke")
	evts := []types.StreamEvent{
		contentDelta("partial"),
		{
			Type:         types.EventError,
			Error:        testErr,
			ErrorMessage: "something broke",
		},
	}

	var buf bytes.Buffer
	fullText, err := RenderStream(context.Background(), makeEvents(evts...), &buf)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "something broke" {
		t.Errorf("err = %v, want 'something broke'", err)
	}

	// fullText should contain the partial content before the error.
	if fullText != "partial" {
		t.Errorf("fullText = %q, want %q", fullText, "partial")
	}

	output := buf.String()
	if !strings.Contains(output, Red) {
		t.Errorf("output missing Red ANSI code, got: %q", output)
	}
	if !strings.Contains(output, "Error: something broke") {
		t.Errorf("output missing error message, got: %q", output)
	}
}

func TestRenderStream_EmptyStream(t *testing.T) {
	ch := make(chan types.StreamEvent)
	close(ch)

	var buf bytes.Buffer
	fullText, err := RenderStream(context.Background(), ch, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fullText != "" {
		t.Errorf("fullText = %q, want empty", fullText)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output, got: %q", buf.String())
	}
}

func TestRenderStream_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Use an unbuffered channel so we can control delivery.
	ch := make(chan types.StreamEvent, 10)
	ch <- contentDelta("hello")
	ch <- contentDelta(" world")

	// Send two events, then cancel.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	var buf bytes.Buffer
	fullText, err := RenderStream(ctx, ch, &buf)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}

	// Should have at least partial text.
	if !strings.Contains(fullText, "hello") {
		t.Errorf("fullText missing 'hello', got: %q", fullText)
	}
}

func TestRenderStream_SplitBold(t *testing.T) {
	// Bold markers split across deltas: "**" is split into "*" and "*"
	evts := []types.StreamEvent{
		contentDelta("hello *"),
		contentDelta("*world*"),
		contentDelta("* end"),
	}

	var buf bytes.Buffer
	fullText, err := RenderStream(context.Background(), makeEvents(evts...), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// fullText should be raw: "hello world end"
	wantFull := "hello world end"
	if fullText != wantFull {
		t.Errorf("fullText = %q, want %q", fullText, wantFull)
	}

	output := buf.String()
	// Should contain Bold ANSI
	if !strings.Contains(output, Bold) {
		t.Errorf("output missing Bold ANSI code for split bold, got: %q", output)
	}
	// Should contain "world" between bold markers
	if !strings.Contains(output, "world") {
		t.Errorf("output missing 'world', got: %q", output)
	}
}

func TestRenderStream_TrailingNewline(t *testing.T) {
	// If last delta doesn't end with newline, one should be added.
	evts := []types.StreamEvent{
		contentDelta("no newline at end"),
	}

	var buf bytes.Buffer
	_, err := RenderStream(context.Background(), makeEvents(evts...), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.HasSuffix(output, "\n") {
		t.Errorf("output should end with newline, got: %q", output)
	}
}

func TestRenderStream_TrailingNewlineAlreadyPresent(t *testing.T) {
	evts := []types.StreamEvent{
		contentDelta("ends with newline\n"),
	}

	var buf bytes.Buffer
	_, err := RenderStream(context.Background(), makeEvents(evts...), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// Should not have double newline at end
	if strings.HasSuffix(output, "\n\n") {
		t.Errorf("output should not have double trailing newline, got: %q", output)
	}
}

func TestRenderStream_ContentDoneAndStatus(t *testing.T) {
	// content.done and status events should be silent.
	evts := []types.StreamEvent{
		contentDelta("data"),
		{Type: types.EventContentDone},
		{Type: types.EventStatus},
	}

	var buf bytes.Buffer
	fullText, err := RenderStream(context.Background(), makeEvents(evts...), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fullText != "data" {
		t.Errorf("fullText = %q, want 'data'", fullText)
	}
}

func TestRenderStream_Emoji(t *testing.T) {
	evts := []types.StreamEvent{
		contentDelta("Hello 🔥 world"),
	}

	var buf bytes.Buffer
	fullText, err := RenderStream(context.Background(), makeEvents(evts...), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantFull := "Hello 🔥 world"
	if fullText != wantFull {
		t.Errorf("fullText = %q, want %q", fullText, wantFull)
	}

	output := buf.String()
	if !strings.Contains(output, "🔥") {
		t.Errorf("output missing emoji, got: %q", output)
	}
}

func TestRenderStream_SplitUTF8(t *testing.T) {
	// 🔥 is U+1F525, UTF-8: F0 9F 94 A5
	// Split: first 2 bytes in delta 1, last 2 bytes in delta 2.
	fire := []byte("🔥") // F0 9F 94 A5
	delta1 := "Hello " + string(fire[:2])
	delta2 := string(fire[2:]) + " world"

	evts := []types.StreamEvent{
		contentDelta(delta1),
		contentDelta(delta2),
	}

	var buf bytes.Buffer
	fullText, err := RenderStream(context.Background(), makeEvents(evts...), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantFull := "Hello 🔥 world"
	if fullText != wantFull {
		t.Errorf("fullText = %q, want %q", fullText, wantFull)
	}

	output := buf.String()
	if !strings.Contains(output, "🔥") {
		t.Errorf("output missing emoji, got: %q", output)
	}
	if !strings.Contains(output, "Hello") {
		t.Errorf("output missing 'Hello', got: %q", output)
	}
	if !strings.Contains(output, "world") {
		t.Errorf("output missing 'world', got: %q", output)
	}
}

func TestRenderStream_CJK(t *testing.T) {
	// 中 is U+4E2D, UTF-8: E4 B8 AD
	// 文 is U+6587, UTF-8: E6 96 87
	// Split 中 across deltas: first byte in delta 1, remaining in delta 2.
	zhong := []byte("中") // E4 B8 AD
	delta1 := "Hello " + string(zhong[:1])
	delta2 := string(zhong[1:]) + "文"

	evts := []types.StreamEvent{
		contentDelta(delta1),
		contentDelta(delta2),
	}

	var buf bytes.Buffer
	fullText, err := RenderStream(context.Background(), makeEvents(evts...), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantFull := "Hello 中文"
	if fullText != wantFull {
		t.Errorf("fullText = %q, want %q", fullText, wantFull)
	}

	output := buf.String()
	if !strings.Contains(output, "中文") {
		t.Errorf("output missing CJK characters, got: %q", output)
	}
}

func TestRenderStream_MixedUTF8AndMarkdown(t *testing.T) {
	evts := []types.StreamEvent{
		contentDelta("**bold** 🎉 `code`"),
	}

	var buf bytes.Buffer
	fullText, err := RenderStream(context.Background(), makeEvents(evts...), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantFull := "bold 🎉 code"
	if fullText != wantFull {
		t.Errorf("fullText = %q, want %q", fullText, wantFull)
	}

	output := buf.String()
	if !strings.Contains(output, Bold) {
		t.Errorf("output missing Bold ANSI code, got: %q", output)
	}
	if !strings.Contains(output, Cyan) {
		t.Errorf("output missing Cyan ANSI code, got: %q", output)
	}
	if !strings.Contains(output, "🎉") {
		t.Errorf("output missing emoji, got: %q", output)
	}
	if !strings.Contains(output, "bold") {
		t.Errorf("output missing 'bold', got: %q", output)
	}
	if !strings.Contains(output, "code") {
		t.Errorf("output missing 'code', got: %q", output)
	}
}
