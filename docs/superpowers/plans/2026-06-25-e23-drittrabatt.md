# E23 — Drittrabatt / Plattform-Gutschein

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** A receipt's invoice gross can differ from what was actually paid because a *third party* (e.g. eBay) granted a voucher. Add a `Rabatt` field that keeps the invoice gross + full VAT, but (method B chosen by the user) reduces the expense/asset line and the paid amount by the gross rebate — so the booking balances at the amount actually paid and the bank reconciliation matches it.

**Method B booking** (for `Rabatt > 0`): expense/asset Soll = `Netto − Rabatt`; Vorsteuer Soll = full; payment Haben = `Brutto − Rabatt`. Reconciliation expected amount = `Brutto − Rabatt`.

**Tech:** Go 1.25, Fyne. Branch `feat/e23-drittrabatt`.

## Global Constraints
- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer.
- **Mirror the existing `Gebuehr float64` field end-to-end.** Grep `Gebuehr` across the repo — `Rabatt` needs an entry everywhere `Gebuehr` has one (CSVRow, Meta, ToMeta/ToCSVRow conversions, DB schema column + ALTER migration + scanInvoiceRows + Insert + Update, CSV export/import column list, and any modal field wiring) EXCEPT where noted. Use German DD.MM.YYYY conventions already in place; decimals stored as REAL.
- Booking lives in `core.BuildBooking` (buchung.go:170). Reconciliation amount in `core.InvoiceEURAmount` (belegabgleich.go).

---

### Task 1: Rabatt field through types + DB + CSV (mirror Gebuehr)

**Files:** `internal/core/types.go`, `internal/db/schema.go`, `internal/db/repository.go`, `internal/core/csvrepo.go`, `internal/db/export.go` (+ wherever else `Gebuehr` is threaded). 

- [ ] **Step 1:** Add `Rabatt float64` to `core.CSVRow` (near `Gebuehr`) and `core.Meta`; thread it through `ToMeta`/`ToCSVRow` (or the equivalent conversion funcs) exactly like `Gebuehr`.
- [ ] **Step 2:** DB: add `rabatt REAL` to `schemaSQL` (after `gebuehr`), add an idempotent `ALTER TABLE invoices ADD COLUMN rabatt REAL` to the migration list in `initSchema` (repository.go), and include `rabatt` in `scanInvoiceRows`, `Insert`, and `Update` — mirror `gebuehr` in each.
- [ ] **Step 3:** CSV: add a `Rabatt` column to the export/import (csvrepo.go + db/export.go) mirroring `Gebuehr`; keep backward compatibility (missing column → 0).
- [ ] **Step 4:** `go build ./... && go test ./...`. If there is a DB round-trip test for Gebuehr, add the parallel Rabatt assertion; otherwise add a small repository test inserting a row with Rabatt and reading it back. Commit `E23: add Rabatt field (types + DB + CSV), mirroring Gebuehr`.

---

### Task 2: Booking + reconciliation honour Rabatt

**Files:** `internal/core/buchung.go`, `internal/core/belegabgleich.go`, callers of `BuildBooking` (grep), `internal/core/buchung_test.go`, `internal/core/belegabgleich_test.go` (or nearby).

**Interfaces:**
- `BuildBooking(rules, kategorie, lines, trinkgeld, expenseAccount, paymentAccount)` gains a trailing `rabatt float64` param. For the `"standard"` category, the expense entry becomes `round2(netTotal - rabatt)` (the payment Haben already = Σ Soll, so it auto-balances to Brutto − Rabatt). For other categories, ignore rabatt (rare combo) — but the new param must be threaded at every call site.
- `InvoiceEURAmount(row)`: subtract `row.Rabatt` from the returned amount (both the EUR-converted and the plain `Bruttobetrag` branch), so reconciliation expects the actually-paid amount.

- [ ] **Step 1: test (buchung):** `BuildBooking` standard, netTotal 1116.85, one 19% line (VAT 212.20), expenseAccount 420, paymentAccount 1270, **rabatt 50** → entries: Soll 420 = 1066.85, Soll Vorsteuer = 212.20, Haben 1270 = 1279.05 (balances). Run → fail.
- [ ] **Step 2:** add the `rabatt` param + standard-case subtraction; thread through all call sites. Run → pass.
- [ ] **Step 3: test (reconcile):** `InvoiceEURAmount(CSVRow{Bruttobetrag:1329.05, Rabatt:50})` == 1279.05; with Rabatt 0 unchanged. Implement. Run → pass + full core.
- [ ] **Step 4:** Commit `E23: BuildBooking + InvoiceEURAmount honour Rabatt (method B)`.

---

### Task 3: UI — Rabatt field in the forms

**Files:** `internal/ui/tableedit.go`, `internal/ui/invoicemodal.go` (mirror how `feeEntry`/`Gebuehr` is presented), `assets/i18n/{de,en}.json`.

- [ ] **Step 1:** In the edit dialog (and the new-invoice modal), add a **"Rabatt/Gutschein (Plattform)"** entry near the fee/currency fields, prefilled from `row.Rabatt`/`meta.Rabatt` via `formatDecimal`, parsed via `parseFloat`. Wire its `OnChanged` into `recomputeBooking` (so the live booking preview updates). Pass the parsed value into the `saveInvoice`/update path so it persists, and into the `BuildBooking(... , rabatt)` call.
- [ ] **Step 2:** Show a small read-only hint **"Tatsächlich gezahlt: <Brutto − Rabatt>"** next to the field (updates on change), so the user sees the paid amount. Use `formatDecimal`.
- [ ] **Step 3:** i18n keys (`field.rabatt`, `field.paidActual`) in both JSONs. `go build ./... && go test ./...`. Commit `E23: Rabatt/Gutschein field in edit + new-invoice forms`.

## Self-Review
Field (Task 1) → booking + reconcile logic (Task 2) → UI (Task 3). Method B: expense reduced by gross rabatt, VAT full, paid = Brutto − Rabatt, reconciliation matches paid. Mirrors Gebuehr for the plumbing.
