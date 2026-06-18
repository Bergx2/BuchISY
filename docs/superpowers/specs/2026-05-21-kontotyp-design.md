# Kontotyp (Bank / Kreditkarte / Barkasse) — Design

**Datum:** 2026-05-21
**Status:** Genehmigt (Design)

## Überblick

Zahlungskonten in BuchISY (`core.BankAccount`) tragen aktuell ein
boolesches `IsCreditCard`-Häkchen. Eine Barkasse lässt sich damit nicht
sauber abbilden. Dieses Feature ersetzt das Häkchen durch einen
dreiwertigen **Kontotyp**: Bank, Kreditkarte oder Barkasse. Der Typ ist
rein organisatorisch und bildet die Grundlage für den späteren
Kassenbericht (eigenes Folge-Feature).

## Ziele

- `BankAccount` bekommt einen Kontotyp statt des `IsCreditCard`-Häkchens.
- Bestehende Kreditkarten-Kennzeichen werden migriert (nicht verloren).
- In den Einstellungen wählt man den Typ je Konto aus einem Auswahlfeld.
- Der Bereich „Bankkonten" wird zu „Zahlungskonten" umbenannt.

## Nicht-Ziele (YAGNI)

- Kein Kassenbericht (separates Folge-Feature).
- Keine Änderung an Rechnungsverarbeitung, Bestätigungsdialog oder CSV.
- Keine Umbenennung der CSV-Spalte „Bankkonto" (bestehende `invoices.csv`
  blieben sonst inkompatibel).

## Komponenten & Datenfluss

### 1. Datenmodell (`internal/core/types.go`)

- Neue Typ-Konstanten:
  - `AccountTypeBank = "bank"`
  - `AccountTypeCreditCard = "creditcard"`
  - `AccountTypeCash = "cash"`
- `BankAccount` bekommt `AccountType string` (`json:"account_type"`).
- `IsCreditCard bool` (`json:"is_credit_card"`) bleibt als **verstecktes
  Legacy-Feld** im Struct — ausschließlich für die einmalige Migration.
  Es wird nicht mehr in der UI verwendet.

### 2. Migration (`internal/core/settings.go`)

- Beim Laden der Einstellungen wird jedes `BankAccount` normalisiert:
  - Ist `AccountType` leer und `IsCreditCard == true` → `AccountType =
    AccountTypeCreditCard`.
  - Ist `AccountType` leer (sonst) → `AccountType = AccountTypeBank`.
  - `IsCreditCard` wird danach auf `false` gesetzt (Legacy-Wert entwertet).
- Dadurch erhalten bestehende `settings.json` ohne `account_type` einen
  gültigen Typ, ohne bereits gesetzte Kreditkarten-Kennzeichen zu verlieren.
- Die Normalisierung ist eine eigene, testbare Funktion
  `normalizeBankAccounts([]BankAccount) []BankAccount`.

### 3. Einstellungen-UI (`internal/ui/settings.go`)

- In der editierbaren Zeile je Zahlungskonto ersetzt ein **Auswahlfeld
  „Typ"** das bisherige „Kreditkarte"-Häkchen. Anzeigewerte: „Bank",
  „Kreditkarte", „Barkasse"; intern auf die Typ-Konstanten gemappt.
- Das Feld „Ausgleich über" ist nur aktiv (enabled), wenn der Typ
  „Kreditkarte" ist; bei „Bank" und „Barkasse" ist es deaktiviert.
- Wechselt der Nutzer den Typ, wird „Ausgleich über" entsprechend
  aktiviert/deaktiviert.
- Umbenennungen der UI-Beschriftungen im Bereich „Konten":
  - „Bankkonten" → „Zahlungskonten"
  - „Standard-Bankkonto" → „Standard-Zahlungskonto"
  - „+ Bankkonto hinzufügen" → „+ Zahlungskonto hinzufügen"
  - „Bankkonto Name" → „Zahlungskonto Name"
- Neu hinzugefügte Konten bekommen standardmäßig `AccountType =
  AccountTypeBank`.

### 4. Verhalten

Der Kontotyp ist rein organisatorisch. Rechnungsverarbeitung,
Bestätigungsdialog, Bankkonto-Auswahl im Dialog und CSV bleiben
unverändert. Der Typ wird in `settings.json` persistiert und steht dem
späteren Kassenbericht zur Verfügung.

## Edge Cases

- `settings.json` ganz ohne `bank_accounts` → leere Liste, keine Migration
  nötig.
- Unbekannter `account_type`-Wert in der JSON → bei der Normalisierung wie
  „leer" behandelt und auf `bank` gesetzt.
- Default-Settings (`DefaultSettings`): das vorgegebene Bankkonto
  „Sparkasse" bekommt `AccountType = AccountTypeBank`.

## Betroffene Dateien

- `internal/core/types.go` — Typ-Konstanten, `BankAccount`-Feld,
  `DefaultSettings`.
- `internal/core/settings.go` — `normalizeBankAccounts` + Aufruf beim Laden.
- `internal/core/settings_test.go` — Test der Migration/Normalisierung.
- `internal/ui/settings.go` — Typ-Auswahlfeld statt Häkchen, „Ausgleich
  über" abhängig vom Typ, Umbenennungen.

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Unit-Test für `normalizeBankAccounts`: Legacy `is_credit_card=true` →
  `creditcard`; leerer Typ → `bank`; unbekannter Typ → `bank`; bereits
  gesetzter Typ bleibt erhalten.
- Manuell: Einstellungen → Konten → Zahlungskonten — Typ je Konto wählbar,
  „Ausgleich über" nur bei Kreditkarte aktiv, Speichern/Neuladen behält den
  Typ.
