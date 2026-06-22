# Matching-Verbesserungen v2 (Phase E12) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Substantially improve invoice↔statement matching: respect debit/credit direction, claim each statement line once, tolerate foreign-currency rounding, learn supplier→statement-text aliases, make the date window/tolerance configurable, show richer suggestions with a "choose another line" option, allow manual unlink, use Claude for cryptic texts, and support grouped (n:1) / partial (1:n) payments.

**Architecture:** The pure matcher (`core/belegabgleich.go`) gains a `MatchConfig` (tolerances, date window, learned aliases) and a credit-line exclusion; `StatementBooking` gains a parsed credit flag. The Belegabgleich dialog (`ui/belegabgleichview.go`) is refactored to parse statements once, claim lines greedily by confidence, show richer rows with a candidate dropdown, and record confirmed aliases. New: `StatementAliasStore` (per-profile learned aliases), a Claude line-ranker, grouped-payment detection, and a manual unlink action.

**Tech Stack:** Go 1.25, Fyne v2, Anthropic Claude. Reuses E10/E11 matcher + dialog, `BuchungRef`, the per-profile JSON store pattern (`companymap.go`), the Extractor.

## Global Constraints

- The matcher stays a PURE function (no I/O); all config/aliases are passed in via `MatchConfig`.
- Amount stays a hard filter, but the tolerance is `max(0.01, amount * ForeignTolerancePct/100)` for foreign-currency invoices (EUR stays 0.01).
- Each statement line is linked to AT MOST ONE invoice (claim-once); auto-link resolves conflicts greedily by descending match score.
- Auto-link is only for unambiguous matches (one amount+direction+window candidate, not contested by a higher-scoring invoice). Everything uncertain is a suggestion the user confirms.
- Credit (Haben/Gutschrift) lines are excluded when matching an expense invoice, but ONLY when the credit signal is clear — ambiguous lines are treated as debits (never silently drop a real match).
- Learned aliases and config are per-profile; defaults: `DateWindowDays=5`, `ForeignTolerancePct=1.5`.
- All user-facing strings via `a.bundle.T(...)` in both JSONs (valid JSON). `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: Debit/credit direction on statement lines

**Files:**
- Modify: `internal/core/booking.go` (`StatementBooking.IstGutschrift`)
- Modify: `internal/core/statement_bookings.go` (`ParseLineIsCredit` + wire it)
- Test: `internal/core/statement_bookings_test.go`

**Interfaces:**
- Produces: `StatementBooking.IstGutschrift bool` (json `gutschrift,omitempty`); `ParseLineIsCredit(text string) bool` — true ONLY for clear credit signals.

- [ ] **Step 1: Write the failing test**

```go
func TestParseLineIsCredit(t *testing.T) {
	credits := []string{
		"05.01. Gutschrift Kunde 2.000,00 H",
		"03.01. Zahlungseingang Müller 500,00",
		"07.01. SEPA-Gutschrift 80,00 +",
	}
	debits := []string{
		"14.01. AMAZON WEB SERVICES 78,53",
		"03.01. Lastschrift Telekom -49,99",
		"02.01. Kartenzahlung REWE 23,40",
	}
	for _, c := range credits {
		if !ParseLineIsCredit(c) {
			t.Errorf("expected credit: %q", c)
		}
	}
	for _, d := range debits {
		if ParseLineIsCredit(d) {
			t.Errorf("expected debit: %q", d)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestParseLineIsCredit`
Expected: FAIL (undefined).

- [ ] **Step 3: Implement**

In `internal/core/booking.go`, add to `StatementBooking` (after `Betrag`):

```go
	IstGutschrift bool `json:"gutschrift,omitempty"` // clearly an incoming credit (Haben)
```

In `internal/core/statement_bookings.go`, add the parser and set it where the booking is built (`Betrag: ParseLineAmount(ln.text)` already there → add `IstGutschrift: ParseLineIsCredit(ln.text)`):

```go
// creditKeywordRe matches clear credit (Haben / incoming) signals on a line.
var creditKeywordRe = regexp.MustCompile(`(?i)gutschrift|zahlungseingang|geldeingang|überweisungseingang|lohn|gehalt`)

// trailingCreditRe matches an amount followed by a credit marker (" H" or "+").
var trailingCreditRe = regexp.MustCompile(`\d,\d{2}\s*([H+])\b|\d,\d{2}\s*\+`)

// ParseLineIsCredit reports whether a statement line is CLEARLY an incoming
// credit (Haben). Ambiguous lines return false (treated as a debit) so a real
// expense match is never silently dropped.
func ParseLineIsCredit(text string) bool {
	if creditKeywordRe.MatchString(text) {
		return true
	}
	if trailingCreditRe.MatchString(text) {
		return true
	}
	return false
}
```

(Do NOT treat a leading "-" as anything — that's a debit. `regexp`/`strings` already imported.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run 'TestParseLineIsCredit|TestParseLineAmount' && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/booking.go internal/core/statement_bookings.go internal/core/statement_bookings_test.go
git commit -m "Parse debit/credit direction on statement lines"
```

---

### Task 2: MatchConfig — foreign tolerance, date window, alias hook, credit exclusion

**Files:**
- Modify: `internal/core/belegabgleich.go`
- Modify: `internal/ui/belegabgleichview.go` (pass a config to the existing call — minimal, keeps it compiling)
- Test: `internal/core/belegabgleich_test.go`

**Interfaces:**
- Produces: `type MatchConfig struct { DateWindowDays int; ForeignTolerancePct float64; Aliases map[string][]string }`; `DefaultMatchConfig() MatchConfig`; `MatchInvoiceToStatement(row CSVRow, lines []StatementBooking, cfg MatchConfig) (MatchKind, []ScoredLine)` (new third param). Credit lines excluded; foreign amount tolerance; configurable auto window; alias-boosted name score.

- [ ] **Step 1: Write the failing test**

```go
func TestMatchConfigForeignToleranceAndCredit(t *testing.T) {
	cfg := DefaultMatchConfig()
	lines := []StatementBooking{
		{Page: 0, LineIdx: 1, Date: "14.01.2026", Text: "VISA AWS 78,90", Betrag: 78.90},
		{Page: 0, LineIdx: 2, Date: "14.01.2026", Text: "Gutschrift 78,53 H", Betrag: 78.53, IstGutschrift: true},
	}
	// Foreign invoice EUR amount 78.53; bank debited 78.90 (rate drift). Within 1.5% → matches line 1, NOT the credit line.
	fx := CSVRow{Auftraggeber: "AWS", Bezahldatum: "14.01.2026", Bruttobetrag: 89.18, Waehrung: "USD", Wechselkurs: 1.1583, Gebuehr: 1.54}
	kind, cands := MatchInvoiceToStatement(fx, lines, cfg)
	if kind == MatchNone || len(cands) == 0 || cands[0].Line.LineIdx != 1 {
		t.Fatalf("foreign tolerance: kind=%v cands=%+v", kind, cands)
	}
	for _, c := range cands {
		if c.Line.IstGutschrift {
			t.Errorf("credit line must be excluded: %+v", c)
		}
	}
	// EUR invoice keeps the strict 0.01 filter: 78.90 line must NOT match an 78.53 EUR invoice.
	eur := CSVRow{Auftraggeber: "X", Bezahldatum: "14.01.2026", Bruttobetrag: 78.53, Waehrung: "EUR"}
	if k, _ := MatchInvoiceToStatement(eur, lines, cfg); k != MatchNone {
		t.Errorf("EUR strict tolerance broken: %v", k)
	}
	// Alias boost: a learned token lets a no-shared-word supplier rank.
	cfg.Aliases = map[string][]string{"aws": {"amazon"}}
	al := []StatementBooking{{Page: 0, LineIdx: 1, Date: "14.01.2026", Text: "AMAZON WEB SERV 78,53", Betrag: 78.53}}
	if k, c := MatchInvoiceToStatement(CSVRow{Auftraggeber: "AWS", Bezahldatum: "14.01.2026", Bruttobetrag: 78.53, Waehrung: "EUR"}, al, cfg); k == MatchNone || len(c) == 0 {
		t.Errorf("alias match failed: %v", k)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestMatchConfig`
Expected: FAIL (signature/behaviour).

- [ ] **Step 3: Implement**

Rewrite the relevant parts of `internal/core/belegabgleich.go`:

```go
// MatchConfig tunes the matcher.
type MatchConfig struct {
	DateWindowDays      int                 // auto-link only within this many days
	ForeignTolerancePct float64             // amount tolerance for non-EUR invoices (percent)
	Aliases             map[string][]string // lowercase supplier → learned statement tokens
}

// DefaultMatchConfig returns sensible defaults.
func DefaultMatchConfig() MatchConfig {
	return MatchConfig{DateWindowDays: 5, ForeignTolerancePct: 1.5}
}

func MatchInvoiceToStatement(row CSVRow, lines []StatementBooking, cfg MatchConfig) (MatchKind, []ScoredLine) {
	amount := InvoiceEURAmount(row)
	if amount <= 0 {
		return MatchNone, nil
	}
	// Amount tolerance: strict for EUR; percentage band for foreign (rate drift).
	tol := 0.01
	if row.Waehrung != "" && row.Waehrung != "EUR" && cfg.ForeignTolerancePct > 0 {
		if band := amount * cfg.ForeignTolerancePct / 100; band > tol {
			tol = band
		}
	}
	invDate := row.Bezahldatum
	if invDate == "" {
		invDate = row.Rechnungsdatum
	}
	nameTokens := tokenize(row.Auftraggeber)
	aliasTokens := cfg.Aliases[strings.ToLower(strings.TrimSpace(row.Auftraggeber))]
	window := cfg.DateWindowDays
	if window <= 0 {
		window = 5
	}

	var cands []ScoredLine
	for _, l := range lines {
		if l.IstGutschrift { // never match an expense to an incoming credit
			continue
		}
		if absf(l.Betrag-amount) > tol {
			continue
		}
		days := dayDistance(invDate, l.Date)
		dateScore := 1.0 / (1.0 + float64(days))
		lineTokens := tokenize(l.Text)
		nameScore := tokenOverlap(nameTokens, lineTokens)
		if a := tokenOverlap(aliasTokens, lineTokens); a > nameScore {
			nameScore = a // learned alias can rescue a no-shared-word supplier
		}
		cands = append(cands, ScoredLine{Line: l, Score: dateScore*2 + nameScore})
	}
	if len(cands) == 0 {
		return MatchNone, nil
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })

	if len(cands) == 1 && dayDistance(invDate, cands[0].Line.Date) <= window {
		return MatchAuto, cands
	}
	return MatchSuggest, cands
}
```

Update the ONE caller in `internal/ui/belegabgleichview.go` (`core.MatchInvoiceToStatement(row, lines)` → `core.MatchInvoiceToStatement(row, lines, core.DefaultMatchConfig())`) so it compiles. (Task 3 swaps in the configured value; Task 6 supplies aliases — for now `DefaultMatchConfig()`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ && go build ./...`
Expected: PASS (the new test + the existing `TestMatchInvoiceToStatement` — NOTE: the existing test calls the 2-arg form; update its calls to pass `DefaultMatchConfig()` as the third arg).

- [ ] **Step 5: Commit**

```bash
git add internal/core/belegabgleich.go internal/core/belegabgleich_test.go internal/ui/belegabgleichview.go
git commit -m "MatchConfig: foreign tolerance, date window, alias boost, credit exclusion"
```

---

### Task 3: Configurable date window + foreign tolerance (Settings)

**Files:**
- Modify: `internal/core/types.go` (`Settings` fields)
- Modify: `internal/ui/settings.go` (two inputs)
- Modify: `internal/ui/belegabgleichview.go` (build config from settings)
- Test: none (covered by build). 

**Interfaces:**
- Produces: `Settings.MatchDateWindowDays int`, `Settings.MatchForeignTolerancePct float64`; a helper `a.matchConfig() core.MatchConfig` reading them (falling back to defaults when zero).

- [ ] **Step 1: Settings fields + helper**

In `internal/core/types.go`, add to `Settings`: `MatchDateWindowDays int \`json:"matchDateWindowDays,omitempty"\`` and `MatchForeignTolerancePct float64 \`json:"matchForeignTolerancePct,omitempty"\``.

In `internal/ui/belegabgleichview.go`, add:

```go
func (a *App) matchConfig() core.MatchConfig {
	cfg := core.DefaultMatchConfig()
	if a.settings.MatchDateWindowDays > 0 {
		cfg.DateWindowDays = a.settings.MatchDateWindowDays
	}
	if a.settings.MatchForeignTolerancePct > 0 {
		cfg.ForeignTolerancePct = a.settings.MatchForeignTolerancePct
	}
	return cfg
}
```

and use `a.matchConfig()` instead of `core.DefaultMatchConfig()` at the call site.

- [ ] **Step 2: Settings UI**

In `internal/ui/settings.go`, add two `widget.NewEntry()` inputs (numeric) for the date window (days) and foreign tolerance (%), seeded from the settings, parsed back on save (use the existing numeric-parse pattern in that file; `0`/empty → leave default). Labels via i18n keys `settings.matchWindow` (de "Abgleich: Datumsfenster (Tage)"/en "Reconciliation: date window (days)") and `settings.matchTolerance` (de "Abgleich: Fremdwährungs-Toleranz (%)"/en "Reconciliation: foreign tolerance (%)"). Add the keys to both JSONs.

- [ ] **Step 3: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs.

- [ ] **Step 4: Commit**

```bash
git add internal/core/types.go internal/ui/settings.go internal/ui/belegabgleichview.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Settings: configurable reconciliation date window + foreign tolerance"
```

---

### Task 4: Dialog refactor — parse once, claim each line once, richer rows

**Files:**
- Modify: `internal/ui/belegabgleichview.go`
- Test: none (covered by build + manual). 

**Interfaces:**
- Consumes: `a.matchConfig()`, the per-account parsed lines, `core.BuchungRef`.
- Produces: each statement file parsed once; each line claimed by at most one invoice (greedy by score); suggestion rows show the line's own amount + full text.

- [ ] **Step 1: Parse-once cache + greedy claim**

Refactor `showBelegabgleich`'s matching section (the `for _, row := range rows` block, lines ~44-125):

1. Build a per-account cache once: `stmtCache := map[string][]stmtLine{}` where `type stmtLine struct { File string; Line core.StatementBooking }`. For each distinct `row.Bankkonto` (bank/creditcard only), parse each `a.listStatements(acct)` file ONCE via `core.ParseStatementBookings`, appending `stmtLine{file, line}` (skip parse errors with a log). Cache keyed by account.
2. A `claimed := map[string]bool{}` keyed by `core.BuchungRef{file,page,lineIdx}.String()`.
3. First compute, for every unlinked bank/creditcard invoice, its match against its account's lines (build a `[]stmtLine` view, call `core.MatchInvoiceToStatement(row, linesOnly, a.matchConfig())`, remember the `stmtLine` of each candidate by (file,page,lineIdx)). Collect `{row, kind, candidates []ScoredLine, fileOf map[...]string}`.
4. **Greedy auto-link by confidence:** gather all invoices whose kind is `MatchAuto`; sort them by their top candidate's `Score` descending; for each, if its line's key is not yet `claimed`, set `BuchungRef`, `dbRepo.Update`, mark claimed, `autoLinked++`; if its line is already claimed, demote it to a suggestion.
5. **Suggestions:** for the rest (and demoted autos), keep only candidates whose line key is not claimed; if none remain, skip; else add a `belegSuggestion` carrying the FULL remaining candidate list (for Task 5's dropdown) — extend `belegSuggestion` with `candidates []core.ScoredLine` and a parallel `files []string` (or a small struct pairing each candidate with its file). On confirm (existing handler) ALSO add the chosen line's key to `claimed` (so a later confirm can't reuse it within the same dialog session).

To carry the file per candidate cleanly, change `belegSuggestion` to:
```go
type belegSuggestion struct {
	row        core.CSVRow
	candidates []scoredWithFile // ranked; [0] is the default
}
type scoredWithFile struct {
	scored core.ScoredLine
	file   string
}
```

- [ ] **Step 2: Richer suggestion label**

The suggestion row label should show the invoice + the DEFAULT candidate's own amount and text, e.g.:
```
<Auftraggeber>  <invoiceEUR> €  →  S.<p> Z.<l> · <lineDate> · <lineBetrag> € · <lineText>  (<file>)
```
Format `lineBetrag` and `invoiceEUR` with the existing comma style. Truncate `lineText` to ~60 runes.

- [ ] **Step 3: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/belegabgleichview.go
git commit -m "Belegabgleich: parse once, claim each line once, richer suggestion rows"
```

---

### Task 5: Choose-another-line dropdown + manual unlink

**Files:**
- Modify: `internal/ui/belegabgleichview.go` (dropdown per suggestion)
- Modify: `internal/ui/table.go` or `internal/ui/tableedit.go`/a context action (manual unlink)
- Test: none (covered by build + manual). 

**Interfaces:**
- Consumes: `belegSuggestion.candidates`, `core.BuchungRef`, `a.dbRepo.Update`, `a.loadInvoices`.
- Produces: a `widget.Select` per suggestion to pick which candidate to confirm; a table action to clear an invoice's `BuchungRef`.

- [ ] **Step 1: Candidate dropdown in the suggestion row**

In each suggestion row, add a `widget.Select` whose options are the candidate lines (label = `S.p Z.l · date · betrag € · short text`), defaulting to index 0. Track the selected candidate index for that suggestion (a captured local). The Confirm button uses the SELECTED candidate (not always `candidates[0]`) to build the `BuchungRef`. If only one candidate, the Select may be omitted.

- [ ] **Step 2: Manual unlink action**

Add a way to clear a wrong link from the table: when a row has `BuchungRef != ""`, offer "Verknüpfung entfernen" — implement as a method `func (a *App) unlinkInvoice(row core.CSVRow)` that sets `row.BuchungRef = ""`, `a.dbRepo.Update(row.Jahr, row.Monat, row.Dateiname, row)`, then `a.loadInvoices()`. Wire it to a table interaction consistent with how the table already exposes per-row actions (search `tabledelete.go`/`tableedit.go` for the existing row-action/right-click/tap pattern and add an "unlink" entry next to them, shown only when `row.BuchungRef != ""`). Confirm with a small `dialog.NewConfirm` before clearing. i18n key `table.unlink` (de "Verknüpfung entfernen"/en "Remove link").

- [ ] **Step 3: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/belegabgleichview.go internal/ui/table.go internal/ui/tableedit.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Belegabgleich: candidate dropdown + manual unlink"
```

---

### Task 6: Learn supplier→statement-text aliases

**Files:**
- Create: `internal/core/statementalias.go`
- Modify: `internal/ui/belegabgleichview.go` (load aliases into config; record on auto-link/confirm)
- Modify: `internal/ui/app.go` (construct the store per profile)
- Test: `internal/core/statementalias_test.go`

**Interfaces:**
- Produces: `StatementAliasStore` (per-profile JSON `statement_aliases.json`): `Load() (map[string][]string, error)`, `Learn(supplier string, lineText string)` (adds distinctive tokens from the line for that supplier), `Save()`. Keys are lowercase supplier; values are deduped tokens (len ≥ 4, excluding generic ones).

- [ ] **Step 1: Write the failing test**

```go
func TestStatementAliasLearn(t *testing.T) {
	dir := t.TempDir()
	s := NewStatementAliasStore(dir)
	s.Learn("AWS", "14.01. AMAZON WEB SERVICES EMEA 78,53")
	m, _ := s.Load()
	got := m["aws"]
	found := false
	for _, tok := range got {
		if tok == "amazon" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected learned token 'amazon' for aws, got %v", got)
	}
	// Persisted across instances.
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	m2, _ := NewStatementAliasStore(dir).Load()
	if len(m2["aws"]) == 0 {
		t.Errorf("aliases not persisted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestStatementAliasLearn`
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/core/statementalias.go` modeled on `companymap.go` (same Load/Save JSON pattern, file `statement_aliases.json` in the profile config dir). `Learn(supplier, lineText)`: lowercase supplier key; `tokenize(lineText)` (reuse the matcher's tokenizer); keep tokens of len ≥ 4 that are NOT already in the supplier's name tokens and not pure digits; union into the stored set (dedupe). In-memory map mutated by `Learn`; `Save` writes JSON; `Load` reads (missing file → empty map, no error).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestStatementAlias && go build ./...`
Expected: PASS.

- [ ] **Step 5: Wire into the dialog + app**

- In `app.go` (where `companyMap`/per-profile stores are built), construct `a.statementAliases = core.NewStatementAliasStore(<profile config dir>)` and load it.
- In `belegabgleichview.go` `matchConfig()`: set `cfg.Aliases, _ = a.statementAliases.Load()` (or a cached map).
- On every successful auto-link AND every confirm, call `a.statementAliases.Learn(row.Auftraggeber, <chosen line>.Text)` then `a.statementAliases.Save()` so future runs match cryptic texts.

- [ ] **Step 6: Build + commit**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`

```bash
git add internal/core/statementalias.go internal/core/statementalias_test.go internal/ui/belegabgleichview.go internal/ui/app.go
git commit -m "Learn supplier→statement-text aliases to match cryptic lines"
```

---

### Task 7: Claude name-ranking for ambiguous suggestions

**Files:**
- Modify: `internal/anthropic/extractor.go` (or a new `matcher.go` in the package)
- Modify: `internal/ui/belegabgleichview.go`
- Test: none for the network call; keep the function small + pure-ish. 

**Interfaces:**
- Produces: `(e *Extractor) RankStatementLine(ctx context.Context, apiKey, model, supplier string, lineTexts []string) (int, error)` — returns the index of the line that best matches the supplier, or -1. Only called for ambiguous suggestions and only in Claude mode.

- [ ] **Step 1: Implement the ranker**

Add a method that sends a tiny prompt: "Welche dieser Kontoauszug-Zeilen gehört am ehesten zum Lieferanten \"<supplier>\"? Antworte nur mit der Nummer (0-basiert) oder -1." + the numbered `lineTexts`. Reuse the package's existing HTTP `client` and JSON-extraction helpers. Parse the integer reply; clamp to `[-1, len-1)`; on any error return `(-1, err)`.

- [ ] **Step 2: Use it for ambiguous suggestions**

In `showBelegabgleich`, after the heuristic produces a `MatchSuggest` with ≥2 candidates whose top two scores are close (e.g. within 0.3) AND the processing mode is Claude (`a.settings.UseClaude`/equivalent — match the existing setting name) AND an API key is available: call `RankStatementLine(supplier, candidate texts)`; if it returns a valid index, reorder that candidate to the front (so the dropdown defaults to Claude's pick). Wrap in a background-safe call consistent with how the app already invokes Claude (it already runs extraction in a goroutine — but here keep it synchronous inside the dialog build, or show a brief progress state; reuse the simplest existing pattern). Errors are non-fatal (fall back to heuristic order). Add no new persisted state.

- [ ] **Step 3: Build + commit**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`

```bash
git add internal/anthropic/ internal/ui/belegabgleichview.go
git commit -m "Use Claude to rank ambiguous statement-line matches"
```

---

### Task 8: Grouped (n:1) and partial (1:n) payments

**Files:**
- Modify: `internal/core/belegabgleich.go` (group/partial detection)
- Modify: `internal/ui/belegabgleichview.go` (present them)
- Test: `internal/core/belegabgleich_test.go`

**Interfaces:**
- Produces: `GroupMatch{ Dateinamen []string; Line StatementBooking; File string }`; `FindGroupedPayments(invoices []CSVRow, lines []StatementBooking, cfg MatchConfig) []GroupMatch` — finds an UNMATCHED statement line whose amount equals the SUM of 2–3 unmatched invoices (same account already filtered by caller) within the date window; `PartialPaymentLines(row CSVRow, lines []StatementBooking) []ScoredLine` — for an invoice flagged `Teilzahlung`, statement lines whose amount is LESS than the invoice amount (partial), ranked by date.

- [ ] **Step 1: Write the failing test**

```go
func TestFindGroupedPayments(t *testing.T) {
	cfg := DefaultMatchConfig()
	invoices := []CSVRow{
		{Dateiname: "a.pdf", Auftraggeber: "X", Bezahldatum: "10.01.2026", Bruttobetrag: 30, Waehrung: "EUR"},
		{Dateiname: "b.pdf", Auftraggeber: "Y", Bezahldatum: "10.01.2026", Bruttobetrag: 70, Waehrung: "EUR"},
		{Dateiname: "c.pdf", Auftraggeber: "Z", Bezahldatum: "10.01.2026", Bruttobetrag: 999, Waehrung: "EUR"},
	}
	lines := []StatementBooking{{Page: 0, LineIdx: 1, Date: "10.01.2026", Text: "Sammelüberweisung 100,00", Betrag: 100}}
	groups := FindGroupedPayments(invoices, lines, cfg)
	if len(groups) != 1 || len(groups[0].Dateinamen) != 2 {
		t.Fatalf("expected one 2-invoice group summing to 100, got %+v", groups)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestFindGroupedPayments`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `internal/core/belegabgleich.go`:
- `FindGroupedPayments`: for each non-credit line, search subsets of size 2 and 3 among invoices whose date is within `cfg.DateWindowDays` of the line and whose `InvoiceEURAmount > 0`, whose summed `InvoiceEURAmount` equals the line `Betrag` within 0.01. Bound the search: only consider invoices within the window (small set); iterate pairs then triples; return the FIRST disjoint group per line (don't reuse an invoice across groups). Return `[]GroupMatch` with the invoice `Dateinamen`, the line, and an empty `File` (the caller fills the file).
- `PartialPaymentLines`: if `!row.Teilzahlung` return nil; else return non-credit lines with `Betrag < InvoiceEURAmount(row) - 0.01` and `Betrag > 0`, ranked by date proximity (reuse `dayDistance`).

Keep the subset search tightly bounded (windowed invoices only; sizes 2–3) so it stays linear-ish in practice.

- [ ] **Step 4: Present in the dialog**

In `showBelegabgleich`, AFTER the 1:1 pass (using the parse-once cache + still-unclaimed lines): run `core.FindGroupedPayments(unmatchedInvoices, unclaimedLines, a.matchConfig())` per account and show each group as an informational suggestion row: `"N Belege (A, B) = <line> (<file>)"`. (Confirming a group — linking all N invoices' BuchungRef to the one line — is the persistence; if that is too large here, present the group read-only with the member filenames and a note, and link the first; keep scope to DISPLAYING the detected groups + a single "alle verknüpfen" button that sets each member's BuchungRef to the shared line and Updates them.) Also, for invoices flagged `Teilzahlung` that stayed unmatched, surface `PartialPaymentLines` as suggestions labeled "Teilzahlung".

- [ ] **Step 5: Run test + build + commit**

Run: `go test ./internal/core/ -run 'TestFindGroupedPayments|TestMatch' && go build ./... && go vet ./internal/ui/ && go test ./...`

```bash
git add internal/core/belegabgleich.go internal/core/belegabgleich_test.go internal/ui/belegabgleichview.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Detect grouped (n:1) and partial (1:n) payments in reconciliation"
```

---

## Self-Review

- **Spec coverage:** direction (T1), foreign tolerance + date window + alias hook + credit exclusion (T2), settings (T3), parse-once + claim-once + richer rows (T4), candidate dropdown + manual unlink (T5), alias learning (T6), Claude ranking (T7), grouped/partial payments (T8) — the 10 requested improvements.
- **Placeholder scan:** core tasks (1,2,6,8) carry full code + tests; UI tasks reference the concrete dialog/table anchors with the exact refactor shape and the `belegSuggestion` type changes threaded through T4→T5→T8.
- **Type consistency:** `MatchConfig` threaded through `MatchInvoiceToStatement` (signature change updated at the one caller + existing test); `belegSuggestion` extended once (T4) and consumed by T5/T8; `StatementBooking.IstGutschrift` set in parsing (T1) and read in the matcher (T2).
- **Data integrity:** claim-once prevents one line linking two invoices; greedy-by-score makes auto-link deterministic; credit exclusion is conservative (clear signals only); foreign tolerance only widens for non-EUR; aliases/config per-profile; grouped-payment search is bounded (windowed, size 2–3, disjoint).
- **Out of scope:** auto-running matching on import; cross-account grouping; learning aliases from rejections; ML ranking. Claude ranking is opt-in (Claude mode only) and non-persisted.
