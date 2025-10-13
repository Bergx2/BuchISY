package ui

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// showConfirmationModal shows the invoice data confirmation modal.
func (a *App) showConfirmationModal(originalPath string, meta core.Meta) {
	// Create form entries
	companyEntry := widget.NewEntry()
	companyEntry.SetText(meta.Firmenname)
	companyEntry.SetPlaceHolder(a.bundle.T("field.company"))

	shortDescEntry := widget.NewEntry()
	shortDescEntry.SetText(meta.Kurzbezeichnung)
	shortDescEntry.SetPlaceHolder(a.bundle.T("field.shortdesc"))
	// Show character count
	shortDescLabel := widget.NewLabel(fmt.Sprintf("%d / 80", len(meta.Kurzbezeichnung)))
	shortDescEntry.OnChanged = func(s string) {
		if len(s) > 80 {
			shortDescEntry.SetText(s[:80])
		}
		shortDescLabel.SetText(fmt.Sprintf("%d / 80", len(shortDescEntry.Text)))
	}

	invoiceNumEntry := widget.NewEntry()
	invoiceNumEntry.SetText(meta.Rechnungsnummer)
	invoiceNumEntry.SetPlaceHolder(a.bundle.T("field.invoicenumber"))

	dateEntry := widget.NewEntry()
	dateEntry.SetText(meta.Rechnungsdatum)
	dateEntry.SetPlaceHolder(a.bundle.T("field.invoiceDate"))

	dateDELabel := widget.NewLabel(meta.DatumDeutsch)
	dateEntry.OnChanged = func(s string) {
		dateDELabel.SetText(core.FormatGermanDate(s))
	}

	netEntry := widget.NewEntry()
	netEntry.SetText(fmt.Sprintf("%.2f", meta.BetragNetto))
	netEntry.SetPlaceHolder(a.bundle.T("field.net"))

	vatPercentEntry := widget.NewEntry()
	vatPercentEntry.SetText(fmt.Sprintf("%.2f", meta.SteuersatzProzent))
	vatPercentEntry.SetPlaceHolder(a.bundle.T("field.vatPercent"))

	vatAmountEntry := widget.NewEntry()
	vatAmountEntry.SetText(fmt.Sprintf("%.2f", meta.SteuersatzBetrag))
	vatAmountEntry.SetPlaceHolder(a.bundle.T("field.vatAmount"))

	grossEntry := widget.NewEntry()
	grossEntry.SetText(fmt.Sprintf("%.2f", meta.Bruttobetrag))
	grossEntry.SetPlaceHolder(a.bundle.T("field.gross"))

	// Currency select
	currencyOptions := []string{"EUR", "USD"}
	currencySelect := widget.NewSelect(currencyOptions, nil)
	if meta.Waehrung != "" {
		currencySelect.SetSelected(meta.Waehrung)
	} else {
		currencySelect.SetSelected(a.settings.CurrencyDefault)
	}

	// Account select
	accountOptions := make([]string, 0, len(a.settings.Accounts))
	accountMap := make(map[string]int)
	for _, acc := range a.settings.Accounts {
		label := fmt.Sprintf("%d - %s", acc.Code, acc.Label)
		accountOptions = append(accountOptions, label)
		accountMap[label] = acc.Code
	}
	accountSelect := widget.NewSelect(accountOptions, nil)
	// Pre-select the suggested account
	for label, code := range accountMap {
		if code == meta.Gegenkonto {
			accountSelect.SetSelected(label)
			break
		}
	}

	// Bank account select
	bankAccountOptions := make([]string, 0, len(a.settings.BankAccounts))
	for _, ba := range a.settings.BankAccounts {
		bankAccountOptions = append(bankAccountOptions, ba.Name)
	}
	bankAccountSelect := widget.NewSelect(bankAccountOptions, nil)
	// Pre-select from meta or default
	if meta.Bankkonto != "" {
		bankAccountSelect.SetSelected(meta.Bankkonto)
	} else {
		bankAccountSelect.SetSelected(a.settings.DefaultBankAccount)
	}

	// Payment date entry
	paymentDateEntry := widget.NewEntry()
	paymentDateEntry.SetText(meta.Bezahldatum)
	paymentDateEntry.SetPlaceHolder(a.bundle.T("field.paymentDate"))

	// Partial payment checkbox
	partialPaymentCheck := widget.NewCheck(a.bundle.T("field.partialPayment"), nil)
	partialPaymentCheck.SetChecked(meta.Teilzahlung)

	// Remember mapping checkbox
	rememberCheck := widget.NewCheck(a.bundle.T("checkbox.rememberMap"), nil)
	rememberCheck.SetChecked(a.settings.RememberCompanyAccount)

	// Original filename
	originalLabel := widget.NewLabel(filepath.Base(originalPath))
	originalLabel.Wrapping = fyne.TextWrapWord
	openOriginalBtn := widget.NewButton(a.bundle.T("modal.openOriginal"), func() {
		fileURI := storage.NewFileURI(originalPath)
		parsed, err := url.Parse(fileURI.String())
		if err != nil {
			a.logger.Warn("Failed to parse file URI: %v", err)
			a.showError(
				a.bundle.T("error.processing.title"),
				a.bundle.T("error.openOriginal", err.Error()),
			)
			return
		}

		if err := a.app.OpenURL(parsed); err != nil {
			a.logger.Warn("Failed to open original PDF: %v", err)
			a.showError(
				a.bundle.T("error.processing.title"),
				a.bundle.T("error.openOriginal", err.Error()),
			)
		}
	})
	openOriginalBtn.Importance = widget.LowImportance

	// Filename preview
	filenamePreview := widget.NewLabel("")
	filenamePreview.Wrapping = fyne.TextWrapBreak
	updateFilenamePreview := func() {
		// Build meta from current form values
		currentMeta := core.Meta{
			Firmenname:        companyEntry.Text,
			Kurzbezeichnung:   shortDescEntry.Text,
			Rechnungsnummer:   invoiceNumEntry.Text,
			Rechnungsdatum:    dateEntry.Text,
			DatumDeutsch:      core.FormatGermanDate(dateEntry.Text),
			BetragNetto:       parseFloat(netEntry.Text),
			SteuersatzProzent: parseFloat(vatPercentEntry.Text),
			SteuersatzBetrag:  parseFloat(vatAmountEntry.Text),
			Bruttobetrag:      parseFloat(grossEntry.Text),
			Waehrung:          currencySelect.Selected,
		}
		// Extract year and month
		parts := strings.Split(dateEntry.Text, "-")
		if len(parts) == 3 {
			currentMeta.Jahr = parts[0]
			currentMeta.Monat = parts[1]
		}

		// Apply template
		filename, err := core.ApplyTemplate(
			a.settings.NamingTemplate,
			currentMeta,
			core.TemplateOpts{DecimalSeparator: a.settings.DecimalSeparator},
		)
		if err != nil {
			filenamePreview.SetText("Fehler: " + err.Error())
		} else {
			filenamePreview.SetText(filename)
		}
	}

	// Update preview on any field change
	onAnyChange := func(string) { updateFilenamePreview() }
	companyEntry.OnChanged = onAnyChange
	shortDescEntry.OnChanged = onAnyChange
	invoiceNumEntry.OnChanged = onAnyChange
	dateEntry.OnChanged = func(s string) {
		dateDELabel.SetText(core.FormatGermanDate(s))
		updateFilenamePreview()
	}
	netEntry.OnChanged = onAnyChange
	vatPercentEntry.OnChanged = onAnyChange
	vatAmountEntry.OnChanged = onAnyChange
	grossEntry.OnChanged = onAnyChange
	currencySelect.OnChanged = onAnyChange

	// Initial preview - call after all widgets are set up
	updateFilenamePreview()

	// Form layout
	form := container.NewVBox(
		widget.NewLabel(a.bundle.T("modal.originalFile")),
		container.NewBorder(nil, nil, nil, openOriginalBtn, originalLabel),
		widget.NewSeparator(),

		widget.NewForm(
			widget.NewFormItem(a.bundle.T("field.company"), companyEntry),
			widget.NewFormItem(a.bundle.T("field.shortdesc"), container.NewBorder(nil, nil, nil, shortDescLabel, shortDescEntry)),
			widget.NewFormItem(a.bundle.T("field.invoicenumber"), invoiceNumEntry),
			widget.NewFormItem(a.bundle.T("field.invoiceDate"), dateEntry),
			widget.NewFormItem(a.bundle.T("field.dateDE"), dateDELabel),
			widget.NewFormItem(a.bundle.T("field.paymentDate"), paymentDateEntry),
			widget.NewFormItem(a.bundle.T("field.net"), netEntry),
			widget.NewFormItem(a.bundle.T("field.vatPercent"), vatPercentEntry),
			widget.NewFormItem(a.bundle.T("field.vatAmount"), vatAmountEntry),
			widget.NewFormItem(a.bundle.T("field.gross"), grossEntry),
			widget.NewFormItem(a.bundle.T("field.currency"), currencySelect),
			widget.NewFormItem(a.bundle.T("field.account"), accountSelect),
			widget.NewFormItem(a.bundle.T("field.bankAccount"), bankAccountSelect),
			widget.NewFormItem("", partialPaymentCheck),
		),

		rememberCheck,
		widget.NewSeparator(),
		widget.NewLabel(a.bundle.T("modal.filenamePreview")),
		filenamePreview,
	)

	// Scroll container for long forms
	scrollForm := container.NewVScroll(form)
	scrollForm.SetMinSize(fyne.NewSize(750, 625))

	// Buttons
	confirmDialog := dialog.NewCustomConfirm(
		a.bundle.T("modal.title"),
		a.bundle.T("btn.save"),
		a.bundle.T("btn.cancel"),
		scrollForm,
		func(confirm bool) {
			if !confirm {
				return
			}

			// Save the invoice
			err := a.saveInvoice(
				originalPath,
				companyEntry.Text,
				shortDescEntry.Text,
				invoiceNumEntry.Text,
				dateEntry.Text,
				paymentDateEntry.Text,
				parseFloat(netEntry.Text),
				parseFloat(vatPercentEntry.Text),
				parseFloat(vatAmountEntry.Text),
				parseFloat(grossEntry.Text),
				currencySelect.Selected,
				accountMap[accountSelect.Selected],
				bankAccountSelect.Selected,
				partialPaymentCheck.Checked,
				rememberCheck.Checked,
			)

			if err != nil {
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

	confirmDialog.Show()
}

// saveInvoice saves an invoice to the file system and CSV.
func (a *App) saveInvoice(
	originalPath string,
	company string,
	shortDesc string,
	invoiceNum string,
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
	rememberMapping bool,
) error {
	// Build meta
	meta := core.Meta{
		Firmenname:        company,
		Kurzbezeichnung:   shortDesc,
		Rechnungsnummer:   invoiceNum,
		Rechnungsdatum:    invoiceDate,
		DatumDeutsch:      core.FormatGermanDate(invoiceDate),
		Bezahldatum:       paymentDate,
		BetragNetto:       net,
		SteuersatzProzent: vatPercent,
		SteuersatzBetrag:  vatAmount,
		Bruttobetrag:      gross,
		Waehrung:          currency,
		Gegenkonto:        account,
		Bankkonto:         bankAccount,
		Teilzahlung:       partialPayment,
	}

	// Extract year and month from invoice date (for filename and CSV fields)
	parts := strings.Split(invoiceDate, "-")
	if len(parts) == 3 {
		meta.Jahr = parts[0]
		meta.Monat = parts[1]
	}

	// Generate filename
	filename, err := core.ApplyTemplate(
		a.settings.NamingTemplate,
		meta,
		core.TemplateOpts{DecimalSeparator: a.settings.DecimalSeparator},
	)
	if err != nil {
		return fmt.Errorf("failed to generate filename: %w", err)
	}

	// IMPORTANT: Save to CURRENTLY SELECTED month, not invoice date month
	// This allows organizing invoices by payment month, not invoice date
	targetFolder := a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth)
	csvPath := a.storageManager.GetCSVPath(a.currentYear, a.currentMonth)

	a.logger.Debug("Saving to folder: %s (current month: %d-%02d)", targetFolder, a.currentYear, a.currentMonth)
	a.logger.Debug("Invoice date month: %s-%s", meta.Jahr, meta.Monat)

	// Check for duplicates
	existingRows, _ := a.csvRepo.Load(csvPath)
	newRow := meta.ToCSVRow()
	newRow.Dateiname = filename

	if core.IsDuplicate(existingRows, newRow) {
		// Show confirmation dialog
		confirmed := false
		dialog.ShowConfirm(
			a.bundle.T("error.duplicate"),
			fmt.Sprintf("%s: %s", a.bundle.T("error.duplicate"), filename),
			func(result bool) {
				confirmed = result
			},
			a.window,
		)
		if !confirmed {
			return nil // User cancelled
		}
	}

	// Move and rename file
	finalFilename, err := a.storageManager.MoveAndRename(originalPath, targetFolder, filename)
	if err != nil {
		return fmt.Errorf("failed to move file: %w", err)
	}

	// Update filename in meta
	meta.Dateiname = finalFilename
	newRow.Dateiname = finalFilename

	// Append to CSV
	if err := a.csvRepo.Append(csvPath, newRow); err != nil {
		return fmt.Errorf("failed to append to CSV: %w", err)
	}

	// Remember company mapping if requested
	if rememberMapping && company != "" {
		a.companyMap.Set(company, account)
		if err := a.companyMap.Save(); err != nil {
			a.logger.Warn("Failed to save company mapping: %v", err)
		}
	}

	a.logger.Info("Saved invoice: %s", finalFilename)
	return nil
}

// parseFloat parses a float from a string with flexible decimal separators.
func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", ".")
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
