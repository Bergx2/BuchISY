package ui

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// resolveInvoicePath locates the file behind a CSV row. The primary path
// is derived from the currently viewed month + the row's Unterordner,
// but that can be stale (Ganzes-Jahr view, externally moved file, stale
// Unterordner). Tries fallbacks before giving up; returns the primary
// path even on miss so callers/log messages still have something
// concrete to reference.
func (a *App) resolveInvoicePath(row core.CSVRow) string {
	primary := core.InvoiceFilePath(
		a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth), row)
	if core.FileExists(primary) {
		return primary
	}

	// Bare filename in the current month folder (handles stale Unterordner).
	curFolder := a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth)
	if alt := filepath.Join(curFolder, row.Dateiname); core.FileExists(alt) {
		return alt
	}

	// Scan every month of the row's invoice year (or the current year as
	// fallback) with and without category subfolders.
	year := a.currentYear
	if y, err := strconv.Atoi(row.Jahr); err == nil && y > 0 {
		year = y
	}
	subdirs := []string{""}
	if row.Unterordner != "" {
		subdirs = append(subdirs, row.Unterordner)
	}
	subdirs = append(subdirs, "Ausgangsrechnungen", "Bar")

	for m := time.January; m <= time.December; m++ {
		folder := a.storageManager.GetMonthFolder(year, m)
		for _, sub := range subdirs {
			candidate := filepath.Join(folder, sub, row.Dateiname)
			if core.FileExists(candidate) {
				return candidate
			}
		}
	}
	return primary
}

// showEditDialog shows a resizable window to edit an existing invoice,
// with a document preview on the right.
func (a *App) showEditDialog(row core.CSVRow, onClose func()) {
	if a.currentMonthLocked {
		a.showInfo(a.bundle.T("period.locked.title"), a.bundle.T("period.locked.msg"))
		return
	}

	meta := row.ToMeta()

	originalPath := a.resolveInvoicePath(row)
	if core.FileExists(originalPath) {
		a.logger.Info("Edit dialog preview path: %s", originalPath)
	} else {
		a.logger.Warn("Edit dialog preview: file NOT found at %s "+
			"(row.Dateiname=%q row.Unterordner=%q row.Jahr=%q currentMonth=%d-%02d)",
			originalPath, row.Dateiname, row.Unterordner, row.Jahr,
			a.currentYear, a.currentMonth)
	}

	// Forward-declared so the calendar buttons can target this window.
	var editWin fyne.Window

	// recomputeBooking is forward-declared so closures created before it
	// is assigned (account picker, bank account select) can call it safely
	// via the nil guard.
	var recomputeBooking func()

	companyEntry := widget.NewEntry()
	companyEntry.SetText(meta.Auftraggeber)
	companyEntry.SetPlaceHolder(a.bundle.T("field.company"))

	shortDescEntry := widget.NewEntry()
	shortDescEntry.SetText(meta.Verwendungszweck)
	shortDescEntry.SetPlaceHolder(a.bundle.T("field.shortdesc"))
	shortDescLabel := widget.NewLabel(fmt.Sprintf("%d / 80", len(meta.Verwendungszweck)))

	invoiceNumEntry := widget.NewEntry()
	invoiceNumEntry.SetText(meta.Rechnungsnummer)
	invoiceNumEntry.SetPlaceHolder(a.bundle.T("field.invoicenumber"))

	vatIDEntry := widget.NewEntry()
	vatIDEntry.SetText(meta.VATID)
	vatIDEntry.SetPlaceHolder("z. B. DE123456789")

	dateEntry := widget.NewEntry()
	dateEntry.SetText(meta.Rechnungsdatum)
	dateEntry.SetPlaceHolder(a.bundle.T("field.invoiceDate"))

	dateCalendarBtn := widget.NewButton("📅", func() {
		a.showDatePicker(editWin, dateEntry.Text, func(selectedDate string) {
			dateEntry.SetText(selectedDate)
		})
	})
	dateCalendarBtn.Importance = widget.LowImportance

	// VAT-lines editor — declared here so updateFilenamePreview can reference it.
	var ed *taxLinesEditor

	currencySelect := widget.NewSelect(core.CurrencyOptions(), nil)
	{
		code := meta.Waehrung
		if code == "" {
			code = a.settings.CurrencyDefault
		}
		currencySelect.SetSelected(core.CurrencyOptionForCode(code))
	}

	// Account picker (SKR04-based)
	selectedAccount := meta.Gegenkonto
	if selectedAccount == 0 {
		selectedAccount = a.settings.DefaultAccount
	}
	accountDisplay := widget.NewEntry()
	accountDisplay.Disable()
	updateAccountDisplay := func() {
		if selectedAccount == 0 {
			accountDisplay.SetText("")
			return
		}
		if acc, ok := a.chart.Find(selectedAccount); ok {
			accountDisplay.SetText(accountLabel(acc))
		} else {
			accountDisplay.SetText(fmt.Sprintf("%d", selectedAccount))
		}
	}
	updateAccountDisplay()
	chooseAccountBtn := widget.NewButton(a.bundle.T("picker.account.choose"), func() {
		a.showAccountSearch(selectedAccount, editWin, func(n int) {
			selectedAccount = n
			updateAccountDisplay()
			if a.accountPrefs != nil { // record invoice Gegenkonto picks as "recently used"
				a.accountPrefs.RecordUse(n)
				_ = a.accountPrefs.Save()
			}
			if recomputeBooking != nil {
				recomputeBooking()
			}
		})
	})

	bankAccountSelect := widget.NewSelect(a.bankAccountOptionList(), nil)
	bankAccountSelect.OnChanged = func(string) {
		if recomputeBooking != nil {
			recomputeBooking()
		}
	}
	a.preselectBankAccount(bankAccountSelect, meta.Bankkonto)
	addBankBtn := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		a.addBankAccountInline(editWin, func(name string) {
			a.refreshBankAccountSelect(bankAccountSelect, name)
		})
	})
	addBankBtn.Importance = widget.LowImportance

	paymentDateEntry := widget.NewEntry()
	paymentDateEntry.SetText(meta.Bezahldatum)
	paymentDateEntry.SetPlaceHolder(a.bundle.T("field.paymentDate"))

	paymentDateCalendarBtn := widget.NewButton("📅", func() {
		a.showDatePicker(editWin, paymentDateEntry.Text, func(selectedDate string) {
			paymentDateEntry.SetText(selectedDate)
		})
	})
	paymentDateCalendarBtn.Importance = widget.LowImportance

	partialPaymentCheck := widget.NewCheck(a.bundle.T("field.partialPayment"), nil)
	partialPaymentCheck.SetChecked(meta.Teilzahlung)

	ausgangsrechnungCheck := widget.NewCheck("Ausgangsrechnung", nil)
	ausgangsrechnungCheck.SetChecked(row.Ausgangsrechnung || row.Unterordner == "Ausgangsrechnungen")

	// Bewirtung fields (shown only when category == "bewirtung")
	anlassEntry := widget.NewEntry()
	anlassEntry.SetText(meta.BewirtungAnlass)
	anlassEntry.SetPlaceHolder(a.bundle.T("field.bewirtungAnlass"))
	teilnehmerEntry := widget.NewEntry()
	teilnehmerEntry.SetText(meta.BewirtungTeilnehmer)
	teilnehmerEntry.SetPlaceHolder(a.bundle.T("field.bewirtungTeilnehmer"))
	// Alternative to electronic entry: Anlass/Teilnehmer handwritten on the
	// receipt/attachment. When checked, the missing-details warning is suppressed.
	aufBelegCheck := widget.NewCheck(a.bundle.T("field.bewirtungAufBeleg"), nil)
	aufBelegCheck.SetChecked(meta.BewirtungAngabenAufBeleg)
	bewirtungBox := container.NewVBox(
		widget.NewLabel(a.bundle.T("field.bewirtungAnlass")),
		anlassEntry,
		widget.NewLabel(a.bundle.T("field.bewirtungTeilnehmer")),
		teilnehmerEntry,
		aufBelegCheck,
	)
	bewirtungBox.Hide()

	// Comment field (multiline)
	commentEntry := widget.NewMultiLineEntry()
	commentEntry.SetText(meta.Kommentar)
	commentEntry.SetPlaceHolder(a.bundle.T("field.comment"))
	commentEntry.SetMinRowsVisible(3)

	// Currency conversion fields (only relevant for non-default currency).
	netEUREntry := widget.NewEntry()
	if meta.BetragNetto_EUR > 0 {
		netEUREntry.SetText(formatDecimal(meta.BetragNetto_EUR, a.settings.DecimalSeparator))
	}
	netEUREntry.SetPlaceHolder(a.bundle.T("field.net_eur"))

	feeEntry := widget.NewEntry()
	if meta.Gebuehr > 0 {
		feeEntry.SetText(formatDecimal(meta.Gebuehr, a.settings.DecimalSeparator))
	}
	feeEntry.SetPlaceHolder(a.bundle.T("field.fee"))

	rabattEntry := widget.NewEntry()
	if meta.Rabatt > 0 {
		rabattEntry.SetText(formatDecimal(meta.Rabatt, a.settings.DecimalSeparator))
	}
	rabattEntry.SetPlaceHolder(a.bundle.T("field.rabatt"))

	paidActualLabel := widget.NewLabel("")
	updatePaidActual := func() {
		brutto := 0.0
		if ed != nil {
			brutto = ed.Brutto()
		}
		rabatt := parseFloat(rabattEntry.Text, a.settings.DecimalSeparator)
		if brutto != 0 || rabatt != 0 {
			paid := brutto - rabatt
			paidActualLabel.SetText(a.bundle.T("field.paidActual") + ": " + formatDecimal(paid, a.settings.DecimalSeparator))
		} else {
			paidActualLabel.SetText("")
		}
	}
	rabattEntry.OnChanged = func(string) {
		updatePaidActual()
		if recomputeBooking != nil {
			recomputeBooking()
		}
	}

	// Exchange rate + CC-fee% entries for live EUR conversion
	kursEntry := widget.NewEntry()
	kursEntry.SetPlaceHolder(a.bundle.T("field.rate"))
	if meta.Wechselkurs > 0 {
		kursEntry.SetText(strings.Replace(fmt.Sprintf("%g", meta.Wechselkurs), ".", ",", 1))
	}
	feePctEntry := widget.NewEntry()
	feePctEntry.SetPlaceHolder(a.bundle.T("field.fee.percent"))
	if meta.GebuehrProzent > 0 {
		feePctEntry.SetText(strings.Replace(fmt.Sprintf("%g", meta.GebuehrProzent), ".", ",", 1))
	}
	gesamtEURLabel := widget.NewLabel("")

	// recomputeEUR reads kurs/pct + gross/net from the tax-lines editor and
	// updates netEUREntry, feeEntry, and gesamtEURLabel live. ed may be nil
	// during initial construction — guarded below.
	recomputeEUR := func() {
		kurs := parseDecimal(kursEntry.Text)
		pct := parseDecimal(feePctEntry.Text)
		var brutto, netto float64
		if ed != nil {
			brutto = ed.Brutto()
			netto = core.SumNetto(ed.Lines())
		}
		c := core.ConvertForeignPayment(brutto, netto, kurs, pct)
		if kurs > 0 {
			netEUREntry.SetText(formatDecimal(c.NettoEUR, a.settings.DecimalSeparator))
			if pct > 0 {
				feeEntry.SetText(formatDecimal(c.GebuehrEUR, a.settings.DecimalSeparator))
			}
			gesamtEURLabel.SetText(a.bundle.T("field.total.eur", formatMoney(c.GesamtEUR, "EUR", a.settings.DecimalSeparator)))
		} else {
			gesamtEURLabel.SetText("") // clear stale total when the rate is removed
		}
	}
	kursEntry.OnChanged = func(string) { recomputeEUR() }
	feePctEntry.OnChanged = func(string) { recomputeEUR() }

	// Currency conversion fields are only shown when a non-default
	// currency is selected; this container is (re)populated on demand.
	currencyConversionContainer := container.NewVBox()
	updateCurrencyConversionVisibility := func() {
		if currencySelect.Selected != "" &&
			core.CurrencyCodeFromOption(currencySelect.Selected) != a.settings.CurrencyDefault {
			defaultCurrency := a.settings.CurrencyDefault
			feeLabel := fmt.Sprintf("%s (%s)", a.bundle.T("field.fee"), defaultCurrency)
			netEURLabel := fmt.Sprintf("%s (%s)", a.bundle.T("field.net_eur"), defaultCurrency)
			currencyConversionContainer.Objects = []fyne.CanvasObject{
				section(a.bundle.T("currency.conversion.section"), widget.NewForm(
					widget.NewFormItem(a.bundle.T("field.rate"), kursEntry),
					widget.NewFormItem(a.bundle.T("field.fee.percent"), feePctEntry),
					widget.NewFormItem(netEURLabel, netEUREntry),
					widget.NewFormItem(feeLabel, feeEntry),
					widget.NewFormItem(a.bundle.T("field.total.eur.label"), gesamtEURLabel),
				)),
			}
			recomputeEUR()
		} else {
			currencyConversionContainer.Objects = []fyne.CanvasObject{}
		}
		currencyConversionContainer.Refresh()
	}

	// Ablagemonat (filing month) — prefilled with the folder the invoice
	// currently lives in.
	// Ablage (filing period) defaults to the invoice's OWN stored Jahr/Monat —
	// not the currently viewed month — so reopening a receipt that was filed in
	// another month (e.g. moved to 02.2026 while the year view sits on June)
	// shows its real filing month instead of reverting to the view's month.
	filingYear := a.currentYear
	if y, err := strconv.Atoi(strings.TrimSpace(row.Jahr)); err == nil && y > 0 {
		filingYear = y
	}
	filingMonth := a.currentMonth
	if m, err := strconv.Atoi(strings.TrimSpace(row.Monat)); err == nil && m >= 1 && m <= 12 {
		filingMonth = time.Month(m)
	}
	yearSelect := widget.NewSelect(generateYearOptions(), nil)
	yearSelect.SetSelected(fmt.Sprintf("%d", filingYear))
	monthSelect := widget.NewSelect(generateMonthOptions(a.bundle), nil)
	monthSelect.SetSelected(fmt.Sprintf("%02d - %-12s", int(filingMonth),
		a.bundle.T(fmt.Sprintf("month.%02d", int(filingMonth)))))

	// Editable filename field.
	filenameEntry := widget.NewEntry()
	filenameEdited := false
	suppressFilenameChange := false
	updateFilenamePreview := func() {
		if filenameEdited {
			return
		}
		// ed may still be nil during initial setup; treat amounts as 0 in that case.
		var brutto, netto, mwstBetrag, mwstProzent float64
		if ed != nil {
			brutto = ed.Brutto()
			netto = core.SumNetto(ed.Lines())
			mwstBetrag = core.SumMwSt(ed.Lines())
			mwstProzent = core.PrimarySatz(ed.Lines())
		}
		currentMeta := core.Meta{
			Auftraggeber:      companyEntry.Text,
			Verwendungszweck:  shortDescEntry.Text,
			Rechnungsnummer:   invoiceNumEntry.Text,
			Rechnungsdatum:    dateEntry.Text,
			BetragNetto:       netto,
			SteuersatzProzent: mwstProzent,
			SteuersatzBetrag:  mwstBetrag,
			Bruttobetrag:      brutto,
			Waehrung:          core.CurrencyCodeFromOption(currencySelect.Selected),
		}
		parts := strings.Split(dateEntry.Text, ".")
		if len(parts) == 3 {
			currentMeta.Jahr = parts[2]
			currentMeta.Monat = parts[1]
		}
		filename, err := core.ApplyTemplate(
			a.settings.NamingTemplate,
			currentMeta,
			core.TemplateOpts{DecimalSeparator: a.settings.DecimalSeparator},
		)
		suppressFilenameChange = true
		if err != nil {
			filenameEntry.SetText("Fehler: " + err.Error())
		} else {
			filenameEntry.SetText(filename)
		}
		suppressFilenameChange = false
	}
	filenameEntry.OnChanged = func(string) {
		if !suppressFilenameChange {
			filenameEdited = true
		}
	}

	shortDescEntry.OnChanged = func(s string) {
		if len(s) > 80 {
			shortDescEntry.SetText(s[:80])
		}
		shortDescLabel.SetText(fmt.Sprintf("%d / 80", len(shortDescEntry.Text)))
		updateFilenamePreview()
	}
	onAnyChange := func(string) { updateFilenamePreview() }
	companyEntry.OnChanged = onAnyChange
	invoiceNumEntry.OnChanged = onAnyChange
	dateEntry.OnChanged = onAnyChange

	// Create the VAT-lines editor now that updateFilenamePreview is defined.
	// onChange is a combined callback: filename preview + booking recompute.
	// recomputeBooking is forward-declared at the top of this function so
	// both this closure and the account/bank-account closures above can
	// reference it safely via the nil guard.
	ed = newTaxLinesEditor(a, meta.TaxLines, meta.Trinkgeld, core.CurrencyCodeFromOption(currencySelect.Selected), func() {
		updateFilenamePreview()
		if recomputeBooking != nil {
			recomputeBooking()
		}
		recomputeEUR()
		updatePaidActual()
	})

	updateFilenamePreview()
	updatePaidActual()

	// Booking category: inferred from the stored booking (so reopening a
	// reverse-charge / Bewirtung invoice doesn't reset to "standard" and
	// overwrite the booking on save), else a learned template, else "standard".
	category := "standard"
	if tmpl, ok := a.bookingTemplates.Get(meta.Auftraggeber); ok {
		category = tmpl.Kategorie
	}
	if inferred := core.InferBookingCategory(row.Buchung); inferred != "" {
		category = inferred
	}
	// When the stored booking is a plain "standard" posting (not manual, not an
	// already-recognised special category), auto-suggest a special category —
	// e.g. an invoice booked fully on the Bewirtung account gets "Bewirtung"
	// proposed so re-saving applies the 70/30 split. The user confirms it first.
	autoSuggestedCategory := ""
	if category == "standard" && !row.Buchung.Manuell {
		if cat, ok := a.bookingRules.SuggestCategory(selectedAccount, meta.Auftraggeber+" "+meta.Verwendungszweck); ok {
			category = cat
			autoSuggestedCategory = cat
		}
	}
	catOptions, catKeyByLabel := a.bookingCategoryOptions()
	categorySelect := widget.NewSelect(catOptions, nil)
	categorySelect.SetSelected(a.bookingCategoryLabel(category))

	// #1: hint that the category was auto-detected so the user knows it's a
	// suggestion to confirm (re-saving applies the split). Hidden once changed.
	categoryHint := widget.NewLabel("")
	categoryHint.Importance = widget.LowImportance
	categoryHint.Wrapping = fyne.TextWrapWord
	categoryHint.Hide()
	if autoSuggestedCategory != "" {
		categoryHint.SetText("ℹ " + a.bundle.T("booking.category.autodetected"))
		categoryHint.Show()
	}

	bookingPrev := newBookingPreview(a)
	var manualBooking *core.Booking
	if row.Buchung.Manuell && len(row.Buchung.Entries) > 0 {
		b := row.Buchung
		manualBooking = &b
	}
	// Forward-declared so recomputeBooking can trigger it after booking is set.
	var refreshWarnings func()
	recomputeBooking = func() {
		if manualBooking != nil {
			bookingPrev.set(*manualBooking, manualBooking.Balanced(), a.bundle.T("booking.manual.hint"))
			if refreshWarnings != nil {
				refreshWarnings()
			}
			return
		}
		// Convert tax lines + scalar amounts to EUR before building the booking,
		// so the stored Buchung is always in EUR even for foreign-currency invoices.
		waehr := core.CurrencyCodeFromOption(currencySelect.Selected)
		kurs := parseDecimal(kursEntry.Text)
		linesEUR := core.TaxLinesEUR(ed.Lines(), waehr, kurs)
		toEUR := func(v float64) float64 {
			if waehr != "" && waehr != "EUR" && kurs > 0 {
				return v / kurs
			}
			return v
		}
		trinkgeldEUR := toEUR(ed.Trinkgeld())
		rabattEUR := toEUR(parseFloat(rabattEntry.Text, a.settings.DecimalSeparator))
		var b core.Booking
		var bookable bool
		var reason string
		if ausgangsrechnungCheck.Checked {
			b, bookable, reason = a.computeRevenueBooking(
				linesEUR, selectedAccount, bankAccountSelect.Selected)
		} else {
			b, bookable, reason = a.computeInvoiceBooking(
				catKeyByLabel[categorySelect.Selected],
				linesEUR, trinkgeldEUR, selectedAccount, bankAccountSelect.Selected,
				rabattEUR)
		}
		bookingPrev.set(b, bookable, reason)
		if refreshWarnings != nil {
			refreshWarnings()
		}
	}
	// updateBewirtungVisibility shows the Anlass/Teilnehmer fields only for Bewirtung.
	updateBewirtungVisibility := func() {
		if catKeyByLabel[categorySelect.Selected] == "bewirtung" {
			bewirtungBox.Show()
		} else {
			bewirtungBox.Hide()
		}
	}
	categorySelect.OnChanged = func(string) {
		categoryHint.Hide() // user has reviewed the category → no longer "auto"
		recomputeBooking()
		updateBewirtungVisibility()
	}
	// Set initial visibility based on the pre-selected category.
	updateBewirtungVisibility()
	ausgangsrechnungCheck.OnChanged = func(bool) { recomputeBooking() }

	editBookingBtn := widget.NewButton(a.bundle.T("booking.manual.adjust"), func() {
		var seed core.Booking
		if manualBooking != nil {
			seed = *manualBooking
		} else {
			if ausgangsrechnungCheck.Checked {
				seed, _, _ = a.computeRevenueBooking(
					ed.Lines(), selectedAccount, bankAccountSelect.Selected)
			} else {
				seed, _, _ = a.computeInvoiceBooking(
					catKeyByLabel[categorySelect.Selected],
					ed.Lines(), ed.Trinkgeld(), selectedAccount, bankAccountSelect.Selected,
					parseFloat(rabattEntry.Text, a.settings.DecimalSeparator))
			}
		}
		a.showBookingEditor(seed, editWin, func(edited core.Booking) {
			manualBooking = &edited
			recomputeBooking()
		})
	})
	autoBookingBtn := widget.NewButton(a.bundle.T("booking.auto"), func() {
		manualBooking = nil
		recomputeBooking()
	})

	// Initial booking preview — call after all widgets are set up.
	recomputeBooking()

	// Live plausibility-warning strip — mirrors the one in invoicemodal.go.
	warningsLabel := widget.NewLabel("")
	warningsLabel.Importance = widget.WarningImportance
	warningsLabel.Wrapping = fyne.TextWrapWord
	warningsLabel.Hide()

	refreshWarnings = func() {
		warnings := core.InvoiceWarnings(core.CSVRow{
			BetragNetto:              core.SumNetto(ed.Lines()),
			SteuersatzBetrag:         core.SumMwSt(ed.Lines()),
			Bruttobetrag:             ed.Brutto(),
			Trinkgeld:                ed.Trinkgeld(),
			Gegenkonto:               selectedAccount,
			Waehrung:                 core.CurrencyCodeFromOption(currencySelect.Selected),
			Wechselkurs:              parseDecimal(kursEntry.Text),
			Rechnungsdatum:           dateEntry.Text,
			VATID:                    vatIDEntry.Text,
			Ausgangsrechnung:         ausgangsrechnungCheck.Checked,
			Buchung:                  bookingPrev.last,
			BewirtungAnlass:          anlassEntry.Text,
			BewirtungTeilnehmer:      teilnehmerEntry.Text,
			BewirtungAngabenAufBeleg: aufBelegCheck.Checked,
		})
		if len(warnings) == 0 {
			warningsLabel.Hide()
		} else {
			warningsLabel.SetText("⚠ " + strings.Join(warnings, " • "))
			warningsLabel.Show()
		}
	}

	cancelBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() {
		editWin.Close()
	})
	saveBtn := widget.NewButton(a.bundle.T("btn.save"), nil)
	saveBtn.Importance = widget.HighImportance

	openBelegBtn := widget.NewButton("Beleg öffnen", func() {
		a.openFileInOS(originalPath)
	})
	openBelegBtn.Importance = widget.LowImportance

	// Preview pane + currently shown strip. Built below; declared up
	// here so the attachments switcher closure can capture them.
	var preview *fyne.Container
	var previewStrip *pdfPreviewStrip

	// Attachments preview switcher: [Original] [Anhang 1] [Anhang 2] …
	// Click swaps the preview content. The switcher is rebuildable so
	// it picks up freshly added attachments without closing the dialog.
	attPaths := a.invoiceAttachmentPaths(row)

	// If this invoice is reconciled to a bank statement, resolve that
	// statement's PDF so the preview switcher can show it ("Kontoauszug")
	// at the matching booking — the amount is highlighted like on the receipt.
	// Recomputed after a single-invoice match links a statement line.
	var statementPreviewPath string
	recomputeStatementPath := func() {
		statementPreviewPath = ""
		if row.BuchungRef != "" && row.BuchungRef != core.CashConfirmedRef {
			if ref := core.FirstBuchungRef(row.BuchungRef); ref.StatementFilename != "" {
				p := filepath.Join(a.statementFolder(row.Bankkonto), ref.StatementFilename)
				if core.FileExists(p) {
					statementPreviewPath = p
				}
			}
		}
	}
	recomputeStatementPath()

	currentPreviewPath := originalPath
	previewSwitcher := container.NewHBox()
	var rebuildSwitcher func()
	var delAttBtn *widget.Button // forward-declared so rebuildSwitcher can toggle it

	// linkedStatementLines returns the StatementBooking for EVERY line referenced
	// by the receipt's BuchungRef. A 1→N split links several lines (possibly on
	// different pages), so this drives both the green frame on each matched line
	// and the "abgeglichen" status list. Empty when nothing is linked / parsable.
	linkedStatementLines := func() []core.StatementBooking {
		cache := map[string][]core.StatementBooking{}
		var out []core.StatementBooking
		for _, ref := range core.ParseBuchungRefs(row.BuchungRef) {
			if ref.StatementFilename == "" {
				continue
			}
			lines, ok := cache[ref.StatementFilename]
			if !ok {
				parsed, err := core.ParseStatementBookings(
					filepath.Join(a.statementFolder(row.Bankkonto), ref.StatementFilename))
				if err != nil {
					continue
				}
				cache[ref.StatementFilename] = parsed
				lines = parsed
			}
			found := false
			for _, l := range lines {
				if l.Page == ref.Page && l.LineIdx == ref.LineIdx {
					out = append(out, l)
					found = true
					break
				}
			}
			if found {
				continue
			}
			// Backward-compat: links made before the Qonto parser captured the
			// real page stored Page=0. Re-find by LineIdx alone when it's unique
			// in the file, so old links still highlight at the correct position.
			var cand *core.StatementBooking
			count := 0
			for i := range lines {
				if lines[i].LineIdx == ref.LineIdx {
					cand = &lines[i]
					count++
				}
			}
			if count == 1 {
				out = append(out, *cand)
			}
		}
		return out
	}

	swapPreview := func(path string) {
		currentPreviewPath = path
		// Green frame around the matched booking when showing the linked
		// statement; soft yellow fill for the receipt/attachments.
		hl := hlYellowFill
		isStatement := statementPreviewPath != "" && path == statementPreviewPath
		if isStatement {
			hl = hlGreenFrame
			lines := linkedStatementLines()
			// Prefer exact position bands when the parser captured a position
			// (e.g. Qonto: page + TopPt). Otherwise fall back to framing the
			// matched amount row(s).
			hasPos := false
			for _, l := range lines {
				if l.TopPt > 0 {
					hasPos = true
					break
				}
			}
			if hasPos {
				hl.statementLines = lines
			} else {
				var vals []string
				for _, l := range lines {
					dot := fmt.Sprintf("%.2f", l.Betrag)
					vals = append(vals, dot, strings.ReplaceAll(dot, ".", ","))
				}
				if len(vals) == 0 {
					dot := fmt.Sprintf("%.2f", core.InvoiceEURAmount(row))
					vals = []string{dot, strings.ReplaceAll(dot, ".", ",")}
				}
				hl.values = vals
			}
		}
		content, strip := renderPreviewContent(path, meta, hl)
		preview.Objects = []fyne.CanvasObject{content}
		preview.Refresh()
		previewStrip = strip
		// Vision highlight only for the receipt/attachments (scanned PDFs); the
		// statement is a text-layer PDF handled by the green text-match above.
		if !isStatement {
			a.visionHighlight(strip, path, fmt.Sprintf("%.2f", row.Bruttobetrag), hl)
		}
		rebuildSwitcher()
	}

	makeSwitcherBtn := func(label, path string) *widget.Button {
		btn := widget.NewButton(label, func() { swapPreview(path) })
		if currentPreviewPath == path {
			btn.Importance = widget.HighImportance
		} else {
			btn.Importance = widget.LowImportance
		}
		return btn
	}

	rebuildSwitcher = func() {
		previewSwitcher.RemoveAll()
		previewSwitcher.Add(makeSwitcherBtn("Original", originalPath))
		for i, p := range attPaths {
			previewSwitcher.Add(makeSwitcherBtn(fmt.Sprintf("Anhang %d", i+1), p))
		}
		if statementPreviewPath != "" {
			previewSwitcher.Add(makeSwitcherBtn("Kontoauszug", statementPreviewPath))
		}
		previewSwitcher.Refresh()

		// "Anhang löschen" is only offered while an attachment is the active
		// preview (not Original / Kontoauszug).
		if delAttBtn != nil {
			isAtt := false
			for _, p := range attPaths {
				if p == currentPreviewPath {
					isAtt = true
					break
				}
			}
			if isAtt {
				delAttBtn.Show()
			} else {
				delAttBtn.Hide()
			}
		}
	}

	addAttBtn := widget.NewButtonWithIcon("+ Anhang",
		theme.ContentAddIcon(), func() {
			a.showFilePicker(func(path string) {
				idx, err := a.addAttachmentToInvoice(row, path)
				if err != nil {
					dialog.ShowError(err, editWin)
					return
				}
				row.AnzahlAnhaenge = idx
				row.HatAnhaenge = true
				attPaths = a.invoiceAttachmentPaths(row)
				rebuildSwitcher()
				a.loadInvoices()
				a.showToast(fmt.Sprintf("✓ Anhang %d hinzugefügt", idx))
			})
		})
	addAttBtn.Importance = widget.LowImportance

	// Delete the attachment currently shown in the preview (Original /
	// Kontoauszug cannot be deleted). Confirm first — attachments can be large.
	delAttBtn = widget.NewButtonWithIcon("Anhang löschen", theme.DeleteIcon(), func() {
		target := currentPreviewPath
		attNum := 0
		for i, p := range attPaths {
			if p == target {
				attNum = i + 1
				break
			}
		}
		if attNum == 0 {
			a.showInfo("Anhang löschen",
				"Bitte zuerst oben den zu löschenden Anhang auswählen — Original und Kontoauszug können nicht gelöscht werden.")
			return
		}
		dialog.ShowConfirm("Anhang löschen",
			fmt.Sprintf("Anhang %d wirklich unwiderruflich löschen?\n\nDatei: %s", attNum, filepath.Base(target)),
			func(ok bool) {
				if !ok {
					return
				}
				if err := a.removeAttachmentFromInvoice(row, target); err != nil {
					dialog.ShowError(err, editWin)
					return
				}
				attPaths = a.invoiceAttachmentPaths(row)
				row.HatAnhaenge = len(attPaths) > 0
				currentPreviewPath = originalPath
				swapPreview(originalPath) // also rebuilds the switcher
				a.loadInvoices()
				a.showToast("✓ Anhang gelöscht")
			}, editWin)
	})
	delAttBtn.Importance = widget.LowImportance

	rebuildSwitcher()

	// AfA / Anlagenverzeichnis status: shows whether this acquisition is tracked
	// as a fixed asset (depreciated), with a link to open the register — or a
	// button to create the asset entry from this invoice.
	afaStatus := container.NewVBox()
	var refreshAfaStatus func()
	refreshAfaStatus = func() {
		afaStatus.RemoveAll()
		if asset, ok := core.FindAssetByBeleg(a.assets, row.Belegnummer); ok {
			note := newCopyableLabel(a.bundle, fmt.Sprintf(
				"📊 In AfA erfasst: AK %s, Nutzungsdauer %d J., Anlage %d / AfA %d",
				formatMoney(asset.Anschaffungswert, "EUR", a.settings.DecimalSeparator),
				asset.NutzungsdauerJahre, asset.Konto, asset.AfaKonto))
			openBtn := widget.NewButton("Anlagenverzeichnis öffnen", func() { a.showAnlagen() })
			openBtn.Importance = widget.LowImportance
			afaStatus.Add(container.NewBorder(nil, nil, nil, openBtn, note))
		} else {
			createBtn := widget.NewButton("Als Anlagegut erfassen (AfA)", func() {
				a.createAssetFromInvoice(editWin, row, selectedAccount, refreshAfaStatus)
			})
			createBtn.Importance = widget.LowImportance
			afaStatus.Add(container.NewBorder(nil, nil,
				widget.NewLabel("Nicht im Anlagenverzeichnis."), nil, createBtn))
		}
		afaStatus.Refresh()
	}
	refreshAfaStatus()

	// Bank-reconciliation status: whether this receipt is linked to a statement
	// line, or a button to match it directly here (without the full Belegabgleich).
	abgleichStatus := container.NewVBox()
	var refreshAbgleichStatus func()
	refreshAbgleichStatus = func() {
		abgleichStatus.RemoveAll()
		at := ""
		for _, ba := range a.settings.BankAccounts {
			if ba.Name == row.Bankkonto {
				at = ba.AccountType
			}
		}
		switch {
		case row.BuchungRef == core.CashConfirmedRef:
			abgleichStatus.Add(widget.NewLabel("✓ Bar bestätigt (Kasse)."))
		case row.BuchungRef != "":
			ls := linkedStatementLines()
			if len(ls) == 0 {
				abgleichStatus.Add(newCopyableLabel(a.bundle, "✓ Abgeglichen: "+core.BuchungRefDisplay(row.BuchungRef)))
				break
			}
			heading := fmt.Sprintf("✓ Abgeglichen — %d Auszugszeilen:", len(ls))
			if len(ls) == 1 {
				heading = "✓ Abgeglichen — 1 Auszugszeile:"
			}
			abgleichStatus.Add(widget.NewLabel(heading))
			// Render the statement once; show each linked line + a cropped
			// screenshot of just that booking (date row + detail line[s]).
			var pages []image.Image
			if f := core.FirstBuchungRef(row.BuchungRef).StatementFilename; f != "" {
				pages, _ = core.RenderPDF(filepath.Join(a.statementFolder(row.Bankkonto), f), statementCropDPI)
			}
			for _, l := range ls {
				sign := "−"
				if l.IstGutschrift {
					sign = "+"
				}
				abgleichStatus.Add(newCopyableLabel(a.bundle, fmt.Sprintf("   • %s  EUR %s%s  (S.%d Z.%d)",
					l.Date, sign, formatDecimal(l.Betrag, a.settings.DecimalSeparator), l.Page+1, l.LineIdx)))
				if crop := cropBookingFromPages(pages, l); crop != nil {
					img := canvas.NewImageFromImage(crop)
					img.FillMode = canvas.ImageFillContain
					img.SetMinSize(fyne.NewSize(0, 40))
					abgleichStatus.Add(img)
				}
			}
		case at == core.AccountTypeBank || at == core.AccountTypeCreditCard:
			matchBtn := widget.NewButton("Mit Kontoauszug abgleichen", func() {
				a.matchInvoiceWithStatement(row, editWin, func(updated core.CSVRow) {
					row.BuchungRef = updated.BuchungRef
					recomputeStatementPath()
					rebuildSwitcher()
					refreshAbgleichStatus()
				})
			})
			matchBtn.Importance = widget.LowImportance
			abgleichStatus.Add(container.NewBorder(nil, nil,
				widget.NewLabel("Noch nicht abgeglichen."), nil, matchBtn))
		default:
			abgleichStatus.Add(widget.NewLabel("—"))
		}
		abgleichStatus.Refresh()
	}
	refreshAbgleichStatus()

	belegnrEntry := widget.NewEntry()
	belegnrEntry.SetText(row.Belegnummer)
	belegnrEntry.SetPlaceHolder("z. B. 2026-0007")
	form := container.NewVBox(
		container.NewBorder(nil, nil,
			container.NewHBox(
				newCopyableLabel(a.bundle, "Datei: "+row.Dateiname),
				openBelegBtn, addAttBtn, delAttBtn),
			container.NewHBox(cancelBtn, saveBtn)),
		previewSwitcher,
		warningsLabel,
		section("Identifikation", selectableForm(a.bundle,
			fi("Beleg-Nr.", belegnrEntry),
			fi(a.bundle.T("field.company"), companyEntry),
			fi(a.bundle.T("field.shortdesc"), container.NewBorder(nil, nil, nil, shortDescLabel, shortDescEntry)),
			fi(a.bundle.T("field.invoicenumber"),
				container.NewGridWithColumns(2,
					invoiceNumEntry,
					container.NewBorder(nil, nil,
						widget.NewLabelWithStyle(a.bundle.T("field.vatid"),
							fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						nil, vatIDEntry),
				)),
		)),
		section("Beträge und Datum", selectableForm(a.bundle,
			fi(a.bundle.T("field.invoiceDate"),
				container.NewGridWithColumns(2,
					container.NewBorder(nil, nil, nil, dateCalendarBtn, dateEntry),
					container.NewBorder(nil, nil,
						widget.NewLabelWithStyle(a.bundle.T("field.paymentDate"),
							fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						paymentDateCalendarBtn, paymentDateEntry),
				)),
			fi("MwSt.-Zeilen", ed.Container()),
			fi(a.bundle.T("field.currency"),
				container.NewBorder(nil, nil, nil, nil, currencySelect)),
			fi(a.bundle.T("field.rabatt"), container.NewBorder(nil, nil, nil, paidActualLabel, rabattEntry)),
		)),
		section("Ablage", selectableForm(a.bundle,
			fi(a.bundle.T("field.account"),
				container.NewGridWithColumns(2,
					container.NewBorder(nil, nil, nil, chooseAccountBtn, accountDisplay),
					container.NewBorder(nil, nil,
						widget.NewLabelWithStyle(a.bundle.T("field.bankAccount"),
							fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						addBankBtn, bankAccountSelect),
				)),
			fi("Ablage (Jahr/Monat)", container.NewGridWithColumns(2, yearSelect, monthSelect)),
			fi("", partialPaymentCheck),
			fi("", ausgangsrechnungCheck),
		)),
		// Währungsumrechnung (nur bei Fremdwährung sichtbar).
		currencyConversionContainer,
		section(a.bundle.T("field.comment"), commentEntry),
		section(a.bundle.T("booking.section"), selectableForm(a.bundle,
			fi(a.bundle.T("booking.category"), container.NewBorder(nil, nil, nil,
				container.NewHBox(editBookingBtn, autoBookingBtn), categorySelect)),
			fi("", categoryHint),
			fi("", bookingPrev.container),
			fi("Kontoauszug-Abgleich", abgleichStatus),
			fi("AfA / Anlage", afaStatus),
		)),
		bewirtungBox,
		widget.NewSeparator(),
		newCopyableLabel(a.bundle, a.bundle.T("modal.filenamePreview")),
		filenameEntry,
	)

	// Show/hide the currency-conversion fields and refresh the preview
	// whenever the currency changes.
	currencySelect.OnChanged = func(string) {
		ed.SetCurrency(core.CurrencyCodeFromOption(currencySelect.Selected))
		updateCurrencyConversionVisibility()
		updateFilenamePreview()
		refreshWarnings()
	}
	updateCurrencyConversionVisibility()

	// Chain refreshWarnings into dateEntry (already has onAnyChange) and wire
	// vatIDEntry (no prior OnChanged). Both affect plausibility checks.
	prevDateChanged := dateEntry.OnChanged
	dateEntry.OnChanged = func(s string) {
		if prevDateChanged != nil {
			prevDateChanged(s)
		}
		refreshWarnings()
	}
	vatIDEntry.OnChanged = func(string) { refreshWarnings() }

	// Bewirtung Anlass/Teilnehmer/auf-Beleg affect the missing-details warning.
	anlassEntry.OnChanged = func(string) { refreshWarnings() }
	teilnehmerEntry.OnChanged = func(string) { refreshWarnings() }
	aufBelegCheck.OnChanged = func(bool) { refreshWarnings() }

	// Initial warnings evaluation — once the full form is assembled.
	refreshWarnings()

	scrollForm := container.NewVScroll(form)
	// Keep just a sliver minimum so the user can collapse the form pane
	// nearly to zero — was 420 px which made the HSplit divider feel
	// "stuck" well before the left edge.
	scrollForm.SetMinSize(fyne.NewSize(60, 400))

	preview, previewStrip = buildDocumentPreview(originalPath, meta)
	split := container.NewHSplit(scrollForm, preview)
	splitOffset := a.settings.PreviewSplitOffset
	// Clamp away from the edges so a previously dragged-too-far divider
	// (e.g. 0.97) doesn't make the preview a 1-px stripe on next open.
	if splitOffset < 0.1 || splitOffset > 0.85 {
		splitOffset = 0.33
	}
	split.SetOffset(splitOffset)

	editWin = a.app.NewWindow("Rechnung bearbeiten")
	a.setupModalCtrlScroll(editWin, preview, func() *pdfPreviewStrip { return previewStrip })
	a.addDialogShortcuts(editWin,
		func() {
			if saveBtn.OnTapped != nil {
				saveBtn.OnTapped()
			}
		},
		func() { editWin.Close() },
	)

	saveBtn.OnTapped = func() {
		targetYear := a.currentYear
		fmt.Sscanf(yearSelect.Selected, "%d", &targetYear)
		targetMonth := a.currentMonth
		if len(monthSelect.Selected) >= 2 {
			var m int
			fmt.Sscanf(monthSelect.Selected[:2], "%d", &m)
			if m >= 1 && m <= 12 {
				targetMonth = time.Month(m)
			}
		}
		// Compute the final booking, branching on manual override and invoice type.
		var finalBooking core.Booking
		learn := false
		if manualBooking != nil {
			finalBooking = *manualBooking
		} else {
			var b core.Booking
			var bookable bool
			if ausgangsrechnungCheck.Checked {
				b, bookable, _ = a.computeRevenueBooking(
					ed.Lines(), selectedAccount, bankAccountSelect.Selected)
			} else {
				b, bookable, _ = a.computeInvoiceBooking(
					catKeyByLabel[categorySelect.Selected],
					ed.Lines(), ed.Trinkgeld(), selectedAccount, bankAccountSelect.Selected,
					parseFloat(rabattEntry.Text, a.settings.DecimalSeparator))
				learn = bookable
			}
			if bookable {
				finalBooking = b
			}
		}
		err := a.updateInvoice(
			row,
			originalPath,
			companyEntry.Text,
			shortDescEntry.Text,
			invoiceNumEntry.Text,
			vatIDEntry.Text,
			dateEntry.Text,
			paymentDateEntry.Text,
			ed.Lines(),
			ed.Trinkgeld(),
			core.CurrencyCodeFromOption(currencySelect.Selected),
			selectedAccount,
			bankAccountSelect.Selected,
			partialPaymentCheck.Checked,
			commentEntry.Text,
			strings.TrimSpace(anlassEntry.Text),
			strings.TrimSpace(teilnehmerEntry.Text),
			aufBelegCheck.Checked,
			parseFloat(netEUREntry.Text, a.settings.DecimalSeparator),
			parseFloat(feeEntry.Text, a.settings.DecimalSeparator),
			parseFloat(rabattEntry.Text, a.settings.DecimalSeparator),
			parseDecimal(kursEntry.Text),
			parseDecimal(feePctEntry.Text),
			filenameEntry.Text,
			targetYear,
			targetMonth,
			ausgangsrechnungCheck.Checked,
			finalBooking,
			belegnrEntry.Text,
		)
		if err != nil {
			dialog.ShowInformation(a.bundle.T("error.processing.title"), err.Error(), editWin)
			return
		}
		// Learn the booking template for this company on successful update
		// (only when using the auto path — skip when a manual booking was set).
		if learn && companyEntry.Text != "" {
			_ = a.bookingTemplates.Set(companyEntry.Text, core.BookingTemplate{
				Kategorie:    catKeyByLabel[categorySelect.Selected],
				ExpenseKonto: selectedAccount,
			})
		}
		a.loadInvoices()
		editWin.Close()
	}

	deleteBtn := widget.NewButton("Löschen", func() {
		dialog.ShowConfirm(
			a.bundle.T("table.delete.confirm.title"),
			a.bundle.T("table.delete.confirm.message", row.Dateiname, row.Auftraggeber, row.Bruttobetrag, row.Waehrung),
			func(confirm bool) {
				if confirm {
					a.deleteInvoice(row)
					editWin.Close()
				}
			},
			editWin,
		)
	})
	deleteBtn.Importance = widget.DangerImportance

	buttonBar := container.NewBorder(nil, nil, deleteBtn, nil)

	editWin.SetOnClosed(func() {
		a.settings.PreviewSplitOffset = split.Offset
		if err := a.settingsMgr.Save(a.settings); err != nil {
			a.logger.Warn("Failed to save preview split offset: %v", err)
		}
		if onClose != nil {
			onClose()
		}
	})

	editWin.SetContent(container.NewBorder(nil, buttonBar, nil, nil, split))
	editWin.Resize(a.dialogSize(1500, 850))
	editWin.CenterOnScreen()
	editWin.Show()
}

// updateInvoice updates an existing invoice: it renames/moves the main file
// (possibly into another month's folder) and updates the CSV(s).
func (a *App) updateInvoice(
	originalRow core.CSVRow,
	originalPath string,
	company string,
	shortDesc string,
	invoiceNum string,
	vatID string,
	invoiceDate string,
	paymentDate string,
	taxLines []core.TaxLine,
	trinkgeld float64,
	currency string,
	account int,
	bankAccount string,
	partialPayment bool,
	comment string,
	bewirtungAnlass string,
	bewirtungTeilnehmer string,
	bewirtungAufBeleg bool,
	netEUR float64,
	fee float64,
	rabatt float64,
	wechselkurs float64,
	gebuehrProzent float64,
	filenameInput string,
	targetYear int,
	targetMonth time.Month,
	ausgangsrechnung bool,
	buchung core.Booking,
	belegnummer string,
) error {
	// Attachments are managed live via the _AnhangN switcher in the edit
	// dialog, so an updated invoice keeps whatever attachments it already had.
	willHaveAttachments := originalRow.HatAnhaenge

	newMeta := core.Meta{
		Belegnummer:              strings.TrimSpace(belegnummer), // editable: manual correction/override
		Auftraggeber:             company,
		Verwendungszweck:         shortDesc,
		Rechnungsnummer:          invoiceNum,
		VATID:                    strings.TrimSpace(vatID),
		Rechnungsdatum:           invoiceDate,
		Bezahldatum:              paymentDate,
		TaxLines:                 taxLines,
		Trinkgeld:                trinkgeld,
		BetragNetto:              core.SumNetto(taxLines),
		SteuersatzProzent:        core.PrimarySatz(taxLines),
		SteuersatzBetrag:         core.SumMwSt(taxLines),
		Bruttobetrag:             core.ComputeBrutto(taxLines, trinkgeld),
		Waehrung:                 currency,
		Gegenkonto:               account,
		Bankkonto:                bankAccount,
		Teilzahlung:              partialPayment,
		Kommentar:                comment,
		BewirtungAnlass:          bewirtungAnlass,
		BewirtungTeilnehmer:      bewirtungTeilnehmer,
		BewirtungAngabenAufBeleg: bewirtungAufBeleg,
		BetragNetto_EUR:          netEUR,
		Gebuehr:                  fee,
		Rabatt:                   rabatt,
		Wechselkurs:              wechselkurs,
		GebuehrProzent:           gebuehrProzent,
		HatAnhaenge:              willHaveAttachments,
		Ausgangsrechnung:         ausgangsrechnung,
	}
	parts := strings.Split(invoiceDate, ".")
	if len(parts) == 3 {
		newMeta.Jahr = parts[2]
		newMeta.Monat = parts[1]
	}

	// Filename from the editable field.
	newFilename := core.SanitizeFilename(strings.TrimSpace(filenameInput))
	if newFilename == "" {
		return fmt.Errorf("Bitte einen Dateinamen eingeben.")
	}
	if mainExt := strings.ToLower(filepath.Ext(originalPath)); mainExt != "" {
		newFilename = core.ReplaceExtension(newFilename, mainExt)
	}

	// Target folder (may differ from the source month).
	targetFolder := a.storageManager.GetMonthFolder(targetYear, targetMonth)
	unterordner := a.invoiceSubfolder(bankAccount, ausgangsrechnung)
	if unterordner != "" {
		targetFolder = filepath.Join(targetFolder, unterordner)
	}
	if err := os.MkdirAll(targetFolder, 0755); err != nil {
		return fmt.Errorf("failed to create target folder: %w", err)
	}
	// Source period = the invoice's OWN stored Jahr/Monat (where the DB row and
	// file actually live), NOT the currently viewed month. Using the view month
	// would mis-key the UPDATE/move when editing from a year view or after the
	// receipt was filed in another month.
	srcYearInt := a.currentYear
	if y, err := strconv.Atoi(strings.TrimSpace(originalRow.Jahr)); err == nil && y > 0 {
		srcYearInt = y
	}
	srcMonthInt := int(a.currentMonth)
	if m, err := strconv.Atoi(strings.TrimSpace(originalRow.Monat)); err == nil && m >= 1 && m <= 12 {
		srcMonthInt = m
	}
	sameMonth := targetYear == srcYearInt && int(targetMonth) == srcMonthInt

	// Move the file FIRST — before any DB write — so a move failure
	// leaves the CSVs and the invoice untouched and retryable. The move
	// is robust (copy fallback) via MoveAndRename, which also resolves
	// name collisions and returns the final name.
	finalName := newFilename
	intendedPath := filepath.Join(targetFolder, newFilename)
	if intendedPath != originalPath {
		oldFolder := filepath.Dir(originalPath)
		if _, statErr := os.Stat(originalPath); statErr == nil {
			moved, mvErr := a.storageManager.MoveAndRename(originalPath, targetFolder, newFilename)
			if mvErr != nil {
				return fmt.Errorf("failed to move file: %w", mvErr)
			}
			finalName = moved
		} else if _, tgtErr := os.Stat(intendedPath); tgtErr != nil {
			// Source is gone and the file is not at the target either.
			return fmt.Errorf("Quelldatei nicht gefunden: %s", originalPath)
		}
		// else: source gone but the file is already at intendedPath — a
		// prior attempt moved it; treat the move as already done.

		// Move the numbered "_Anhang<N>" attachment siblings alongside the
		// invoice: they adopt the invoice's final base name and target
		// folder so invoiceAttachmentPaths keeps finding them. Best-effort.
		if err := a.storageManager.MoveInvoiceAttachments(oldFolder, originalRow.Dateiname, targetFolder, finalName); err != nil {
			a.logger.Warn("Failed to move attachments: %v", err)
		}
	}

	// Jahr/Monat in der CSV = Ablage-Periode (wohin die Datei gelegt
	// wird), nicht das Rechnungsdatum. Der Filename behält das
	// Rechnungsdatum via Template.
	newMeta.Jahr = fmt.Sprintf("%04d", targetYear)
	newMeta.Monat = fmt.Sprintf("%02d", int(targetMonth))

	newRow := newMeta.ToCSVRow()
	newRow.Dateiname = finalName
	newRow.HatAnhaenge = originalRow.HatAnhaenge
	newRow.AnzahlAnhaenge = originalRow.AnzahlAnhaenge
	// Preserve the statement reconciliation link (not a form field) — otherwise
	// saving would wipe a BuchungRef set via the single-invoice match.
	newRow.BuchungRef = originalRow.BuchungRef
	newRow.Unterordner = unterordner
	newRow.Buchung = buchung

	// SQLite is the source of truth. Jahr/Monat columns track the filing
	// period (target folder), not the invoice date.
	srcJahr := fmt.Sprintf("%04d", srcYearInt)
	srcMonat := fmt.Sprintf("%02d", srcMonthInt)
	tgtJahr := newMeta.Jahr
	tgtMonat := newMeta.Monat

	// Single UPDATE handles both an in-place edit and a move to another
	// filing month: the row is located by its source (jahr, monat, dateiname)
	// and its jahr/monat columns are rewritten to the target period.
	if err := a.dbRepo.Update(srcJahr, srcMonat, originalRow.Dateiname, newRow); err != nil {
		return fmt.Errorf("failed to update database: %w", err)
	}

	a.logger.Info("Updated invoice in database: %s", finalName)

	// Export affected month(s) to CSV (database is source of truth).
	srcCSV := a.storageManager.GetCSVPath(srcYearInt, time.Month(srcMonthInt))
	if err := a.dbRepo.ExportToCSV(srcJahr, srcMonat, srcCSV, a.csvRepo); err != nil {
		a.logger.Warn("Failed to export source month to CSV after update: %v", err)
	}
	if !sameMonth {
		tgtCSV := a.storageManager.GetCSVPath(targetYear, targetMonth)
		if err := a.dbRepo.ExportToCSV(tgtJahr, tgtMonat, tgtCSV, a.csvRepo); err != nil {
			a.logger.Warn("Failed to export target month to CSV after update: %v", err)
		}
	}

	a.logger.Info("Updated invoice: %s", finalName)
	a.showToast(fmt.Sprintf("✓ Rechnung aktualisiert: %s", finalName))
	return nil
}
