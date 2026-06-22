# Fremdwährung: Kurs & Kreditkartengebühr (Phase E7) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** For a foreign-currency (e.g. USD) credit-card payment, let the user enter the exchange rate and the credit-card fee percentage; the form converts to EUR and computes the fee automatically, and all values are stored.

**Worked example (must match):** USD 89,18 is debited at a rate of **1,1583 USD per EUR** → **EUR 76,99** (= 89,18 ÷ 1,1583). The bank charges **2 %** → **EUR 1,54** (= 76,99 × 2 %). Total debited = **EUR 78,53**.

**Architecture:** Two new persisted fields `Wechselkurs` (foreign units per EUR) and `GebuehrProzent` (fee %). A pure `ConvertForeignPayment` turns the foreign gross/net + rate + fee% into EUR amounts. Both invoice dialogs' currency-conversion section (shown when currency ≠ default) gains a Kurs entry and a Gebühr-% entry, recomputing the EUR net, the fee, and the total live. The existing `BetragNetto_EUR` (net in EUR) and `Gebuehr` (fee in EUR) fields keep their meaning and are filled from the conversion.

**Tech Stack:** Go 1.25, Fyne v2, SQLite. Reuses the existing currency-conversion UI (`netEUREntry`/`feeEntry`), the `round2` helper, and the column-persistence pattern.

## Global Constraints

- Conversion direction: **EUR = Fremdbetrag ÷ Wechselkurs** (Wechselkurs is foreign units per 1 EUR, e.g. 1,1583). Fee: **Gebühr(EUR) = Brutto(EUR) × GebuehrProzent ÷ 100**. Total = Brutto(EUR) + Gebühr(EUR). All rounded to 2 decimals with `round2`.
- New fields `Meta.Wechselkurs float64` + `Meta.GebuehrProzent float64` (and on `CSVRow`), persisted in CSV (columns `Wechselkurs`, `GebuehrProzent`) and SQLite (`wechselkurs REAL DEFAULT 0`, `gebuehr_prozent REAL DEFAULT 0`), NULL-safe on read.
- `BetragNetto_EUR` keeps meaning "net in EUR" = BetragNetto ÷ Wechselkurs; `Gebuehr` = fee in EUR. The fee field stays user-editable (the % auto-fills it but the user may override).
- The Kurs/Gebühr-% fields appear only when currency ≠ default (same visibility rule as the existing conversion fields). When currency = EUR, no rate/fee conversion.
- Insert/Update/List column/placeholder/arg counts must stay aligned (a miscount is a runtime error). Count them.
- All new user-facing strings via `a.bundle.T(...)` in both JSONs (valid JSON).
- `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: Persist Wechselkurs + GebuehrProzent

**Files:**
- Modify: `internal/core/types.go` (Meta + CSVRow + conversions)
- Modify: `internal/core/csvrepo.go` (columns + load/save)
- Modify: `internal/db/schema.go` + `internal/db/repository.go` (columns + migration + Insert/Update/List)
- Test: `internal/core/csvrepo_test.go`, `internal/db/repository_test.go`

**Interfaces:**
- Produces: `Meta.Wechselkurs float64` + `Meta.GebuehrProzent float64` and the same on `CSVRow`, round-tripped through CSV (`Wechselkurs`/`GebuehrProzent` columns) and SQLite (`wechselkurs`/`gebuehr_prozent` REAL columns).

- [ ] **Step 1: Write the failing tests**

Add to `internal/core/csvrepo_test.go`:

```go
func TestCSVWechselkursRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoices.csv")
	repo := NewCSVRepository()
	if err := repo.Append(path, CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "06", Wechselkurs: 1.1583, GebuehrProzent: 2}); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.Load(path)
	if len(rows) != 1 || rows[0].Wechselkurs != 1.1583 || rows[0].GebuehrProzent != 2 {
		t.Fatalf("kurs/prozent not round-tripped: %+v", rows)
	}
}
```

Add to `internal/db/repository_test.go`:

```go
func TestDBWechselkursRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.Insert(core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "06", Wechselkurs: 1.1583, GebuehrProzent: 2}); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.List("2026", "06")
	if len(rows) != 1 || rows[0].Wechselkurs != 1.1583 || rows[0].GebuehrProzent != 2 {
		t.Fatalf("DB kurs/prozent round-trip failed: %+v", rows)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/core/ -run TestCSVWechselkursRoundTrip; go test ./internal/db/ -run TestDBWechselkursRoundTrip`
Expected: FAIL (unknown field Wechselkurs).

- [ ] **Step 3: Implement (follow the existing Exportiert/Gebuehr column pattern exactly)**

In `internal/core/types.go`: add `Wechselkurs float64` and `GebuehrProzent float64` to `Meta` and `CSVRow` (after `Gebuehr`); carry both in `ToCSVRow()` and `ToMeta()`.

In `internal/core/csvrepo.go`: add `"Wechselkurs"` and `"GebuehrProzent"` to `DefaultCSVColumns` (after `"Gebuehr"`), to `ColumnDisplayNames` (`"Wechselkurs": "Wechselkurs"`, `"GebuehrProzent": "Gebühr %"`) and `ColumnTranslationKeys` (`"table.col.wechselkurs"`, `"table.col.gebuehrprozent"`); in `Load` after the row literal: `row.Wechselkurs = parseFloat(valueForColumn(record, headerMap, "Wechselkurs"))` and the same for GebuehrProzent; in `rowToRecord` valueMap: `"Wechselkurs": r.formatFloat(row.Wechselkurs)`, `"GebuehrProzent": r.formatFloat(row.GebuehrProzent)`. Add i18n keys `table.col.wechselkurs` (de "Wechselkurs"/en "Exchange rate") + `table.col.gebuehrprozent` (de "Gebühr %"/en "Fee %") to both JSONs.

In `internal/db/schema.go`: add `wechselkurs REAL DEFAULT 0,` and `gebuehr_prozent REAL DEFAULT 0,` after `exportiert INTEGER DEFAULT 0,`. In `internal/db/repository.go`:
- `initSchema` ALTER loop: add `"ALTER TABLE invoices ADD COLUMN wechselkurs REAL DEFAULT 0"` and `"ALTER TABLE invoices ADD COLUMN gebuehr_prozent REAL DEFAULT 0"`.
- `Insert`: add both columns + two `?` + two args (`row.Wechselkurs`, `row.GebuehrProzent`).
- `Update`: add `wechselkurs = ?`, `gebuehr_prozent = ?` to the SET clause + the two args (in the matching position before the jahr/monat/WHERE args).
- `List`: add both columns to SELECT + scan into `sql.NullFloat64` then `row.Wechselkurs = nk.Float64` (NULL-safe like trinkgeld).
COUNT discipline: after editing Insert and List, confirm column-count == placeholder-count == arg-count (was 25, now 27); Update adds two real placeholders+args.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/core/ ./internal/db/ && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/types.go internal/core/csvrepo.go internal/core/csvrepo_test.go internal/db/schema.go internal/db/repository.go internal/db/repository_test.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Persist Wechselkurs and GebuehrProzent per invoice"
```

---

### Task 2: ConvertForeignPayment (core)

**Files:**
- Create: `internal/core/waehrung.go`
- Test: `internal/core/waehrung_test.go`

**Interfaces:**
- Consumes: `round2`.
- Produces: `type ForeignConversion struct { BruttoEUR, NettoEUR, GebuehrEUR, GesamtEUR float64 }`;
  `ConvertForeignPayment(bruttoForeign, nettoForeign, kurs, gebuehrProzent float64) ForeignConversion` — `BruttoEUR = round2(bruttoForeign/kurs)`, `NettoEUR = round2(nettoForeign/kurs)`, `GebuehrEUR = round2(BruttoEUR*gebuehrProzent/100)`, `GesamtEUR = round2(BruttoEUR+GebuehrEUR)`. If `kurs <= 0`, all EUR amounts are 0 (avoid divide-by-zero).

- [ ] **Step 1: Write the failing test**

```go
package core

import "testing"

func TestConvertForeignPayment(t *testing.T) {
	// USD 89.18 gross at 1.1583 USD/EUR → 76.99 EUR; 2% fee → 1.54; total 78.53.
	c := ConvertForeignPayment(89.18, 74.94, 1.1583, 2)
	if !almost(c.BruttoEUR, 76.99) {
		t.Errorf("BruttoEUR = %v, want 76.99", c.BruttoEUR)
	}
	if !almost(c.GebuehrEUR, 1.54) {
		t.Errorf("GebuehrEUR = %v, want 1.54", c.GebuehrEUR)
	}
	if !almost(c.GesamtEUR, 78.53) {
		t.Errorf("GesamtEUR = %v, want 78.53", c.GesamtEUR)
	}
	// kurs 0 → no divide-by-zero, all zero
	if z := ConvertForeignPayment(89.18, 74.94, 0, 2); z.BruttoEUR != 0 || z.GesamtEUR != 0 {
		t.Errorf("kurs 0 should yield zero EUR: %+v", z)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestConvertForeignPayment`
Expected: FAIL (undefined ConvertForeignPayment).

- [ ] **Step 3: Implement**

Create `internal/core/waehrung.go`:

```go
package core

// ForeignConversion holds the EUR amounts derived from a foreign-currency
// payment plus its credit-card fee.
type ForeignConversion struct {
	BruttoEUR  float64
	NettoEUR   float64
	GebuehrEUR float64
	GesamtEUR  float64
}

// ConvertForeignPayment converts a foreign gross/net to EUR at the given rate
// (foreign units per EUR) and adds a percentage credit-card fee on the EUR
// gross. kurs <= 0 yields all-zero (no divide-by-zero).
func ConvertForeignPayment(bruttoForeign, nettoForeign, kurs, gebuehrProzent float64) ForeignConversion {
	if kurs <= 0 {
		return ForeignConversion{}
	}
	brutto := round2(bruttoForeign / kurs)
	netto := round2(nettoForeign / kurs)
	gebuehr := round2(brutto * gebuehrProzent / 100)
	return ForeignConversion{
		BruttoEUR:  brutto,
		NettoEUR:   netto,
		GebuehrEUR: gebuehr,
		GesamtEUR:  round2(brutto + gebuehr),
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestConvertForeignPayment && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/waehrung.go internal/core/waehrung_test.go
git commit -m "Add ConvertForeignPayment: rate-based EUR conversion + CC fee"
```

---

### Task 3: Currency list (core)

**Files:**
- Create: `internal/core/currencies.go`
- Test: `internal/core/currencies_test.go`

**Interfaces:**
- Produces: `type Currency struct { Code, Name string }`;
  `var ISOCurrencies []Currency` (the active ISO 4217 currencies, ~150, English names);
  `CurrencyOptions() []string` — "CODE — Name" strings with EUR, USD, CAD, AUD first (in that order), then the remaining currencies alphabetical by code;
  `CurrencyOptionForCode(code string) string` — the option string for a code (or the bare code if unknown/empty);
  `CurrencyCodeFromOption(opt string) string` — the 3-letter code parsed from an option (text before " — ", trimmed; or the whole trimmed string if no separator).

- [ ] **Step 1: Write the failing test**

```go
package core

import "testing"

func TestCurrencyOptions(t *testing.T) {
	opts := CurrencyOptions()
	if len(opts) < 140 {
		t.Fatalf("want >=140 currencies, got %d", len(opts))
	}
	// EUR, USD, CAD, AUD must be the first four, in that order.
	wantTop := []string{"EUR", "USD", "CAD", "AUD"}
	for i, code := range wantTop {
		if CurrencyCodeFromOption(opts[i]) != code {
			t.Errorf("opts[%d] code = %q, want %q", i, CurrencyCodeFromOption(opts[i]), code)
		}
	}
	// round-trip
	if CurrencyCodeFromOption(CurrencyOptionForCode("USD")) != "USD" {
		t.Error("USD round-trip failed")
	}
	// unknown code passes through
	if CurrencyCodeFromOption(CurrencyOptionForCode("XXX")) != "XXX" {
		t.Error("unknown code should pass through")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestCurrencyOptions`
Expected: FAIL (undefined CurrencyOptions).

- [ ] **Step 3: Implement**

Create `internal/core/currencies.go`. Define `Currency`, then populate `ISOCurrencies` with the active ISO 4217 currencies — the 3-letter code + the English currency name — for the circulating national currencies of the world (target ~150 entries). It MUST include at least: EUR, USD, CAD, AUD, GBP, CHF, JPY, CNY, SEK, NOK, DKK, PLN, CZK, HUF, RON, BGN, TRY, RUB, INR, BRL, MXN, ZAR, SGD, HKD, NZD, KRW, THB, IDR, MYR, PHP, AED, SAR, ILS, plus the broad long tail (AFN, ALL, AMD, ANG, AOA, ARS, AWG, AZN, BAM, BBD, … through to ZMW). Accuracy of the 3-letter codes matters; aim for exhaustive but best-effort. Then:

```go
// Currency is an ISO 4217 currency: 3-letter code + English name.
type Currency struct {
	Code string
	Name string
}

// topCurrencies are shown first in dropdowns, in this order.
var topCurrencies = []string{"EUR", "USD", "CAD", "AUD"}

// CurrencyOptions returns "CODE — Name" strings: the top currencies first,
// then the rest sorted by code.
func CurrencyOptions() []string {
	byCode := map[string]Currency{}
	for _, c := range ISOCurrencies {
		byCode[c.Code] = c
	}
	isTop := map[string]bool{}
	var opts []string
	for _, code := range topCurrencies {
		if c, ok := byCode[code]; ok {
			opts = append(opts, c.Code+" — "+c.Name)
			isTop[code] = true
		}
	}
	rest := make([]Currency, 0, len(ISOCurrencies))
	for _, c := range ISOCurrencies {
		if !isTop[c.Code] {
			rest = append(rest, c)
		}
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].Code < rest[j].Code })
	for _, c := range rest {
		opts = append(opts, c.Code+" — "+c.Name)
	}
	return opts
}

// CurrencyOptionForCode returns the option string for a code (bare code if unknown).
func CurrencyOptionForCode(code string) string {
	for _, c := range ISOCurrencies {
		if c.Code == code {
			return c.Code + " — " + c.Name
		}
	}
	return code
}

// CurrencyCodeFromOption parses the 3-letter code from a "CODE — Name" option.
func CurrencyCodeFromOption(opt string) string {
	if i := strings.Index(opt, " — "); i >= 0 {
		return strings.TrimSpace(opt[:i])
	}
	return strings.TrimSpace(opt)
}
```

(Imports: `sort`, `strings`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestCurrencyOptions && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/currencies.go internal/core/currencies_test.go
git commit -m "Add ISO 4217 currency list with EUR/USD/CAD/AUD first"
```

---

### Task 4: Kurs + Gebühr-% + currency dropdown in the confirmation modal

**Files:**
- Modify: `internal/ui/invoicemodal.go`
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `core.ConvertForeignPayment`, `core.CurrencyOptions`/`core.CurrencyOptionForCode`/`core.CurrencyCodeFromOption`, the existing `currencySelect`/`netEUREntry`/`feeEntry`/`currencyConversionContainer`/`updateCurrencyConversionVisibility`, the gross + net entry widgets, `parseFloat`/`parseDecimal`.
- Produces: the currencySelect populated from the full ISO list; a Kurs entry + Gebühr-% entry in the conversion section; live recompute of net-EUR + fee + a "Gesamt (EUR)" display; `Waehrung` (code) / `Wechselkurs` / `GebuehrProzent` stored on save.

- [ ] **Step 0: Full currency dropdown**

The modal currently builds `currencyOptions := []string{"EUR", "USD"}` and `currencySelect` (~line 183-188), and saves `Waehrung: currencySelect.Selected` (~line 327). Replace:
- options → `core.CurrencyOptions()` (full ISO list, EUR/USD/CAD/AUD first);
- preselect → `currencySelect.SetSelected(core.CurrencyOptionForCode(meta.Waehrung))` (if `meta.Waehrung==""` use `a.settings.CurrencyDefault`);
- on save → `Waehrung: core.CurrencyCodeFromOption(currencySelect.Selected)`;
- the currency-conversion visibility check `currencySelect.Selected != a.settings.CurrencyDefault` must compare CODES — use `core.CurrencyCodeFromOption(currencySelect.Selected) != a.settings.CurrencyDefault`.

- [ ] **Step 1: Add the entries + recompute**

Near `netEUREntry`/`feeEntry` (~line 262-272), add:

```go
	kursEntry := widget.NewEntry()
	kursEntry.SetPlaceHolder(a.bundle.T("field.rate"))
	if meta.Wechselkurs > 0 {
		kursEntry.SetText(strings.Replace(fmt.Sprintf("%g", meta.Wechselkurs), ".", ",", 1))
	}
	feePctEntry := widget.NewEntry()
	feePctEntry.SetPlaceHolder(a.bundle.T("field.fee.percent"))
	if meta.GebuehrProzent > 0 {
		feePctEntry.SetText(strings.Replace(fmt.Sprintf("%g", meta.GebuehrProzent), ".", ",", 1))
	}
	gesamtEURLabel := widget.NewLabel("")
```

Find the gross + net entry widgets in this modal (search for the field bound to `Bruttobetrag` and `BetragNetto` — likely `grossEntry`/`bruttoEntry` and `netEntry`/`nettoEntry`). Add a recompute closure that runs on change of kurs / feePct / gross / net:

```go
	recomputeEUR := func() {
		kurs := parseDecimal(kursEntry.Text)
		pct := parseDecimal(feePctEntry.Text)
		brutto := parseDecimal(<grossEntry>.Text)
		netto := parseDecimal(<netEntry>.Text)
		c := core.ConvertForeignPayment(brutto, netto, kurs, pct)
		if kurs > 0 {
			netEUREntry.SetText(fmt.Sprintf("%.2f", c.NettoEUR))
			if pct > 0 {
				feeEntry.SetText(fmt.Sprintf("%.2f", c.GebuehrEUR))
			}
			gesamtEURLabel.SetText(a.bundle.T("field.total.eur", fmt.Sprintf("%.2f", c.GesamtEUR)))
		}
	}
	kursEntry.OnChanged = func(string) { recomputeEUR() }
	feePctEntry.OnChanged = func(string) { recomputeEUR() }
```

Append `recomputeEUR()` to the existing `OnChanged` of the gross and net entries (wrap, don't replace).

- [ ] **Step 2: Show the new fields in the conversion form**

In `updateCurrencyConversionVisibility`, add the Kurs + Gebühr-% rows (and the Gesamt label) to the form ABOVE the existing net-EUR + fee rows:

```go
		currencyConversionContainer.Objects = []fyne.CanvasObject{
			widget.NewForm(
				widget.NewFormItem(a.bundle.T("field.rate"), kursEntry),
				widget.NewFormItem(a.bundle.T("field.fee.percent"), feePctEntry),
				widget.NewFormItem(netEURLabel, netEUREntry),
				widget.NewFormItem(feeLabel, feeEntry),
				widget.NewFormItem(a.bundle.T("field.total.eur.label"), gesamtEURLabel),
			),
		}
```

Call `recomputeEUR()` once when the section becomes visible.

- [ ] **Step 3: Persist on save**

In the save handler where the `core.CSVRow`/`Meta` is built (search `BetragNetto_EUR: netEUR`, ~line 758), set `Wechselkurs: parseDecimal(kursEntry.Text)` and `GebuehrProzent: parseDecimal(feePctEntry.Text)`. Keep `BetragNetto_EUR` and `Gebuehr` read from their entries as before (they're now auto-filled).

Add i18n keys (both JSONs): `field.rate` (de "Wechselkurs (Fremdwährung pro EUR)"/en "Exchange rate (foreign per EUR)"), `field.fee.percent` (de "Gebühr %"/en "Fee %"), `field.total.eur.label` (de "Gesamt (EUR)"/en "Total (EUR)"), `field.total.eur` (de "%s €"/en "%s €").

- [ ] **Step 4: Build + vet + test + manual smoke**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs. Smoke: USD + 89,18 brutto + Kurs 1,1583 + 2 % → net-EUR/fee/total reflect 76,99 / 1,54 / 78,53.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/invoicemodal.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Confirmation modal: exchange rate + CC fee% with live EUR conversion"
```

---

### Task 5: Kurs + Gebühr-% + currency dropdown in the edit dialog

**Files:**
- Modify: `internal/ui/tableedit.go`
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: the same helpers (`core.ConvertForeignPayment`, `core.CurrencyOptions`, `core.CurrencyCodeFromOption`); the edit dialog's `currencySelect`/`netEUREntry`/`feeEntry`/conversion-visibility + gross/net entries.
- Produces: the same full currency dropdown + Kurs + Gebühr-% live conversion, seeded from `meta`, persisted via `updateInvoice`.

- [ ] **Step 1: Mirror Task 4 in tableedit.go**

`internal/ui/tableedit.go` has the same currency section (`currencySelect` at ~line 117, `netEUREntry`/`feeEntry` at ~191-199). Apply the SAME additions as Task 4: (a) the full currency dropdown — `currencySelect := widget.NewSelect(core.CurrencyOptions(), nil)`, preselect via `core.CurrencyOptionForCode(meta.Waehrung)` (fall back to the default currency), and on save store `core.CurrencyCodeFromOption(currencySelect.Selected)` as `Waehrung`; (b) `kursEntry` + `feePctEntry` + `gesamtEURLabel` seeded from `meta.Wechselkurs`/`meta.GebuehrProzent`; the `recomputeEUR` closure wired to kurs/feePct/gross/net changes; the conversion-visibility form gains the Kurs/%/Gesamt rows; the save (`updateInvoice` CSVRow build) sets `Wechselkurs`/`GebuehrProzent`. Reuse the i18n keys from Task 4 (do NOT re-add). Match how this dialog currently sets `Waehrung` from `currencySelect.Selected` — replace it with the code-extraction.

- [ ] **Step 2: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/tableedit.go
git commit -m "Edit dialog: exchange rate + CC fee% with live EUR conversion"
```

---

## Self-Review

- **Spec coverage:** manual exchange rate + CC fee% with auto EUR conversion (Tasks 2/4/5), persisted (Task 1); full ISO 4217 currency dropdown with EUR/USD/CAD/AUD first (Task 3 + the dropdown step in Tasks 4/5). Matches the worked example (89,18 USD ÷ 1,1583 = 76,99 EUR; 2 % = 1,54; total 78,53).
- **Placeholder scan:** Tasks 1/2/3 fully coded (Task 3's ISO data is generated by the implementer to ~150 entries); UI tasks reference concrete anchors (the existing conversion widgets, currencySelect, the save CSVRow build) with explicit "find the gross/net entry" steps.
- **Type consistency:** `Meta/CSVRow.Wechselkurs`+`GebuehrProzent`, `ConvertForeignPayment(brutto,netto,kurs,pct)ForeignConversion{BruttoEUR,NettoEUR,GebuehrEUR,GesamtEUR}`, `Currency{Code,Name}`/`CurrencyOptions`/`CurrencyOptionForCode`/`CurrencyCodeFromOption` — consistent across tasks. Both dialogs store `Waehrung` as the bare code via `CurrencyCodeFromOption`.
- **Data integrity:** conversion direction EUR=Fremd÷Kurs and fee=BruttoEUR×%/100 are unit-tested against the example; kurs≤0 guarded; column counts aligned (25→27); fee stays user-overridable; the currency code (not the display label) is persisted so existing EUR/USD rows still match.
- **Out of scope:** booking the invoice in EUR (the engine still posts the invoice-currency amounts) is a later concern; this phase covers entry + storage + EUR conversion + the currency list as requested.
