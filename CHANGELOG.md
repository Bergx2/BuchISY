# Changelog

All notable changes to BuchISY are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project uses [semantic-ish versioning](https://semver.org/) via git tags.

> **Source of truth:** the authoritative version is the latest git tag
> (`git tag --sort=-v:refname | head -1`). This file is the human-readable record;
> keep it in sync when you cut a new tag.

## [Unreleased]

### Changed
- Documentation overhaul: README and `.claude/CLAUDE.md` rewritten to reflect the
  current product (a GoBD-compliant bookkeeping pre-system, v2.16) instead of the
  stale "v2.3 invoice organizer" framing.
- Documentation completeness pass: documented ~18 previously-undocumented features
  (UI zoom, keyboard map, filter chips, symbol legend, sum bar, global search,
  Konten view, document preview, undo-on-delete, account picker Recent/Favorites,
  payment accounts, DATEV identifiers, own-VAT-ID exclusion, reconciliation match
  config, chart-of-accounts CSV import + SKR03/SKR04 switch, re-book-to-EUR,
  Bewirtung/Trinkgeld/bar-bezahlt fields, manual booking editor, Kassenbericht /
  cash-year-overview / Buchungsjournal PDFs, image receipts, PDF/Qonto statements).
- Fixed developer-doc breakers: Go version 1.22 → 1.25 (BUILDING.md), i18n file
  format `.toml` → `.json` (CONTRIBUTING/CLAUDE), removed a reference to a
  non-existent cross-compile script.
- Fixed a CLAUDE.md error: `db/maintenance.go` was described as providing "Vacuum"
  and "Backup" — neither exists there (no Vacuum in the code; backup lives in
  `core/backup.go`).
- Added `docs/FUNCTIONAL_SPEC.md`: a complete, IST-exact, stack-independent functional
  specification (data model, business rules with formulas, file formats, state machines,
  algorithms) detailed enough to rebuild BuchISY on another stack with identical behavior.
- Added this CHANGELOG.

### Added
- **Kassenbuch polish:** a newly saved cash receipt now appears immediately (the
  view auto-refreshes and the row blinks once); a linked Belegnummer column;
  Bar-Ausgaben laid out as aligned columns with headers; monthly Einnahmen/Ausgaben
  totals; EUR shown on all balances; Anfangsbestand editable only behind an
  explicit "Ändern" click; "Kassenbericht PDF" now opens the generated report.
- **Table UX:** whole-row hover highlight with a blue outline (Belege + Konten);
  the selected/edited row is marked; column widths are remembered across restarts.
- **Datei → Profil wechseln:** switch company profiles from the menu (with a
  confirm + save of the current profile).
- **Hilfe → Über BuchISY** now shows the build's commit + date, Go version,
  active profile and storage location.

### Fixed
- **Bank-statement parsing:** ignore date lines outside the left date column
  (wrapped description rows are no longer counted as bookings); pick the amount
  column rather than a same-row running balance.
- **Statement links** re-attach by stable identity (amount + date) instead of the
  line index, so a parser improvement can't move a link to the wrong booking; the
  booking cache is versioned so parser fixes reach already-imported statements.
- **Account rename** no longer orphans a Zahlungskonto's statement folder.
- **PDF upload freeze:** some PDFs drove the `ledongthuc/pdf` text parser into an
  unbounded-allocation loop that exhausted memory and froze the whole machine.
  Text extraction now uses go-fitz (MuPDF) only; the highlight-rectangle parse
  (which still needs ledongthuc's per-word boxes) runs in a killable child
  process hard-capped at 1 GiB (Windows Job Object), so a hostile PDF can never
  hang the app.
- **Bank-statement parsing (Sparkasse):** a dedicated parser for the "Umsätze -
  Druckansicht" online export (was reporting 2×N+1 phantom zero-amount rows), and
  the classic statement format now captures amounts printed as a separate
  right-column run (they were read as 0,00, breaking amount-based reconciliation
  for PDF-imported accounts).
- **Account rename no longer orphans statements:** a Zahlungskonto's on-disk
  statement folder is realigned to its (possibly renamed) name via a per-account
  folder pointer, reconciled on startup and on save — renaming in the settings UI
  or by editing `settings.json` directly no longer strands the statements.

---

## The bookkeeping buildout — v2.5 to v2.16 (June 2026)

In a rapid series of feature epics (internally numbered Phase A–E20), BuchISY grew
from an invoice organizer into a full German bookkeeping pre-system. The releases
below group those epics by milestone.

## [2.16.0] - 2026-06-24 — GoBD compliance suite (E20)

### Added
- **Audit trail (Änderungsprotokoll):** append-only log of create/update/delete/
  lock/unlock events, with a viewer dialog. (E20.1)
- **Period locking (Festschreibung):** lock a month immutably; edits and deletes are
  guarded against locked periods, including cross-month moves. (E20.2)
- **OPOS (Offene-Posten-Liste):** open receivables/payables with aging buckets and
  PDF export. (E20.3)
- **SuSa + GuV/BWA:** trial balance and P&L reports with PDF export. (E20.4)
- **Auto-booking rules engine:** per-supplier/keyword booking templates with opt-in
  auto-book (default off) and a plausibility gate. (E20.5)
- **Bank import:** in-house CAMT.053 and MT940 parsers with auto-format detection,
  plus a "missing receipts" list. (E20.6)
- **Fixed assets (Anlagenbuchhaltung):** asset register, linear AfA, GWG hint at
  ≤800 €, and an Anlagenspiegel PDF. (E20.7)
- **GoBD/DATEV-Belegpaket:** export a ZIP bundling the DATEV-EXTF Stapel, linked
  receipt images, a `manifest.csv`, and a GoBD `index.xml` (GDPdU/Z3). (E20.8)
- **Verfahrensdokumentation:** generate the GoBD-required process documentation as a
  PDF from the profile's own settings. (E20.9)
- New database tables `audit_log` and `period_locks`.

## [2.13.0] - 2026-06-23 — Capture ergonomics & navigation (E17–E19)

### Added
- **Global search** across all months, ◀▶ month navigation (Ctrl+←/→), per-year KPI
  overview, and table keyboard navigation (↑/↓/Enter/Del). (E19.3)
- **Readability:** Gegenkonto shows account names, enriched tooltips, consistent
  status symbols, and a legend. (E19.2)
- **Feedback & safety:** "Alle ★ bestätigen" bulk-confirm, undo-delete toast, and a
  zoom-percentage overlay. (E19.1, E19.4)
- **Account picker** with recent + favorites, stored per profile. (E19.5)
- **Reconciliation comfort:** auto-start, confirm-each everywhere, and Kassen-Abgleich. (E18)
- **Entry validation:** richer warnings, early duplicate banner, source badge, live
  warnings, and keyword→account suggestions for new suppliers. (E17.x)

## [2.10.0] - 2026-06-22 — Revenue, VAT filings & reconciliation (E15–E16)

### Added
- **Revenue / outgoing invoices (Ausgangsrechnungen):** Erlöskonten, revenue booking
  and export, auto-flag, and Erlöskonto suggestions. (E15.1, E16.1)
- **Soll-Besteuerung:** Forderungskonto 1400 → Bank on payment. (E16.2)
- **UStVA** with output VAT, §13b reverse-charge, and Zahllast, including the official
  ELSTER Kennzahlen schema (Kz 81/86/21/45/84/85/66/67/83). (E15.2, E15.6)
- **ZM (Zusammenfassende Meldung)** per customer VAT-ID, with a warning when the
  customer VAT-ID is missing. (E15.3, E16.4)
- **Controlling** split (Einnahmen/Ausgaben). (E15.3)
- **Erlös-Abgleich:** revenue reconciliation against bank credits, with grouped/partial
  matching, alias learning, and Claude ranking. (E15.4, E16.3)
- **XML export** for UStVA + ZM, **PDF export** for UStVA + ZM, and a
  Rechnungsausgangsbuch. (E15.7, E16.5, E16.6)
- **Sequential Belegnummer** per profile and year, editable, with a renumber function. (E14)

## [2.5.0] - 2026-06-18 — Booking foundation (Phase A–E8)

### Added
- **Double-entry booking engine:** multiple VAT lines, SKR04 chart of accounts, and
  §13b reverse-charge handling.
- **Multi-profile (Mandanten):** separate chart and booking rules per profile, stored
  under `profiles/<name>/`.
- **Kassenbuch (cash book):** per-Barkasse ledger with opening-balance carry-over and
  cash-coverage checks.
- Foundational booking templates and account mapping.

---

## The invoice-organizer era — v1.0 to v2.4 (October 2025)

## [2.4.0] - 2025-10-16

### Added
- Verwendungszweck/Auftraggeber refinements and a CSV→DB import fix.
- Kassenbuch CSV groundwork.

### Security
- Bumped `golang.org/x/crypto` and `golang.org/x/image`.

## [2.3.0] - 2025-10-15

### Added
- Improved landing page, Code of Conduct, and Contributing guidelines.
- Sample PDFs (XRechnung, ZUGFeRD) for testing.
- Website integration (www.buchisy.de).

### Security
- Updated dependencies with security patches.

## [2.1.0] - 2025-10-15

### Fixed
- PDF vision extraction crash on macOS ARM64 (Apple Silicon), via platform-specific
  rendering (external commands for ARM64, go-fitz elsewhere).

### Changed
- Optimized invoice modal header with a unified file-information layout.
- Real-time filename preview in new/edit dialogs; fixed file-picker scrolling.

## [2.0.0] - 2025-10-15

### Added
- **SQLite database** as source-of-truth, with auto-generated CSV export.
- File attachments, currency conversion fields, comments/notes, and USt-IdNr
  (VAT ID) extraction.
- Sortable file picker with date column, resizable dialogs with saved dimensions.
- Configurable CSV format (separator, encoding, quotes) and column order.

### Changed (breaking)
- SQLite replaces CSV as the primary store; CSV files become auto-generated exports.
- Field renames: `Firmenname` → `Auftraggeber`, `Kurzbezeichnung` → `Verwendungszweck`
  (old CSV column names are still read for backward compatibility).

## [1.2.0] - 2025-10-14

### Added
- Invoice modal with comment, currency conversion, and attachments (v1.3 features).

## [1.1.0] - 2025-10-14

### Added
- Early macOS and Windows release builds.

## [1.0.0] - 2025-10-14

### Added
- Initial release: PDF invoice metadata extraction (Claude API or local heuristics),
  month-based folder organization, and CSV output.

[Unreleased]: https://github.com/Bergx2/BuchISY/compare/v2.16.0...HEAD
[2.16.0]: https://github.com/Bergx2/BuchISY/releases/tag/v2.16.0
[2.13.0]: https://github.com/Bergx2/BuchISY/releases/tag/v2.13.0
[2.10.0]: https://github.com/Bergx2/BuchISY/releases/tag/v2.10.0
[2.5.0]: https://github.com/Bergx2/BuchISY/releases/tag/v2.5
[2.4.0]: https://github.com/Bergx2/BuchISY/releases/tag/v2.4
[2.3.0]: https://github.com/Bergx2/BuchISY/releases/tag/v2.3
[2.1.0]: https://github.com/Bergx2/BuchISY/releases/tag/v2.1
[2.0.0]: https://github.com/Bergx2/BuchISY/releases/tag/v2.0.0
[1.2.0]: https://github.com/Bergx2/BuchISY/releases/tag/v1.2.0
[1.1.0]: https://github.com/Bergx2/BuchISY/releases/tag/v1.1
[1.0.0]: https://github.com/Bergx2/BuchISY/releases/tag/v1.0
