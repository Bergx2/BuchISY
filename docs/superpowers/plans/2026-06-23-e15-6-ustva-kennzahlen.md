# E15.6 — UStVA auf das offizielle Kennzahlen-Schema

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Compute the UStVA as the official ELSTER Kennzahlen (net bases + derived VAT + Zahllast), classified from invoice metadata, replacing the account-sum view from E15.2 with a Finanzamt-conform one.

**Architecture:** New `core.ComputeUStVAOfficial(rows, rules) UStVAOfficial` classifies each invoice into a Kennzahl from its `Ausgangsrechnung` flag, `TaxLines` (net/VAT per rate) and counterparty `VATID`, reusing `IsEUVatID`. The UStVA dialog renders the Kennzahlen + month/quarter/year. `ComputeUStVA` (E15.2) stays for internal account detail but the dialog switches to the official view.

**Tech Stack:** Go 1.25, Fyne. `internal/core`, `internal/ui`.

## Global Constraints

- Go 1.25; `go build ./...`, `go test ./...`. Amounts accumulate raw, round once at the end (no double-rounding).
- Branch `feat/e15-6-ustva-kennzahlen`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- Classification (all from metadata; `net=SumNetto`, `vat=SumMwSt`, threshold 0.005):
  - Ausgangsrechnung & vat>0 → domestic taxable: per TaxLine 19%→Kz81, 7%→Kz86 (net base).
  - Ausgangsrechnung & vat≈0 & IsEUVatID(VATID) → Kz21 (intra-EU §18b, net).
  - Ausgangsrechnung & vat≈0 & !IsEUVatID → Kz45 (non-taxable foreign, net).
  - incoming & IsEUVatID(VATID) & vat≈0 → §13b: Kz84 (net base).
  - incoming else → Kz66 (input VAT amount = vat).
  - Derived: Kz85 = round2(Kz84 × RcSatz/100); Kz67 = Kz85; USt81 = round2(Kz81×0.19); USt86 = round2(Kz86×0.07); Kz83 = round2((USt81+USt86+Kz85) − (Kz66+Kz67)).
  - RcSatz from `rules.Rule("reverse_charge").RcSatz` (default 19 if absent).
- i18n keys in BOTH de.json + en.json, valid JSON, BOM-free.

---

### Task 1: Core `ComputeUStVAOfficial`

**Files:**
- Create: `internal/core/ustva_official.go`
- Test: `internal/core/ustva_official_test.go`

**Interfaces:**
- Produces: `UStVAOfficial` struct + `ComputeUStVAOfficial(rows []CSVRow, rules *BookingRules) UStVAOfficial`. Reuses `IsEUVatID`, `SumNetto`, `SumMwSt`, `round2` (all already in package).

- [ ] **Step 1: Write the failing test** `internal/core/ustva_official_test.go`:

```go
package core

import "testing"

func TestComputeUStVAOfficial(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{"regeln":[{"kategorie":"reverse_charge","rc_satz":19}]}`))
	rows := []CSVRow{
		// domestic sale 19% (Symeo): net 6500 → Kz81
		{Ausgangsrechnung: true, VATID: "DE123", TaxLines: []TaxLine{{Netto: 6500, SatzProzent: 19, MwStBetrag: 1235}}},
		// foreign non-taxable (Wullehus CH): 0%, no EU VAT-ID → Kz45
		{Ausgangsrechnung: true, VATID: "", TaxLines: []TaxLine{{Netto: 1000, SatzProzent: 0, MwStBetrag: 0}}},
		// intra-EU service (EU customer): 0%, EU VAT-ID → Kz21
		{Ausgangsrechnung: true, VATID: "FI26378052", TaxLines: []TaxLine{{Netto: 2000, SatzProzent: 0, MwStBetrag: 0}}},
		// §13b incoming (Google IE): 0%, EU supplier VAT-ID → Kz84/85/67
		{Ausgangsrechnung: false, VATID: "IE123", TaxLines: []TaxLine{{Netto: 462.40, SatzProzent: 0, MwStBetrag: 0}}},
		// normal incoming with VAT: Kz66 += 31.19
		{Ausgangsrechnung: false, VATID: "DE999", TaxLines: []TaxLine{{Netto: 164.16, SatzProzent: 19, MwStBetrag: 31.19}}},
	}
	u := ComputeUStVAOfficial(rows, rules)
	check := func(name string, got, want float64) {
		if !almost(got, want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	check("Kz81", u.Kz81, 6500)
	check("Kz45", u.Kz45, 1000)
	check("Kz21", u.Kz21, 2000)
	check("Kz84", u.Kz84, 462.40)
	check("Kz85", u.Kz85, 87.86) // 462.40 * 19%
	check("Kz67", u.Kz67, 87.86)
	check("Kz66", u.Kz66, 31.19)
	check("USt81", u.USt81, 1235) // 6500 * 19%
	// Kz83 = (1235 + 0 + 87.86) - (31.19 + 87.86) = 1203.81
	check("Kz83", u.Kz83, 1203.81)
}
```

- [ ] **Step 2: Run → fail** (`go test ./internal/core/ -run TestComputeUStVAOfficial`).

- [ ] **Step 3: Implement** `internal/core/ustva_official.go`:

```go
package core

// UStVAOfficial is the VAT return in the official ELSTER Kennzahlen, computed
// from invoice metadata. Net bases (Kz81/86/21/45/84) plus the derived output
// VAT and the Zahllast (Kz83). Structured to feed an ELSTER export later.
type UStVAOfficial struct {
	Kz81 float64 // steuerpflichtige Umsätze 19 % (Bemessungsgrundlage, netto)
	Kz86 float64 // steuerpflichtige Umsätze 7 % (Bemessungsgrundlage, netto)
	Kz21 float64 // nicht steuerbare innergem. sonstige Leistungen (§ 18b), netto
	Kz45 float64 // übrige nicht steuerbare Umsätze (Leistungsort nicht im Inland), netto
	Kz84 float64 // § 13b Bemessungsgrundlage (netto)
	Kz85 float64 // § 13b Steuer
	Kz66 float64 // Vorsteuer aus Rechnungen anderer Unternehmer
	Kz67 float64 // Vorsteuer aus § 13b Leistungen
	USt81 float64 // = Kz81 × 19 % (derived)
	USt86 float64 // = Kz86 × 7 % (derived)
	Kz83  float64 // Zahllast/Überschuss = (USt81+USt86+Kz85) − (Kz66+Kz67)
}

// ComputeUStVAOfficial classifies each invoice into its Kennzahl from the
// Ausgangsrechnung flag, the tax lines, and the counterparty VAT-ID.
func ComputeUStVAOfficial(rows []CSVRow, rules *BookingRules) UStVAOfficial {
	rcSatz := 19.0
	if rc, ok := rules.Rule("reverse_charge"); ok && rc.RcSatz > 0 {
		rcSatz = rc.RcSatz
	}
	var u UStVAOfficial
	for _, r := range rows {
		net := SumNetto(r.TaxLines)
		vat := SumMwSt(r.TaxLines)
		if r.Ausgangsrechnung {
			if vat > 0.005 { // domestic taxable sale
				for _, l := range r.TaxLines {
					switch int(l.SatzProzent + 0.5) {
					case 19:
						u.Kz81 += l.Netto
					case 7:
						u.Kz86 += l.Netto
					}
				}
			} else if IsEUVatID(r.VATID) {
				u.Kz21 += net
			} else {
				u.Kz45 += net
			}
		} else { // incoming
			if IsEUVatID(r.VATID) && vat < 0.005 { // § 13b reverse-charge
				u.Kz84 += net
			} else {
				u.Kz66 += vat
			}
		}
	}
	u.Kz81 = round2(u.Kz81)
	u.Kz86 = round2(u.Kz86)
	u.Kz21 = round2(u.Kz21)
	u.Kz45 = round2(u.Kz45)
	u.Kz84 = round2(u.Kz84)
	u.Kz66 = round2(u.Kz66)
	u.Kz85 = round2(u.Kz84 * rcSatz / 100)
	u.Kz67 = u.Kz85
	u.USt81 = round2(u.Kz81 * 0.19)
	u.USt86 = round2(u.Kz86 * 0.07)
	u.Kz83 = round2((u.USt81 + u.USt86 + u.Kz85) - (u.Kz66 + u.Kz67))
	return u
}
```

- [ ] **Step 4: Run → pass** (target test + full `go test ./internal/core/`).
- [ ] **Step 5: Commit** `E15.6: ComputeUStVAOfficial (official Kennzahlen from metadata)`.

---

### Task 2: UStVA view shows the official Kennzahlen

**Files:**
- Modify: `internal/ui/ustvaview.go`
- Modify: `assets/i18n/de.json`, `en.json`

**Interfaces:**
- Consumes: `core.ComputeUStVAOfficial`. Keeps the existing `a.collectInvoiceRows` + the period toggle (ADD a quarter option like zmview).

- [ ] **Step 1:** Read `internal/ui/ustvaview.go` and `internal/ui/zmview.go` (for the 3-way month/quarter/year toggle pattern). Rewrite `showUStVADialog` to:
  - Period toggle: month / quarter / year (quarter = `q:=(int(a.currentMonth)-1)/3; months q*3+1..q*3+3`).
  - `u := core.ComputeUStVAOfficial(rows, a.bookingRules)`.
  - Render the Kennzahlen as labelled lines, only showing non-zero rows plus always Kz83. Each line: `"<Kz> <label>   <amount> €"`. Group with section headers (A. Umsätze: Kz81/86 + their USt; E. nicht steuerbar: Kz21/45; D. §13b: Kz84/85; F. Vorsteuer: Kz66/67). Bottom: bold Kz83 (Zahllast or Überschuss when negative).
  - German decimal comma (mirror existing `strings.Replace(fmt.Sprintf("%.2f",...),".",",",1)`).
- [ ] **Step 2:** Add i18n keys (de + en) for the Kennzahl labels and section headers: `ustva.kz81` ("Steuerpflichtige Umsätze 19 %"), `ustva.kz86` ("Steuerpflichtige Umsätze 7 %"), `ustva.kz21` ("Nicht steuerbare innergem. sonstige Leistungen (§ 18b)"), `ustva.kz45` ("Übrige nicht steuerbare Umsätze (Ausland)"), `ustva.kz84` ("§ 13b Bemessungsgrundlage"), `ustva.kz85` ("§ 13b Steuer"), `ustva.kz66` ("Vorsteuer aus Rechnungen"), `ustva.kz67` ("Vorsteuer § 13b"), `ustva.ust` ("Umsatzsteuer"), `ustva.zahllast` (already exists), `ustva.ueberschuss` (exists). Sensible English equivalents.
- [ ] **Step 3:** `go build ./... && go test ./...` → pass. Commit `E15.6: UStVA dialog shows official Kennzahlen (month/quarter/year)`.

---

## Self-Review

**Spec coverage:** Kennzahlen computation → Task 1 (mapping table in Global Constraints). View → Task 2. Future ELSTER export = the `UStVAOfficial` struct is the clean data carrier (out of scope here).

**Placeholder scan:** complete code for the core; view task references the existing zmview quarter pattern + ustvaview structure.

**Type consistency:** `UStVAOfficial` + `ComputeUStVAOfficial(rows, rules)` consistent between Task 1 (def) and Task 2 (consumer). Reuses `IsEUVatID` from E15.3.

**Tax note:** single RcSatz (19 %) per the booking model's limitation; a §13b 7 % invoice would be valued at 19 % — documented, matches the existing booking engine. Net bases come from metadata (TaxLines), so the UStVA is correct even for invoices without a stored booking.
