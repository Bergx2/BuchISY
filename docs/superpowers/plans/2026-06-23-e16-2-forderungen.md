# E16.2 — Soll-Besteuerung / Forderungen (Option A)

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** An outgoing invoice books **Soll Forderung (1400) / Haben Erlös + USt** (receivable recognised at issuance, Soll-Besteuerung). When the Erlös-Abgleich links the incoming payment, it switches the Soll side **1400 → bank** (paid).

**Tech:** Go 1.25, Fyne. `internal/core`, `internal/ui`. Branch `feat/e16-2-forderungen`.

## Global Constraints

- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- The change is gated on a configured `forderungskonto`: when `rules.ForderungsKonto != 0` the revenue booking's Soll = that account (Soll-Besteuerung); otherwise it stays `paymentAccount` (cash-basis — unchanged for profiles without the config). Backward-compatible.
- UStVA is already issuance-based (`ComputeUStVAOfficial`) → no UStVA change.

---

### Task 1: Core — Forderungskonto in the booking + settlement helper

**Files:** `internal/core/buchungsregeln.go`, `internal/core/buchung.go`, tests in `internal/core/buchung_test.go`.

**Interfaces:** `BookingRules.ForderungsKonto int` (json `forderungskonto,omitempty`); `BuildRevenueBooking` Soll = ForderungsKonto when set; new `func (b Booking) WithSettlementAccount(bankKonto int) Booking`.

- [ ] **Step 1: Tests** (append to `buchung_test.go`):

```go
func TestBuildRevenueBookingWithForderung(t *testing.T) {
	rules := &BookingRules{UmsatzsteuerKonten: map[string]int{"19": 1776}, ForderungsKonto: 1400}
	lines := []TaxLine{{Netto: 6500, SatzProzent: 19, MwStBetrag: 1235}}
	b, err := BuildRevenueBooking(rules, lines, 8400, 1200) // paymentAccount 1200 ignored when Forderung set
	if err != nil {
		t.Fatal(err)
	}
	base, _, ok := b.PaymentAndCounters(true) // single Soll
	if !ok || base.Konto != 1400 || base.Betrag != 7735 {
		t.Fatalf("Soll = %+v, want 1400 / 7735", base)
	}
	if !b.Balanced() {
		t.Fatal("not balanced")
	}
	// settle: switch Soll 1400 → 1200 (paid)
	s := b.WithSettlementAccount(1200)
	sb, _, _ := s.PaymentAndCounters(true)
	if sb.Konto != 1200 || sb.Betrag != 7735 {
		t.Errorf("settled Soll = %+v, want 1200 / 7735", sb)
	}
	// Haben side (Erlös + USt) unchanged after settlement.
	if !s.Balanced() {
		t.Error("settled booking not balanced")
	}
}

func TestBuildRevenueBookingCashBasisFallback(t *testing.T) {
	rules := &BookingRules{UmsatzsteuerKonten: map[string]int{"19": 1776}} // no ForderungsKonto
	b, _ := BuildRevenueBooking(rules, []TaxLine{{Netto: 100, SatzProzent: 19, MwStBetrag: 19}}, 8400, 1200)
	base, _, _ := b.PaymentAndCounters(true)
	if base.Konto != 1200 {
		t.Errorf("cash-basis Soll = %d, want 1200 (paymentAccount)", base.Konto)
	}
}
```

- [ ] **Step 2:** run → fail.
- [ ] **Step 3:** Add `ForderungsKonto int json:"forderungskonto,omitempty"` to `BookingRules`. In `BuildRevenueBooking`, replace the final Soll entry's account: compute `sollKonto := paymentAccount; if rules.ForderungsKonto != 0 { sollKonto = rules.ForderungsKonto }` and use `sollKonto` in the prepended Soll entry. Add to `buchung.go`:

```go
// WithSettlementAccount returns a copy of a revenue booking with its single Soll
// (receivable) entry's account changed to bankKonto — used when the incoming
// payment of an outgoing invoice is reconciled (Forderung → Bank). No-op unless
// there is exactly one Soll entry.
func (b Booking) WithSettlementAccount(bankKonto int) Booking {
	sollCount := 0
	for _, e := range b.Entries {
		if e.Soll {
			sollCount++
		}
	}
	if sollCount != 1 {
		return b
	}
	out := Booking{Info: b.Info, Manuell: b.Manuell, Entries: make([]BookingEntry, len(b.Entries))}
	copy(out.Entries, b.Entries)
	for i := range out.Entries {
		if out.Entries[i].Soll {
			out.Entries[i].Konto = bankKonto
		}
	}
	return out
}
```

- [ ] **Step 4:** run → pass + full core. Commit `E16.2: revenue books Soll Forderungskonto; Booking.WithSettlementAccount`.

---

### Task 2: UI — Erlös-Abgleich settles Forderung → Bank on link

**Files:** `internal/ui/erloesabgleichview.go`.

**Context:** READ `showErloesAbgleich` (built in E15.4). It links an outgoing invoice to a credit line by setting `row.BuchungRef` and `a.dbRepo.Update(...)`, in TWO places: the greedy auto-link loop and the confirm-button handler.

- [ ] **Step 1:** In BOTH link points, right before the `Update`, also settle the booking: resolve the bank account `pay, ok := a.settings.PaymentAccountSKR04(row.Bankkonto)` and, if ok, `row.Buchung = row.Buchung.WithSettlementAccount(pay)`. (For a bank-paid Ausgangsrechnung this switches the Soll 1400 → 1200, so the booking now reads "paid".) Keep everything else (BuchungRef, Update, alias/label) unchanged.
- [ ] **Step 2:** `go build ./... && go test ./...`. Commit `E16.2: Erlös-Abgleich switches Forderung → Bank on payment link`.

---

### Task 3: Rollout data (no repo code)

- [ ] (app CLOSED) Add chart account **1400 "Forderungen aus Lieferungen und Leistungen"** to the Bergx2 `chart_skr04.json`; add `"forderungskonto": 1400` to the profile `buchungsregeln.json`. Re-book the 2 existing Ausgangsrechnungen (Symeo, Wullehus) to Soll 1400 (issuance state; the user reconciles via Erlös-Abgleich to mark paid). Rebuild exe.

## Self-Review

Coverage: B#3 Option A → Task 1 (booking + settlement helper) + Task 2 (reconciliation settles) + Task 3 (data). Cash-basis profiles unchanged (fallback when forderungskonto unset). Export already handles a single-Soll revenue booking via `PaymentAndCounters(true)` regardless of which account the Soll is (1400 or bank).
