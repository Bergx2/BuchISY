# Token „erste 8 Zeichen der Kurzbezeichnung" — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ein neuer Dateinamen-Token `${Kurzbez8}` liefert die ersten 8 Zeichen der Kurzbezeichnung; er steht in der Token-Liste und in der neuen Standard-Vorlage, die auch beide bestehenden Profile erhalten.

**Architecture:** Der Token wird in der Template-Engine ergänzt; die Token-Hilfe (i18n) und die Standard-Vorlage werden aktualisiert; die beiden Profil-`settings.json` bekommen die neue Vorlage beim Deployment.

**Tech Stack:** Go 1.25, Fyne v2.6.3, Standard-`testing`.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen. Jede Aufgabe endet mit `go build`/`go vet`/`go test`.

---

### Task 1: Token `${Kurzbez8}` in der Template-Engine (TDD)

**Files:**
- Modify: `internal/core/template.go`
- Test: `internal/core/template_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/core/template_test.go` (falls die Datei nicht existiert, neu anlegen mit `package core` + `import "testing"`):

```go
func TestApplyTemplateKurzbez8(t *testing.T) {
	opts := TemplateOpts{DecimalSeparator: ","}
	cases := []struct {
		kurzbez string
		want    string
	}{
		{"Software Projekt Entwicklung", "Software"},
		{"Abc", "Abc"},
		{"", ""},
	}
	for _, c := range cases {
		got, err := ApplyTemplate("${Kurzbez8}", Meta{Kurzbezeichnung: c.kurzbez}, opts)
		if err != nil {
			t.Fatalf("ApplyTemplate(%q): %v", c.kurzbez, err)
		}
		if got != c.want {
			t.Errorf("ApplyTemplate(${Kurzbez8}) with %q = %q, want %q", c.kurzbez, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestApplyTemplateKurzbez8 -v`
Expected: FAIL — `${Kurzbez8}` is not replaced, so the result is the literal `${Kurzbez8}` (sanitized), not the expected value.

- [ ] **Step 3: Implement the token**

In `internal/core/template.go`, add this helper at the end of the file:

```go
// first8 returns the first 8 runes of s (fewer if s is shorter).
func first8(s string) string {
	r := []rune(s)
	if len(r) > 8 {
		r = r[:8]
	}
	return string(r)
}
```

In `ApplyTemplate`, the `replacements` map currently contains the entry
`"${Kurzbezeichnung}": meta.Kurzbezeichnung,`. Directly after that line, add:

```go
		"${Kurzbez8}":        first8(meta.Kurzbezeichnung),
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestApplyTemplateKurzbez8 -v`
Expected: PASS.

- [ ] **Step 5: Build, vet, full core tests**

Run: `go build ./... && go vet ./internal/core/... && go test ./internal/core/...`
Expected: PASS.

---

### Task 2: Token-Liste + Standard-Vorlage

**Files:**
- Modify: `assets/i18n/de.json`
- Modify: `assets/i18n/en.json`
- Modify: `internal/core/types.go`

- [ ] **Step 1: Add `${Kurzbez8}` to the German token help**

In `assets/i18n/de.json`, the entry is currently:

```json
  "settings.templateHelp": "Verfügbare Tokens: ${YYYY}, ${MM}, ${DD}, ${Company}, ${InvoiceNumber}, ${GrossAmount}, ${Currency}, usw.",
```

Change it to:

```json
  "settings.templateHelp": "Verfügbare Tokens: ${YYYY}, ${MM}, ${DD}, ${Company}, ${Kurzbez8}, ${InvoiceNumber}, ${GrossAmount}, ${Currency}, usw.",
```

- [ ] **Step 2: Add `${Kurzbez8}` to the English token help**

In `assets/i18n/en.json`, the entry is currently:

```json
  "settings.templateHelp": "Available tokens: ${YYYY}, ${MM}, ${DD}, ${Company}, ${InvoiceNumber}, ${GrossAmount}, ${Currency}, etc.",
```

Change it to:

```json
  "settings.templateHelp": "Available tokens: ${YYYY}, ${MM}, ${DD}, ${Company}, ${Kurzbez8}, ${InvoiceNumber}, ${GrossAmount}, ${Currency}, etc.",
```

- [ ] **Step 3: Update the default naming template**

In `internal/core/types.go`, in `DefaultSettings()`, the field is currently:

```go
		NamingTemplate:     "${YYYY}-${MM}-${DD}_${Company}_${GrossAmount}_${Currency}.pdf",
```

Change it to:

```go
		NamingTemplate:     "${YYYY}-${MM}-${DD}_${Company}_${Kurzbez8}_${InvoiceNumber}_${Currency}_${GrossAmount}.pdf",
```

- [ ] **Step 4: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 3: Build, Auslieferung + Vorlage für beide Profile setzen

**Files:** none in the repo (build/deploy + per-profile config)

- [ ] **Step 1: Final build + vet + tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all succeed.

- [ ] **Step 2: Package the Windows executable**

Run (from `C:\Users\istok\Desktop\Dev\BuchISY`):
`fyne package -os windows -name BuchISY -src ./cmd/buchisy`
Expected: `cmd/buchisy/BuchISY.exe` produced.

- [ ] **Step 3: Stop the running app**

Terminate any running `BuchISY` process (PowerShell `wmic process where "ProcessId=<id>" call terminate` per running PID). BuchISY MUST be stopped before Step 4 so it cannot overwrite the edited settings files.

- [ ] **Step 4: Set the naming template for both existing profiles**

While BuchISY is stopped, set the `naming_template` field in each profile's
`settings.json` to the new template, preserving all other fields. Run (it
loads each JSON, changes only `naming_template`, writes it back):

```
python -c "import json; [ (lambda p: json.dump((lambda d: (d.update(naming_template='${YYYY}-${MM}-${DD}_${Company}_${Kurzbez8}_${InvoiceNumber}_${Currency}_${GrossAmount}.pdf') or d))(json.load(open(p,encoding='utf-8'))), open(p,'w',encoding='utf-8'), indent=2, ensure_ascii=False))(r'C:\Users\istok\AppData\Roaming\BuchISY\profiles\%s\settings.json' % name) for name in ['Bergx2 GmbH','Boomstraat GmbH'] ]"
```

After running, verify each file: `naming_template` equals
`${YYYY}-${MM}-${DD}_${Company}_${Kurzbez8}_${InvoiceNumber}_${Currency}_${GrossAmount}.pdf`
and the other settings fields are unchanged.

- [ ] **Step 5: Deploy and restart**

Copy `cmd/buchisy/BuchISY.exe` over `C:\Users\istok\Desktop\BuchISY.exe`, then launch
`C:\Users\istok\Desktop\BuchISY.exe` with working directory `C:\Users\istok\Desktop`.

- [ ] **Step 6: Manual smoke test**

1. Einstellungen: in der Zeile „Verfügbare Tokens" steht `${Kurzbez8}`.
2. Einstellungen: das Vorlagen-Feld zeigt
   `${YYYY}-${MM}-${DD}_${Company}_${Kurzbez8}_${InvoiceNumber}_${Currency}_${GrossAmount}.pdf`
   (in beiden Profilen).
3. Eine Rechnung mit gesetzter Kurzbezeichnung speichern/bearbeiten → die
   Dateiname-Vorschau enthält die ersten 8 Zeichen der Kurzbezeichnung,
   z. B. `…_Boomstraat GmbH_Software_17698_EUR_17.850,00.pdf`.

---

## Self-Review

**Spec coverage:**
- Neuer Token `${Kurzbez8}` (erste 8 Zeichen, rune-basiert) → Task 1 (`first8` + `replacements`-Eintrag).
- Token in der „Verfügbare Tokens"-Liste → Task 2 Steps 1+2 (de/en).
- Festgelegte Vorlage als Standard → Task 2 Step 3 (`DefaultSettings`).
- Vorlage für beide bestehenden Profile → Task 3 Step 4 (`settings.json` je Profil).
- Edge Cases (kürzer als 8, leer, Umlaute) → `first8` per `[]rune`; Test in Task 1 deckt „kürzer" und „leer" ab.
- Unit-Test `ApplyTemplate` mit `${Kurzbez8}` → Task 1.

**Placeholder scan:** Keine TBD/TODO; alle Code-Schritte enthalten vollständigen Code. Task 3 Step 4 ist ein konkreter, vollständiger Befehl mit anschließender Verifikation.

**Type consistency:** `first8(string) string` (Task 1) wird in `ApplyTemplate`s `replacements`-Map verwendet. Der Token-String `${Kurzbez8}` ist in Task 1 (Engine), Task 2 (i18n-Liste, Standard-Vorlage) und Task 3 (Profil-Vorlage) identisch geschrieben. Die Vorlage `${YYYY}-${MM}-${DD}_${Company}_${Kurzbez8}_${InvoiceNumber}_${Currency}_${GrossAmount}.pdf` ist in Task 2 Step 3, Task 3 Step 4 und dem Smoke-Test wörtlich gleich.
