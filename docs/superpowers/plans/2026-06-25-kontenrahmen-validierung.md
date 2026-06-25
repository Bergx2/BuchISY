# Kontenrahmen-Validierung + SKR-Umstellung + §13b-Warnung

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** (#1) validate that every account the booking rules / bookings reference exists in the chart; (#2) a one-click "apply SKR03/SKR04" that sets all standard tax/booking accounts consistently; (#3) warn when a 0 %-VAT expense from a likely foreign supplier isn't booked as §13b reverse-charge.

**Tech:** Go 1.25, Fyne. Branch `feat/kontenrahmen-validierung`.

## Global Constraints
- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer.
- `core.BookingRules` (internal/core/buchungsregeln.go): `VorsteuerKonten map[string]int` (json `vorsteuer_konten`), `UmsatzsteuerKonten map[string]int`, `ErloesKonten map[string]int` (keys "inland"/"eu"/"drittland"), `Regeln []BookingRule`. `BookingRule{Kategorie, Name string; AbziehbarProzent, RcSatz float64; KontoAbziehbar, KontoNichtAbziehbar, KontoVStRC, KontoUStRC, DefaultKonto, KontoNichtAbziehbarGeschenk... int}` — grep the exact field names + json tags in buchungsregeln.go before coding.
- `core.ChartOfAccounts` has `Find(number int)(SKRAccount,bool)` and an All()/list accessor — grep it.
- Standard account numbers (use EXACTLY these):
  - **SKR03**: Vorsteuer 19→1576 7→1571 · Umsatzsteuer 19→1776 7→1771 · §13b VSt 1577 USt 1787 · Bewirtung abz 4650 nicht 4654 · Erlöse inland 8400 eu 8341 drittland 8200
  - **SKR04**: Vorsteuer 19→1406 7→1401 · Umsatzsteuer 19→3806 7→3801 · §13b VSt 1407 USt 3837 · Bewirtung abz 6640 nicht 6644 · Erlöse inland 4400 eu 4125 drittland 4120

---

### Task 1: core — SKR maps, apply, validate, detect

**Files:** `internal/core/skr.go` (new), `internal/core/skr_test.go` (new).

**Interfaces:**
```go
// SKRAccounts holds the standard tax/booking accounts of a chart variant.
type SKRAccounts struct {
    Vorsteuer    map[string]int // "19","7"
    Umsatzsteuer map[string]int
    VStRC, UStRC int            // §13b
    BewAbz, BewNicht int        // Bewirtung 70/30
    ErloesInland, ErloesEU, ErloesDrittland int
}
func StandardSKR(variant string) (SKRAccounts, bool) // "SKR03"/"SKR04"
// ApplySKRVariant returns a copy of rules with all standard accounts set to the
// variant's (Vorsteuer/Umsatzsteuer/Erloes maps + the bewirtung + reverse_charge
// rule accounts); other rule fields (percentages, names) are preserved.
func ApplySKRVariant(rules *BookingRules, variant string) *BookingRules
// ValidateBookingAccounts returns human-readable issues: every account referenced
// by the rules that is NOT in chart (e.g. `Vorsteuer 19%: Konto 1406 nicht im Kontenrahmen`).
func ValidateBookingAccounts(rules *BookingRules, chart *ChartOfAccounts) []string
// DetectSKRVariant guesses the chart's variant from marker accounts
// (SKR03 has 1576 Vorsteuer; SKR04 has 1406). Returns "" if unclear.
func DetectSKRVariant(chart *ChartOfAccounts) string
```

- [ ] **Step 1: test:** `StandardSKR("SKR03").Vorsteuer["19"]==1576`; `ApplySKRVariant(rulesWithSKR04, "SKR03")` → VorsteuerKonten["19"]==1576 and the bewirtung rule's KontoAbziehbar==4650; `ValidateBookingAccounts` flags an account missing from a small test chart; `DetectSKRVariant` returns "SKR03" for a chart containing 1576. Run → fail.
- [ ] **Step 2:** implement. Run → pass + full core.
- [ ] **Step 3:** Commit `Kontenrahmen: SKR maps + ApplySKRVariant + ValidateBookingAccounts + DetectSKRVariant`.

---

### Task 2: core — §13b reverse-charge warning (#3)

**Files:** `internal/core/warnings.go`, `internal/core/warnings_test.go`.

- [ ] **Step 1:** In `InvoiceWarningsAsOf`, add: if NOT `row.Ausgangsrechnung`, `row.SteuersatzBetrag == 0`, `row.Bruttobetrag > 0`, the booking has NO §13b account (none of 1577/1787/1407/3837 in `row.Buchung.Entries`), AND a foreign-supplier signal is present — `row.Waehrung != "" && row.Waehrung != "EUR"` OR `row.VATID` is a non-DE EU VAT-ID (reuse `IsEUVatID` + a `strings.HasPrefix(...,"DE")` exclusion) — then warn: `"0 % USt ohne Reverse-Charge — bei ausländischem Lieferant §13b (Kz 46/47) prüfen"`. Mirror the existing GWG/Bewirtung warning style.
- [ ] **Step 2: test:** a USD 0 %-VAT expense with no §13b booking → warns; the same already booked with §13b (1577/1787 in entries) → no warn; a domestic EUR 0 % expense (e.g. Gegenkonto 4138, no VAT-ID) → no warn. Run → pass.
- [ ] **Step 3:** Commit `Warning: flag 0%-VAT foreign expense not booked as §13b`.

---

### Task 3: UI — Settings "Kontenrahmen" section + startup hint

**Files:** `internal/ui/settings.go` (Erweitert tab), maybe `internal/ui/app.go` (startup banner via the existing config-hint mechanism — grep `MissingConfigHints`).

- [ ] **Step 1:** In Settings → Erweitert, add a **"Kontenrahmen"** section: a "Prüfen" button that runs `core.ValidateBookingAccounts(a.bookingRules, a.chart)` and lists the issues (or "✓ Alle Buchungskonten im Kontenrahmen vorhanden"). Show the detected variant via `DetectSKRVariant`.
- [ ] **Step 2:** Two buttons **"SKR03 anwenden"** / **"SKR04 anwenden"** → confirm dialog → `core.ApplySKRVariant(a.bookingRules, variant)`, persist via the booking-rules store (grep how rules are saved — a per-profile `buchungsregeln.json`; reuse the existing save path), reload `a.bookingRules`, toast the result. Pre-highlight the button matching `DetectSKRVariant`.
- [ ] **Step 3:** If `ValidateBookingAccounts` returns issues at startup, surface a dezent config-hint banner ("Buchungskonten passen nicht zum Kontenrahmen — Einstellungen → Kontenrahmen prüfen") using the existing banner mechanism; dismissable. `go build ./... && go test ./...`. Commit `UI: Kontenrahmen validation + SKR03/SKR04 apply in settings`.

## Self-Review
#1 validation (Task 1 core + Task 3 UI), #2 apply (Task 1 core + Task 3 UI), #3 §13b warning (Task 2). Standard account numbers fixed per Global Constraints. Apply preserves non-account rule fields.
