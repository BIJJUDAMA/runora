package mouse

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRectContains(t *testing.T) {
	r := Rect{X: 10, Y: 5, W: 20, H: 4}

	// Inside points
	if !r.Contains(10, 5) {
		t.Errorf("expected (10, 5) to be inside rect (top-left corner)")
	}
	if !r.Contains(29, 8) {
		t.Errorf("expected (29, 8) to be inside rect (bottom-right corner)")
	}
	if !r.Contains(15, 6) {
		t.Errorf("expected (15, 6) to be inside rect (interior)")
	}

	// Outside points
	if r.Contains(9, 5) {
		t.Errorf("expected (9, 5) to be outside rect (left)")
	}
	if r.Contains(30, 5) {
		t.Errorf("expected (30, 5) to be outside rect (right)")
	}
	if r.Contains(10, 4) {
		t.Errorf("expected (10, 4) to be outside rect (top)")
	}
	if r.Contains(10, 9) {
		t.Errorf("expected (10, 9) to be outside rect (bottom)")
	}
}

func TestRegistryClickAndZIndexPriority(t *testing.T) {
	reg := NewRegistry()

	var baseClicked bool
	var modalClicked bool

	reg.Register(Region{
		ID:     "base-card",
		Bounds: Rect{X: 0, Y: 0, W: 50, H: 20},
		ZIndex: 0,
		OnClick: func(msg tea.MouseMsg) tea.Cmd {
			baseClicked = true
			return nil
		},
	})

	reg.Register(Region{
		ID:     "modal-card",
		Bounds: Rect{X: 10, Y: 5, W: 30, H: 10},
		ZIndex: 10,
		OnClick: func(msg tea.MouseMsg) tea.Cmd {
			modalClicked = true
			return nil
		},
	})

	// Click in overlapping area (15, 7) should hit modal (ZIndex 10), not base (ZIndex 0)
	reg.Dispatch(tea.MouseMsg{X: 15, Y: 7, Type: tea.MouseLeft})
	if !modalClicked {
		t.Errorf("expected modal region to be clicked due to higher ZIndex")
	}
	if baseClicked {
		t.Errorf("expected base region NOT to be clicked when occluded by modal")
	}

	// Reset flags
	baseClicked = false
	modalClicked = false

	// Click outside modal but inside base (5, 2)
	reg.Dispatch(tea.MouseMsg{X: 5, Y: 2, Type: tea.MouseLeft})
	if !baseClicked {
		t.Errorf("expected base region to be clicked when clicking outside modal bounds")
	}
	if modalClicked {
		t.Errorf("expected modal region NOT to be clicked")
	}
}

func TestRegistryClickAwayDismissal(t *testing.T) {
	reg := NewRegistry()

	var dismissed bool
	var innerClicked bool

	modalBounds := Rect{X: 10, Y: 5, W: 20, H: 10}
	reg.SetModal(modalBounds, func() tea.Cmd {
		dismissed = true
		return nil
	})

	reg.Register(Region{
		ID:     "modal-button",
		Bounds: Rect{X: 12, Y: 7, W: 10, H: 2},
		ZIndex: 10,
		OnClick: func(msg tea.MouseMsg) tea.Cmd {
			innerClicked = true
			return nil
		},
	})

	// 1. Click inside modal on button
	reg.Dispatch(tea.MouseMsg{X: 14, Y: 8, Type: tea.MouseLeft})
	if !innerClicked {
		t.Errorf("expected button inside modal to be clicked")
	}
	if dismissed {
		t.Errorf("expected modal NOT to be dismissed when clicking inside")
	}

	// Reset flags
	innerClicked = false
	dismissed = false

	// 2. Click outside modal bounds (e.g. at (2, 2))
	reg.Dispatch(tea.MouseMsg{X: 2, Y: 2, Type: tea.MouseLeft})
	if !dismissed {
		t.Errorf("expected modal to be dismissed when clicking outside its bounds")
	}
	if innerClicked {
		t.Errorf("expected inner button NOT to be clicked")
	}
}

func TestRegistryDoubleClick(t *testing.T) {
	reg := NewRegistry()

	var clickCount int
	var dblClickCount int

	reg.Register(Region{
		ID:     "profile-item",
		Bounds: Rect{X: 0, Y: 0, W: 20, H: 5},
		ZIndex: 0,
		OnClick: func(msg tea.MouseMsg) tea.Cmd {
			clickCount++
			return nil
		},
		OnDblClick: func(msg tea.MouseMsg) tea.Cmd {
			dblClickCount++
			return nil
		},
	})

	// First click
	reg.Dispatch(tea.MouseMsg{X: 5, Y: 2, Type: tea.MouseLeft})
	if clickCount != 1 || dblClickCount != 0 {
		t.Errorf("expected 1 click and 0 dblclicks, got clicks=%d, dblclicks=%d", clickCount, dblClickCount)
	}

	// Immediate second click (within 450ms)
	reg.Dispatch(tea.MouseMsg{X: 5, Y: 2, Type: tea.MouseLeft})
	if clickCount != 1 || dblClickCount != 1 {
		t.Errorf("expected dblclick to trigger on rapid second click, got clicks=%d, dblclicks=%d", clickCount, dblClickCount)
	}

	// Third click after delay
	time.Sleep(500 * time.Millisecond)
	reg.Dispatch(tea.MouseMsg{X: 5, Y: 2, Type: tea.MouseLeft})
	if clickCount != 2 || dblClickCount != 1 {
		t.Errorf("expected regular click after delay, got clicks=%d, dblclicks=%d", clickCount, dblClickCount)
	}
}

func TestRegistryScrollEventRouting(t *testing.T) {
	reg := NewRegistry()

	var scrollUpCount int
	var scrollDownCount int

	reg.Register(Region{
		ID:     "log-viewport",
		Bounds: Rect{X: 0, Y: 0, W: 40, H: 20},
		ZIndex: 0,
		OnScroll: func(up bool) tea.Cmd {
			if up {
				scrollUpCount++
			} else {
				scrollDownCount++
			}
			return nil
		},
	})

	// Scroll Up inside region
	reg.Dispatch(tea.MouseMsg{X: 10, Y: 10, Type: tea.MouseWheelUp})
	if scrollUpCount != 1 {
		t.Errorf("expected scrollUpCount=1, got %d", scrollUpCount)
	}

	// Scroll Down inside region
	reg.Dispatch(tea.MouseMsg{X: 10, Y: 10, Type: tea.MouseWheelDown})
	if scrollDownCount != 1 {
		t.Errorf("expected scrollDownCount=1, got %d", scrollDownCount)
	}

	// Scroll outside region
	reg.Dispatch(tea.MouseMsg{X: 50, Y: 50, Type: tea.MouseWheelUp})
	if scrollUpCount != 1 {
		t.Errorf("expected scrollUpCount to remain 1 after out-of-bounds scroll, got %d", scrollUpCount)
	}
}

func TestRegistryClear(t *testing.T) {
	reg := NewRegistry()

	reg.Register(Region{ID: "btn-1", Bounds: Rect{X: 0, Y: 0, W: 10, H: 2}})
	reg.SetModal(Rect{X: 5, Y: 5, W: 10, H: 10}, nil)

	if reg.RegionCount() != 1 {
		t.Errorf("expected 1 region before clear, got %d", reg.RegionCount())
	}
	if !reg.HasModal() {
		t.Errorf("expected modal to be active before clear")
	}

	reg.Clear()

	if reg.RegionCount() != 0 {
		t.Errorf("expected 0 regions after clear, got %d", reg.RegionCount())
	}
	if reg.HasModal() {
		t.Errorf("expected modal to be nil after clear")
	}
}
