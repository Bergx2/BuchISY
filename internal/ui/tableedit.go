package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
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
		a.showAccountSearch(selectedAccount, func(n int) {
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
			gesamtEURLabel.SetText(a.bundle.T("field.total.eur", fmt.Sprintf("%.2f", c.GesamtEUR)))
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
				widget.NewForm(
					widget.NewFormItem(a.bundle.T("field.rate"), kursEntry),
					widget.NewFormItem(a.bundle.T("field.fee.percent"), feePctEntry),
					widget.NewFormItem(netEURLabel, netEUREntry),
					widget.NewFormItem(feeLabel, feeEntry),
					widget.NewFormItem(a.bundle.T("field.total.eur.label"), gesamtEURLabel),
				),
			}
			recomputeEUR()
		} else {
			currencyConversionContainer.Objects = []fyne.CanvasObject{}
		}
		currencyConversionContainer.Refresh()
	}

	// Ablagemonat (filing month) — prefilled with the folder the invoice
	// currently lives in.
	yearSelect := widget.NewSelect(generateYearOptions(), nil)
	yearSelect.SetSelected(fmt.Sprintf("%d", a.currentYear))
	monthSelect := widget.NewSelect(generateMonthOptions(a.bundle), nil)
	monthSelect.SetSelected(fmt.Sprintf("%02d - %-12s", int(a.currentMonth),
		a.bundle.T(fmt.Sprintf("month.%02d", int(a.currentMonth)))))

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
	ed = newTaxLinesEditor(a, meta.TaxLines, meta.Trinkgeld, func() {
		updateFilenamePreview()
		if recomputeBooking != nil {
			recomputeBooking()
		}
		recomputeEUR()
	})

	updateFilenamePreview()

	// Booking category: learned template for this company, else "standard".
	category := "standard"
	if tmpl, ok := a.bookingTemplates.Get(meta.Auftraggeber); ok {
		category = tmpl.Kategorie
	}
	catOptions, catKeyByLabel := a.bookingCategoryOptions()
	categorySelect := widget.NewSelect(catOptions, nil)
	categorySelect.SetSelected(a.bookingCategoryLabel(category))

	bookingPrev := newBookingPreview(a)
	var manualBooking *core.Booking
	if row.Buchung.Manuell && len(row.Buchung.Entries) > 0 {
		b := row.Buchung
		manualBooking = &b
	}
	recomputeBooking = func() {
		if manualBooking != nil {
			bookingPrev.set(*manualBooking, manualBooking.Balanced(), a.bundle.T("booking.manual.hint"))
			return
		}
		var b core.Booking
		var bookable bool
		var reason string
		if ausgangsrechnungCheck.Checked {
			b, bookable, reason = a.computeRevenueBooking(
				ed.Lines(), selectedAccount, bankAccountSelect.Selected)
		} else {
			b, bookable, reason = a.computeInvoiceBooking(
				catKeyByLabel[categorySelect.Selected],
				ed.Lines(), ed.Trinkgeld(), selectedAccount, bankAccountSelect.Selected)
		}
		bookingPrev.set(b, bookable, reason)
	}
	categorySelect.OnChanged = func(string) { recomputeBooking() }
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
					ed.Lines(), ed.Trinkgeld(), selectedAccount, bankAccountSelect.Selected)
			}
		}
		a.showBookingEditor(seed, func(edited core.Booking) {
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
	var statementPreviewPath string
	if row.BuchungRef != "" && row.BuchungRef != core.CashConfirmedRef {
		if ref := core.ParseBuchungRef(row.BuchungRef); ref.StatementFilename != "" {
			p := filepath.Join(a.statementFolder(row.Bankkonto), ref.StatementFilename)
			if core.FileExists(p) {
				statementPreviewPath = p
			}
		}
	}

	currentPreviewPath := originalPath
	previewSwitcher := container.NewHBox()
	var rebuildSwitcher func()

	swapPreview := func(path string) {
		currentPreviewPath = path
		// Green frame around the matched booking when showing the linked
		// statement; soft yellow fill for the receipt/attachments.
		hl := hlYellowFill
		if statementPreviewPath != "" && path == statementPreviewPath {
			hl = hlGreenFrame
		}
		content, strip := renderPreviewContent(path, meta, hl)
		preview.Objects = []fyne.CanvasObject{content}
		preview.Refresh()
		previewStrip = strip
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

	rebuildSwitcher()

	belegnrEntry := widget.NewEntry()
	belegnrEntry.SetText(row.Belegnummer)
	belegnrEntry.SetPlaceHolder("z. B. 2026-0007")
	form := container.NewVBox(
		container.NewBorder(nil, nil,
			container.NewHBox(
				newCopyableLabel(a.bundle, "Datei: "+row.Dateiname),
				openBelegBtn, addAttBtn),
			container.NewHBox(cancelBtn, saveBtn)),
		previewSwitcher,
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
		section("Beträge & Datum", selectableForm(a.bundle,
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
			fi("", bookingPrev.container),
		)),
		widget.NewSeparator(),
		newCopyableLabel(a.bundle, a.bundle.T("modal.filenamePreview")),
		filenameEntry,
	)

	// Show/hide the currency-conversion fields and refresh the preview
	// whenever the currency changes.
	currencySelect.OnChanged = func(string) {
		updateCurrencyConversionVisibility()
		updateFilenamePreview()
	}
	updateCurrencyConversionVisibility()

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
					ed.Lines(), ed.Trinkgeld(), selectedAccount, bankAccountSelect.Selected)
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
			parseFloat(netEUREntry.Text, a.settings.DecimalSeparator),
			parseFloat(feeEntry.Text, a.settings.DecimalSeparator),
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
	editWin.Resize(fyne.NewSize(1500, 850))
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
	netEUR float64,
	fee float64,
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
		Belegnummer:       strings.TrimSpace(belegnummer), // editable: manual correction/override
		Auftraggeber:      company,
		Verwendungszweck:  shortDesc,
		Rechnungsnummer:   invoiceNum,
		VATID:             strings.TrimSpace(vatID),
		Rechnungsdatum:    invoiceDate,
		Bezahldatum:       paymentDate,
		TaxLines:          taxLines,
		Trinkgeld:         trinkgeld,
		BetragNetto:       core.SumNetto(taxLines),
		SteuersatzProzent: core.PrimarySatz(taxLines),
		SteuersatzBetrag:  core.SumMwSt(taxLines),
		Bruttobetrag:      core.ComputeBrutto(taxLines, trinkgeld),
		Waehrung:          currency,
		Gegenkonto:        account,
		Bankkonto:         bankAccount,
		Teilzahlung:       partialPayment,
		Kommentar:         comment,
		BetragNetto_EUR:   netEUR,
		Gebuehr:           fee,
		Wechselkurs:       wechselkurs,
		GebuehrProzent:    gebuehrProzent,
		HatAnhaenge:       willHaveAttachments,
		Ausgangsrechnung:  ausgangsrechnung,
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
	sameMonth := targetYear == a.currentYear && targetMonth == a.currentMonth

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
	newRow.Unterordner = unterordner
	newRow.Buchung = buchung

	// SQLite is the source of truth. Jahr/Monat columns track the filing
	// period (target folder), not the invoice date.
	srcJahr := fmt.Sprintf("%04d", a.currentYear)
	srcMonat := fmt.Sprintf("%02d", int(a.currentMonth))
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
	srcCSV := a.storageManager.GetCSVPath(a.currentYear, a.currentMonth)
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
