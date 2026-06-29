package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

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

	col := container.NewVBox()
	for _, g := range groups {
		col.Add(widget.NewLabelWithStyle(a.bundle.T(g.titleKey),
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		for _, it := range g.items {
			item := it // capture
			btn := widget.NewButton(a.bundle.T(item.key), item.action)
			btn.Alignment = widget.ButtonAlignLeading
			btn.Importance = widget.LowImportance
			col.Add(btn)
		}
		col.Add(widget.NewSeparator())
	}

	// Fixed-width sidebar. No scroll container: NewVScroll forces its own
	// MinSize.Width to the widest child (Fyne ScrollVerticalOnly), which then
	// overflows the fixed width and shows a horizontal scrollbar — unprofessional.
	// The ~15 entries fit in the window height; fixedWidthLayout resizes the
	// VBox to the column width so leading-aligned buttons fill it cleanly.
	return container.New(fixedWidthLayout{width: 210}, col)
}
