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

	netEntry := widget.NewEntry()
	netEntry.SetText(formatDecimal(meta.BetragNetto, a.settings.DecimalSeparator))
	netEntry.SetPlaceHolder(a.bundle.T("field.net"))

	vatPercentEntry := widget.NewEntry()
	vatPercentEntry.SetText(formatDecimal(meta.SteuersatzProzent, a.settings.DecimalSeparator))
	vatPercentEntry.SetPlaceHolder(a.bundle.T("field.vatPercent"))

	vatAmountEntry := widget.NewEntry()
	vatAmountEntry.SetText(formatDecimal(meta.SteuersatzBetrag, a.settings.DecimalSeparator))
	vatAmountEntry.SetPlaceHolder(a.bundle.T("field.vatAmount"))

	grossEntry := widget.NewEntry()
	grossEntry.SetText(formatDecimal(meta.Bruttobetrag, a.settings.DecimalSeparator))
	grossEntry.SetPlaceHolder(a.bundle.T("field.gross"))

	currencySelect := widget.NewSelect([]string{"EUR", "USD"}, nil)
	if meta.Waehrung != "" {
		currencySelect.SetSelected(meta.Waehrung)
	} else {
		currencySelect.SetSelected(a.settings.CurrencyDefault)
	}

	accountOptions := make([]string, 0, len(a.settings.Accounts))
	accountMap := make(map[string]int)
	for _, acc := range a.settings.Accounts {
		label := fmt.Sprintf("%d - %s", acc.Code, acc.Label)
		accountOptions = append(accountOptions, label)
		accountMap[label] = acc.Code
	}
	accountSelect := widget.NewSelect(accountOptions, nil)
	for label, code := range accountMap {
		if code == meta.Gegenkonto {
			accountSelect.SetSelected(label)
			break
		}
	}

	bankAccountSelect := widget.NewSelect(a.bankAccountOptionList(), nil)
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
	ausgangsrechnungCheck.SetChecked(row.Unterordner == "Ausgangsrechnungen")

	// Comment field (multiline)
	commentEntry := widget.NewMultiLineEntry()
	commentEntry.SetText(meta.Kommentar)
	commentEntry.SetPlaceHolder(a.bundle.T("field.comment"))
	commentEntry.SetMinRowsVisible(3)

	// Currency conversion fields (only relevant for non-default currency).
	netEUREntry := widget.NewEntry()
	if meta.BetragNetto_EUR > 0 {
		netEUREntry.SetText(fmt.Sprintf("%.2f", meta.BetragNetto_EUR))
	}
	netEUREntry.SetPlaceHolder(a.bundle.T("field.net_eur"))

	feeEntry := widget.NewEntry()
	if meta.Gebuehr > 0 {
		feeEntry.SetText(fmt.Sprintf("%.2f", meta.Gebuehr))
	}
	feeEntry.SetPlaceHolder(a.bundle.T("field.fee"))

	// Currency conversion fields are only shown when a non-default
	// currency is selected; this container is (re)populated on demand.
	currencyConversionContainer := container.NewVBox()
	updateCurrencyConversionVisibility := func() {
		if currencySelect.Selected != "" && currencySelect.Selected != a.settings.CurrencyDefault {
			defaultCurrency := a.settings.CurrencyDefault
			feeLabel := fmt.Sprintf("%s (%s)", a.bundle.T("field.fee"), defaultCurrency)
			currencyConversionContainer.Objects = []fyne.CanvasObject{
				selectableForm(a.bundle,
					fi(fmt.Sprintf("%s (%s)", a.bundle.T("field.net_eur"), defaultCurrency), netEUREntry),
					fi(feeLabel, feeEntry),
				),
			}
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
		currentMeta := core.Meta{
			Auftraggeber:      companyEntry.Text,
			Verwendungszweck:  shortDescEntry.Text,
			Rechnungsnummer:   invoiceNumEntry.Text,
			Rechnungsdatum:    dateEntry.Text,
			BetragNetto:       parseFloat(netEntry.Text, a.settings.DecimalSeparator),
			SteuersatzProzent: parseFloat(vatPercentEntry.Text, a.settings.DecimalSeparator),
			SteuersatzBetrag:  parseFloat(vatAmountEntry.Text, a.settings.DecimalSeparator),
			Bruttobetrag:      parseFloat(grossEntry.Text, a.settings.DecimalSeparator),
			Waehrung:          currencySelect.Selected,
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
	netEntry.OnChanged = onAnyChange
	vatPercentEntry.OnChanged = onAnyChange
	vatAmountEntry.OnChanged = onAnyChange
	grossEntry.OnChanged = onAnyChange

	// As soon as two of {Netto, MwSt %, MwSt-Betrag, Brutto} are
	// entered, fill in the others automatically.
	wireAmountAutoCompute(netEntry, vatPercentEntry, vatAmountEntry, grossEntry,
		a.settings.DecimalSeparator)

	updateFilenamePreview()

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

	form := container.NewVBox(
		container.NewBorder(nil, nil,
			container.NewHBox(newCopyableLabel(a.bundle, "Datei: "+row.Dateiname),
				openBelegBtn, addAttBtn),
			container.NewHBox(cancelBtn, saveBtn)),
		previewSwitcher,
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
			fi(a.bundle.T("field.net"),
				container.NewGridWithColumns(3,
					netEntry,
					container.NewBorder(nil, nil,
						widget.NewLabelWithStyle(a.bundle.T("field.vatPercent"),
							fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						nil, vatPercentEntry),
					container.NewBorder(nil, nil,
						widget.NewLabelWithStyle(a.bundle.T("field.vatAmount"),
							fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						nil, vatAmountEntry),
				)),
			fi(a.bundle.T("field.gross"),
				container.NewGridWithColumns(2,
					grossEntry,
					container.NewBorder(nil, nil,
						widget.NewLabelWithStyle(a.bundle.T("field.currency"),
							fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						nil, currencySelect),
				)),
		)),
		section("Ablage", selectableForm(a.bundle,
			fi(a.bundle.T("field.account"),
				container.NewGridWithColumns(2,
					accountSelect,
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
		err := a.updateInvoice(
			row,
			originalPath,
			companyEntry.Text,
			shortDescEntry.Text,
			invoiceNumEntry.Text,
			vatIDEntry.Text,
			dateEntry.Text,
			paymentDateEntry.Text,
			parseFloat(netEntry.Text, a.settings.DecimalSeparator),
			parseFloat(vatPercentEntry.Text, a.settings.DecimalSeparator),
			parseFloat(vatAmountEntry.Text, a.settings.DecimalSeparator),
			parseFloat(grossEntry.Text, a.settings.DecimalSeparator),
			currencySelect.Selected,
			accountMap[accountSelect.Selected],
			bankAccountSelect.Selected,
			partialPaymentCheck.Checked,
			commentEntry.Text,
			parseFloat(netEUREntry.Text, a.settings.DecimalSeparator),
			parseFloat(feeEntry.Text, a.settings.DecimalSeparator),
			filenameEntry.Text,
			targetYear,
			targetMonth,
			ausgangsrechnungCheck.Checked,
		)
		if err != nil {
			dialog.ShowInformation(a.bundle.T("error.processing.title"), err.Error(), editWin)
			return
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
	net float64,
	vatPercent float64,
	vatAmount float64,
	gross float64,
	currency string,
	account int,
	bankAccount string,
	partialPayment bool,
	comment string,
	netEUR float64,
	fee float64,
	filenameInput string,
	targetYear int,
	targetMonth time.Month,
	ausgangsrechnung bool,
) error {
	// Attachments are managed live via the _AnhangN switcher in the edit
	// dialog, so an updated invoice keeps whatever attachments it already had.
	willHaveAttachments := originalRow.HatAnhaenge

	newMeta := core.Meta{
		Auftraggeber:      company,
		Verwendungszweck:  shortDesc,
		Rechnungsnummer:   invoiceNum,
		VATID:             strings.TrimSpace(vatID),
		Rechnungsdatum:    invoiceDate,
		Bezahldatum:       paymentDate,
		BetragNetto:       net,
		SteuersatzProzent: vatPercent,
		SteuersatzBetrag:  vatAmount,
		Bruttobetrag:      gross,
		Waehrung:          currency,
		Gegenkonto:        account,
		Bankkonto:         bankAccount,
		Teilzahlung:       partialPayment,
		Kommentar:         comment,
		BetragNetto_EUR:   netEUR,
		Gebuehr:           fee,
		HatAnhaenge:       willHaveAttachments,
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
		// Move the folder-based attachments alongside the invoice first, so
		// the "<base>-files" folder keeps tracking its invoice. Best-effort:
		// a failure here is logged but does not abort the update.
		if a.storageManager.HasAttachments(originalPath) {
			oldAttachmentsFolder := a.storageManager.GetAttachmentsFolder(originalPath)
			newAttachmentsFolder := a.storageManager.GetAttachmentsFolder(intendedPath)
			a.logger.Info("Renaming attachments folder from %s to %s",
				filepath.Base(oldAttachmentsFolder), filepath.Base(newAttachmentsFolder))
			if err := os.MkdirAll(filepath.Dir(newAttachmentsFolder), 0755); err != nil {
				a.logger.Warn("Failed to prepare attachments folder: %v", err)
			} else if err := os.Rename(oldAttachmentsFolder, newAttachmentsFolder); err != nil {
				a.logger.Warn("Failed to rename attachments folder: %v", err)
			}
		}

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

	// SQLite is the source of truth. Jahr/Monat columns track the filing
	// period (target folder), not the invoice date.
	srcJahr := fmt.Sprintf("%04d", a.currentYear)
	srcMonat := fmt.Sprintf("%02d", int(a.currentMonth))
	tgtJahr := newMeta.Jahr
	tgtMonat := newMeta.Monat

	if sameMonth {
		// In-place update within the same filing month.
		if err := a.dbRepo.Update(srcJahr, srcMonat, originalRow.Dateiname, newRow); err != nil {
			return fmt.Errorf("failed to update database: %w", err)
		}
	} else {
		// Cross-month move: remove from the source month, insert into target.
		if err := a.dbRepo.Delete(srcJahr, srcMonat, originalRow.Dateiname); err != nil {
			return fmt.Errorf("failed to remove invoice from source month: %w", err)
		}
		if _, err := a.dbRepo.Insert(newRow); err != nil {
			return fmt.Errorf("failed to insert invoice into target month: %w", err)
		}
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
