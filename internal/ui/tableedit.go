package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// showEditDialog shows a dialog to edit an existing invoice.
func (a *App) showEditDialog(row core.CSVRow) {
	// Convert CSVRow back to Meta
	meta := row.ToMeta()

	// Build the current file path
	targetFolder := a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth)
	originalPath := filepath.Join(targetFolder, row.Dateiname)

	// Create form entries (same as showConfirmationModal)
	companyEntry := widget.NewEntry()
	companyEntry.SetText(meta.Firmenname)
	companyEntry.SetPlaceHolder(a.bundle.T("field.company"))

	shortDescEntry := widget.NewEntry()
	shortDescEntry.SetText(meta.Kurzbezeichnung)
	shortDescEntry.SetPlaceHolder(a.bundle.T("field.shortdesc"))
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

	// Add calendar button for invoice date
	dateCalendarBtn := widget.NewButton("📅", func() {
		a.showDatePicker(dateEntry.Text, func(selectedDate string) {
			dateEntry.SetText(selectedDate)
		})
	})
	dateCalendarBtn.Importance = widget.LowImportance

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
	// Pre-select the current account
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
	if meta.Bankkonto != "" {
		bankAccountSelect.SetSelected(meta.Bankkonto)
	} else {
		bankAccountSelect.SetSelected(a.settings.DefaultBankAccount)
	}

	// Payment date entry
	paymentDateEntry := widget.NewEntry()
	paymentDateEntry.SetText(meta.Bezahldatum)
	paymentDateEntry.SetPlaceHolder(a.bundle.T("field.paymentDate"))

	// Add calendar button for payment date
	paymentDateCalendarBtn := widget.NewButton("📅", func() {
		a.showDatePicker(paymentDateEntry.Text, func(selectedDate string) {
			paymentDateEntry.SetText(selectedDate)
		})
	})
	paymentDateCalendarBtn.Importance = widget.LowImportance

	// Partial payment checkbox
	partialPaymentCheck := widget.NewCheck(a.bundle.T("field.partialPayment"), nil)
	partialPaymentCheck.SetChecked(meta.Teilzahlung)

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
			BetragNetto:       parseFloat(netEntry.Text),
			SteuersatzProzent: parseFloat(vatPercentEntry.Text),
			SteuersatzBetrag:  parseFloat(vatAmountEntry.Text),
			Bruttobetrag:      parseFloat(grossEntry.Text),
			Waehrung:          currencySelect.Selected,
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
	dateEntry.OnChanged = onAnyChange
	netEntry.OnChanged = onAnyChange
	vatPercentEntry.OnChanged = onAnyChange
	vatAmountEntry.OnChanged = onAnyChange
	grossEntry.OnChanged = onAnyChange
	currencySelect.OnChanged = onAnyChange

	// Initial preview
	updateFilenamePreview()

	// Form layout
	form := container.NewVBox(
		widget.NewLabel("Datei: "+row.Dateiname),
		widget.NewSeparator(),

		widget.NewForm(
			widget.NewFormItem(a.bundle.T("field.company"), companyEntry),
			widget.NewFormItem(a.bundle.T("field.shortdesc"), container.NewBorder(nil, nil, nil, shortDescLabel, shortDescEntry)),
			widget.NewFormItem(a.bundle.T("field.invoicenumber"), invoiceNumEntry),
			widget.NewFormItem(a.bundle.T("field.invoiceDate"), container.NewBorder(nil, nil, nil, dateCalendarBtn, dateEntry)),
			widget.NewFormItem(a.bundle.T("field.paymentDate"), container.NewBorder(nil, nil, nil, paymentDateCalendarBtn, paymentDateEntry)),
			widget.NewFormItem(a.bundle.T("field.net"), netEntry),
			widget.NewFormItem(a.bundle.T("field.vatPercent"), vatPercentEntry),
			widget.NewFormItem(a.bundle.T("field.vatAmount"), vatAmountEntry),
			widget.NewFormItem(a.bundle.T("field.gross"), grossEntry),
			widget.NewFormItem(a.bundle.T("field.currency"), currencySelect),
			widget.NewFormItem(a.bundle.T("field.account"), accountSelect),
			widget.NewFormItem(a.bundle.T("field.bankAccount"), bankAccountSelect),
			widget.NewFormItem("", partialPaymentCheck),
		),

		widget.NewSeparator(),
		widget.NewLabel(a.bundle.T("modal.filenamePreview")),
		filenamePreview,
	)

	// Scroll container for long forms
	scrollForm := container.NewVScroll(form)
	scrollForm.SetMinSize(fyne.NewSize(750, 625))

	// Buttons
	confirmDialog := dialog.NewCustomConfirm(
		"Rechnung bearbeiten", // Edit title
		a.bundle.T("btn.save"),
		a.bundle.T("btn.cancel"),
		scrollForm,
		func(confirm bool) {
			if !confirm {
				return
			}

			// Update the invoice
			err := a.updateInvoice(
				row,                    // Original row
				originalPath,           // Original file path
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

// updateInvoice updates an existing invoice in the CSV and renames the file if necessary.
func (a *App) updateInvoice(
	originalRow core.CSVRow,
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
) error {
	// Build new meta
	newMeta := core.Meta{
		Firmenname:        company,
		Kurzbezeichnung:   shortDesc,
		Rechnungsnummer:   invoiceNum,
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
	}

	// Extract year and month from invoice date
	parts := strings.Split(invoiceDate, ".")
	if len(parts) == 3 {
		newMeta.Jahr = parts[2]  // Year is the third part
		newMeta.Monat = parts[1] // Month is the second part
	}

	// Generate new filename
	newFilename, err := core.ApplyTemplate(
		a.settings.NamingTemplate,
		newMeta,
		core.TemplateOpts{DecimalSeparator: a.settings.DecimalSeparator},
	)
	if err != nil {
		return fmt.Errorf("failed to generate filename: %w", err)
	}

	targetFolder := a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth)
	newPath := filepath.Join(targetFolder, newFilename)

	// Rename file if filename changed
	if originalRow.Dateiname != newFilename {
		a.logger.Info("Renaming file from %s to %s", originalRow.Dateiname, newFilename)

		// Check if target file already exists (handle collisions)
		finalName := newFilename
		finalPath := newPath
		counter := 2
		for {
			if _, err := os.Stat(finalPath); os.IsNotExist(err) {
				break
			}
			// File exists, try with counter
			ext := filepath.Ext(newFilename)
			base := newFilename[:len(newFilename)-len(ext)]
			finalName = fmt.Sprintf("%s_%d%s", base, counter, ext)
			finalPath = filepath.Join(targetFolder, finalName)
			counter++
		}

		newFilename = finalName
		newPath = finalPath

		// Rename the file
		if err := os.Rename(originalPath, newPath); err != nil {
			return fmt.Errorf("failed to rename file: %w", err)
		}
	}

	// Update CSV
	csvPath := a.storageManager.GetCSVPath(a.currentYear, a.currentMonth)
	existingRows, err := a.csvRepo.Load(csvPath)
	if err != nil {
		return fmt.Errorf("failed to load CSV: %w", err)
	}

	// Find and update the row
	updatedRows := make([]core.CSVRow, 0, len(existingRows))
	found := false
	for _, r := range existingRows {
		if r.Dateiname == originalRow.Dateiname {
			// Update this row
			newRow := newMeta.ToCSVRow()
			newRow.Dateiname = newFilename
			updatedRows = append(updatedRows, newRow)
			found = true
		} else {
			updatedRows = append(updatedRows, r)
		}
	}

	if !found {
		return fmt.Errorf("original row not found in CSV")
	}

	// Rewrite CSV
	if err := a.rewriteCSV(csvPath, updatedRows); err != nil {
		return fmt.Errorf("failed to update CSV: %w", err)
	}

	a.logger.Info("Updated invoice: %s", newFilename)

	// Show success message
	a.showInfo(
		"Gespeichert",
		fmt.Sprintf("Rechnung wurde aktualisiert: %s", newFilename),
	)

	return nil
}
