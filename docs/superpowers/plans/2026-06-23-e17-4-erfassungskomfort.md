# E17.4 — Erfassungs-Komfort (#5 Warnungen live, #6 Zahlstatus, #7 Quelle, #8 Tastatur)

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Four entry-quality improvements in the review modal: live plausibility warnings, payment-status → settled revenue booking, an extraction-source badge, and keyboard shortcuts.

**Tech:** Go 1.25, Fyne. `internal/core`, `internal/ui`. Branch `feat/e17-4-erfassungskomfort`.

## Global Constraints

- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- Reuse: `core.InvoiceWarnings(row)` (#5), `core.Booking.WithSettlementAccount(bankKonto)` + `settings.PaymentAccountSKR04(bankkonto)` (#6), `core.Meta` (#7).
- The modal (`invoicemodal.go`) already has `paymentDateEntry` (Bezahldatum, ~line 273), `ausgangsrechnungCheck`, a booking recompute path, a bank-account selector, and the early-dedup banner from E17.1.

---

### Task 1: `Meta.Quelle` + set it during extraction (#7 backend)

**Files:** `internal/core/types.go`, `internal/ui/app.go` (`extractPDFData` ~1103, `extractImageData` ~1299, `extractPDFWithVision`).

- [ ] **Step 1:** Add `Quelle string` to `core.Meta` (transient extraction-source label; not persisted). Document it as such.
- [ ] **Step 2:** Set it in `extractPDFData` (`app.go`): the E-invoice branch (before `return meta, nil` at the DetectFormat block) → `meta.Quelle = "E-Rechnung"`; in the Claude/local text block, set `meta.Quelle = "Claude (Text)"` when ProcessingMode=="claude" else `"Lokal"`, before the final `return meta, nil`. In `extractPDFWithVision` set `meta.Quelle = "Vision"`; in `extractImageData` set `meta.Quelle = "Vision"`. (For blank/empty-form fallbacks leave it "".)
- [ ] **Step 3:** `go build ./...`. Commit `E17.4: Meta.Quelle extraction-source label`.

---

### Task 2: Modal — source badge, live warnings, payment settlement, keyboard

**Files:** `internal/ui/invoicemodal.go`.

- [ ] **Step 1 (#7 badge):** Near the top of the modal content (next to the early-dedup banner), add a small label showing the source when `meta.Quelle != ""`, e.g. `widget.NewLabelWithStyle("Quelle: "+meta.Quelle, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})`. Hide/omit when empty.
- [ ] **Step 2 (#5 live warnings):** Add a warnings strip (a `widget.Label` with `Importance: widget.WarningImportance`, multi-line) near the top. Add a `refreshWarnings()` closure that builds the same `core.CSVRow` the save-time call builds (BetragNetto/SteuersatzBetrag/Brutto/Trinkgeld/Gegenkonto/Waehrung/Wechselkurs/Rechnungsdatum/VATID/Ausgangsrechnung from the live widgets) and sets the strip text to the joined `core.InvoiceWarnings(row)` (empty → hide). Call it at build time and chain it into the existing OnChanged hooks of the amount/date/currency/account/Ausgangsrechnung widgets (do NOT remove existing handlers — append). Keep the save-time confirm dialog unchanged.
- [ ] **Step 3 (#6 payment settlement):** Find where the revenue booking is recomputed for an Ausgangsrechnung (the E16.2 path that books Soll Forderung). When `ausgangsrechnungCheck.Checked` AND `strings.TrimSpace(paymentDateEntry.Text) != ""`, settle the booking to the bank: `if pay, ok := a.settings.PaymentAccountSKR04(<selected bankkonto>); ok { booking = booking.WithSettlementAccount(pay) }` — so a paid outgoing invoice books Soll Bank instead of Forderung 1400 already at entry. Re-run this inside the recompute path so toggling the box or editing the Bezahldatum updates it. Chain `recompute` into `paymentDateEntry.OnChanged`.
- [ ] **Step 4 (#8 keyboard):** After `confirmWin.Show()` (or before), set `confirmWin.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent){ ... })`: `fyne.KeyEscape` → trigger the existing cancel/close path (`confirmWin.Close()`); `fyne.KeyReturn`/`fyne.KeyEnter` → only when NOT focused in a multi-line entry, trigger the save action. If reliably detecting focus is hard, bind save to a modifier instead by leaving Return alone and documenting that Escape cancels. Prefer: Escape = cancel (always safe); Return = save only if it does not interfere with text entry.
- [ ] **Step 5:** `go build ./... && go test ./...`. Commit `E17.4: source badge + live warnings + payment-settled booking + Esc/Enter keys`.

## Self-Review

Coverage: #7 → Task 1 + Task 2 step 1; #5 → step 2; #6 → step 3; #8 → step 4. All gated on existing widgets; expense flow and save-time validation unchanged. `Meta.Quelle` is transient (not written to DB/CSV).
