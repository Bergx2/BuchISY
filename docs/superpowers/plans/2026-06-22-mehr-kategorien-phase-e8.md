# Mehr Buchungskategorien (Phase E8) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add four booking categories — §13b Reverse-Charge, Geschenke (35-€ threshold), Reisekosten, Kfz — with per-profile-editable accounts, so auto-bookings cover these cases.

**Architecture:** Extend `BookingRule` with the new categories' account fields and seed `buchungsregeln.json` with standard SKR04 defaults. Extend `BuildBooking`: §13b and Geschenke>35 € have their own balanced structure (early return); Geschenke≤35 €, Reisekosten, Kfz reuse the shared Vorsteuer+payment logic. The E6 Settings "Buchungsregeln" section gains pickers for the new accounts (per profile).

**Tech Stack:** Go 1.25, Fyne v2. Reuses Phase C `BuildBooking`, Phase E6 `BookingRulesStore` + the Settings rules section, the SKR04 account-picker pattern.

## Global Constraints

- NEVER invent accounts. Defaults are standard SKR04: §13b Vorsteuer **1407**, §13b Umsatzsteuer **3837** (19 %); Geschenke abziehbar **6610**, nicht abziehbar **6620**; Reisekosten **6650**; Kfz **6520**. All are per-profile-editable in Settings (e.g. Boomstraat §13b 1577/1787).
- Conversion correctness:
  - **reverse_charge**: Soll expenseAccount = net; Soll §13b-Vorsteuer = net×rc_satz%; Haben §13b-Umsatzsteuer = net×rc_satz%; Haben payment = net. (Saldoneutral VAT; payment = net only.) The §13b VAT is computed from the net (the supplier invoice carries no VAT line).
  - **geschenke ≤ Schwelle**: Soll konto_abziehbar = net; + normal Vorsteuer per rate; Haben payment = gross.
  - **geschenke > Schwelle**: Soll konto_nicht_abziehbar = gross (net+VAT); Haben payment = gross. (No input-VAT deduction.)
  - **reisekosten / kfz**: like "standard" but Soll = the rule's default_konto (not the dialog Gegenkonto); + normal Vorsteuer per rate; Haben payment = gross.
- Every returned Booking balances (Σ Soll == Σ Haben). Money `float64`, `round2`.
- The category dropdown auto-lists rules from `bookingRules.Regeln` (existing `bookingCategoryOptions`), so the new categories appear with no UI change.
- All user-facing strings via `a.bundle.T(...)` in both JSONs (valid JSON). `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: BookingRule fields + rules-base data

**Files:**
- Modify: `internal/core/buchungsregeln.go` (BookingRule fields)
- Modify: `assets/buchungsregeln.json` (4 new rules)
- Test: `internal/core/buchungsregeln_test.go`

**Interfaces:**
- Produces: `BookingRule` gains `RcSatz float64` (json `rc_satz,omitempty`), `KontoVStRC int` (`konto_vst_rc,omitempty`), `KontoUStRC int` (`konto_ust_rc,omitempty`), `Schwelle float64` (`schwelle,omitempty`), `DefaultKonto int` (`default_konto,omitempty`). The bundled rules base gains `reverse_charge`, `geschenke`, `reisekosten`, `kfz`.

- [ ] **Step 1: Write the failing test**

```go
func TestBundledChartHasNewCategories(t *testing.T) {
	r, err := ParseBookingRules(assetsBuchungsregeln(t))
	if err != nil {
		t.Fatal(err)
	}
	rc, ok := r.Rule("reverse_charge")
	if !ok || rc.RcSatz != 19 || rc.KontoVStRC != 1407 || rc.KontoUStRC != 3837 {
		t.Errorf("reverse_charge rule = %+v", rc)
	}
	g, ok := r.Rule("geschenke")
	if !ok || g.Schwelle != 35 || g.KontoAbziehbar != 6610 || g.KontoNichtAbziehbar != 6620 {
		t.Errorf("geschenke rule = %+v", g)
	}
	rk, ok := r.Rule("reisekosten")
	if !ok || rk.DefaultKonto != 6650 {
		t.Errorf("reisekosten rule = %+v", rk)
	}
	kfz, ok := r.Rule("kfz")
	if !ok || kfz.DefaultKonto != 6520 {
		t.Errorf("kfz rule = %+v", kfz)
	}
}

// assetsBuchungsregeln reads the bundled rules JSON from disk for the test.
func assetsBuchungsregeln(t *testing.T) []byte {
	data, err := os.ReadFile(filepath.Join("..", "..", "assets", "buchungsregeln.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
```

(Add `os` + `path/filepath` to the test imports if missing. If a helper with that name already exists in the test file, reuse it instead.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestBundledChartHasNewCategories`
Expected: FAIL (unknown field / rule missing).

- [ ] **Step 3: Implement**

In `internal/core/buchungsregeln.go`, add to `BookingRule`:

```go
	RcSatz       float64 `json:"rc_satz,omitempty"`        // §13b VAT rate (percent)
	KontoVStRC   int     `json:"konto_vst_rc,omitempty"`   // §13b Vorsteuer account
	KontoUStRC   int     `json:"konto_ust_rc,omitempty"`   // §13b Umsatzsteuer account
	Schwelle     float64 `json:"schwelle,omitempty"`       // threshold (e.g. Geschenke 35 €)
	DefaultKonto int     `json:"default_konto,omitempty"`  // default expense account (Reisekosten/Kfz)
```

In `assets/buchungsregeln.json`, add the four rules to the `regeln` array (after `bewirtung`):

```json
    { "kategorie": "reverse_charge", "name": "Reverse-Charge (§ 13b UStG)", "rc_satz": 19, "konto_vst_rc": 1407, "konto_ust_rc": 3837 },
    { "kategorie": "geschenke", "name": "Geschenke", "schwelle": 35, "konto_abziehbar": 6610, "konto_nicht_abziehbar": 6620 },
    { "kategorie": "reisekosten", "name": "Reisekosten", "default_konto": 6650 },
    { "kategorie": "kfz", "name": "Kfz-Kosten", "default_konto": 6520 }
```

(Ensure valid JSON: a comma after the `bewirtung` entry, no trailing comma after `kfz`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestBundledChartHasNewCategories && go build ./...`
Expected: PASS. Validate the JSON: `python -c "import json;json.load(open('assets/buchungsregeln.json'))"`.

- [ ] **Step 5: Commit**

```bash
git add internal/core/buchungsregeln.go assets/buchungsregeln.json internal/core/buchungsregeln_test.go
git commit -m "Add booking rule fields + rules for §13b, Geschenke, Reisekosten, Kfz"
```

---

### Task 2: BuildBooking — new category logic

**Files:**
- Modify: `internal/core/buchung.go` (the `BuildBooking` switch)
- Test: `internal/core/buchung_test.go`

**Interfaces:**
- Consumes: the new `BookingRule` fields, `SumNetto`/`SumMwSt`, `round2`.
- Produces: `BuildBooking` handles `reverse_charge`, `geschenke`, `reisekosten`, `kfz` (in addition to `standard`/`bewirtung`), each returning a balanced Booking.

- [ ] **Step 1: Write the failing test**

```go
func TestBuildBookingNewCategories(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{"vorsteuer_konten":{"19":1406,"7":1401},"regeln":[
		{"kategorie":"standard","name":"S"},
		{"kategorie":"reverse_charge","name":"RC","rc_satz":19,"konto_vst_rc":1407,"konto_ust_rc":3837},
		{"kategorie":"geschenke","name":"G","schwelle":35,"konto_abziehbar":6610,"konto_nicht_abziehbar":6620},
		{"kategorie":"reisekosten","name":"R","default_konto":6650}]}`))

	// reverse_charge: net 100 → expense 100, VSt§13b 19, USt§13b 19, payment 100; balanced.
	rc, err := BuildBooking(rules, "reverse_charge", []TaxLine{{Netto: 100, SatzProzent: 0, MwStBetrag: 0}}, 0, 6300, 1800)
	if err != nil || !rc.Balanced() {
		t.Fatalf("rc not balanced: %+v err=%v", rc, err)
	}
	got := sollByKonto(rc)
	if !almost(got[6300], 100) || !almost(got[1407], 19) {
		t.Errorf("rc soll: %+v", got)
	}
	if !almost(habenByKonto(rc)[3837], 19) || !almost(habenByKonto(rc)[1800], 100) {
		t.Errorf("rc haben: %+v", habenByKonto(rc))
	}

	// geschenke ≤ 35: net 20, VAT 3.80 → 6610=20, 1406=3.80, payment 23.80.
	g1, _ := BuildBooking(rules, "geschenke", []TaxLine{{Netto: 20, SatzProzent: 19, MwStBetrag: 3.80}}, 0, 0, 1800)
	if !g1.Balanced() || !almost(sollByKonto(g1)[6610], 20) || !almost(sollByKonto(g1)[1406], 3.80) {
		t.Errorf("geschenke≤35: %+v", g1)
	}

	// geschenke > 35: net 40, VAT 7.60 → 6620 = 47.60 (gross), no Vorsteuer, payment 47.60.
	g2, _ := BuildBooking(rules, "geschenke", []TaxLine{{Netto: 40, SatzProzent: 19, MwStBetrag: 7.60}}, 0, 0, 1800)
	if !g2.Balanced() || !almost(sollByKonto(g2)[6620], 47.60) || sollByKonto(g2)[1406] != 0 {
		t.Errorf("geschenke>35: %+v", g2)
	}

	// reisekosten: net 100, VAT 19 → 6650=100, 1406=19, payment 119 (ignores passed expenseAccount).
	r, _ := BuildBooking(rules, "reisekosten", []TaxLine{{Netto: 100, SatzProzent: 19, MwStBetrag: 19}}, 0, 9999, 1800)
	if !r.Balanced() || !almost(sollByKonto(r)[6650], 100) || sollByKonto(r)[9999] != 0 {
		t.Errorf("reisekosten: %+v", r)
	}
}

func sollByKonto(b Booking) map[int]float64 {
	m := map[int]float64{}
	for _, e := range b.Entries {
		if e.Soll {
			m[e.Konto] += e.Betrag
		}
	}
	return m
}
func habenByKonto(b Booking) map[int]float64 {
	m := map[int]float64{}
	for _, e := range b.Entries {
		if !e.Soll {
			m[e.Konto] += e.Betrag
		}
	}
	return m
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestBuildBookingNewCategories`
Expected: FAIL (unknown category errors).

- [ ] **Step 3: Implement**

In `internal/core/buchung.go` `BuildBooking`, BEFORE the existing `switch` (which builds `entries` then runs the shared Vorsteuer loop + payment), add early-return handling for the two self-contained categories, then add cases to the switch for the shared-logic ones.

Replace the `switch kategorie { ... }` block plus the shared tail with this structure (keep `netTotal := round2(SumNetto(lines) + trinkgeld)` above it):

```go
	switch kategorie {
	case "reverse_charge":
		net := round2(SumNetto(lines) + trinkgeld)
		vat := round2(net * rule.RcSatz / 100)
		return Booking{Entries: []BookingEntry{
			{Konto: expenseAccount, Betrag: net, Soll: true},
			{Konto: rule.KontoVStRC, Betrag: vat, Soll: true},
			{Konto: rule.KontoUStRC, Betrag: vat, Soll: false},
			{Konto: paymentAccount, Betrag: net, Soll: false},
		}}, nil
	case "geschenke":
		if netTotal > rule.Schwelle {
			gross := round2(netTotal + SumMwSt(lines))
			return Booking{Entries: []BookingEntry{
				{Konto: rule.KontoNichtAbziehbar, Betrag: gross, Soll: true},
				{Konto: paymentAccount, Betrag: gross, Soll: false},
			}}, nil
		}
		entries = append(entries, BookingEntry{Konto: rule.KontoAbziehbar, Betrag: netTotal, Soll: true})
	case "bewirtung":
		abz := round2(netTotal * rule.AbziehbarProzent / 100)
		nicht := round2(netTotal - abz)
		entries = append(entries,
			BookingEntry{Konto: rule.KontoAbziehbar, Betrag: abz, Soll: true},
			BookingEntry{Konto: rule.KontoNichtAbziehbar, Betrag: nicht, Soll: true},
		)
	case "reisekosten", "kfz":
		entries = append(entries, BookingEntry{Konto: rule.DefaultKonto, Betrag: netTotal, Soll: true})
	case "standard":
		entries = append(entries, BookingEntry{Konto: expenseAccount, Betrag: netTotal, Soll: true})
	default:
		return Booking{}, fmt.Errorf("Buchungskategorie ohne Buchungslogik: %s", kategorie)
	}

	// Vorsteuer per rate (Soll), for the categories that fall through here.
	for _, l := range lines {
		if l.MwStBetrag == 0 {
			continue
		}
		if konto, ok := rules.VorsteuerKonto(l.SatzProzent); ok {
			entries = append(entries, BookingEntry{Konto: konto, Betrag: round2(l.MwStBetrag), Soll: true})
		}
	}

	// Payment (Haben) = Σ Soll, so the booking always balances.
	var sollSum float64
	for _, e := range entries {
		sollSum += e.Betrag
	}
	entries = append(entries, BookingEntry{Konto: paymentAccount, Betrag: round2(sollSum), Soll: false})
	return Booking{Entries: entries}, nil
```

(This preserves the existing standard/bewirtung behaviour and the Σ-Soll payment derivation; only the two early-return cases bypass the shared tail. Confirm `SumMwSt` exists in `taxline.go` — it does.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestBuildBooking && go build ./...`
Expected: PASS (the new test + the existing `TestBuildBooking*` all green).

- [ ] **Step 5: Commit**

```bash
git add internal/core/buchung.go internal/core/buchung_test.go
git commit -m "BuildBooking: §13b reverse-charge, Geschenke threshold, Reisekosten/Kfz"
```

---

### Task 3: Per-profile accounts for the new categories (Settings)

**Files:**
- Modify: `internal/ui/settings.go` (extend the E6 "Buchungsregeln" section)
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `a.bookingRules`, `a.bookingRulesStore.Save`, `a.showAccountSearch`, `paymentSKR04Label`.
- Produces: account pickers in the Buchungsregeln settings section for §13b Vorsteuer/Umsatzsteuer, Geschenke abziehbar/nicht-abziehbar, Reisekosten, Kfz — saved per profile.

- [ ] **Step 1: Add pickers + seed from rules**

In `internal/ui/settings.go`, find the E6 "Buchungsregeln" section (search `settings.rules.section` / `bewAbz`). Seed six more locals from the loaded rules (default to the bundled values when a rule is missing):

```go
	vstRC, ustRC := 1407, 3837
	if r, ok := a.bookingRules.Rule("reverse_charge"); ok {
		if r.KontoVStRC != 0 { vstRC = r.KontoVStRC }
		if r.KontoUStRC != 0 { ustRC = r.KontoUStRC }
	}
	geschAbz, geschNicht := 6610, 6620
	if r, ok := a.bookingRules.Rule("geschenke"); ok {
		if r.KontoAbziehbar != 0 { geschAbz = r.KontoAbziehbar }
		if r.KontoNichtAbziehbar != 0 { geschNicht = r.KontoNichtAbziehbar }
	}
	reiseKonto, kfzKonto := 6650, 6520
	if r, ok := a.bookingRules.Rule("reisekosten"); ok && r.DefaultKonto != 0 { reiseKonto = r.DefaultKonto }
	if r, ok := a.bookingRules.Rule("kfz"); ok && r.DefaultKonto != 0 { kfzKonto = r.DefaultKonto }
```

Build a label + pick button for each (mirror the existing E6 `vst19Lbl`/`vst19Btn` rows: `a.showAccountSearch(current, func(n int){ current = n; lbl.SetText(paymentSKR04Label(a, n)) })`), with i18n labels: `settings.rules.vstrc` ("§13b Vorsteuer"), `settings.rules.ustrc` ("§13b Umsatzsteuer"), `settings.rules.geschabz` ("Geschenke abziehbar"), `settings.rules.geschnicht` ("Geschenke nicht abziehbar"), `settings.rules.reise` ("Reisekosten-Konto"), `settings.rules.kfz` ("Kfz-Konto"). Reuse `settings.rules.pick` for the button caption. Add the six rows to the Buchungsregeln section layout.

- [ ] **Step 2: Persist on save (edit the loaded rules in place)**

In the settings save action, where E6 already edits `a.bookingRules` in place (search `rules.VorsteuerKonten["19"] = vst19`), add — after that block — updates for the new categories. Use a small helper to find-or-append a rule by category, OR (simpler) only update rules that already exist (they do, because they're in the bundled base loaded at startup):

```go
	for i := range rules.Regeln {
		switch rules.Regeln[i].Kategorie {
		case "reverse_charge":
			rules.Regeln[i].KontoVStRC = vstRC
			rules.Regeln[i].KontoUStRC = ustRC
		case "geschenke":
			rules.Regeln[i].KontoAbziehbar = geschAbz
			rules.Regeln[i].KontoNichtAbziehbar = geschNicht
		case "reisekosten":
			rules.Regeln[i].DefaultKonto = reiseKonto
		case "kfz":
			rules.Regeln[i].DefaultKonto = kfzKonto
		}
	}
```

(This runs on the same `rules := a.bookingRules` pointer E6 already mutates and `a.bookingRulesStore.Save(rules)` already persists, then `a.bookingRules = rules`. No second Save needed — fold these into the existing block before the existing Save call.)

Add the six i18n keys to both JSONs (valid).

- [ ] **Step 3: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/settings.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Settings: per-profile accounts for §13b, Geschenke, Reisekosten, Kfz"
```

---

## Self-Review

- **Spec coverage:** §13b reverse-charge (saldoneutral VAT), Geschenke 35-€ threshold split, Reisekosten/Kfz default-account categories (Tasks 1/2), all with per-profile-editable accounts (Task 3) + standard SKR04 defaults. Skonto intentionally out of scope (user-confirmed). The dropdown auto-lists them (existing `bookingCategoryOptions`).
- **Placeholder scan:** Tasks 1/2 fully coded with the balanced-booking examples; Task 3 references the concrete E6 anchors (the rules section, the in-place edit block) with full snippets.
- **Type consistency:** `BookingRule.{RcSatz,KontoVStRC,KontoUStRC,Schwelle,DefaultKonto}`; the new `BuildBooking` cases use them; Settings edits the same fields in place. Consistent.
- **Data integrity:** all bookings balance (early-return cases hand-build Σ Soll == Σ Haben; the rest derive payment = Σ Soll); accounts are real SKR04 defaults, never invented, profile-overridable; §13b VAT computed from net at rc_satz; Geschenke>35 books gross with no input-VAT deduction.
- **Out of scope:** Skonto; auto-detecting §13b/Geschenke (the user picks the category in the dialog).
