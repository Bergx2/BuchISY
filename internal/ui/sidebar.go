package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// sidebarHeaderBG is the light-blue band behind each workflow-group title;
// sidebarHeaderText is the (black) title colour on top of it.
var (
	sidebarHeaderBG   = color.NRGBA{R: 210, G: 224, B: 245, A: 255}
	sidebarHeaderText = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	// sidebarActiveBG is the soft-amber band behind the sidebar entry for the
	// screen the user is currently on ("you are here").
	sidebarActiveBG = color.NRGBA{R: 255, G: 224, B: 130, A: 255}
)

// sidebarGroupHeader renders a workflow-group title as a bold, small,
// upper-case section header on a light-blue band with black text — visually
// distinct from the clickable entries below it. A thin transparent spacer
// above sets the group apart from the preceding group's entries.
func sidebarGroupHeader(text string) fyne.CanvasObject {
	t := canvas.NewText(strings.ToUpper(text), sidebarHeaderText)
	t.TextStyle = fyne.TextStyle{Bold: true}
	t.TextSize = theme.TextSize() // same size as the entries below (not caption-small)
	bg := canvas.NewRectangle(sidebarHeaderBG)
	bg.CornerRadius = 4
	// Pad the header text so it lines up with the buttons' inset labels.
	header := container.NewStack(bg, container.NewPadded(t))
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(0, theme.Padding()*2))
	return container.NewVBox(spacer, header)
}

// vGapLayout stacks children top-to-bottom with a small fixed gap between them
// (instead of theme.Padding() like VBox), so sidebar entries sit closer.
type vGapLayout struct{ gap float32 }

func (l *vGapLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var w, h float32
	for i, o := range objs {
		m := o.MinSize()
		if m.Width > w {
			w = m.Width
		}
		if i > 0 {
			h += l.gap
		}
		h += m.Height
	}
	return fyne.NewSize(w, h)
}

func (l *vGapLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for _, o := range objs {
		mh := o.MinSize().Height
		o.Resize(fyne.NewSize(size.Width, mh))
		o.Move(fyne.NewPos(0, y))
		y += mh + l.gap
	}
}

// navItem is one entry in the workflow sidebar: a translation key plus the
// action to run when the entry is tapped.
type navItem struct {
	key    string
	action func()
}

// lockToggleNavItem returns the Festschreibung entry for the ABSCHLUSS group,
// showing "Zeitraum entsperren" when the current month is already locked, else
// "Zeitraum sperren". Both lock and unlock stay reachable from the sidebar.
func (a *App) lockToggleNavItem() navItem {
	if a.currentMonthLocked {
		return navItem{"nav.unlock", a.unlockCurrentMonth}
	}
	return navItem{"nav.lock", a.lockCurrentMonth}
}

// activeNavKey returns the translation key of the sidebar entry that
// corresponds to the screen currently shown in the main content area, so
// buildSidebar can highlight it ("you are here"). Only the two persistent,
// buildUI-driven views are tracked (Belege and Konten); the other entries
// open dialogs on top of whichever of those two is active.
func (a *App) activeNavKey() string {
	switch a.viewMode {
	case "konten":
		return "nav.konten"
	case "kassenbuch":
		return "nav.kassenbuch"
	default:
		return "nav.belege"
	}
}

// buildSidebar returns the persistent workflow navigation column (fixed width).
// It groups every screen by workflow phase (ERFASSEN, BUCHEN, AUSWERTEN,
// FINANZAMT, ABSCHLUSS) and renders each group as a bold header followed by
// leading-aligned LowImportance buttons.
func (a *App) buildSidebar() fyne.CanvasObject {
	groups := []struct {
		titleKey string
		items    []navItem
	}{
		{"nav.group.erfassen", []navItem{
			{"nav.belege", a.switchToBelege},
			{"nav.kassenbuch", a.showCashBookView},
		}},
		{"nav.group.buchen", []navItem{
			{"nav.konten", a.openKontenPicker},
			{"nav.belegabgleich", a.showBelegabgleich},
			{"nav.erloesabgleich", a.showErloesAbgleich},
			{"nav.anlagen", a.showAnlagen},
		}},
		{"nav.group.auswerten", []navItem{
			{"nav.susa", a.showSuSa},
			{"nav.guv", a.showGuV},
			{"nav.opos", a.showOpenItems},
			{"nav.controlling", a.showControllingDialog},
			{"nav.yearoverview", a.showYearOverviewDialog},
		}},
		{"nav.group.finanzamt", []navItem{
			{"nav.ustva", a.showUStVADialog},
			{"nav.zm", a.showZMDialog},
		}},
		{"nav.group.abschluss", []navItem{
			a.lockToggleNavItem(), // lock OR unlock depending on a.currentMonthLocked
			{"nav.audit", a.showAuditLog},
			{"nav.verfahrensdoku", a.showVerfahrensdokuPDF},
			{"nav.gobdexport", a.showExportPackage},
		}},
	}

	// Tight vertical stack so the entries under a group sit close together;
	// each group header carries its own top spacer for group separation.
	col := container.New(&vGapLayout{gap: 1})
	active := a.activeNavKey()
	for _, g := range groups {
		col.Add(sidebarGroupHeader(a.bundle.T(g.titleKey)))
		for _, it := range g.items {
			item := it // capture
			btn := widget.NewButton(a.bundle.T(item.key), item.action)
			btn.Alignment = widget.ButtonAlignLeading
			btn.Importance = widget.LowImportance
			if item.key == active {
				// Highlight the current screen with an amber band behind the
				// (transparent LowImportance) button so it shows through.
				bg := canvas.NewRectangle(sidebarActiveBG)
				bg.CornerRadius = 4
				col.Add(container.NewStack(bg, btn))
			} else {
				col.Add(btn)
			}
		}
	}

	// Fixed-width sidebar that scrolls vertically: wrapping the column in a
	// vertical-only scroll drops the sidebar's min height (it no longer forces
	// the whole window to stay tall enough for all ~15 entries), so the window
	// can be made shorter and a scrollbar appears when the entries don't fit.
	// fixedWidthLayout pins the scroll to 210px; vertical-only scrolling lays
	// the content out at that width, so no horizontal scrollbar appears.
	return container.New(fixedWidthLayout{width: 210}, container.NewVScroll(col))
}
