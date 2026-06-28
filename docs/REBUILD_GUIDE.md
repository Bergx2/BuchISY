# BuchISY Rebuild Guide

Use this when rebuilding BuchISY on another stack. This file is the map; it is not the detailed spec.

## Document Authority

1. `docs/FUNCTIONAL_SPEC.md` is the normative IST specification. It defines current behavior, storage, formats, formulas, quirks, and compatibility rules.
2. `docs/ACCEPTANCE_MATRIX.md` is the parity checklist. Every rebuilt subsystem should link to a spec section and a verification method.
3. `docs/UI_INVENTORY.md` is the user-surface inventory. Use it to rebuild screens, dialogs, commands, menus, keyboard behavior, and visible states.
4. `README.md`, `SETUP.md`, and `BUILDING.md` are user/developer onboarding docs for the current Go/Fyne app. They are not the rebuild contract.
5. `docs/superpowers/` contains historical implementation plans and design notes. Use them only as archaeology when the functional spec is unclear.
6. `docs/xrechnung.md` is historical design context. Current e-invoice behavior is specified in `FUNCTIONAL_SPEC.md`.

## Rebuild Goal

The rebuilt app is correct when it can open existing BuchISY profile data, preserve all files and JSON/SQLite artifacts, reproduce all calculations and exports, and pass the acceptance matrix without requiring users to manually migrate data.

## Non-Negotiable Compatibility

- One SQLite database per profile: `<configDir>/profiles/<profile>/invoices.db`.
- Profile config directory: `<configDir>/profiles/<profile>/`.
- Storage tree: `<StorageRoot>/<YYYY>/<YYYY-MM>/...` when month subfolders are enabled.
- Attachments are numbered sibling files: `<base>_Anhang<N>.<ext>`, not attachment folders.
- `invoices.csv` is regenerated from SQLite, but legacy CSV import/reading behavior remains compatibility-relevant.
- Existing keychain account names and profile handling must keep working.
- Existing side-channel JSON must remain readable: `settings.json`, `company_accounts.json`, `chart_skr04.json`, `buchungsregeln.json`, `booking_templates.json`, per-month `kassenbuch.json`, per-account `metadata.json`.
- Export formats must preserve column order, separators, encodings, line endings, date formats, rounding, and filename rules where specified.

## Recommended Rebuild Order

1. Domain model and pure functions: metadata, tax lines, bookings, formatting, currency normalization, warnings.
2. Persistence compatibility: profile paths, settings JSON, SQLite schema/migrations, CSV read/write, side-channel JSON.
3. File/storage layer: month folders, filename templates, collision suffixes, attachments, backup/export file discovery.
4. Extraction layer: e-invoice CII path, PDF text, local extraction, AI extraction, scanned-PDF vision fallback.
5. Booking and reporting: double-entry builder, UStVA/ZM, SuSa/GuV/OPOS/Controlling, AfA, Kassenbuch.
6. Bank import and reconciliation: CAMT/MT940/PDF parsing, statement metadata cache, dual link model.
7. UI parity: profile picker, Belege, Konten, dialogs, settings, reports, export flows.
8. Packaging and platform behavior: macOS and Windows paths, keychain, file pickers, open/reveal in OS, clipboard, signing.

## Required Test Corpus

- `sample-pdfs/XRECHNUNG_Einfach.pdf`
- `sample-pdfs/ZUGFeRD-Example.pdf`
- An anonymized real profile directory with:
  - multiple months and years;
  - at least one incoming invoice, outgoing invoice, attachment, cash receipt, foreign-currency receipt, locked period, booking export, bank statement metadata file, cash book, and asset;
  - at least one old CSV-only month if legacy import remains in scope.

## Done Criteria

- `docs/ACCEPTANCE_MATRIX.md` is green.
- Existing profile opens without destructive migration.
- Round-trip writes preserve current artifact semantics.
- All generated accounting exports match the functional spec.
- Manual UI smoke test covers every entry in `docs/UI_INVENTORY.md`.
