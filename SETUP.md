# BuchISY — Setup auf einem neuen Rechner

Kurzanleitung, um BuchISY auf einem anderen Laptop **zu nutzen** und/oder **weiterzuentwickeln**.

> Alles ist auf GitHub (`Bergx2/BuchISY`). Ein `git clone` liefert den kompletten Quellcode,
> `CLAUDE.md` und alle Pläne unter `docs/superpowers/plans/`.

---

## 1. Nur die App benutzen (laufen lassen)

Die `BuchISY.exe` ist **eigenständig** — Assets sind einkompiliert, kein Installer nötig.
Auf einem **Windows-64-bit**-Rechner einfach starten. Zwei Wege:

- **Empfohlen:** aus dem aktuellen Release laden →
  <https://github.com/Bergx2/BuchISY/releases/latest> → `BuchISY.exe` (oder `BuchISY-Windows.zip`).
- **Oder** die lokal gebaute `dist/BuchISY.exe` per USB/Cloud kopieren.

### Daten & API-Key mitnehmen

Die Daten liegen **nicht** in der `.exe`, sondern unter `%APPDATA%\BuchISY\`:

| Datei/Ordner | Inhalt |
|---|---|
| `invoices.db` | SQLite-Datenbank (alle Belege) |
| `settings.json` | Einstellungen |
| `company_accounts.json` | Firma→Konto-Zuordnungen |
| `profiles\` | SKR04-Kontenrahmen + Buchungsregeln je Profil (Bergx2, Boomstraat) |
| `logs\` | Logdateien |

Zum Übertragen: den Ordner `%APPDATA%\BuchISY\` kopieren und auf dem neuen Laptop an dieselbe Stelle legen.

> ⚠️ **Der Claude-API-Key wandert NICHT mit** — er liegt im Windows-Anmeldeinformations-Manager
> (Dienst `BuchISY`), nicht im Datenordner. Auf dem neuen Laptop einmal in den **Einstellungen** neu eintragen.

---

## 2. Weiterentwickeln (Dev-Setup)

### Voraussetzungen installieren

| Tool | Zweck | Quelle |
|---|---|---|
| **Git** | klonen / committen | <https://git-scm.com> |
| **Go 1.25+** | Build (siehe `go.mod`: `go 1.25.0`) | <https://go.dev/dl> |
| **C-Compiler (gcc)** | ⚠️ **Pflicht** — Fyne + go-fitz nutzen `cgo` | MSYS2 (mingw-w64) oder TDM-GCC |
| gh CLI *(optional)* | Releases taggen / pushen | <https://cli.github.com> |

### Klonen & bauen

```powershell
git clone https://github.com/Bergx2/BuchISY.git
cd BuchISY
go build -ldflags "-H=windowsgui" -o dist/BuchISY.exe ./cmd/buchisy
```

(oder `make build`). Starten:

```powershell
.\dist\BuchISY.exe
```

### Git / GitHub einrichten (zum Pushen)

```powershell
git config --global user.name  "istok"
git config --global user.email "istok@bergx2.de"
gh auth login        # falls gh genutzt wird (Push / Releases)
```

---

## Der häufigste Stolperstein: gcc fehlt

Fyne-Apps lassen sich auf Windows **nicht ohne C-Compiler** bauen. Bricht `go build` mit
`cgo: C compiler "gcc" not found` ab:

1. **MSYS2** installieren (<https://www.msys2.org>).
2. In der MSYS2-Shell: `pacman -S mingw-w64-x86_64-gcc`
3. Den bin-Ordner in den `PATH` aufnehmen, z. B. `C:\msys64\mingw64\bin`.
4. Neue Terminal-Sitzung öffnen, `gcc --version` prüfen, dann `go build` erneut.

> Zum **Laufen** der fertigen `.exe` braucht es **keinen** gcc — nur zum **Bauen**.

---

## Release bauen & veröffentlichen

Ein Tag `vX.Y.Z` auf `main` löst die GitHub-Actions-Builds (Windows/macOS) aus und erstellt ein Release:

```powershell
git tag -a v2.11.0 -m "Beschreibung"
git push origin v2.11.0
```

Fortschritt: <https://github.com/Bergx2/BuchISY/actions>.

---

## Was NICHT über das Repo mitkommt

- **Daten** (`%APPDATA%\BuchISY\`) und **API-Key** (Credential Manager) — siehe Abschnitt 1.
- **Claude-Code-Projektgedächtnis** (`~/.claude/...`) — laptop-lokal; Code + Pläne im Repo bleiben verfügbar.

---

## Architektur-Überblick

Siehe `.claude/CLAUDE.md` (vollständige Architektur) und `README.md` (Nutzer-Doku).
Implementierungspläne der Phasen A–E10 liegen in `docs/superpowers/plans/`.
