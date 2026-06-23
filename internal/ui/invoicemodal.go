package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// bankAccountOptionList returns the names to populate a Zahlungskonto
// dropdown — just the names of every configured BankAccount in order.
func (a *App) bankAccountOptionList() []string {
	seen := make(map[string]bool, len(a.settings.BankAccounts))
	opts := make([]string, 0, len(a.settings.BankAccounts))
	for _, ba := range a.settings.BankAccounts {
		if ba.Name == "" || seen[ba.Name] {
			continue
		}
		opts = append(opts, ba.Name)
		seen[ba.Name] = true
	}
	return opts
}

// preselectBankAccount picks an initial value for a Zahlungskonto
// dropdown: the invoice's existing bank account when present and known,
// otherwise the first available option.
func (a *App) preselectBankAccount(sel *widget.Select, current string) {
	if len(sel.Options) == 0 {
		sel.PlaceHolder = "Noch kein Zahlungskonto — mit + anlegen"
		sel.Refresh()
		a.logger.Warn("Bank account select empty: a.settings.BankAccounts has %d entries.",
			len(a.settings.BankAccounts))
		return
	}
	for _, o := range sel.Options {
		if current != "" && o == current {
			sel.SetSelected(o)
			return
		}
	}
	sel.SetSelected(sel.Options[0])
}

// addBankAccountInline opens a small two-field form to add a new bank
// account without leaving the calling dialog. Saves immediately so the
// new account becomes available everywhere; calls onAdded with the new
// name so the caller can refresh and select it in its dropdown.
func (a *App) addBankAccountInline(parent fyne.Window, onAdded func(name string)) {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("z. B. Sparkasse, Qonto, Kasse")
	ibanEntry := widget.NewEntry()
	ibanEntry.SetPlaceHolder("IBAN (optional)")

	dialog.ShowForm("Neues Zahlungskonto", "Hinzufügen", "Abbrechen",
		[]*widget.FormItem{
			widget.NewFormItem("Name", nameEntry),
			widget.NewFormItem("IBAN", ibanEntry),
		},
		func(ok bool) {
			if !ok {
				return
			}
			name := strings.TrimSpace(nameEntry.Text)
			if name == "" {
				return
			}
			for _, ba := range a.settings.BankAccounts {
				if ba.Name == name {
					dialog.ShowInformation("Konto existiert",
						"Ein Zahlungskonto mit diesem Namen existiert bereits.",
						parent)
					return
				}
			}
			a.settings.BankAccounts = append(a.settings.BankAccounts, core.BankAccount{
				Name:        name,
				IBAN:        strings.TrimSpace(ibanEntry.Text),
				AccountType: core.AccountTypeBank,
			})
			if a.settings.DefaultBankAccount == "" {
				a.settings.DefaultBankAccount = name
			}
			if err := a.settingsMgr.Save(a.settings); err != nil {
				a.logger.Warn("Failed to save new bank account: %v", err)
				dialog.ShowError(err, parent)
				return
			}
			if onAdded != nil {
				onAdded(name)
			}
		},
		parent)
}

// refreshBankAccountSelect re-populates a Zahlungskonto select from
// the current settings and pre-selects `name` (or runs the standard
// preselection if name is empty).
func (a *App) refreshBankAccountSelect(sel *widget.Select, name string) {
	sel.Options = a.bankAccountOptionList()
	sel.PlaceHolder = ""
	if name != "" {
		sel.SetSelected(name)
	} else {
		a.preselectBankAccount(sel, "")
	}
	sel.Refresh()
}

// invoiceSubfolder determines the category subfolder for an invoice from
// the "Ausgangsrechnung" flag and the chosen bank account.
func (a *App) invoiceSubfolder(bankAccount string, ausgangsrechnung bool) string {
	if ausgangsrechnung {
		return "Ausgangsrechnungen"
	}
	for _, ba := range a.settings.BankAccounts {
		if ba.Name == bankAccount {
			switch ba.AccountType {
			case core.AccountTypeCash:
				return "Bar"
			case core.AccountTypePayroll:
				return "Lohnerstattung"
			}
		}
	}
	return ""
}

// showConfirmationModal shows the invoice data confirmation modal.
func (a *App) showConfirmationModal(originalPath string, attachments []string, meta core.Meta, onClose func()) {
	// Forward-declared so the calendar buttons can open the date picker
	// on this window (assigned further down).
	var confirmWin fyne.Window

	// recomputeBooking is forward-declared so closures created before it
	// is assigned (account picker, bank account select) can call it safely
	// via the nil guard.
	var recomputeBooking func()

	// Create form entries
	companyEntry := widget.NewEntry()
	companyEntry.SetText(meta.Auftraggeber)
	companyEntry.SetPlaceHolder(a.bundle.T("field.company"))

	shortDescEntry := widget.NewEntry()
	shortDescEntry.SetText(meta.Verwendungszweck)
	shortDescEntry.SetPlaceHolder(a.bundle.T("field.shortdesc"))
	// Show character count
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

	// Add calendar button for invoice date
	dateCalendarBtn := widget.NewButton("📅", func() {
		a.showDatePicker(confirmWin, dateEntry.Text, func(selectedDate string) {
			dateEntry.SetText(selectedDate)
			// OnChanged callback will handle updateFilenamePreview
		})
	})
	dateCalendarBtn.Importance = widget.LowImportance

	// VAT-lines editor (replaces the four individual net/vat%/vat-amount/gross entries).
	// Seeded with the extracted TaxLines + Trinkgeld; onChange is wired after
	// updateFilenamePreview is defined below.

	// Placeholder for the editor — created after updateFilenamePreview is defined.
	var ed *taxLinesEditor

	// Currency select — full ISO list, EUR/USD/CAD/AUD first
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
	accountManuallyPicked := false
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
			accountManuallyPicked = true
			selectedAccount = n
			updateAccountDisplay()
			if recomputeBooking != nil {
				recomputeBooking()
			}
		})
	})

	// Suggestion chips: shown only when Claude returned account hints.
	suggestionBox := container.NewHBox()
	if len(meta.KontoVorschlaege) > 0 {
		suggestionBox.Add(widget.NewLabel(a.bundle.T("field.suggestions")))
		for _, k := range meta.KontoVorschlaege {
			k := k
			label := fmt.Sprintf("%d", k)
			if acc, ok := a.chart.Find(k); ok {
				label = accountLabel(acc)
			}
			btn := widget.NewButton(label, func() {
				selectedAccount = k
				updateAccountDisplay()
				if recomputeBooking != nil {
					recomputeBooking()
				}
			})
			btn.Importance = widget.LowImportance
			suggestionBox.Add(btn)
		}
	}

	// Bank account select
	bankAccountSelect := widget.NewSelect(a.bankAccountOptionList(), nil)
	bankAccountSelect.OnChanged = func(string) {
		if recomputeBooking != nil {
			recomputeBooking()
		}
	}
	a.preselectBankAccount(bankAccountSelect, meta.Bankkonto)
	addBankBtn := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		a.addBankAccountInline(confirmWin, func(name string) {
			a.refreshBankAccountSelect(bankAccountSelect, name)
		})
	})
	addBankBtn.Importance = widget.LowImportance

	// Payment date entry
	paymentDateEntry := widget.NewEntry()
	paymentDateEntry.SetText(meta.Bezahldatum)
	paymentDateEntry.SetPlaceHolder(a.bundle.T("field.paymentDate"))

	// Add calendar button for payment date
	paymentDateCalendarBtn := widget.NewButton("📅", func() {
		a.showDatePicker(confirmWin, paymentDateEntry.Text, func(selectedDate string) {
			paymentDateEntry.SetText(selectedDate)
		})
	})
	paymentDateCalendarBtn.Importance = widget.LowImportance

	// Partial payment checkbox
	partialPaymentCheck := widget.NewCheck(a.bundle.T("field.partialPayment"), nil)
	partialPaymentCheck.SetChecked(meta.Teilzahlung)

	// Ausgangsrechnung checkbox
	ausgangsrechnungCheck := widget.NewCheck("Ausgangsrechnung", nil)
	ausgangsrechnungCheck.SetChecked(meta.Ausgangsrechnung)

	// Comment field (multiline)
	commentEntry := widget.NewMultiLineEntry()
	commentEntry.SetText(meta.Kommentar)
	commentEntry.SetPlaceHolder(a.bundle.T("field.comment"))
	commentEntry.SetMinRowsVisible(3)

	// Currency conversion fields (shown only for non-default currency)
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

	// Container for currency conversion fields (initially hidden)
	currencyConversionContainer := container.NewVBox()

	// Remember mapping checkbox
	rememberCheck := widget.NewCheck(a.bundle.T("checkbox.rememberMap"), nil)
	rememberCheck.SetChecked(a.settings.RememberCompanyAccount)

	// Ablagemonat (filing month) — prefilled with the currently viewed
	// month, lets the user file the invoice in a different folder than
	// the current selection (e.g. file a Nov invoice under Dec).
	yearSelect := widget.NewSelect(generateYearOptions(), nil)
	yearSelect.SetSelected(fmt.Sprintf("%d", a.currentYear))

	// Pre-allocate the next sequential Belegnummer for the dialog's filing year
	// so it shows in the live filename preview (${Belegnr}) and is stored on save.
	// Read, not reserved, so cancelling the dialog leaves no gap. Keyed on the
	// year prefix, matching the filing-year default; the filing-month dropdown
	// does not change the filename, so the year prefix stays the dialog-open year.
	nextBelegnr, err := a.dbRepo.NextBelegnummer(fmt.Sprintf("%04d", a.currentYear))
	if err != nil {
		a.logger.Warn("Failed to compute next Belegnummer: %v", err)
		nextBelegnr = ""
	}
	monthSelect := widget.NewSelect(generateMonthOptions(a.bundle), nil)
	monthSelect.SetSelected(fmt.Sprintf("%02d - %-12s", int(a.currentMonth),
		a.bundle.T(fmt.Sprintf("month.%02d", int(a.currentMonth)))))

	// Original filename (entry for copy-paste, keep enabled for proper dark mode colors)
	originalEntry := widget.NewEntry()
	originalEntry.SetText(filepath.Base(originalPath))
	originalEntry.MultiLine = false
	// Note: Keeping entry enabled so text is visible in dark mode
	// User can technically edit but it doesn't affect processing
	openOriginalBtn := widget.NewButton(a.bundle.T("modal.openOriginal"), func() {
		a.openFileInOS(originalPath)
	})
	openOriginalBtn.Importance = widget.LowImportance

	// Filename preview
	filenameEntry := widget.NewEntry()
	filenameEdited := false
	suppressFilenameChange := false
	updateFilenamePreview := func() {
		if filenameEdited {
			return
		}
		// Build meta from current form values.
		// ed may still be nil during initial setup; treat Brutto as 0 in that case.
		var brutto, netto, mwstBetrag, mwstProzent float64
		if ed != nil {
			brutto = ed.Brutto()
			netto = core.SumNetto(ed.Lines())
			mwstBetrag = core.SumMwSt(ed.Lines())
			mwstProzent = core.PrimarySatz(ed.Lines())
		}
		currentMeta := core.Meta{
			Belegnummer:       nextBelegnr,
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
		// Extract year and month from DD.MM.YYYY format
		parts := strings.Split(dateEntry.Text, ".")
		if len(parts) == 3 {
			currentMeta.Jahr = parts[2]  // Year is the third part
			currentMeta.Monat = parts[1] // Month is the second part
		}

		// Apply template
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

	// Duplicate warning banner – hidden until checkDuplicate() reveals it.
	dupBanner := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	dupBanner.Importance = widget.WarningImportance
	dupBanner.Hide()

	// checkDuplicate runs a single indexed query and shows/hides dupBanner.
	var checkDuplicate func()
	checkDuplicate = func() {
		if a.dbRepo == nil {
			return
		}
		row := core.CSVRow{
			Auftraggeber:    companyEntry.Text,
			Rechnungsnummer: invoiceNumEntry.Text,
		}
		label, found, _ := a.dbRepo.FindDuplicate(row)
		if found {
			dupBanner.SetText("⚠ Mögliche Dublette von " + label)
			dupBanner.Show()
		} else {
			dupBanner.Hide()
		}
	}

	// Update preview on any field change
	companyEntry.OnChanged = func(s string) {
		updateFilenamePreview()
		checkDuplicate()
	}
	// Special handler for shortDescEntry to handle both character limit and preview
	shortDescEntry.OnChanged = func(s string) {
		if len(s) > 80 {
			shortDescEntry.SetText(s[:80])
		}
		shortDescLabel.SetText(fmt.Sprintf("%d / 80", len(shortDescEntry.Text)))
		updateFilenamePreview()
	}
	invoiceNumEntry.OnChanged = func(s string) {
		updateFilenamePreview()
		checkDuplicate()
	}
	dateEntry.OnChanged = func(s string) { updateFilenamePreview() }

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
	// suggestErloeskonto sets the Gegenkonto to the appropriate revenue account
	// when the Ausgangsrechnung box is checked — only when the user has not
	// already manually picked an account for this dialog session.
	suggestErloeskonto := func() {
		if !ausgangsrechnungCheck.Checked || accountManuallyPicked {
			return
		}
		if k, ok := a.bookingRules.ErloesKonto(meta.VATID, core.SumMwSt(ed.Lines())); ok {
			selectedAccount = k
			updateAccountDisplay()
			if recomputeBooking != nil {
				recomputeBooking()
			}
		}
	}

	categorySelect.OnChanged = func(string) { recomputeBooking() }
	ausgangsrechnungCheck.OnChanged = func(checked bool) {
		if checked {
			suggestErloeskonto()
		}
		recomputeBooking()
	}

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

	// Initial preview - call after all widgets are set up
	updateFilenamePreview()
	recomputeBooking()
	// If the invoice was already classified as an Ausgangsrechnung by the
	// extractor, pre-suggest the matching revenue account (unless the user
	// already has a manually chosen account from a prior session).
	suggestErloeskonto()

	// Attachments — mutable copy of the initial list so the "+ Anhang"-
	// button can append more sources before the user saves. saveBtn's
	// closure reads this variable by reference, so additions are picked
	// up at save time.
	dynamicAttachments := append([]string(nil), attachments...)

	// Preview pane + currently shown strip. Built below; declared up
	// here so the attachments switcher closure can capture them.
	var preview *fyne.Container
	var previewStrip *pdfPreviewStrip

	// Preview switcher: [Original] [Anhang 1] [Anhang 2] …
	currentPreviewPath := originalPath
	previewSwitcher := container.NewHBox()
	var rebuildSwitcher func()

	swapPreview := func(path string) {
		currentPreviewPath = path
		content, strip := renderPreviewContent(path, meta)
		preview.Objects = []fyne.CanvasObject{content}
		preview.Refresh()
		previewStrip = strip
		rebuildSwitcher()
	}

	rebuildSwitcher = func() {
		previewSwitcher.RemoveAll()
		makeBtn := func(label, path string) *widget.Button {
			b := widget.NewButton(label, func() { swapPreview(path) })
			if currentPreviewPath == path {
				b.Importance = widget.HighImportance
			} else {
				b.Importance = widget.LowImportance
			}
			return b
		}
		previewSwitcher.Add(makeBtn("Original", originalPath))
		for i, p := range dynamicAttachments {
			previewSwitcher.Add(makeBtn(fmt.Sprintf("Anhang %d", i+1), p))
		}
		previewSwitcher.Refresh()
	}

	addAttBtn := widget.NewButtonWithIcon("+ Anhang",
		theme.ContentAddIcon(), func() {
			a.showFilePicker(func(path string) {
				dynamicAttachments = append(dynamicAttachments, path)
				rebuildSwitcher()
			})
		})
	addAttBtn.Importance = widget.LowImportance

	rebuildSwitcher()

	// Form layout
	belegnrText := "Beleg-Nr. —"
	if nextBelegnr != "" {
		belegnrText = "Beleg-Nr. " + nextBelegnr
	}
	// Run the initial duplicate check now that all entry widgets exist.
	checkDuplicate()

	formItems := []fyne.CanvasObject{
		widget.NewLabelWithStyle(belegnrText, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		dupBanner,
		newCopyableLabel(a.bundle, a.bundle.T("modal.originalFile")),
		container.NewBorder(nil, nil, nil,
			container.NewHBox(addAttBtn, openOriginalBtn), originalEntry),
		previewSwitcher,
	}
	formItems = append(formItems,
		widget.NewSeparator(),
		section("Identifikation", selectableForm(a.bundle,
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
			fi("", suggestionBox),
			fi("Ablage (Jahr/Monat)", container.NewGridWithColumns(2, yearSelect, monthSelect)),
			fi("", partialPaymentCheck),
			fi("", ausgangsrechnungCheck),
		)),
		// Currency conversion fields (shown only for non-default currency).
		currencyConversionContainer,
		section(a.bundle.T("field.comment"), commentEntry),
		section(a.bundle.T("booking.section"), selectableForm(a.bundle,
			fi(a.bundle.T("booking.category"), container.NewBorder(nil, nil, nil,
				container.NewHBox(editBookingBtn, autoBookingBtn), categorySelect)),
			fi("", bookingPrev.container),
		)),
		rememberCheck,
		widget.NewSeparator(),
		newCopyableLabel(a.bundle, a.bundle.T("modal.filenamePreview")),
		filenameEntry,
	)

	// Currency conversion visibility logic: show kurs/fee%/net-EUR/fee fields
	// only when the chosen currency differs from the default currency.
	// Compare by CODE (not the full "CODE — Name" option string).
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

	// Rebuild the conversion fields and refresh the filename preview when
	// the currency changes.
	currencySelect.OnChanged = func(string) {
		updateCurrencyConversionVisibility()
		updateFilenamePreview()
	}
	updateCurrencyConversionVisibility()

	cancelBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() {
		confirmWin.Close()
	})
	saveBtn := widget.NewButton(a.bundle.T("btn.save"), nil)
	saveBtn.Importance = widget.HighImportance

	form := container.NewVBox(append(
		[]fyne.CanvasObject{
			container.NewBorder(nil, nil, nil, container.NewHBox(cancelBtn, saveBtn)),
			widget.NewSeparator(),
		},
		formItems...,
	)...)

	// Scroll container for long forms
	scrollForm := container.NewVScroll(form)
	// Keep just a sliver minimum so the user can collapse the form pane
	// nearly to zero — was 420 px which made the HSplit divider feel
	// "stuck" well before the left edge.
	scrollForm.SetMinSize(fyne.NewSize(60, 400))

	// Separate, user-resizable window (a Fyne dialog cannot be drag-resized).
	confirmWin = a.app.NewWindow(a.bundle.T("modal.title"))

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

		doSave := func() {
			err := a.saveInvoice(
				originalPath,
				dynamicAttachments,
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
				rememberCheck.Checked,
				filenameEntry.Text,
				ausgangsrechnungCheck.Checked,
				targetYear,
				targetMonth,
				finalBooking,
				nextBelegnr,
			)
			if err != nil {
				// Keep the window open so the user can correct the data.
				dialog.ShowInformation(a.bundle.T("error.processing.title"), err.Error(), confirmWin)
				return
			}
			// Learn the booking template for this company on successful save
			// (only when using the auto path — skip when a manual booking was set).
			if learn && companyEntry.Text != "" {
				_ = a.bookingTemplates.Set(companyEntry.Text, core.BookingTemplate{
					Kategorie:    catKeyByLabel[categorySelect.Selected],
					ExpenseKonto: selectedAccount,
				})
			}
			a.loadInvoices()
			confirmWin.Close()
		}

		warnings := core.InvoiceWarnings(core.CSVRow{
			BetragNetto:      core.SumNetto(ed.Lines()),
			SteuersatzBetrag: core.SumMwSt(ed.Lines()),
			Bruttobetrag:     ed.Brutto(),
			Trinkgeld:        ed.Trinkgeld(),
			Gegenkonto:       selectedAccount,
			Waehrung:         core.CurrencyCodeFromOption(currencySelect.Selected),
			Wechselkurs:      parseDecimal(kursEntry.Text),
			Rechnungsdatum:   dateEntry.Text,
			VATID:            vatIDEntry.Text,
			Ausgangsrechnung: ausgangsrechnungCheck.Checked,
		})
		if len(warnings) > 0 {
			msg := a.bundle.T("warnings.intro") + "\n• " + strings.Join(warnings, "\n• ")
			dialog.NewConfirm(a.bundle.T("warnings.title"), msg, func(ok bool) {
				if ok {
					doSave()
				}
			}, confirmWin).Show()
			return
		}
		doSave()
	}

	preview, previewStrip = buildDocumentPreview(originalPath, meta)
	a.setupModalCtrlScroll(confirmWin, preview, func() *pdfPreviewStrip { return previewStrip })
	a.addDialogShortcuts(confirmWin,
		func() {
			if saveBtn.OnTapped != nil {
				saveBtn.OnTapped()
			}
		},
		func() { confirmWin.Close() },
	)
	split := container.NewHSplit(scrollForm, preview)
	splitOffset := a.settings.PreviewSplitOffset
	// Clamp away from the edges so a previously dragged-too-far divider
	// (e.g. 0.97) doesn't make the preview a 1-px stripe on next open.
	if splitOffset < 0.1 || splitOffset > 0.85 {
		splitOffset = 0.33 // form ~1/3, preview ~2/3
	}
	split.SetOffset(splitOffset)

	// Remember the divider position the user leaves the window at.
	confirmWin.SetOnClosed(func() {
		a.settings.PreviewSplitOffset = split.Offset
		if err := a.settingsMgr.Save(a.settings); err != nil {
			a.logger.Warn("Failed to save preview split offset: %v", err)
		}
		if onClose != nil {
			onClose()
		}
	})

	confirmWin.SetContent(split)
	confirmWin.Resize(fyne.NewSize(1500, 850))
	confirmWin.CenterOnScreen()
	confirmWin.Show()
}

// saveDialogSize saves the current dialog window size to settings.
func (a *App) saveDialogSize(win fyne.Window) {
	size := win.Canvas().Size()

	a.settings.DialogWidth = int(size.Width)
	a.settings.DialogHeight = int(size.Height)

	if err := a.settingsMgr.Save(a.settings); err != nil {
		a.logger.Warn("Failed to save dialog size: %v", err)
	} else {
		a.logger.Debug("Saved dialog size: %dx%d", a.settings.DialogWidth, a.settings.DialogHeight)
	}
}

// saveInvoice saves an invoice to the file system and CSV.
func (a *App) saveInvoice(
	originalPath string,
	attachments []string,
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
	rememberMapping bool,
	filenameInput string,
	ausgangsrechnung bool,
	targetYear int,
	targetMonth time.Month,
	buchung core.Booking,
	belegnummer string,
) error {
	// Build meta
	meta := core.Meta{
		Belegnummer:       belegnummer,
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
		HatAnhaenge:       len(attachments) > 0,
		Ausgangsrechnung:  ausgangsrechnung,
	}

	// Extract year and month from invoice date (for filename template only)
	// Date is in DD.MM.YYYY format
	invoiceDateParts := strings.Split(invoiceDate, ".")
	if len(invoiceDateParts) == 3 {
		meta.Jahr = invoiceDateParts[2]  // Year is the third part (for template)
		meta.Monat = invoiceDateParts[1] // Month is the second part (for template)
	}

	// Use the filename supplied by the editable field.
	filename := core.SanitizeFilename(strings.TrimSpace(filenameInput))
	if filename == "" {
		return fmt.Errorf("Bitte einen Dateinamen eingeben.")
	}

	// The naming template ends with a literal ".pdf"; use the main file's
	// real extension instead (no-op when the main file is a PDF).
	if mainExt := strings.ToLower(filepath.Ext(originalPath)); mainExt != "" {
		filename = core.ReplaceExtension(filename, mainExt)
	}

	// File into the month the user picked in the dialog (defaults to the
	// currently viewed month). Lets you e.g. file a Nov invoice under Dec.
	targetFolder := a.storageManager.GetMonthFolder(targetYear, targetMonth)
	unterordner := a.invoiceSubfolder(bankAccount, ausgangsrechnung)
	if unterordner != "" {
		targetFolder = filepath.Join(targetFolder, unterordner)
	}
	csvPath := a.storageManager.GetCSVPath(targetYear, targetMonth)

	a.logger.Debug("Saving to folder: %s (filing month: %d-%02d)", targetFolder, targetYear, targetMonth)
	a.logger.Debug("Invoice date month: %s-%s", meta.Jahr, meta.Monat)

	// Jahr/Monat in der CSV sollen die Ablage-Periode spiegeln (wohin
	// die Datei tatsächlich abgelegt wird) — der Dateiname behält über
	// das Template das Rechnungsdatum.
	meta.Jahr = fmt.Sprintf("%04d", targetYear)
	meta.Monat = fmt.Sprintf("%02d", int(targetMonth))

	newRow := meta.ToCSVRow()
	newRow.Dateiname = filename
	newRow.Unterordner = unterordner
	newRow.Buchung = buchung

	// Check for duplicates in database
	isDuplicate, err := a.dbRepo.IsDuplicate(meta.Jahr, meta.Monat, newRow)
	if err != nil {
		a.logger.Warn("Failed to check duplicate in database: %v", err)
		isDuplicate = false // Continue anyway
	}

	// Helper function to complete the save
	completeSave := func() error {
		// File the main invoice file (copy for uploads, move for scans).
		finalFilename, err := a.placeFile(originalPath, targetFolder, filename)
		if err != nil {
			return fmt.Errorf("failed to save file: %w", err)
		}

		// File each attachment as <invoice>_AnhangN<ext>. seq numbers only
		// successfully filed attachments, so the suffixes stay contiguous.
		var failed []string
		seq := 0
		for _, attPath := range attachments {
			attExt := strings.ToLower(filepath.Ext(attPath))
			attName := core.AttachmentName(finalFilename, seq+1, attExt)
			if _, mvErr := a.placeFile(attPath, targetFolder, attName); mvErr != nil {
				a.logger.Warn("Failed to move attachment %s: %v", attPath, mvErr)
				failed = append(failed, filepath.Base(attPath))
				continue
			}
			seq++
		}

		// Update filename + attachment info. Attachments use the
		// "<invoice>_AnhangN" naming model (see invoiceAttachmentPaths /
		// addAttachmentToInvoice), consistent with the rest of the UI; the
		// folder-based attachment model from origin/main is intentionally
		// not used here to avoid double-filing each attachment.
		meta.Dateiname = finalFilename
		newRow.Dateiname = finalFilename
		newRow.HatAnhaenge = seq > 0
		newRow.AnzahlAnhaenge = seq

		// Insert into SQLite database
		_, err = a.dbRepo.Insert(newRow)
		if err != nil {
			return fmt.Errorf("failed to insert into database: %w", err)
		}

		// Export to CSV (database is source of truth)
		err = a.dbRepo.ExportToCSV(meta.Jahr, meta.Monat, csvPath, a.csvRepo)
		if err != nil {
			a.logger.Warn("Failed to export to CSV: %v", err)
			// Don't fail the whole operation if CSV export fails
		}

		// Remember company mapping if requested
		if rememberMapping && company != "" {
			a.companyMap.Set(company, account)
			if err := a.companyMap.Save(); err != nil {
				a.logger.Warn("Failed to save company mapping: %v", err)
			}
		}

		// Attachment move failures are non-fatal: the invoice is filed.
		if len(failed) > 0 {
			a.showError(
				a.bundle.T("error.processing.title"),
				"Folgende Anhänge konnten nicht abgelegt werden: "+strings.Join(failed, ", "),
			)
		}

		a.logger.Info("Saved invoice: %s (%d attachments)", finalFilename, seq)
		return nil
	}

	if isDuplicate {
		// Show confirmation dialog (async)
		dialog.ShowConfirm(
			a.bundle.T("error.duplicate"),
			fmt.Sprintf("%s: %s", a.bundle.T("error.duplicate"), filename),
			func(confirmed bool) {
				if !confirmed {
					a.logger.Info("User cancelled duplicate save")
					return
				}
				// User confirmed, proceed with save
				if err := completeSave(); err != nil {
					a.showError(
						a.bundle.T("error.processing.title"),
						err.Error(),
					)
				} else {
					// Reload table
					a.loadInvoices()
				}
			},
			a.window,
		)
		return nil // Return nil since async dialog will handle the rest
	}

	// Not a duplicate, proceed with save
	return completeSave()
}

// parseFloat parses a user-entered amount, tolerating thousands separators.
// The thousands separator is whichever of "." / "," is not decimalSep.
func parseFloat(s string, decimalSep string) float64 {
	s = strings.TrimSpace(s)
	if decimalSep == "," {
		s = strings.ReplaceAll(s, ".", "")  // strip thousands separators
		s = strings.ReplaceAll(s, ",", ".") // decimal comma -> dot
	} else {
		s = strings.ReplaceAll(s, ",", "") // strip thousands separators
	}
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// isFromScanInbox reports whether path lies inside the configured scan
// inbox folder.
func (a *App) isFromScanInbox(path string) bool {
	inbox := strings.TrimSpace(a.settings.ScanInboxFolder)
	if inbox == "" {
		return false
	}
	absInbox, err1 := filepath.Abs(inbox)
	absPath, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil {
		return false
	}
	return strings.HasPrefix(absPath, absInbox+string(filepath.Separator))
}

// formatAttachmentsLabel renders the comma-separated attachments list
// shown in the confirmation modal. Empty when no attachments are queued.
func formatAttachmentsLabel(paths []string) string {
	if len(paths) == 0 {
		return "Anhänge: keine"
	}
	names := make([]string, len(paths))
	for i, p := range paths {
		names[i] = filepath.Base(p)
	}
	return fmt.Sprintf("Anhänge (%d): %s", len(paths), strings.Join(names, ", "))
}

// placeFile files a source file into targetFolder under newName. A file
// from the scan inbox is moved (original removed); any other file is
// copied (original kept). Returns the final, collision-free name.
func (a *App) placeFile(sourcePath, targetFolder, newName string) (string, error) {
	if a.isFromScanInbox(sourcePath) {
		return a.storageManager.MoveAndRename(sourcePath, targetFolder, newName)
	}
	return a.storageManager.CopyAndRename(sourcePath, targetFolder, newName)
}

// uriToNativePath turns a Fyne file-dialog URI into a native filesystem
// path. On Windows fyne.URI.Path returns "/C:/foo" — the leading slash
// has to go before os.Open/CopyFile can use it.
func uriToNativePath(u fyne.URI) string {
	p := u.Path()
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

// invoiceAttachmentPaths returns the file paths of the row's existing
// attachments by scanning the invoice folder for the
// "<basename>_Anhang<N>.<ext>" sibling files, ordered Anhang1, Anhang2, ….
// The filesystem is the source of truth, so this works regardless of any
// stale count stored in the database/CSV. Empty when the main file can't
// be located or there are no attachments.
func (a *App) invoiceAttachmentPaths(row core.CSVRow) []string {
	invoicePath := a.resolveInvoicePath(row)
	if !core.FileExists(invoicePath) {
		return nil
	}
	return core.AttachmentPathsIn(filepath.Dir(invoicePath), row.Dateiname)
}

// addAttachmentToInvoice files a new attachment next to the invoice's main
// file as "<base>_Anhang<N>.<ext>" (next contiguous index, derived from the
// existing sibling files) and marks the invoice as having attachments in the
// database. Returns the new attachment's 1-based index so callers can update
// their UI.
func (a *App) addAttachmentToInvoice(row core.CSVRow, sourcePath string) (int, error) {
	invoicePath := a.resolveInvoicePath(row)
	if !core.FileExists(invoicePath) {
		return 0, fmt.Errorf("Rechnungsdatei nicht gefunden: %s", row.Dateiname)
	}
	attachmentFolder := filepath.Dir(invoicePath)
	monthFolder := attachmentFolder
	if row.Unterordner != "" {
		monthFolder = filepath.Dir(attachmentFolder)
	}

	nextSeq := core.CountAttachmentsIn(attachmentFolder, row.Dateiname) + 1
	attExt := strings.ToLower(filepath.Ext(sourcePath))
	attName := core.AttachmentName(row.Dateiname, nextSeq, attExt)
	if _, err := a.placeFile(sourcePath, attachmentFolder, attName); err != nil {
		return 0, fmt.Errorf("Anhang konnte nicht abgelegt werden: %w", err)
	}

	// The exact count is derived from the filesystem at load time; here we
	// only need the database flag to reflect that attachments now exist.
	row.HatAnhaenge = true
	if err := a.dbRepo.Update(row.Jahr, row.Monat, row.Dateiname, row); err != nil {
		return 0, fmt.Errorf("Datenbank-Aktualisierung fehlgeschlagen: %w", err)
	}
	csvPath := filepath.Join(monthFolder, "invoices.csv")
	if err := a.dbRepo.ExportToCSV(row.Jahr, row.Monat, csvPath, a.csvRepo); err != nil {
		a.logger.Warn("CSV-Export nach Anhang fehlgeschlagen: %v", err)
	}
	return nextSeq, nil
}
