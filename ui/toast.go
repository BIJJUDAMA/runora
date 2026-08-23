package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// ToastType defines the semantic style and icon for floating toast notifications.
type ToastType int

const (
	ToastInfo ToastType = iota
	ToastSuccess
	ToastWarning
	ToastDanger
)

// Toast represents an individual notification item.
type Toast struct {
	ID        int
	Message   string
	Type      ToastType
	CreatedAt time.Time
	ExpiresAt time.Time
}

// ToastExpireMsg is a tea.Msg sent when a toast's duration expires.
type ToastExpireMsg struct {
	ID int
}

// ToastManager manages active floating toast notifications and their lifecycles.
type ToastManager struct {
	toasts []Toast
	nextID int
}

// NewToastManager initializes an empty ToastManager.
func NewToastManager() *ToastManager {
	return &ToastManager{
		toasts: make([]Toast, 0),
	}
}

// Add queues a new toast with the specified type and duration.
func (m *ToastManager) Add(message string, toastType ToastType, duration time.Duration) tea.Cmd {
	m.nextID++
	id := m.nextID
	now := time.Now()
	if duration <= 0 {
		duration = 2 * time.Second
	}

	toast := Toast{
		ID:        id,
		Message:   message,
		Type:      toastType,
		CreatedAt: now,
		ExpiresAt: now.Add(duration),
	}
	m.toasts = append(m.toasts, toast)

	return tea.Tick(duration, func(t time.Time) tea.Msg {
		return ToastExpireMsg{ID: id}
	})
}

// Show queues a default informational toast (2 seconds).
func (m *ToastManager) Show(message string) tea.Cmd {
	return m.Add(message, ToastInfo, 2*time.Second)
}

// ShowSuccess queues a success toast (2 seconds).
func (m *ToastManager) ShowSuccess(message string) tea.Cmd {
	return m.Add(message, ToastSuccess, 2*time.Second)
}

// ShowWarning queues a warning toast (2 seconds).
func (m *ToastManager) ShowWarning(message string) tea.Cmd {
	return m.Add(message, ToastWarning, 2*time.Second)
}

// ShowDanger queues an error/danger toast (2 seconds).
func (m *ToastManager) ShowDanger(message string) tea.Cmd {
	return m.Add(message, ToastDanger, 2*time.Second)
}

// Update handles expiration messages and timer updates.
func (m *ToastManager) Update(msg tea.Msg) {
	switch msg := msg.(type) {
	case ToastExpireMsg:
		m.Remove(msg.ID)
	}
	m.PruneExpired()
}

// Remove removes a toast by ID.
func (m *ToastManager) Remove(id int) {
	filtered := m.toasts[:0]
	for _, t := range m.toasts {
		if t.ID != id {
			filtered = append(filtered, t)
		}
	}
	m.toasts = filtered
}

// PruneExpired removes all toasts whose expiration timestamp has elapsed.
func (m *ToastManager) PruneExpired() {
	now := time.Now()
	filtered := m.toasts[:0]
	for _, t := range m.toasts {
		if now.Before(t.ExpiresAt) {
			filtered = append(filtered, t)
		}
	}
	m.toasts = filtered
}

// Active returns true if there is at least one active toast.
func (m *ToastManager) Active() bool {
	m.PruneExpired()
	return len(m.toasts) > 0
}

// Count returns the number of active toasts.
func (m *ToastManager) Count() int {
	m.PruneExpired()
	return len(m.toasts)
}

// Clear removes all active toasts immediately.
func (m *ToastManager) Clear() {
	m.toasts = m.toasts[:0]
}

// GetToasts returns a slice of currently active toasts.
func (m *ToastManager) GetToasts() []Toast {
	m.PruneExpired()
	return m.toasts
}

// RenderToasts builds the visual representation of active toasts.
func (m *ToastManager) RenderToasts() string {
	m.PruneExpired()
	if len(m.toasts) == 0 {
		return ""
	}

	var rendered []string
	// Limit to rendering at most 3 simultaneous toasts to prevent screen clutter
	start := 0
	if len(m.toasts) > 3 {
		start = len(m.toasts) - 3
	}

	for _, t := range m.toasts[start:] {
		var icon string
		var borderCol lipgloss.TerminalColor
		var iconCol lipgloss.TerminalColor

		switch t.Type {
		case ToastSuccess:
			icon = "✓"
			borderCol = ColorSecondary
			iconCol = ColorSecondary
		case ToastWarning:
			icon = "⚠"
			borderCol = ColorWarning
			iconCol = ColorWarning
		case ToastDanger:
			icon = "✗"
			borderCol = ColorDanger
			iconCol = ColorDanger
		default:
			icon = "ℹ"
			borderCol = ColorPrimary
			iconCol = ColorPrimary
		}

		iconStyled := lipgloss.NewStyle().Foreground(iconCol).Bold(true).Render(icon)
		textStyled := lipgloss.NewStyle().Foreground(ColorText).Bold(true).Render(t.Message)

		toastBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderCol).
			Background(ColorDim).
			Padding(0, 1).
			Render(fmt.Sprintf("%s  %s", iconStyled, textStyled))

		rendered = append(rendered, toastBox)
	}

	return lipgloss.JoinVertical(lipgloss.Right, rendered...)
}

// Overlay renders active toasts floating over the top-right corner of baseView.
func (m *ToastManager) Overlay(baseView string, screenWidth, screenHeight int) string {
	if !m.Active() {
		return baseView
	}

	toastBlock := m.RenderToasts()
	if toastBlock == "" {
		return baseView
	}

	return OverlayTopRight(baseView, toastBlock, screenWidth, 1, 2)
}

// OverlayTopRight composites an overlay block over a base view at the top-right.
func OverlayTopRight(base, overlay string, totalWidth, topOffset, rightMargin int) string {
	if overlay == "" {
		return base
	}

	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	// Calculate overlay width
	overlayWidth := 0
	for _, ol := range overlayLines {
		w := ansi.StringWidth(ol)
		if w > overlayWidth {
			overlayWidth = w
		}
	}

	if totalWidth <= 0 {
		for _, bl := range baseLines {
			w := ansi.StringWidth(bl)
			if w > totalWidth {
				totalWidth = w
			}
		}
	}
	if totalWidth < overlayWidth+rightMargin {
		totalWidth = overlayWidth + rightMargin
	}

	targetX := totalWidth - overlayWidth - rightMargin
	if targetX < 0 {
		targetX = 0
	}

	for i, oLine := range overlayLines {
		row := topOffset + i
		if row < 0 {
			continue
		}
		for len(baseLines) <= row {
			baseLines = append(baseLines, "")
		}

		bLine := baseLines[row]
		oWidth := ansi.StringWidth(oLine)

		// Truncate baseLine up to targetX
		leftPart := ansi.Truncate(bLine, targetX, "")
		leftPartWidth := ansi.StringWidth(leftPart)
		if leftPartWidth < targetX {
			leftPart += strings.Repeat(" ", targetX-leftPartWidth)
		}

		// Remainder of base line beyond overlay
		var rightPart string
		bWidth := ansi.StringWidth(bLine)
		if bWidth > targetX+oWidth {
			rightPart = ansiCutLeft(bLine, targetX+oWidth)
		}

		baseLines[row] = leftPart + oLine + rightPart
	}

	return strings.Join(baseLines, "\n")
}

// ansiCutLeft removes the first `cutCells` visual cells from an ANSI string.
func ansiCutLeft(s string, cutCells int) string {
	if cutCells <= 0 {
		return s
	}
	w := ansi.StringWidth(s)
	if w <= cutCells {
		return ""
	}
	return ansi.Cut(s, cutCells, w)
}
