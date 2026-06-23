# E16.1 — Ausgangsrechnungs-Komfort: Auto-Häkchen + Erlöskonto-Vorschlag

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** (A#1) auto-detect outgoing invoices (issued by the user's own company) and pre-check the Ausgangsrechnung box; (A#2) suggest the revenue account by customer type when it's an outgoing invoice.

**Tech:** Go 1.25, Fyne. `internal/core`, `internal/anthropic`, `internal/ui`. Branch `feat/e16-1-ausgang-komfort`.

## Global Constraints

- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- Revenue-account selection (mirrors the UStVA Kennzahlen classification): outgoing invoice with German VAT (SumMwSt>0) → domestic account; 0% VAT + EU customer VAT-ID (`IsEUVatID`) → §18b account; 0% VAT + no EU VAT-ID → Drittland account.
- Accounts are profile-specific → new `erloes_konten` config in `buchungsregeln.json` (like `umsatzsteuer_konten`).

---

### Task 1: `erloes_konten` config + revenue-account selector

**Files:** `internal/core/buchungsregeln.go`, `internal/core/bookingrulesstore.go` (mergeBundledIntoSaved), `assets/buchungsregeln.json`, test `internal/core/buchungsregeln_test.go` (or bookingrulesstore_test.go).

**Interfaces:** `BookingRules.ErloesKonten` (json `erloes_konten`, keys `"inland"`,`"eu"`,`"drittland"`) + `BookingRules.ErloesKonto(vatID string, mwst float64) (int, bool)`.

- [ ] **Step 1: Test** (append):

```go
func TestErloesKonto(t *testing.T) {
	r := &BookingRules{ErloesKonten: map[string]int{"inland": 8400, "eu": 8341, "drittland": 8200}}
	if k, _ := r.ErloesKonto("DE123", 19); k != 8400 {
		t.Errorf("domestic (with VAT) = %d, want 8400", k)
	}
	if k, _ := r.ErloesKonto("FI26378052", 0); k != 8341 {
		t.Errorf("EU 0%% = %d, want 8341", k)
	}
	if k, _ := r.ErloesKonto("", 0); k != 8200 {
		t.Errorf("Drittland 0%% = %d, want 8200", k)
	}
	if _, ok := (&BookingRules{}).ErloesKonto("DE", 19); ok {
		t.Error("unset config must return ok=false")
	}
}
```

- [ ] **Step 2:** run → fail.
- [ ] **Step 3:** Add to `BookingRules`: `ErloesKonten map[string]int json:"erloes_konten,omitempty"`. Add method:

```go
// ErloesKonto picks the revenue account for an outgoing invoice from its
// counterparty VAT-ID and VAT amount: German VAT → "inland"; 0% + EU VAT-ID →
// "eu" (§18b); 0% + non-EU → "drittland".
func (r *BookingRules) ErloesKonto(vatID string, mwst float64) (int, bool) {
	key := "drittland"
	if mwst > 0.005 {
		key = "inland"
	} else if IsEUVatID(vatID) {
		key = "eu"
	}
	k, ok := r.ErloesKonten[key]
	return k, ok
}
```

Extend `mergeBundledIntoSaved` to gap-fill `erloes_konten` (mirror the umsatzsteuer_konten merge). Add to `assets/buchungsregeln.json`: `"erloes_konten": { "inland": 8400, "eu": 8341, "drittland": 8200 }` (SKR04 seeds — fine as defaults; Bergx2 profile overrides via Task 4 data step).

- [ ] **Step 4:** run → pass + full core. Commit `E16.1: erloes_konten config + ErloesKonto selector`.

---

### Task 2: Extractor auto-detects Ausgangsrechnung

**Files:** `internal/anthropic/extractor.go`, test `internal/anthropic/extractor_test.go`.

**Context:** READ `extractor.go` — `systemPromptFor(ownVATIDs)` (~line 57) builds the JSON-schema prompt; the invoice result struct (~line 413) parses Claude's JSON; the Meta is built right after. `ownVATIDs` is already passed through `ExtractFromImage`/Extract.

- [ ] **Step 1:** In `systemPromptFor`, add to the JSON schema an `"ausgangsrechnung": true/false` field, with the instruction: *"ausgangsrechnung = true, wenn die Rechnung vom App-Nutzer AUSGESTELLT wurde — d.h. wenn der Rechnungs-AUSSTELLER eine der oben genannten eigenen USt-IdNrn trägt (Nutzer = Verkäufer/Aussteller). Bei einer normalen Eingangsrechnung (Nutzer = Empfänger) ist es false. Wenn keine eigene USt-IdNr bekannt/sichtbar ist, false."* Only emit this instruction meaningfully when `len(ownVATIDs) > 0` (else default false).
- [ ] **Step 2:** Add `Ausgangsrechnung *bool json:"ausgangsrechnung"` to the invoice result struct (~line 413). After building `meta`, set `if result.Ausgangsrechnung != nil { meta.Ausgangsrechnung = *result.Ausgangsrechnung }`.
- [ ] **Step 3:** Add/extend a test asserting the result struct unmarshals `"ausgangsrechnung": true` into `meta.Ausgangsrechnung` (call the JSON→Meta parse path used by the existing extractor tests; if there isn't a direct one, test that the struct field parses). `go build ./... && go test ./internal/anthropic/`.
- [ ] **Step 4:** Commit `E16.1: extractor auto-detects Ausgangsrechnung from own VAT-ID`.

---

### Task 3: UI — pre-check box + suggest Erlöskonto

**Files:** `internal/ui/invoicemodal.go`.

**Context:** READ the modal's account-selection (`selectedAccount` ~line 198) and the `ausgangsrechnungCheck` + `recomputeBooking` wiring (added in E15.1/E15.6). `meta.Ausgangsrechnung` now arrives from extraction (Task 2).

- [ ] **Step 1:** When building the modal, if `meta.Ausgangsrechnung` → `ausgangsrechnungCheck.SetChecked(true)` (the seed line added by the earlier review).
- [ ] **Step 2:** Add a helper that, for an outgoing invoice, suggests the revenue account: `if k, ok := a.bookingRules.ErloesKonto(meta.VATID, core.SumMwSt(ed.Lines())); ok { selectedAccount = k; accountDisplay update }`. Call it (a) on initial build when the box is pre-checked, and (b) in `ausgangsrechnungCheck.OnChanged` when it becomes checked — only overriding the account when the user hasn't manually picked one for this dialog (a simple bool guard `accountManuallyPicked`, set true in the account-search callback). Keep the expense flow unchanged when the box is off.
- [ ] **Step 3:** `go build ./... && go test ./...`. Commit `E16.1: pre-check Ausgangsrechnung + suggest Erlöskonto by customer type`.

---

### Task 4: Rollout data (no repo code)

- [ ] Set Bergx2 profile (app CLOSED): `settings.json` `own_vat_id` = `"287472874"`; `buchungsregeln.json` add `"erloes_konten": { "inland": 8400, "eu": 8341, "drittland": 8200 }`. Rebuild exe.

## Self-Review

Coverage: A#2 → Task 1+3; A#1 → Task 2+3. Config mirrors umsatzsteuer_konten. `IsEUVatID` reused. Expense flow untouched (all new logic gated on the Ausgangsrechnung flag).
