package ui

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

	// USt-IdNr field
	ustIdNrEntry := widget.NewEntry()
	ustIdNrEntry.SetText(meta.UStIdNr)
	ustIdNrEntry.SetPlaceHolder(a.bundle.T("field.ustidnr"))

	dateEntry := widget.NewEntry()
	dateEntry.SetText(meta.Rechnungsdatum)
	dateEntry.SetPlaceHolder(a.bundle.T("field.invoiceDate"))

	// Add calendar button for invoice date
	dateCalendarBtn := widget.NewButton("📅", func() {
		a.showDatePicker(dateEntry.Text, func(selectedDate string) {
			dateEntry.SetText(selectedDate)
			// OnChanged callback will handle updateFilenamePreview
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

	// Container for currency conversion fields (initially hidden)
	currencyConversionContainer := container.NewVBox()

	// Remember mapping checkbox
	rememberCheck := widget.NewCheck(a.bundle.T("checkbox.rememberMap"), nil)
	rememberCheck.SetChecked(a.settings.RememberCompanyAccount)

	// Original filename label (read-only)
	originalLabel := widget.NewLabel(filepath.Base(originalPath))

	// Open original button
	openOriginalBtn := widget.NewButton("Datei öffnen", func() {
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

	// New filename preview label
	newFilenameLabel := widget.NewLabel("")

	// List to track selected attachment files
	selectedAttachments := []string{}
	attachmentsLabel := widget.NewLabel("Keine")

	// Container for attachment list (will be populated when attachments are added)
	attachmentListContainer := container.NewVBox()

	// Create button with dummy callback (will be replaced after dialogWindow is created)
	selectAttachmentsBtn := widget.NewButton("Anhänge hinzufügen", func() {
		// Will be replaced
	})
	selectAttachmentsBtn.Importance = widget.LowImportance
	updateFilenamePreview := func() {
		// Build meta from current form values
		currentMeta := core.Meta{
			Auftraggeber:      companyEntry.Text,
			Verwendungszweck:  shortDescEntry.Text,
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
			newFilenameLabel.SetText("Fehler: " + err.Error())
		} else {
			newFilenameLabel.SetText(filename)
		}
	}

	// Update preview on any field change
	onAnyChange := func(string) { updateFilenamePreview() }
	companyEntry.OnChanged = onAnyChange
	// Special handler for shortDescEntry to handle both character limit and preview
	shortDescEntry.OnChanged = func(s string) {
		if len(s) > 80 {
			shortDescEntry.SetText(s[:80])
		}
		shortDescLabel.SetText(fmt.Sprintf("%d / 80", len(shortDescEntry.Text)))
		updateFilenamePreview()
	}
	invoiceNumEntry.OnChanged = onAnyChange
	dateEntry.OnChanged = onAnyChange
	netEntry.OnChanged = onAnyChange
	vatPercentEntry.OnChanged = onAnyChange
	vatAmountEntry.OnChanged = onAnyChange
	grossEntry.OnChanged = onAnyChange

	// Initial preview update
	updateFilenamePreview()

	// Currency conversion visibility logic
	updateCurrencyConversionVisibility := func() {
		if currencySelect.Selected != "" && currencySelect.Selected != a.settings.CurrencyDefault {
			// Show currency conversion fields (both in default currency EUR)
			defaultCurrency := a.settings.CurrencyDefault
			feeLabel := fmt.Sprintf("%s (%s)", a.bundle.T("field.fee"), defaultCurrency)

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
				widget.NewFormItem(a.bundle.T("field.ustidnr"), ustIdNrEntry),
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

	// Update currency select handler to rebuild form
	currencySelect.OnChanged = func(s string) {
		rebuildForm()
		updateCurrencyConversionVisibility()
		updateFilenamePreview() // Update filename preview when currency changes
	}

	// Initial form build and currency conversion visibility
	rebuildForm()
	updateCurrencyConversionVisibility()

	// Create header section with buttons immediately after labels
	headerSection := container.NewVBox(
		// Original file (left) + New filename (right) on same row
		container.NewBorder(nil, nil,
			// Left side: Original file
			container.NewHBox(
				widget.NewLabel("Originaldatei:"),
				originalLabel,
				openOriginalBtn,
			),
			// Right side: New filename
			container.NewHBox(
				widget.NewLabel("Neuer Dateiname:"),
				newFilenameLabel,
			),
			nil, // Center is empty
		),
		// Attachments row
		container.NewHBox(
			widget.NewLabel("Anhänge:"),
			attachmentsLabel,
			selectAttachmentsBtn,
		),
		// Attachment list (only visible when attachments are added)
		attachmentListContainer,
		widget.NewSeparator(),
	)

	// Form layout
	form := container.NewVBox(
		headerSection,

		mainFormContainer,

		// Currency conversion fields (shown only for non-default currency)
		currencyConversionContainer,

		widget.NewSeparator(),
		widget.NewForm(
			widget.NewFormItem(a.bundle.T("field.comment"), commentEntry),
		),

		widget.NewSeparator(),
		rememberCheck,
	)

	// Scroll container for long forms
	scrollForm := container.NewVScroll(form)

	// Create resizable window
	dialogWindow := a.app.NewWindow(a.bundle.T("modal.title"))

	// Now replace the button callback to use dialogWindow as parent
	selectAttachmentsBtn.OnTapped = func() {
		fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			defer func() { _ = reader.Close() }()

			attachmentPath := reader.URI().Path()
			selectedAttachments = append(selectedAttachments, attachmentPath)

			// Update label and attachment list
			if len(selectedAttachments) == 0 {
				attachmentsLabel.SetText("Keine")
				attachmentListContainer.Objects = []fyne.CanvasObject{}
			} else {
				// Update count label
				if len(selectedAttachments) == 1 {
					attachmentsLabel.SetText(fmt.Sprintf("%d Datei", len(selectedAttachments)))
				} else {
					attachmentsLabel.SetText(fmt.Sprintf("%d Dateien", len(selectedAttachments)))
				}

				// Update attachment list
				attachmentItems := []fyne.CanvasObject{}
				for _, path := range selectedAttachments {
					attachmentItems = append(attachmentItems,
						widget.NewLabel("  • " + filepath.Base(path)))
				}
				attachmentListContainer.Objects = attachmentItems
				attachmentListContainer.Refresh()
			}
		}, dialogWindow)

		fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".pdf", ".jpg", ".jpeg", ".png", ".doc", ".docx", ".xls", ".xlsx", ".txt"}))
		fileDialog.Resize(fyne.NewSize(1000, 700)) // Make it twice as big
		fileDialog.Show()
	}

	// Create buttons (now we can reference dialogWindow)
	saveBtn := widget.NewButton(a.bundle.T("btn.save"), func() {
		// Save the invoice
		err := a.saveInvoice(
			originalPath,
			companyEntry.Text,
			shortDescEntry.Text,
			invoiceNumEntry.Text,
			ustIdNrEntry.Text,
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
			rememberCheck.Checked,
		)

		if err != nil {
			a.showError(
				a.bundle.T("error.processing.title"),
				err.Error(),
			)
		} else {
			// Save dialog size
			a.saveDialogSize(dialogWindow)
			// Close dialog
			dialogWindow.Close()
			// Reload table
			a.loadInvoices()
		}
	})
	saveBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() {
		a.saveDialogSize(dialogWindow)
		dialogWindow.Close()
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
	dialogWindow.Resize(fyne.NewSize(dialogWidth, dialogHeight))
	dialogWindow.CenterOnScreen()

	// Set content with buttons at bottom
	dialogWindow.SetContent(container.NewBorder(
		nil,
		buttonBar,
		nil, nil,
		scrollForm,
	))

	// Set close handler to save size
	dialogWindow.SetOnClosed(func() {
		a.saveDialogSize(dialogWindow)
	})

	dialogWindow.Show()
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
	company string,
	shortDesc string,
	invoiceNum string,
	ustIdNr string,
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
	attachments []string,
	rememberMapping bool,
) error {
	// Build meta
	meta := core.Meta{
		Auftraggeber:      company,
		Verwendungszweck:  shortDesc,
		Rechnungsnummer:   invoiceNum,
		UStIdNr:           ustIdNr,
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
		HatAnhaenge:       len(attachments) > 0,
	}

	// Extract year and month from invoice date (for filename template only)
	// Date is in DD.MM.YYYY format
	invoiceDateParts := strings.Split(invoiceDate, ".")
	if len(invoiceDateParts) == 3 {
		meta.Jahr = invoiceDateParts[2]  // Year is the third part (for template)
		meta.Monat = invoiceDateParts[1] // Month is the second part (for template)
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

	// Override Jahr/Monat with CURRENTLY SELECTED month for database storage
	// This ensures the invoice is stored in the month folder where it physically resides
	meta.Jahr = fmt.Sprintf("%04d", a.currentYear)
	meta.Monat = fmt.Sprintf("%02d", a.currentMonth)

	a.logger.Debug("Saving to folder: %s (current month: %d-%02d)", targetFolder, a.currentYear, a.currentMonth)
	a.logger.Debug("Invoice date: %s, but storing in month: %s-%s", invoiceDate, meta.Jahr, meta.Monat)

	// Prepare CSV row
	newRow := meta.ToCSVRow()
	newRow.Dateiname = filename

	// Check for duplicates in database
	isDuplicate, err := a.dbRepo.IsDuplicate(meta.Jahr, meta.Monat, newRow)
	if err != nil {
		a.logger.Warn("Failed to check duplicate in database: %v", err)
		isDuplicate = false // Continue anyway
	}

	// Helper function to complete the save
	completeSave := func() error {
		// Move and rename file
		finalFilename, err := a.storageManager.MoveAndRename(originalPath, targetFolder, filename)
		if err != nil {
			return fmt.Errorf("failed to move file: %w", err)
		}

		// Update filename in meta
		meta.Dateiname = finalFilename
		newRow.Dateiname = finalFilename

		// Copy attachment files if any
		if len(attachments) > 0 {
			invoicePath := filepath.Join(targetFolder, finalFilename)
			for _, attachmentPath := range attachments {
				copiedName, err := a.storageManager.CopyFileToAttachments(attachmentPath, invoicePath)
				if err != nil {
					a.logger.Warn("Failed to copy attachment %s: %v", filepath.Base(attachmentPath), err)
					// Continue with other attachments even if one fails
				} else {
					a.logger.Debug("Copied attachment: %s", copiedName)
				}
			}
		}

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

		a.logger.Info("Saved invoice: %s", finalFilename)
		if len(attachments) > 0 {
			a.logger.Info("Saved %d attachment(s)", len(attachments))
		}
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

// parseFloat parses a float from a string with flexible decimal separators.
func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", ".")
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// showDatePicker shows a date picker dialog.
func (a *App) showDatePicker(initialDate string, onSelect func(string)) {
	// Parse initial date (DD.MM.YYYY format)
	var day, month, year int
	dateValid := false

	if initialDate != "" {
		parts := strings.Split(initialDate, ".")
		if len(parts) == 3 {
			parsedDay, errDay := strconv.Atoi(strings.TrimSpace(parts[0]))
			parsedMonth, errMonth := strconv.Atoi(strings.TrimSpace(parts[1]))
			parsedYear, errYear := strconv.Atoi(strings.TrimSpace(parts[2]))

			if errDay == nil && errMonth == nil && errYear == nil {
				day = parsedDay
				month = parsedMonth
				year = parsedYear
				dateValid = true
			}
		}
	}

	// Use current date if no initial date or parsing failed
	if !dateValid || day == 0 || month == 0 || year == 0 {
		now := time.Now()
		day = now.Day()
		month = int(now.Month())
		year = now.Year()
	}

	// Create day options (1-31)
	days := make([]string, 31)
	for i := 0; i < 31; i++ {
		days[i] = fmt.Sprintf("%d", i+1)
	}

	// Create month options with German names
	months := []string{
		"1 - Januar", "2 - Februar", "3 - März", "4 - April",
		"5 - Mai", "6 - Juni", "7 - Juli", "8 - August",
		"9 - September", "10 - Oktober", "11 - November", "12 - Dezember",
	}

	// Create year options (current year ± 10 years)
	currentYear := time.Now().Year()
	years := make([]string, 21)
	for i := 0; i < 21; i++ {
		years[i] = fmt.Sprintf("%d", currentYear-10+i)
	}

	// Create select widgets
	daySelect := widget.NewSelect(days, nil)
	monthSelect := widget.NewSelect(months, nil)
	yearSelect := widget.NewSelect(years, nil)

	// Set selected values AFTER creating the widgets
	// Day: always set if valid
	if day >= 1 && day <= 31 {
		daySelect.SetSelected(fmt.Sprintf("%d", day))
	} else {
		// Default to current day if invalid
		daySelect.SetSelected(fmt.Sprintf("%d", time.Now().Day()))
	}

	// Month: always set if valid
	if month >= 1 && month <= 12 {
		monthSelect.SetSelected(months[month-1])
	} else {
		// Default to current month if invalid
		monthSelect.SetSelected(months[int(time.Now().Month())-1])
	}

	// Year: always set if valid and in range
	if year >= currentYear-10 && year <= currentYear+10 {
		yearSelect.SetSelected(fmt.Sprintf("%d", year))
	} else {
		// Default to current year if invalid or out of range
		yearSelect.SetSelected(fmt.Sprintf("%d", currentYear))
	}

	// Create form
	form := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Tag", daySelect),
			widget.NewFormItem("Monat", monthSelect),
			widget.NewFormItem("Jahr", yearSelect),
		),
	)

	// Create dialog
	dateDialog := dialog.NewCustomConfirm(
		"Datum wählen",
		"OK",
		"Abbrechen",
		form,
		func(ok bool) {
			if !ok {
				return
			}

			// Parse selected values
			selectedDay := 1
			selectedMonth := 1
			selectedYear := time.Now().Year()

			if daySelect.Selected != "" {
				fmt.Sscanf(daySelect.Selected, "%d", &selectedDay)
			}

			if monthSelect.Selected != "" {
				// Extract month number from "1 - Januar" format
				fmt.Sscanf(monthSelect.Selected, "%d", &selectedMonth)
			}

			if yearSelect.Selected != "" {
				fmt.Sscanf(yearSelect.Selected, "%d", &selectedYear)
			}

			// Format as DD.MM.YYYY
			formattedDate := fmt.Sprintf("%02d.%02d.%04d", selectedDay, selectedMonth, selectedYear)
			onSelect(formattedDate)
		},
		a.window,
	)

	dateDialog.Resize(fyne.NewSize(350, 250))
	dateDialog.Show()
}
