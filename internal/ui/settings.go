package ui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/zalando/go-keyring"

	"github.com/bergx2/buchisy/internal/core"
	"github.com/bergx2/buchisy/internal/logging"
)

// persistBankAccounts writes the given bank account list to disk and
// to the in-memory settings immediately, so a newly added Zahlungskonto
// is usable without waiting for the outer "Speichern" button. Other
// dialog fields stay unsaved until the user clicks Speichern. Also
// (re)creates the per-account storage folder for every entry.
func (a *App) persistBankAccounts(accounts []core.BankAccount) {
	a.settings.BankAccounts = accounts
	if err := a.settingsMgr.Save(a.settings); err != nil {
		a.logger.Warn("Auto-save of bank accounts failed: %v", err)
	}
	a.ensureAccountFolders()
}

// showSettingsView replaces the main window content with a full-page
// settings view containing the four sub-pages (Allgemein, Verarbeitung,
// Konten, Erweitert) as tabs.
func (a *App) showSettingsView() {
	// Storage section
	storageRootEntry := widget.NewEntry()
	storageRootEntry.SetText(a.settings.StorageRoot)

	browseFolderBtn := widget.NewButton("...", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			storageRootEntry.SetText(uri.Path())
		}, a.window)
	})

	scanInboxEntry := widget.NewEntry()
	scanInboxEntry.SetText(a.settings.ScanInboxFolder)
	scanInboxEntry.SetPlaceHolder("leer = aus")

	browseScanInboxBtn := widget.NewButton("...", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			scanInboxEntry.SetText(uri.Path())
		}, a.window)
	})

	useMonthFoldersCheck := widget.NewCheck(
		a.bundle.T("settings.useMonthFolders"),
		nil,
	)
	useMonthFoldersCheck.SetChecked(a.settings.UseMonthSubfolders)

	// Filename template
	templateEntry := widget.NewEntry()
	templateEntry.SetText(a.settings.NamingTemplate)
	templateEntry.SetPlaceHolder(a.bundle.T("settings.template"))

	templateHelp := newCopyableLabel(a.bundle, a.bundle.T("settings.templateHelp"))
	templateHelp.Wrapping = fyne.TextWrapWord

	// Decimal separator
	decimalSelect := widget.NewSelect([]string{",", "."}, nil)
	decimalSelect.SetSelected(a.settings.DecimalSeparator)

	// CSV separator
	csvSeparatorSelect := widget.NewSelect([]string{",", ";", "\\t"}, nil)
	if a.settings.CSVSeparator == "\t" {
		csvSeparatorSelect.SetSelected("\\t")
	} else {
		csvSeparatorSelect.SetSelected(a.settings.CSVSeparator)
	}

	// CSV encoding
	csvEncodingSelect := widget.NewSelect([]string{"ISO-8859-1", "UTF-8"}, nil)
	csvEncodingSelect.SetSelected(a.settings.CSVEncoding)

	// Currency default
	currencyEntry := widget.NewEntry()
	currencyEntry.SetText(a.settings.CurrencyDefault)

	// Own VAT-IDs (comma-separated). Used to exclude the user's own
	// company VAT-IDs from auto-extraction so the extractor returns
	// the SENDER's VAT-ID and not the receiver's.
	ownVATIDEntry := widget.NewEntry()
	ownVATIDEntry.SetPlaceHolder("z. B. DE287472874, DE319686097")
	ownVATIDEntry.SetText(a.settings.OwnVATID)

	// Processing mode
	modeSelect := widget.NewRadioGroup([]string{
		a.bundle.T("settings.mode.claude"),
		a.bundle.T("settings.mode.local"),
	}, nil)
	if a.settings.ProcessingMode == "claude" {
		modeSelect.SetSelected(a.bundle.T("settings.mode.claude"))
	} else {
		modeSelect.SetSelected(a.bundle.T("settings.mode.local"))
	}

	// Model
	modelEntry := widget.NewEntry()
	modelEntry.SetText(a.settings.AnthropicModel)

	// API Key
	apiKeyEntry := widget.NewPasswordEntry()
	// Try to load existing key
	existingKey, err := keyring.Get("BuchISY", a.keyringAccount())
	if err == nil && existingKey != "" {
		apiKeyEntry.SetText(existingKey)
	}

	// Show/hide API key fields based on mode
	apiKeyContainer := container.NewVBox()
	updateAPIKeyVisibility := func() {
		if modeSelect.Selected == a.bundle.T("settings.mode.claude") {
			apiKeyContainer.Objects = []fyne.CanvasObject{
				widget.NewForm(
					widget.NewFormItem(a.bundle.T("settings.model"), modelEntry),
					widget.NewFormItem(a.bundle.T("settings.apiKey"), apiKeyEntry),
				),
			}
		} else {
			apiKeyContainer.Objects = []fyne.CanvasObject{}
		}
		apiKeyContainer.Refresh()
	}
	modeSelect.OnChanged = func(string) { updateAPIKeyVisibility() }
	updateAPIKeyVisibility()

	// Accounts
	defaultAccountEntry := widget.NewEntry()
	defaultAccountEntry.SetText(fmt.Sprintf("%d", a.settings.DefaultAccount))

	// Account list (make a copy to modify)
	tempAccounts := make([]core.Account, len(a.settings.Accounts))
	copy(tempAccounts, a.settings.Accounts)

	// Account list container
	accountsList := container.NewVBox()

	var refreshAccounts func()
	refreshAccounts = func() {
		accountsList.Objects = accountsList.Objects[:0]

		for idx, acc := range tempAccounts {
			currentIdx := idx
			label := widget.NewLabel(fmt.Sprintf("  %d - %s", acc.Code, acc.Label))
			label.Alignment = fyne.TextAlignLeading
			label.Wrapping = fyne.TextWrapOff

			removeBtn := widget.NewButton("Entfernen", func() {
				// Remove this account
				if currentIdx < len(tempAccounts) {
					tempAccounts = append(tempAccounts[:currentIdx], tempAccounts[currentIdx+1:]...)
					refreshAccounts()
				}
			})
			removeBtn.Importance = widget.LowImportance

			row := container.NewBorder(nil, nil, nil, removeBtn, label)
			accountsList.Add(container.NewPadded(row))
		}

		accountsList.Refresh()
	}

	// Add account controls — Code/Bezeichnung stay hidden until the button
	// is clicked once; a further click then adds the account. The fields are
	// narrowed to ~1/3 of the row width.
	newAccountCodeEntry := widget.NewEntry()
	newAccountCodeEntry.SetPlaceHolder(a.bundle.T("settings.accountCode"))

	newAccountLabelEntry := widget.NewEntry()
	newAccountLabelEntry.SetPlaceHolder(a.bundle.T("settings.accountLabel"))

	addAccountFields := container.NewGridWithColumns(3,
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("settings.accountCode")), nil, newAccountCodeEntry),
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("settings.accountLabel")), nil, newAccountLabelEntry),
	)
	addAccountFields.Hide()

	addAccountBtn := widget.NewButton("+ Konto hinzufügen", nil)
	addAccountBtn.OnTapped = func() {
		if !addAccountFields.Visible() {
			addAccountFields.Show()
			return
		}
		code, err := strconv.Atoi(newAccountCodeEntry.Text)
		if err != nil || code <= 0 {
			a.showError("Fehler", "Ungültiger Kontocode. Bitte eine Zahl eingeben.")
			return
		}
		label := newAccountLabelEntry.Text
		if label == "" {
			a.showError("Fehler", "Bitte eine Bezeichnung eingeben.")
			return
		}
		for _, acc := range tempAccounts {
			if acc.Code == code {
				a.showError("Fehler", fmt.Sprintf("Konto mit Code %d existiert bereits.", code))
				return
			}
		}
		if len(tempAccounts) >= 10 {
			a.showError("Fehler", "Maximal 10 Konten erlaubt.")
			return
		}
		tempAccounts = append(tempAccounts, core.Account{
			Code:  code,
			Label: label,
		})
		newAccountCodeEntry.SetText("")
		newAccountLabelEntry.SetText("")
		addAccountFields.Hide()
		refreshAccounts()
	}

	refreshAccounts()

	accountsNote := newCopyableLabel(a.bundle, a.bundle.T("settings.accountsNote"))
	accountsNote.Wrapping = fyne.TextWrapWord

	rememberCompanyCheck := widget.NewCheck(
		a.bundle.T("settings.rememberCompanyAccount"),
		nil,
	)
	rememberCompanyCheck.SetChecked(a.settings.RememberCompanyAccount)

	autoSelectCheck := widget.NewCheck(
		a.bundle.T("settings.autoSelectAccount"),
		nil,
	)
	autoSelectCheck.SetChecked(a.settings.AutoSelectAccount)

	// Bank account list (working copy, edited inline, persisted on save)
	tempBankAccounts := make([]core.BankAccount, len(a.settings.BankAccounts))
	copy(tempBankAccounts, a.settings.BankAccounts)

	bankAccountsList := container.NewVBox()

	// Drag-and-drop reorder for bank accounts (mirrors the column-order
	// list further down). swapBankAccounts performs an in-place adjacent
	// swap; the drag handler accumulates pixel distance and fires the
	// swap once half a row pitch has passed.
	var bankAccountRows []*draggableRow
	swapBankAccounts := func(i, j int) {
		n := len(tempBankAccounts)
		if i < 0 || j < 0 || i >= n || j >= n || i == j {
			return
		}
		tempBankAccounts[i], tempBankAccounts[j] = tempBankAccounts[j], tempBankAccounts[i]
		bankAccountRows[i], bankAccountRows[j] = bankAccountRows[j], bankAccountRows[i]
		bankAccountsList.Objects[i], bankAccountsList.Objects[j] =
			bankAccountsList.Objects[j], bankAccountsList.Objects[i]
		bankAccountRows[i].index = i
		bankAccountRows[j].index = j
		bankAccountsList.Refresh()
	}
	onBankAccountDragEnd := func(row *draggableRow) {
		row.dragAccum = 0
		a.persistBankAccounts(tempBankAccounts)
	}
	onBankAccountDrag := func(row *draggableRow, dy float32) {
		row.dragAccum += dy
		pitch := row.Size().Height
		if idx := row.index; idx+1 < len(bankAccountRows) {
			pitch = bankAccountRows[idx+1].Position().Y - row.Position().Y
		} else if idx > 0 {
			pitch = row.Position().Y - bankAccountRows[idx-1].Position().Y
		}
		if pitch <= 0 {
			return
		}
		for row.dragAccum > pitch/2 && row.index < len(bankAccountRows)-1 {
			swapBankAccounts(row.index, row.index+1)
			row.dragAccum -= pitch
		}
		for row.dragAccum < -pitch/2 && row.index > 0 {
			swapBankAccounts(row.index, row.index-1)
			row.dragAccum += pitch
		}
	}

	var refreshBankAccounts func()
	refreshBankAccounts = func() {
		bankAccountsList.Objects = bankAccountsList.Objects[:0]
		bankAccountRows = bankAccountRows[:0]

		for idx := range tempBankAccounts {
			currentIdx := idx

			nameEntry := widget.NewEntry()
			nameEntry.SetPlaceHolder("Name")
			nameEntry.SetText(tempBankAccounts[currentIdx].Name)
			nameEntry.OnChanged = func(s string) {
				tempBankAccounts[currentIdx].Name = s
			}

			ibanEntry := widget.NewEntry()
			ibanEntry.SetPlaceHolder("IBAN / Konto")
			ibanEntry.SetText(tempBankAccounts[currentIdx].IBAN)
			ibanEntry.OnChanged = func(s string) {
				tempBankAccounts[currentIdx].IBAN = s
			}

			// Settlement-Konto: dropdown of every OTHER configured
			// payment account. Only relevant for credit-card rows; the
			// stored value is the chosen account's Name.
			settlementOptions := make([]string, 0, len(tempBankAccounts))
			for i, ba := range tempBankAccounts {
				if i == currentIdx || ba.Name == "" {
					continue
				}
				settlementOptions = append(settlementOptions, ba.Name)
			}
			settlementSelect := widget.NewSelect(settlementOptions, func(sel string) {
				tempBankAccounts[currentIdx].SettlementAccount = sel
			})
			settlementSelect.PlaceHolder = "Ausgleich über"
			if cur := tempBankAccounts[currentIdx].SettlementAccount; cur != "" {
				settlementSelect.SetSelected(cur)
			}
			if tempBankAccounts[currentIdx].AccountType != core.AccountTypeCreditCard {
				settlementSelect.Disable()
			}

			typeSelect := widget.NewSelect([]string{"Bank", "Kreditkarte", "Barkasse", "Lohnerstattung"}, nil)
			switch tempBankAccounts[currentIdx].AccountType {
			case core.AccountTypeCreditCard:
				typeSelect.SetSelected("Kreditkarte")
			case core.AccountTypeCash:
				typeSelect.SetSelected("Barkasse")
			case core.AccountTypePayroll:
				typeSelect.SetSelected("Lohnerstattung")
			default:
				typeSelect.SetSelected("Bank")
			}

			// SKR04 account picker for this payment account row. Wrap instead of
			// truncating so the full "Nr — Name" (or the placeholder) is readable.
			skr04Display := widget.NewLabel(paymentSKR04Label(a, tempBankAccounts[currentIdx].SKR04Konto))
			skr04Display.Wrapping = fyne.TextWrapWord
			skr04Btn := widget.NewButton(a.bundle.T("settings.payment.skr04"), func() {
				a.showAccountSearch(tempBankAccounts[currentIdx].SKR04Konto, a.window, func(n int) {
					tempBankAccounts[currentIdx].SKR04Konto = n
					skr04Display.SetText(paymentSKR04Label(a, n))
				})
			})
			skr04Box := container.NewBorder(nil, nil, widget.NewLabel("SKR04"), nil,
				container.NewVBox(skr04Display, skr04Btn))

			// Borders for each field column — built once, slotted into a
			// dynamic grid depending on whether "Ausgleich" applies.
			nameBox := container.NewBorder(nil, nil, widget.NewLabel("Name"), nil, nameEntry)
			ibanBox := container.NewBorder(nil, nil, widget.NewLabel("IBAN / Konto"), nil, ibanEntry)
			typeBox := container.NewBorder(nil, nil, widget.NewLabel("Typ"), nil, typeSelect)
			settlementBox := container.NewBorder(nil, nil, widget.NewLabel("Ausgleich"), nil, settlementSelect)
			fields := container.New(layout.NewGridLayoutWithColumns(5))

			showSettlement := func(visible bool) {
				if visible {
					fields.Layout = layout.NewGridLayoutWithColumns(5)
					fields.Objects = []fyne.CanvasObject{nameBox, ibanBox, typeBox, settlementBox, skr04Box}
				} else {
					fields.Layout = layout.NewGridLayoutWithColumns(4)
					fields.Objects = []fyne.CanvasObject{nameBox, ibanBox, typeBox, skr04Box}
				}
				fields.Refresh()
			}
			showSettlement(tempBankAccounts[currentIdx].AccountType == core.AccountTypeCreditCard)

			typeSelect.OnChanged = func(sel string) {
				switch sel {
				case "Kreditkarte":
					tempBankAccounts[currentIdx].AccountType = core.AccountTypeCreditCard
					settlementSelect.Enable()
					showSettlement(true)
				case "Barkasse":
					tempBankAccounts[currentIdx].AccountType = core.AccountTypeCash
					settlementSelect.Disable()
					showSettlement(false)
				case "Lohnerstattung":
					tempBankAccounts[currentIdx].AccountType = core.AccountTypePayroll
					settlementSelect.Disable()
					showSettlement(false)
				default:
					tempBankAccounts[currentIdx].AccountType = core.AccountTypeBank
					settlementSelect.Disable()
					showSettlement(false)
				}
			}

			removeBtn := widget.NewButton("Entfernen", func() {
				if currentIdx < len(tempBankAccounts) {
					tempBankAccounts = append(tempBankAccounts[:currentIdx], tempBankAccounts[currentIdx+1:]...)
					refreshBankAccounts()
					a.persistBankAccounts(tempBankAccounts)
				}
			})
			removeBtn.Importance = widget.LowImportance

			grip := widget.NewLabel("↕")
			content := container.NewPadded(
				container.NewBorder(nil, nil, grip, removeBtn, fields),
			)
			row := newDraggableRow(content, currentIdx)
			row.onDrag = onBankAccountDrag
			row.onDragEnd = onBankAccountDragEnd
			bankAccountRows = append(bankAccountRows, row)
			bankAccountsList.Add(row)
		}

		bankAccountsList.Refresh()
	}

	// Add payment account controls — the input fields stay hidden until the
	// "+ Zahlungskonto hinzufügen" button is clicked; the fields row carries
	// its own "Zahlungskonto anlegen" button (far left) that creates it.
	newBankAccountEntry := widget.NewEntry()
	newBankAccountEntry.SetPlaceHolder("Zahlungskonto Name")

	newBankAccountIBANEntry := widget.NewEntry()
	newBankAccountIBANEntry.SetPlaceHolder("IBAN / Konto")

	createBankAccountBtn := widget.NewButton("Zahlungskonto anlegen", nil)
	createBankAccountBtn.Importance = widget.HighImportance

	addBankFields := container.NewBorder(nil, nil, createBankAccountBtn, nil,
		container.NewGridWithColumns(2,
			container.NewBorder(nil, nil, widget.NewLabel("Zahlungskonto Name"), nil, newBankAccountEntry),
			container.NewBorder(nil, nil, widget.NewLabel("IBAN / Konto"), nil, newBankAccountIBANEntry),
		),
	)
	addBankFields.Hide()

	createBankAccountBtn.OnTapped = func() {
		name := newBankAccountEntry.Text
		if name == "" {
			a.showError("Fehler", "Bitte einen Namen eingeben.")
			return
		}
		for _, ba := range tempBankAccounts {
			if ba.Name == name {
				a.showError("Fehler", fmt.Sprintf("Zahlungskonto '%s' existiert bereits.", name))
				return
			}
		}
		if len(tempBankAccounts) >= 30 {
			a.showError("Fehler", "Maximal 30 Zahlungskonten erlaubt.")
			return
		}
		tempBankAccounts = append(tempBankAccounts, core.BankAccount{
			Name:        name,
			IBAN:        newBankAccountIBANEntry.Text,
			AccountType: core.AccountTypeBank,
		})
		newBankAccountEntry.SetText("")
		newBankAccountIBANEntry.SetText("")
		addBankFields.Hide()
		refreshBankAccounts()
		a.persistBankAccounts(tempBankAccounts)
	}

	// Header button: only toggles the visibility of the input fields.
	addBankAccountBtn := widget.NewButton("+ Zahlungskonto hinzufügen", nil)
	addBankAccountBtn.OnTapped = func() {
		if addBankFields.Visible() {
			addBankFields.Hide()
		} else {
			addBankFields.Show()
		}
	}

	refreshBankAccounts()

	// Language
	languageSelect := widget.NewSelect([]string{
		a.bundle.T("settings.language.de"),
		a.bundle.T("settings.language.en"),
	}, nil)
	if a.settings.Language == "de" {
		languageSelect.SetSelected(a.bundle.T("settings.language.de"))
	} else {
		languageSelect.SetSelected(a.bundle.T("settings.language.en"))
	}

	// Debug mode
	debugModeCheck := widget.NewCheck(
		a.bundle.T("settings.debugMode"),
		nil,
	)
	debugModeCheck.SetChecked(a.settings.DebugMode)

	debugHint := newCopyableLabel(a.bundle, a.bundle.T("settings.debugMode.hint"))
	debugHint.Wrapping = fyne.TextWrapWord

	// DATEV identifier entries
	datevBeraterEntry := widget.NewEntry()
	datevBeraterEntry.SetText(a.settings.DatevBeraterNr)
	datevBeraterEntry.SetPlaceHolder("z. B. 1234567")

	datevMandantEntry := widget.NewEntry()
	datevMandantEntry.SetText(a.settings.DatevMandantNr)
	datevMandantEntry.SetPlaceHolder("z. B. 10000")

	datevWJBeginnEntry := widget.NewEntry()
	datevWJBeginnEntry.SetText(a.settings.DatevWJBeginn)
	datevWJBeginnEntry.SetPlaceHolder("01012026")

	datevHint := newCopyableLabel(a.bundle, a.bundle.T("settings.datev.hint"))
	datevHint.Wrapping = fyne.TextWrapWord

	// Reconciliation match config
	matchWindowEntry := widget.NewEntry()
	if a.settings.MatchDateWindowDays > 0 {
		matchWindowEntry.SetText(strconv.Itoa(a.settings.MatchDateWindowDays))
	}
	matchWindowEntry.SetPlaceHolder(strconv.Itoa(core.DefaultMatchConfig().DateWindowDays))

	matchToleranceEntry := widget.NewEntry()
	if a.settings.MatchForeignTolerancePct > 0 {
		matchToleranceEntry.SetText(strings.Replace(fmt.Sprintf("%g", a.settings.MatchForeignTolerancePct), ".", ",", 1))
	}
	matchToleranceEntry.SetPlaceHolder(strings.Replace(fmt.Sprintf("%g", core.DefaultMatchConfig().ForeignTolerancePct), ".", ",", 1))

	// Wipe database button
	wipeDBBtn := widget.NewButton(a.bundle.T("settings.wipeDatabase"), func() {
		// Show confirmation dialog
		dialog.ShowConfirm(
			a.bundle.T("settings.wipeDatabase.confirm.title"),
			a.bundle.T("settings.wipeDatabase.confirm.message"),
			func(confirmed bool) {
				if !confirmed {
					return
				}

				// Wipe the database
				if a.dbRepo != nil {
					if err := a.dbRepo.WipeDatabase(); err != nil {
						a.showError(
							a.bundle.T("error.processing.title"),
							fmt.Sprintf("Failed to wipe database: %v", err),
						)
						return
					}

					a.logger.Info("Database wiped successfully")

					// Reload invoices (will be empty)
					a.loadInvoices()

					a.showInfo(
						a.bundle.T("settings.wipeDatabase"),
						a.bundle.T("settings.wipeDatabase.success"),
					)
				}
			},
			a.window,
		)
	})
	wipeDBBtn.Importance = widget.DangerImportance

	// Rebook foreign-currency invoices to EUR
	rebookForeignBtn := widget.NewButton("Fremdwährungs-Belege auf EUR umstellen", func() {
		dialog.ShowConfirm(
			"Fremdwährungs-Umstellung",
			"Alle Buchungen von Fremdwährungs-Belegen (z. B. USD) werden auf EUR umgerechnet\n"+
				"(Betrag ÷ Wechselkurs). Idempotent: bereits umgerechnete Buchungen werden übersprungen.\n\n"+
				"Fortfahren?",
			func(confirmed bool) {
				if !confirmed || a.dbRepo == nil {
					return
				}
				conv, skip, missing, rebookErr := a.dbRepo.RebookForeignToEUR()
				if rebookErr != nil {
					a.showError("Fehler", fmt.Sprintf("Umrechnung fehlgeschlagen: %v", rebookErr))
					return
				}
				a.loadInvoices()
				a.showInfo(
					"Fremdwährungs-Umstellung abgeschlossen",
					fmt.Sprintf("Umgerechnet: %d\nÜbersprungen (bereits EUR): %d\nKein Kurs vorhanden: %d",
						conv, skip, missing),
				)
			},
			a.window,
		)
	})

	// Column order
	tempColumnOrder := make([]string, len(a.settings.ColumnOrder))
	copy(tempColumnOrder, a.settings.ColumnOrder)
	// If empty, use defaults
	if len(tempColumnOrder) == 0 {
		tempColumnOrder = make([]string, len(core.DefaultCSVColumns))
		copy(tempColumnOrder, core.DefaultCSVColumns)
	} else {
		// Filter out columns that no longer exist in ColumnDisplayNames
		validColumns := make([]string, 0, len(tempColumnOrder))
		for _, col := range tempColumnOrder {
			if _, ok := core.ColumnDisplayNames[col]; ok {
				validColumns = append(validColumns, col)
			}
		}
		tempColumnOrder = validColumns

		// Add any missing columns from defaults (e.g., newly added fields)
		existingCols := make(map[string]bool)
		for _, col := range tempColumnOrder {
			existingCols[col] = true
		}
		for _, col := range core.DefaultCSVColumns {
			if !existingCols[col] {
				tempColumnOrder = append(tempColumnOrder, col)
			}
		}
	}

	columnList := container.NewVBox()
	var columnRows []*draggableRow

	// syncColumnRows refreshes every row's label and button states to
	// match its current position in tempColumnOrder.
	syncColumnRows := func() {
		n := len(columnRows)
		for i, row := range columnRows {
			row.index = i
			colID := tempColumnOrder[i]
			name := columnHeaderFor(a.bundle, colID)
			row.rowLabel.SetText(fmt.Sprintf("%d. %s", i+1, name))
			if i == 0 {
				row.upBtn.Disable()
			} else {
				row.upBtn.Enable()
			}
			if i == n-1 {
				row.downBtn.Disable()
			} else {
				row.downBtn.Enable()
			}
		}
	}

	// swapColumns swaps two adjacent entries in the column order and the
	// UI in place, keeping any in-progress drag attached to its widget.
	swapColumns := func(a, b int) {
		n := len(tempColumnOrder)
		if a < 0 || b < 0 || a >= n || b >= n || a == b {
			return
		}
		tempColumnOrder[a], tempColumnOrder[b] = tempColumnOrder[b], tempColumnOrder[a]
		columnRows[a], columnRows[b] = columnRows[b], columnRows[a]
		columnList.Objects[a], columnList.Objects[b] = columnList.Objects[b], columnList.Objects[a]
		syncColumnRows()
		columnList.Refresh()
	}

	onColumnDragEnd := func(row *draggableRow) {
		row.dragAccum = 0
	}

	// onColumnDrag accumulates the vertical drag distance and swaps the
	// row with a neighbour each time it crosses half a row's height.
	onColumnDrag := func(row *draggableRow, dy float32) {
		row.dragAccum += dy

		pitch := row.Size().Height
		if idx := row.index; idx+1 < len(columnRows) {
			pitch = columnRows[idx+1].Position().Y - row.Position().Y
		} else if idx > 0 {
			pitch = row.Position().Y - columnRows[idx-1].Position().Y
		}
		if pitch <= 0 {
			return
		}

		for row.dragAccum > pitch/2 && row.index < len(columnRows)-1 {
			swapColumns(row.index, row.index+1)
			row.dragAccum -= pitch
		}
		for row.dragAccum < -pitch/2 && row.index > 0 {
			swapColumns(row.index, row.index-1)
			row.dragAccum += pitch
		}
	}

	for idx := range tempColumnOrder {
		rowLabel := widget.NewLabel("")
		rowLabel.Alignment = fyne.TextAlignLeading
		rowLabel.Wrapping = fyne.TextWrapOff

		grip := widget.NewLabel("↕")

		upBtn := widget.NewButton("↑", nil)
		downBtn := widget.NewButton("↓", nil)
		upBtn.Importance = widget.LowImportance
		downBtn.Importance = widget.LowImportance

		buttons := container.NewHBox(upBtn, downBtn)
		content := container.NewPadded(
			container.NewBorder(nil, nil, grip, buttons, rowLabel),
		)

		row := newDraggableRow(content, idx)
		row.rowLabel = rowLabel
		row.upBtn = upBtn
		row.downBtn = downBtn
		row.onDrag = onColumnDrag
		row.onDragEnd = onColumnDragEnd

		upBtn.OnTapped = func() { swapColumns(row.index, row.index-1) }
		downBtn.OnTapped = func() { swapColumns(row.index, row.index+1) }

		columnRows = append(columnRows, row)
		columnList.Objects = append(columnList.Objects, row)
	}

	syncColumnRows()
	columnList.Refresh()

	columnHint := newCopyableLabel(a.bundle, a.bundle.T("settings.columns.hint"))
	columnHint.Wrapping = fyne.TextWrapWord

	// Tab 1: General settings
	generalTab := container.NewVScroll(container.NewVBox(
		widget.NewLabel(a.bundle.T("settings.storage")),
		widget.NewForm(
			widget.NewFormItem(a.bundle.T("settings.targetFolder"),
				container.NewBorder(nil, nil, nil, browseFolderBtn, storageRootEntry)),
			widget.NewFormItem("Scan-Eingang-Ordner",
				container.NewBorder(nil, nil, nil, browseScanInboxBtn, scanInboxEntry)),
		),
		useMonthFoldersCheck,
		widget.NewSeparator(),

		widget.NewLabel(a.bundle.T("settings.template")),
		templateEntry,
		templateHelp,
		widget.NewForm(
			widget.NewFormItem(a.bundle.T("settings.decimal"), decimalSelect),
			widget.NewFormItem(a.bundle.T("settings.currencyDefault"), currencyEntry),
		),
		widget.NewSeparator(),

		widget.NewLabel("Eigene VAT-IDs"),
		ownVATIDEntry,
		widget.NewLabelWithStyle(
			"Komma-getrennt eintragen. Diese Nummern werden bei der automatischen Rechnungs-Extraktion NIE als Absender-VAT-ID übernommen (sie gehören dir).",
			fyne.TextAlignLeading,
			fyne.TextStyle{Italic: true},
		),
		widget.NewSeparator(),

		widget.NewLabel(a.bundle.T("settings.csv")),
		widget.NewForm(
			widget.NewFormItem(a.bundle.T("settings.csvSeparator"), csvSeparatorSelect),
			widget.NewFormItem(a.bundle.T("settings.csvEncoding"), csvEncodingSelect),
		),
		widget.NewSeparator(),

		widget.NewLabel(a.bundle.T("settings.language")),
		languageSelect,
	))

	// Tab 2: Processing settings
	processingTab := container.NewVScroll(container.NewVBox(
		widget.NewLabel(a.bundle.T("settings.processing")),
		modeSelect,
		apiKeyContainer,
	))

	// SKR04 import button
	skr04ImportBtn := widget.NewButton(a.bundle.T("settings.skr04.import"), func() {
		a.showFilePicker(func(path string) {
			data, err := os.ReadFile(path)
			if err != nil {
				a.showError(a.bundle.T("error.processing.title"), a.bundle.T("settings.skr04.readError", err))
				return
			}
			accs, err := core.ParseChartCSV(data)
			if err != nil {
				a.showError(a.bundle.T("error.processing.title"), a.bundle.T("settings.skr04.parseError", err))
				return
			}
			if err := a.chartStore.SaveImport(accs); err != nil {
				a.showError(a.bundle.T("error.processing.title"), a.bundle.T("settings.skr04.saveError", err))
				return
			}
			if c, err := a.chartStore.Load(); err == nil {
				a.chart = c
			}
			a.showToast(a.bundle.T("settings.skr04.imported", len(accs)))
		})
	})

	// Booking rules section — seeded from current profile rules
	vst19 := 1406
	if k, ok := a.bookingRules.VorsteuerKonto(19); ok {
		vst19 = k
	}
	vst7 := 1401
	if k, ok := a.bookingRules.VorsteuerKonto(7); ok {
		vst7 = k
	}
	bewAbz, bewNicht, bewProzent := 6640, 6644, 70.0
	if rule, ok := a.bookingRules.Rule("bewirtung"); ok {
		bewAbz, bewNicht, bewProzent = rule.KontoAbziehbar, rule.KontoNichtAbziehbar, rule.AbziehbarProzent
	}
	vstRC, ustRC := 1407, 3837
	if r, ok := a.bookingRules.Rule("reverse_charge"); ok {
		if r.KontoVStRC != 0 {
			vstRC = r.KontoVStRC
		}
		if r.KontoUStRC != 0 {
			ustRC = r.KontoUStRC
		}
	}
	geschAbz, geschNicht := 6610, 6620
	if r, ok := a.bookingRules.Rule("geschenke"); ok {
		if r.KontoAbziehbar != 0 {
			geschAbz = r.KontoAbziehbar
		}
		if r.KontoNichtAbziehbar != 0 {
			geschNicht = r.KontoNichtAbziehbar
		}
	}
	reiseKonto, kfzKonto := 6650, 6520
	if r, ok := a.bookingRules.Rule("reisekosten"); ok && r.DefaultKonto != 0 {
		reiseKonto = r.DefaultKonto
	}
	if r, ok := a.bookingRules.Rule("kfz"); ok && r.DefaultKonto != 0 {
		kfzKonto = r.DefaultKonto
	}

	vst19Lbl := widget.NewLabel(paymentSKR04Label(a, vst19))
	vst19Btn := widget.NewButton(a.bundle.T("settings.rules.pick"), func() {
		a.showAccountSearch(vst19, a.window, func(n int) { vst19 = n; vst19Lbl.SetText(paymentSKR04Label(a, n)) })
	})
	vst7Lbl := widget.NewLabel(paymentSKR04Label(a, vst7))
	vst7Btn := widget.NewButton(a.bundle.T("settings.rules.pick"), func() {
		a.showAccountSearch(vst7, a.window, func(n int) { vst7 = n; vst7Lbl.SetText(paymentSKR04Label(a, n)) })
	})
	bewAbzLbl := widget.NewLabel(paymentSKR04Label(a, bewAbz))
	bewAbzBtn := widget.NewButton(a.bundle.T("settings.rules.pick"), func() {
		a.showAccountSearch(bewAbz, a.window, func(n int) { bewAbz = n; bewAbzLbl.SetText(paymentSKR04Label(a, n)) })
	})
	bewNichtLbl := widget.NewLabel(paymentSKR04Label(a, bewNicht))
	bewNichtBtn := widget.NewButton(a.bundle.T("settings.rules.pick"), func() {
		a.showAccountSearch(bewNicht, a.window, func(n int) { bewNicht = n; bewNichtLbl.SetText(paymentSKR04Label(a, n)) })
	})
	bewProzentEntry := widget.NewEntry()
	bewProzentEntry.SetText(strings.Replace(fmt.Sprintf("%g", bewProzent), ".", ",", 1))

	vstRCLbl := widget.NewLabel(paymentSKR04Label(a, vstRC))
	vstRCBtn := widget.NewButton(a.bundle.T("settings.rules.pick"), func() {
		a.showAccountSearch(vstRC, a.window, func(n int) { vstRC = n; vstRCLbl.SetText(paymentSKR04Label(a, n)) })
	})
	ustRCLbl := widget.NewLabel(paymentSKR04Label(a, ustRC))
	ustRCBtn := widget.NewButton(a.bundle.T("settings.rules.pick"), func() {
		a.showAccountSearch(ustRC, a.window, func(n int) { ustRC = n; ustRCLbl.SetText(paymentSKR04Label(a, n)) })
	})
	geschAbzLbl := widget.NewLabel(paymentSKR04Label(a, geschAbz))
	geschAbzBtn := widget.NewButton(a.bundle.T("settings.rules.pick"), func() {
		a.showAccountSearch(geschAbz, a.window, func(n int) { geschAbz = n; geschAbzLbl.SetText(paymentSKR04Label(a, n)) })
	})
	geschNichtLbl := widget.NewLabel(paymentSKR04Label(a, geschNicht))
	geschNichtBtn := widget.NewButton(a.bundle.T("settings.rules.pick"), func() {
		a.showAccountSearch(geschNicht, a.window, func(n int) { geschNicht = n; geschNichtLbl.SetText(paymentSKR04Label(a, n)) })
	})
	reiseLbl := widget.NewLabel(paymentSKR04Label(a, reiseKonto))
	reiseBtn := widget.NewButton(a.bundle.T("settings.rules.pick"), func() {
		a.showAccountSearch(reiseKonto, a.window, func(n int) { reiseKonto = n; reiseLbl.SetText(paymentSKR04Label(a, n)) })
	})
	kfzLbl := widget.NewLabel(paymentSKR04Label(a, kfzKonto))
	kfzBtn := widget.NewButton(a.bundle.T("settings.rules.pick"), func() {
		a.showAccountSearch(kfzKonto, a.window, func(n int) { kfzKonto = n; kfzLbl.SetText(paymentSKR04Label(a, n)) })
	})

	rulesSection := widget.NewCard("", a.bundle.T("settings.rules.section"), container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("settings.rules.vst19")), vst19Btn, vst19Lbl),
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("settings.rules.vst7")), vst7Btn, vst7Lbl),
		widget.NewSeparator(),
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("settings.rules.bewAbz")), bewAbzBtn, bewAbzLbl),
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("settings.rules.bewNicht")), bewNichtBtn, bewNichtLbl),
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("settings.rules.bewProzent")), nil, bewProzentEntry),
		widget.NewSeparator(),
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("settings.rules.vstrc")), vstRCBtn, vstRCLbl),
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("settings.rules.ustrc")), ustRCBtn, ustRCLbl),
		widget.NewSeparator(),
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("settings.rules.geschabz")), geschAbzBtn, geschAbzLbl),
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("settings.rules.geschnicht")), geschNichtBtn, geschNichtLbl),
		widget.NewSeparator(),
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("settings.rules.reise")), reiseBtn, reiseLbl),
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("settings.rules.kfz")), kfzBtn, kfzLbl),
	))

	// Tab 3: Accounts settings
	accountsTab := container.NewVScroll(container.NewVBox(
		container.NewBorder(nil, nil,
			widget.NewLabel(a.bundle.T("settings.accounts")), addAccountBtn),
		container.NewGridWithColumns(3,
			container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("settings.defaultAccount")), nil, defaultAccountEntry),
		),
		widget.NewCard("", "", container.NewVBox(
			widget.NewLabel("Konten verwalten (max. 10):"),
			addAccountFields,
			widget.NewSeparator(),
			accountsList,
		)),
		accountsNote,
		rememberCompanyCheck,
		autoSelectCheck,
		widget.NewSeparator(),

		container.NewBorder(nil, nil,
			widget.NewLabel("Zahlungskonten"), addBankAccountBtn),
		widget.NewCard("", "", container.NewVBox(
			addBankFields,
			widget.NewSeparator(),
			bankAccountsList,
		)),
		widget.NewSeparator(),
		widget.NewLabel(a.bundle.T("settings.skr04.section")),
		skr04ImportBtn,
		widget.NewSeparator(),
		rulesSection,
	))

	// Kontenrahmen section: validate + apply SKR variant
	detectedVariant := core.DetectSKRVariant(a.chart)

	skrStatusLbl := widget.NewLabel("")
	skrStatusLbl.Wrapping = fyne.TextWrapWord

	pruefenBtn := widget.NewButton("Prüfen", func() {
		issues := core.ValidateBookingAccounts(a.bookingRules, a.chart)
		variant := core.DetectSKRVariant(a.chart)
		variantText := "Erkannter Kontenrahmen: "
		if variant != "" {
			variantText += variant
		} else {
			variantText += "(unbekannt)"
		}
		if len(issues) == 0 {
			skrStatusLbl.SetText(variantText + "\n✓ Alle Buchungskonten im Kontenrahmen vorhanden")
		} else {
			text := variantText + "\nProbleme:\n"
			for _, iss := range issues {
				text += "• " + iss + "\n"
			}
			skrStatusLbl.SetText(text)
		}
	})

	skr03Btn := widget.NewButton("SKR03 anwenden", nil)
	skr04Btn := widget.NewButton("SKR04 anwenden", nil)
	if detectedVariant == "SKR03" {
		skr03Btn.Importance = widget.HighImportance
	} else if detectedVariant == "SKR04" {
		skr04Btn.Importance = widget.HighImportance
	}

	applyVariant := func(variant string) {
		dialog.ShowConfirm(
			"Kontenrahmen anwenden",
			"Alle Standard-Buchungskonten werden auf "+variant+" umgestellt.\n\nFortfahren?",
			func(confirmed bool) {
				if !confirmed {
					return
				}
				newRules := core.ApplySKRVariant(a.bookingRules, variant)
				if err := a.bookingRulesStore.Save(newRules); err != nil {
					a.showError("Fehler", fmt.Sprintf("Buchungsregeln konnten nicht gespeichert werden: %v", err))
					return
				}
				a.bookingRules = newRules
				skrStatusLbl.SetText("✓ " + variant + " erfolgreich angewendet und gespeichert")
				a.showToast("✓ " + variant + " angewendet")
			},
			a.window,
		)
	}
	skr03Btn.OnTapped = func() { applyVariant("SKR03") }
	skr04Btn.OnTapped = func() { applyVariant("SKR04") }

	kontenrahmenSection := widget.NewCard("", "Kontenrahmen", container.NewVBox(
		container.NewHBox(pruefenBtn, skr03Btn, skr04Btn),
		skrStatusLbl,
	))

	// Tab 4: Advanced settings
	advancedTab := container.NewVScroll(container.NewVBox(
		widget.NewLabel(a.bundle.T("settings.columns")),
		columnHint,
		columnList,
		widget.NewSeparator(),

		widget.NewLabel(a.bundle.T("settings.debug")),
		debugModeCheck,
		debugHint,
		widget.NewSeparator(),

		widget.NewLabel("DATEV"),
		widget.NewForm(
			widget.NewFormItem(a.bundle.T("settings.datev.berater"), datevBeraterEntry),
			widget.NewFormItem(a.bundle.T("settings.datev.mandant"), datevMandantEntry),
			widget.NewFormItem(a.bundle.T("settings.datev.wj"), datevWJBeginnEntry),
		),
		datevHint,
		widget.NewSeparator(),

		widget.NewLabel(a.bundle.T("reconcile.title")),
		widget.NewForm(
			widget.NewFormItem(a.bundle.T("settings.matchWindow"), matchWindowEntry),
			widget.NewFormItem(a.bundle.T("settings.matchTolerance"), matchToleranceEntry),
		),
		widget.NewSeparator(),

		kontenrahmenSection,
		widget.NewSeparator(),

		widget.NewLabel(a.bundle.T("settings.database")),
		wipeDBBtn,
		widget.NewLabel(a.bundle.T("settings.wipeDatabase.hint")),
		widget.NewSeparator(),

		widget.NewLabel("Daten-Migration"),
		rebookForeignBtn,
		widget.NewLabel("Einmalige Umrechnung: Buchungsbeträge in Fremdwährung werden auf EUR skaliert (÷ Wechselkurs). Bereits umgerechnete Buchungen werden übersprungen."),
	))

	// Build tabbed container
	tabs := container.NewAppTabs(
		container.NewTabItem("Allgemein", generalTab),
		container.NewTabItem("Verarbeitung", processingTab),
		container.NewTabItem("Konten", accountsTab),
		container.NewTabItem("Erweitert", advancedTab),
	)

	// Save action: persists settings and returns to the main view.
	saveAction := func() {
		newSettings := a.settings
		prevColumnOrder := append([]string{}, a.settings.ColumnOrder...)

		newSettings.StorageRoot = storageRootEntry.Text
		newSettings.ScanInboxFolder = scanInboxEntry.Text
		newSettings.UseMonthSubfolders = useMonthFoldersCheck.Checked
		newSettings.NamingTemplate = templateEntry.Text
		newSettings.DecimalSeparator = decimalSelect.Selected
		newSettings.CurrencyDefault = currencyEntry.Text
		newSettings.OwnVATID = strings.TrimSpace(ownVATIDEntry.Text)

		if modeSelect.Selected == a.bundle.T("settings.mode.claude") {
			newSettings.ProcessingMode = "claude"
		} else {
			newSettings.ProcessingMode = "local"
		}

		newSettings.AnthropicModel = modelEntry.Text

		// CSV settings
		if csvSeparatorSelect.Selected == "\\t" {
			newSettings.CSVSeparator = "\t"
		} else {
			newSettings.CSVSeparator = csvSeparatorSelect.Selected
		}
		newSettings.CSVEncoding = csvEncodingSelect.Selected

		// Save API key to keyring
		if apiKeyEntry.Text != "" {
			err := keyring.Set("BuchISY", a.keyringAccount(), apiKeyEntry.Text)
			if err != nil {
				a.logger.Warn("Failed to save API key: %v", err)
				a.showError(
					a.bundle.T("error.processing.title"),
					fmt.Sprintf("Failed to save API key: %v", err),
				)
			}
		}

		defaultAccount, _ := strconv.Atoi(defaultAccountEntry.Text)
		newSettings.DefaultAccount = defaultAccount

		// Save modified accounts
		newSettings.Accounts = tempAccounts

		// Save bank accounts (the Standard-Zahlungskonto fields are
		// gone; we don't write them anymore so they fade out of new
		// settings.json files).
		newSettings.DefaultBankAccount = ""
		newSettings.DefaultBankAccountIBAN = ""
		newSettings.BankAccounts = tempBankAccounts

		newSettings.RememberCompanyAccount = rememberCompanyCheck.Checked
		newSettings.AutoSelectAccount = autoSelectCheck.Checked

		// Update CSV repository settings
		a.csvRepo.SetSeparator(newSettings.CSVSeparator)
		a.csvRepo.SetEncoding(newSettings.CSVEncoding)
		a.csvRepo.SetDecimalSeparator(newSettings.DecimalSeparator)

		if languageSelect.Selected == a.bundle.T("settings.language.de") {
			newSettings.Language = "de"
		} else {
			newSettings.Language = "en"
		}

		newSettings.DebugMode = debugModeCheck.Checked
		newSettings.DatevBeraterNr = strings.TrimSpace(datevBeraterEntry.Text)
		newSettings.DatevMandantNr = strings.TrimSpace(datevMandantEntry.Text)
		newSettings.DatevWJBeginn = strings.TrimSpace(datevWJBeginnEntry.Text)

		// Reconciliation match config
		if v, err := strconv.Atoi(strings.TrimSpace(matchWindowEntry.Text)); err == nil && v > 0 {
			newSettings.MatchDateWindowDays = v
		} else {
			newSettings.MatchDateWindowDays = 0
		}
		if v := parseDecimal(matchToleranceEntry.Text); v > 0 {
			newSettings.MatchForeignTolerancePct = v
		} else {
			newSettings.MatchForeignTolerancePct = 0
		}

		newSettings.ColumnOrder = tempColumnOrder
		columnOrderChanged := !equalStringSlices(prevColumnOrder, newSettings.ColumnOrder)

		// Save to disk
		if err := a.settingsMgr.Save(newSettings); err != nil {
			a.showError(
				a.bundle.T("error.processing.title"),
				fmt.Sprintf("Failed to save settings: %v", err),
			)
			return
		}

		// Update app settings
		a.settings = newSettings
		a.storageManager = core.NewStorageManager(&a.settings)

		// Update logger level if debug mode changed
		if newSettings.DebugMode {
			a.logger.SetLevel(logging.DEBUG)
			a.logger.Debug("Debug mode enabled")
		} else {
			a.logger.SetLevel(logging.INFO)
		}

		// Update extractor debug flag
		a.anthropicExtractor.SetDebug(newSettings.DebugMode)

		// Update CSV column order
		a.csvRepo.SetColumnOrder(newSettings.ColumnOrder)

		if columnOrderChanged {
			if err := a.rewriteAllCSVs(); err != nil {
				a.logger.Warn("Failed to rewrite CSV files: %v", err)
				a.showError(
					a.bundle.T("error.processing.title"),
					a.bundle.T("settings.columns.rewriteError", err.Error()),
				)
			}
		}

		if a.invoiceTable != nil {
			a.invoiceTable.SetColumnOrder(newSettings.ColumnOrder)
			a.invoiceTable.SetDecimalSeparator(newSettings.DecimalSeparator)
		}
		a.loadInvoices()

		// Save booking rules (per-profile: Vorsteuer accounts + Bewirtung)
		rules := a.bookingRules
		if rules.VorsteuerKonten == nil {
			rules.VorsteuerKonten = map[string]int{}
		}
		rules.VorsteuerKonten["19"] = vst19
		rules.VorsteuerKonten["7"] = vst7
		for i := range rules.Regeln {
			switch rules.Regeln[i].Kategorie {
			case "bewirtung":
				rules.Regeln[i].KontoAbziehbar = bewAbz
				rules.Regeln[i].KontoNichtAbziehbar = bewNicht
				if p := parseDecimal(bewProzentEntry.Text); p > 0 {
					rules.Regeln[i].AbziehbarProzent = p
				}
			case "reverse_charge":
				rules.Regeln[i].KontoVStRC = vstRC
				rules.Regeln[i].KontoUStRC = ustRC
			case "geschenke":
				rules.Regeln[i].KontoAbziehbar = geschAbz
				rules.Regeln[i].KontoNichtAbziehbar = geschNicht
			case "reisekosten":
				rules.Regeln[i].DefaultKonto = reiseKonto
			case "kfz":
				rules.Regeln[i].DefaultKonto = kfzKonto
			}
		}
		if err := a.bookingRulesStore.Save(rules); err != nil {
			a.logger.Warn("Failed to save booking rules: %v", err)
		}
		a.bookingRules = rules

		// Return to main view, then a quiet toast — no modal dialog
		// breaks the flow.
		a.showMainView()
		a.showToast("✓ Einstellungen gespeichert")
	}

	// Header bar: light blue title strip + action buttons on the right.
	titleLabel := widget.NewLabelWithStyle(
		a.bundle.T("settings.title"),
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)
	cancelBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() {
		a.showMainView()
	})
	saveBtn := widget.NewButton(a.bundle.T("btn.save"), saveAction)
	saveBtn.Importance = widget.HighImportance

	headerBar := container.NewBorder(
		nil, nil,
		container.NewPadded(titleLabel),
		container.NewPadded(container.NewHBox(cancelBtn, saveBtn)),
	)
	headerBg := canvas.NewRectangle(headerBackgroundColor)
	header := container.NewStack(headerBg, headerBar)

	settingsView := container.NewBorder(header, nil, nil, nil, tabs)
	a.window.SetContent(settingsView)
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// paymentSKR04Label renders the SKR04 mapping for a payment account row.
// Returns the i18n "none" string when konto == 0, the human-readable account
// label when the chart contains the account, or the bare number as fallback.
func paymentSKR04Label(a *App, konto int) string {
	if konto == 0 {
		return a.bundle.T("settings.payment.skr04.none")
	}
	if acc, ok := a.chart.Find(konto); ok {
		return accountLabel(acc)
	}
	return fmt.Sprintf("%d", konto)
}
