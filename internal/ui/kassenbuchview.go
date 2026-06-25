package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
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

// showCashBookView replaces the window content with the cash-book view
// for the currently selected month.
func (a *App) showCashBookView() {
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

	accounts := a.cashAccounts()

	titleLabel := widget.NewLabelWithStyle(
		"Kassenbuch — "+monthLabel, fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
	)
	backBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() { a.showMainView() })

	body := container.NewVBox()

	if len(accounts) == 0 {
		body.Add(newCopyableLabel(a.bundle,
			"Kein Konto vom Typ \"Barkasse\" vorhanden. Lege in den Einstellungen unter Konten ein Zahlungskonto mit Typ \"Barkasse\" an.",
		))
		header := container.NewBorder(nil, nil, container.NewPadded(titleLabel),
			container.NewPadded(backBtn))
		a.window.SetContent(container.NewBorder(header, nil, nil, nil, container.NewVScroll(body)))
		return
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

		startEntry := widget.NewEntry()
		startEntry.SetText(formatDecimal(book.Anfangsbestand, a.settings.DecimalSeparator))
		startEntry.OnChanged = func(s string) { book.Anfangsbestand = parseFloat(s, a.settings.DecimalSeparator) }

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

		outflowList := container.NewVBox()
		for _, e := range entries {
			if e.Ausgabe == 0 {
				continue
			}
			row, ok := rowByName[e.Beleg]
			label := fmt.Sprintf("%s  —  %s  —  %s  —  %s",
				e.Datum, e.Beschreibung,
				formatDecimal(e.Ausgabe, a.settings.DecimalSeparator), e.Beleg)
			if !ok {
				outflowList.Add(newCopyableLabel(a.bundle, "  "+label))
				continue
			}
			btn := widget.NewButton(label, func() {
				a.showEditDialog(row, rebuild)
			})
			btn.Importance = widget.LowImportance
			btn.Alignment = widget.ButtonAlignLeading
			outflowList.Add(btn)
		}
		if len(outflowList.Objects) == 0 {
			outflowList.Add(newCopyableLabel(a.bundle, "  (keine Bar-Ausgaben in diesem Monat)"))
		}

		editArea.Add(widget.NewForm(
			widget.NewFormItem("Anfangsbestand", startEntry),
		))
		editArea.Add(widget.NewLabelWithStyle("Einlagen", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		editArea.Add(depositList)
		editArea.Add(addDepositBtn)
		editArea.Add(widget.NewSeparator())
		editArea.Add(widget.NewLabelWithStyle("Bar-Ausgaben (aus Rechnungen)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		editArea.Add(outflowList)
		editArea.Add(widget.NewSeparator())
		editArea.Add(widget.NewLabelWithStyle(
			"Endbestand: "+formatDecimal(endbestand, a.settings.DecimalSeparator), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
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
		var made []string
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
			made = append(made, filepath.Base(outPath))
		}
		dialog.ShowInformation("Kassenbericht",
			"Erstellt:\n"+strings.Join(made, "\n"), a.window)
	})

	yearViewBtn := widget.NewButton("Jahresübersicht", func() {
		a.showCashYearView(accountSelect.Selected, a.currentYear)
	})

	header := container.NewBorder(nil, nil,
		container.NewPadded(titleLabel),
		container.NewPadded(container.NewHBox(backBtn, saveBtn, pdfBtn, yearViewBtn)),
		container.NewPadded(accountSelect),
	)

	a.window.SetContent(container.NewBorder(
		header, nil, nil, nil, container.NewVScroll(editArea)))
}

// showCashYearView shows a read-only twelve-month overview for one cash
// account: per month the opening balance, deposits, cash expenses and
// closing balance. Clicking a month opens that month's cash book.
func (a *App) showCashYearView(account string, year int) {
	accounts := a.cashAccounts()
	if len(accounts) == 0 {
		a.showMainView()
		return
	}
	// Fall back to the first account if the requested one no longer exists.
	sel := accounts[0]
	for _, n := range accounts {
		if n == account {
			sel = account
			break
		}
	}

	curYear := year

	titleLabel := widget.NewLabelWithStyle(
		fmt.Sprintf("Jahresübersicht — %d", curYear),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	backBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() { a.showMainView() })

	accountSelect := widget.NewSelect(accounts, nil)
	accountSelect.SetSelected(sel)
	yearSelect := widget.NewSelect(generateYearOptions(), nil)
	yearSelect.SetSelected(fmt.Sprintf("%d", curYear))

	tableArea := container.NewVBox()
	var lastSummaries []core.MonthSummary // for the PDF export

	rebuild := func() {
		curAccount := accountSelect.Selected
		titleLabel.SetText(fmt.Sprintf("Jahresübersicht — %d", curYear))

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
				a.currentYear = curYear
				a.currentMonth = m
				a.yearSelect.SetSelected(fmt.Sprintf("%d", curYear))
				a.monthSelect.SetSelected(
					fmt.Sprintf("%02d - %-12s", int(m), a.bundle.T(fmt.Sprintf("month.%02d", int(m)))))
				a.showCashBookView()
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
	yearSelect.OnChanged = func(s string) {
		var y int
		fmt.Sscanf(s, "%d", &y)
		if y != 0 {
			curYear = y
		}
		rebuild()
	}
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

	header := container.NewBorder(nil, nil,
		container.NewPadded(titleLabel),
		container.NewPadded(container.NewHBox(exportBtn, backBtn)),
		container.NewPadded(container.NewHBox(accountSelect, yearSelect)),
	)
	a.window.SetContent(container.NewBorder(
		header, nil, nil, nil, container.NewVScroll(tableArea)))
}
