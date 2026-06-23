// Package ui provides the Fyne-based user interface for BuchISY.
package ui

import (
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/assets"
	"github.com/bergx2/buchisy/internal/anthropic"
	"github.com/bergx2/buchisy/internal/core"
	"github.com/bergx2/buchisy/internal/db"
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
	statementAliases   *core.StatementAliasStore
	pdfExtractor       *core.PDFTextExtractor
	localExtractor     *core.LocalExtractor
	anthropicExtractor *anthropic.Extractor
	eInvoiceExtractor  *core.EInvoiceExtractor
	dbRepo             *db.Repository
	csvRepo            *core.CSVRepository // Kept for CSV export
	storageManager     *core.StorageManager
	chartStore         *core.ChartStore
	chart              *core.ChartOfAccounts
	bookingRules       *core.BookingRules
	bookingRulesStore  *core.BookingRulesStore
	bookingTemplates   *core.BookingTemplateStore

	// Current state
	currentYear    int
	currentMonth   time.Month
	viewWholeYear  bool
	cashUncovered  map[string]bool // Dateiname → not covered; recomputed each loadInvoices

	// Main-view mode: "" / "belege" → invoice table (default), "konten"
	// → bank-statement browser per Zahlungskonto.
	viewMode      string
	kontenAccount string // currently selected Zahlungskonto in Konten view
	kontenSortCol string // sort column key for the Konten table
	kontenSortAsc bool   // sort direction

	// Batch entry queue (E17.3): sequential processing of multiple files.
	pendingFiles []string
	batchTotal   int
	batchDone    int

	// UI components
	yearSelect   *highlightedSelect
	monthSelect  *highlightedSelect
	invoiceTable *InvoiceTable
	mainContent  fyne.CanvasObject
	theme        *buchisyTheme
	assetsDir    string
	profile      string

	// Zoom feedback overlay (Task 1)
	scalePopup  *widget.PopUp
	scaleTimer  *time.Timer

	// Dismissed config-hint banners (Task 2): keyed by hint i18n key.
	// Hints dismissed with ✕ stay hidden for the session.
	dismissedHints map[string]bool

	// Empty-state widgets (Task 3): centerWrapper holds either the table or
	// the empty-state; swapped after every loadInvoices() call.
	centerWrapper *fyne.Container
	emptyState    fyne.CanvasObject
}

// New creates the BuchISY application and shows the profile picker.
func New(assetsDir string) (*App, error) {
	fyneApp := app.NewWithID("com.bergx2.buchisy")
	a := &App{
		app:            fyneApp,
		assetsDir:      assetsDir,
		window:         fyneApp.NewWindow("BuchISY"),
		dismissedHints: make(map[string]bool),
	}
	// Create the theme up-front and wire Ctrl+scroll / Ctrl+plus zoom
	// keyboard handlers before any window is shown, so the profile
	// picker can resize too. startProfile later re-scales this same
	// theme to the profile's saved value. The initial scale comes from
	// app preferences so the picker opens at the last zoom the user
	// chose (independent of profile).
	prefs := fyneApp.Preferences()
	initialScale := float32(1.0)
	if saved := prefs.Float("ui_scale"); saved > 0 {
		initialScale = float32(saved)
	}
	a.theme = newBuchisyTheme(initialScale)

	// Restore last-used Konten table sort (app-wide, not per profile —
	// users typically prefer the same ordering everywhere).
	a.kontenSortCol = prefs.String("konten_sort_col")
	a.kontenSortAsc = prefs.BoolWithFallback("konten_sort_asc", true)
	a.app.Settings().SetTheme(a.theme)
	a.registerZoomShortcuts()
	a.registerCtrlScrollZoom()
	a.showProfilePicker()
	return a, nil
}

// startProfile initializes the application for the chosen profile and
// replaces the window content with the main UI.
func (a *App) startProfile(profile string) {
	a.profile = profile

	configDir, err := core.GetProfileConfigDir(profile)
	if err != nil {
		dialog.ShowError(fmt.Errorf("Konfigurationsverzeichnis nicht ermittelbar: %w", err), a.window)
		return
	}

	logDir := filepath.Join(configDir, "logs")
	logger, err := logging.New(logDir, logging.INFO)
	if err != nil {
		dialog.ShowError(fmt.Errorf("Logger-Initialisierung fehlgeschlagen: %w", err), a.window)
		return
	}
	logger.Info("Starting BuchISY profile: %s", profile)

	settingsPath := filepath.Join(configDir, "settings.json")
	settingsMgr := core.NewSettingsManager(settingsPath)
	settings, err := settingsMgr.Load()
	if err != nil {
		logger.Warn("Failed to load settings, using defaults: %v", err)
		settings = core.DefaultSettings()
	}

	if settings.StorageRoot == "" {
		docsDir, err := core.GetDocumentsDir()
		if err != nil {
			logger.Warn("Failed to get documents directory: %v", err)
		} else {
			settings.StorageRoot = filepath.Join(docsDir, "BuchISY")
		}
	}

	uiScale := settings.UIScale
	if uiScale <= 0 {
		uiScale = 1.0
	}
	// Re-scale the shared theme to this profile's saved zoom level.
	// (Theme + Ctrl+scroll were already registered in App.New so the
	// profile picker has them too.)
	a.theme.SetScale(uiScale)
	a.app.Settings().SetTheme(a.theme)

	bundle, err := i18n.Load(a.assetsDir, settings.Language)
	if err != nil {
		logger.Warn("Failed to load translations: %v", err)
		bundle = &i18n.Bundle{}
	}

	companyMap := core.NewCompanyAccountMap(configDir)
	if err := companyMap.Load(); err != nil {
		logger.Warn("Failed to load company account map: %v", err)
	}

	statementAliases := core.NewStatementAliasStore(configDir)
	if _, err := statementAliases.Load(); err != nil {
		logger.Warn("Failed to load statement aliases: %v", err)
	}

	if settings.DebugMode {
		logger.SetLevel(logging.DEBUG)
		logger.Debug("Debug mode enabled")
	}

	pdfExtractor := core.NewPDFTextExtractor()
	localExtractor := core.NewLocalExtractor()
	anthropicExtractor := anthropic.NewExtractor(logger, settings.DebugMode)
	eInvoiceExtractor := core.NewEInvoiceExtractor()

	// Initialize SQLite database (global database for all invoices)
	dbPath := db.GetGlobalDBPath(configDir)
	dbRepo, err := db.NewRepository(dbPath)
	if err != nil {
		dialog.ShowError(fmt.Errorf("failed to initialize database: %w", err), a.window)
		return
	}
	logger.Info("Initialized SQLite database: %s", dbPath)

	// CSV repository for export and migration
	csvRepo := core.NewCSVRepository()
	csvRepo.SetColumnOrder(settings.ColumnOrder)
	csvRepo.SetSeparator(settings.CSVSeparator)
	csvRepo.SetEncoding(settings.CSVEncoding)
	csvRepo.SetDecimalSeparator(settings.DecimalSeparator)
	storageManager := core.NewStorageManager(&settings)

	// One-time, idempotent storage migrations.
	warn := func(msg string) { logger.Warn("%s", msg) }
	if err := storageManager.MigrateToYearFolders(warn); err != nil {
		logger.Warn("Year-folder migration failed: %v", err)
	}
	cashAccounts := make(map[string]struct{})
	for _, ba := range settings.BankAccounts {
		if ba.AccountType == core.AccountTypeCash {
			cashAccounts[ba.Name] = struct{}{}
		}
	}
	if err := storageManager.MigrateCashToBar(csvRepo, cashAccounts, warn); err != nil {
		logger.Warn("Bar migration failed: %v", err)
	}

	// Back-fill the database from existing CSVs the first time a profile runs
	// on the SQLite build (no-op once the database holds invoices). Without
	// this, a profile migrated from the CSV-only era shows an empty table.
	if imported, err := dbRepo.MigrateCSVToDatabase(settings.StorageRoot, csvRepo, logger); err != nil {
		logger.Warn("CSV-to-database import failed: %v", err)
	} else if imported > 0 {
		logger.Info("Imported %d invoices from CSV into the database", imported)
	}

	now := time.Now()

	a.bundle = bundle
	a.logger = logger
	a.settings = settings
	a.settingsMgr = settingsMgr
	a.companyMap = companyMap
	a.statementAliases = statementAliases
	a.pdfExtractor = pdfExtractor
	a.localExtractor = localExtractor
	a.anthropicExtractor = anthropicExtractor
	a.eInvoiceExtractor = eInvoiceExtractor
	a.dbRepo = dbRepo
	a.csvRepo = csvRepo
	a.storageManager = storageManager
	a.currentYear = now.Year()
	a.currentMonth = now.Month()

	a.chartStore = core.NewChartStore(configDir, assets.SKR04JSON)
	if chart, err := a.chartStore.Load(); err != nil {
		logger.Warn("Failed to load chart of accounts: %v", err)
		a.chart = core.NewChartOfAccounts(nil)
	} else {
		a.chart = chart
	}
	a.anthropicExtractor.SetAccountHints(a.chart.All())

	a.bookingRulesStore = core.NewBookingRulesStore(configDir, assets.BuchungsregelnJSON)
	if rules, err := a.bookingRulesStore.Load(); err != nil {
		logger.Warn("Failed to load booking rules: %v", err)
		a.bookingRules = &core.BookingRules{}
	} else {
		a.bookingRules = rules
	}
	a.bookingTemplates = core.NewBookingTemplateStore(configDir)
	if err := a.bookingTemplates.Load(); err != nil {
		logger.Warn("Failed to load booking templates: %v", err)
	}

	// One folder per Zahlungskonto, created at <StorageRoot>/<Name>/.
	a.ensureAccountFolders()

	a.window.SetTitle("BuchISY — " + profile)
	if settings.WindowWidth > 0 && settings.WindowHeight > 0 {
		a.window.Resize(fyne.NewSize(float32(settings.WindowWidth), float32(settings.WindowHeight)))
	} else {
		a.window.Resize(fyne.NewSize(1500, 875))
	}
	a.window.CenterOnScreen()

	a.window.SetContent(a.buildUI())

	// Start watching the scan-inbox folder for new PDFs.
	newScanWatcher(a).start()
}

// keyringAccount returns the OS-keyring account name for the active
// profile's API key, e.g. "Bergx2-claude".
func (a *App) keyringAccount() string {
	return a.profile + "-" + a.settings.AnthropicAPIKeyRef
}

// hasAPIKey reports whether a non-empty Claude API key is stored in the
// OS keyring for the active profile.
func (a *App) hasAPIKey() bool {
	val, err := keyring.Get("BuchISY", a.keyringAccount())
	return err == nil && val != ""
}

// ownVATIDList parses Settings.OwnVATID into a slice of VAT-IDs.
// The setting accepts a comma- (or newline-) separated list so that
// users with multiple companies (e.g. Bergx2 + Boomstraat) can exclude
// all of them from the auto-extraction.
func (a *App) ownVATIDList() []string {
	raw := a.settings.OwnVATID
	if raw == "" {
		return nil
	}
	// Normalize separators: replace newlines and semicolons with commas.
	raw = strings.NewReplacer("\n", ",", "\r", ",", ";", ",").Replace(raw)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// registerZoomShortcuts wires Ctrl+Plus / Ctrl+Minus / Ctrl+0 to the
// theme scale and persists the change.
func (a *App) registerZoomShortcuts() {
	canvas := a.window.Canvas()

	canvas.AddShortcut(
		&desktop.CustomShortcut{KeyName: fyne.KeyPlus, Modifier: fyne.KeyModifierControl},
		func(fyne.Shortcut) { a.adjustUIScale(UIScaleStep) },
	)
	// Equal key covers US layouts where "+" lives on Shift+= and the
	// Shift modifier is consumed before reporting "Plus".
	canvas.AddShortcut(
		&desktop.CustomShortcut{KeyName: fyne.KeyEqual, Modifier: fyne.KeyModifierControl},
		func(fyne.Shortcut) { a.adjustUIScale(UIScaleStep) },
	)
	canvas.AddShortcut(
		&desktop.CustomShortcut{KeyName: fyne.KeyAsterisk, Modifier: fyne.KeyModifierControl},
		func(fyne.Shortcut) { a.adjustUIScale(UIScaleStep) },
	)
	canvas.AddShortcut(
		&desktop.CustomShortcut{KeyName: fyne.KeyMinus, Modifier: fyne.KeyModifierControl},
		func(fyne.Shortcut) { a.adjustUIScale(-UIScaleStep) },
	)
	canvas.AddShortcut(
		&desktop.CustomShortcut{KeyName: fyne.Key0, Modifier: fyne.KeyModifierControl},
		func(fyne.Shortcut) { a.setUIScale(1.0) },
	)
}

// adjustUIScale changes the zoom by delta and refreshes the UI.
func (a *App) adjustUIScale(delta float32) {
	a.setUIScale(a.theme.Scale() + delta)
}

// registerCtrlScrollZoom enables Ctrl + mouse-wheel zooming by adding
// a transparent scroll-only overlay on top of the canvas while a Ctrl
// key is held. The overlay intercepts wheel events (Fyne routes them
// to the topmost Scrollable at the cursor) and feeds them into the
// zoom logic. Because it does not implement Tappable or Hoverable,
// clicks and hover effects fall through unaffected.
func (a *App) registerCtrlScrollZoom() {
	deskCanvas, ok := a.window.Canvas().(desktop.Canvas)
	if !ok {
		return // non-desktop driver: silently skip
	}

	overlay := newCtrlScrollOverlay(func(ev *fyne.ScrollEvent) {
		if ev.Scrolled.DY > 0 {
			a.adjustUIScale(UIScaleStep)
		} else if ev.Scrolled.DY < 0 {
			a.adjustUIScale(-UIScaleStep)
		}
	})

	var visible bool
	show := func() {
		if visible {
			return
		}
		visible = true
		overlay.Resize(a.window.Canvas().Size())
		a.window.Canvas().Overlays().Add(overlay)
	}
	hide := func() {
		if !visible {
			return
		}
		visible = false
		a.window.Canvas().Overlays().Remove(overlay)
	}

	isCtrl := func(name fyne.KeyName) bool {
		return name == desktop.KeyControlLeft || name == desktop.KeyControlRight
	}

	deskCanvas.SetOnKeyDown(func(ev *fyne.KeyEvent) {
		if isCtrl(ev.Name) {
			show()
		}
	})
	deskCanvas.SetOnKeyUp(func(ev *fyne.KeyEvent) {
		if isCtrl(ev.Name) {
			hide()
		}
	})
}

// setupModalCtrlScroll wires Ctrl+wheel inside a secondary window
// (confirmation / edit dialog) so that scrolling over the form area
// adjusts the app's UI scale, while scrolling over the preview area
// zooms the preview. Uses the same transparent-overlay pattern as the
// main window so wheel events are intercepted only while Ctrl is held.
func (a *App) setupModalCtrlScroll(win fyne.Window, preview fyne.CanvasObject, stripFn func() *pdfPreviewStrip) {
	deskCanvas, ok := win.Canvas().(desktop.Canvas)
	if !ok {
		return
	}

	overlay := newCtrlScrollOverlay(func(ev *fyne.ScrollEvent) {
		var strip *pdfPreviewStrip
		if stripFn != nil {
			strip = stripFn()
		}
		if preview != nil && strip != nil && isOverObject(ev.AbsolutePosition, preview) {
			if ev.Scrolled.DY > 0 {
				strip.setZoom(strip.zoom + previewZoomStep)
			} else if ev.Scrolled.DY < 0 {
				strip.setZoom(strip.zoom - previewZoomStep)
			}
			return
		}
		if ev.Scrolled.DY > 0 {
			a.adjustUIScale(UIScaleStep)
		} else if ev.Scrolled.DY < 0 {
			a.adjustUIScale(-UIScaleStep)
		}
	})

	var visible bool
	show := func() {
		if visible {
			return
		}
		visible = true
		overlay.Resize(win.Canvas().Size())
		win.Canvas().Overlays().Add(overlay)
	}
	hide := func() {
		if !visible {
			return
		}
		visible = false
		win.Canvas().Overlays().Remove(overlay)
	}
	isCtrl := func(n fyne.KeyName) bool {
		return n == desktop.KeyControlLeft || n == desktop.KeyControlRight
	}
	deskCanvas.SetOnKeyDown(func(ev *fyne.KeyEvent) {
		if isCtrl(ev.Name) {
			show()
		}
	})
	deskCanvas.SetOnKeyUp(func(ev *fyne.KeyEvent) {
		if isCtrl(ev.Name) {
			hide()
		}
	})
}

// persistKontenSort writes the current Konten table sort state to
// app-wide preferences so it survives app restarts.
func (a *App) persistKontenSort() {
	prefs := a.app.Preferences()
	prefs.SetString("konten_sort_col", a.kontenSortCol)
	prefs.SetBool("konten_sort_asc", a.kontenSortAsc)
}

// persistInvoiceSort does the same for the Belege table.
func (a *App) persistInvoiceSort(col string, asc bool) {
	prefs := a.app.Preferences()
	prefs.SetString("invoice_sort_col", col)
	prefs.SetBool("invoice_sort_asc", asc)
}

// isOverObject reports whether absPos (canvas-absolute coordinates) lies
// within obj's current screen rectangle.
func isOverObject(absPos fyne.Position, obj fyne.CanvasObject) bool {
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(obj)
	size := obj.Size()
	return absPos.X >= pos.X && absPos.X < pos.X+size.Width &&
		absPos.Y >= pos.Y && absPos.Y < pos.Y+size.Height
}

// showScaleOverlay displays a brief "125 %" centered popup that auto-hides
// after ~900 ms. Rapid zooming reuses a single PopUp and replaces the timer
// so only the most-recent timer dismisses it.
func (a *App) showScaleOverlay() {
	if a.window == nil {
		return
	}
	txt := fmt.Sprintf("%.0f %%", a.theme.Scale()*100)
	canvas := a.window.Canvas()

	lbl := widget.NewLabel(txt)
	lbl.TextStyle = fyne.TextStyle{Bold: true}

	if a.scalePopup == nil {
		padded := container.NewPadded(lbl)
		a.scalePopup = widget.NewPopUp(padded, canvas)
	} else {
		// Reuse popup — update the label text in-place.
		if padded, ok := a.scalePopup.Content.(*fyne.Container); ok {
			if existing, ok2 := padded.Objects[0].(*widget.Label); ok2 {
				existing.SetText(txt)
			}
		}
		a.scalePopup.Show()
	}

	// Center the popup on the canvas.
	canvasSize := canvas.Size()
	popupSize := a.scalePopup.MinSize()
	a.scalePopup.Move(fyne.NewPos(
		(canvasSize.Width-popupSize.Width)/2,
		(canvasSize.Height-popupSize.Height)/2,
	))
	a.scalePopup.Show()

	// Cancel any previous timer so a stale hide cannot close a freshly shown popup.
	if a.scaleTimer != nil {
		a.scaleTimer.Stop()
	}
	popup := a.scalePopup
	a.scaleTimer = time.AfterFunc(900*time.Millisecond, func() {
		fyne.Do(func() {
			if a.scalePopup == popup {
				a.scalePopup.Hide()
			}
		})
	})
}

// setUIScale applies the given zoom factor, persists it, and refreshes
// the UI. The scale is saved app-wide via Fyne preferences (so the
// profile picker opens at the last zoom) and, when a profile is
// active, also in that profile's settings.json.
func (a *App) setUIScale(scale float32) {
	if a.theme == nil {
		return
	}
	a.theme.SetScale(scale)
	a.app.Settings().SetTheme(a.theme) // triggers global refresh

	// App-wide preference — survives across profile starts and is read
	// in App.New before any profile is selected.
	a.app.Preferences().SetFloat("ui_scale", float64(a.theme.Scale()))

	if a.settingsMgr == nil {
		return
	}
	a.settings.UIScale = a.theme.Scale()
	if err := a.settingsMgr.Save(a.settings); err != nil {
		if a.logger != nil {
			a.logger.Warn("Failed to persist UI scale: %v", err)
		}
	} else if a.logger != nil {
		a.logger.Debug("UI scale set to %.2f", a.settings.UIScale)
	}

	a.showScaleOverlay()
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
	if a.dbRepo != nil {
		_ = a.dbRepo.Close()
	}
	if a.logger != nil {
		_ = a.logger.Close()
	}
}

// saveWindowState saves the current window size.
// Note: Position saving is not supported in Fyne v2, OS handles window position memory.
func (a *App) saveWindowState() {
	if a.settingsMgr == nil || a.logger == nil {
		return // no profile selected yet
	}
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

// buildUI constructs the main UI layout. Dispatches to the Konten view
// when that mode is active; otherwise builds the invoice ("Belege") view.
func (a *App) buildUI() fyne.CanvasObject {
	a.applyAccentForMode()
	if a.viewMode == "konten" {
		a.mainContent = a.buildKontenUI()
		return a.mainContent
	}
	// Create table first (before top bar, so callbacks don't crash)
	a.invoiceTable = NewInvoiceTable(a.bundle, a)
	a.invoiceTable.SetColumnOrder(a.settings.ColumnOrder)
	a.invoiceTable.SetDecimalSeparator(a.settings.DecimalSeparator)
	a.invoiceTable.SetWindow(a.window)

	// Top bar (this may trigger callbacks when setting initial values).
	// It hosts the upload drop field on the left and all other controls
	// on the right; the table fills the rest of the window.
	topBar := a.buildTopBar()

	// OS-level file drops anywhere on the window: in Belege mode they
	// kick off invoice extraction; in Konten mode they're filed as a
	// Kontoauszug for the currently selected Zahlungskonto.
	a.window.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		if a.viewMode == "konten" {
			// In Konten mode: file only the first supported statement.
			for _, uri := range uris {
				path := uri.Path()
				if core.IsSupportedFile(filepath.Base(path)) {
					a.fileStatement(path)
					return
				}
			}
			return
		}
		// In Belege mode: enqueue ALL supported files for sequential entry.
		var paths []string
		for _, uri := range uris {
			path := uri.Path()
			if core.IsSupportedFile(filepath.Base(path)) {
				paths = append(paths, path)
			}
		}
		if len(paths) > 0 {
			a.enqueueSubmissions(paths)
		}
	})

	// Load initial data now that everything is set up
	a.loadInvoices()

	// Header (top bar + filter + chip row) so the search input lives at
	// window-level instead of nested above the table.
	filterRow := container.NewBorder(nil, nil,
		a.invoiceTable.ChipRow(), nil,
		a.invoiceTable.FilterEntry())

	// Config-hint banners: one slim dismissible row per unmet precondition.
	headerObjects := []fyne.CanvasObject{}
	for _, hintKey := range core.MissingConfigHints(a.settings, a.hasAPIKey()) {
		hintKey := hintKey // capture
		if a.dismissedHints[hintKey] {
			continue
		}
		lbl := widget.NewLabel(a.bundle.T(hintKey))
		lbl.Wrapping = fyne.TextWrapWord
		settingsBtn := widget.NewButton(a.bundle.T("menu.settings"), func() {
			a.showSettingsView()
		})
		settingsBtn.Importance = widget.MediumImportance
		dismissBtn := widget.NewButton("✕ "+a.bundle.T("hint.dismiss"), func() {
			a.dismissedHints[hintKey] = true
			a.showMainView()
		})
		dismissBtn.Importance = widget.LowImportance
		row := container.NewBorder(nil, nil,
			widget.NewIcon(theme.WarningIcon()),
			container.NewHBox(settingsBtn, dismissBtn),
			lbl,
		)
		headerObjects = append(headerObjects, row, widget.NewSeparator())
	}
	headerObjects = append(headerObjects, topBar, filterRow)
	header := container.NewVBox(headerObjects...)

	// Empty-state shown when the selected month has zero invoices.
	emptyTitle := widget.NewLabelWithStyle(
		a.bundle.T("empty.title"),
		fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true},
	)
	emptyHint := widget.NewLabelWithStyle(
		a.bundle.T("empty.hint"),
		fyne.TextAlignCenter,
		fyne.TextStyle{},
	)
	emptyBtn := widget.NewButtonWithIcon(
		a.bundle.T("empty.add"),
		theme.ContentAddIcon(),
		func() { a.showCustomFilePicker() },
	)
	emptyBtn.Importance = widget.HighImportance
	a.emptyState = container.NewCenter(container.NewVBox(emptyTitle, emptyHint, emptyBtn))

	// centerWrapper holds either the table or the empty-state; swapped in loadInvoices.
	a.centerWrapper = container.NewStack(a.invoiceTable.Container())

	a.mainContent = container.NewBorder(header, a.buildStatusBar(), nil, nil, a.centerWrapper)
	return a.mainContent
}

// buildStatusBar renders a slim footer with profile + record count
// info, giving the user a constant orientation cue.
func (a *App) buildStatusBar() fyne.CanvasObject {
	prof := a.profile
	if prof == "" {
		prof = "—"
	}
	period := fmt.Sprintf("%04d / %02d", a.currentYear, int(a.currentMonth))
	if a.viewWholeYear {
		period = fmt.Sprintf("%04d (Ganzes Jahr)", a.currentYear)
	}
	count := 0
	if a.invoiceTable != nil {
		count = len(a.invoiceTable.filtered)
	}
	text := fmt.Sprintf("  %s   •   %s   •   %d Belege", prof, period, count)
	lbl := widget.NewLabel(text)

	bg := canvas.NewRectangle(cardBackgroundColor())
	bar := container.NewStack(bg, container.NewPadded(lbl))
	return container.NewBorder(widget.NewSeparator(), nil, nil, nil, bar)
}

// showMainView swaps the window content back to the main view, always
// rebuilding it fresh. Settings changes (new bank accounts, column
// order, processing mode, …) need this to take effect immediately
// without a restart — otherwise the cached top-bar / dropdowns stay
// stale.
func (a *App) showMainView() {
	a.applyAccentForMode()
	a.window.SetContent(a.buildUI())
}

// addDialogShortcuts wires Strg+S → save and Esc → close on the
// given secondary window. Use in showEditDialog / showConfirmationModal
// so power users can drive them from the keyboard.
func (a *App) addDialogShortcuts(win fyne.Window, save, cancel func()) {
	cv := win.Canvas()
	if cv == nil {
		return
	}
	if save != nil {
		cv.AddShortcut(
			&desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierControl},
			func(fyne.Shortcut) { save() },
		)
	}
	if cancel != nil {
		// Escape isn't a standard "shortcut" — wire via key handler.
		cv.SetOnTypedKey(func(ev *fyne.KeyEvent) {
			if ev.Name == fyne.KeyEscape {
				cancel()
			}
		})
	}
}

// applyAccentForMode swaps the theme's accent colour based on the
// current view mode so HighImportance buttons / sliders pick up the
// Belege-blue or Konten-green identity.
func (a *App) applyAccentForMode() {
	if a.theme == nil {
		return
	}
	if a.viewMode == "konten" {
		a.theme.SetAccent(accentKonten)
	} else {
		a.theme.SetAccent(accentBelege)
	}
	a.app.Settings().SetTheme(a.theme)
}

// buildTopBar creates the top toolbar.
func (a *App) buildTopBar() fyne.CanvasObject {
	// "Now" markers used to draw a thin border around today's year/month
	// inside the dropdown popups, regardless of which period is selected.
	now := time.Now()
	nowYearStr := fmt.Sprintf("%d", now.Year())
	nowMonthStr := fmt.Sprintf("%02d - %-12s", now.Month(),
		a.bundle.T(fmt.Sprintf("month.%02d", now.Month())))

	// Year selector
	years := generateYearOptions()
	currentYearStr := fmt.Sprintf("%d", a.currentYear)
	a.yearSelect = newHighlightedSelect(years, nowYearStr, func(selected string) {
		var year int
		_, _ = fmt.Sscanf(selected, "%d", &year)
		a.currentYear = year
		a.onMonthChanged()
	})
	a.yearSelect.SetSelected(currentYearStr)

	// Month selector — prepend a "Ganzes Jahr" option that switches the
	// table to show invoices from all 12 months of the selected year.
	months := append([]string{wholeYearOption}, generateMonthOptions(a.bundle)...)
	// Build the full string to match the dropdown options (e.g., "09 - September")
	monthKey := fmt.Sprintf("month.%02d", a.currentMonth)
	monthName := a.bundle.T(monthKey)
	currentMonthStr := fmt.Sprintf("%02d - %-12s", a.currentMonth, monthName)

	a.monthSelect = newHighlightedSelect(months, nowMonthStr, func(selected string) {
		if selected == wholeYearOption {
			a.viewWholeYear = true
			a.onMonthChanged()
			return
		}
		var month int
		fmt.Sscanf(selected[:2], "%d", &month)
		a.currentMonth = time.Month(month)
		a.viewWholeYear = false
		a.onMonthChanged()
	})
	a.monthSelect.SetSelected(currentMonthStr)

	// Wrap month select in a container with minimum width
	monthContainer := container.NewStack(a.monthSelect)

	// Settings button (always visible — top-right corner with gear icon)
	settingsBtn := widget.NewButtonWithIcon("", theme.SettingsIcon(), func() {
		a.showSettingsView()
	})
	settingsBtn.Importance = widget.LowImportance

	// Overflow menu — secondary actions tucked behind a three-dot button.
	overflowBtn := widget.NewButtonWithIcon("", theme.MoreVerticalIcon(), nil)
	overflowBtn.Importance = widget.LowImportance
	overflowBtn.OnTapped = func() {
		menu := fyne.NewMenu("",
			fyne.NewMenuItem(a.bundle.T("menu.openTarget"), func() { a.openTargetFolder() }),
			fyne.NewMenuItem("Mehrere Belege importieren…", func() {
				a.showFilesPicker(func(paths []string) { a.enqueueSubmissions(paths) })
			}),
			fyne.NewMenuItem("Kassenbuch", func() { a.showCashBookView() }),
			fyne.NewMenuItem("CSV-Export", func() { a.showCSVExportDialog() }),
			fyne.NewMenuItem("Buchungen exportieren", func() { a.showBookingExportDialog() }),
			fyne.NewMenuItem("Controlling", func() { a.showControllingDialog() }),
			fyne.NewMenuItem("USt-Voranmeldung", func() { a.showUStVADialog() }),
			fyne.NewMenuItem("Zusammenfassende Meldung", func() { a.showZMDialog() }),
			fyne.NewMenuItem("Belegliste (PDF)", func() { a.showBelegListePDF() }),
			fyne.NewMenuItem("Rechnungsausgangsbuch (PDF)", func() { a.showSalesJournalPDF() }),
			fyne.NewMenuItem("Belegabgleich", func() { a.showBelegabgleich() }),
			fyne.NewMenuItem("Erlös-Abgleich", func() { a.showErloesAbgleich() }),
			fyne.NewMenuItem("Belegnummern neu vergeben", func() { a.renumberBelegnummern() }),
			fyne.NewMenuItem("Backup erstellen", func() { a.showBackup() }),
		)
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(overflowBtn)
		pos.Y += overflowBtn.Size().Height
		widget.ShowPopUpMenuAtPosition(menu, a.window.Canvas(), pos)
	}

	// Constrain the year select to a tighter width so the 4-digit value
	// doesn't claim as much horizontal space as the long month names.
	yearWrap := container.New(fixedWidthLayout{width: 90}, a.yearSelect)

	// Upload card — feels like a real drop-zone now: subtle card
	// background, upload icon, label and the two action buttons.
	uploadIcon := widget.NewIcon(theme.UploadIcon())
	uploadLabel := widget.NewLabel(a.bundle.T("dd.upload"))
	uploadLabel.TextStyle = fyne.TextStyle{Bold: true}

	pasteBtn := widget.NewButtonWithIcon("", theme.ContentPasteIcon(), func() {
		a.pasteFromClipboard()
	})
	pasteBtn.Importance = widget.LowImportance

	moreBtn := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() { a.selectPDFFiles() })
	moreBtn.Importance = widget.LowImportance

	uploadButtons := container.NewHBox(pasteBtn, moreBtn)
	uploadInner := container.NewBorder(nil, nil,
		container.NewHBox(uploadIcon, uploadLabel),
		uploadButtons)
	uploadBg := canvas.NewRectangle(cardBackgroundColor())
	uploadBg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	uploadBg.StrokeWidth = 1
	uploadBg.CornerRadius = 6
	uploadBoxRaw := container.NewStack(uploadBg, container.NewPadded(uploadInner))

	uploadBox := newContextMenuWrap(uploadBoxRaw, func(e *fyne.PointEvent) {
		menu := fyne.NewMenu("",
			fyne.NewMenuItem("Einfügen", func() { a.pasteFromClipboard() }),
		)
		widget.ShowPopUpMenuAtPosition(menu, a.window.Canvas(), e.AbsolutePosition)
	})

	rightControls := container.NewHBox(
		yearWrap,
		widget.NewLabel("-"),
		monthContainer,
		widget.NewSeparator(),
		overflowBtn,
		settingsBtn,
	)

	belegeBtn, kontenBtn := a.viewToggleButtons()
	leftGroup := container.NewHBox(belegeBtn, widget.NewSeparator(), uploadBox, widget.NewSeparator(), kontenBtn)

	return container.NewBorder(nil, nil, leftGroup, rightControls)
}

// pasteFromClipboard handles two clipboard contents:
//  1. file paths (Explorer "Copy") → process the first supported file
//  2. raw images (Snipping Tool, screenshots) → save as a temp PNG
//     and process that
//
// Falls back to a friendly info dialog when neither yields anything.
func (a *App) pasteFromClipboard() {
	for _, f := range clipboardFiles() {
		if core.IsSupportedFile(filepath.Base(f)) {
			a.processSubmission(f, nil, nil)
			return
		}
	}

	img, decodeNotes := clipboardImageDiagnostic()
	if img != nil {
		tmp, err := os.CreateTemp("", "buchisy-paste-*.png")
		if err != nil {
			a.logger.Error("Clipboard paste: temp file: %v", err)
			a.showError("Zwischenablage", "Konnte Temp-Datei nicht anlegen: "+err.Error())
			return
		}
		if err := png.Encode(tmp, img); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			a.logger.Error("Clipboard paste: PNG encode: %v", err)
			a.showError("Zwischenablage", "Konnte Bild nicht speichern: "+err.Error())
			return
		}
		tmp.Close()
		a.processSubmission(tmp.Name(), nil, nil)
		return
	}

	formats := clipboardFormatsDiagnostic()
	a.logger.Warn("Clipboard paste: no usable file or image. Formats: %s. Decode attempts: %s",
		formats, decodeNotes)
	dialog.ShowInformation("Zwischenablage",
		"In der Zwischenablage wurde weder eine Datei noch ein Bild gefunden.\n\n"+
			"Aktuell liegt dort:\n"+formats+"\n\n"+
			"Kopiere eine Datei (Strg+C im Explorer) oder ein Bild (z. B. aus "+
			"dem Snipping Tool, Browser oder Bildbetrachter) und versuche es erneut.",
		a.window)
}

// showBelegListePDF builds an invoice list PDF for the current month and
// opens a save dialog so the user can store it wherever they like.
func (a *App) showBelegListePDF() {
	period := fmt.Sprintf("%04d-%02d", a.currentYear, int(a.currentMonth))
	rows := a.collectInvoiceRows(a.currentYear, int(a.currentMonth), a.currentYear, int(a.currentMonth))
	data, err := core.BuildInvoiceListPDF(rows, "Belegliste "+period)
	if err != nil {
		a.showError(a.bundle.T("error.processing.title"), err.Error())
		return
	}
	a.savePDF("Belegliste_"+period+".pdf", data)
}

// showSalesJournalPDF exports the Rechnungsausgangsbuch — a monthly list of all
// outgoing invoices (Ausgangsrechnungen) with per-invoice net/VAT/gross + totals.
func (a *App) showSalesJournalPDF() {
	period := fmt.Sprintf("%04d-%02d", a.currentYear, int(a.currentMonth))
	rows := a.collectInvoiceRows(a.currentYear, int(a.currentMonth), a.currentYear, int(a.currentMonth))
	data, err := core.BuildSalesJournalPDF(rows, a.chart, "Rechnungsausgangsbuch "+period)
	if err != nil {
		a.showError(a.bundle.T("error.processing.title"), err.Error())
		return
	}
	a.savePDF("Rechnungsausgangsbuch_"+period+".pdf", data)
}

// contextMenuWrap wraps any CanvasObject and forwards right-clicks to
// `onSecondary`. Used to give the upload field a "Einfügen"-Menü
// without turning the whole box into a button.
type contextMenuWrap struct {
	widget.BaseWidget
	child       fyne.CanvasObject
	onSecondary func(*fyne.PointEvent)
}

func newContextMenuWrap(child fyne.CanvasObject, onSecondary func(*fyne.PointEvent)) *contextMenuWrap {
	w := &contextMenuWrap{child: child, onSecondary: onSecondary}
	w.ExtendBaseWidget(w)
	return w
}

func (w *contextMenuWrap) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.child)
}

func (w *contextMenuWrap) TappedSecondary(e *fyne.PointEvent) {
	if w.onSecondary != nil {
		w.onSecondary(e)
	}
}

// fixedWidthLayout pins its children to a specific width while letting
// the height come from the child's MinSize. Used to make the year-select
// noticeably narrower than the month-select without truncating the year.
type fixedWidthLayout struct {
	width float32
}

func (l fixedWidthLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range objs {
		if mh := o.MinSize().Height; mh > h {
			h = mh
		}
	}
	return fyne.NewSize(l.width, h)
}

func (l fixedWidthLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Resize(fyne.NewSize(l.width, size.Height))
		o.Move(fyne.NewPos(0, 0))
	}
}

// buildRightPanel is no longer needed - table is created in buildUI
// Kept for reference if needed later

// onMonthChanged is called when the year or month selection changes.
func (a *App) onMonthChanged() {
	if a.viewWholeYear {
		a.logger.Info("View changed to year %d (all months)", a.currentYear)
	} else {
		a.logger.Info("Month changed to %d-%02d", a.currentYear, a.currentMonth)
	}
	a.loadInvoices()
}

// renumberBelegnummern reassigns all receipt numbers chronologically per year
// (gap-free), after a confirmation — for backfilling legacy invoices and closing
// gaps left by deletions.
func (a *App) renumberBelegnummern() {
	dialog.ShowConfirm("Belegnummern neu vergeben",
		"Alle Belege werden pro Jahr chronologisch (nach Rechnungsdatum) lückenlos neu nummeriert (JJJJ-NNNN). Bestehende Belegnummern werden dabei überschrieben.\n\nFortfahren?",
		func(ok bool) {
			if !ok {
				return
			}
			n, err := a.dbRepo.RenumberBelegnummern()
			if err != nil {
				a.showError(a.bundle.T("error.processing.title"), err.Error())
				return
			}
			// Refresh the current month's CSV export + the table.
			csvPath := a.storageManager.GetCSVPath(a.currentYear, a.currentMonth)
			if err := a.dbRepo.ExportToCSV(fmt.Sprintf("%04d", a.currentYear), fmt.Sprintf("%02d", int(a.currentMonth)), csvPath, a.csvRepo); err != nil {
				a.logger.Warn("Failed to export CSV after renumber: %v", err)
			}
			a.loadInvoices()
			a.showToast(fmt.Sprintf("%d Belege neu nummeriert", n))
		}, a.window)
}

// loadInvoices loads invoices from the SQLite database for the current
// month, or for the entire year if "Ganzes Jahr" is selected in the
// month dropdown.
func (a *App) loadInvoices() {
	// Safety check in case table isn't initialized yet
	if a.invoiceTable == nil {
		return
	}

	jahr := fmt.Sprintf("%04d", a.currentYear)

	if a.viewWholeYear {
		var all []core.CSVRow
		for m := time.January; m <= time.December; m++ {
			monat := fmt.Sprintf("%02d", int(m))
			monthRows, err := a.dbRepo.List(jahr, monat)
			if err != nil {
				a.logger.Error("Failed to load invoices for %d-%02d: %v", a.currentYear, int(m), err)
				continue
			}
			all = append(all, monthRows...)
		}
		a.annotateAttachments(all)
		// Rebuild coverage for every displayed month.
		a.cashUncovered = map[string]bool{}
		for m := time.January; m <= time.December; m++ {
			for _, acct := range a.cashAccounts() {
				books, _ := core.LoadCashBooks(filepath.Join(a.storageManager.GetMonthFolder(a.currentYear, m), "kassenbuch.json"))
				var book core.CashBook
				for _, b := range books {
					if b.Konto == acct {
						book = b
						break
					}
				}
				unc, _ := core.CashCoverage(book, a.cashInvoicesForMonth(acct, a.currentYear, m))
				for k := range unc {
					a.cashUncovered[k] = true
				}
			}
		}
		a.invoiceTable.SetData(all)
		a.refreshCenterContent()
		return
	}

	// Load from SQLite database
	monat := fmt.Sprintf("%02d", a.currentMonth)
	rows, err := a.dbRepo.List(jahr, monat)
	if err != nil {
		a.logger.Error("Failed to load invoices from database: %v", err)
		a.invoiceTable.SetData([]core.CSVRow{})
		a.refreshCenterContent()
		return
	}

	a.annotateAttachments(rows)
	// Rebuild cash coverage for the displayed month.
	a.cashUncovered = map[string]bool{}
	for _, acct := range a.cashAccounts() {
		books, _ := core.LoadCashBooks(filepath.Join(a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth), "kassenbuch.json"))
		var book core.CashBook
		for _, b := range books {
			if b.Konto == acct {
				book = b
				break
			}
		}
		unc, _ := core.CashCoverage(book, a.cashInvoicesForMonth(acct, a.currentYear, a.currentMonth))
		for k := range unc {
			a.cashUncovered[k] = true
		}
	}
	a.invoiceTable.SetData(rows)
	a.refreshCenterContent()
}

// refreshCenterContent swaps the centerWrapper between the invoice table
// and the empty-state depending on how many rows the table currently has.
// Called after every loadInvoices() so month/year changes and add/delete
// operations all get the correct view.
func (a *App) refreshCenterContent() {
	if a.centerWrapper == nil || a.emptyState == nil || a.invoiceTable == nil {
		return
	}
	if a.invoiceTable.RowCount() == 0 {
		a.centerWrapper.Objects = []fyne.CanvasObject{a.emptyState}
	} else {
		a.centerWrapper.Objects = []fyne.CanvasObject{a.invoiceTable.Container()}
	}
	a.centerWrapper.Refresh()
}

// annotateAttachments fills each row's AnzahlAnhaenge/HatAnhaenge from the
// filesystem — the "<base>_Anhang<N>" sibling files are the source of truth
// for attachments, so the table reflects reality regardless of any stale
// value stored in the database or CSV.
func (a *App) annotateAttachments(rows []core.CSVRow) {
	for i := range rows {
		p := a.resolveInvoicePath(rows[i])
		if !core.FileExists(p) {
			rows[i].AnzahlAnhaenge = 0
			rows[i].HatAnhaenge = false
			continue
		}
		n := core.CountAttachmentsIn(filepath.Dir(p), rows[i].Dateiname)
		rows[i].AnzahlAnhaenge = n
		rows[i].HatAnhaenge = n > 0
	}
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

// showSettingsView is now in settings.go

// extractPDFData extracts metadata from a PDF (safe to call from background thread).
// Returns Meta and error. UI calls should happen in main thread.
func (a *App) extractPDFData(ctx context.Context, path string) (core.Meta, error) {
	a.logger.Debug("=== PDF EXTRACTION START ===")
	a.logger.Debug("File: %s", path)

	// STEP 1: Check for XRechnung/ZUGFeRD structured data (highest priority)
	if format, ok := a.eInvoiceExtractor.DetectFormat(path); ok {
		a.logger.Info("✓ Detected %s format - using structured data extraction", format)
		meta, confidence, err := a.eInvoiceExtractor.Extract(path)
		if err == nil {
			a.logger.Info("E-invoice extraction successful (confidence: %.2f)", confidence)
			if a.settings.DebugMode {
				a.logger.Debug("=== E-INVOICE METADATA ===")
				a.logger.Debug("Format: %s", format)
				a.logger.Debug("Company: %s", meta.Auftraggeber)
				a.logger.Debug("Invoice #: %s", meta.Rechnungsnummer)
				a.logger.Debug("Date: %s", meta.Rechnungsdatum)
				a.logger.Debug("Gross: %.2f %s", meta.Bruttobetrag, meta.Waehrung)
				a.logger.Debug("Net: %.2f", meta.BetragNetto)
				a.logger.Debug("Tax: %.2f%% (%.2f)", meta.SteuersatzProzent, meta.SteuersatzBetrag)
			}

			// Suggest account
			if a.settings.AutoSelectAccount && meta.Auftraggeber != "" {
				if account, ok := core.SuggestAccountForCompany(a.companyMap, meta.Auftraggeber, a.settings.DefaultAccount); ok {
					meta.Gegenkonto = account
				} else if len(meta.KontoVorschlaege) > 0 {
					meta.Gegenkonto = meta.KontoVorschlaege[0]
				} else {
					meta.Gegenkonto = a.settings.DefaultAccount
				}
			} else if len(meta.KontoVorschlaege) > 0 {
				meta.Gegenkonto = meta.KontoVorschlaege[0]
			} else {
				meta.Gegenkonto = a.settings.DefaultAccount
			}

			meta.Quelle = "E-Rechnung"
			return meta, nil
		}
		a.logger.Warn("E-invoice extraction failed: %v, falling back to text extraction", err)
	}

	// STEP 2: Extract text from PDF
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

		// If using Claude mode, try vision extraction
		if a.settings.ProcessingMode == "claude" {
			a.logger.Info("Attempting vision extraction with Claude...")
			return a.extractPDFWithVision(ctx, path)
		}

		// For local mode, we can't process images
		return core.Meta{}, fmt.Errorf("no text found in PDF")
	}

	// Extract metadata based on processing mode
	var meta core.Meta
	var confidence float64

	if a.settings.ProcessingMode == "claude" {
		// Get API key from keyring
		apiKey, err := keyring.Get("BuchISY", a.keyringAccount())
		if err != nil {
			return core.Meta{}, fmt.Errorf("failed to get API key: %w", err)
		}

		// Multimodal: send the extracted text together with the rendered page
		// images, so receipts whose tables are images (POS / SumUp / restaurant
		// bills) are read too. Falls back to text-only if rendering fails.
		images, mediaType, imgErr := core.PDFAllPagesToBase64(path)
		if imgErr != nil || len(images) == 0 {
			a.logger.Warn("Page rendering for multimodal extraction failed (%v); using text only", imgErr)
			meta, confidence, err = a.anthropicExtractor.Extract(ctx, apiKey, a.settings.AnthropicModel, text, a.ownVATIDList()...)
		} else {
			a.logger.Info("Multimodal extraction: text + %d page image(s)", len(images))
			meta, confidence, err = a.anthropicExtractor.ExtractMultimodal(ctx, apiKey, a.settings.AnthropicModel, text, images, mediaType, a.ownVATIDList()...)
		}
		if err != nil {
			return core.Meta{}, fmt.Errorf("claude extraction failed: %w", err)
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
		a.logger.Debug("Company: %s", meta.Auftraggeber)
		a.logger.Debug("Invoice #: %s", meta.Rechnungsnummer)
		a.logger.Debug("Date: %s", meta.Rechnungsdatum)
		a.logger.Debug("Gross: %.2f %s", meta.Bruttobetrag, meta.Waehrung)
		a.logger.Debug("Net: %.2f", meta.BetragNetto)
		a.logger.Debug("Tax: %.2f%% (%.2f)", meta.SteuersatzProzent, meta.SteuersatzBetrag)
	}

	// Suggest account
	if a.settings.AutoSelectAccount && meta.Auftraggeber != "" {
		if account, ok := core.SuggestAccountForCompany(a.companyMap, meta.Auftraggeber, a.settings.DefaultAccount); ok {
			meta.Gegenkonto = account
		} else if len(meta.KontoVorschlaege) > 0 {
			meta.Gegenkonto = meta.KontoVorschlaege[0]
		} else {
			meta.Gegenkonto = a.settings.DefaultAccount
		}
	} else if len(meta.KontoVorschlaege) > 0 {
		meta.Gegenkonto = meta.KontoVorschlaege[0]
	} else {
		meta.Gegenkonto = a.settings.DefaultAccount
	}

	if a.settings.ProcessingMode == "claude" {
		meta.Quelle = "Claude (Text)"
	} else {
		meta.Quelle = "Lokal"
	}
	return meta, nil
}

// extractPDFWithVision extracts metadata from a PDF using Claude's vision API.
func (a *App) extractPDFWithVision(ctx context.Context, path string) (core.Meta, error) {
	a.logger.Info("=== PDF VISION EXTRACTION START ===")
	a.logger.Info("File: %s", path)

	// Convert PDF first page to PNG image
	imageBase64, mediaType, err := core.PDFToImageBase64(path)
	if err != nil {
		a.logger.Error("Failed to convert PDF to image: %v", err)
		return core.Meta{}, fmt.Errorf("failed to render PDF for vision: %w", err)
	}

	a.logger.Info("PDF page rendered to %s, base64 size: %d bytes", mediaType, len(imageBase64))

	// Get API key from keyring
	apiKey, err := keyring.Get("BuchISY", a.keyringAccount())
	if err != nil {
		return core.Meta{}, fmt.Errorf("failed to get API key: %w", err)
	}

	// Extract using vision API
	meta, confidence, err := a.anthropicExtractor.ExtractFromImage(
		ctx,
		apiKey,
		a.settings.AnthropicModel,
		imageBase64,
		mediaType,
		a.ownVATIDList()...,
	)
	if err != nil {
		return core.Meta{}, fmt.Errorf("claude vision extraction failed: %w", err)
	}

	a.logger.Info("Vision extraction succeeded with confidence %.2f", confidence)

	if a.settings.DebugMode {
		a.logger.Debug("=== EXTRACTED METADATA (VISION) ===")
		a.logger.Debug("Company: %s", meta.Auftraggeber)
		a.logger.Debug("Invoice #: %s", meta.Rechnungsnummer)
		a.logger.Debug("Date: %s", meta.Rechnungsdatum)
		a.logger.Debug("Gross: %.2f %s", meta.Bruttobetrag, meta.Waehrung)
	}

	// Suggest account
	if a.settings.AutoSelectAccount && meta.Auftraggeber != "" {
		if account, ok := core.SuggestAccountForCompany(a.companyMap, meta.Auftraggeber, a.settings.DefaultAccount); ok {
			meta.Gegenkonto = account
		} else if len(meta.KontoVorschlaege) > 0 {
			meta.Gegenkonto = meta.KontoVorschlaege[0]
		} else {
			meta.Gegenkonto = a.settings.DefaultAccount
		}
	} else if len(meta.KontoVorschlaege) > 0 {
		meta.Gegenkonto = meta.KontoVorschlaege[0]
	} else {
		meta.Gegenkonto = a.settings.DefaultAccount
	}

	meta.Quelle = "Vision"
	return meta, nil
}

// extractImageData runs an image file (jpg, png, gif, webp) through the
// Claude Vision API to pre-fill the invoice form, same JSON contract as
// the PDF path. Safe to call from a background goroutine.
func (a *App) extractImageData(ctx context.Context, path string) (core.Meta, error) {
	mediaType := core.ImageMediaType(path)
	if mediaType == "" {
		return core.Meta{}, fmt.Errorf("unsupported image type: %s", filepath.Ext(path))
	}

	a.logger.Info("=== IMAGE VISION EXTRACTION START === file=%s mediaType=%s",
		path, mediaType)

	data, err := os.ReadFile(path)
	if err != nil {
		return core.Meta{}, fmt.Errorf("failed to read image: %w", err)
	}
	imageBase64 := base64.StdEncoding.EncodeToString(data)

	apiKey, err := keyring.Get("BuchISY", a.keyringAccount())
	if err != nil {
		return core.Meta{}, fmt.Errorf("failed to get API key: %w", err)
	}

	meta, confidence, err := a.anthropicExtractor.ExtractFromImage(
		ctx, apiKey, a.settings.AnthropicModel, imageBase64, mediaType, a.ownVATIDList()...,
	)
	if err != nil {
		return core.Meta{}, fmt.Errorf("image vision extraction failed: %w", err)
	}
	a.logger.Info("Image vision extraction succeeded (confidence %.2f)", confidence)

	// Mirror the PDF path's account suggestion.
	if a.settings.AutoSelectAccount && meta.Auftraggeber != "" {
		if account, ok := core.SuggestAccountForCompany(a.companyMap, meta.Auftraggeber, a.settings.DefaultAccount); ok {
			meta.Gegenkonto = account
		} else {
			meta.Gegenkonto = a.settings.DefaultAccount
		}
	} else {
		meta.Gegenkonto = a.settings.DefaultAccount
	}
	if meta.Waehrung == "" {
		meta.Waehrung = a.settings.CurrencyDefault
	}
	meta.Quelle = "Vision"
	return meta, nil
}

// handleNoTextPDF handles PDFs without extractable text (e.g., scanned images).
// Must be called from UI thread.
func (a *App) handleNoTextPDF(path string, attachments []string, onComplete func()) {
	dialog.ShowConfirm(
		a.bundle.T("error.noText"),
		a.bundle.T("error.manualEntry"),
		func(manual bool) {
			if manual {
				emptyMeta := core.Meta{
					Waehrung:   a.settings.CurrencyDefault,
					Gegenkonto: a.settings.DefaultAccount,
				}
				a.showConfirmationModal(path, attachments, emptyMeta, onComplete)
			} else if onComplete != nil {
				onComplete()
			}
		},
		a.window,
	)
}

// showConfirmationModal is now in invoicemodal.go

// Helper functions

// wholeYearOption is the sentinel string shown above "01 - Januar" in the
// top-bar month dropdown. Selecting it switches the invoice table to show
// all months of the currently selected year.
const wholeYearOption = "Ganzes Jahr"

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
		// Add padding to prevent truncation of longer month names
		months[i-1] = fmt.Sprintf("%02d - %-12s", i, monthName)
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
