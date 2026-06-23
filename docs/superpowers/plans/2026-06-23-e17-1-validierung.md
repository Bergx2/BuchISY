# E17.1 — Beleg-Eingabe: mehr Plausibilität + frühe Dublettenwarnung

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** (#4) richer plausibility warnings on an invoice; (#2) warn about a likely duplicate **early** in the review modal (not only at save), naming the existing Beleg.

**Tech:** Go 1.25, Fyne. `internal/core`, `internal/db`, `internal/ui`. Branch `feat/e17-1-validierung`.

## Global Constraints

- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- Dates are `DD.MM.YYYY`. Parse with layout `02.01.2006`.
- Do not change existing warning strings or the save-time duplicate confirm (`invoicemodal.go:979/1051`) — this phase ADDS checks and an early banner.

---

### Task 1: Core warnings (#4) + db.FindDuplicate (#2 backend)

**Files:** `internal/core/warnings.go`, `internal/core/warnings_test.go`, `internal/db/repository.go`, `internal/db/repository_test.go` (or a new `finddup_test.go`).

**Interfaces:**
- `core.InvoiceWarningsAsOf(row CSVRow, today time.Time) []string`; keep `InvoiceWarnings(row)` = `InvoiceWarningsAsOf(row, time.Now())`.
- `db.(*Repository).FindDuplicate(row core.CSVRow) (label string, found bool, err error)` — strongest-signal match across ALL months: same `LOWER(TRIM(auftraggeber))` AND non-empty `rechnungsnummer` equal; returns the existing invoice's Belegnummer (fallback Dateiname) as `label`. If `row.Rechnungsnummer` is blank → `found=false` (no early signal).

- [ ] **Step 1: Core test** (append to `warnings_test.go`):

```go
func TestInvoiceWarningsAsOf(t *testing.T) {
	today := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	future := CSVRow{Rechnungsdatum: "01.08.2026", Bruttobetrag: 119, BetragNetto: 100, SteuersatzBetrag: 19, Gegenkonto: 4980, Waehrung: "EUR"}
	if !hasWarn(InvoiceWarningsAsOf(future, today), "Zukunft") {
		t.Error("expected future-date warning")
	}
	zero := CSVRow{Rechnungsdatum: "01.06.2026", Bruttobetrag: 0, Gegenkonto: 4980, Waehrung: "EUR"}
	if !hasWarn(InvoiceWarningsAsOf(zero, today), "Bruttobetrag") {
		t.Error("expected zero-amount warning")
	}
	badVat := CSVRow{Rechnungsdatum: "01.06.2026", Bruttobetrag: 119, BetragNetto: 100, SteuersatzBetrag: 19, Gegenkonto: 4980, Waehrung: "EUR", VATID: "12345"}
	if !hasWarn(InvoiceWarningsAsOf(badVat, today), "USt-IdNr") {
		t.Error("expected invalid VAT-ID format warning")
	}
	ok := CSVRow{Rechnungsdatum: "01.06.2026", Bruttobetrag: 119, BetragNetto: 100, SteuersatzBetrag: 19, Gegenkonto: 4980, Waehrung: "EUR", VATID: "DE287472874"}
	for _, w := range InvoiceWarningsAsOf(ok, today) {
		if strings.Contains(w, "Zukunft") || strings.Contains(w, "USt-IdNr") || strings.Contains(w, "Bruttobetrag") {
			t.Errorf("clean invoice should not warn: %q", w)
		}
	}
}
```

- [ ] **Step 2:** run → fail.
- [ ] **Step 3:** In `warnings.go`, rename the body to `InvoiceWarningsAsOf(row CSVRow, today time.Time) []string`, keep `func InvoiceWarnings(row CSVRow) []string { return InvoiceWarningsAsOf(row, time.Now()) }`. Add these checks (after the existing ones):
  - Future date: parse `row.Rechnungsdatum` with `time.Parse("02.01.2006", ...)`; if ok and `d.After(today)` → `"Rechnungsdatum liegt in der Zukunft"`.
  - Zero amount: `if row.Bruttobetrag <= 0 { w=append(w,"Bruttobetrag fehlt oder ist 0") }`.
  - VAT-ID format: if `strings.TrimSpace(row.VATID) != ""` and it does NOT match `^[A-Z]{2}[0-9A-Za-z]{6,14}$` (after removing spaces, uppercased) → `"USt-IdNr hat ungültiges Format"`. Use `regexp.MustCompile` at package scope.
  Add imports `time`, `regexp`. Keep the existing gross-mismatch / Gegenkonto / Fremdwährung / ZM-gap checks unchanged.
- [ ] **Step 4: db test** (append): insert two invoices with the same Auftraggeber+Rechnungsnummer in different months; assert `FindDuplicate` returns `found=true` with the existing Belegnummer; assert a blank-Rechnungsnummer row returns `found=false`. (Mirror existing repository_test.go setup for opening a temp DB.)
- [ ] **Step 5:** Implement `FindDuplicate`: `SELECT belegnummer, dateiname FROM invoices WHERE LOWER(TRIM(auftraggeber))=LOWER(TRIM(?)) AND rechnungsnummer=? AND rechnungsnummer<>'' LIMIT 1`; guard blank `row.Rechnungsnummer` → return `"",false,nil`; `label` = belegnummer or, if empty, dateiname.
- [ ] **Step 6:** run → pass + full core & db. Commit `E17.1: richer InvoiceWarnings (#4) + Repository.FindDuplicate (#2 backend)`.

---

### Task 2: Modal early duplicate banner (#2 frontend)

**Files:** `internal/ui/invoicemodal.go`.

**Context:** READ the modal build (`formItems` ~line 609, the header area). `meta` holds the extracted data; `a.dbRepo.FindDuplicate` is now available. There is a real-time recompute path already (filename preview / booking) wired to field changes.

- [ ] **Step 1:** Add a hidden warning banner (e.g. `widget.NewLabelWithStyle("", ...)` with a colored/`theme.WarningIcon()` container, or `widget.RichText`) near the top of the modal content.
- [ ] **Step 2:** Add a `checkDuplicate()` closure: build a `core.CSVRow` from the current Auftraggeber + Rechnungsnummer fields, call `label, found, _ := a.dbRepo.FindDuplicate(row)`; if found → set the banner text `"⚠ Mögliche Dublette von "+label` and Show() it, else Hide(). Guard `a.dbRepo != nil`.
- [ ] **Step 3:** Call `checkDuplicate()` once at build time and on `OnChanged` of the Auftraggeber and Rechnungsnummer entries (reuse the existing change hooks if present). Keep it cheap (one indexed query).
- [ ] **Step 4:** `go build ./... && go test ./...`. Commit `E17.1: early duplicate banner in the review modal`.

## Self-Review

Coverage: #4 → Task 1 (InvoiceWarningsAsOf, surfaced by the existing save-time warning dialog). #2 → Task 1 (FindDuplicate) + Task 2 (live banner). Save-time strict dedup untouched. Backward-compatible (`InvoiceWarnings` signature preserved).
