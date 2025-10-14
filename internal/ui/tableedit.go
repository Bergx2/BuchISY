package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
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

	// Comment field (multiline)
	commentEntry := widget.NewMultiLineEntry()
	commentEntry.SetText(meta.Kommentar)
	commentEntry.SetPlaceHolder(a.bundle.T("field.comment"))
	commentEntry.SetMinRowsVisible(3)

	// Currency conversion fields (shown only for non-default currency)
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

	// Container for currency conversion fields
	currencyConversionContainer := container.NewVBox()

	// List to track newly selected attachment files
	selectedAttachments := []string{}
	attachmentsLabel := widget.NewLabel("")

	// Update attachments label based on existing and new attachments
	updateAttachmentsLabel := func() {
		existingCount := 0
		if a.storageManager.HasAttachments(originalPath) {
			attachments, err := a.storageManager.ListAttachments(originalPath)
			if err == nil {
				existingCount = len(attachments)
			}
		}

		newCount := len(selectedAttachments)
		totalCount := existingCount + newCount

		if totalCount == 0 {
			attachmentsLabel.SetText(a.bundle.T("field.attachments.none"))
		} else if newCount == 0 {
			// Only existing attachments
			if existingCount == 1 {
				attachmentsLabel.SetText(fmt.Sprintf("1 vorhandene Datei"))
			} else {
				attachmentsLabel.SetText(fmt.Sprintf("%d vorhandene Dateien", existingCount))
			}
		} else {
			// Has new attachments to add
			if newCount == 1 {
				attachmentsLabel.SetText(fmt.Sprintf("%d vorhandene, %d neue Datei", existingCount, newCount))
			} else {
				attachmentsLabel.SetText(fmt.Sprintf("%d vorhandene, %d neue Dateien", existingCount, newCount))
			}
		}
	}

	// Button to add more attachments
	selectAttachmentsBtn := widget.NewButton(a.bundle.T("btn.addAttachments"), func() {
		fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			defer reader.Close()

			filepath := reader.URI().Path()
			selectedAttachments = append(selectedAttachments, filepath)
			updateAttachmentsLabel()
		}, a.window)

		fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".pdf", ".jpg", ".jpeg", ".png", ".doc", ".docx", ".xls", ".xlsx", ".txt"}))
		fileDialog.Show()
	})
	selectAttachmentsBtn.Importance = widget.LowImportance

	updateAttachmentsLabel()

	// Open PDF button
	openPDFBtn := widget.NewButton(a.bundle.T("btn.openPDF"), func() {
		a.openFile(originalPath)
	})
	openPDFBtn.Importance = widget.MediumImportance

	// Open attachments folder button (shown only if attachments exist or will be added)
	openAttachmentsBtn := widget.NewButton(a.bundle.T("btn.openAttachments"), func() {
		attachmentsFolder := a.storageManager.GetAttachmentsFolder(originalPath)
		a.openFolder(attachmentsFolder)
	})
	openAttachmentsBtn.Importance = widget.MediumImportance

	// File actions container
	fileActionsContainer := container.NewVBox()
	updateFileActionsVisibility := func() {
		actions := []fyne.CanvasObject{
			container.NewHBox(openPDFBtn),
		}

		// Add open attachments button if attachments exist
		if a.storageManager.HasAttachments(originalPath) {
			actions = append(actions, container.NewHBox(openAttachmentsBtn))
		}

		// Always show the add attachments section
		actions = append(actions,
			widget.NewSeparator(),
			widget.NewLabel(a.bundle.T("field.attachments")),
			container.NewHBox(selectAttachmentsBtn, attachmentsLabel),
		)

		fileActionsContainer.Objects = actions
		fileActionsContainer.Refresh()
	}

	updateFileActionsVisibility()

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

	// Helper function to get currency-aware label
	getCurrencyLabel := func(baseKey string) string {
		currency := currencySelect.Selected
		if currency == "" {
			currency = a.settings.CurrencyDefault
		}
		return fmt.Sprintf("%s (%s)", a.bundle.T(baseKey), currency)
	}

	// Container for the main form - will be rebuilt when currency changes
	mainFormContainer := container.NewVBox()

	// Function to rebuild the form with current currency
	rebuildForm := func() {
		mainFormContainer.Objects = []fyne.CanvasObject{
			widget.NewForm(
				widget.NewFormItem(a.bundle.T("field.company"), companyEntry),
				widget.NewFormItem(a.bundle.T("field.shortdesc"), container.NewBorder(nil, nil, nil, shortDescLabel, shortDescEntry)),
				widget.NewFormItem(a.bundle.T("field.invoicenumber"), invoiceNumEntry),
				widget.NewFormItem(a.bundle.T("field.invoiceDate"), container.NewBorder(nil, nil, nil, dateCalendarBtn, dateEntry)),
				widget.NewFormItem(a.bundle.T("field.paymentDate"), container.NewBorder(nil, nil, nil, paymentDateCalendarBtn, paymentDateEntry)),
				widget.NewFormItem(getCurrencyLabel("field.net"), netEntry),
				widget.NewFormItem(a.bundle.T("field.vatPercent"), vatPercentEntry),
				widget.NewFormItem(getCurrencyLabel("field.vatAmount"), vatAmountEntry),
				widget.NewFormItem(getCurrencyLabel("field.gross"), grossEntry),
				widget.NewFormItem(a.bundle.T("field.currency"), currencySelect),
				widget.NewFormItem(a.bundle.T("field.account"), accountSelect),
				widget.NewFormItem(a.bundle.T("field.bankAccount"), bankAccountSelect),
				widget.NewFormItem("", partialPaymentCheck),
			),
		}
		mainFormContainer.Refresh()
	}

	// Currency conversion visibility logic
	updateCurrencyConversionVisibility := func() {
		if currencySelect.Selected != "" && currencySelect.Selected != a.settings.CurrencyDefault {
			// Show currency conversion fields
			currency := currencySelect.Selected
			if currency == "" {
				currency = a.settings.CurrencyDefault
			}
			feeLabel := fmt.Sprintf("%s (%s)", a.bundle.T("field.fee"), currency)

			currencyConversionContainer.Objects = []fyne.CanvasObject{
				widget.NewForm(
					widget.NewFormItem(a.bundle.T("field.net_eur"), netEUREntry),
					widget.NewFormItem(feeLabel, feeEntry),
				),
			}
		} else {
			// Hide currency conversion fields
			currencyConversionContainer.Objects = []fyne.CanvasObject{}
		}
		currencyConversionContainer.Refresh()
	}

	// Update currency select handler to rebuild form
	currencySelect.OnChanged = func(s string) {
		rebuildForm()
		updateCurrencyConversionVisibility()
		onAnyChange(s)
	}

	// Initial form build and currency conversion visibility
	rebuildForm()
	updateCurrencyConversionVisibility()

	// Form layout
	form := container.NewVBox(
		widget.NewLabel("Datei: "+row.Dateiname),
		widget.NewSeparator(),

		// File actions section
		widget.NewLabel(a.bundle.T("label.fileActions")),
		fileActionsContainer,
		widget.NewSeparator(),

		mainFormContainer,

		// Currency conversion fields (conditional)
		currencyConversionContainer,

		widget.NewSeparator(),
		widget.NewForm(
			widget.NewFormItem(a.bundle.T("field.comment"), commentEntry),
		),

		widget.NewSeparator(),
		widget.NewLabel(a.bundle.T("modal.filenamePreview")),
		filenamePreview,
	)

	// Scroll container for long forms
	scrollForm := container.NewVScroll(form)

	// Create resizable window
	editWindow := a.app.NewWindow("Rechnung bearbeiten") // Edit title

	// Create buttons (now we can reference editWindow)
	saveBtn := widget.NewButton(a.bundle.T("btn.save"), func() {
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
			commentEntry.Text,
			parseFloat(netEUREntry.Text),
			parseFloat(feeEntry.Text),
			selectedAttachments,
		)

		if err != nil {
			a.showError(
				a.bundle.T("error.processing.title"),
				err.Error(),
			)
		} else {
			// Save dialog size
			a.saveDialogSize(editWindow)
			// Close dialog
			editWindow.Close()
			// Reload table
			a.loadInvoices()
		}
	})
	saveBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() {
		a.saveDialogSize(editWindow)
		editWindow.Close()
	})

	buttonBar := container.NewHBox(
		saveBtn,
		cancelBtn,
	)

	// Set size from settings or defaults
	dialogWidth := float32(a.settings.DialogWidth)
	dialogHeight := float32(a.settings.DialogHeight)
	if dialogWidth < 500 {
		dialogWidth = 850
	}
	if dialogHeight < 400 {
		dialogHeight = 700
	}
	editWindow.Resize(fyne.NewSize(dialogWidth, dialogHeight))
	editWindow.CenterOnScreen()

	// Set content with buttons at bottom
	editWindow.SetContent(container.NewBorder(
		nil,
		buttonBar,
		nil, nil,
		scrollForm,
	))

	// Set close handler to save size
	editWindow.SetOnClosed(func() {
		a.saveDialogSize(editWindow)
	})

	editWindow.Show()
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
	comment string,
	netEUR float64,
	fee float64,
	newAttachments []string,
) error {
	// Check if we will have attachments after this update
	willHaveAttachments := originalRow.HatAnhaenge || len(newAttachments) > 0

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
		Kommentar:         comment,
		BetragNetto_EUR:   netEUR,
		Gebuehr:           fee,
		HatAnhaenge:       willHaveAttachments,
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

		// If attachments folder exists, rename it first
		if a.storageManager.HasAttachments(originalPath) {
			oldAttachmentsFolder := a.storageManager.GetAttachmentsFolder(originalPath)
			newAttachmentsFolder := a.storageManager.GetAttachmentsFolder(newPath)

			a.logger.Info("Renaming attachments folder from %s to %s",
				filepath.Base(oldAttachmentsFolder), filepath.Base(newAttachmentsFolder))

			if err := os.Rename(oldAttachmentsFolder, newAttachmentsFolder); err != nil {
				a.logger.Warn("Failed to rename attachments folder: %v", err)
			}
		}

		// Rename the file
		if err := os.Rename(originalPath, newPath); err != nil {
			return fmt.Errorf("failed to rename file: %w", err)
		}

		// Update originalPath to the new path for attachment operations
		originalPath = newPath
	}

	// Copy any new attachment files
	if len(newAttachments) > 0 {
		for _, attachmentPath := range newAttachments {
			copiedName, err := a.storageManager.CopyFileToAttachments(attachmentPath, originalPath)
			if err != nil {
				a.logger.Warn("Failed to copy attachment %s: %v", filepath.Base(attachmentPath), err)
				// Continue with other attachments even if one fails
			} else {
				a.logger.Debug("Copied attachment: %s", copiedName)
			}
		}
		a.logger.Info("Added %d new attachment(s)", len(newAttachments))
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
