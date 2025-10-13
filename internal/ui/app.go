// Package ui provides the Fyne-based user interface for BuchISY.
package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/anthropic"
	"github.com/bergx2/buchisy/internal/core"
	"github.com/bergx2/buchisy/internal/i18n"
	"github.com/bergx2/buchisy/internal/logging"
	"github.com/zalando/go-keyring"
)

// App represents the BuchISY application.
type App struct {
	app                fyne.App
	window             fyne.Window
	bundle             *i18n.Bundle
	logger             *logging.Logger
	settings           core.Settings
	settingsMgr        *core.SettingsManager
	companyMap         *core.CompanyAccountMap
	pdfExtractor       *core.PDFTextExtractor
	localExtractor     *core.LocalExtractor
	anthropicExtractor *anthropic.Extractor
	csvRepo            *core.CSVRepository
	storageManager     *core.StorageManager

	// Current state
	currentYear  int
	currentMonth time.Month

	// UI components
	yearSelect   *widget.Select
	monthSelect  *widget.Select
	invoiceTable *InvoiceTable
}

// New creates a new BuchISY application.
func New(assetsDir string) (*App, error) {
	// Initialize Fyne app
	fyneApp := app.NewWithID("com.bergx2.buchisy")

	// Get config directory
	configDir, err := core.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	// Initialize logger
	logDir := filepath.Join(configDir, "logs")
	logger, err := logging.New(logDir, logging.INFO)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	logger.Info("Starting BuchISY")

	// Load settings
	settingsPath := filepath.Join(configDir, "settings.json")
	settingsMgr := core.NewSettingsManager(settingsPath)
	settings, err := settingsMgr.Load()
	if err != nil {
		logger.Warn("Failed to load settings, using defaults: %v", err)
		settings = core.DefaultSettings()
	}

	// Initialize storage root if empty
	if settings.StorageRoot == "" {
		docsDir, err := core.GetDocumentsDir()
		if err != nil {
			logger.Warn("Failed to get documents directory: %v", err)
		} else {
			settings.StorageRoot = filepath.Join(docsDir, "BuchISY")
		}
	}

	// Load i18n bundle
	bundle, err := i18n.Load(assetsDir, settings.Language)
	if err != nil {
		logger.Warn("Failed to load translations: %v", err)
		// Create a fallback bundle
		bundle = &i18n.Bundle{}
	}

	// Initialize company account map
	companyMap := core.NewCompanyAccountMap(configDir)
	if err := companyMap.Load(); err != nil {
		logger.Warn("Failed to load company account map: %v", err)
	}

	// Set logger level based on debug mode
	if settings.DebugMode {
		logger.SetLevel(logging.DEBUG)
		logger.Debug("Debug mode enabled")
	}

	// Initialize components
	pdfExtractor := core.NewPDFTextExtractor()
	localExtractor := core.NewLocalExtractor()
	anthropicExtractor := anthropic.NewExtractor(logger, settings.DebugMode)
	csvRepo := core.NewCSVRepository()
	csvRepo.SetColumnOrder(settings.ColumnOrder)
	storageManager := core.NewStorageManager(&settings)

	// Determine current month (default to last month)
	now := time.Now()
	lastMonth := now.AddDate(0, -1, 0)
	currentYear := lastMonth.Year()
	currentMonth := lastMonth.Month()

	application := &App{
		app:                fyneApp,
		bundle:             bundle,
		logger:             logger,
		settings:           settings,
		settingsMgr:        settingsMgr,
		companyMap:         companyMap,
		pdfExtractor:       pdfExtractor,
		localExtractor:     localExtractor,
		anthropicExtractor: anthropicExtractor,
		csvRepo:            csvRepo,
		storageManager:     storageManager,
		currentYear:        currentYear,
		currentMonth:       currentMonth,
	}

	// Create main window
	application.window = fyneApp.NewWindow(bundle.T("app.title"))

	// Restore window size (position is handled by OS)
	if settings.WindowWidth > 0 && settings.WindowHeight > 0 {
		application.window.Resize(fyne.NewSize(float32(settings.WindowWidth), float32(settings.WindowHeight)))
	} else {
		application.window.Resize(fyne.NewSize(1500, 875)) // Default: 25% larger than 1200x700
	}

	application.window.CenterOnScreen()

	// Build UI
	content := application.buildUI()
	application.window.SetContent(content)

	return application, nil
}

// Run starts the application.
func (a *App) Run() {
	// Set up window close handler to save position/size
	a.window.SetOnClosed(func() {
		a.saveWindowState()
	})

	// ShowAndRun automatically brings window to front and starts event loop
	a.window.ShowAndRun()

	// Cleanup
	a.logger.Close()
}

// saveWindowState saves the current window size.
// Note: Position saving is not supported in Fyne v2, OS handles window position memory.
func (a *App) saveWindowState() {
	size := a.window.Canvas().Size()

	a.settings.WindowWidth = int(size.Width)
	a.settings.WindowHeight = int(size.Height)

	if err := a.settingsMgr.Save(a.settings); err != nil {
		a.logger.Warn("Failed to save window state: %v", err)
	} else {
		a.logger.Debug("Saved window size: %dx%d",
			a.settings.WindowWidth, a.settings.WindowHeight)
	}
}

// buildUI constructs the main UI layout.
func (a *App) buildUI() fyne.CanvasObject {
	// Create table first (before top bar, so callbacks don't crash)
	a.invoiceTable = NewInvoiceTable(a.bundle, a)
	a.invoiceTable.SetColumnOrder(a.settings.ColumnOrder)
	a.invoiceTable.SetWindow(a.window)

	// Top bar (this may trigger callbacks when setting initial values)
	topBar := a.buildTopBar()

	// Main content: left panel (drop area) + right panel (table)
	leftPanel := a.buildLeftPanel()
	rightPanel := a.invoiceTable.Container()

	mainContent := container.NewHSplit(leftPanel, rightPanel)
	mainContent.SetOffset(0.35) // 35% left, 65% right

	// Load initial data now that everything is set up
	a.loadInvoices()

	// Combine
	return container.NewBorder(topBar, nil, nil, nil, mainContent)
}

// buildTopBar creates the top toolbar.
func (a *App) buildTopBar() fyne.CanvasObject {
	// Year selector
	years := generateYearOptions()
	currentYearStr := fmt.Sprintf("%d", a.currentYear)
	a.yearSelect = widget.NewSelect(years, func(selected string) {
		var year int
		fmt.Sscanf(selected, "%d", &year)
		a.currentYear = year
		a.onMonthChanged()
	})
	a.yearSelect.SetSelected(currentYearStr)

	// Month selector
	months := generateMonthOptions(a.bundle)
	// Build the full string to match the dropdown options (e.g., "09 - September")
	monthKey := fmt.Sprintf("month.%02d", a.currentMonth)
	monthName := a.bundle.T(monthKey)
	currentMonthStr := fmt.Sprintf("%02d - %s", a.currentMonth, monthName)

	a.monthSelect = widget.NewSelect(months, func(selected string) {
		var month int
		fmt.Sscanf(selected[:2], "%d", &month)
		a.currentMonth = time.Month(month)
		a.onMonthChanged()
	})
	a.monthSelect.SetSelected(currentMonthStr)

	// Wrap month select in a container with fixed width for full month names
	monthContainer := container.NewMax(a.monthSelect)
	monthContainer.Resize(fyne.NewSize(220, 40))

	// Open folder button
	openFolderBtn := widget.NewButton(a.bundle.T("menu.openTarget"), func() {
		a.openTargetFolder()
	})

	// Settings button
	settingsBtn := widget.NewButton(a.bundle.T("menu.settings"), func() {
		a.showSettingsDialog()
	})

	return container.NewHBox(
		widget.NewLabel(a.bundle.T("app.title")),
		widget.NewLabel(" | "),
		a.yearSelect,
		widget.NewLabel("-"),
		monthContainer,
		widget.NewSeparator(),
		openFolderBtn,
		settingsBtn,
	)
}

// buildLeftPanel creates the drag-and-drop area.
func (a *App) buildLeftPanel() fyne.CanvasObject {
	dropLabel := widget.NewLabel(a.bundle.T("dd.area"))
	dropLabel.Wrapping = fyne.TextWrapWord
	dropLabel.Alignment = fyne.TextAlignCenter

	selectBtn := widget.NewButton(a.bundle.T("select.pdf"), func() {
		a.selectPDFFiles()
	})

	return container.NewVBox(
		widget.NewCard("", "", container.NewVBox(
			dropLabel,
			selectBtn,
		)),
	)
}

// buildRightPanel is no longer needed - table is created in buildUI
// Kept for reference if needed later

// onMonthChanged is called when the year or month selection changes.
func (a *App) onMonthChanged() {
	a.logger.Info("Month changed to %d-%02d", a.currentYear, a.currentMonth)
	a.loadInvoices()
}

// loadInvoices loads invoices for the current month.
func (a *App) loadInvoices() {
	// Safety check in case table isn't initialized yet
	if a.invoiceTable == nil {
		return
	}

	csvPath := a.storageManager.GetCSVPath(a.currentYear, a.currentMonth)
	rows, err := a.csvRepo.Load(csvPath)
	if err != nil {
		a.logger.Error("Failed to load invoices: %v", err)
		a.invoiceTable.SetData([]core.CSVRow{})
		return
	}

	a.invoiceTable.SetData(rows)
}

func (a *App) rewriteAllCSVs() error {
	paths, err := a.storageManager.ListAllCSVPaths()
	if err != nil {
		return err
	}

	for _, path := range paths {
		rows, err := a.csvRepo.Load(path)
		if err != nil {
			return fmt.Errorf("failed to load CSV %s: %w", path, err)
		}
		if err := a.csvRepo.Rewrite(path, rows); err != nil {
			return fmt.Errorf("failed to rewrite CSV %s: %w", path, err)
		}
	}

	return nil
}

// openTargetFolder opens the target folder in the system file manager.
func (a *App) openTargetFolder() {
	folder := a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth)

	// Ensure folder exists
	if err := a.storageManager.EnsureMonthFolder(a.currentYear, a.currentMonth); err != nil {
		a.showError(
			a.bundle.T("error.processing.title"),
			fmt.Sprintf("Failed to create folder: %v", err),
		)
		return
	}

	a.openFolderInOS(folder)
}

// selectPDFFiles is now in filepicker.go

// showSettingsDialog is now in settings.go

// extractPDFData extracts metadata from a PDF (safe to call from background thread).
// Returns Meta and error. UI calls should happen in main thread.
func (a *App) extractPDFData(ctx context.Context, path string) (core.Meta, error) {
	a.logger.Debug("=== PDF EXTRACTION START ===")
	a.logger.Debug("File: %s", path)

	// Extract text
	text, err := a.pdfExtractor.ExtractText(path)
	if err != nil {
		a.logger.Debug("PDF extraction error: %v", err)
		return core.Meta{}, fmt.Errorf("failed to extract text: %w", err)
	}

	a.logger.Info("Extracted %d characters from PDF", len(text))

	if a.settings.DebugMode {
		a.logger.Debug("Text preview (first 1000 chars): %s", truncateString(text, 1000))
	}

	// Check if we have text
	hasText := core.HasText(text)
	if !hasText {
		a.logger.Warn("No text found in PDF (length: %d, might be scanned image)", len(text))
		a.logger.Debug("Full text content: '%s'", text)
		// Return empty meta, caller will handle "no text" case
		return core.Meta{}, fmt.Errorf("no text found in PDF")
	}

	// Extract metadata based on processing mode
	var meta core.Meta
	var confidence float64

	if a.settings.ProcessingMode == "claude" {
		// Get API key from keyring
		apiKey, err := keyring.Get("BuchISY", a.settings.AnthropicAPIKeyRef)
		if err != nil {
			return core.Meta{}, fmt.Errorf("failed to get API key: %w", err)
		}

		meta, confidence, err = a.anthropicExtractor.Extract(ctx, apiKey, a.settings.AnthropicModel, text)
		if err != nil {
			return core.Meta{}, fmt.Errorf("Claude extraction failed: %w", err)
		}
	} else {
		meta, confidence, err = a.localExtractor.Extract(text)
		if err != nil {
			return core.Meta{}, fmt.Errorf("local extraction failed: %w", err)
		}
	}

	a.logger.Info("Extracted metadata with confidence %.2f", confidence)

	if a.settings.DebugMode {
		a.logger.Debug("=== EXTRACTED METADATA ===")
		a.logger.Debug("Company: %s", meta.Firmenname)
		a.logger.Debug("Invoice #: %s", meta.Rechnungsnummer)
		a.logger.Debug("Date: %s", meta.Rechnungsdatum)
		a.logger.Debug("Gross: %.2f %s", meta.Bruttobetrag, meta.Waehrung)
		a.logger.Debug("Net: %.2f", meta.BetragNetto)
		a.logger.Debug("Tax: %.2f%% (%.2f)", meta.SteuersatzProzent, meta.SteuersatzBetrag)
	}

	// Suggest account
	if a.settings.AutoSelectAccount && meta.Firmenname != "" {
		if account, ok := core.SuggestAccountForCompany(a.companyMap, meta.Firmenname, a.settings.DefaultAccount); ok {
			meta.Gegenkonto = account
		} else {
			meta.Gegenkonto = a.settings.DefaultAccount
		}
	} else {
		meta.Gegenkonto = a.settings.DefaultAccount
	}

	return meta, nil
}

// handleNoTextPDF handles PDFs without extractable text (e.g., scanned images).
// Must be called from UI thread.
func (a *App) handleNoTextPDF(path string) {
	// Ask user if they want to enter data manually
	dialog.ShowConfirm(
		a.bundle.T("error.noText"),
		a.bundle.T("error.manualEntry"),
		func(manual bool) {
			if manual {
				// Show empty modal for manual entry
				emptyMeta := core.Meta{
					Waehrung:   a.settings.CurrencyDefault,
					Gegenkonto: a.settings.DefaultAccount,
				}
				a.showConfirmationModal(path, emptyMeta)
			}
		},
		a.window,
	)
}

// showConfirmationModal is now in invoicemodal.go

// Helper functions

func generateYearOptions() []string {
	currentYear := time.Now().Year()
	years := make([]string, 0, 10)
	for i := currentYear - 3; i <= currentYear+3; i++ {
		years = append(years, fmt.Sprintf("%d", i))
	}
	return years
}

func generateMonthOptions(bundle *i18n.Bundle) []string {
	months := make([]string, 12)
	for i := 1; i <= 12; i++ {
		monthKey := fmt.Sprintf("month.%02d", i)
		monthName := bundle.T(monthKey)
		months[i-1] = fmt.Sprintf("%02d - %s", i, monthName)
	}
	return months
}

// truncateString truncates a string to n characters.
func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
