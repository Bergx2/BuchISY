# E20.9 — Verfahrensdokumentation — Implementation Plan

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Generate the GoBD-required Verfahrensdokumentation as a PDF from the profile's settings + a GoBD template.

**Tech:** Go 1.25, Fyne. `internal/core`, `internal/ui`. Branch `feat/e20-9-verfahrensdoku`.

## Global Constraints
- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer. i18n both JSONs.
- `core.Settings` fields: StorageRoot, NamingTemplate, DecimalSeparator, CurrencyDefault, ProcessingMode. PDF helpers in pdfreport.go (`newReportPDF`, multi-line text). Chart: `*ChartOfAccounts` (count via `.All()`).

---

### Task 1: core builder

**Files:** `internal/core/verfahrensdoku.go` (new), `internal/core/verfahrensdoku_test.go` (new).

**Interface:** `func BuildVerfahrensdokumentationPDF(s Settings, chartAccounts int, profilName string, datum string) ([]byte, error)`.

- [ ] **Step 1: test**: build with a sample Settings → output starts with `%PDF`, length > 500; build with zero Settings → no error. Run → fail.
- [ ] **Step 2:** implement in `verfahrensdoku.go` using `newReportPDF` (portrait). Render numbered GoBD sections as headings + paragraphs, filling in the dynamic facts:
  1. **Allgemeines** — Programm BuchISY, Profil/Mandant `profilName`, Stand `datum`.
  2. **Belegerfassung** — Modus (`ProcessingMode`: "claude"→KI-Extraktion via Claude / "local"→lokale Mustererkennung); Eingangskanäle (PDF/Bild-Upload, Drag&Drop, Scan-Ordner); E-Rechnung (XRechnung/ZUGFeRD).
  3. **Belegfluss & Ablage** — Speicherpfad `StorageRoot`, Monatsordner (YYYY-MM), Dateibenennung `NamingTemplate`, Anhänge.
  4. **Belegnummernkreis** — fortlaufende, lückenlose Belegnummer „YYYY-NNNN" pro Jahr.
  5. **Buchung** — Soll/Haben, Kontenrahmen (SKR, `chartAccounts` Konten), USt/Vorsteuer, §13b; DATEV-/Lexware-Export.
  6. **Unveränderbarkeit & Festschreibung** — Periodenabschluss (Monat sperren), Änderungsprotokoll (Audit-Trail), nur Storno nach Festschreibung.
  7. **Aufbewahrung** — 10 Jahre, Original-PDFs unverändert, SQLite-Datenbank als führendes System, CSV-Export.
  8. **Datensicherung** — Backup-Funktion (DB + Config + CSVs als ZIP).
  9. **GoBD-Datenexport** — DATEV-Belegpaket (EXTF + Belegbilder + index.xml) für die Betriebsprüfung.
  10. **Verantwortlichkeiten** — vom Betrieb auszufüllen.
  Use a wrapped-text helper (fpdf `MultiCell`).
- [ ] **Step 3:** run → pass + full core. Commit `E20.9: BuildVerfahrensdokumentationPDF`.

---

### Task 2: UI menu

**Files:** `internal/ui/app.go`, `assets/i18n/{de,en}.json`.

- [ ] **Step 1:** Menu item "Verfahrensdokumentation (PDF)" → handler: `data, err := core.BuildVerfahrensdokumentationPDF(a.settings, len(a.chart.All()), a.currentProfileName(or profile string), todayString)`; `a.savePDF("Verfahrensdokumentation.pdf", data)`. Use a today string from `time.Now().Format("02.01.2006")` (UI side is fine). Find how the profile name is available (e.g. `a.profileName`/the active profile); pass "" if unsure.
- [ ] **Step 2:** i18n key `verfahrensdoku.menu` both JSONs. `go build ./... && go test ./...`. Commit `E20.9: Verfahrensdokumentation menu`.

## Self-Review
GoBD-Pflichtdokument aus den eigenen Einstellungen generiert; deckt Erfassung, Ablage, Festschreibung, Aufbewahrung, Export. Verantwortlichkeiten-Abschnitt vom Betrieb auszufüllen (Hinweis im PDF).
