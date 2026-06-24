# E21 — Belegabgleich-Ausbau: Saldo-Abstimmung + Zeilen-Status + Unlink

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** In the Belegabgleich (now year-wide): show a reconciliation status per bank account (#2 tie-out + #5 line overview) and let the user undo a wrong link (#6).

**Tech:** Go 1.25, Fyne. `internal/core`, `internal/ui`. Branch `feat/e21-belegabgleich-ausbau`.

## Global Constraints
- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer. i18n both JSONs.
- `core.StatementBooking{Page,LineIdx,Date,Text,Betrag,IstGutschrift}`; an invoice links to a line via `BuchungRef{StatementFilename,Page,LineIdx}.String()` stored in `CSVRow.BuchungRef`. The Belegabgleich builds a per-account statement-line cache (`ensureCache`/`stmtCache[acct]` of `{File string, Line StatementBooking}`) and has `accountType`. Statement opening/closing balances live in `core.StatementMetadata{OpeningBalance,ClosingBalance}` (kontenview loads them; reuse its access).

---

### Task 1: core ReconcileSummary (#2/#5 backend)

**Files:** `internal/core/reconcile_status.go` (new), `internal/core/reconcile_status_test.go` (new).

**Interfaces:**
```go
type LineRef struct { Key string; Betrag float64; IstGutschrift bool } // Key = the line's BuchungRef string
type ReconcileResult struct {
    LinesTotal, LinesMatched, LinesOpen int
    OpenBelastung, OpenGutschrift float64 // sums of UNMATCHED debit / credit lines
}
func ReconcileSummary(lines []LineRef, linked map[string]bool) ReconcileResult
```
A line is matched iff `linked[line.Key]`. Open debit lines (IstGutschrift=false) add to OpenBelastung; open credit lines to OpenGutschrift.

- [ ] **Step 1: test**: 4 lines (2 debit, 2 credit), linked={key0,key2} → LinesTotal 4, LinesMatched 2, LinesOpen 2, OpenBelastung/OpenGutschrift = the two unmatched amounts. Empty input → zero result. Run → fail.
- [ ] **Step 2:** implement. Run → pass + full core.
- [ ] **Step 3:** Commit `E21: core ReconcileSummary (line match status)`.

---

### Task 2: UI status section in the Belegabgleich (#2 + #5)

**Files:** `internal/ui/belegabgleichview.go`, `assets/i18n/{de,en}.json`.

**Context:** READ `showBelegabgleich`: how `stmtCache`/`ensureCache` builds per-account lines (with their File), how `claimed`/the linked set is derived, and where the dialog content (suggestions + Barkasse block) is assembled. Rows now span the year (`collectInvoiceRows(year,1,year,12)`).

- [ ] **Step 1:** Build the linked-ref set: collect every `row.BuchungRef` (non-empty) from the year's rows into a `map[string]bool`.
- [ ] **Step 2:** For each bank/credit-card account referenced (use the same set as the matching loop): from `stmtCache[acct]` build `[]core.LineRef{Key: refKey(File,Page,LineIdx), Betrag, IstGutschrift}` and call `core.ReconcileSummary(lines, linkedSet)`.
- [ ] **Step 3:** Add a **status block** (above or below the suggestion list) per account:
  `"<acct>: <Matched>/<Total> Auszugszeilen zugeordnet · offen <OpenBelastung+OpenGutschrift> €"` plus, when available, the account's latest statement **Endsaldo** (reuse the metadata access from kontenview — best-effort, omit if not loadable). Show `✓ vollständig abgeglichen` when LinesOpen==0, else `⚠ <n> Zeile(n) offen`.
- [ ] **Step 4:** List the **open lines** compactly (Datum · Betrag · Text) under the status block (this is the integrated "Fehlende Belege" view, but now also covers credit lines). Cap the list length and note if truncated.
- [ ] **Step 5:** i18n keys (`reconcile.status`, `reconcile.complete`, `reconcile.openLines`, `reconcile.endsaldo`) both JSONs. `go build ./... && go test ./...`. Commit `E21: reconciliation status + statement-line overview (tie-out) in Belegabgleich`.

---

### Task 3: Unlink within the dialog (#6)

**Files:** `internal/ui/belegabgleichview.go`, `assets/i18n/{de,en}.json`.

- [ ] **Step 1:** Add a **"Bereits zugeordnet"** collapsible/section listing the year's rows with a non-empty `BuchungRef` (bank/credit-card): Belegnr · Auftraggeber · Betrag · the linked line (`core.ParseBuchungRef(...).Display()`), each with a **"Zuordnung aufheben"** button.
- [ ] **Step 2:** On tap: set `row.BuchungRef = ""`, `a.dbRepo.Update(row.Jahr,row.Monat,row.Dateiname,row)` (writes an audit entry via E20.1), refresh the dialog (rebuild like the bulk-confirm does). Reuse the existing `a.unlinkInvoice` if its behaviour matches (read it); else inline the BuchungRef-clear + Update.
- [ ] **Step 3:** i18n keys (`reconcile.linked`, `reconcile.unlink`) both JSONs. `go build ./... && go test ./...`. Commit `E21: unlink an existing assignment from within the Belegabgleich`.

## Self-Review
#2 tie-out + #5 line overview → Task 1 (core) + Task 2 (UI). #6 unlink → Task 3. Year-wide rows already in place. Core summary unit-tested.
