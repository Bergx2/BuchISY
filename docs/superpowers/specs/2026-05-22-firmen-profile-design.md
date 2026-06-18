# Firmen-Profile — Profil-Auswahl beim Start — Design

**Datum:** 2026-05-22
**Status:** Genehmigt (Design)

## Überblick

BuchISY soll für mehrere Firmen (z. B. Bergx2 GmbH, Boomstraat GmbH) mit
jeweils vollständig getrennten Einstellungen, Konto-Zuordnungen, Logs,
Ablageordnern und API-Schlüsseln nutzbar sein — bei **einer** Codebasis
und **einer** `.exe`.

Lösung: ein **Profil**. Beim Start zeigt BuchISY einen
Profil-Auswahlbildschirm; nach der Auswahl initialisiert sich die App
vollständig für dieses Profil. Es läuft danach genau eine, für ein Profil
initialisierte Instanz — kein Umschalten im laufenden Betrieb.

## Nicht-Ziele (YAGNI)

- Kein Profilwechsel im laufenden Betrieb (dazu BuchISY neu starten).
- Keine umfangreiche Profil-Verwaltung — nur: Profile auflisten, eines
  wählen, ein neues anlegen.
- Kein Umbenennen/Löschen von Profilen über die UI (ginge zur Not über
  den Dateimanager).

## Komponenten & Datenfluss

### 1. Profil-Verzeichnis (`internal/core/settings.go`)

- Die Konfiguration eines Profils liegt unter
  `<AppData>/BuchISY/profiles/<Profil>/` — `settings.json`,
  `company_accounts.json`, `logs/`.
- `GetConfigDir()` wird zu `GetConfigDir(profile string)` und liefert
  `<AppData>/BuchISY/profiles/<profile>`. Der bisherige Pfad
  `<AppData>/BuchISY` bleibt der „Stamm" (enthält `profiles/` und ggf.
  eine Alt-Konfiguration).
- Eine Funktion `ListProfiles()` liefert die Namen der vorhandenen
  Profile (Unterordner von `<AppData>/BuchISY/profiles/`).
- Profilnamen werden auf dateinamentaugliche Zeichen beschränkt
  (`SanitizeFilename`); ein leerer Name ist unzulässig.

### 2. Start-Ablauf mit Auswahlbildschirm (`cmd/buchisy/main.go`, `internal/ui/app.go`, neue Datei `internal/ui/profilepicker.go`)

- `main.go` startet wie bisher die App; der eigentliche Aufbau der
  Hauptansicht wird jedoch **erst nach der Profilauswahl** ausgelöst.
- Beim Start zeigt BuchISY in seinem Fenster einen Auswahlbildschirm:
  - Liste der vorhandenen Profile (über `ListProfiles()`), je ein Knopf.
  - Ein Knopf „+ Neues Profil".
- Klick auf ein vorhandenes Profil → die App lädt dessen Konfiguration
  und baut die Hauptansicht.
- „+ Neues Profil" → Eingabe eines Namens → das Profilverzeichnis wird
  angelegt, mit Standardeinstellungen initialisiert und direkt geöffnet.
- Die App-Initialisierung (Einstellungen laden, `StorageManager`,
  `CSVRepository`, i18n, Migrationen, Hauptfenster) erfolgt erst, wenn das
  Profil feststeht — also wird der heutige Initialisierungs-Code von
  `New()` in eine Funktion ausgelagert, die der Auswahlbildschirm mit dem
  gewählten Profil aufruft.

### 3. Profil-bezogener API-Schlüssel (`internal/ui/app.go`, `internal/ui/settings.go`)

- Der API-Schlüssel liegt im Betriebssystem-Tresor. Heute: Dienst
  `"BuchISY"`, Konto `settings.AnthropicAPIKeyRef` (Standard `"claude"`).
- Künftig wird der Konto-Name mit dem Profil präfixiert, z. B.
  `Bergx2-claude`, damit sich die Schlüssel zweier Firmen nicht
  überschreiben. Die vier `keyring.Get`/`keyring.Set`-Aufrufstellen nutzen
  einen gemeinsamen Helfer `(a *App) keyringAccount() string`, der
  `<profil>-<AnthropicAPIKeyRef>` zurückgibt.

### 4. Übernahme der Alt-Konfiguration

- Eine „Alt-Konfiguration" ist eine `settings.json` direkt unter
  `<AppData>/BuchISY/` (aus der bisherigen Einzel-Installation).
- Legt der Nutzer ein Profil **neu** an und existiert eine
  Alt-Konfiguration, fragt BuchISY: „Bestehende Konfiguration diesem
  Profil zuordnen, oder leer starten?"
  - *Zuordnen* → `settings.json`, `company_accounts.json` und `logs/`
    werden aus `<AppData>/BuchISY/` nach `profiles/<Profil>/` verschoben;
    der API-Schlüssel im Tresor wird vom alten Konto (`claude`) auf das
    profil-bezogene Konto (`<Profil>-claude`) kopiert.
  - *Leer starten* → das Profil startet mit Standardeinstellungen; die
    Alt-Konfiguration bleibt unverändert liegen.
- Sobald die Alt-Konfiguration verschoben ist, entfällt die Abfrage.

### 5. Fenstertitel

- Der Fenstertitel der Hauptansicht zeigt das aktive Profil:
  „BuchISY — <Profil>".

## Edge Cases

- Noch keine Profile vorhanden → der Auswahlbildschirm zeigt nur
  „+ Neues Profil".
- Profilname mit unzulässigen Zeichen / leer → Eingabe wird abgewiesen
  bzw. bereinigt; bei Konflikt mit einem bestehenden Profil ein Hinweis.
- Keine Alt-Konfiguration vorhanden → beim Anlegen eines Profils keine
  Abfrage, direkt Standardeinstellungen.
- Profilordner existiert, aber `settings.json` fehlt → wie ein leeres
  Profil behandelt (Standardeinstellungen).

## Betroffene / neue Dateien

- `internal/core/settings.go` — `GetConfigDir(profile)`, `ListProfiles()`.
- `cmd/buchisy/main.go` — Start ohne sofortigen Hauptaufbau.
- `internal/ui/app.go` — Initialisierung in eine vom Profil abhängige
  Funktion ausgelagert; `keyringAccount()`-Helfer; Fenstertitel mit
  Profil; die vier Keyring-Aufrufe.
- `internal/ui/settings.go` — Keyring-Aufrufe über `keyringAccount()`.
- `internal/ui/profilepicker.go` — neu: Auswahlbildschirm, „Neues Profil",
  Übernahme-Abfrage der Alt-Konfiguration.

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Unit-Test `GetConfigDir(profile)`: liefert
  `<Root>/BuchISY/profiles/<profile>`.
- Unit-Test `ListProfiles()`: ein temporäres `profiles/`-Verzeichnis mit
  zwei Unterordnern → liefert beide Namen; fehlendes Verzeichnis → leere
  Liste, kein Fehler.
- Manuell:
  - Erster Start: Auswahlbildschirm, „+ Neues Profil" → „Bergx2" anlegen
    → Abfrage zur Alt-Konfiguration → „Zuordnen" → bisherige Einstellungen
    sind im Bergx2-Profil vorhanden.
  - „+ Neues Profil" → „Boomstraat" → startet leer; eigener Ablageordner
    in den Einstellungen setzbar; beide Profile beeinflussen sich nicht.
  - Fenstertitel zeigt das gewählte Profil.
  - Neustart → Auswahlbildschirm zeigt beide Profile.
