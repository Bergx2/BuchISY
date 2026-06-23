# E15.3 — Controlling (Einnahmen vs. Ausgaben) + Zusammenfassende Meldung (ZM)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Split Controlling into revenue vs. expense (with Saldo), and add a Zusammenfassende Meldung (EC Sales List) of intra-EU reverse-charge supplies per customer VAT-ID.

**Architecture:** Mirror existing core+view patterns. Controlling stops summing all Soll-by-account; it excludes tax + payment accounts and splits the rest by booking side (Soll→Ausgaben, Haben→Einnahmen). ZM is a new core computation + dialog driven by the `Ausgangsrechnung` flag + the customer VAT-ID in `CSVRow.VATID`.

**Tech Stack:** Go 1.25, Fyne. `internal/core`, `internal/ui`.

## Global Constraints

- Go 1.25; `go build ./...`, `go test ./...`. Amounts via `round2`.
- Branch `feat/e15-3-controlling-zm`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- i18n keys added to BOTH `assets/i18n/de.json` and `en.json`, valid JSON, BOM-free.
- ZM is filed QUARTERLY; the official line shows customer USt-IdNr + net (full euros) + "Sonstige Leistung". Bergx2 does services → default Art der Leistung = "Sonstige Leistung".
- ZM-relevant = `r.Ausgangsrechnung && IsEUVatID(r.VATID) && core.SumMwSt(r.TaxLines)==0` (outgoing, EU customer VAT-ID, no VAT charged → reverse charge). Switzerland/Drittland (no EU VAT-ID) is correctly excluded.

---

### Task 1: Controlling core — Einnahmen/Ausgaben split

**Files:**
- Modify: `internal/core/controlling.go`
- Test: `internal/core/controlling_test.go`

**Interfaces:**
- Produces: `Controlling` struct + `AggregateControlling(rows []CSVRow, rules *BookingRules, paymentKonten map[int]bool, chart *ChartOfAccounts) Controlling`. Keep `AccountSum` as-is. Keep the existing `AggregateBookingsByAccount` (still used elsewhere) — ADD the new function, don't remove the old.

- [ ] **Step 1: Write the failing test** — `internal/core/controlling_test.go` (new or append):

```go
func TestAggregateControlling(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{"vorsteuer_konten":{"19":1576},"umsatzsteuer_konten":{"19":1776},"regeln":[{"kategorie":"reverse_charge","rc_satz":19,"konto_vst_rc":1577,"konto_ust_rc":1787}]}`))
	pay := map[int]bool{1200: true, 1000: true}
	rows := []CSVRow{
		// expense: Soll 4240 (expense) + 1576 (VAT, excluded), Haben 1200 (payment, excluded)
		{Buchung: Booking{Entries: []BookingEntry{{Konto: 4240, Betrag: 100, Soll: true}, {Konto: 1576, Betrag: 19, Soll: true}, {Konto: 1200, Betrag: 119, Soll: false}}}},
		// revenue: Soll 1200 (payment, excluded), Haben 8400 (revenue) + 1776 (VAT, excluded)
		{Ausgangsrechnung: true, Buchung: Booking{Entries: []BookingEntry{{Konto: 1200, Betrag: 119, Soll: true}, {Konto: 8400, Betrag: 100, Soll: false}, {Konto: 1776, Betrag: 19, Soll: false}}}},
		// §13b: Soll 27 (expense) + 1577 (VAT, excluded), Haben 1787 (VAT, excluded) + 1200 (payment)
		{Buchung: Booking{Entries: []BookingEntry{{Konto: 27, Betrag: 50, Soll: true}, {Konto: 1577, Betrag: 9.5, Soll: true}, {Konto: 1787, Betrag: 9.5, Soll: false}, {Konto: 1200, Betrag: 50, Soll: false}}}},
	}
	c := AggregateControlling(rows, rules, pay, nil)
	if !almost(c.EinnahmenGesamt, 100) {
		t.Errorf("Einnahmen = %v, want 100 (only 8400)", c.EinnahmenGesamt)
	}
	if !almost(c.AusgabenGesamt, 150) {
		t.Errorf("Ausgaben = %v, want 150 (4240 + 27)", c.AusgabenGesamt)
	}
	if !almost(c.Saldo, -50) {
		t.Errorf("Saldo = %v, want -50", c.Saldo)
	}
	if len(c.Einnahmen) != 1 || c.Einnahmen[0].Konto != 8400 {
		t.Fatalf("Einnahmen lines = %+v (want one, 8400)", c.Einnahmen)
	}
	if len(c.Ausgaben) != 2 {
		t.Fatalf("Ausgaben lines = %+v (want 4240 and 27)", c.Ausgaben)
	}
}
```

- [ ] **Step 2: Run → fail**: `go test ./internal/core/ -run TestAggregateControlling` → `AggregateControlling`/`Controlling` undefined.

- [ ] **Step 3: Implement** in `controlling.go`:

```go
// Controlling splits a period's bookings into revenue (Einnahmen, Haben on
// non-tax/non-payment accounts) and expense (Ausgaben, Soll on non-tax/non-
// payment accounts), with the resulting Saldo (Einnahmen − Ausgaben).
type Controlling struct {
	Einnahmen       []AccountSum
	Ausgaben        []AccountSum
	EinnahmenGesamt float64
	AusgabenGesamt  float64
	Saldo           float64
}

// AggregateControlling sums P&L accounts only: it excludes the profile's VAT
// accounts (Vorsteuer + Umsatzsteuer + §13b) and the payment accounts
// (paymentKonten). On the remaining accounts, Soll entries are expenses and
// Haben entries are revenue. Names come from chart (nil → empty).
func AggregateControlling(rows []CSVRow, rules *BookingRules, paymentKonten map[int]bool, chart *ChartOfAccounts) Controlling {
	exclude := map[int]bool{}
	for k := range paymentKonten {
		exclude[k] = true
	}
	for _, k := range rules.VorsteuerKonten {
		exclude[k] = true
	}
	for _, k := range rules.UmsatzsteuerKonten {
		exclude[k] = true
	}
	if rc, ok := rules.Rule("reverse_charge"); ok {
		if rc.KontoVStRC != 0 {
			exclude[rc.KontoVStRC] = true
		}
		if rc.KontoUStRC != 0 {
			exclude[rc.KontoUStRC] = true
		}
	}

	einn := map[int]float64{}
	ausg := map[int]float64{}
	for _, r := range rows {
		for _, e := range r.Buchung.Entries {
			if exclude[e.Konto] {
				continue
			}
			if e.Soll {
				ausg[e.Konto] += e.Betrag
			} else {
				einn[e.Konto] += e.Betrag
			}
		}
	}
	var c Controlling
	c.Einnahmen, c.EinnahmenGesamt = toSums(einn, chart)
	c.Ausgaben, c.AusgabenGesamt = toSums(ausg, chart)
	c.Saldo = round2(c.EinnahmenGesamt - c.AusgabenGesamt)
	return c
}

// toSums turns an account→amount map into sorted AccountSums + the rounded total.
func toSums(byKonto map[int]float64, chart *ChartOfAccounts) ([]AccountSum, float64) {
	sums := make([]AccountSum, 0, len(byKonto))
	var total float64
	for konto, summe := range byKonto {
		name := ""
		if chart != nil {
			if acc, ok := chart.Find(konto); ok {
				name = acc.Name
			}
		}
		sums = append(sums, AccountSum{Konto: konto, Name: name, Summe: round2(summe)})
		total += summe
	}
	sort.Slice(sums, func(i, j int) bool { return sums[i].Konto < sums[j].Konto })
	return sums, round2(total)
}
```

- [ ] **Step 4: Run → pass**: `go test ./internal/core/ -run TestAggregateControlling`. Also `go test ./internal/core/` (full).
- [ ] **Step 5: Commit** `E15.3: Controlling splits revenue vs expense (AggregateControlling)`.

---

### Task 2: Controlling view + PDF

**Files:**
- Modify: `internal/ui/controllingview.go`
- Modify: `internal/core/pdfreport.go` (`BuildControllingPDF` ~line 121) + `internal/core/pdfreport_test.go`
- Modify: `assets/i18n/de.json`, `en.json`

**Interfaces:**
- Consumes: `core.AggregateControlling`, `core.Controlling`.
- The UI builds `paymentKonten` from settings: for each `a.settings.BankAccounts`, if `SKR04Konto != 0` add it; else add the type default (`PaymentAccountSKR04` returns 1800 bank / 1600 cash). Simplest: `pay, ok := a.settings.PaymentAccountSKR04(ba.Name); if ok { paymentKonten[pay]=true }`.

- [ ] **Step 1:** In `controllingview.go` `reload()`, replace `core.AggregateBookingsByAccount(rows, a.chart)` with building `paymentKonten` (loop `a.settings.BankAccounts` → `a.settings.PaymentAccountSKR04(ba.Name)`) then `c := core.AggregateControlling(rows, a.bookingRules, paymentKonten, a.chart)`. Render two tables (Einnahmen / Ausgaben) each with its total, and a bold `Saldo` line (use container.VBox with two labelled tables, or two widget.Table; keep the month/year toggle and the PDF button). Add i18n keys `controlling.einnahmen`, `controlling.ausgaben`, `controlling.saldo`.

- [ ] **Step 2:** Update `BuildControllingPDF` to take the `Controlling` struct (or both slices + saldo) and render two sections + Saldo. Update `pdfreport_test.go` accordingly. Signature change: `BuildControllingPDF(c Controlling, title string) ([]byte, error)`; update the caller in `controllingview.go`.

- [ ] **Step 3:** `go build ./... && go test ./...` → pass. Commit `E15.3: Controlling view + PDF show Einnahmen/Ausgaben/Saldo`.

---

### Task 3: ZM core

**Files:**
- Modify/Create: `internal/core/zm.go`
- Test: `internal/core/zm_test.go`

**Interfaces:**
- Produces: `ZMZeile{UStIdNr string; Netto float64}`, `ZM{Zeilen []ZMZeile; Kontrollsumme float64}`, `ComputeZM(rows []CSVRow) ZM`, `IsEUVatID(s string) bool`.

- [ ] **Step 1: Write the failing test** `internal/core/zm_test.go`:

```go
package core

import "testing"

func TestIsEUVatID(t *testing.T) {
	cases := map[string]bool{
		"FI26378052": true, "ATU12345678": true, "FR12345678901": true,
		"DE287472874": false, // domestic is not a ZM counterparty
		"CHE123456789": false, // Switzerland is not EU
		"": false, "12345": false,
	}
	for in, want := range cases {
		if IsEUVatID(in) != want {
			t.Errorf("IsEUVatID(%q) = %v, want %v", in, !want, want)
		}
	}
}

func TestComputeZM(t *testing.T) {
	rows := []CSVRow{
		// EU customer, no VAT, outgoing → ZM. net 6500.
		{Ausgangsrechnung: true, VATID: "FI26378052", TaxLines: []TaxLine{{Netto: 6500, SatzProzent: 0, MwStBetrag: 0}}},
		// same customer again, net 1000 → accumulates.
		{Ausgangsrechnung: true, VATID: "FI26378052", TaxLines: []TaxLine{{Netto: 1000, SatzProzent: 0, MwStBetrag: 0}}},
		// domestic outgoing (DE, has VAT) → excluded.
		{Ausgangsrechnung: true, VATID: "DE123", TaxLines: []TaxLine{{Netto: 100, SatzProzent: 19, MwStBetrag: 19}}},
		// Swiss outgoing, no EU VAT-ID → excluded.
		{Ausgangsrechnung: true, VATID: "", TaxLines: []TaxLine{{Netto: 500, SatzProzent: 0, MwStBetrag: 0}}},
		// incoming EU supplier (not Ausgangsrechnung) → excluded.
		{Ausgangsrechnung: false, VATID: "IE123", TaxLines: []TaxLine{{Netto: 200, SatzProzent: 0, MwStBetrag: 0}}},
	}
	z := ComputeZM(rows)
	if len(z.Zeilen) != 1 || z.Zeilen[0].UStIdNr != "FI26378052" || !almost(z.Zeilen[0].Netto, 7500) {
		t.Fatalf("ZM = %+v (want one line FI26378052 / 7500)", z.Zeilen)
	}
	if !almost(z.Kontrollsumme, 7500) {
		t.Errorf("Kontrollsumme = %v, want 7500", z.Kontrollsumme)
	}
}
```

- [ ] **Step 2: Run → fail**.

- [ ] **Step 3: Implement** `internal/core/zm.go`:

```go
package core

import (
	"sort"
	"strings"
)

// euVatPrefixes are the 2-letter country codes of EU member states that prefix a
// USt-IdNr (Greece uses "EL"). "DE" is intentionally excluded — a domestic
// customer is never a ZM counterparty.
var euVatPrefixes = map[string]bool{
	"AT": true, "BE": true, "BG": true, "CY": true, "CZ": true, "DK": true,
	"EE": true, "EL": true, "ES": true, "FI": true, "FR": true, "HR": true,
	"HU": true, "IE": true, "IT": true, "LT": true, "LU": true, "LV": true,
	"MT": true, "NL": true, "PL": true, "PT": true, "RO": true, "SE": true,
	"SI": true, "SK": true,
}

// IsEUVatID reports whether s looks like an EU VAT-ID of another member state
// (2-letter EU prefix, not DE, plus at least one following character).
func IsEUVatID(s string) bool {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) < 3 {
		return false
	}
	return euVatPrefixes[s[:2]]
}

// ZMZeile is one ZM line: a customer's EU VAT-ID and the summed net supplies.
type ZMZeile struct {
	UStIdNr string
	Netto   float64
}

// ZM is the Zusammenfassende Meldung for a period: one line per EU customer
// VAT-ID plus the control total.
type ZM struct {
	Zeilen        []ZMZeile
	Kontrollsumme float64
}

// ComputeZM sums the net of intra-EU reverse-charge supplies (outgoing invoices
// to an EU customer VAT-ID with no VAT charged), grouped per customer VAT-ID.
func ComputeZM(rows []CSVRow) ZM {
	byVat := map[string]float64{}
	for _, r := range rows {
		if !r.Ausgangsrechnung || !IsEUVatID(r.VATID) || SumMwSt(r.TaxLines) != 0 {
			continue
		}
		byVat[strings.ToUpper(strings.TrimSpace(r.VATID))] += SumNetto(r.TaxLines)
	}
	var z ZM
	for vat, netto := range byVat {
		netto = round2(netto)
		z.Zeilen = append(z.Zeilen, ZMZeile{UStIdNr: vat, Netto: netto})
		z.Kontrollsumme += netto
	}
	sort.Slice(z.Zeilen, func(i, j int) bool { return z.Zeilen[i].UStIdNr < z.Zeilen[j].UStIdNr })
	z.Kontrollsumme = round2(z.Kontrollsumme)
	return z
}
```

- [ ] **Step 4: Run → pass** (`TestIsEUVatID`, `TestComputeZM`, full core).
- [ ] **Step 5: Commit** `E15.3: ZM core (intra-EU sales per customer VAT-ID)`.

---

### Task 4: ZM view + menu

**Files:**
- Create: `internal/ui/zmview.go`
- Modify: `internal/ui/app.go` (menu ~line 730)
- Modify: `assets/i18n/de.json`, `en.json`

**Interfaces:**
- Consumes: `core.ComputeZM`, `a.collectInvoiceRows`.

- [ ] **Step 1:** Create `showZMDialog` in `internal/ui/zmview.go`, modeled on `showUStVADialog` (`internal/ui/ustvaview.go`) — read that file first for the exact pattern. Differences: a period toggle with THREE options — month / quarter / year (quarter = the calendar quarter containing `a.currentMonth`: Q = ((m-1)/3); months `(Q*3)+1 .. (Q*3)+3`). Build rows via `a.collectInvoiceRows(...)`, compute `core.ComputeZM(rows)`, and render one line per `ZMZeile` as `"{UStIdNr}   {Netto} €   {Sonstige Leistung}"`, plus a bold Kontrollsumme. Show the own VAT-ID from `a.settings.OwnVATID` in the header if set. i18n: `zm.title`, `zm.heading`, `zm.kontrollsumme` (`"Kontrollsumme: %s €"`), `zm.art.sonstige` (`"Sonstige Leistung"`), `zm.quarter` (`"Quartal"`), `zm.empty` (`"Keine EU-Umsätze im Zeitraum"`).

- [ ] **Step 2:** Add the menu item in `app.go` after `"USt-Voranmeldung"` (line ~730): `fyne.NewMenuItem("Zusammenfassende Meldung", func() { a.showZMDialog() }),`.

- [ ] **Step 3:** `go build ./... && go test ./...` → pass. Commit `E15.3: ZM dialog + menu (Zusammenfassende Meldung)`.

---

## Self-Review

**Spec coverage:** Controlling split → Task 1+2. ZM → Task 3+4. UStVA Kennzahlen mapping is explicitly deferred (E15.6, istok). 

**Placeholder scan:** complete code for all core logic; view/menu tasks describe wiring against named existing patterns (showUStVADialog, the app.go menu, BuildControllingPDF) the implementer reads first.

**Type consistency:** `Controlling`, `AggregateControlling(rows, rules, paymentKonten, chart)`, `ZM`, `ZMZeile`, `ComputeZM(rows)`, `IsEUVatID(string)` consistent between definitions (Tasks 1/3) and consumers (Tasks 2/4).
