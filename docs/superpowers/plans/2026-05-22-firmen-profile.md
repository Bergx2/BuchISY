# Firmen-Profile — Profil-Auswahl beim Start — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** BuchISY zeigt beim Start einen Profil-Auswahlbildschirm; jedes Profil hat eine vollständig getrennte Konfiguration (Einstellungen, Konto-Zuordnungen, Logs, Ablageordner, API-Schlüssel).

**Architecture:** Die Konfiguration eines Profils liegt unter `<AppData>/BuchISY/profiles/<Profil>/`. `New()` baut nicht mehr sofort die Hauptansicht, sondern zeigt einen Auswahlbildschirm; erst die Profilauswahl löst die eigentliche Initialisierung (`startProfile`) aus. Der API-Schlüssel im Tresor wird mit dem Profilnamen präfixiert.

**Tech Stack:** Go 1.25, Fyne v2.6.3, `go-keyring`.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen. Jede Aufgabe endet mit `go build`/`go vet`/`go test`.

---

### Task 1: Profil-Pfadfunktionen in `core/settings.go` (TDD)

**Files:**
- Modify: `internal/core/settings.go`
- Test: `internal/core/settings_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/core/settings_test.go` (Datei existiert ggf. schon; dann nur die zwei Funktionen anhängen — `os`, `path/filepath`, `testing` ggf. zum Import-Block ergänzen):

```go
func TestGetProfileConfigDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("APPDATA", tmp)
	got, err := GetProfileConfigDir("Bergx2")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(tmp, "BuchISY", "profiles", "Bergx2")
	if got != want {
		t.Errorf("GetProfileConfigDir = %q, want %q", got, want)
	}
}

func TestListProfiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("APPDATA", tmp)

	names, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles on missing dir: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected no profiles, got %v", names)
	}

	base := filepath.Join(tmp, "BuchISY", "profiles")
	if err := os.MkdirAll(filepath.Join(base, "Bergx2"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "Boomstraat"), 0755); err != nil {
		t.Fatal(err)
	}
	names, err = ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 profiles, got %v", names)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run "TestGetProfileConfigDir|TestListProfiles" -v`
Expected: FAIL — `GetProfileConfigDir` und `ListProfiles` sind undefiniert.

- [ ] **Step 3: Implement**

In `internal/core/settings.go`, direkt nach der bestehenden Funktion `GetConfigDir()` einfügen (der Import-Block hat `os` und `path/filepath` bereits):

```go
// GetProfileConfigDir returns the config directory for a named profile,
// i.e. <AppData>/BuchISY/profiles/<profile>.
func GetProfileConfigDir(profile string) (string, error) {
	root, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "profiles", profile), nil
}

// ListProfiles returns the names of all existing profiles (the directory
// names under <AppData>/BuchISY/profiles). A missing profiles directory
// yields an empty list, not an error.
func ListProfiles() ([]string, error) {
	root, err := GetConfigDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "profiles"))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
```

(`GetConfigDir()` bleibt unverändert und liefert weiterhin den Stamm `<AppData>/BuchISY` — er wird für `profiles/` und die Alt-Konfiguration gebraucht.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run "TestGetProfileConfigDir|TestListProfiles" -v`
Expected: PASS.

- [ ] **Step 5: Build, vet, test**

Run: `go build ./... && go vet ./internal/core/... && go test ./internal/core/...`
Expected: PASS — bestehende Aufrufer von `GetConfigDir()` sind unberührt.

---

### Task 2: Start-Ablauf mit Profil-Auswahlbildschirm

**Files:**
- Modify: `internal/ui/app.go`
- Create: `internal/ui/profilepicker.go`

- [ ] **Step 1: Add `assetsDir` and `profile` fields to the `App` struct**

In `internal/ui/app.go`, im `App`-Struct (beginnt bei `type App struct {`) zwei Felder ergänzen — direkt nach dem Feld `theme`:

```go
	theme              *buchisyTheme
	assetsDir          string
	profile            string
```

- [ ] **Step 2: Split `New()` into `New()` + `startProfile()`**

In `internal/ui/app.go` ist `New(assetsDir string) (*App, error)` aktuell **eine** Funktion (von `func New(` bis zum `return application, nil`). Sie wird ersetzt durch ein schlankes `New()` plus eine neue Methode `startProfile`.

Ersetze die **gesamte** Funktion `New` (von der Zeile `// New creates a new BuchISY application.` bzw. `func New(assetsDir string) (*App, error) {` bis zum schließenden `}` der Funktion, also einschließlich `return application, nil`) durch:

```go
// New creates the BuchISY application and shows the profile picker.
func New(assetsDir string) (*App, error) {
	fyneApp := app.NewWithID("com.bergx2.buchisy")
	a := &App{
		app:       fyneApp,
		assetsDir: assetsDir,
		window:    fyneApp.NewWindow("BuchISY"),
	}
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
	customTheme := newBuchisyTheme(uiScale)
	a.app.Settings().SetTheme(customTheme)

	bundle, err := i18n.Load(a.assetsDir, settings.Language)
	if err != nil {
		logger.Warn("Failed to load translations: %v", err)
		bundle = &i18n.Bundle{}
	}

	companyMap := core.NewCompanyAccountMap(configDir)
	if err := companyMap.Load(); err != nil {
		logger.Warn("Failed to load company account map: %v", err)
	}

	if settings.DebugMode {
		logger.SetLevel(logging.DEBUG)
		logger.Debug("Debug mode enabled")
	}

	pdfExtractor := core.NewPDFTextExtractor()
	localExtractor := core.NewLocalExtractor()
	anthropicExtractor := anthropic.NewExtractor(logger, settings.DebugMode)
	eInvoiceExtractor := core.NewEInvoiceExtractor()
	csvRepo := core.NewCSVRepository()
	csvRepo.SetColumnOrder(settings.ColumnOrder)
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

	now := time.Now()
	lastMonth := now.AddDate(0, -1, 0)

	a.bundle = bundle
	a.logger = logger
	a.settings = settings
	a.settingsMgr = settingsMgr
	a.companyMap = companyMap
	a.pdfExtractor = pdfExtractor
	a.localExtractor = localExtractor
	a.anthropicExtractor = anthropicExtractor
	a.eInvoiceExtractor = eInvoiceExtractor
	a.csvRepo = csvRepo
	a.storageManager = storageManager
	a.currentYear = lastMonth.Year()
	a.currentMonth = lastMonth.Month()
	a.theme = customTheme

	a.window.SetTitle("BuchISY — " + profile)
	if settings.WindowWidth > 0 && settings.WindowHeight > 0 {
		a.window.Resize(fyne.NewSize(float32(settings.WindowWidth), float32(settings.WindowHeight)))
	} else {
		a.window.Resize(fyne.NewSize(1500, 875))
	}
	a.window.CenterOnScreen()

	a.window.SetContent(a.buildUI())

	a.registerZoomShortcuts()
	a.registerCtrlScrollZoom()
}
```

Falls dadurch ein bisher in `New()` genutzter Import nun ungenutzt ist, meldet das der Build in Step 4 — dann entfernen. (Erwartung: alle Importe — `app`, `core`, `i18n`, `logging`, `anthropic`, `dialog`, `fyne`, `fmt`, `filepath`, `time` — bleiben genutzt.)

- [ ] **Step 3: Create the profile picker**

Create `internal/ui/profilepicker.go`:

```go
package ui

import (
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// showProfilePicker shows the profile-selection screen as the window content.
func (a *App) showProfilePicker() {
	a.window.SetTitle("BuchISY")
	a.window.Resize(fyne.NewSize(420, 400))
	a.window.CenterOnScreen()
	a.window.SetContent(a.buildProfilePicker())
}

// buildProfilePicker builds the profile-selection UI.
func (a *App) buildProfilePicker() fyne.CanvasObject {
	title := widget.NewLabelWithStyle("Firma wählen", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	list := container.NewVBox()
	profiles, err := core.ListProfiles()
	if err != nil {
		dialog.ShowError(err, a.window)
	}
	for _, name := range profiles {
		p := name
		btn := widget.NewButton(p, func() { a.startProfile(p) })
		list.Add(btn)
	}

	newBtn := widget.NewButton("+ Neues Profil", func() { a.promptNewProfile() })
	newBtn.Importance = widget.HighImportance

	return container.NewBorder(title, newBtn, nil, nil, container.NewVScroll(list))
}

// promptNewProfile asks for a profile name, creates its directory and opens it.
func (a *App) promptNewProfile() {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Firmenname")
	dialog.ShowForm("Neues Profil", "Anlegen", "Abbrechen",
		[]*widget.FormItem{widget.NewFormItem("Name", entry)},
		func(ok bool) {
			if !ok {
				return
			}
			name := core.SanitizeFilename(strings.TrimSpace(entry.Text))
			if name == "" {
				dialog.ShowInformation("Ungültiger Name",
					"Bitte einen gültigen Profilnamen eingeben.", a.window)
				return
			}
			existing, _ := core.ListProfiles()
			for _, e := range existing {
				if e == name {
					dialog.ShowInformation("Profil existiert",
						"Ein Profil mit diesem Namen existiert bereits.", a.window)
					return
				}
			}
			dir, err := core.GetProfileConfigDir(name)
			if err != nil {
				dialog.ShowError(err, a.window)
				return
			}
			if err := os.MkdirAll(dir, 0755); err != nil {
				dialog.ShowError(err, a.window)
				return
			}
			a.startProfile(name)
		},
		a.window)
}
```

- [ ] **Step 4: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS. (`core.SanitizeFilename` existiert in `internal/core/sanitize.go`.)

---

### Task 3: Profil-bezogener API-Schlüssel im Tresor

**Files:**
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/settings.go`

- [ ] **Step 1: Add the keyring-account helper**

In `internal/ui/app.go` diese Methode ergänzen (an beliebiger Stelle auf Dateiebene, z. B. nach `startProfile`):

```go
// keyringAccount returns the OS-keyring account name for the active
// profile's API key, e.g. "Bergx2-claude".
func (a *App) keyringAccount() string {
	return a.profile + "-" + a.settings.AnthropicAPIKeyRef
}
```

- [ ] **Step 2: Route the four keyring calls through the helper**

Es gibt genau vier Tresor-Aufrufe; alle nutzen aktuell `a.settings.AnthropicAPIKeyRef` als Konto:

- `internal/ui/app.go`: `keyring.Get("BuchISY", a.settings.AnthropicAPIKeyRef)` — zweimal.
- `internal/ui/settings.go`: `keyring.Get("BuchISY", a.settings.AnthropicAPIKeyRef)` — einmal.
- `internal/ui/settings.go`: `keyring.Set("BuchISY", a.settings.AnthropicAPIKeyRef, apiKeyEntry.Text)` — einmal.

In allen vier Aufrufen wird das zweite Argument `a.settings.AnthropicAPIKeyRef` durch `a.keyringAccount()` ersetzt:

- `keyring.Get("BuchISY", a.keyringAccount())`
- `keyring.Set("BuchISY", a.keyringAccount(), apiKeyEntry.Text)`

(Die Dateien lesen, die vier Stellen per `keyring.`-Suche finden, ersetzen. Sonst keine Änderung.)

- [ ] **Step 3: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 4: Übernahme der Alt-Konfiguration

**Files:**
- Modify: `internal/ui/profilepicker.go`

- [ ] **Step 1: Add the legacy-config migration**

In `internal/ui/profilepicker.go` den Import-Block erweitern um:

```go
	"fmt"
	"path/filepath"
```

und (vom Modul) `"github.com/zalando/go-keyring"` ergänzen — der Import-Block lautet danach:

```go
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
	"github.com/zalando/go-keyring"
)
```

Am Ende von `internal/ui/profilepicker.go` anhängen:

```go
// maybeMigrateLegacyConfig checks for a pre-profiles configuration directly
// in the config root. If found, it asks whether to assign it to the new
// profile; proceed is called once the decision has been handled.
func (a *App) maybeMigrateLegacyConfig(profile string, proceed func()) {
	root, err := core.GetConfigDir()
	if err != nil {
		proceed()
		return
	}
	if _, err := os.Stat(filepath.Join(root, "settings.json")); err != nil {
		proceed() // no legacy config present
		return
	}
	dialog.ShowConfirm("Bestehende Konfiguration",
		"Es wurde eine bestehende Konfiguration gefunden. Diesem Profil ("+profile+") zuordnen?",
		func(yes bool) {
			if yes {
				if err := migrateLegacyConfig(root, profile); err != nil {
					dialog.ShowError(err, a.window)
				}
			}
			proceed()
		}, a.window)
}

// migrateLegacyConfig moves the legacy config files into the profile
// directory and copies the API key to the profile-scoped keyring account.
func migrateLegacyConfig(root, profile string) error {
	dst, err := core.GetProfileConfigDir(profile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	for _, name := range []string{"settings.json", "company_accounts.json", "logs"} {
		src := filepath.Join(root, name)
		if _, err := os.Stat(src); err != nil {
			continue // not present — skip
		}
		if err := os.Rename(src, filepath.Join(dst, name)); err != nil {
			return fmt.Errorf("Verschieben von %s fehlgeschlagen: %w", name, err)
		}
	}
	// Copy the API key from the legacy keyring account ("claude") to the
	// profile-scoped account ("<profile>-claude").
	if val, err := keyring.Get("BuchISY", "claude"); err == nil {
		_ = keyring.Set("BuchISY", profile+"-claude", val)
	}
	return nil
}
```

- [ ] **Step 2: Wire the migration into the new-profile flow**

In `internal/ui/profilepicker.go`, in `promptNewProfile`, die letzte Zeile vor dem Schließen der Callback-Funktion — aktuell:

```go
			a.startProfile(name)
```

ersetzen durch:

```go
			a.maybeMigrateLegacyConfig(name, func() { a.startProfile(name) })
```

- [ ] **Step 3: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 5: Build, Paketierung, Auslieferung

**Files:** none (build/deploy only)

- [ ] **Step 1: Final build + vet + tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all succeed.

- [ ] **Step 2: Package the Windows executable**

Run (from `C:\Users\istok\Desktop\Dev\BuchISY`):
`fyne package -os windows -name BuchISY -src ./cmd/buchisy`
Expected: `cmd/buchisy/BuchISY.exe` produced.

- [ ] **Step 3: Stop the running app**

Terminate any running `BuchISY` process (PowerShell `wmic process where "ProcessId=<id>" call terminate` per running PID).

- [ ] **Step 4: Deploy and restart**

Copy `cmd/buchisy/BuchISY.exe` over `C:\Users\istok\Desktop\BuchISY.exe`, then launch
`C:\Users\istok\Desktop\BuchISY.exe` with working directory `C:\Users\istok\Desktop`.

- [ ] **Step 5: Manual smoke test**

1. Start → Auswahlbildschirm „Firma wählen" erscheint.
2. „+ Neues Profil" → „Bergx2" → Abfrage „Bestehende Konfiguration … zuordnen?" → *Ja* → die bisherigen Einstellungen/Konten sind vorhanden; Fenstertitel „BuchISY — Bergx2".
3. BuchISY beenden, neu starten → Auswahlbildschirm zeigt „Bergx2".
4. „+ Neues Profil" → „Boomstraat" → startet leer (eigener Ablageordner in den Einstellungen setzbar); Fenstertitel „BuchISY — Boomstraat".
5. Beide Profile beeinflussen sich nicht (getrennte Einstellungen, getrennte Ablageordner).

---

## Self-Review

**Spec coverage:**
- Profil-Verzeichnis `profiles/<Profil>/`, `GetProfileConfigDir`, `ListProfiles` → Task 1.
- Start-Ablauf mit Auswahlbildschirm; `New`/`startProfile`-Aufteilung; „Neues Profil" → Task 2.
- Profil-bezogener API-Schlüssel (`keyringAccount()`, vier Aufrufstellen) → Task 3.
- Übernahme der Alt-Konfiguration (Abfrage, Dateien verschieben, Tresor-Eintrag kopieren) → Task 4.
- Fenstertitel mit Profil → Task 2 Step 2 (`a.window.SetTitle("BuchISY — " + profile)`).
- Profilname-Bereinigung (`SanitizeFilename`), leerer Name / Konflikt → Task 2 Step 3 (`promptNewProfile`).
- Edge Case „keine Profile" → leerer Auswahlbildschirm, nur „+ Neues Profil" (Task 2 Step 3).
- Edge Case „keine Alt-Konfiguration" → `maybeMigrateLegacyConfig` ruft `proceed()` direkt (Task 4).

**Placeholder scan:** Keine TBD/TODO. Alle Code-Schritte enthalten vollständigen Code. Task 2 Step 2 ersetzt eine ganze Funktion verbatim; die Aufrufstellen-Ersetzungen in Task 3 sind über die `keyring.`-Suche eindeutig (genau vier Stellen, in der Aufgabe einzeln benannt).

**Type consistency:** `App` erhält `assetsDir string` und `profile string` (Task 2 Step 1), beide in `New`/`startProfile` genutzt. `core.GetProfileConfigDir(string) (string, error)` und `core.ListProfiles() ([]string, error)` (Task 1) werden in Task 2 und Task 4 mit genau diesen Signaturen aufgerufen. `(a *App) startProfile(string)`, `showProfilePicker()`, `promptNewProfile()`, `keyringAccount() string`, `maybeMigrateLegacyConfig(string, func())` und `migrateLegacyConfig(string, string) error` — alle im Paket `ui`. `GetConfigDir()` bleibt unverändert (Stamm `<AppData>/BuchISY`) und wird in Task 4 zum Auffinden der Alt-Konfiguration genutzt.
