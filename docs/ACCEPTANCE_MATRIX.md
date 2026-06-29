# BuchISY Acceptance Matrix

Parity gate for a rebuild. Each row needs an automated test where practical, plus a real-profile smoke test before release.

| Area | Must Reproduce | Spec | Verification |
|---|---|---|---|
| Profile paths | Config root, `profiles/<name>`, per-profile `invoices.db`, profile picker, legacy config migration | Functional Spec, Overview & Data Model | Open existing profile; create/switch profile; compare config paths |
| SQLite schema | `invoices`, `audit_log`, `period_locks`, indexes, trigger, idempotent ALTER migrations, no invoice UNIQUE constraint | Functional Spec, Overview & Data Model | Schema introspection; open old DB; migration idempotency test |
| Settings JSON | Exact keys/defaults, pretty JSON, bank-account normalization, default storage root | Functional Spec, Settings | JSON round-trip fixture; missing/corrupt file behavior |
| CSV journal | Default columns, custom order, separator, encoding, legacy column names, LF line endings | Functional Spec, Exports | Golden CSV write/read tests for UTF-8 and ISO-8859-1 |
| File layout | `YYYY/YYYY-MM`, category subfolders, `_Anhang<N>` siblings, `_2/_3` collision suffixes | Functional Spec, On-disk layout and Filename Rules | Temp-dir storage tests; rename invoice with attachments |
| Intake | Drag/drop, picker, batch, clipboard file/image, scan inbox, attachment main-file marker | Functional Spec, Capture & Extraction; UI Inventory | UI smoke test; batch import with mixed files |
| E-invoice | Attachment extraction, filename/content detection, CII-only parsing, first-tax-line behavior, confidence 1.0 | Functional Spec, Capture & Extraction | `sample-pdfs` extraction tests; UBL negative/blank-meta behavior documented |
| PDF/text extraction | E-invoice first, text path, local heuristic path, Claude path, vision fallback | Functional Spec, Capture & Extraction | Sample native PDF, scanned PDF, image, local mode |
| Account model | SKR seed, chart import, SKR03/SKR04 detection/switch, payment vs counter-account distinction | Functional Spec, Chart of Accounts | Chart import fixture; validation errors; account picker smoke |
| Company memory | Normalization, suffix stripping, exact normalized lookup, save failures non-fatal | Functional Spec, Company Mapping | Unit tests for normalization and lookup |
| Booking engine | Incoming, outgoing, VAT lines, §13b, categories, discounts, fees, balance tolerance | Functional Spec, Booking Engine and Revenue | Port Go unit tests as golden vectors |
| Manual booking editor | Editable Soll/Haben lines, balance indicator, reset-to-auto | UI Inventory; Functional Spec, Booking Engine | Dialog smoke test; unbalanced save behavior |
| Period locking | Lock/unlock audit, edit/delete/cross-month guard, locked UI state | Functional Spec, GoBD Mechanisms | Repo tests; UI lock/edit/delete smoke |
| Audit log | Best-effort append-only CRUD/lock entries, update field diff, newest-first view | Functional Spec, Audit | DB tests; viewer smoke |
| Receipt numbers | `YYYY-NNNN`, gap-free assignment/renumbering, year scope | Functional Spec, GoBD Mechanisms | Renumber test with deleted/moved rows |
| UStVA | Official Kennzahlen, account-based legacy view, XML/PDF, period defaults, own VAT-ID | Functional Spec, VAT Filings | Golden numbers and XML text compare |
| ZM | EU VAT-ID rules, 0 VAT exact test, grouping/sorting/control sum, XML/PDF | Functional Spec, VAT Filings | Golden numbers and XML text compare |
| Reports | SuSa, GuV, OPOS, Controlling, Year Overview, Belegliste, Sales Journal, Booking Journal | Functional Spec, Reports | Unit tests for numbers; PDF smoke by `%PDF` and extracted text |
| AfA/assets | Manual asset register, ID generation, linear AfA, GWG behavior, report-only account metadata | Functional Spec, Fixed Assets | Unit tests for AfA/RBW; UI smoke |
| Kassenbuch | Per-month per-account JSON, deposits, cash invoices, carry-in 60-month lookback, PDFs | Functional Spec, Kassenbuch | Unit tests; real cash account smoke |
| Bank import | CAMT.053, MT940, PDF positioned-text parsing, Qonto path | Functional Spec, Bank Import | Parser fixtures; real statement smoke |
| Reconciliation | Match scoring, confirmation-only linking, grouped/partial payments, alias learning, dual link sync | Functional Spec, Bank Import & Reconciliation | Fixture matches; link/unlink smoke; metadata preservation |
| Auto-booking | Template learning, opt-in `autobook`, plausibility gate, duplicate pre-check, fallback modal | Functional Spec, Auto-Booking Rules | Unit tests; batch import smoke |
| DATEV export | Header/columns, CRLF, cp1252 on disk, row expansion, field cleaning, period naming | Functional Spec, Exports | Byte/text golden; invalid booking skip test |
| Lexware export | Header, semicolon, CRLF, no quotes, entry orientation, field cleaning | Functional Spec, Exports | Byte/text golden |
| GoBD package | ZIP entries, manifest, `index.xml`, receipt copying, unreadable file skip | Functional Spec, Exports | ZIP listing and extracted text/XML compare |
| Backup | DB, config JSON, storage files included as specified | Functional Spec, Backup ZIP | ZIP listing compare |
| Verfahrensdoku | Generated PDF sections, current profile/settings, known attachment text quirk | Functional Spec, Verfahrensdokumentation | PDF extracted text smoke |
| i18n | German/English JSON keys, fallback behavior, visible labels | UI Inventory | Run both languages; missing-key scan |
| Platform integration | Keychain, OS open/reveal, file pickers, clipboard, window/dialog geometry, UI zoom | Functional Spec; UI Inventory | macOS and Windows smoke |

## Release Gate

- Run the full current Go gate before starting parity comparison: `go test ./...`.
- For the rebuild, mirror the Go unit tests as domain tests before implementing UI.
- Before handoff, run an anonymized real-profile smoke test and record exceptions in this file.
