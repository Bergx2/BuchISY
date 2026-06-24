# E19.4 — Feedback & Sicherheit — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** A one-click "confirm all unambiguous (★)" action in the reconciliation dialogs, and an Undo for deleting an invoice via an action-toast.

**Architecture:** `showToast(text)` exists (`toast.go`). We add `showToastWithAction(text, actionLabel, action)`. Delete (`tabledelete.go deleteInvoice`) buffers the file bytes before removal so Undo can restore file + DB row. The reconciliation dialogs (`belegabgleichview.go`, `erloesabgleichview.go`) already flag `highConfidence` suggestions (★); we add a button that confirms them all, reusing the per-row link logic + claim-once.

**Tech Stack:** Go 1.25, Fyne. `internal/ui` (+ a repo re-insert call). Branch `feat/e19-4-feedback-sicherheit`.

## Global Constraints

- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- i18n via `a.bundle.T(...)`, keys in BOTH `assets/i18n/{de,en}.json`.
- istok's principle: confirming all ★ is still an explicit user action (with a mini-confirm) — not silent automation.
- Claim-once must hold: a line linked by the bulk action can't be re-linked.

---

### Task 1: "Alle ★ bestätigen" in both reconciliation dialogs

**Files:** `internal/ui/belegabgleichview.go`, `internal/ui/erloesabgleichview.go`, `assets/i18n/{de,en}.json`.

**Context:** READ `showBelegabgleich`: `suggestions []belegSuggestion` (each has `highConfidence bool`, `row`, `candidates`); the per-row confirm handler (~lines 602-631) does: claim-once guard (`if claimed[key]`), `row.BuchungRef = ...`, `a.dbRepo.Update(...)`, `a.statementAliases.Learn(...)`+`Save()`. `showErloesAbgleich` mirrors it PLUS settlement (`row.Buchung = row.Buchung.WithSettlementAccount(pay)` when `pay,ok := a.settings.PaymentAccountSKR04(row.Bankkonto)`).

- [ ] **Step 1 (Belegabgleich):** Add a button "Alle eindeutigen (★) bestätigen — N" (N = count of high-confidence suggestions whose top candidate's line is not yet claimed) above the suggestion list, shown only when N>0. On tap: `dialog.ShowConfirm` with "N Belege verknüpfen?"; on yes, iterate the high-confidence suggestions, and for each whose top candidate's line key is not in `claimed`: perform the SAME link as the per-row handler (set BuchungRef from the top candidate, `claimed[key]=true`, `dbRepo.Update`, alias Learn/Save). Then refresh the dialog (re-run the build, or hide+reopen — match how the per-row confirm refreshes). Use the existing `refKey`/`claimed`/candidate structures; do not bypass claim-once.
- [ ] **Step 2 (Erlös-Abgleich):** Same button in `showErloesAbgleich`, but the per-item link must ALSO settle: `if pay, ok := a.settings.PaymentAccountSKR04(row.Bankkonto); ok { row.Buchung = row.Buchung.WithSettlementAccount(pay) }` before `Update` (mirror its per-row handler exactly).
- [ ] **Step 3:** i18n keys `reconcile.confirmAllStar` ("Alle eindeutigen (★) bestätigen — %d" / "Confirm all unambiguous (★) — %d") and `reconcile.confirmAllAsk` ("%d Belege jetzt verknüpfen?" / "Link %d receipts now?") in both JSONs.
- [ ] **Step 4:** `go build ./... && go test ./...`. Manually reason: only ★ rows are bulk-linked; ambiguous stay; a line claimed by an earlier ★ blocks a later ★ on the same line. Commit `E19.4: "Alle ★ bestätigen" in Beleg- and Erlös-Abgleich`.

---

### Task 2: Action-toast + Undo for delete

**Files:** `internal/ui/toast.go`, `internal/ui/tabledelete.go`, `internal/db/repository.go` (re-insert), `internal/ui/app.go` or export site (an export toast).

**Context:** READ `toast.go` `showToast`. READ `deleteInvoice` (`tabledelete.go:54-109`): it `os.Remove(filePath)`, `dbRepo.Delete(jahr,monat,dateiname)`, exports CSV, reloads, `showToast`. Find the repo method that INSERTS an invoice row (grep `func (r *Repository) Create`/`Insert`/`Save`/`Upsert` — the one `saveInvoice` uses) — reuse it to re-insert on undo.

- [ ] **Step 1:** In `toast.go`, add `func (a *App) showToastWithAction(text, actionLabel string, action func())` — like `showToast` but the toast contains a tappable action button (e.g. a `widget.Button(actionLabel, ...)` that runs `action()` then dismisses the toast); auto-dismiss after a longer window (~8s) so the user can click Undo. If toast rendering is a single label today, extend the container to hold label+button. Keep `showToast` unchanged.
- [ ] **Step 2:** In `deleteInvoice`, BEFORE `os.Remove`, read the file bytes: `data, _ := os.ReadFile(filePath)` (tolerate missing file → data=nil, undo just re-inserts the row). Capture the `row`, `filePath`, `jahr`, `monat`, and `data`. After a successful delete (the existing path), replace the final `showToast(...)` with `showToastWithAction("✓ Rechnung gelöscht: "+row.Dateiname, a.bundle.T("undo"), func(){ a.undoDelete(row, jahr, monat, filePath, data) })`.
- [ ] **Step 3:** Implement `func (a *App) undoDelete(row core.CSVRow, jahr, monat, filePath string, data []byte)`: if `len(data)>0` write it back via `os.WriteFile(filePath, data, 0644)` (recreate parent dir if needed); re-insert the row via the repo insert method found above (jahr/monat preserved); re-export the CSV for that month; `a.loadInvoices()`; `a.showToast(a.bundle.T("undo.done"))`. Guard errors with `showError`.
- [ ] **Step 4:** Add an export-completion toast where invoices are exported to DATEV/Lexware if one is missing (grep the export action handler; add `a.showToast("✓ Export: "+name)` after a successful export). Keep it minimal — only if absent.
- [ ] **Step 5:** i18n keys `undo` ("Rückgängig" / "Undo"), `undo.done` ("Löschen rückgängig gemacht" / "Deletion undone") in both JSONs. `go build ./... && go test ./...`. Commit `E19.4: action-toast with Undo for invoice deletion`.

## Self-Review

Coverage: #1 → Task 1 (both dialogs, claim-once preserved, settlement on revenue). #9 → Task 2 (action-toast + soft-undo via buffered bytes + row re-insert; toasts already widespread, export toast added if missing). No silent automation. The undo window is in-memory only (process-lifetime), which is acceptable for an immediate "oops".
