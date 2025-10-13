package ui

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/zalando/go-keyring"

	"github.com/bergx2/buchisy/internal/core"
	"github.com/bergx2/buchisy/internal/logging"
)

// showSettingsDialog shows the settings dialog.
func (a *App) showSettingsDialog() {
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

	useMonthFoldersCheck := widget.NewCheck(
		a.bundle.T("settings.useMonthFolders"),
		nil,
	)
	useMonthFoldersCheck.SetChecked(a.settings.UseMonthSubfolders)

	// Filename template
	templateEntry := widget.NewEntry()
	templateEntry.SetText(a.settings.NamingTemplate)
	templateEntry.SetPlaceHolder(a.bundle.T("settings.template"))

	templateHelp := widget.NewLabel(a.bundle.T("settings.templateHelp"))
	templateHelp.Wrapping = fyne.TextWrapWord

	// Decimal separator
	decimalSelect := widget.NewSelect([]string{",", "."}, nil)
	decimalSelect.SetSelected(a.settings.DecimalSeparator)

	// Currency default
	currencyEntry := widget.NewEntry()
	currencyEntry.SetText(a.settings.CurrencyDefault)

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
	existingKey, err := keyring.Get("BuchISY", a.settings.AnthropicAPIKeyRef)
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

	// Add account controls
	newAccountCodeEntry := widget.NewEntry()
	newAccountCodeEntry.SetPlaceHolder(a.bundle.T("settings.accountCode"))
	newAccountCodeEntry.SetText("")

	newAccountLabelEntry := widget.NewEntry()
	newAccountLabelEntry.SetPlaceHolder(a.bundle.T("settings.accountLabel"))
	newAccountLabelEntry.SetText("")

	addAccountBtn := widget.NewButton("+ Konto hinzufügen", func() {
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

		// Check for duplicate codes
		for _, acc := range tempAccounts {
			if acc.Code == code {
				a.showError("Fehler", fmt.Sprintf("Konto mit Code %d existiert bereits.", code))
				return
			}
		}

		// Check limit
		if len(tempAccounts) >= 10 {
			a.showError("Fehler", "Maximal 10 Konten erlaubt.")
			return
		}

		// Add account
		tempAccounts = append(tempAccounts, core.Account{
			Code:  code,
			Label: label,
		})

		// Clear inputs
		newAccountCodeEntry.SetText("")
		newAccountLabelEntry.SetText("")

		refreshAccounts()
	})

	refreshAccounts()

	accountsNote := widget.NewLabel(a.bundle.T("settings.accountsNote"))
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

	// Bank accounts
	defaultBankAccountEntry := widget.NewEntry()
	defaultBankAccountEntry.SetText(a.settings.DefaultBankAccount)

	// Bank account list (make a copy to modify)
	tempBankAccounts := make([]core.BankAccount, len(a.settings.BankAccounts))
	copy(tempBankAccounts, a.settings.BankAccounts)

	// Bank account list container
	bankAccountsList := container.NewVBox()

	var refreshBankAccounts func()
	refreshBankAccounts = func() {
		bankAccountsList.Objects = bankAccountsList.Objects[:0]

		for idx, ba := range tempBankAccounts {
			currentIdx := idx
			label := widget.NewLabel(fmt.Sprintf("  %s", ba.Name))
			label.Alignment = fyne.TextAlignLeading
			label.Wrapping = fyne.TextWrapOff

			removeBtn := widget.NewButton("Entfernen", func() {
				// Remove this bank account
				if currentIdx < len(tempBankAccounts) {
					tempBankAccounts = append(tempBankAccounts[:currentIdx], tempBankAccounts[currentIdx+1:]...)
					refreshBankAccounts()
				}
			})
			removeBtn.Importance = widget.LowImportance

			row := container.NewBorder(nil, nil, nil, removeBtn, label)
			bankAccountsList.Add(container.NewPadded(row))
		}

		bankAccountsList.Refresh()
	}

	// Add bank account controls
	newBankAccountEntry := widget.NewEntry()
	newBankAccountEntry.SetPlaceHolder("Bankkonto Name")
	newBankAccountEntry.SetText("")

	addBankAccountBtn := widget.NewButton("+ Bankkonto hinzufügen", func() {
		name := newBankAccountEntry.Text
		if name == "" {
			a.showError("Fehler", "Bitte einen Namen eingeben.")
			return
		}

		// Check for duplicate names
		for _, ba := range tempBankAccounts {
			if ba.Name == name {
				a.showError("Fehler", fmt.Sprintf("Bankkonto '%s' existiert bereits.", name))
				return
			}
		}

		// Check limit
		if len(tempBankAccounts) >= 10 {
			a.showError("Fehler", "Maximal 10 Bankkonten erlaubt.")
			return
		}

		// Add bank account
		tempBankAccounts = append(tempBankAccounts, core.BankAccount{
			Name: name,
		})

		// Clear input
		newBankAccountEntry.SetText("")

		refreshBankAccounts()
	})

	refreshBankAccounts()

	bankAccountsNote := widget.NewLabel("Verwalten Sie Ihre Bankkonten hier.")
	bankAccountsNote.Wrapping = fyne.TextWrapWord

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

	debugHint := widget.NewLabel(a.bundle.T("settings.debugMode.hint"))
	debugHint.Wrapping = fyne.TextWrapWord

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

	var refreshColumns func()
	refreshColumns = func() {
		columnList.Objects = columnList.Objects[:0]

		for idx, colID := range tempColumnOrder {
			currentIdx := idx
			displayName := core.ColumnDisplayNames[colID]
			if displayName == "" {
				displayName = colID
			}

			label := widget.NewLabel(fmt.Sprintf("%d. %s", idx+1, displayName))
			label.Alignment = fyne.TextAlignLeading
			label.Wrapping = fyne.TextWrapOff

			upBtn := widget.NewButton("↑", func() {
				if currentIdx > 0 {
					tempColumnOrder[currentIdx], tempColumnOrder[currentIdx-1] = tempColumnOrder[currentIdx-1], tempColumnOrder[currentIdx]
					refreshColumns()
				}
			})
			downBtn := widget.NewButton("↓", func() {
				if currentIdx < len(tempColumnOrder)-1 {
					tempColumnOrder[currentIdx], tempColumnOrder[currentIdx+1] = tempColumnOrder[currentIdx+1], tempColumnOrder[currentIdx]
					refreshColumns()
				}
			})

			if currentIdx == 0 {
				upBtn.Disable()
			} else {
				upBtn.Enable()
			}

			if currentIdx == len(tempColumnOrder)-1 {
				downBtn.Disable()
			} else {
				downBtn.Enable()
			}

			upBtn.Importance = widget.LowImportance
			downBtn.Importance = widget.LowImportance

			buttons := container.NewHBox(upBtn, downBtn)
			row := container.NewBorder(nil, nil, nil, buttons, label)
			columnList.Add(container.NewPadded(row))
		}

		columnList.Refresh()
	}

	refreshColumns()

	columnHint := widget.NewLabel(a.bundle.T("settings.columns.hint"))
	columnHint.Wrapping = fyne.TextWrapWord

	// Tab 1: General settings
	generalTab := container.NewVScroll(container.NewVBox(
		widget.NewLabel(a.bundle.T("settings.storage")),
		widget.NewForm(
			widget.NewFormItem(a.bundle.T("settings.targetFolder"),
				container.NewBorder(nil, nil, nil, browseFolderBtn, storageRootEntry)),
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

		widget.NewLabel(a.bundle.T("settings.language")),
		languageSelect,
	))

	// Tab 2: Processing settings
	processingTab := container.NewVScroll(container.NewVBox(
		widget.NewLabel(a.bundle.T("settings.processing")),
		modeSelect,
		apiKeyContainer,
	))

	// Tab 3: Accounts settings
	accountsTab := container.NewVScroll(container.NewVBox(
		widget.NewLabel(a.bundle.T("settings.accounts")),
		widget.NewForm(
			widget.NewFormItem(a.bundle.T("settings.defaultAccount"), defaultAccountEntry),
		),
		widget.NewCard("", "", container.NewVBox(
			widget.NewLabel("Konten verwalten (max. 10):"),
			widget.NewForm(
				widget.NewFormItem(a.bundle.T("settings.accountCode"), newAccountCodeEntry),
				widget.NewFormItem(a.bundle.T("settings.accountLabel"), newAccountLabelEntry),
			),
			addAccountBtn,
			widget.NewSeparator(),
			accountsList,
		)),
		accountsNote,
		rememberCompanyCheck,
		autoSelectCheck,
		widget.NewSeparator(),

		widget.NewLabel("Bankkonten"),
		widget.NewForm(
			widget.NewFormItem("Standard-Bankkonto", defaultBankAccountEntry),
		),
		widget.NewCard("", "", container.NewVBox(
			widget.NewLabel("Bankkonten verwalten (max. 10):"),
			widget.NewForm(
				widget.NewFormItem("Bankkonto Name", newBankAccountEntry),
			),
			addBankAccountBtn,
			widget.NewSeparator(),
			bankAccountsList,
		)),
		bankAccountsNote,
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
	))

	// Build tabbed container
	tabs := container.NewAppTabs(
		container.NewTabItem("Allgemein", generalTab),
		container.NewTabItem("Verarbeitung", processingTab),
		container.NewTabItem("Konten", accountsTab),
		container.NewTabItem("Erweitert", advancedTab),
	)

	// Dialog buttons
	settingsDialog := dialog.NewCustomConfirm(
		a.bundle.T("settings.title"),
		a.bundle.T("btn.save"),
		a.bundle.T("btn.cancel"),
		tabs,
		func(save bool) {
			if !save {
				return
			}

			// Save settings
			newSettings := a.settings
			prevColumnOrder := append([]string{}, a.settings.ColumnOrder...)

			newSettings.StorageRoot = storageRootEntry.Text
			newSettings.UseMonthSubfolders = useMonthFoldersCheck.Checked
			newSettings.NamingTemplate = templateEntry.Text
			newSettings.DecimalSeparator = decimalSelect.Selected
			newSettings.CurrencyDefault = currencyEntry.Text

			if modeSelect.Selected == a.bundle.T("settings.mode.claude") {
				newSettings.ProcessingMode = "claude"
			} else {
				newSettings.ProcessingMode = "local"
			}

			newSettings.AnthropicModel = modelEntry.Text

			// Save API key to keyring
			if apiKeyEntry.Text != "" {
				err := keyring.Set("BuchISY", a.settings.AnthropicAPIKeyRef, apiKeyEntry.Text)
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

			// Save bank accounts
			newSettings.DefaultBankAccount = defaultBankAccountEntry.Text
			newSettings.BankAccounts = tempBankAccounts

			newSettings.RememberCompanyAccount = rememberCompanyCheck.Checked
			newSettings.AutoSelectAccount = autoSelectCheck.Checked

			if languageSelect.Selected == a.bundle.T("settings.language.de") {
				newSettings.Language = "de"
			} else {
				newSettings.Language = "en"
			}

			newSettings.DebugMode = debugModeCheck.Checked
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
			}
			a.loadInvoices()

			a.showInfo(
				a.bundle.T("settings.title"),
				a.bundle.T("settings.saved"),
			)
		},
		a.window,
	)

	settingsDialog.Resize(fyne.NewSize(750, 650))
	settingsDialog.Show()
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
