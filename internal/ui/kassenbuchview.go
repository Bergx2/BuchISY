package ui

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// formatDecimal formats v with two decimals using the given separator.
func formatDecimal(v float64, sep string) string {
	return core.FormatAmount(v, sep)
}

// cashAccounts returns the names of all bank accounts of type cash.
func (a *App) cashAccounts() []string {
	var names []string
	for _, ba := range a.settings.BankAccounts {
		if ba.AccountType == core.AccountTypeCash {
			names = append(names, ba.Name)
		}
	}
	return names
}

// isCashAccount reports whether the named bank account has type cash.
func (a *App) isCashAccount(name string) bool {
	for _, ba := range a.settings.BankAccounts {
		if ba.Name == name && ba.AccountType == core.AccountTypeCash {
			return true
		}
	}
	return false
}

// cashInvoicesForMonth returns the invoices of the given month booked to
// the named cash account.
func (a *App) cashInvoicesForMonth(account string, year int, month time.Month) []core.CSVRow {
	csvPath := a.storageManager.GetCSVPath(year, month)
	rows, err := a.csvRepo.Load(csvPath)
	if err != nil {
		return nil
	}
	var out []core.CSVRow
	for _, r := range rows {
		if r.Bankkonto == account {
			out = append(out, r)
		}
	}
	return out
}

// cashInvoicesFor returns the current month's invoices booked to the named
// cash account.
func (a *App) cashInvoicesFor(account string) []core.CSVRow {
	return a.cashInvoicesForMonth(account, a.currentYear, a.currentMonth)
}

// cashCarryIn returns the opening balance carried into (year, month) for a
// cash account: it walks backwards to the most recent month that has a
// stored cash book (the anchor), then rolls the balance forward — counting
// each month's cash invoices — up to the month before (year, month). ok is
// false when no stored cash book exists in the lookback window.
func (a *App) cashCarryIn(account string, year int, month time.Month) (float64, bool) {
	const maxLookback = 60 // months
	type ym struct {
		y int
		m time.Month
	}
	// chain: the previous month first, walking back to the anchor (inclusive).
	var chain []ym
	var anchorBook core.CashBook
	found := false
	y, m := year, month
	for i := 0; i < maxLookback && !found; i++ {
		m--
		if m < time.January {
			m, y = time.December, y-1
		}
		chain = append(chain, ym{y, m})
		mb, _ := core.LoadCashBooks(
			filepath.Join(a.storageManager.GetMonthFolder(y, m), "kassenbuch.json"))
		for _, b := range mb {
			if b.Konto == account {
				anchorBook = b
				found = true
				break
			}
		}
	}
	if !found {
		return 0, false
	}
	// Roll forward: the anchor's closing balance, then each later month
	// (no stored book → empty book seeded with the running balance).
	anchor := chain[len(chain)-1]
	_, balance := core.ComputeCashReport(anchorBook, a.cashInvoicesForMonth(account, anchor.y, anchor.m))
	for i := len(chain) - 2; i >= 0; i-- {
		mo := chain[i]
		_, balance = core.ComputeCashReport(
			core.CashBook{Konto: account, Anfangsbestand: balance},
			a.cashInvoicesForMonth(account, mo.y, mo.m))
	}
	return balance, true
}

// showCashBookView switches the main view to the cash book (single-month
// mode), keeping the workflow sidebar. The period (Ganzes Jahr / a month /
// another year) is chosen via the header's cash period dropdown; see
// buildCashPeriodButton.
func (a *App) showCashBookView() {
	a.viewMode = "kassenbuch"
	a.cashWholeYear = false
	a.window.SetContent(a.buildUI())
}

// buildCashBookContent is the Kassenbuch body (no chrome — the shell supplies
// the sidebar and the period header). It renders the whole-year overview or a
// single month's cash book, per a.cashWholeYear.
func (a *App) buildCashBookContent() fyne.CanvasObject {
	accounts := a.cashAccounts()
	if len(accounts) == 0 {
		return container.NewPadded(newCopyableLabel(a.bundle,
			"Kein Konto vom Typ \"Barkasse\" vorhanden. Lege in den Einstellungen unter Konten ein Zahlungskonto mit Typ \"Barkasse\" an."))
	}
	if a.cashWholeYear {
		return a.buildCashYearBody(accounts)
	}
	return a.buildCashMonthBody(accounts)
}

// buildCashMonthBody renders one month's cash book for a selectable account.
func (a *App) buildCashMonthBody(accounts []string) fyne.CanvasObject {
	monthFolder := a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth)
	jsonPath := filepath.Join(monthFolder, "kassenbuch.json")
	monthLabel := fmt.Sprintf("%04d-%02d", a.currentYear, a.currentMonth)

	books, err := core.LoadCashBooks(jsonPath)
	if err != nil {
		a.logger.Warn("Failed to load cash book: %v", err)
		books = nil
	}

	// bookFor returns a pointer to the working CashBook for an account,
	// creating it in books if absent. A freshly created book pre-fills its
	// opening balance from the previous month's closing balance.
	bookFor := func(account string) *core.CashBook {
		for i := range books {
			if books[i].Konto == account {
				return &books[i]
			}
		}
		nb := core.CashBook{Konto: account}
		if end, ok := a.cashCarryIn(account, a.currentYear, a.currentMonth); ok {
			nb.Anfangsbestand = end
		}
		books = append(books, nb)
		return &books[len(books)-1]
	}

	// Account selector
	accountSelect := widget.NewSelect(accounts, nil)
	accountSelect.SetSelected(accounts[0])

	// Per-account editing area, rebuilt when the account changes.
	editArea := container.NewVBox()

	var rebuild func()
	rebuild = func() {
		account := accountSelect.Selected
		book := bookFor(account)
		editArea.Objects = editArea.Objects[:0]

		// Anfangsbestand is shown read-only (it is normally carried over from the
		// previous month) and only editable behind an explicit "Ändern" click, so
		// it can't be changed by accident. The value is persisted on "Speichern".
		startRow := container.NewHBox(
			widget.NewLabelWithStyle("Anfangsbestand:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabel("EUR "+formatDecimal(book.Anfangsbestand, a.settings.DecimalSeparator)),
			func() *widget.Button {
				b := widget.NewButton("Ändern", func() {
					entry := widget.NewEntry()
					entry.SetText(formatDecimal(book.Anfangsbestand, a.settings.DecimalSeparator))
					dialog.ShowForm("Anfangsbestand ändern", "Übernehmen", "Abbrechen",
						[]*widget.FormItem{widget.NewFormItem("Anfangsbestand (EUR)", entry)},
						func(ok bool) {
							if !ok {
								return
							}
							book.Anfangsbestand = parseFloat(entry.Text, a.settings.DecimalSeparator)
							rebuild()
						}, a.window)
				})
				b.Importance = widget.LowImportance
				return b
			}())

		depositList := container.NewVBox()
		var refreshDeposits func()
		refreshDeposits = func() {
			depositList.Objects = depositList.Objects[:0]
			for i := range book.Einlagen {
				idx := i
				dateE := widget.NewEntry()
				dateE.SetPlaceHolder("TT.MM.JJJJ")
				dateE.SetText(book.Einlagen[idx].Datum)
				dateE.OnChanged = func(s string) { book.Einlagen[idx].Datum = s }

				descE := widget.NewEntry()
				descE.SetPlaceHolder("Beschreibung")
				descE.SetText(book.Einlagen[idx].Beschreibung)
				descE.OnChanged = func(s string) { book.Einlagen[idx].Beschreibung = s }

				amtE := widget.NewEntry()
				amtE.SetPlaceHolder("Betrag")
				amtE.SetText(formatDecimal(book.Einlagen[idx].Betrag, a.settings.DecimalSeparator))
				amtE.OnChanged = func(s string) { book.Einlagen[idx].Betrag = parseFloat(s, a.settings.DecimalSeparator) }

				removeBtn := widget.NewButton("Entfernen", func() {
					book.Einlagen = append(book.Einlagen[:idx], book.Einlagen[idx+1:]...)
					refreshDeposits()
				})
				removeBtn.Importance = widget.LowImportance

				row := container.NewBorder(nil, nil, nil, removeBtn,
					container.NewGridWithColumns(3, dateE, descE, amtE))
				depositList.Add(row)
			}
			depositList.Refresh()
		}
		refreshDeposits()

		addDepositBtn := widget.NewButton("+ Einlage", func() {
			book.Einlagen = append(book.Einlagen, core.CashDeposit{})
			refreshDeposits()
		})

		// Cash invoices (clickable) + computed end balance.
		invoices := a.cashInvoicesFor(account)
		entries, endbestand := core.ComputeCashReport(*book, invoices)

		rowByName := make(map[string]core.CSVRow, len(invoices))
		for _, inv := range invoices {
			rowByName[inv.Dateiname] = inv
		}

		sep := a.settings.DecimalSeparator
		var sumEin, sumAus float64
		for _, e := range entries {
			sumEin += e.Einnahme
			sumAus += e.Ausgabe
		}

		// Bar-Ausgaben as aligned columns: Beleg-Nr. (clickable → receipt) |
		// Datum | Beschreibung | Ausgabe.
		outflowList := container.NewVBox()
		outflowList.Add(container.NewGridWithColumns(4,
			widget.NewLabelWithStyle("Beleg-Nr.", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Datum", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Beschreibung", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Ausgabe", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
		))
		nOut := 0
		for _, e := range entries {
			if e.Ausgabe == 0 {
				continue
			}
			nOut++
			row, ok := rowByName[e.Beleg]
			belegTxt := e.Belegnummer
			if belegTxt == "" {
				belegTxt = "—"
			}
			var belegCell fyne.CanvasObject
			if ok {
				captured := row
				b := widget.NewButton(belegTxt, func() { a.showEditDialog(captured, rebuild) })
				b.Importance = widget.LowImportance
				b.Alignment = widget.ButtonAlignLeading
				belegCell = b
			} else {
				belegCell = widget.NewLabel(belegTxt)
			}
			var rowUI fyne.CanvasObject = container.NewGridWithColumns(4,
				belegCell,
				widget.NewLabel(e.Datum),
				widget.NewLabel(e.Beschreibung),
				widget.NewLabelWithStyle(formatDecimal(e.Ausgabe, sep), fyne.TextAlignTrailing, fyne.TextStyle{}),
			)
			// A freshly saved cash receipt blinks once so it's easy to spot.
			if e.Beleg != "" && e.Beleg == a.cashFlash {
				bgRect := canvas.NewRectangle(color.NRGBA{R: 255, G: 224, B: 130, A: 200})
				rowUI = container.NewStack(bgRect, rowUI)
				time.AfterFunc(1100*time.Millisecond, func() {
					fyne.Do(func() { bgRect.FillColor = color.Transparent; bgRect.Refresh() })
				})
			}
			outflowList.Add(rowUI)
		}
		if nOut == 0 {
			outflowList.Add(newCopyableLabel(a.bundle, "  (keine Bar-Ausgaben in diesem Monat)"))
		}
		a.cashFlash = "" // one-shot

		editArea.Add(startRow)
		editArea.Add(widget.NewLabelWithStyle("Einlagen", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		editArea.Add(depositList)
		editArea.Add(container.NewHBox(addDepositBtn)) // left-aligned, natural width
		editArea.Add(widget.NewSeparator())
		editArea.Add(widget.NewLabelWithStyle("Bar-Ausgaben (aus Rechnungen)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		editArea.Add(outflowList)
		editArea.Add(widget.NewSeparator())
		// Month totals + closing balance (cash book is always EUR).
		editArea.Add(container.NewGridWithColumns(3,
			widget.NewLabelWithStyle("Summe Einnahmen: EUR "+formatDecimal(sumEin, sep), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Summe Ausgaben: EUR "+formatDecimal(sumAus, sep), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Endbestand: EUR "+formatDecimal(endbestand, sep), fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
		))
		editArea.Refresh()
	}
	accountSelect.OnChanged = func(string) { rebuild() }
	rebuild()

	saveBtn := widget.NewButton(a.bundle.T("btn.save"), func() {
		if err := core.SaveCashBooks(jsonPath, books); err != nil {
			dialog.ShowInformation(a.bundle.T("error.processing.title"), err.Error(), a.window)
			return
		}
		rebuild() // refresh computed balance
	})
	saveBtn.Importance = widget.HighImportance

	pdfBtn := widget.NewButton("Kassenbericht PDF", func() {
		if err := core.SaveCashBooks(jsonPath, books); err != nil {
			dialog.ShowInformation(a.bundle.T("error.processing.title"), err.Error(), a.window)
			return
		}
		var madePaths []string
		for _, acc := range accounts {
			book := bookFor(acc)
			invoices := a.cashInvoicesFor(acc)
			entries, endbestand := core.ComputeCashReport(*book, invoices)
			outPath := filepath.Join(monthFolder,
				"Kassenbericht_"+core.SanitizeFilename(acc)+"_"+monthLabel+".pdf")
			if err := core.WriteCashReportPDF(outPath, *book, entries, endbestand, monthLabel, a.settings.DecimalSeparator); err != nil {
				dialog.ShowInformation(a.bundle.T("error.processing.title"), err.Error(), a.window)
				return
			}
			madePaths = append(madePaths, outPath)
		}
		// Open the generated report(s) in the OS default PDF viewer so the click
		// visibly produces something.
		for _, p := range madePaths {
			a.openFileInOS(p)
		}
		var names []string
		for _, p := range madePaths {
			names = append(names, filepath.Base(p))
		}
		dialog.ShowInformation("Kassenbericht",
			"Erstellt und geöffnet:\n"+strings.Join(names, "\n"), a.window)
	})

	toolbar := container.NewBorder(nil, nil,
		container.NewHBox(widget.NewLabel("Konto:"), accountSelect),
		container.NewHBox(saveBtn, pdfBtn),
	)
	return container.NewBorder(
		container.NewPadded(toolbar), nil, nil, nil,
		container.NewVScroll(editArea))
}

// buildCashYearBody renders a read-only twelve-month overview ("Ganzes Jahr")
// for one cash account: per month the opening balance, deposits, cash expenses
// and closing balance. Clicking a month jumps to that month's cash book. The
// year is a.currentYear (changed via the period dropdown, not here).
func (a *App) buildCashYearBody(accounts []string) fyne.CanvasObject {
	curYear := a.currentYear

	accountSelect := widget.NewSelect(accounts, nil)
	accountSelect.SetSelected(accounts[0])

	tableArea := container.NewVBox()
	var lastSummaries []core.MonthSummary // for the PDF export

	rebuild := func() {
		curAccount := accountSelect.Selected

		// Collect the twelve months' input data.
		months := make([]core.MonthInput, 12)
		for i := 0; i < 12; i++ {
			m := time.Month(i + 1)
			folder := a.storageManager.GetMonthFolder(curYear, m)
			storedBooks, _ := core.LoadCashBooks(filepath.Join(folder, "kassenbuch.json"))
			mi := core.MonthInput{Invoices: a.cashInvoicesForMonth(curAccount, curYear, m)}
			for _, b := range storedBooks {
				if b.Konto == curAccount {
					mi.HasStoredBook = true
					mi.Book = b
					break
				}
			}
			months[i] = mi
		}
		carriedIn, _ := a.cashCarryIn(curAccount, curYear, time.January)
		summaries := core.ComputeYearOverview(carriedIn, months)
		lastSummaries = summaries

		tableArea.Objects = tableArea.Objects[:0]
		tableArea.Add(container.NewGridWithColumns(5,
			widget.NewLabelWithStyle("Monat", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Anfangsbestand", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Einnahmen", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Ausgaben", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Endbestand", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
		))
		for _, s := range summaries {
			m := s.Month
			monthBtn := widget.NewButton(fmt.Sprintf("%s %d", a.bundle.T(fmt.Sprintf("month.%02d", int(m))), curYear), func() {
				a.currentMonth = m
				a.cashWholeYear = false
				a.window.SetContent(a.buildUI())
			})
			monthBtn.Importance = widget.LowImportance
			anfLbl := newCopyableLabel(a.bundle, formatDecimal(s.Anfangsbestand, a.settings.DecimalSeparator))
			anfLbl.Alignment = fyne.TextAlignTrailing
			einLbl := newCopyableLabel(a.bundle, formatDecimal(s.Einnahmen, a.settings.DecimalSeparator))
			einLbl.Alignment = fyne.TextAlignTrailing
			ausLbl := newCopyableLabel(a.bundle, formatDecimal(s.Ausgaben, a.settings.DecimalSeparator))
			ausLbl.Alignment = fyne.TextAlignTrailing
			endLbl := newCopyableLabel(a.bundle, formatDecimal(s.Endbestand, a.settings.DecimalSeparator))
			endLbl.Alignment = fyne.TextAlignTrailing
			tableArea.Add(container.NewGridWithColumns(5,
				monthBtn, anfLbl, einLbl, ausLbl, endLbl,
			))
		}
		tableArea.Refresh()
	}

	accountSelect.OnChanged = func(string) { rebuild() }
	rebuild()

	exportBtn := widget.NewButton("PDF-Export", func() {
		data, err := core.BuildCashYearOverviewPDF(lastSummaries, accountSelect.Selected, curYear, a.profile)
		if err != nil {
			a.showError("PDF-Export", err.Error())
			return
		}
		a.savePDF(fmt.Sprintf("Kassen-Jahresuebersicht_%s_%d.pdf",
			core.SanitizeFilename(accountSelect.Selected), curYear), data)
	})
	exportBtn.Importance = widget.LowImportance

	toolbar := container.NewBorder(nil, nil,
		container.NewHBox(widget.NewLabel("Konto:"), accountSelect),
		container.NewHBox(exportBtn),
	)
	return container.NewBorder(
		container.NewPadded(toolbar), nil, nil, nil,
		container.NewVScroll(tableArea))
}

// cashPeriodLabel is the text shown on the Kassenbuch period dropdown button.
func (a *App) cashPeriodLabel() string {
	if a.cashWholeYear {
		return fmt.Sprintf("Kassenbuch — Ganzes Jahr %d", a.currentYear)
	}
	return fmt.Sprintf("Kassenbuch — %s %d",
		a.bundle.T(fmt.Sprintf("month.%02d", int(a.currentMonth))), a.currentYear)
}

// buildCashPeriodButton is the Kassenbuch period picker shown top-left in the
// header: a button that opens a menu with "Ganzes Jahr", each month of the
// current year, and a "Weitere Jahre" submenu listing other years that have
// cash activity. Selecting an entry rebuilds the whole view.
func (a *App) buildCashPeriodButton() fyne.CanvasObject {
	var btn *widget.Button
	btn = widget.NewButton(a.cashPeriodLabel()+"  ▾", func() {
		items := []*fyne.MenuItem{
			fyne.NewMenuItem(fmt.Sprintf("Ganzes Jahr %d", a.currentYear), func() {
				a.cashWholeYear = true
				a.window.SetContent(a.buildUI())
			}),
			fyne.NewMenuItemSeparator(),
		}
		for i := 1; i <= 12; i++ {
			mm := time.Month(i)
			items = append(items, fyne.NewMenuItem(
				fmt.Sprintf("%s %d", a.bundle.T(fmt.Sprintf("month.%02d", i)), a.currentYear),
				func() {
					a.currentMonth = mm
					a.cashWholeYear = false
					a.window.SetContent(a.buildUI())
				}))
		}
		// "Weitere Jahre": other years that ever had a cash book.
		var yearItems []*fyne.MenuItem
		for _, y := range a.cashDataYears() {
			if y == a.currentYear {
				continue
			}
			yy := y
			yearItems = append(yearItems, fyne.NewMenuItem(fmt.Sprintf("%d", yy), func() {
				a.currentYear = yy
				a.window.SetContent(a.buildUI())
			}))
		}
		if len(yearItems) > 0 {
			sub := fyne.NewMenuItem("Weitere Jahre", nil)
			sub.ChildMenu = fyne.NewMenu("", yearItems...)
			items = append(items, fyne.NewMenuItemSeparator(), sub)
		}
		menu := fyne.NewMenu("", items...)
		canvas := fyne.CurrentApp().Driver().CanvasForObject(btn)
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(btn)
		pos.Y += btn.Size().Height
		widget.ShowPopUpMenuAtPosition(menu, canvas, pos)
	})
	btn.Alignment = widget.ButtonAlignLeading
	return btn
}

// cashDataYears returns the sorted set of years that have any stored cash book
// (kassenbuch.json), always including the current year, so the period dropdown
// can offer "Weitere Jahre".
func (a *App) cashDataYears() []int {
	set := map[int]bool{a.currentYear: true}
	now := time.Now().Year()
	for y := now - 8; y <= now+1; y++ {
		for m := time.January; m <= time.December; m++ {
			p := filepath.Join(a.storageManager.GetMonthFolder(y, m), "kassenbuch.json")
			if _, err := os.Stat(p); err == nil {
				set[y] = true
				break
			}
		}
	}
	years := make([]int, 0, len(set))
	for y := range set {
		years = append(years, y)
	}
	sort.Ints(years)
	return years
}
