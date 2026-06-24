# E20.2 — Periodenabschluss / Festschreibung — Implementation Plan

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Lock a month so its invoices become unchangeable (GoBD-Festschreibung); locked periods reject edit/delete; lock/unlock is audited.

**Tech:** Go 1.25, Fyne. `internal/db`, `internal/ui`. Branch `feat/e20-2-periodenabschluss`.

## Global Constraints
- `go build ./...`, `go test ./...`. Commit per task. Co-author: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`. i18n in both JSONs.
- `schemaSQL` in db/schema.go (add table with CREATE TABLE IF NOT EXISTS). CRUD: Insert(94)/Update(140)/Delete(279). Audit: `LogAudit(core.AuditEntry)` exists (E20.1) — log lock/unlock.

---

### Task 1: period_locks table + lock guard in CRUD

**Files:** `internal/db/schema.go`, `internal/db/repository.go`, test `internal/db/periodlock_test.go` (new).

**Interfaces:** `LockPeriod(jahr,monat string) error`, `UnlockPeriod(jahr,monat string) error`, `IsPeriodLocked(jahr,monat string) (bool,error)`, `LockedPeriods() ([]string,error)` (e.g. "2026-06"). Package error `var ErrPeriodLocked = errors.New("Periode ist festgeschrieben")`.

- [ ] **Step 1:** Add to schemaSQL: `CREATE TABLE IF NOT EXISTS period_locks (jahr TEXT NOT NULL, monat TEXT NOT NULL, locked_at DATETIME DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(jahr,monat));`
- [ ] **Step 2: test** (periodlock_test.go, temp DB): LockPeriod("2026","06"); IsPeriodLocked→true; Insert/Update/Delete on jahr=2026 monat=06 → return ErrPeriodLocked; on an UNLOCKED month → succeed; UnlockPeriod → IsPeriodLocked false + ops succeed again; LockedPeriods contains "2026-06". Run → fail.
- [ ] **Step 3:** Implement Lock/Unlock/IsPeriodLocked/LockedPeriods. Lock/Unlock also call `LogAudit({Aktion:"lock"/"unlock", Entitaet:"period", Schluessel: jahr+"-"+monat})`. In `Insert`/`Update`/`Delete`, at the TOP check `IsPeriodLocked(jahr,monat)` (for Insert use row.Jahr/row.Monat; for Update use the target jahr/monat param AND the row's; for Delete the jahr/monat param) → if locked return `ErrPeriodLocked` BEFORE mutating. (Update across months: block if EITHER the old or new period is locked.)
- [ ] **Step 4:** run → pass + full db. Commit `E20.2: period_locks + Festschreibung guard in CRUD`.

---

### Task 2: UI — abschließen/öffnen + locked indicator

**Files:** `internal/ui/app.go` (menu + handlers), `internal/ui/table.go` (lock indicator), `internal/ui/tabledelete.go`/`tableedit.go` (guard messages), `assets/i18n/{de,en}.json`.

- [ ] **Step 1:** Menu items "Monat abschließen" / "Monat öffnen" (near reports) → `a.lockCurrentMonth()` / `a.unlockCurrentMonth()`: `dialog.ShowConfirm` then `a.dbRepo.LockPeriod/UnlockPeriod(jahr,monat)` (jahr=`%04d`, monat=`%02d` of currentYear/Month), `a.loadInvoices()`, `a.showToast`. On lock, warn that edits will be blocked.
- [ ] **Step 2:** Track the current month's locked state (a field `a.currentMonthLocked bool`, set in `loadInvoices` via `IsPeriodLocked`). Show a 🔒 indicator in the status bar / header when locked.
- [ ] **Step 3:** In the edit (`showEditDialog`) and delete (`showDeleteConfirmation`) entry points, if `a.currentMonthLocked` (or `IsPeriodLocked`), show an info dialog "Periode festgeschrieben — Bearbeiten/Löschen gesperrt. Zum Ändern Monat öffnen." and return WITHOUT opening the editor/deleting. (The repo guard is the hard backstop; this is the friendly UI message.)
- [ ] **Step 4:** i18n keys (`period.lock`, `period.unlock`, `period.lockConfirm`, `period.unlockConfirm`, `period.locked.indicator`, `period.locked.msg`, `period.locked.title`) in both JSONs. `go build ./... && go test ./...`. Commit `E20.2: Monat abschließen/öffnen UI + locked indicator + guarded edit/delete`.

## Self-Review
GoBD-Festschreibung: locked month rejects mutations at the repo layer (hard guard) + friendly UI block; lock/unlock audited. Unlock allowed (Vorsystem-pragmatic, audited). Storno path deferred (noted).
