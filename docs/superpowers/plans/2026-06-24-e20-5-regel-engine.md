# E20.5 — Regel-Engine / Auto-Buchung — Implementation Plan

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Known suppliers can be booked automatically (no modal) when a per-supplier rule opts in AND the receipt is plausible. **Default OFF** — strictly opt-in per supplier; inert until the user enables a rule.

**Tech:** Go 1.25, Fyne. `internal/core`, `internal/ui`. Branch `feat/e20-5-regel-engine`.

## Global Constraints
- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer. i18n both JSONs.
- `core.BookingTemplate`/`BookingTemplateStore` (bookingtemplate.go) already map company→{kategorie, expense_konto}. `a.bookingTemplates`. The modal pre-fills from them. Save path: `a.saveInvoice(...)` (invoicemodal.go:1029) and `showConfirmationModal` (138). Batch import: `enqueueSubmissions`/`processNextPending` + `processSubmission` (filepicker.go).
- **SAFETY:** auto-book only when the matched template has `Autobook==true` (default false). No template is auto-set to true.

---

### Task 1: rule match + plausibility (core)

**Files:** `internal/core/bookingtemplate.go` (add field), `internal/core/autobook.go` (new), `internal/core/autobook_test.go` (new).

- [ ] **Step 1:** Add `Autobook bool json:"autobook,omitempty"` to `BookingTemplate`.
- [ ] **Step 2: test** (`autobook_test.go`): `AutobookPlausible(meta)` true when meta has tax lines, Gegenkonto>0, Bruttobetrag≈Netto+USt (±0.02), no foreign-currency-without-rate, Waehrung default; false otherwise (zero amount, missing konto, gross mismatch). Run → fail.
- [ ] **Step 3:** Implement in `autobook.go`:
```go
// AutobookPlausible reports whether a receipt is safe to book without review.
func AutobookPlausible(m Meta) bool { ... }   // tax lines present; Gegenkonto>0; |Brutto-(Netto+USt+Trinkgeld)|<=0.02; Bruttobetrag>0; (Waehrung=="" or no rate-missing). Reuse SumNetto/SumMwSt over m.Steuerzeilen.
// MatchAutobookRule returns the supplier's template iff it exists AND Autobook is on.
func MatchAutobookRule(company string, store *BookingTemplateStore) (BookingTemplate, bool) { ... }  // store.Get(company); ok && tpl.Autobook
```
Run → pass + full core.
- [ ] **Step 4:** Commit `E20.5: Autobook flag + MatchAutobookRule + AutobookPlausible`.

---

### Task 2: auto-book in the import flow + rules management

**Files:** `internal/ui/filepicker.go` (processSubmission), `internal/ui/invoicemodal.go` (a programmatic save helper), `internal/ui/settings.go` or a new `internal/ui/autorulesview.go` (management), `assets/i18n/{de,en}.json`.

- [ ] **Step 1: programmatic save.** Add `func (a *App) autoBookInvoice(mainPath string, attachments []string, meta core.Meta, tpl core.BookingTemplate) error` that reproduces what the modal's save does WITHOUT the dialog: resolve account (`tpl.ExpenseKonto`), build the booking via the SAME helper the modal uses (find `computeInvoiceBooking`/`BuildBooking` usage), tax lines from `meta.Steuerzeilen`, filename from the template engine, belegnummer via `NextBelegnummer`, year/month from `meta`, then call `a.saveInvoice(...)` with those values (rememberMapping=false; partialPayment=false). Keep it close to the modal's save call (invoicemodal.go ~905) — read it and mirror the argument construction.
- [ ] **Step 2: wire into import.** In `processSubmission` (or right before each `showConfirmationModal` call), if `tpl, ok := core.MatchAutobookRule(meta.Auftraggeber, a.bookingTemplates); ok && core.AutobookPlausible(meta) && !<isDuplicate>`: call `autoBookInvoice(...)`, count it as auto-booked, and DO NOT open the modal (call `onComplete`/advance the queue). Else open the modal as today. Track counts across a batch (reuse the `batchTotal/batchDone` area) and at batch end show a toast `"N automatisch gebucht · M zur Prüfung"`. Duplicate check: reuse `a.dbRepo.IsDuplicate` / `FindDuplicate`.
- [ ] **Step 3: management.** Add a dialog/menu "Auto-Buchungs-Regeln" listing the learned templates (`bookingTemplates`) — supplier · Konto · Kategorie · a per-row **Autobook**-Checkbox that sets `tpl.Autobook` + saves the store. Make it clear this enables booking WITHOUT review. (If BookingTemplateStore lacks a list/set API, add `List()`/`Set()` minimal helpers.)
- [ ] **Step 4:** i18n keys (`autorules.title`, `autorules.col.*`, `autorules.autobook`, `autobook.result`) both JSONs. `go build ./... && go test ./...`. Commit `E20.5: opt-in auto-book in import + rules management`.

## Self-Review
Default OFF (Autobook false) → inert until the user enables a supplier; plausibility-gated; duplicate-checked; reuses saveInvoice (same booking/file/belegnr logic). DEFERRED for the user to verify before relying on silent booking (note in the final summary).
