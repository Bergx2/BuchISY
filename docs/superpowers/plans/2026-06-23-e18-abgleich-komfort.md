# E18 — Abgleich-Komfort: Auto-Start nach Import + Bestätigung-pro-Beleg + Kassen-Abgleich

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Design (decisions confirmed by istok):**
- After a bank-statement import, the Belegabgleich opens automatically (no menu detour).
- **No more silent auto-linking — everywhere** (auto-import AND manual menu): every match, even an unambiguous one, is presented as a confirm-row the user must approve.
- Cash receipts (Barkasse) get an **internal per-receipt confirmation** in the Belegabgleich (there is no external cash statement; the cash book is generated). Confirming a cash receipt marks it reconciled; coverage (E11) is shown alongside.

**Tech:** Go 1.25, Fyne. `internal/ui` (+ a tiny `internal/core` sentinel). Branch `feat/e18-abgleich-komfort`.

## Global Constraints

- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- Reuse: `showBelegabgleich` (belegabgleichview.go) greedy auto-link is at lines ~240-292; the suggestion list + confirm UI follow it. The invoice filter at line ~144 skips non-bank/non-creditcard (`accountType(row.Bankkonto)`). `accountType` closure at line 90. `core.AccountTypeCash = "cash"`. Cash coverage: `App.cashUncovered` / `core.ComputeCashReport` (E11).
- Keep the claim-once guard, alias learning, and Claude ranking intact.

---

### Task 1: E18.1 — Belegabgleich confirm-each (no silent auto-link)

**Files:** `internal/ui/belegabgleichview.go`.

**Context:** READ lines 240-360. Today: auto results (`core.MatchAuto`) are silently linked (loop 264-292, sets `BuchungRef`+`Update`+alias, `autoLinked++`); the rest become `suggestions`. Goal: link NOTHING silently — route auto results into the suggestion/confirm list (pre-sorted, top candidate first, flagged as high-confidence), so the user confirms each.

- [ ] **Step 1:** Replace the silent-link loop (264-292) so that for each `autoResults` entry it does NOT set BuchungRef/Update/alias; instead it is appended to `suggestions` (or `suggestResults`) with its `candidates` (top first), the same way ambiguous ones are. Preserve the claim-once line dedup conceptually, but since nothing is linked yet, no `claimed` pre-population is needed from autos — keep `claimed` empty at suggestion-build time (the confirm handlers already guard against linking an already-claimed line at click time). Set `autoLinked = 0`.
- [ ] **Step 2:** Carry a per-suggestion `highConfidence bool` (true for former-auto results) on the `belegSuggestion` struct; in the row UI, prefix high-confidence rows with a marker (e.g. "★ ") and keep them sorted to the top, so the user sees which are unambiguous and can confirm quickly.
- [ ] **Step 3:** Update the dialog's summary/header text: instead of "N automatisch verknüpft", show e.g. "Bitte jede Zuordnung bestätigen — N Vorschläge" (use an existing i18n pattern or a literal German string consistent with the file). Alias learning + the existing per-row confirm handler (which sets BuchungRef + Update + Learn) stay unchanged — they already run on confirm.
- [ ] **Step 4:** `go build ./... && go test ./...`. Manually reason: an unambiguous single match now appears as a ★ confirm-row, not silently linked; confirming it links exactly as before; the claim-once guard still prevents double-linking a line. Commit `E18.1: Belegabgleich requires per-match confirmation (no silent auto-link)`.

---

### Task 2: E18.2 — auto-open Belegabgleich after statement import

**Files:** `internal/ui/kontenview.go` (`autoFillNewStatements` ~ the `fyne.DoAndWait` completion), `internal/ui/belegabgleichview.go` (only if a guard is needed).

**Context:** `fileStatement` → `autoFillNewStatements(folder, names)` extracts metadata in a goroutine, then on the main thread hides progress, shows any failure dialog, and `a.window.SetContent(a.buildUI())`. It does NOT open any reconciliation.

- [ ] **Step 1:** After the successful-import UI rebuild in `autoFillNewStatements` (inside the `fyne.DoAndWait`, after `SetContent`), call `a.showBelegabgleich()` so the confirm-list opens automatically. Only when `len(names) > len(failures)` (at least one statement imported OK). Guard: if `showBelegabgleich` has nothing to match it already shows an "alles abgeglichen" path — that's fine.
- [ ] **Step 2:** `go build ./... && go test ./...`. Commit `E18.2: open the Belegabgleich automatically after a statement import`.

---

### Task 3: E18.3 — cash receipts confirmation in the Belegabgleich

**Files:** `internal/core/types.go` (a sentinel), `internal/ui/belegabgleichview.go`.

**Context:** The invoice loop (line ~144) skips `accountType != Bank && != CreditCard`, excluding cash. Cash receipts have no external statement line. We add a "Barkasse" confirm section: each unconfirmed cash receipt gets a "Bestätigen" button that marks it reconciled via a sentinel BuchungRef, so the table ✓/○ status works without a new DB column.

- [ ] **Step 1:** In `internal/core/types.go`, add `const CashConfirmedRef = "kassenbuch|0|0"` (a sentinel BuchungRef value meaning "cash receipt confirmed against the generated cash book"). Document it.
- [ ] **Step 2:** In `showBelegabgleich`, after the bank/creditcard sections, add a **Barkasse** section: collect rows where `accountType(row.Bankkonto) == core.AccountTypeCash` AND `row.BuchungRef == ""`. For each, show Belegnummer/date/Auftraggeber/amount + the coverage hint (✓/⚠ from `a.cashUncovered` if available) + a "Bestätigen" button. On click: `row.BuchungRef = core.CashConfirmedRef`; `a.dbRepo.Update(...)`; refresh the row to ✓. Skip the section entirely if there are no unconfirmed cash receipts.
- [ ] **Step 3:** Ensure bank-matching never assigns `CashConfirmedRef` to a non-cash row and never matches cash rows (the existing `accountType` filter already excludes cash from bank matching — confirm it). The table's existing ✓/○ display treats any non-empty BuchungRef as ✓, which is what we want.
- [ ] **Step 4:** `go build ./... && go test ./...`. Commit `E18.3: per-receipt cash confirmation (Barkasse) in the Belegabgleich`.

## Self-Review

Coverage: auto-start → Task 2; confirm-each everywhere → Task 1 (manual + the Task 2 auto-open both use the now-confirm-only dialog); cash per-receipt confirm → Task 3. No silent linking remains. Cash uses a sentinel BuchungRef (no schema change). Claim-once/alias/Claude-ranking preserved.
