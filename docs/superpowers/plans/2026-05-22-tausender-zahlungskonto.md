# Tausender-Trennzeichen + Umbenennung „Zahlungskonto" — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Beträge werden mit Tausender-Trennzeichen angezeigt und in Dateinamen geschrieben; die Beschriftung „Bankkonto" heißt überall „Zahlungskonto".

**Architecture:** Ein `core.FormatAmount`-Helfer formatiert Beträge mit Dezimal- und Tausender-Trennzeichen; UI-Anzeige und Dateinamen-Vorlage nutzen ihn. Der UI-Parser `parseFloat` wird Tausender-tolerant. Die Umbenennung ändert nur Anzeigetexte (i18n-Werte).

**Tech Stack:** Go 1.25, Fyne v2.6.3, Standard-`testing`.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen. Jede Aufgabe endet mit `go build`/`go vet`/`go test`.

---

### Task 1: `core.FormatAmount` + Dateinamen-Vorlage (TDD)

**Files:**
- Create: `internal/core/format.go`
- Create: `internal/core/format_test.go`
- Modify: `internal/core/template.go`

- [ ] **Step 1: Write the failing test**

Create `internal/core/format_test.go`:

```go
package core

import "testing"

func TestFormatAmount(t *testing.T) {
	cases := []struct {
		value float64
		sep   string
		want  string
	}{
		{15000, ",", "15.000,00"},
		{1234567.5, ",", "1.234.567,50"},
		{42, ",", "42,00"},
		{999, ",", "999,00"},
		{-1234, ",", "-1.234,00"},
		{0, ",", "0,00"},
		{15000, ".", "15,000.00"},
		{1234567.5, ".", "1,234,567.50"},
	}
	for _, c := range cases {
		got := FormatAmount(c.value, c.sep)
		if got != c.want {
			t.Errorf("FormatAmount(%v, %q) = %q, want %q", c.value, c.sep, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestFormatAmount -v`
Expected: FAIL — `FormatAmount` undefined.

- [ ] **Step 3: Implement `FormatAmount`**

Create `internal/core/format.go`:

```go
package core

import (
	"fmt"
	"strings"
)

// FormatAmount formats a monetary value with two decimal places, the given
// decimal separator ("," or "."), and a thousands separator grouping the
// integer part in threes. The thousands separator is whichever of "." / ","
// is not the decimal separator. A negative sign is kept in front.
func FormatAmount(value float64, decimalSep string) string {
	thousandsSep := "."
	if decimalSep != "," {
		decimalSep = "."
		thousandsSep = ","
	}

	s := fmt.Sprintf("%.2f", value) // always "-?<digits>.dd"
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart := s[:len(s)-3]  // digits before ".dd"
	fracPart := s[len(s)-2:] // the two decimals

	var b strings.Builder
	for i, d := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteString(thousandsSep)
		}
		b.WriteRune(d)
	}

	result := b.String() + decimalSep + fracPart
	if neg {
		result = "-" + result
	}
	return result
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestFormatAmount -v`
Expected: PASS.

- [ ] **Step 5: Wire the filename template to `FormatAmount`**

In `internal/core/template.go`, the amount-formatting closure is currently:

```go
	// Format amounts according to decimal separator
	formatAmount := func(amount float64) string {
		formatted := fmt.Sprintf("%.2f", amount)
		if opts.DecimalSeparator == "," {
			formatted = strings.Replace(formatted, ".", ",", 1)
		}
		return formatted
	}
```

Replace it with:

```go
	// Format amounts with decimal + thousands separators.
	formatAmount := func(amount float64) string {
		return FormatAmount(amount, opts.DecimalSeparator)
	}
```

- [ ] **Step 6: Build, vet, test**

Run: `go build ./... && go vet ./internal/core/... && go test ./internal/core/...`
Expected: PASS. If the build reports `fmt` or `strings` as now-unused in `template.go`, remove the unused import(s) and re-run until clean.

---

### Task 2: UI-Anzeige + Tausender-toleranter Parser

**Files:**
- Modify: `internal/ui/kassenbuchview.go`
- Modify: `internal/ui/invoicemodal.go`
- Modify: `internal/ui/tableedit.go`

- [ ] **Step 1: Route `formatDecimal` through `core.FormatAmount`**

In `internal/ui/kassenbuchview.go`, `formatDecimal` is currently:

```go
func formatDecimal(v float64, sep string) string {
	s := fmt.Sprintf("%.2f", v)
	if sep == "," {
		s = strings.ReplaceAll(s, ".", ",")
	}
	return s
}
```

Replace it with:

```go
func formatDecimal(v float64, sep string) string {
	return core.FormatAmount(v, sep)
}
```

(`core` is already imported in `kassenbuchview.go`. If `fmt` or `strings` become unused there, the build in Step 4 reports it — then remove the unused import.)

- [ ] **Step 2: Make `parseFloat` thousands-tolerant**

In `internal/ui/invoicemodal.go`, `parseFloat` is currently:

```go
func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", ".")
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
```

Replace it with:

```go
// parseFloat parses a user-entered amount, tolerating thousands separators.
// The thousands separator is whichever of "." / "," is not decimalSep.
func parseFloat(s string, decimalSep string) float64 {
	s = strings.TrimSpace(s)
	if decimalSep == "," {
		s = strings.ReplaceAll(s, ".", "") // strip thousands separators
		s = strings.ReplaceAll(s, ",", ".") // decimal comma -> dot
	} else {
		s = strings.ReplaceAll(s, ",", "") // strip thousands separators
	}
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
```

- [ ] **Step 3: Update every `parseFloat` call site**

`parseFloat` now needs the decimal separator. There are 18 call sites across three files; every one is inside an `*App` method, so `a.settings.DecimalSeparator` is in scope. Add `, a.settings.DecimalSeparator` as the second argument to each call:

- `internal/ui/kassenbuchview.go` — 2 calls: `parseFloat(s)` → `parseFloat(s, a.settings.DecimalSeparator)` (both currently `parseFloat(s)`).
- `internal/ui/invoicemodal.go` — 8 calls: `parseFloat(netEntry.Text)`, `parseFloat(vatPercentEntry.Text)`, `parseFloat(vatAmountEntry.Text)`, `parseFloat(grossEntry.Text)` — each appears twice (in the `core.Meta` literal and in the `saveInvoice` argument list). Each becomes `parseFloat(<sameArg>, a.settings.DecimalSeparator)`.
- `internal/ui/tableedit.go` — 8 calls: the same four entries, each twice. Each becomes `parseFloat(<sameArg>, a.settings.DecimalSeparator)`.

Verify afterwards with a grep that no bare `parseFloat(` with a single argument remains in `internal/ui` except the function definition itself.

- [ ] **Step 4: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 3: Umbenennung „Bankkonto" → „Zahlungskonto" (Anzeigetexte)

**Files:**
- Modify: `assets/i18n/de.json`
- Modify: `assets/i18n/en.json`
- Modify: `internal/core/csvrepo.go`

- [ ] **Step 1: Rename the German display values**

In `assets/i18n/de.json`, change the two values (keys unchanged):

- `"table.col.bankaccount": "Bankkonto"` → `"table.col.bankaccount": "Zahlungskonto"`
- `"field.bankAccount": "Bankkonto"` → `"field.bankAccount": "Zahlungskonto"`

- [ ] **Step 2: Rename the English display values**

In `assets/i18n/en.json`, change the two values (keys unchanged):

- `"table.col.bankaccount": "Bank Account"` → `"table.col.bankaccount": "Payment Account"`
- `"field.bankAccount": "Bank Account"` → `"field.bankAccount": "Payment Account"`

- [ ] **Step 3: Rename the column display name**

In `internal/core/csvrepo.go`, in the `ColumnDisplayNames` map, change:

```go
	"Bankkonto":          "Bankkonto",
```

to:

```go
	"Bankkonto":          "Zahlungskonto",
```

(The map *key* `"Bankkonto"` is the internal column ID and stays unchanged; only the display *value* changes.)

- [ ] **Step 4: Verify no other display occurrences**

Grep `internal/ui` for `Bankkonto` and `Bankkonten`. The settings UI already uses „Zahlungskonto" everywhere; expected remaining hits are only internal identifiers (`CSVRow.Bankkonto`, `row.Bankkonto`, `.Bankkonto` field access, the CSV column ID string `"Bankkonto"`). If any *user-facing label string* still reads „Bankkonto"/„Bankkonten", change it to „Zahlungskonto"/„Zahlungskonten". Do NOT change struct field names, the CSV column ID, or i18n keys.

- [ ] **Step 5: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 4: Build, Paketierung, Auslieferung

**Files:** none (build/deploy only)

- [ ] **Step 1: Final build + vet + tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all succeed.

- [ ] **Step 2: Package the Windows executable**

Run (from `C:\Users\istok\Desktop\Dev\BuchISY`):
`fyne package -os windows -name BuchISY -src ./cmd/buchisy`
Expected: `cmd/buchisy/BuchISY.exe` produced.

- [ ] **Step 3: Stop the running app**

Terminate any running `BuchISY` process (PowerShell `wmic process where "ProcessId=<id>" call terminate` per running PID).

- [ ] **Step 4: Deploy and restart**

Copy `cmd/buchisy/BuchISY.exe` over `C:\Users\istok\Desktop\BuchISY.exe`, then launch
`C:\Users\istok\Desktop\BuchISY.exe` with working directory `C:\Users\istok\Desktop`.

- [ ] **Step 5: Manual smoke test**

1. Haupttabelle: Beträge zeigen das Tausender-Trennzeichen (`17.850,00`).
2. „Rechnungsdaten prüfen" / „Rechnung bearbeiten": Betragsfelder vorbefüllt mit Tausender-Trennzeichen; eine Rechnung mit großem Betrag speichern und erneut öffnen → Wert unverändert.
3. Kassenbuch: Beträge mit Tausender-Trennzeichen.
4. Der erzeugte Dateiname enthält das Tausender-Trennzeichen (`…_17.850,00_EUR.pdf`).
5. Feldbeschriftung und Tabellenspalte heißen „Zahlungskonto".

---

## Self-Review

**Spec coverage:**
- A1 gemeinsamer Helfer `core.FormatAmount` → Task 1 Steps 1-4.
- A2 Anzeige (`formatDecimal` → `core.FormatAmount`; greift in Tabelle, Kassenbuch, Dialogfeldern) → Task 2 Step 1.
- A3 Eingabe/Speichern (`parseFloat` Tausender-tolerant, kein Live-Reformat) → Task 2 Steps 2-3.
- A4 Dateiname (Vorlage nutzt `FormatAmount`) → Task 1 Step 5.
- A5 CSV unverändert → es wird kein CSV-Schreibcode angefasst (`core/csvrepo.go` `formatFloat` bleibt); Task 3 ändert in `csvrepo.go` nur einen Map-Wert.
- B Umbenennung (de/en i18n-Werte, `ColumnDisplayNames`, übrige Anzeigetexte) → Task 3.
- Tests `FormatAmount` → Task 1; die Tausender-tolerante Einlese-Logik ist über die manuelle Probe (Task 4 Step 5 Punkt 2) abgedeckt — `parseFloat` ist eine UI-Funktion ohne eigenes Test-Setup.

**Placeholder scan:** Keine TBD/TODO; alle Code-Schritte enthalten vollständigen Code. Task 2 Step 3 ist eine mechanische, klar abgegrenzte Mehrfach-Ersetzung (18 Stellen, je `+ ", a.settings.DecimalSeparator"`), mit Grep-Verifikation.

**Type consistency:** `core.FormatAmount(float64, string) string` (Task 1) wird in `template.go` (Task 1 Step 5) und in `formatDecimal` (Task 2 Step 1) mit dieser Signatur aufgerufen. `parseFloat` erhält die Signatur `(string, string) float64` (Task 2 Step 2); alle 18 Aufrufstellen werden in Task 2 Step 3 entsprechend angepasst. `formatDecimal(float64, string) string` behält seine Signatur — die vorhandenen Aufrufstellen in `table.go`, `kassenbuchview.go`, `invoicemodal.go`, `tableedit.go` bleiben unverändert.
