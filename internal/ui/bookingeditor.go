package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// bookingEditRow is the edited value of one booking line.
type bookingEditRow struct {
	Konto  int
	Betrag float64
	Soll   bool
}

// bookingFromRows builds a manual Booking from editor rows. Rows without an
// account (Konto==0) are dropped as incomplete.
func bookingFromRows(rows []bookingEditRow) core.Booking {
	entries := make([]core.BookingEntry, 0, len(rows))
	for _, r := range rows {
		if r.Konto == 0 {
			continue
		}
		entries = append(entries, core.BookingEntry{Konto: r.Konto, Betrag: r.Betrag, Soll: r.Soll})
	}
	return core.Booking{Manuell: true, Entries: entries}
}

// parseDecimal reads a German/English decimal string ("12,71" or "12.71").
func parseDecimal(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// showBookingEditor opens a dialog to hand-edit a booking's Soll/Haben lines.
// On OK it calls onSave with a Booking{Manuell:true}.
func (a *App) showBookingEditor(current core.Booking, parent fyne.Window, onSave func(core.Booking)) {
	if parent == nil {
		parent = a.window
	}
	rows := make([]bookingEditRow, 0, len(current.Entries))
	for _, e := range current.Entries {
		rows = append(rows, bookingEditRow{Konto: e.Konto, Betrag: e.Betrag, Soll: e.Soll})
	}
	if len(rows) == 0 {
		rows = append(rows, bookingEditRow{Soll: true})
	}

	rowsBox := container.NewVBox()
	balanceLabel := widget.NewLabel("")
	var rebuild func()

	refreshBalance := func() {
		b := bookingFromRows(rows)
		amount := strings.Replace(fmt.Sprintf("%.2f", b.SollSum()-b.HabenSum()), ".", ",", 1)
		if b.Balanced() {
			balanceLabel.SetText(a.bundle.T("booking.balanced"))
		} else {
			balanceLabel.SetText(a.bundle.T("booking.editor.diff", amount))
		}
	}

	rebuild = func() {
		rowsBox.RemoveAll()
		for i := range rows {
			i := i
			r := &rows[i]
			kontoLabel := widget.NewLabel(a.bookingKontoLabel(r.Konto))
			pickBtn := widget.NewButton("…", func() {
				a.showAccountSearch(r.Konto, parent, func(n int) {
					r.Konto = n
					kontoLabel.SetText(a.bookingKontoLabel(n))
					refreshBalance()
				})
			})
			betrag := widget.NewEntry()
			betrag.SetText(strings.Replace(fmt.Sprintf("%.2f", r.Betrag), ".", ",", 1))
			betrag.OnChanged = func(s string) { r.Betrag = parseDecimal(s); refreshBalance() }
			sh := widget.NewSelect([]string{a.bundle.T("booking.soll"), a.bundle.T("booking.haben")}, func(sel string) {
				r.Soll = sel == a.bundle.T("booking.soll")
				refreshBalance()
			})
			if r.Soll {
				sh.SetSelected(a.bundle.T("booking.soll"))
			} else {
				sh.SetSelected(a.bundle.T("booking.haben"))
			}
			del := widget.NewButton("✕", func() {
				rows = append(rows[:i], rows[i+1:]...)
				rebuild()
				refreshBalance()
			})
			kontoCell := container.NewBorder(nil, nil, nil, pickBtn, kontoLabel)
			row := container.NewBorder(nil, nil, nil, del,
				container.New(layout.NewGridLayoutWithColumns(3), kontoCell, betrag, sh))
			rowsBox.Add(row)
		}
		rowsBox.Refresh()
	}

	addBtn := widget.NewButton(a.bundle.T("booking.editor.add"), func() {
		rows = append(rows, bookingEditRow{Soll: true})
		rebuild()
		refreshBalance()
	})

	rebuild()
	refreshBalance()

	content := container.NewBorder(nil, container.NewVBox(addBtn, balanceLabel), nil, nil,
		container.NewVScroll(rowsBox))
	d := dialog.NewCustomConfirm(a.bundle.T("booking.editor.title"), a.bundle.T("btn.save"), a.bundle.T("btn.cancel"),
		content, func(ok bool) {
			if ok {
				onSave(bookingFromRows(rows))
			}
		}, parent)
	d.Resize(fyne.NewSize(520, 460))
	d.Show()
}

// bookingKontoLabel renders an account for the editor (number — name, or a
// placeholder when unset).
func (a *App) bookingKontoLabel(konto int) string {
	if konto == 0 {
		return a.bundle.T("booking.editor.pickaccount")
	}
	if acc, ok := a.chart.Find(konto); ok {
		return accountLabel(acc)
	}
	return fmt.Sprintf("%d", konto)
}
