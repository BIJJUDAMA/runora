package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// TruncateVisual truncates a string to maxWidth visual display cells without slicing
// multi-byte runes or tearing ANSI escape sequences, appending ellipsis if truncated.
func TruncateVisual(text string, maxWidth int, ellipsis string) string {
	if maxWidth <= 0 {
		return ""
	}

	w := ansi.StringWidth(text)
	if w <= maxWidth {
		return text
	}

	ellipsisWidth := ansi.StringWidth(ellipsis)
	if maxWidth <= ellipsisWidth {
		return ansi.Truncate(ellipsis, maxWidth, "")
	}

	return ansi.Truncate(text, maxWidth, ellipsis)
}

// PadOrTruncate pads text with spaces to reach target visual width, or truncates
// with ellipsis if it exceeds the visual width.
func PadOrTruncate(text string, width int, ellipsis string) string {
	if width <= 0 {
		return ""
	}
	w := ansi.StringWidth(text)
	if w > width {
		return TruncateVisual(text, width, ellipsis)
	}
	if w < width {
		return text + strings.Repeat(" ", width-w)
	}
	return text
}

// VisualWidth returns the visible terminal cell width of the given string,
// accounting for multi-byte runes and ignoring ANSI escape sequences.
func VisualWidth(text string) int {
	return ansi.StringWidth(text)
}

// RuneWidth returns the display cell width of a single rune.
func RuneWidth(r rune) int {
	return runewidth.RuneWidth(r)
}
