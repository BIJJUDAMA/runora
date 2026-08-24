package mouse

import (
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Rect represents an absolute terminal bounding box.
type Rect struct {
	X int // 0-indexed column
	Y int // 0-indexed row
	W int // Width in terminal cells
	H int // Height in terminal rows
}

// Contains checks if terminal coordinate (x, y) falls inside the rectangle.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// ActionFunc executes on a click event and optionally returns a tea.Cmd.
type ActionFunc func(msg tea.MouseMsg) tea.Cmd

// ScrollFunc executes on a mouse wheel event and optionally returns a tea.Cmd.
type ScrollFunc func(up bool) tea.Cmd

// Region defines an interactive area on screen.
type Region struct {
	ID         string
	Bounds     Rect
	ZIndex     int // 0 = base screen, 10 = modal/overlay
	OnClick    ActionFunc
	OnDblClick ActionFunc
	OnScroll   ScrollFunc
}

// ModalBounds defines the bounding box and dismissal callback of an active overlay modal.
type ModalBounds struct {
	Bounds    Rect
	OnDismiss func() tea.Cmd
}

// Registry stores all registered regions for the current frame.
type Registry struct {
	regions     []Region
	modal       *ModalBounds
	lastClickID string
	lastClickAt time.Time
}

// NewRegistry initializes an empty spatial registry.
func NewRegistry() *Registry {
	return &Registry{
		regions: make([]Region, 0, 64),
	}
}

// Clear resets the registry at the beginning of a render frame.
func (r *Registry) Clear() {
	r.regions = r.regions[:0]
	r.modal = nil
}

// Register adds a clickable or scrollable region.
func (r *Registry) Register(region Region) {
	r.regions = append(r.regions, region)
}

// SetModal registers an active modal bounding box with a click-away dismissal handler.
func (r *Registry) SetModal(bounds Rect, onDismiss func() tea.Cmd) {
	r.modal = &ModalBounds{
		Bounds:    bounds,
		OnDismiss: onDismiss,
	}
}

// HasModal returns whether a modal bounding box is currently active.
func (r *Registry) HasModal() bool {
	return r.modal != nil
}

// RegionCount returns the number of active registered regions.
func (r *Registry) RegionCount() int {
	return len(r.regions)
}

// Dispatch evaluates a mouse event against active regions and executes the matching callback.
func (r *Registry) Dispatch(msg tea.MouseMsg) tea.Cmd {
	// 1. Handle Scroll Wheel Events
	if msg.Type == tea.MouseWheelUp || msg.Type == tea.MouseWheelDown {
		isUp := msg.Type == tea.MouseWheelUp
		// If modal is active and cursor is outside modal, do not scroll underlying screen
		if r.modal != nil && !r.modal.Bounds.Contains(msg.X, msg.Y) {
			return nil
		}

		// Sort regions by ZIndex descending
		sorted := make([]Region, len(r.regions))
		copy(sorted, r.regions)
		sort.SliceStable(sorted, func(i, j int) bool {
			return sorted[i].ZIndex > sorted[j].ZIndex
		})

		for _, reg := range sorted {
			if reg.OnScroll != nil && reg.Bounds.Contains(msg.X, msg.Y) {
				return reg.OnScroll(isUp)
			}
		}
		return nil
	}

	// 2. Handle Left Click Events
	if msg.Type == tea.MouseLeft {
		// Check modal click-away
		if r.modal != nil {
			if !r.modal.Bounds.Contains(msg.X, msg.Y) {
				if r.modal.OnDismiss != nil {
					return r.modal.OnDismiss()
				}
				return nil
			}
		}

		// Sort regions by ZIndex descending to prioritize overlays
		sorted := make([]Region, len(r.regions))
		copy(sorted, r.regions)
		sort.SliceStable(sorted, func(i, j int) bool {
			return sorted[i].ZIndex > sorted[j].ZIndex
		})

		for _, reg := range sorted {
			if reg.Bounds.Contains(msg.X, msg.Y) {
				now := time.Now()
				isDblClick := reg.ID == r.lastClickID && now.Sub(r.lastClickAt) < 450*time.Millisecond
				r.lastClickID = reg.ID
				r.lastClickAt = now

				if isDblClick && reg.OnDblClick != nil {
					return reg.OnDblClick(msg)
				}
				if reg.OnClick != nil {
					return reg.OnClick(msg)
				}
				return nil
			}
		}
	}

	return nil
}
