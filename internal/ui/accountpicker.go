package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
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

// showAccountSearch opens a modal search dialog over the chart of accounts.
// current is the account number that is pre-selected (informational only).
// onPick is called with the chosen account number and the dialog is closed.
func (a *App) showAccountSearch(current int, onPick func(number int)) {
	if a.chart == nil {
		return
	}

	// Working slice shown in the list, rebuilt on every query change.
	results := a.chart.All()

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder(a.bundle.T("picker.account.search"))

	var list *widget.List
	var dlg dialog.Dialog

	list = widget.NewList(
		func() int { return len(results) },
		func() fyne.CanvasObject {
			return widget.NewLabel("template label")
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id >= len(results) {
				return
			}
			lbl := item.(*widget.Label)
			lbl.SetText(accountLabel(results[id]))
		},
	)

	list.OnSelected = func(id widget.ListItemID) {
		if id >= len(results) {
			return
		}
		chosen := results[id]
		dlg.Hide()
		onPick(chosen.Number)
	}

	searchEntry.OnChanged = func(q string) {
		q = strings.TrimSpace(q)
		if q == "" {
			results = a.chart.All()
		} else {
			results = a.chart.Search(q)
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
