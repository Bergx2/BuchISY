# KI-Gegenkonto-Vorschlag für unbekannte Lieferanten (Phase E9) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a receipt's supplier is NOT already remembered, propose 1–3 fitting SKR04 Gegenkonten — Claude picks from the profile's own chart based on the invoice — pre-fill the top one, and show all as clickable chips in the confirmation modal.

**Architecture:** The Claude extractor is given the profile's chart and asked to also return `gegenkonto_vorschlaege` (1–3 account numbers from that chart). `Meta` gains a transient `KontoVorschlaege []int` (not persisted). On upload, a KNOWN supplier still uses its remembered account (unchanged); an UNKNOWN supplier uses `KontoVorschlaege[0]` as the pre-fill instead of the bare default. The modal shows the suggestions as chips that set the Gegenkonto on click.

**Tech Stack:** Go 1.25, Fyne v2, Anthropic Claude. Reuses the existing extractor, `CompanyAccountMap` (remembered accounts), `a.chart`, the account-picker UI.

## Global Constraints

- KNOWN-supplier behaviour is UNCHANGED: `SuggestAccountForCompany` (remembered Firma→Konto) still wins. Suggestions are only used when the supplier is unknown.
- Claude must suggest ONLY account numbers that exist in the passed chart (the prompt lists them); the app does not invent accounts. If Claude returns none / unknown numbers, fall back to the configured `DefaultAccount`.
- `Meta.KontoVorschlaege []int` is TRANSIENT — used only for the dialog; NOT added to CSV/DB/conversions.
- Suggestions cost a few hundred tokens per extraction (the chart list) — acceptable; only the Claude extraction path produces them (local mode yields none → default account, as today).
- All user-facing strings via `a.bundle.T(...)` in both JSONs (valid JSON). `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: Extractor proposes Gegenkonten

**Files:**
- Modify: `internal/core/types.go` (`Meta.KontoVorschlaege`)
- Modify: `internal/anthropic/extractor.go` (account-hint prompt + parse)
- Test: `internal/anthropic/extractor_test.go`

**Interfaces:**
- Produces: `Meta.KontoVorschlaege []int` (transient, no json/CSV/DB); `(e *Extractor) SetAccountHints(accounts []core.SKRAccount)`; the extraction prompt gains a chart list + asks for `gegenkonto_vorschlaege`; `parseExtractionJSON` fills `Meta.KontoVorschlaege` from `gegenkonto_vorschlaege`.

- [ ] **Step 1: Write the failing test**

```go
func TestParseExtractionAccountSuggestions(t *testing.T) {
	resp := `{"auftraggeber":"AWS","verwendungszweck":"Hosting","bruttobetrag":119,"gegenkonto_vorschlaege":[6837,6800,27]}`
	meta, err := parseExtractionJSON(resp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.KontoVorschlaege) != 3 || meta.KontoVorschlaege[0] != 6837 {
		t.Fatalf("KontoVorschlaege = %v", meta.KontoVorschlaege)
	}
	// absent field → empty (no crash)
	m2, _ := parseExtractionJSON(`{"auftraggeber":"X"}`, nil)
	if len(m2.KontoVorschlaege) != 0 {
		t.Errorf("expected no suggestions, got %v", m2.KontoVorschlaege)
	}
}

func TestAccountHintSectionListsAccounts(t *testing.T) {
	e := NewExtractor(nil, false)
	e.SetAccountHints([]core.SKRAccount{{Number: 6837, Name: "Fremdleistungen"}, {Number: 6815, Name: "Bürobedarf"}})
	s := e.accountHintSection()
	if !strings.Contains(s, "6837") || !strings.Contains(s, "Fremdleistungen") || !strings.Contains(s, "gegenkonto_vorschlaege") {
		t.Errorf("account hint section missing content:\n%s", s)
	}
	// no hints → empty section (no token cost)
	if NewExtractor(nil, false).accountHintSection() != "" {
		t.Error("no hints should yield empty section")
	}
}
```

(`NewExtractor(nil, false)` — confirm it tolerates a nil logger; if not, pass a discard logger the test package already uses. `strings` import in the test.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/anthropic/ -run 'TestParseExtractionAccountSuggestions|TestAccountHintSection'`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `internal/core/types.go`, add to `Meta` (after `Gegenkonto`):

```go
	KontoVorschlaege []int // transient: AI-suggested Gegenkonten for unknown suppliers (not persisted)
```

In `internal/anthropic/extractor.go`:
- Add a field to `Extractor`: `accountHints []core.SKRAccount`, and a setter:

```go
// SetAccountHints provides the profile's chart so the extractor can propose
// fitting Gegenkonten for unknown suppliers.
func (e *Extractor) SetAccountHints(accounts []core.SKRAccount) {
	e.accountHints = accounts
}

// accountHintSection renders the chart + the suggestion instruction appended to
// the extraction prompt. Empty when no hints are set.
func (e *Extractor) accountHintSection() string {
	if len(e.accountHints) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n=== VERFÜGBARE GEGENKONTEN (SKR04) ===\n")
	b.WriteString("Schlage zusätzlich 1–3 am besten passende Gegenkonten für diese Rechnung vor. Verwende AUSSCHLIESSLICH Kontonummern aus der folgenden Liste und gib sie als Feld \"gegenkonto_vorschlaege\" (JSON-Array von Zahlen, beste zuerst) zurück. Wenn keines passt, ein leeres Array.\n")
	for _, a := range e.accountHints {
		b.WriteString(fmt.Sprintf("%d %s\n", a.Number, a.Name))
	}
	return b.String()
}
```

- In `Extract`, `ExtractFromImage`, `ExtractMultimodal`, change `prompt := systemPromptFor(ownVATIDs)` to `prompt := systemPromptFor(ownVATIDs) + e.accountHintSection()`.
- In `parseExtractionJSON`, add to the anonymous result struct: `GegenkontoVorschlaege []int \`json:"gegenkonto_vorschlaege"\`` and, in the Meta conversion, `meta.KontoVorschlaege = result.GegenkontoVorschlaege`.

(Confirm `core` and `strings` and `fmt` are imported in extractor.go — they are.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/anthropic/ && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/types.go internal/anthropic/extractor.go internal/anthropic/extractor_test.go
git commit -m "Extractor proposes Gegenkonten from the chart (gegenkonto_vorschlaege)"
```

---

### Task 2: Use suggestions for unknown suppliers (app wiring)

**Files:**
- Modify: `internal/ui/app.go` (set hints + use suggestion when supplier unknown)
- Test: none (wiring; covered by build). 

**Interfaces:**
- Consumes: `a.anthropicExtractor.SetAccountHints`, `a.chart.All()`, `meta.KontoVorschlaege`, the existing `SuggestAccountForCompany` branches.
- Produces: the extractor gets the chart at startup; for an unknown supplier the pre-filled `meta.Gegenkonto` is `KontoVorschlaege[0]` (when present) instead of the bare default.

- [ ] **Step 1: Give the extractor the chart**

In `startProfile`, AFTER both `a.anthropicExtractor` and `a.chart` are assigned, add:

```go
	a.anthropicExtractor.SetAccountHints(a.chart.All())
```

- [ ] **Step 2: Use the suggestion for unknown suppliers**

There are three account-preselect blocks in `app.go` (~lines 1021, 1110, 1167) of the form:

```go
	if a.settings.AutoSelectAccount && meta.Auftraggeber != "" {
		if account, ok := core.SuggestAccountForCompany(a.companyMap, meta.Auftraggeber, a.settings.DefaultAccount); ok {
			meta.Gegenkonto = account
		} else {
			meta.Gegenkonto = a.settings.DefaultAccount
		}
	} else {
		meta.Gegenkonto = a.settings.DefaultAccount
	}
```

In EACH, replace the inner `else { meta.Gegenkonto = a.settings.DefaultAccount }` (the unknown-supplier branch) with a suggestion-aware version:

```go
		} else if len(meta.KontoVorschlaege) > 0 {
			meta.Gegenkonto = meta.KontoVorschlaege[0]
		} else {
			meta.Gegenkonto = a.settings.DefaultAccount
		}
```

Leave the outer `else` (AutoSelectAccount off or no Auftraggeber) — but optionally let it also use a suggestion: replace `else { meta.Gegenkonto = a.settings.DefaultAccount }` with the same `else if len(meta.KontoVorschlaege) > 0 { ... } else { default }`. (Apply the suggestion fallback in BOTH the inner unknown branch and the outer branch, so a suggestion is used whenever there is no remembered account.) `meta.KontoVorschlaege` already rides along on `meta` to `showConfirmationModal`.

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/app.go
git commit -m "Pre-fill Gegenkonto from AI suggestion for unknown suppliers"
```

---

### Task 3: Suggestion chips in the confirmation modal

**Files:**
- Modify: `internal/ui/invoicemodal.go`
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `meta.KontoVorschlaege`, `a.chart`, `accountLabel`, `selectedAccount`/`updateAccountDisplay`/`recomputeBooking`.
- Produces: a "Vorschläge" row of clickable chips (shown only when `meta.KontoVorschlaege` is non-empty) that set the Gegenkonto on click.

- [ ] **Step 1: Build the chips row**

In `showConfirmationModal`, after `selectedAccount`, `updateAccountDisplay`, and `chooseAccountBtn` exist (and `recomputeBooking` is forward-declared), build a suggestions container shown only when there are suggestions:

```go
	suggestionBox := container.NewHBox()
	if len(meta.KontoVorschlaege) > 0 {
		suggestionBox.Add(widget.NewLabel(a.bundle.T("field.suggestions")))
		for _, k := range meta.KontoVorschlaege {
			k := k
			label := fmt.Sprintf("%d", k)
			if acc, ok := a.chart.Find(k); ok {
				label = accountLabel(acc)
			}
			btn := widget.NewButton(label, func() {
				selectedAccount = k
				updateAccountDisplay()
				if recomputeBooking != nil {
					recomputeBooking()
				}
			})
			btn.Importance = widget.LowImportance
			suggestionBox.Add(btn)
		}
	}
```

Place `suggestionBox` in the layout directly below the Gegenkonto row (the row with `accountDisplay` + `chooseAccountBtn`). It is empty (renders nothing) when there are no suggestions.

Add i18n key `field.suggestions` (de "Vorschläge:" / en "Suggestions:") to both JSONs.

- [ ] **Step 2: Build + vet + test + manual smoke**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs. Smoke: a new supplier (not in company map) shows the chips; clicking one sets the Gegenkonto + updates the booking preview.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/invoicemodal.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Show AI Gegenkonto suggestions as clickable chips in the modal"
```

---

## Self-Review

- **Spec coverage:** known supplier → remembered account (unchanged); unknown supplier → Claude suggests 1–3 chart accounts (Task 1), top one pre-filled (Task 2), all shown as clickable chips (Task 3). Matches the request.
- **Placeholder scan:** Task 1 fully coded + tested; Tasks 2/3 reference the concrete app.go preselect blocks and the modal account row with full snippets.
- **Type consistency:** `Meta.KontoVorschlaege []int` (transient), `Extractor.SetAccountHints([]core.SKRAccount)` + `accountHintSection()`, `parseExtractionJSON` fills the field. Consistent.
- **Data integrity:** suggestions are chart-only (prompt restricts to listed numbers; unknown numbers just won't match the chart on display but are harmless); remembered-account behaviour untouched; `KontoVorschlaege` never persisted; local-mode extraction yields no suggestions → default account (unchanged).
- **Out of scope:** persisting suggestions, suggesting the booking category (the company template already learns it), suggestions in the edit dialog (a re-opened invoice already has its account).
