package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// accountLabel returns a human-readable label for an SKR account in the form
// "<Number> — <Name>". If Name is empty, only the number is returned.
func accountLabel(acc core.SKRAccount) string {
	if acc.Name == "" {
		return fmt.Sprintf("%d", acc.Number)
	}
	return fmt.Sprintf("%d — %s", acc.Number, acc.Name)
}

// pickerRow is a row in the account picker list. When header != "", the row
// renders as a non-selectable section header; otherwise account holds the data.
type pickerRow struct {
	account core.SKRAccount
	header  string
}

// buildEmptyResults constructs the picker rows for an empty search query:
// Recent section, Favorites section (minus items already in Recent), then
// All Accounts section (minus items already shown). Each section starts with a
// header row. Sections with no entries are omitted entirely.
func (a *App) buildEmptyResults() []pickerRow {
	var rows []pickerRow

	seen := map[int]bool{}

	// --- Zuletzt benutzt ---
	if a.accountPrefs != nil {
		recent := a.accountPrefs.RecentList()
		var recentAccs []core.SKRAccount
		for _, n := range recent {
			if acc, ok := a.chart.Find(n); ok {
				recentAccs = append(recentAccs, acc)
			}
		}
		if len(recentAccs) > 0 {
			rows = append(rows, pickerRow{header: a.bundle.T("picker.recent")})
			for _, acc := range recentAccs {
				rows = append(rows, pickerRow{account: acc})
				seen[acc.Number] = true
			}
		}

		// --- Favoriten ---
		favs := a.accountPrefs.FavoriteList()
		var favAccs []core.SKRAccount
		for _, n := range favs {
			if !seen[n] {
				if acc, ok := a.chart.Find(n); ok {
					favAccs = append(favAccs, acc)
				}
			}
		}
		if len(favAccs) > 0 {
			rows = append(rows, pickerRow{header: a.bundle.T("picker.favorites")})
			for _, acc := range favAccs {
				rows = append(rows, pickerRow{account: acc})
				seen[acc.Number] = true
			}
		}
	}

	// --- Alle Konten ---
	all := a.chart.All()
	var restAccs []core.SKRAccount
	for _, acc := range all {
		if !seen[acc.Number] {
			restAccs = append(restAccs, acc)
		}
	}
	if len(restAccs) > 0 {
		rows = append(rows, pickerRow{header: a.bundle.T("picker.all")})
		for _, acc := range restAccs {
			rows = append(rows, pickerRow{account: acc})
		}
	}

	return rows
}

// buildSearchResults wraps a flat chart.Search result as pickerRows (no headers).
func buildSearchResults(accs []core.SKRAccount) []pickerRow {
	rows := make([]pickerRow, len(accs))
	for i, acc := range accs {
		rows[i] = pickerRow{account: acc}
	}
	return rows
}

// showAccountSearch opens a modal search dialog over the chart of accounts.
// current is the account number that is pre-selected (informational only).
// onPick is called with the chosen account number and the dialog is closed.
func (a *App) showAccountSearch(current int, onPick func(number int)) {
	if a.chart == nil {
		return
	}

	// Working slice shown in the list, rebuilt on every query change.
	results := a.buildEmptyResults()

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder(a.bundle.T("picker.account.search"))

	var list *widget.List
	var dlg dialog.Dialog

	// rebuildList is called from the favorite toggle button to refresh in place.
	var rebuildList func()

	list = widget.NewList(
		func() int { return len(results) },
		// Template: an HBox with a label on the left and a star button on the right.
		// We reuse the same template for both header rows and account rows; the
		// update func configures visibility appropriately.
		func() fyne.CanvasObject {
			lbl := widget.NewLabel("template")
			lbl.TextStyle = fyne.TextStyle{}
			starBtn := widget.NewButtonWithIcon("", theme.AccountIcon(), func() {})
			row := container.NewBorder(nil, nil, nil, starBtn, lbl)
			return row
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id >= len(results) {
				return
			}
			row := results[id]
			c := item.(*fyne.Container)
			lbl := c.Objects[0].(*widget.Label)
			starBtn := c.Objects[1].(*widget.Button)

			if row.header != "" {
				// Header row: bold label, no star button.
				lbl.SetText(row.header)
				lbl.TextStyle = fyne.TextStyle{Bold: true}
				lbl.Refresh()
				starBtn.Hide()
				return
			}

			// Account row.
			lbl.SetText(accountLabel(row.account))
			lbl.TextStyle = fyne.TextStyle{}
			lbl.Refresh()

			// Determine star state.
			isFav := a.accountPrefs != nil && a.accountPrefs.IsFavorite(row.account.Number)
			if isFav {
				starBtn.SetIcon(theme.RadioButtonCheckedIcon())
				starBtn.SetText("★")
			} else {
				starBtn.SetIcon(theme.RadioButtonIcon())
				starBtn.SetText("☆")
			}
			// Capture account number for the closure.
			accNum := row.account.Number
			starBtn.OnTapped = func() {
				if a.accountPrefs == nil {
					return
				}
				a.accountPrefs.ToggleFavorite(accNum)
				if err := a.accountPrefs.Save(); err != nil && a.logger != nil {
					a.logger.Warn("Failed to save account prefs: %v", err)
				}
				rebuildList()
			}
			starBtn.Show()
		},
	)

	rebuildList = func() {
		q := strings.TrimSpace(searchEntry.Text)
		if q == "" {
			results = a.buildEmptyResults()
		} else {
			results = buildSearchResults(a.chart.Search(q))
		}
		list.UnselectAll()
		list.Refresh()
	}

	list.OnSelected = func(id widget.ListItemID) {
		if id >= len(results) {
			return
		}
		row := results[id]
		if row.header != "" {
			// Non-selectable header — deselect immediately.
			list.UnselectAll()
			return
		}
		chosen := row.account
		// Note: "recently used" is recorded by the caller (the invoice
		// Gegenkonto picks), NOT here — so picking a structural account in the
		// Settings dialog doesn't pollute the recent list.
		dlg.Hide()
		onPick(chosen.Number)
	}

	searchEntry.OnChanged = func(q string) {
		q = strings.TrimSpace(q)
		if q == "" {
			results = a.buildEmptyResults()
		} else {
			results = buildSearchResults(a.chart.Search(q))
		}
		list.UnselectAll()
		list.Refresh()
	}

	content := container.NewBorder(
		searchEntry,
		nil, nil, nil,
		list,
	)

	dlg = dialog.NewCustom(
		a.bundle.T("picker.account.title"),
		a.bundle.T("common.close"),
		container.NewStack(
			container.New(fixedHeightLayout{height: 400}, content),
		),
		a.window,
	)
	dlg.Resize(fyne.NewSize(480, 460))
	dlg.Show()

	a.window.Canvas().Focus(searchEntry)
}

// fixedHeightLayout is a minimal Fyne layout that gives its single child a
// fixed height so the dialog content does not collapse to zero.
type fixedHeightLayout struct{ height float32 }

func (l fixedHeightLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.NewSize(0, l.height)
	}
	min := objects[0].MinSize()
	if min.Height < l.height {
		min.Height = l.height
	}
	return min
}

func (l fixedHeightLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}
}
