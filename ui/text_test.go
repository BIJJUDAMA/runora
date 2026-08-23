package ui

import (
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

func TestTruncateVisual_ASCII(t *testing.T) {
	tests := []struct {
		input    string
		maxWidth int
		ellipsis string
		expected string
	}{
		{"hello", 10, "...", "hello"},
		{"hello world", 8, "...", "hello..."},
		{"hello world", 5, "...", "he..."},
		{"hello world", 3, "...", "..."},
		{"hello world", 2, "...", ".."},
		{"hello world", 0, "...", ""},
		{"hello world", -5, "...", ""},
		{"short", 5, "...", "short"},
	}

	for _, tt := range tests {
		res := TruncateVisual(tt.input, tt.maxWidth, tt.ellipsis)
		if res != tt.expected {
			t.Errorf("TruncateVisual(%q, %d, %q) = %q, expected %q", tt.input, tt.maxWidth, tt.ellipsis, res, tt.expected)
		}
		if tt.maxWidth > 0 && VisualWidth(res) > tt.maxWidth {
			t.Errorf("TruncateVisual(%q, %d, %q) result width %d exceeds maxWidth %d", tt.input, tt.maxWidth, tt.ellipsis, VisualWidth(res), tt.maxWidth)
		}
	}
}

func TestTruncateVisual_MultiByteRuneSafety(t *testing.T) {
	cjk := "你好世界，人工智能" // Each CJK char has visual width 2
	for w := 1; w <= 20; w++ {
		truncated := TruncateVisual(cjk, w, "...")
		if !utf8.ValidString(truncated) {
			t.Fatalf("TruncateVisual created invalid UTF-8 string at width %d: %q", w, truncated)
		}
		width := VisualWidth(truncated)
		if width > w {
			t.Errorf("VisualWidth(%q) = %d > maxWidth %d", truncated, width, w)
		}
	}

	emoji := "🚀🚀🚀 Llama-3-8B-Instruct-Q4_K_M.gguf 🔥"
	for w := 1; w <= 40; w++ {
		truncated := TruncateVisual(emoji, w, "...")
		if !utf8.ValidString(truncated) {
			t.Fatalf("TruncateVisual created invalid UTF-8 at width %d: %q", w, truncated)
		}
		if VisualWidth(truncated) > w {
			t.Errorf("VisualWidth(%q) = %d > maxWidth %d", truncated, VisualWidth(truncated), w)
		}
	}
}

func TestTruncateVisual_ANSIEscapes(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("Styled Model Name 12345")
	for w := 1; w <= 30; w++ {
		truncated := TruncateVisual(styled, w, "...")
		if !utf8.ValidString(truncated) {
			t.Fatalf("TruncateVisual created invalid UTF-8 with ANSI at width %d", w)
		}
		vw := VisualWidth(truncated)
		if vw > w {
			t.Errorf("VisualWidth(%q) = %d > maxWidth %d", truncated, vw, w)
		}
	}
}

func TestPadOrTruncate(t *testing.T) {
	padded := PadOrTruncate("hi", 6, "...")
	if padded != "hi    " {
		t.Errorf("expected %q, got %q", "hi    ", padded)
	}
	if VisualWidth(padded) != 6 {
		t.Errorf("expected visual width 6, got %d", VisualWidth(padded))
	}

	truncated := PadOrTruncate("hello world", 8, "...")
	if truncated != "hello..." {
		t.Errorf("expected %q, got %q", "hello...", truncated)
	}
	if VisualWidth(truncated) != 8 {
		t.Errorf("expected visual width 8, got %d", VisualWidth(truncated))
	}
}
