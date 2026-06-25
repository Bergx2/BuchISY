# EUR-führende Fremdwährung — Implementierungsplan

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.
> Spec: `docs/superpowers/specs/2026-06-25-eur-fuehrende-fremdwaehrung-design.md`

**Goal:** EUR is the leading booking value for foreign-currency invoices, throughout booking + all evaluations + reports; the original foreign amount + manual rate stay as documentation.

**Tech:** Go 1.25, Fyne. Branch `feat/eur-fuehrende-fremdwaehrung`.

## Global Constraints
- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer.
- Manual rate; convention: `Wechselkurs` = foreign per 1 EUR. EUR = `round2(foreign / Wechselkurs)`.
- A row is "foreign" iff `Waehrung != "" && Waehrung != "EUR"`. Foreign + `Wechselkurs <= 0` = rate missing → keep face values + flag, never silently convert.
- `core.CSVRow` money fields: `BetragNetto, SteuersatzBetrag, Bruttobetrag, BetragNetto_EUR, Gebuehr, Trinkgeld, Rabatt, Waehrung, Wechselkurs`. `core.Booking{Entries []BookingEntry{Konto,Betrag,Soll}}`. `core.TaxLine{Netto, SatzProzent, MwStBetrag}`.

---

### Task 1: core EUR normalization

**Files:** `internal/core/eur.go` (new), `internal/core/eur_test.go` (new).

**Interfaces:**
```go
// RowEUR returns a copy of row with all money fields converted to EUR (Waehrung
// "EUR", Wechselkurs 0). rateMissing is true for a foreign row without a rate
// (amounts left at face value). EUR rows are returned unchanged.
func RowEUR(row CSVRow) (eur CSVRow, rateMissing bool)
// RowsEUR maps RowEUR over a slice (rate-missing rows pass through at face value).
func RowsEUR(rows []CSVRow) []CSVRow
// TaxLinesEUR converts each line's Netto + MwStBetrag to EUR (round2) for a
// foreign currency with a rate; returns lines unchanged for EUR / no rate.
func TaxLinesEUR(lines []TaxLine, waehrung string, kurs float64) []TaxLine
```
Convert: BetragNetto, SteuersatzBetrag, Bruttobetrag, BetragNetto_EUR, Gebuehr, Trinkgeld, Rabatt each = `round2(x / Wechselkurs)`; set `Waehrung="EUR"`, `Wechselkurs=0`.

- [ ] **Step 1: test:** USD row (Brutto 200, Netto 200, kurs 1.1720) → RowEUR Netto ≈170.65, Waehrung "EUR", rateMissing false. Foreign, kurs 0 → rateMissing true, amounts unchanged. EUR row unchanged. TaxLinesEUR converts net+vat. Run → fail.
- [ ] **Step 2:** implement (use the existing `round2`). Run → pass + full core.
- [ ] **Step 3:** Commit `EUR: RowEUR/RowsEUR/TaxLinesEUR normalization helpers`.

---

### Task 2: evaluations + reconciliation on EUR

**Files:** `internal/core/ustva*.go`, `susa.go`, `opos.go`, `controlling.go`, `belegabgleich.go`, plus their tests.

- [ ] **Step 1:** At the very start of `ComputeUStVAOfficial`, `ComputeSuSa`, `ComputeGuV`, `ComputeOpenItems`, `AggregateControlling`, insert `rows = RowsEUR(rows)` (find the exact param name in each). This makes every figure EUR with minimal change.
- [ ] **Step 2:** Reimplement `InvoiceEURAmount(row)` via `RowEUR` so it returns the EUR gross − rabatt consistently (it already divides by kurs — unify). Keep behaviour for EUR rows identical.
- [ ] **Step 3:** `go build ./... && go test ./...`. Add/adjust a test: a USD invoice flows into SuSa/Controlling at its EUR value, not face value. Commit `EUR: all evaluations + reconciliation compute on EUR`.

---

### Task 3: booking in EUR (modal + helper)

**Files:** `internal/ui/bookinghelpers.go` (`computeInvoiceBooking`), `internal/ui/invoicemodal.go`, `internal/ui/tableedit.go`.

- [ ] **Step 1:** In `recomputeBooking` of BOTH forms, before calling `computeInvoiceBooking`/`computeRevenueBooking`, convert the tax lines + trinkgeld + rabatt to EUR using the current currency + rate: `linesEUR := core.TaxLinesEUR(ed.Lines(), core.CurrencyCodeFromOption(currencySelect.Selected), parseDecimal(kursEntry.Text))`, and convert `ed.Trinkgeld()` / rabatt likewise (divide by rate when foreign+rate>0). Pass the EUR values in. So the stored `buchung` is in EUR.
- [ ] **Step 2:** Ensure the booking preview (`bookingPrev`) therefore shows EUR; add/confirm a prominent "Gebucht in EUR: <brutto-eur>" line in the Währungsumrechnung section. The existing "Fremdwährung ohne Wechselkurs" warning must fire when foreign + no rate (it already exists in `InvoiceWarningsAsOf` — verify it shows in both forms).
- [ ] **Step 3:** `go build ./... && go test ./...`. Commit `EUR: invoice booking is built in EUR (foreign lines converted via rate)`.

---

### Task 4: PDF + CSV — EUR amounts + original-currency column

**Files:** `internal/core/csvrepo.go`, `internal/db/export.go`, `internal/core/pdfreport.go` (row-based reports: Belegliste, Buchungsjournal, Rechnungsausgangsbuch), their callers.

- [ ] **Step 1:** CSV: export the main amount columns in EUR (apply `RowsEUR` before writing) and ADD documentation columns `Originalwaehrung`, `Originalbetrag_Brutto` (the pre-conversion `Waehrung` + `Bruttobetrag`). Backward compatible (new columns appended; readers tolerate absence).
- [ ] **Step 2:** Row-based PDF reports: feed `RowsEUR(rows)` so amounts are EUR, and add a small "Whg./Original" column or suffix showing the original currency + gross for foreign rows (e.g. "(USD 200,00)"). Computed-struct reports (UStVA/SuSa/GuV/OPOS/Controlling) are already EUR via Task 2 — no change.
- [ ] **Step 3:** `go build ./... && go test ./...`. Commit `EUR: PDF/CSV exports in EUR with original-currency documentation column`.

---

### Task 5: Migration — active EUR re-booking of existing foreign invoices

**Files:** `internal/db/maintenance.go` (new func), `internal/ui/settings.go` or a maintenance menu (trigger), test.

**Interface:**
```go
// RebookForeignToEUR rescales the stored booking of each foreign-currency
// invoice (Waehrung != EUR, Wechselkurs > 0) to EUR by dividing every entry's
// Betrag by the rate (round2), then nudging the payment line so Σ Soll = Σ Haben.
// Idempotent: skips a booking whose total already matches the EUR gross (not the
// foreign gross). Returns counts {converted, skipped, rateMissing} and never
// touches EUR invoices. Caller makes a DB backup first.
func (r *Repository) RebookForeignToEUR() (converted, skipped, rateMissing int, err error)
```
- [ ] **Step 1: test:** a foreign invoice whose `buchung` is in foreign amounts → after RebookForeignToEUR the entries are EUR and balanced; an already-EUR-gross booking is skipped; a rate-missing foreign invoice is counted, not converted. Run → fail.
- [ ] **Step 2:** implement (parse `buchung`, rescale, rounding-adjust the Haben payment line, write back; audit-log each change). Run → pass.
- [ ] **Step 3:** Add a guarded trigger (e.g. Settings → "Fremdwährungs-Belege auf EUR umstellen") that calls it after a confirm + makes a backup, and shows the counts; mark rate-missing invoices with a warning toast. `go build ./... && go test ./...`. Commit `EUR: one-off migration to rebook existing foreign invoices in EUR`.

## Self-Review
`RowEUR`/`RowsEUR` (Task 1) is the single source; evaluations (2), booking (3), exports (4), migration (5) all consume it. Manual rate, EUR-leading, original kept as documentation + extra columns. Rate-missing never silently converted. Out-of-scope (BMF fetch, Kursdifferenz, multi-rate) excluded.
