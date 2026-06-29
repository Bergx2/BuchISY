# BuchISY — Functional Specification (for rebuild on any stack)

## Purpose

This document specifies the **current (IST)** behavior of BuchISY precisely enough to **rebuild it on a different technology stack with identical behavior**. It is **stack-independent**: it describes data, formats, formulas, and observable behavior — not implementation. The reference implementation is Go + Fyne + SQLite (v2.16) and is named only as the source of truth for IST behavior; **nothing in this document mandates that stack, language, framework, or storage engine**. A conforming rebuild on any technology that reproduces the data model, formats, formulas, and behaviors described here is correct.

## How to read this

- **IST-exact.** Every chapter documents what BuchISY does **today**, including quirks and non-obvious behavior. Quirks are called out inline as `> Quirk:` blocks. A quirk is part of the spec: reproduce it unless explicitly noted otherwise.
- **Formats and formulas are normative.** Field layouts, column orders, file/path formats, rounding rules, and calculation formulas must be reproduced exactly. Deviation is non-conformance.
- **Worked examples are illustrative.** Concrete numbers, sample files, and step-by-step walkthroughs exist to disambiguate the normative text. Where an example appears to conflict with a formula or format rule, the **formula/format rule wins**.
- **Document hierarchy.** For a rebuild, read this file first, then `docs/REBUILD_GUIDE.md`, `docs/ACCEPTANCE_MATRIX.md`, and `docs/UI_INVENTORY.md`. Historical design notes under `docs/superpowers/` and `docs/xrechnung.md` are non-normative unless this spec explicitly says otherwise.

## Conventions

- **Dates:** `DD.MM.YYYY` everywhere (display, storage, exports).
- **Decimal separator:** comma `,` in the UI; dot `.` in CSV/export files.
- **Currency:** ISO 4217 codes (e.g. `EUR`); symbols are normalized (`€` → `EUR`).
- **Money:** decimal values rounded to `0.01` (two fractional digits).
- **Comparison tolerance:** monetary equality is checked within `≈ 0.005`.

## Scope boundary

BuchISY is a **Vorsystem** (capture, book, lock, VAT, export) — it does **not** produce a full HGB Jahresabschluss / Bilanz.

## Table of Contents

1. Overview & Data Model
2. Capture & Extraction
3. Chart of Accounts, SKR & Account Model
4. Booking Engine (Double-Entry & VAT)
5. Revenue & Outgoing Invoices
6. VAT Filings: UStVA & ZM
7. Reports: SuSa, GuV, OPOS, Controlling, Overviews
8. Fixed Assets (AfA) & Cash Book (Kassenbuch)
9. Bank Import & Reconciliation
10. Auto-Booking Rules Engine
11. Exports, GoBD Compliance & Filename/Storage Rules

---

## Overview & Data Model

### 1. What BuchISY is

BuchISY is a single-user **German bookkeeping pre-system** (*Vorsystem*) for a desktop computer. It implements GoBD-supporting mechanisms such as audit trail, period locking, receipt numbers, backups, and export packages, but it is not by itself a substitute for legal/tax review. It is **not** a *Jahresabschluss* (annual-accounts) package and does not produce a full HGB balance sheet, annual close, or tax return. It does produce operational evaluations and filings such as GuV-style reports, UStVA, and ZM. Its job is to take incoming and outgoing invoices (mostly PDFs, plus E-Rechnung XML), extract their metadata (supplier, amounts, VAT lines, dates) — automatically via the Anthropic Claude API or a local heuristic extractor — let the user **capture** and correct each receipt, **book** it as a balanced double-entry posting against an SKR04-style chart of accounts, **lock** (festschreiben) a filing period so it can no longer be changed, reconcile receipts against scanned bank-statement PDFs (*Belegabgleich*) and a cash book (*Kassenbuch*), and finally **export** the bookings to downstream accounting software (DATEV, CSV, journal PDFs). The canonical flow is: **capture → book → lock → file → export**. Because it is a Vorsystem, the bookings it emits are handed to a tax advisor's real accounting system; BuchISY's authoritative goal is a complete, immutable, auditable trail of receipts and their proposed postings.

Data is organised **per profile** (a profile is one bookkeeping entity / client). Each profile has its own SQLite database, its own settings, its own chart of accounts, its own booking rules, and its own document storage tree on disk (under `Documents/BuchISY` by default, organised into `YYYY/YYYY-MM` month folders). The invoice metadata lives in a single SQLite database per profile; the original receipt PDFs, bank-statement PDFs, the generated cash book, exports, and a handful of side-channel JSON files live as plain files in the storage tree and in the profile's config directory. A re-implementation must reproduce both stores and keep them consistent, because some data (the invoice rows) lives only in SQLite while related data (cash book opening balances, statement metadata, company→account memory) lives only in JSON files next to the documents or in the config dir.

> Quirk: `internal/db/paths.go` contains a `GetDBPath(storageRoot, jahr, monat)` returning `<storageRoot>/YYYY-MM/invoices.db` — a **per-month** database layout. This is **dead/alternative code and is NOT used.** The app actually opens **one global database per profile** via `GetGlobalDBPath(configDir)` = `<profileConfigDir>/invoices.db`. Re-implementers should use the single-DB-per-profile model and ignore the per-month path.

### 2. The complete data model

The profile's SQLite database (`<profileConfigDir>/invoices.db`) holds three tables: `invoices`, `audit_log`, `period_locks`, plus one trigger.

#### 2.1 Table `invoices`

Created with `CREATE TABLE IF NOT EXISTS`. The base schema has 35 columns; three more (`bewirtung_anlass`, `bewirtung_teilnehmer`, and historically `trinkgeld`/`steuerzeilen`/`buchung`/`exportiert`/`wechselkurs`/`gebuehr_prozent`/`rabatt`/`buchung_ref`/`belegnummer`/`ausgangsrechnung`) are added idempotently via `ALTER TABLE` on startup (see §7). The effective column set (in physical order) is:

| # | Column | Type | Default | Meaning |
|---|--------|------|---------|---------|
| 1 | `id` | INTEGER | PK AUTOINCREMENT | Surrogate row id. |
| 2 | `dateiname` | TEXT NOT NULL | — | Final on-disk filename of the receipt's main file. Acts as the natural row key within (jahr, monat). |
| 3 | `rechnungsdatum` | TEXT | NULL | Invoice date, **DD.MM.YYYY**. |
| 4 | `jahr` | TEXT | NULL | Filing year **YYYY** (string). |
| 5 | `monat` | TEXT | NULL | Filing month **MM** (string, zero-padded). |
| 6 | `auftraggeber` | TEXT | NULL | Counterparty / company name (supplier for incoming, customer for outgoing). |
| 7 | `verwendungszweck` | TEXT | `'-'` | Purpose / short description. Empty string is normalised to `"-"` on write. |
| 8 | `rechnungsnummer` | TEXT | NULL | Invoice number. |
| 9 | `betrag_netto` | REAL | NULL | Net amount (aggregate; sum of tax-line nets). |
| 10 | `steuersatz_prozent` | REAL | NULL | Primary VAT rate in percent (display aggregate; rate of first tax line). |
| 11 | `steuersatz_betrag` | REAL | NULL | VAT amount (aggregate; sum of tax-line VAT). |
| 12 | `bruttobetrag` | REAL | NULL | Gross amount (net + VAT + Trinkgeld). |
| 13 | `waehrung` | TEXT | NULL | Currency code (EUR, USD, …). |
| 14 | `gegenkonto` | INTEGER | NULL | Account code (Gegenkonto / expense or revenue account). |
| 15 | `bankkonto` | TEXT | NULL | Payment account name (*Zahlungskonto*; the bank/cash/credit-card account name). |
| 16 | `bezahldatum` | TEXT | NULL | Payment date, **DD.MM.YYYY**. |
| 17 | `teilzahlung` | BOOLEAN | `0` | Partial-payment flag. |
| 18 | `kommentar` | TEXT | NULL | Free-text note. |
| 19 | `bewirtung_anlass` | TEXT | `''` (added by ALTER) | Entertainment occasion (§4 Abs.5 EStG). |
| 20 | `bewirtung_teilnehmer` | TEXT | `''` (added by ALTER) | Entertainment participants. |
| 21 | `betrag_netto_eur` | REAL | NULL | Net amount in EUR for foreign-currency receipts. |
| 22 | `gebuehr` | REAL | NULL | Fee (e.g. FX fee), absolute. |
| 23 | `rabatt` | REAL | `0` | Third-party rebate / platform voucher (gross rebate deducted from payment). |
| 24 | `hat_anhaenge` | BOOLEAN | `0` | Has additional file attachments. |
| 25 | `ustidnr` | TEXT | NULL | Counterparty VAT-ID (maps to Meta/CSV field `VATID`). **Column name differs from field name.** |
| 26 | `trinkgeld` | REAL | NULL | Tip; no VAT; part of gross only. |
| 27 | `steuerzeilen` | TEXT | `''` | VAT lines as JSON array (see §4). |
| 28 | `buchung` | TEXT | `''` | Double-entry booking as JSON (see §4). |
| 29 | `exportiert` | INTEGER | `0` | 1 once included in a booking export. |
| 30 | `wechselkurs` | REAL | `0` | FX rate used for currency conversion. |
| 31 | `gebuehr_prozent` | REAL | `0` | Fee rate in percent. |
| 32 | `buchung_ref` | TEXT | `''` | Reference to a bank-statement booking, format `statementFilename|page|lineIdx` (see §4). |
| 33 | `belegnummer` | TEXT | `''` | Sequential receipt number per profile+year, format **YYYY-NNNN**. |
| 34 | `ausgangsrechnung` | INTEGER | `0` | 1 = outgoing/revenue invoice (Erlös); 0 = incoming/expense. |
| 35 | `created_at` | DATETIME | `CURRENT_TIMESTAMP` | Row creation. |
| 36 | `updated_at` | DATETIME | `CURRENT_TIMESTAMP` | Last update (maintained by trigger). |

**No `UNIQUE` constraint exists anywhere in the schema** — not on `dateiname`, not on `belegnummer`, not on any combination. There is therefore nothing in the database that prevents duplicate rows; duplicate detection is done in application code only (see `IsDuplicate`, §2.4). A re-implementer must replicate this: do **not** add a uniqueness constraint.

**Indexes on `invoices`** (all `CREATE INDEX IF NOT EXISTS`, all non-unique):
- `idx_invoices_monat` on `(jahr, monat)`
- `idx_invoices_datum` on `(rechnungsdatum)`
- `idx_invoices_auftraggeber` on `(auftraggeber)`
- `idx_invoices_rechnungsnummer` on `(rechnungsnummer)`
- `idx_invoices_dateiname` on `(dateiname)`
- `idx_invoices_belegnummer` on `(belegnummer)` — created **after** the ALTER-TABLE migrations, not in the base schema (see §7 for why).

**Trigger** `update_invoices_timestamp`: `AFTER UPDATE ON invoices FOR EACH ROW` sets `updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id`.

> Quirk: The DB column is `ustidnr` but every other layer (Meta, CSV, exports) calls this field `VATID`. The column is `verwendungszweck DEFAULT '-'`, and empty values are coerced to `"-"` at the application layer too.

#### 2.2 Table `audit_log`

Append-only audit trail.

| Column | Type | Default | Meaning |
|--------|------|---------|---------|
| `id` | INTEGER | PK AUTOINCREMENT | — |
| `ts` | DATETIME | `CURRENT_TIMESTAMP` | Event timestamp. |
| `aktion` | TEXT NOT NULL | — | `"create"`, `"update"`, `"delete"`, `"lock"`, `"unlock"`. |
| `entitaet` | TEXT | NULL | `"invoice"` or `"period"`. |
| `schluessel` | TEXT | NULL | Key. For invoices: `"<belegnummer> <dateiname>"`. For periods: `"<jahr>-<monat>"`. |
| `details` | TEXT | NULL | JSON field-diff for updates (see below); empty string for create/delete/lock/unlock. |

Index: `idx_audit_ts` on `(ts)`. Reads order newest-first: `ORDER BY ts DESC, id DESC`.

The **update diff** (`details`) is a JSON object containing only the changed fields, each as `{"alt":<old>,"neu":<new>}`. The diffable fields, compared in this order, are: `Auftraggeber`, `Rechnungsnummer`, `Rechnungsdatum`, `BetragNetto`, `SteuersatzBetrag`, `Bruttobetrag`, `Gegenkonto`, `Bankkonto`, `Bezahldatum`, `BuchungRef`, `Belegnummer`, `Ausgangsrechnung`. Comparison is by string-formatted value; if nothing changed the diff is the literal `"{}"`. Example: `{"Gegenkonto":{"alt":2000,"neu":4930},"Bankkonto":{"alt":"Sparkasse","neu":"Kasse"}}`.

Audit logging is **best-effort**: a failure to write the audit row logs a warning but never fails the Insert/Update/Delete/Lock.

#### 2.3 Table `period_locks`

GoBD period write-protection (*Festschreibung*).

| Column | Type | Default | Meaning |
|--------|------|---------|---------|
| `jahr` | TEXT NOT NULL | — | Year YYYY. |
| `monat` | TEXT NOT NULL | — | Month MM. |
| `locked_at` | DATETIME | `CURRENT_TIMESTAMP` | When locked. |
| — | — | `PRIMARY KEY(jahr, monat)` | One lock row per period. |

Operations:
- **LockPeriod(jahr, monat)**: `INSERT OR IGNORE` (idempotent), plus audit `lock`.
- **UnlockPeriod(jahr, monat)**: `DELETE`, plus audit `unlock`.
- **IsPeriodLocked**: `SELECT COUNT(*) > 0`.
- **LockedPeriods**: returns `"YYYY-MM"` strings ordered by `(jahr, monat)`.

Enforcement: `Insert` checks the row's period; `Delete` checks the row's period; `Update` checks **both** the old period `(jahr, monat)` **and**, if the row moves to a different month, the new period `(row.Jahr, row.Monat)`. Any locked period causes the mutation to fail with error `ErrPeriodLocked` whose message is the German string `"Periode ist festgeschrieben"`.

#### 2.4 Duplicate detection (no DB constraint)

Because there is no UNIQUE constraint, the app guards against duplicate import/insert with `IsDuplicate(jahr, monat, row)`, matching within the same filing period on:
```
LOWER(TRIM(auftraggeber)) = LOWER(TRIM(?))
AND rechnungsnummer = ?
AND rechnungsdatum = ?
AND ABS(bruttobetrag - ?) < 0.01
AND teilzahlung = ?
```
This is the only de-dup logic; it is case-insensitive and whitespace-insensitive on the company name, exact on invoice number and date, gross-amount-equal within 0.01, and partial-payment-flag-equal.

### 3. The Meta domain object and column mapping

`Meta` is the in-memory representation of one receipt as captured/edited in the UI. It is converted to `CSVRow` (`ToCSVRow`) for persistence and export, and back (`ToMeta`). The persisted fields and their DB columns:

| Meta field | Type | DB column | Notes |
|------------|------|-----------|-------|
| `Belegnummer` | string | `belegnummer` | `YYYY-NNNN`. |
| `Auftraggeber` | string | `auftraggeber` | |
| `Verwendungszweck` | string | `verwendungszweck` | `""` → `"-"` on conversion to CSVRow. |
| `Rechnungsnummer` | string | `rechnungsnummer` | |
| `VATID` | string | `ustidnr` | Counterparty VAT-ID. |
| `BetragNetto` | decimal | `betrag_netto` | Aggregate. |
| `SteuersatzProzent` | decimal | `steuersatz_prozent` | Aggregate (primary rate). |
| `SteuersatzBetrag` | decimal | `steuersatz_betrag` | Aggregate. |
| `Bruttobetrag` | decimal | `bruttobetrag` | |
| `TaxLines` | array | `steuerzeilen` (JSON) | Detail; aggregates are its sums. |
| `Trinkgeld` | decimal | `trinkgeld` | Untaxed tip. |
| `Waehrung` | string | `waehrung` | |
| `Rechnungsdatum` | string | `rechnungsdatum` | DD.MM.YYYY. |
| `Jahr` | string | `jahr` | |
| `Monat` | string | `monat` | |
| `Gegenkonto` | int | `gegenkonto` | |
| `Bankkonto` | string | `bankkonto` | Zahlungskonto name. |
| `Bezahldatum` | string | `bezahldatum` | DD.MM.YYYY. |
| `Teilzahlung` | bool | `teilzahlung` | |
| `Ausgangsrechnung` | bool | `ausgangsrechnung` | |
| `Dateiname` | string | `dateiname` | |
| `Kommentar` | string | `kommentar` | |
| `BewirtungAnlass` | string | `bewirtung_anlass` | |
| `BewirtungTeilnehmer` | string | `bewirtung_teilnehmer` | |
| `BetragNetto_EUR` | decimal | `betrag_netto_eur` | |
| `Gebuehr` | decimal | `gebuehr` | |
| `Rabatt` | decimal | `rabatt` | |
| `Wechselkurs` | decimal | `wechselkurs` | |
| `GebuehrProzent` | decimal | `gebuehr_prozent` | |
| `HatAnhaenge` | bool | `hat_anhaenge` | |
| `BuchungRef` | string | `buchung_ref` | |
| `Buchung` | object | `buchung` (JSON) | |
| `Exportiert` | bool | `exportiert` | |

**Transient Meta fields (NOT persisted to DB or CSV):**
- `KontoVorschlaege` (array of int) — AI-suggested Gegenkonten for unknown suppliers.
- `BarBezahlt` (bool) — receipt paid in cash; JSON tag `-`; not persisted.
- `Quelle` (string) — extraction source label (e.g. `"E-Rechnung"`, `"Claude (Text)"`, `"Lokal"`, `"Vision"`).

`CSVRow` carries a few extra **export-only / CSV-only** fields not stored in the DB: `AnzahlAnhaenge` (int, count of attachments), `Unterordner` (string: `""` | `"Bar"` | `"Ausgangsrechnungen"`, the category subfolder), `Originalwaehrung` (string) and `Originalbetrag_Brutto` (decimal) — documentation columns for foreign-currency receipts populated by the export layer.

### 4. TaxLine (`steuerzeilen`), Booking (`buchung`), and `buchung_ref`

#### 4.1 TaxLine — `steuerzeilen` JSON

A receipt's VAT detail is an array of tax lines. Each line:
```
{ "netto": <decimal>, "satz_prozent": <decimal>, "mwst_betrag": <decimal> }
```
- `netto` — net amount taxed at this rate.
- `satz_prozent` — VAT rate in percent (e.g. 19, 7, 0).
- `mwst_betrag` — VAT amount for this line.

Stored as **compact JSON** (no indentation). **An empty/zero array serialises to the empty string `""`, not `"[]"`.** Parsing `""` or invalid JSON yields *nil* (no lines).

Aggregates: `BetragNetto = Σ netto`, `SteuersatzBetrag = Σ mwst_betrag`, `Bruttobetrag = Σ netto + Σ mwst_betrag + Trinkgeld`, `SteuersatzProzent = satz_prozent of the first line` (0 if none).

**Legacy reconstruction** (for old rows with no `steuerzeilen`): from the aggregate fields, build one line `{netto, satz_prozent, mwst_betrag}`. If net, rate, and VAT are all 0 but gross > 0, build a single line with `netto = brutto` (so the total is preserved). If all four are 0, no lines.

Worked example: a €119 receipt at 19% VAT → `[{"netto":100,"satz_prozent":19,"mwst_betrag":19}]`, aggregates net=100, VAT=19, gross=119.

#### 4.2 Booking — `buchung` JSON

A balanced double-entry posting:
```
{
  "entries": [ { "konto": <int>, "betrag": <decimal>, "soll": <bool>, "steuerschluessel": "<string?>" }, ... ],
  "info": "<free-text rationale / Buchungswissen>",
  "manuell": <bool>
}
```
- Each entry: `konto` = account code; `betrag` = amount; `soll` = true → debit (Soll), false → credit (Haben); `steuerschluessel` = optional DATEV tax key (omitted when empty).
- `info` — free-text note ("Buchungswissen"); omitted when empty.
- `manuell` — true if hand-edited rather than auto-generated; omitted when false.

Serialised as compact JSON; an empty booking (no entries **and** empty info) serialises to `""`. Parsing `""`/invalid yields an empty booking. A booking is *Balanced* when it has ≥1 entry and `|ΣSoll − ΣHaben| < 0.005`. All booking amounts are rounded to 2 decimals (`round(v*100)/100`).

Worked example (incoming €119 invoice at 19%, expense account 6815, payment account 1800): entries = Soll 6815 net 100.00, Soll 1406 (Vorsteuer 19%) 19.00, Haben 1800 119.00 → ΣSoll 119.00 = ΣHaben 119.00.

#### 4.3 `buchung_ref` wire format

`buchung_ref` links a receipt to a specific booking line on a bank-statement PDF. Wire format: **`<statementFilename>|<page>|<lineIdx>`** where `page` is **0-based** and `lineIdx` is **1-based**. Empty string = not linked. Parsing requires exactly 3 `|`-separated parts with integer page/line; anything malformed parses to the zero value (no link) — it never fails loudly, because legacy CSVs may contain garbage here.

- Display form: `"<filename> · S.<page+1> Z.<lineIdx>"` (e.g. `Konto_...0002.pdf · S.1 Z.3`).
- **Sentinel `CashConfirmedRef = "kassenbuch|0|0"`**: marks a cash-register receipt as confirmed against the generated Kassenbuch. Because any non-empty `buchung_ref` renders as ✓ in the invoice table, this sentinel gives cash receipts the same visual "matched" treatment without a new DB column.

### 5. Settings object (all ~46 fields)

Settings persist as pretty-printed (2-space indent) JSON at `<profileConfigDir>/settings.json`. A missing file yields defaults; a parse failure also yields defaults (with an error logged). Grouped:

**Storage & filenames**
| JSON key | Type | Default | Meaning |
|----------|------|---------|---------|
| `storage_root` | string | `""` → `Documents/BuchISY` on first run | Root of the document tree. |
| `scan_inbox_folder` | string | `""` | Inbox folder for scanned PDFs. |
| `use_month_subfolders` | bool | `true` | Organise into `YYYY/YYYY-MM`. |
| `naming_template` | string | `${YYYY}-${MM}-${DD}_${Company}_${Kurzbez8}_${InvoiceNumber}_${Currency}_${GrossAmount}.pdf` | Filename template. |
| `last_used_folder` | string | `""` | Last folder for Belege/attachments. |
| `last_statement_folder` | string | `""` | Last folder for Kontoauszüge. |

**Processing / extraction**
| Key | Type | Default | Meaning |
|-----|------|---------|---------|
| `processing_mode` | string | `"claude"` | `"claude"` or `"local"`. |
| `anthropic_model` | string | `"claude-sonnet-4-6"` | Model id. |
| `anthropic_api_key_ref` | string | `"claude"` | Keyring **account name suffix** (see §6). |
| `language` | string | `"de"` | UI language. |
| `decimal_separator` | string | `","` | Decimal separator for display/CSV. |
| `currency_default` | string | `"EUR"` | Default currency. |
| `own_vat_id` | string | `""` | The user's own VAT-ID(s); excluded during auto-extract. |
| `debug_mode` | bool | `false` | Verbose logging. |

**Accounts (Gegenkonten)**
| Key | Type | Default | Meaning |
|-----|------|---------|---------|
| `default_account` | int | `2000` | Default Gegenkonto. |
| `accounts` | array of `{code:int,label:string}` | `[{2000,"Ausgaben"}]` | User account list. |
| `remember_company_account` | bool | `true` | Persist company→account memory. |
| `auto_select_account` | bool | `true` | Auto-pick known company's account. |

**Payment accounts (Zahlungskonten)**
| Key | Type | Default | Meaning |
|-----|------|---------|---------|
| `default_bank_account` | string | `"Sparkasse"` | Default payment account name. |
| `default_bank_account_iban` | string | `""` | IBAN of the default. |
| `bank_accounts` | array of BankAccount | `[{Name:"Sparkasse",AccountType:"bank"}]` | All payment accounts. |

Each **BankAccount**: `name` (string), `iban` (string), `account_type` (`"bank"` \| `"creditcard"` \| `"cash"` \| `"payroll"`), `settlement_account` (string; account that settles a credit card monthly), `skr04_konto` (int, omitempty), `is_credit_card` (bool — **legacy migration flag only**). On load, `normalizeBankAccounts` assigns a valid `account_type` to every account: keep if already valid; otherwise `"creditcard"` if the legacy `is_credit_card` was set, else `"bank"`; then clear `is_credit_card`.

**Payment-account → Haben account mapping** (`PaymentAccountSKR04`): an explicit `skr04_konto` wins; otherwise by type: `bank → 1800`, `cash → 1600`. Credit-card and payroll have **no default** and require an explicit `skr04_konto`; otherwise returns "no mapping".

> Quirk: `AccountTypePayroll` ("payroll") is a receipt advanced by an employee, reimbursed via payroll. Its Haben side posts to a clearing account (`skr04_konto`, e.g. *Verrechnungskonto Lohn u. Gehalt*) instead of a bank/cash account, and such receipts are **excluded from bank-statement reconciliation**.

**Booking-rule / DATEV identifiers**
| Key | Type | Default | Meaning |
|-----|------|---------|---------|
| `datev_berater_nr` | string (omitempty) | `""` | DATEV consultant number. |
| `datev_mandant_nr` | string (omitempty) | `""` | DATEV client number. |
| `datev_wj_beginn` | string (omitempty) | `""` | Fiscal-year start, **YYYYMMDD**. |

**Reconciliation**
| Key | Type | Default | Meaning |
|-----|------|---------|---------|
| `matchDateWindowDays` | int (omitempty) | `0` (= use built-in default) | Date window in days for statement matching. |
| `matchForeignTolerancePct` | float (omitempty) | `0` (= default) | FX tolerance % for matching. |

**CSV format**
| Key | Type | Default | Meaning |
|-----|------|---------|---------|
| `csv_separator` | string | `","` | `,` \| `;` \| `\t`. |
| `csv_encoding` | string | `"ISO-8859-1"` | `ISO-8859-1` or `UTF-8`. |
| `column_order` | array of string | `DefaultCSVColumns` (see below) | Column order in table & CSV. |

`DefaultCSVColumns` (full ordered list of 36): `Belegnummer, Dateiname, Rechnungsdatum, Jahr, Monat, Auftraggeber, Verwendungszweck, Rechnungsnummer, VATID, BetragNetto, Steuersatz_Prozent, Steuersatz_Betrag, Bruttobetrag, Waehrung, Gegenkonto, Bankkonto, Bezahldatum, Teilzahlung, Ausgangsrechnung, Kommentar, BewirtungAnlass, BewirtungTeilnehmer, BetragNetto_EUR, Gebuehr, Rabatt, Wechselkurs, GebuehrProzent, HatAnhaenge, AnzahlAnhaenge, Unterordner, BuchungRef, Trinkgeld, Steuerzeilen, Buchung, Exportiert, Originalwaehrung, Originalbetrag_Brutto`.

**Window / UI / advanced**
| Key | Type | Default | Meaning |
|-----|------|---------|---------|
| `window_width` | int | `1500` | px. |
| `window_height` | int | `875` | px. |
| `window_x` | int | `-1` (= center) | px. |
| `window_y` | int | `-1` (= center) | px. |
| `dialog_width` | int | `850` | Invoice-dialog width px. |
| `dialog_height` | int | `700` | Invoice-dialog height px. |
| `ui_scale` | float | `1.0` | UI zoom (1.0 = 100%). Coerced to 1.0 if ≤ 0 at startup. |
| `preview_split_offset` | float | `0.33` | Divider position (0..1) in the confirmation window. |

### 6. On-disk layout

**OS config dir** (`GetConfigDir`):
- Windows (env `APPDATA` set): `%APPDATA%\BuchISY`
- macOS / Linux (default branch): `~/Library/Application Support/BuchISY`

> Quirk: there is no genuine Linux-specific branch. On Linux without `APPDATA`, the config dir is `~/Library/Application Support/BuchISY` (the macOS path), because the switch falls through to the default. A faithful port should replicate this rather than use XDG.

**Profiles.** A profile's config dir is `<configDir>/profiles/<name>` (`GetProfileConfigDir`). `ListProfiles` returns the subdirectory names under `<configDir>/profiles` (a missing `profiles` dir → empty list, not an error). Per profile, the config dir contains:

| Path | Content |
|------|---------|
| `profiles/<name>/settings.json` | The Settings object (pretty JSON, 2-space indent, 0644). |
| `profiles/<name>/invoices.db` | The profile's SQLite database (the **global** DB). |
| `profiles/<name>/logs/` | Log files. |
| `profiles/<name>/company_accounts.json` | Map of **normalized company name → account code** (pretty JSON). Loaded/saved by `CompanyAccountMap`. |
| `profiles/<name>/chart_skr04.json` | Chart-of-accounts override; if absent, the bundled SKR04 asset (`assets.SKR04JSON`) is used. |
| `profiles/<name>/buchungsregeln.json` | Booking-rules override; if absent, the bundled defaults (`assets.BuchungsregelnJSON`) are used. |
| `profiles/<name>/` (account-prefs / statement-alias stores) | Additional per-profile JSON stores loaded at startup (`NewAccountPrefs(configDir)`, `NewStatementAliasStore(configDir)`). |

**Window/UI sort preferences** are stored via the Fyne `Preferences` mechanism (app-global, not in settings.json): keys `ui_scale` (float), `konten_sort_col`/`konten_sort_asc`, `invoice_sort_col`/`invoice_sort_asc`.

**Document storage tree** under `storage_root` (when `use_month_subfolders` = true):
```
<storage_root>/
  YYYY/
    YYYY-MM/
      <receipt>.pdf                         (incoming receipts; main + _AnhangN attachments)
      Bar/                                  (cash-paid receipts; Unterordner="Bar")
      Ausgangsrechnungen/                   (outgoing invoices; Unterordner="Ausgangsrechnungen")
      invoices.csv                          (legacy/export CSV per month)
      kassenbuch.json                       (cash books for this month — see below)
  <PaymentAccountFolder>/                   (per Zahlungskonto: scanned bank-statement PDFs)
    metadata.json                           (per-statement metadata, keyed by relative path)
```
When `use_month_subfolders` = false, the month folder is just `storage_root` itself. Invoice main-file path = `<monthFolder>/<Unterordner?>/<dateiname>`. File moves use collision-safe naming: on name clash, append `_2`, `_3`, … before the extension.

**`kassenbuch.json`** (per month folder, pretty JSON): an array of CashBook objects:
```
[ { "konto": "<cash account name>",
    "anfangsbestand": <decimal>,
    "einlagen": [ { "datum": "DD.MM.YYYY", "beschreibung": "<text>", "betrag": <decimal> }, ... ] } ]
```
A missing file = empty slice. The running cash report combines `anfangsbestand` + deposits + cash-paid invoices (ordered by `bezahldatum`, falling back to `rechnungsdatum`; unparseable dates sorted last); each invoice is an *Ausgabe* of its `bruttobetrag`; deposits are *Einnahme*; the running `saldo` carries forward. Invoices booked while the running balance is negative (< −0.005) are flagged "uncovered".

**`metadata.json`** (per payment-account folder, pretty JSON): a map `"<relative path>" → StatementMetadata`:
```
{ "2026/Auszug-05.pdf": {
    "date_from": "DD.MM.YYYY", "date_to": "DD.MM.YYYY",
    "number": "5/2026",
    "opening_balance": <decimal>, "closing_balance": <decimal>,
    "reviewed": <bool>, "note": "<text>",
    "bookings_parsed_mtime": <unix seconds>,
    "bookings": [ <StatementBooking>, ... ] } }
```
`bookings_parsed_mtime` caches the source PDF's mtime: the statement parser only re-runs when the file's mtime changes; existing invoice-link annotations are preserved by matching on `(page, lineIdx)`.

**Keychain.** The Anthropic API key is stored in the OS keyring under **service name `"BuchISY"`** and account name **`<profile>-<anthropic_api_key_ref>`** (e.g. `"acme-claude"`). The key is read on demand; `hasAPIKey()` = a non-empty value exists for that account.

### 7. Startup migrations to reproduce

These run (in this order) when a profile starts, after the DB is opened and the CSV repo configured. All are idempotent.

**(a) Idempotent ALTER-TABLE column-add pattern** (inside `initSchema`, every DB open). After running the base `schemaSQL`, the app executes a fixed list of `ALTER TABLE invoices ADD COLUMN …` statements:
```
trinkgeld REAL DEFAULT 0
steuerzeilen TEXT DEFAULT ''
buchung TEXT DEFAULT ''
exportiert INTEGER DEFAULT 0
wechselkurs REAL DEFAULT 0
gebuehr_prozent REAL DEFAULT 0
rabatt REAL DEFAULT 0
buchung_ref TEXT DEFAULT ''
belegnummer TEXT DEFAULT ''
ausgangsrechnung INTEGER DEFAULT 0
bewirtung_anlass TEXT DEFAULT ''
bewirtung_teilnehmer TEXT DEFAULT ''
```
Each ADD COLUMN is run unconditionally; an error is ignored **only** if it contains the substring `"duplicate column name"` (i.e. the column already exists). Any other error aborts schema init. **Then**, and only then, the `idx_invoices_belegnummer` index is created. The index on `belegnummer` is deliberately created here, after the ALTERs, **not** in the base schema: an old pre-belegnummer database already has an `invoices` table, so `CREATE TABLE IF NOT EXISTS` is a no-op and the column would not exist yet; creating the index in the base schema would fail with "no such column: belegnummer". A re-implementer must keep this ordering. The `List` read path is also NULL-safe for older DBs whose columns were added without a default.

**(b) `MigrateToYearFolders`** (storage). No-op unless `use_month_subfolders` is true. Scans entries directly under `storage_root`; any directory whose name matches `^(\d{4})-(\d{2})$` (a bare `YYYY-MM` month folder) is moved into a `YYYY/` year folder (`<root>/<YYYY>/<YYYY-MM>`). If the target already exists, it is skipped with a warning. Idempotent: a second run finds nothing to move.

**(c) `MigrateCashToBar`** (storage). No-op unless `use_month_subfolders` is true and there is ≥1 cash account. Builds the set of cash-account names from `bank_accounts` where `account_type = "cash"`. For every `invoices.csv` found, for rows whose `Unterordner` is empty and whose `Bankkonto` is a cash account: it first **probes** that the CSV is writable (rewrites it) before moving any file (so a write failure cannot leave files relocated but the CSV stale); then moves each such receipt into a `Bar/` subfolder of its month folder (collision-safe `_2`, `_3` naming), sets the row's `Dateiname` to the final name and `Unterordner = "Bar"`, and rewrites the CSV. Rows that already have a non-empty `Unterordner` are skipped — idempotent.

**(d) `MigrateCSVToDatabase`** (db). Back-fills the SQLite DB from legacy per-month `invoices.csv` files **only when the database is still empty** (`Count() == 0`); otherwise a cheap no-op. It walks `storage_root` for files named exactly `invoices.csv`, imports each via `ImportFromCSV` (which uses `IsDuplicate` to skip rows already present — import is idempotent), and logs the totals. This exists so a profile migrated from the pre-SQLite, CSV-only era is not shown an empty table.

### Re-implementation checklist

- **One SQLite database per profile** at `<configDir>/profiles/<name>/invoices.db` (the "global" DB). Ignore the unused per-month `GetDBPath`.
- `invoices` table: 36 columns in the exact order/types/defaults listed; `verwendungszweck DEFAULT '-'` (and coerce `""`→`"-"` in code); DB column `ustidnr` maps to logical field `VATID`. **No UNIQUE constraint anywhere.**
- Six non-unique indexes plus `idx_invoices_belegnummer`; create the belegnummer index **after** the ALTER-TABLE column adds, never in the base schema.
- `update_invoices_timestamp` trigger sets `updated_at` on every UPDATE.
- `audit_log` (newest-first, `ORDER BY ts DESC, id DESC`), `period_locks` (PK `(jahr,monat)`). Lock blocks Insert/Delete on the row's period and Update on **both** old and new periods; error message `"Periode ist festgeschrieben"`. Audit logging is best-effort.
- Audit invoice key = `"<belegnummer> <dateiname>"`; period key = `"<jahr>-<monat>"`; update diff = JSON of changed fields only `{"Field":{"alt":…,"neu":…}}`, `"{}"` when none, comparing exactly the 12 listed fields.
- Duplicate detection only in code: same `(jahr,monat)`, case/trim-insensitive `auftraggeber`, exact `rechnungsnummer`+`rechnungsdatum`, gross within 0.01, equal `teilzahlung`.
- `steuerzeilen` = compact JSON array `{netto,satz_prozent,mwst_betrag}`; empty → `""` (not `"[]"`). `buchung` = compact JSON `{entries:[{konto,betrag,soll,steuerschluessel?}],info?,manuell?}`; empty → `""`. All booking amounts rounded to 2 decimals; balanced within 0.005.
- `buchung_ref` = `filename|page|lineIdx` with **0-based page, 1-based lineIdx**; malformed → no link (silent). Sentinel `kassenbuch|0|0` marks confirmed cash receipts.
- Settings = pretty JSON `settings.json` per profile, 46 fields with exact keys/defaults; on load normalize `bank_accounts` types and migrate `is_credit_card`; default storage root `Documents/BuchISY`; `ui_scale ≤ 0 → 1.0`.
- Config dir: `%APPDATA%\BuchISY` on Windows else `~/Library/Application Support/BuchISY` (no XDG branch). Profiles under `profiles/<name>/`.
- Side-channel JSON: `company_accounts.json` (config dir), `chart_skr04.json` + `buchungsregeln.json` (config dir, fall back to bundled assets), per-month `kassenbuch.json`, per-payment-account `metadata.json` keyed by relative path.
- Keychain service `"BuchISY"`, account `"<profile>-<anthropic_api_key_ref>"`.
- Reproduce all four startup migrations in order, all idempotent: idempotent ALTER-TABLE (ignore only "duplicate column name"), `MigrateToYearFolders` (`^\d{4}-\d{2}$` → `YYYY/`), `MigrateCashToBar` (cash receipts → `Bar/`, write-probe before move), `MigrateCSVToDatabase` (only when DB empty).

---

## Capture & Extraction

This chapter specifies how BuchISY ingests source documents (invoices/receipts, and to a lesser extent bank statements) and turns them into a structured `Meta` record that pre-fills the booking-confirmation form. It documents every ingest path, the exact extraction **priority chain**, every extractor's selection conditions, formats, prompts, regex rules, confidence values, and the post-extraction account suggestion.

### 1. Ingest paths

All ingest paths converge on the same entry function `processSubmission(mainPath, attachments, onComplete)` (single document) or `enqueueSubmissions(paths)` (batch). The four ways a document enters the pipeline:

#### 1.1 Drag-and-drop (OS file drop on the window)

- Handler fires for a list of dropped file URIs.
- Behavior is **view-mode dependent**:
  - **Belege (invoices) mode**: every dropped path whose basename passes `IsSupportedFile` is collected, then `enqueueSubmissions(paths)` queues them all for **sequential** review.
  - **Konten (accounts) mode**: only the **first** supported file is filed as a bank statement (`fileStatement(path)`) for the currently selected Zahlungskonto; the rest are ignored.

#### 1.2 Clipboard paste (Ctrl+V shortcut or context-menu "Einfügen")

`pasteFromClipboard()` handles two clipboard contents, in this order:

1. **File paths** (e.g. Explorer "Copy"): iterate `clipboardFiles()`; the **first** entry whose basename passes `IsSupportedFile` is processed via `processSubmission(f, nil, nil)` and the loop returns.
2. **Raw bitmap image** (Snipping Tool, screenshot, browser image): `clipboardImageDiagnostic()` returns a decoded `image.Image`. It is **PNG-encoded** into a temp file named `buchisy-paste-*.png` (Go `os.CreateTemp` pattern, random infix), then that temp PNG is processed.
3. If neither yields anything, an info dialog lists the available clipboard formats (`clipboardFormatsDiagnostic()`) and tells the user to copy a file or image.

> Quirk: Clipboard file/image reading is **Windows-only**. On all other platforms `clipboardFiles()`, `clipboardImage()`, and `clipboardImageDiagnostic()` are no-ops returning `nil`/`"(not supported)"`. Windows reading uses Win32 directly: `OpenClipboard` with brief retry, `GetClipboardData(CF_HDROP=15)` for files, and DIB formats for images.

#### 1.3 File / batch picker

- `selectPDFFiles()` → `showCustomFilePicker()` (a custom search-enabled picker) for invoices; the picked paths go to `enqueueSubmissions`.
- `showFilesPickerFor(kind, ...)` / `showFilePickerFor(kind, ...)`: native multi/single-file dialog on Windows (`nativePickerAvailable()`), Fyne fallback (single-file only) elsewhere.
- The picker remembers a **per-profile, per-kind "last used folder"**: `pickerBeleg` ↔ `settings.LastUsedFolder`; `pickerKontoauszug` ↔ `settings.LastStatementFolder`. The remembered folder is the **directory of the chosen file**, persisted only when changed.

#### 1.4 Watched scan inbox

A background `scanWatcher` polls `settings.ScanInboxFolder` **every 5 seconds**:

- Only files with extension `.pdf` (case-insensitive) are considered.
- **Stability gate** (`scanFileReady`): a file is dispatched only when it was **seen on a previous poll with an identical byte size** AND not already handled this session. This avoids ingesting a half-written scan. Formally: `seenBefore && !handled && prevSize == curSize`.
- At most **one** file is dispatched per poll, and only when not already `busy`. The watcher sets `busy=true` and marks the path `handled` before dispatching `processSubmission(candidate, nil, onScanDone)`; `onScanDone` clears `busy` so the next file can proceed.
- The setting is read on the UI thread (`fyne.DoAndWait`) to avoid a data race with settings saves.

#### 1.5 Supported file types

`IsSupportedFile` (invoice main file or attachment) accepts these extensions (lower-cased): `.pdf .doc .docx .xls .xlsx .ppt .pptx .odt .ods .odp .jpg .jpeg .png .gif .bmp .tif .tiff .webp .heic .svg`.

`IsPDF`: extension `== .pdf`.

`ImageMediaType` (decides whether a non-PDF goes through Claude image-vision) returns a Claude-compatible media type **only** for: `.jpg/.jpeg → image/jpeg`, `.png → image/png`, `.gif → image/gif`, `.webp → image/webp`. Returns `""` for everything else (so `.bmp/.tif/.heic/.svg/.doc…` are accepted as files but are **never** auto-extracted — they open a blank form).

### 2. Routing in `processSubmission`

Given the main file path and `settings.ProcessingMode` (`"claude"` or `"local"`):

1. **PDF** (`IsPDF` true) → `extractPDFData(ctx, path)` (the priority chain, §3). On success → confirmation modal pre-filled. On error `"no text found in PDF"` → `handleNoTextPDF` (asks user whether to enter manually). Other errors → error dialog.
2. **Non-PDF image** (`ImageMediaType != ""`) **and** `ProcessingMode == "claude"` → `extractImageData(ctx, path)` (Claude Vision on the raw image bytes). Any failure → fall back to a **blank** `Meta{Waehrung: settings.CurrencyDefault, Gegenkonto: settings.DefaultAccount}`.
3. **Any other non-PDF** (or image while in `local` mode) → open the confirmation modal with the blank `Meta` (no extraction).

After a successful PDF extraction, an **auto-booking** short-circuit may apply: if a `MatchAutobookRule(meta.Auftraggeber, …)` matches, `AutobookPlausible(meta)` is true, and `FindDuplicate` finds no duplicate, the invoice is booked silently without showing the modal (covered in the Booking chapter, not here).

### 3. Extraction priority chain (PDF) — `extractPDFData`

This is the core ordering a re-implementation must reproduce **exactly**:

```
STEP 1  E-invoice (XRechnung / ZUGFeRD CII)   → if detected & parse OK: return, confidence 1.0
STEP 2  PDF text extraction
          if HasText(text):
             if ProcessingMode == "claude": Claude multimodal (text + page images)  conf 0.95
                                            (fallback: Claude text-only)             conf 0.90
             else:                          local regex extraction                  conf = matched/total
          else (no meaningful text):
             if ProcessingMode == "claude": Claude Vision on first page             conf 0.95
             else:                          error "no text found in PDF"
```

Each branch stamps a transient `meta.Quelle` label: `"E-Rechnung"`, `"Claude (Text)"`, `"Lokal"`, or `"Vision"`. (Note: the multimodal and vision paths both label `"Claude (Text)"` / `"Vision"` respectively per the code; multimodal returns through the same block that sets `"Claude (Text)"`.)

#### 3.1 STEP 1 — E-invoice detection & extraction

**Detection** (`DetectFormat`): attempt to extract the first XML attachment from the PDF. If none, not an e-invoice. Format is decided by:

1. **Attachment filename** (lower-cased): contains `factur-x` or `zugferd` → `ZUGFeRD`; contains `xrechnung` → `XRechnung`.
2. **XML content** (if filename inconclusive): contains `xrechnung` or `urn:cen.eu:en16931` → `XRechnung`; contains `zugferd` or `urn:ferd:` → `ZUGFeRD`.
3. **Fallback**: XML present but unclassifiable → `ZUGFeRD` (assumed more common).

**XML location** (`extractXMLAttachment`): uses the **pdfcpu** library `ExtractAttachmentsFile` to extract **all** embedded files (PDF/A-3 attachments) into a temp dir, then picks the XML:

- First match any file whose lower-cased name **contains** one of: `factur-x.xml`, `zugferd-invoice.xml`, `xrechnung.xml` (and `zugferd-invoice.xml` again — duplicate in the list).
- Otherwise the first file ending in `.xml`.
- The temp dir is removed after reading.

> Quirk: **Only CII (Cross-Industry Invoice) XML is parsed. UBL is not supported.** The XML is unmarshalled into a `CrossIndustryInvoice` root; a UBL XRechnung would extract as XML, be format-detected, but `parseXML` would yield empty fields (no error), producing an essentially blank `Meta` at confidence 1.0.

**Field mapping** (CII → `Meta`), exact XML paths:

| Meta field | CII source | Transform |
|---|---|---|
| `Rechnungsnummer` | `ExchangedDocument/ID` | as-is |
| `Rechnungsdatum` | `ExchangedDocument/IssueDateTime/DateTimeString` (value, `YYYYMMDD`) | → `DD.MM.YYYY` |
| `Jahr` | first 4 chars of the `YYYYMMDD` value | — |
| `Monat` | chars 5–6 of the `YYYYMMDD` value | — |
| `Auftraggeber` | `…/ApplicableHeaderTradeAgreement/SellerTradeParty/Name` | as-is (always the **seller**) |
| `Waehrung` | `…/ApplicableHeaderTradeSettlement/InvoiceCurrencyCode` | as-is |
| `BetragNetto` | `…/SpecifiedTradeSettlementHeaderMonetarySummation/TaxBasisTotalAmount` | parse to decimal |
| `Bruttobetrag` | `…/SpecifiedTradeSettlementHeaderMonetarySummation/GrandTotalAmount` | parse to decimal |
| `SteuersatzProzent` | `ApplicableTradeTax[0]/RateApplicablePercent` | parse to decimal |
| `SteuersatzBetrag` | `ApplicableTradeTax[0]/CalculatedAmount` | parse to decimal |
| `Bezahldatum` | `SpecifiedTradePaymentTerms/DueDateDateTime/DateTimeString` (optional) | → `DD.MM.YYYY` |
| `Verwendungszweck` | literal `"Rechnung " + Rechnungsnummer` | — |

Date conversion `YYYYMMDD → DD.MM.YYYY`: only applied when the trimmed value is **exactly 8 chars**, else returned unchanged. Example: `20250131` → `31.01.2025`.

Amount parse: trim, replace `,` with `.`, scan as float (European-format tolerant).

> Quirk: Only the **first** `ApplicableTradeTax` entry is mapped to the legacy rate/amount fields; multi-rate CII invoices lose the other rates (no `TaxLines` are built from CII — `TaxLines` stays empty on this path). `Auftraggeber` is **always** the seller, even for an outgoing invoice.

**Confidence: always `1.0`** for any successfully parsed e-invoice.

#### 3.2 STEP 2 — PDF text extraction

`PDFTextExtractor.ExtractText` uses the **ledongthuc/pdf** library:

- Iterates pages 1..N; null pages skipped; each page's `GetPlainText` is concatenated with a trailing `\n`. Per-page errors are swallowed (continue with remaining pages).
- **Post-processing quirk**: the Unicode replacement char `U+FFFD` ("�") is globally replaced with `-` (hyphen), because some fonts fail to decode a separator glyph in receipt numbers (e.g. `MC9C7PFZ�103052` → `MC9C7PFZ-103052`).

`HasText(text)`: returns true iff the **trimmed** text length `> 10` characters. This is the gate that decides text-vs-vision.

#### 3.3 STEP 2a — Claude text / multimodal (when `HasText` true and `ProcessingMode == "claude"`)

The app renders **all pages** to base64 PNG (`PDFAllPagesToBase64`) and:

- If rendering succeeds and ≥1 image: **multimodal** extraction (`ExtractMultimodal`) — sends the extracted text **plus** every page image. **Confidence `0.95`.** Quelle stamped `"Claude (Text)"`.
- If rendering fails or yields 0 images: **text-only** extraction (`Extract`). **Confidence `0.90`.**

Rationale: POS / SumUp / restaurant receipts often render their VAT table as an image the text layer can't read; the page images let Claude read both.

#### 3.4 STEP 2b — Local regex extraction (when `HasText` true and `ProcessingMode == "local"`)

See §5. Confidence is a fraction (matched fields / 4).

#### 3.5 STEP 3 — Claude Vision (when `HasText` false and `ProcessingMode == "claude"`)

`extractPDFWithVision`: renders the **first page only** (`PDFToImageBase64`) to PNG, sends one image to Claude (`ExtractFromImage`). **Confidence `0.95`** (logged comment says "assume 0.95"; the function literal returns `0.95`). Quelle `"Vision"`.

> Quirk: In `local` mode a no-text PDF returns the error `"no text found in PDF"`, which triggers the manual-entry confirm dialog. There is **no local OCR fallback**.

### 4. Claude integration (Anthropic Messages API)

#### 4.1 Client & transport

- Endpoint: `POST https://api.anthropic.com/v1/messages`.
- Headers: `Content-Type: application/json`, `x-api-key: <key>`, `anthropic-version: 2023-06-01`.
- The API key is read from the OS keyring: service `"BuchISY"`, account = `keyringAccount()` (per profile).
- Request body fields: `model`, `max_tokens: 4096`, `messages: [{role:"user", content:…}]`, `system: <systemPrompt>`. Temperature is **not** set (server default).
- HTTP timeout **60 s**. **Retry**: up to `maxRetries = 3` attempts; on retry, sleep `retryDelay(2s) × attempt` (i.e. 2 s before attempt #2, 4 s before #3). Retries **only** on `APIError` with status **429** or **≥500**; all other errors fail immediately.
- Response: text is taken from `response.content[0].text`. Error responses parse `{error:{type,message}}` into an `APIError{StatusCode, Type, Message}`.

#### 4.2 Model

Default `settings.AnthropicModel = "claude-sonnet-4-6"` (user-editable in settings). 

> Quirk: One fallback in `visionHighlight` (the amount-locator, §7) defaults to `"claude-opus-4-5"` when the model setting is empty — inconsistent with the global `claude-sonnet-4-6` default. The main extraction paths always pass `settings.AnthropicModel` directly with no fallback.

#### 4.3 Message shapes

- **Text** (`Send`): user `content` is the plain text string.
- **Image** (`SendWithImage`): user `content` is `[{type:"image", source:{type:"base64", media_type, data}}, {type:"text", text}]`.
- **Multi-image** (`SendWithImages`): user `content` is N image blocks (in page order) followed by **one** text block. Used by multimodal extraction, statement extraction, and the amount-locator.

#### 4.4 Text preprocessing (10 000-char cap)

Before a text request, `preprocessText(text, 10000)`:

- If `len(text) ≤ 10000`: unchanged.
- Else: split into lines; a line is "priority" if its lower-cased form **contains** any keyword in: `rechnung, invoice, datum, date, netto, net, brutto, gross, mwst, ust, vat, total, gesamt, rechnungsnr, invoice no, betrag, amount, eur, €, usd, $`. All priority lines are concatenated first; then non-priority lines are appended until the 10 000-char budget is hit (last block truncated mid-string). Final result is hard-capped to 10 000 chars.

> Quirk: the byte length (`len`) is used, not rune count — multi-byte UTF-8 (umlauts, €) count as >1 toward the cap, and truncation can split a multi-byte rune.

#### 4.5 Extraction system prompt & JSON schema (invoice)

The base system prompt (German) instructs Claude to return **only** a strict JSON object with exactly these keys (German snake-case):

```json
{
  "auftraggeber": "string|null",
  "verwendungszweck": "string|null",
  "rechnungsnummer": "string|null",
  "vat_id": "supplier VAT-ID|null",
  "steuerzeilen": [{"satz": 0.0, "netto": 0.0, "mwst": 0.0}],
  "trinkgeld": 0.0,
  "bruttobetrag": 0.0,
  "waehrung": "EUR|USD|other ISO4217|null",
  "rechnungsdatum": "dd.MM.yyyy|null",
  "jahr": "YYYY|null",
  "monat": "MM|null",
  "bezahldatum": "dd.MM.yyyy|null",
  "bar_bezahlt": false,
  "ausgangsrechnung": false
}
```

Key rules embedded in the prompt (must be reproduced for identical behavior):

- Respond **only** with JSON, no prose. If unsure → `null`, do not guess.
- `auftraggeber` = always the **business partner**, never the app user. For an incoming invoice that's the issuer (vendor); for an outgoing invoice (`ausgangsrechnung=true`) it's the **recipient/customer**.
- `rechnungsnummer`: also accept labels "Belegnummer", "Beleg-Nr.", "Quittungsnummer", "Bon-Nr.", "Receipt No.", "Transaktionsnummer".
- `rechnungsdatum`: prefer the field near "Rechnung/Rechnungsdatum/Datum"; normalize to `dd.MM.yyyy`.
- `steuerzeilen`: one line per VAT rate (`satz` %, `netto`, `mwst`); exactly one line if single rate. `trinkgeld` separate, no VAT, 0 if none.
- `bruttobetrag` = total. Use **dot** as decimal separator in JSON, strip thousands separators. Consistency check encouraged: `sum(netto)+sum(mwst)+trinkgeld ≈ brutto`.
- `waehrung`: ISO code; `€ ⇒ EUR`.
- `verwendungszweck`: short human summary (≤ ~80 chars), e.g. "Cloud-Abo Oktober 2025".
- `bezahldatum`: set **only** when the document explicitly evidences a completed payment (cash receipt, "bar bezahlt", "bezahlt am", Kassenbon, EC-Zahlung, …); use the payment/receipt date. Otherwise `null`.
- `bar_bezahlt`: true only if paid in **cash** (bar, Bargeld, Kassenbon, Quittung, Cash); false for transfer/EC/credit card/PayPal/unknown.
- `vat_id`: the **issuer's** USt-IdNr.; examples of valid forms `DE123456789`, `ATU12345678`, `FR12345678901`, `GB123456789`. A plain Steuernummer like `112/197/12644` is **not** a VAT-ID.

**Own-VAT-ID exclusion** (`systemPromptFor(ownVATIDs)`): when the user has configured own VAT-IDs (`settings.OwnVATID`, split on `, ; \n \r`), two extra blocks are appended:

1. `=== AUSGANGSRECHNUNG-ERKENNUNG ===`: `ausgangsrechnung = true` iff the **issuer** carries one of the user's own VAT-IDs (listed); otherwise false; false if no own VAT-ID visible.
2. `=== STRENG VERBOTEN ===`: the listed own VAT-IDs must **never** be returned as `vat_id`; if the only visible VAT-ID is one of them, return `null`.

**Account-hint section** (`accountHintSection`, appended when account hints set): a `=== VERFÜGBARE GEGENKONTEN (SKR04) ===` block listing `"<number> <name>"` lines and instructing Claude to additionally return 1–3 best-fitting account numbers as `gegenkonto_vorschlaege` (JSON array of numbers, best first; empty array if none). Empty section (no token cost) when no hints.

The vision/multimodal user messages are short German prompts:
- Vision (`ExtractFromImage`): `"Bitte extrahiere die Rechnungsinformationen aus diesem Dokument."`
- Multimodal (`ExtractMultimodal`): `"Dokumenttext (kann unvollständig sein — Tabellen/Beträge stehen oft nur in den beigefügten Seitenbildern, dann diese auswerten):\n\n" + text`.

#### 4.6 Response parsing & cleanup (`parseExtractionJSON`)

1. `cleanJSONResponse`: trim; strip leading ```` ```json ```` / ```` ``` ```` fences (and trailing ```` ``` ````); if the result still doesn't start with `{`, fall back to the substring between the **first** `{` and the **last** `}`. This salvages responses where Claude narrated before the JSON.
2. Unmarshal into a struct with nullable pointers for each field plus `gegenkonto_vorschlaege` as `[]int`.
3. Mapping into `Meta`:
   - `Verwendungszweck` is passed through `NormalizeVerwendungszweck` (replace `&` → `und`, see §6).
   - `vat_id`: trimmed; if `isOwnVATID(val, ownVATIDs)` (defense-in-depth, see normalization §8) → set to `""`.
   - `steuerzeilen[]` → `TaxLines[]` (each: `Netto`, `SatzProzent`, `MwStBetrag`).
   - `BetragNetto = SumNetto(TaxLines)`, `SteuersatzBetrag = SumMwSt(TaxLines)`, `SteuersatzProzent = PrimarySatz(TaxLines)` (= rate of first line, 0 if none).
   - `Bruttobetrag`: use returned `bruttobetrag` if `> 0`; **else** compute `ComputeBrutto(TaxLines, Trinkgeld) = sum(netto)+sum(mwst)+trinkgeld`.
   - `KontoVorschlaege = gegenkonto_vorschlaege`.
   - `bezahldatum` set only if non-empty and `Bezahldatum` not already set.

**Worked example** (from tests):
Input JSON:
```json
{"auftraggeber":"R","steuerzeilen":[{"satz":19,"netto":14.20,"mwst":2.70},
{"satz":7,"netto":18.69,"mwst":1.31}],"trinkgeld":2.00}
```
Result: 2 tax lines; `Trinkgeld = 2.00`; `BetragNetto = 32.89`; `Bruttobetrag = ComputeBrutto = 32.89 + 4.01 + 2.00 = 38.90`.

**Worked example (own-VAT filtering):**
`{"vat_id":"DE287472874", …}` with own=`["DE287472874"]` → `VATID == ""`.
`{"vat_id":"IE6388047V", …}` with own=`["DE287472874"]` → `VATID == "IE6388047V"` (foreign kept).

**Worked example (suggestions):**
`{"…","gegenkonto_vorschlaege":[6837,6800,27]}` → `KontoVorschlaege = [6837,6800,27]`.

> Quirk: extra/unknown keys in the JSON (e.g. `"firmenname"` instead of `auftraggeber`) are silently ignored by the unmarshaller — they do not populate any field.

#### 4.7 Bank-statement metadata extraction (Claude Vision)

A separate prompt (`statementSystemPrompt`) extracts header data from statement **page images**. For PDFs, **all pages** are rendered (`PDFAllPagesToBase64`) and sent as multiple images, because the closing balance often lives on the last page. Output JSON:

```json
{"date_from":"dd.MM.yyyy|null","date_to":"dd.MM.yyyy|null",
 "number":"Auszugsnummer|null","opening_balance":0.0,"closing_balance":0.0}
```

Rules: response must begin `{` and end `}`, no prose/markdown; dot decimal separator, no thousands separators; opening = "Anfangssaldo / Saldo Vortrag / Alter Kontostand", closing = "Endsaldo / Neuer Saldo"; when two "Kontostand am DD.MM.YYYY" appear, the **earlier** date is opening, the **later** is closing. `number` formats: prefer `"N/YYYY"` (e.g. `"Kontoauszug 1/2026" → "1/2026"`); else `"Auszug Nr. N"/"Statement No. N" → "N"`. Parsed leniently into `StatementMetadata` (missing fields tolerated). This path returns a `StatementMetadata`, **not** a confidence value.

### 5. Local regex extraction (no Claude)

`LocalExtractor.Extract(text)` builds a `Meta` from heuristics; `Waehrung` defaults `"EUR"`. It tracks `matched/total` over **4** field groups (company, invoice number, date, amounts); **confidence = matched/4** (0.0, 0.25, 0.50, 0.75, 1.0).

**Company** (`extractCompany`): scan the first ≤10 lines; return the first trimmed line of length 5–100 that matches (case-insensitive) `gmbh|ag|kg|ltd|inc|corp`. Fallback: first trimmed line of length 5–100.

**Invoice number** (`extractInvoiceNumber`): first capture from these regexes, in order (capture group = `[A-Z0-9\-/]+` unless noted):
1. `(?i)rechnungsnr[.:\s]+([A-Z0-9\-/]+)`
2. `(?i)rechnung\s+nr[.:\s]+([A-Z0-9\-/]+)`
3. `(?i)invoice\s+no[.:\s]+([A-Z0-9\-/]+)`
4. `(?i)invoice\s+number[.:\s]+([A-Z0-9\-/]+)`
5. `(?i)re[-\s]?([0-9]{4,})`

**Date** (`extractDate`) → normalized to `DD.MM.YYYY`:
1. `(?i)rechnungsdatum[:\s]+(\d{1,2})\.(\d{1,2})\.(\d{4})`
2. `(?i)datum[:\s]+(\d{1,2})\.(\d{1,2})\.(\d{4})`
3. `(\d{1,2})\.(\d{1,2})\.(\d{4})`
4. ISO fallback `(\d{4})-(\d{2})-(\d{2})` → `DD.MM.YYYY`.
Day/month are zero-padded to 2 digits. From the result, `Jahr` = parts[2], `Monat` = parts[1].

**Amounts** (`extractAmounts`) — returns gross/net/vat/vatPercent:
- Gross (first match): `(?i)gesamt[:\s]+([\d.,]+)`, `(?i)brutto…`, `(?i)total…`, `(?i)rechnungsbetrag…`.
- Net: `(?i)netto…`, `(?i)net…`, `(?i)summe\s+netto…`.
- VAT (captures rate **and** amount): `(?i)ust\s+(\d+)%[:\s]+([\d.,]+)`, `(?i)mwst\s+(\d+)%…`, `(?i)vat\s+(\d+)%…`, `(\d+)%[:\s]+([\d.,]+)`.
- **Derivations**: if gross & vat but no net → `net = gross − vat`; if net & vat but no gross → `gross = net + vat`; if net & gross but no vat → `vat = gross − net`, `vatPercent = vat/net×100`.
- The amounts group counts as "matched" only if `gross > 0`.
- After extraction: `TaxLines = ReconstructTaxLines(net, vatPercent, vat, gross)`; if `Bruttobetrag == 0` → `ComputeBrutto(TaxLines, Trinkgeld)`.

**Currency** (`extractCurrency`): contains `€` or `EUR` → `EUR`; `$` or `USD` → `USD`; default `EUR`.

**Short description** (`generateShortDesc` → then `NormalizeVerwendungszweck`): if text (lower) contains a keyword in `wartung, lizenz, abo, subscription, service, beratung`, return `Capitalize(keyword) [+ " " + GermanMonthName(Monat) + " " + Jahr if month known]`. Else `"Rechnung " + Auftraggeber` or `"Rechnung"`.

**Amount parsing** (`parseAmount`) — shared decimal logic:
- Strip `'` and spaces.
- Decimal separator = **whichever of `,` / `.` appears last**: if last `,` is after last `.`, comma is decimal (remove `.`, replace `,`→`.`); else dot is decimal (remove all `,`).
- Parse float, **round to 2 decimals** (`round(f*100)/100`).
- Examples: `"1.234,56"` → `1234.56`; `"1,234.56"` → `1234.56`; `"1234,5"` → `1234.50`.

German month names map: `01 Januar, 02 Februar, 03 März, 04 April, 05 Mai, 06 Juni, 07 Juli, 08 August, 09 September, 10 Oktober, 11 November, 12 Dezember`.

### 6. Verwendungszweck normalization

`NormalizeVerwendungszweck(s)`: replace every `&` (with any surrounding whitespace) by ` und ` and trim. Applied to extracted purposes only — **not** to company names (which keep `&`).
Examples: `"Einstellgebühr & Top-Anzeige"` → `"Einstellgebühr und Top-Anzeige"`; `"A&B"` → `"A und B"`; `"A&B&C"` → `"A und B und C"`; `"& Anfang"` → `"und Anfang"`; `"Ende &"` → `"Ende und"`.

### 7. Amount-locator (Claude Vision bounding box)

For the PDF preview highlight feature, `LocateValue` asks Claude Vision for the bounding box of a money amount on rendered page images (`PDFAllPagesToBase64`). It is invoked only when the **text layer** did not already locate the value (`HighlightRects` empty). The prompt (`locateSystemPrompt`, German) demands strictly:

```json
{"found":true,"page":<0-based>,"box":[x0,y0,x1,y1]}   // or  {"found":false}
```

Coordinates are normalized `0.0–1.0`, **top-left origin** (x0,y0 = top-left corner; x1,y1 = bottom-right). `parseLocateJSON`: strips fences (same as `cleanJSONResponse`), **clamps** each coordinate to `[0,1]`, and treats a **degenerate box** (`x1 ≤ x0` or `y1 ≤ y0`) as `Found=false`. Results (including negatives) are cached per `(basename, value)` in `<profileConfigDir>/locate_cache.json` so a receipt is never re-queried.

Worked examples (tests): `{"found":true,"page":1,"box":[0.1,0.2,0.3,0.25]}` → Found, page 1, box (0.1,0.2,0.3,0.25); ```` ```json{"found":true,"page":0,"box":[0.05,0.1,0.4,0.15]}``` ```` → parsed through fences; `{"found":true,"page":0,"box":[0.5,0.1,0.3,0.4]}` → Found=false (degenerate).

### 8. VAT-ID normalization & own-VAT exclusion

`normalizeVATID(s)`: uppercase, trim, then drop all spaces, tabs, `-`, `.`, `/`. So `"DE 287 472 874"` ≡ `"de-287472874"` ≡ `"de.287.472.874"` → `"DE287472874"`.
`isOwnVATID(extracted, own)`: true iff normalized `extracted` (non-empty) equals any normalized `own`. Used both in the prompt (instruction) and post-parse (defense-in-depth) so the user's own number is never stored even if Claude ignores the prompt.

### 9. Platform-specific PDF rendering

| Function | Used for | Renderer |
|---|---|---|
| `PDFToImageBase64(path)` | first page → PNG (Vision STEP 3) | **macOS ARM64**: external tools; **all others**: go-fitz |
| `PDFAllPagesToBase64(path)` | all pages → PNG[] (multimodal, statement, locator) | **always go-fitz** (`doc.Image`, default DPI) |
| `RenderPDF(path, dpi)` | preview highlight rendering | go-fitz `ImageDPI(n, dpi)`, `previewDPI = 110.0` |

**go-fitz path** (`pdfToImageGoFitz`, used on Windows/Linux/macOS-Intel and for all-pages everywhere): MuPDF via go-fitz; first page rendered with `doc.Image(0)`, which is **2× resolution = 144 DPI** (per code comment), then PNG-encoded → base64, media type `image/png`. Wrapped in a `recover()` guard.

**macOS ARM64 external path** (`pdfToImageExternal`, first-page Vision only) — chosen because go-fitz's signal handling crashes on ARM64. Writes to a temp PNG `buchisy_<pid>.png`, then tries in order:
1. `sips -s format png --resampleHeightWidthMax 2400 <in> --out <tmp>`
2. on failure: `convert -density 200 -quality 90 <in>[0] <tmp>` (ImageMagick)
3. on failure: `gs -dNOPAUSE -dBATCH -sDEVICE=png16m -r200 -dFirstPage=1 -dLastPage=1 -sOutputFile=<tmp> <in>` (Ghostscript)

The generated file is validated as a real PNG (`png.Decode`) before base64-encoding; media type `image/png`.

> Quirk: `PDFAllPagesToBase64` always uses go-fitz, even on macOS ARM64 — so the multimodal/statement/locator paths can still hit the ARM64 go-fitz codepath that the first-page path was specifically rewritten to avoid. Only the single-page Vision path uses the external tools.

### 10. Image-file extraction (non-PDF)

`extractImageData(path)` (Claude mode only, image extensions only): read raw file bytes, base64-encode them directly (no re-render), send via `ExtractFromImage` with the file's `ImageMediaType` (jpeg/png/gif/webp). Same JSON contract as the PDF Vision path, **confidence `0.95`**, Quelle `"Vision"`. After extraction it mirrors the account-suggestion logic and sets `Waehrung = CurrencyDefault` if empty.

### 11. Account suggestion (post-extraction, all PDF/image paths)

After any successful extraction, `Gegenkonto` is filled (identical logic in every branch):
1. If `settings.AutoSelectAccount` and `Auftraggeber != ""`: try `SuggestAccountForCompany(companyMap, Auftraggeber, DefaultAccount)`; on hit use it.
2. Else if `KontoVorschlaege` (AI suggestions) non-empty: use `KontoVorschlaege[0]`.
3. Else: `settings.DefaultAccount` (default `2000`).

(`KontoVorschlaege` is transient — used for the suggestion, not persisted.)

### Re-implementation checklist

- **Ingest paths**: drag-drop (Belege = enqueue all supported; Konten = first statement only), clipboard (Windows-only: files first, else bitmap→temp PNG), search picker / native multi-file picker with per-kind last-folder memory, and a **5 s** scan-inbox poller that only dispatches a `.pdf` whose size was **stable across two polls**, one at a time.
- **Priority chain exactly**: e-invoice (conf **1.0**) → PDF text; if `HasText` (trimmed length **> 10**): claude multimodal (text+all-page images, conf **0.95**) / claude text-only fallback (conf **0.90**) / local regex (conf = matched/4); if no text: claude Vision first-page (conf **0.95**) / local-mode error `"no text found in PDF"`.
- **E-invoice**: detect via pdfcpu attachment extraction; classify by filename then content (`urn:cen.eu:en16931`→XRechnung, `urn:ferd:`→ZUGFeRD, else ZUGFeRD); **parse CII only (no UBL)**; map exactly the documented fields; date `YYYYMMDD`→`DD.MM.YYYY` only when 8 chars; `Auftraggeber` = seller always; `Verwendungszweck = "Rechnung " + number`.
- **PDF text quirk**: replace `U+FFFD` with `-`.
- **Claude**: Messages API, `anthropic-version: 2023-06-01`, `x-api-key`, `max_tokens 4096`, no temperature; retry 3× only on 429/5xx with 2 s×attempt backoff; 60 s timeout; default model `claude-sonnet-4-6`.
- **Exact German JSON schema** with all 14 keys; brutto fallback `= sum(netto)+sum(mwst)+trinkgeld`; net/vat/rate derived from `steuerzeilen`; markdown-fence + first-`{`/last-`}` salvage.
- **Own-VAT exclusion** both in prompt and post-parse; normalization drops case/space/`-`/`.`/`/`.
- **10 000-char preprocessing** with the exact keyword priority list (byte-length cap).
- **Local regex**: exact pattern lists for company/number/date/amounts; `parseAmount` last-separator-wins decimal logic; ISO date fallback; confidence = matched/4.
- **Verwendungszweck**: `&`→` und ` (purposes only, not company names).
- **Amount-locator**: normalized `[0,1]` top-left boxes, clamp, degenerate→not-found, cached per `(basename,value)`.
- **Rendering**: macOS-ARM64 first-page Vision uses sips→convert→gs (temp PNG, max dim 2400 / density 200 / r200); everywhere else and all-pages use go-fitz (first page 144 DPI; preview `RenderPDF` at 110 DPI). Output always `image/png` (except raw image files, which keep their own media type).
- **Confidence values**: e-invoice 1.0, multimodal 0.95, vision/image 0.95, claude text 0.90, local matched/4.

---

## Chart of Accounts, SKR & Account Model

This chapter specifies the chart-of-accounts subsystem: the account record model, the bundled SKR04 seed and how it is merged with a per-profile import, CSV/JSON import formats, the SKR03↔SKR04 standard-account tables plus the "Prüfen" (validate) / "anwenden" (apply) switch, account display formatting, the distinction between expense **Gegenkonten** and payment **Zahlungskonten**, per-profile account preferences (recent + favorites), and the company→account mapping that drives account suggestion and learning.

All persisted JSON files described here live in the **active profile config directory** (one directory per profile). File names are fixed and not configurable.

---

### 1. Account record model

#### 1.1 SKRAccount

A single account in the chart. Fields and JSON keys:

| Field | JSON key | Type | Notes |
|---|---|---|---|
| Number | `number` | int | The account number, e.g. `6640`. Primary key within a chart. |
| Name | `name` | string | German account name, e.g. `"Bewirtungskosten (abziehbar)"`. May be empty. |
| Type | `type` | string | Category. Known values: `expense`, `revenue`, `asset`, `liability`, `equity`, `vat`. Other/unknown values are allowed and stored verbatim. |
| TaxKey | `tax_key` | string | Optional DATEV BU-/Steuerschlüssel. Omitted from JSON output when empty (`omitempty`). |

> Quirk: `Type` is a free-form string. Only the five values `expense/revenue/asset/liability/equity` get a German tooltip word (see §6); `vat` and any unknown type produce **no** parenthetical in tooltips. The seed file uses `vat` for the Vorsteuer accounts, so those accounts have no tooltip type word.

#### 1.2 ChartOfAccounts (in-memory index)

Built from a slice of accounts. Construction rules (must be replicated exactly):

1. **Last-write-wins on duplicate number.** Insert each account into a map keyed by `number`. A later account with the same number **replaces** an earlier one. This is the mechanism by which an imported override replaces a bundled default.
2. **De-duplicated, number-sorted list.** The public list is the map's values, sorted ascending by `number`. Duplicates therefore collapse to one entry.

Operations:

- **Find(number)** → `(account, found)`. Map lookup by number.
- **All()** → copy of the sorted account list.
- **Search(q)** → case-insensitive substring search. `q` is trimmed and lowercased. Empty `q` returns `All()`. Otherwise an account matches if **the decimal string of its number contains `q`** OR **its lowercased name contains `q`**. Results preserve number-sorted order.

Worked example (from tests): a chart of `{6644 "Nicht abziehbare Bewirtungskosten", 6640 "Bewirtungskosten", 1800 "Bank"}`:
- `All()` → ordered `[1800, 6640, 6644]`.
- `Search("bewirt")` → 2 results (both 6640 and 6644, name match).
- `Search("1800")` → 1 result (number-string match).

---

### 2. Bundled SKR04 seed

The application embeds a starter chart (`assets/skr04.json`). It is a **minimal** seed of exactly **8 accounts** — not a full SKR04. A re-implementation must ship this exact list as the bundled default:

```json
[
  {"number": 1406, "name": "Abziehbare Vorsteuer 19 %", "type": "vat"},
  {"number": 1401, "name": "Abziehbare Vorsteuer 7 %", "type": "vat"},
  {"number": 1600, "name": "Kasse", "type": "asset"},
  {"number": 1800, "name": "Bank", "type": "asset"},
  {"number": 4300, "name": "Erlöse 7 % USt", "type": "revenue"},
  {"number": 4400, "name": "Erlöse 19 % USt", "type": "revenue"},
  {"number": 6640, "name": "Bewirtungskosten (abziehbar)", "type": "expense"},
  {"number": 6644, "name": "Nicht abziehbare Bewirtungskosten", "type": "expense"}
]
```

> Quirk: The bundled seed contains **only** these 8 accounts. Many accounts referenced by the default booking rules (e.g. output-VAT 3806/3801, reverse-charge 1407/3837, EU/Drittland revenue 4125/4120) are **not** in the seed. Until the user imports a fuller chart, `ValidateBookingAccounts` (§5.2) will report those as missing, and `paymentSKR04Label` (§7) shows the bare number rather than a name. This is intended IST behavior, not a bug to "fix."

---

### 3. ChartStore: merge & persistence

The `ChartStore` owns two things: the embedded `bundled` bytes, and a fixed per-profile import file path:

```
<profileConfigDir>/chart_skr04.json
```

**Load()** algorithm:
1. Parse the bundled JSON → bundled accounts (errors out if bundled JSON is invalid).
2. If `chart_skr04.json` exists and is readable, parse it → imported accounts. If the file does not exist (read error), the import is an empty list (no error). If the file exists but is invalid JSON, Load **fails** with an error.
3. Build the chart from `bundled ++ imported` (imported appended **after** bundled). Because of last-write-wins (§1.2), imported entries **override** bundled entries with the same number and **extend** the chart with new numbers.

**SaveImport(accounts)** writes the given accounts to `chart_skr04.json` as **pretty-printed JSON** (2-space indent), file mode `0644`. This overwrites the whole import file (it is not additive across calls).

Worked example (from tests): bundled `[{6640 "Bewirtungskosten"}]`; import `[{6640 "Bewirtung (eigene Liste)"}, {1800 "Sparkasse"}]`. After reload: `Find(6640).Name == "Bewirtung (eigene Liste)"` (override) and `Find(1800)` exists (extension).

---

### 4. Import formats

#### 4.1 JSON import (ParseChartJSON)

Input is a JSON **array** of account objects (same shape as §1.1). Rules:
- Blank/whitespace-only input → **empty list, not an error**.
- Invalid JSON → error `"failed to parse chart JSON: ..."`.

#### 4.2 CSV import (ParseChartCSV)

This is the format used by the "SKR04 importieren" button in Settings (it reads a file, parses as CSV, calls `SaveImport`, then reloads the chart). Algorithm, in order:

1. **Encoding:** If the bytes are **not** valid UTF-8, decode them as **ISO-8859-1 (Latin-1)**. (If they are valid UTF-8, use as-is.)
2. **Separator auto-detection:** Count `;` vs `,` in the raw bytes. If `;` count > `,` count, the delimiter is `;`; otherwise `,`. (Ties and comma-majority → comma.)
3. **Parse** as CSV with: variable fields per record allowed, lazy quotes enabled (tolerant of stray quote characters).
4. **Per row:**
   - Skip empty rows.
   - Take cell[0], trim whitespace, parse as integer. **If it is not a valid integer, skip the entire row.** This silently drops header rows and any junk rows.
   - Column mapping (positional):
     - **Column 1 (index 0): Konto** → `number` (required, integer).
     - **Column 2 (index 1): Bezeichnung** → `name` (trimmed). Optional.
     - **Column 3 (index 2): Steuerschlüssel** → `tax_key` (trimmed). Optional.
   - Extra columns beyond the third are ignored.

Worked example (from tests). Input:
```
Konto;Bezeichnung;Steuerschlüssel
6640;Bewirtungskosten;
1800;Bank;
nicht;eine Zahl;
```
→ 2 accounts: `{6640, "Bewirtungskosten"}` and `{1800, "Bank"}`. The header row (`Konto…`, non-numeric cell[0]) and the `nicht;…` row are dropped. The trailing empty third cells produce empty `tax_key`.

> Quirk: Header detection is purely "is cell[0] an integer?" — there is no header-keyword matching. A data row whose first cell is non-numeric is silently lost.

---

### 5. SKR03 / SKR04 standard accounts, validation, detection, and switch

#### 5.1 Standard account tables (StandardSKR)

For each variant, the standard tax/booking accounts are hard-coded. A re-implementation must reproduce these tables exactly:

| Concept | Key | SKR03 | SKR04 |
|---|---|---|---|
| Vorsteuer (input VAT) 19% | `Vorsteuer["19"]` | 1576 | 1406 |
| Vorsteuer 7% | `Vorsteuer["7"]` | 1571 | 1401 |
| Umsatzsteuer (output VAT) 19% | `Umsatzsteuer["19"]` | 1776 | 3806 |
| Umsatzsteuer 7% | `Umsatzsteuer["7"]` | 1771 | 3801 |
| Vorsteuer §13b reverse charge | `VStRC` | 1577 | 1407 |
| Umsatzsteuer §13b reverse charge | `UStRC` | 1787 | 3837 |
| Bewirtung abziehbar (deductible) | `BewAbz` | 4650 | 6640 |
| Bewirtung nicht abziehbar | `BewNicht` | 4654 | 6644 |
| Erlöse Inland (domestic revenue) | `ErloesInland` | 8400 | 4400 |
| Erlöse EU | `ErloesEU` | 8341 | 4125 |
| Erlöse Drittland (non-EU) | `ErloesDrittland` | 8200 | 4120 |

Unknown variant names (anything other than the literal strings `"SKR03"` / `"SKR04"`) return "not found".

What distinguishes the two charts conceptually: SKR04 numbers expense/asset accounts in the 1xxx/3xxx/4xxx/6xxx ranges (balance-sheet-oriented), while SKR03 uses the 1xxx/4xxx/8xxx ranges (process-oriented). The application keys off two marker accounts only (see §5.4).

#### 5.2 Validate booking accounts (ValidateBookingAccounts)

Returns a list of human-readable German issue strings — one per **referenced** booking account that is **not present** in the chart. Inputs: the current booking rules and the current chart. If either is null → no issues.

Algorithm: for every account number referenced by the rules, if the number is non-zero **and** `chart.Find(number)` fails, emit one issue. **Account number 0 is treated as "unset" and skipped** (never reported). Referenced accounts checked, with their issue labels:

- Each entry of `VorsteuerKonten[rate]` → label `"Vorsteuer <rate>%"`.
- Each entry of `UmsatzsteuerKonten[rate]` → label `"Umsatzsteuer <rate>%"`.
- Each entry of `ErloesKonten[key]` → label `"Erlöskonto <key>"`.
- For each rule, using the rule's `Name` if non-empty else its `Kategorie` as the label prefix: the rule's `KontoAbziehbar`, `KontoNichtAbziehbar`, `KontoVStRC`, `KontoUStRC`, `DefaultKonto`.

Issue string format: `"<label>: Konto <number> nicht im Kontenrahmen"`.

Worked example (from tests): a tiny chart `{1576, 1571, 8400}` against rules referencing `Vorsteuer 19=1576, 7=1571; Umsatzsteuer 19=1776; Erlös inland=8400, eu=8341; bewirtung KontoAbziehbar=4650, KontoNichtAbziehbar=0; reverse_charge KontoVStRC=1576, KontoUStRC=1787` yields exactly **4** issues — for `1776`, `8341`, `4650`, and `1787`. The `0` (KontoNichtAbziehbar) is skipped; `1576` (used twice) and `1571`/`8400` are present so they pass.

> Quirk: Map iteration order is not deterministic, so the **order** of issues is not stable. Re-implementers should not rely on ordering; only the **set** of issues matters.

#### 5.3 Apply a variant (ApplySKRVariant) — the "switch"

Returns a **new, deep-copied** rules object with all standard accounts set to the chosen variant's values; non-account fields (percentages, names, `Kategorie`, `DefaultKonto`, `Schwelle`, `ForderungsKonto`, `KontoStichwoerter`, etc.) are preserved from the original. The **original rules object is never mutated.** Unknown variant → returns null.

Transformation, field by field:
1. **VorsteuerKonten:** **replaced entirely** by the variant's Vorsteuer map (`{"19":…, "7":…}`). Any other rates the user had are dropped.
2. **UmsatzsteuerKonten:** start from a copy of the **existing** map, then overwrite keys `"19"` and `"7"` with the variant's values. Other existing rates are preserved.
3. **ErloesKonten:** start from a copy of the existing map, then set keys `"inland"`, `"eu"`, `"drittland"` to the variant's revenue accounts. Other keys preserved.
4. **KontoStichwoerter:** deep-copied unchanged (null stays null).
5. **Regeln (rules):** copied; then for each rule, by `Kategorie`:
   - `"bewirtung"` → set `KontoAbziehbar = BewAbz`, `KontoNichtAbziehbar = BewNicht`.
   - `"reverse_charge"` → set `KontoVStRC = VStRC`, `KontoUStRC = UStRC`.
   - All other rule fields preserved.
6. **Reverse-charge auto-create:** if **no** rule with `Kategorie == "reverse_charge"` existed, append a new one: `{Kategorie:"reverse_charge", Name:"Reverse Charge §13b", KontoVStRC, KontoUStRC}` with the variant's values.
7. **ForderungsKonto** is carried over unchanged.

Worked example (from tests): SKR04 rules → `ApplySKRVariant(…, "SKR03")` yields `VorsteuerKonten {19:1576, 7:1571}`, `UmsatzsteuerKonten[19]=1776`, `ErloesKonten {inland:8400, eu:8341, drittland:8200}`, bewirtung `{KontoAbziehbar:4650, KontoNichtAbziehbar:4654}` with `AbziehbarProzent:70` preserved, reverse_charge `{KontoVStRC:1577, KontoUStRC:1787}` with `RcSatz:19` preserved. The source object's `VorsteuerKonten[19]` is still `1406` (no mutation).

> Quirk: `VorsteuerKonten` is a full **replacement** (rates other than 19/7 are lost), but `UmsatzsteuerKonten` and `ErloesKonten` are **merged** (extra keys survive). This asymmetry must be replicated.

#### 5.4 Detect the variant (DetectSKRVariant)

Guesses the chart's variant from **marker accounts**, in this priority:
1. If the chart contains account **1576** → `"SKR03"`.
2. Else if it contains account **1406** → `"SKR04"`.
3. Else → `""` (unknown).

Null chart → `""`.

> Quirk: Detection checks only these two Vorsteuer-19% marker accounts. If a chart somehow contains **both** 1576 and 1406, SKR03 wins (checked first). The bundled seed contains 1406 → detected as SKR04.

#### 5.5 The "Prüfen" / "anwenden" UI flow

In Settings, a "Kontenrahmen" card exposes three buttons: **Prüfen**, **SKR03 anwenden**, **SKR04 anwenden**. The currently-detected variant's apply button is rendered as high-importance (visually emphasized).

- **Prüfen** runs `DetectSKRVariant` and `ValidateBookingAccounts` against the current chart and rules, then shows a status label:
  - Header line: `"Erkannter Kontenrahmen: <variant>"`, or `"Erkannter Kontenrahmen: (unbekannt)"` when detection returns empty.
  - If no issues: second line `"✓ Alle Buchungskonten im Kontenrahmen vorhanden"`.
  - If issues: `"Probleme:"` followed by one `"• <issue>"` line per issue.
- **SKR03/SKR04 anwenden** opens a confirm dialog titled `"Kontenrahmen anwenden"` with body `"Alle Standard-Buchungskonten werden auf <variant> umgestellt.\n\nFortfahren?"`. On confirm: compute `ApplySKRVariant`, **persist the new rules** via the booking-rules store, set them as the live rules, and show `"✓ <variant> erfolgreich angewendet und gespeichert"` + a toast `"✓ <variant> angewendet"`. On cancel: nothing happens.

Note: the apply switch changes **booking rules** (which standard accounts postings target). It does **not** rewrite the chart of accounts itself — the chart is changed separately via CSV/JSON import. A complete switch to another SKR therefore means: import the matching chart **and** apply the matching variant.

---

### 6. Account display formatting

Two render functions, both producing `"<Number> — <Name>"` style strings. The separator is an em-dash with surrounding spaces: ` — ` (space, U+2014 EM DASH, space).

- **AccountDisplay(a):** `"<number> — <name>"`. Used in compact cells. (No special handling of empty name.)
- **AccountTooltip(a):** `"<number> — <name> (<TypWord>)"` when the type maps to a German word, else `"<number> — <name>"` (no parenthetical).
  - Type→word map: `expense→Aufwand`, `revenue→Erlös`, `asset→Aktiva`, `liability→Passiva`, `equity→Eigenkapital`. Any other type (including `vat`, empty) → no word.
- **accountLabel(a)** (UI helper, used in pickers/labels): `"<number> — <name>"`, but if `name` is empty, just `"<number>"`.

Worked examples (from tests):
- `AccountDisplay({4663, "Reisekosten Arbeitnehmer, Fahrtkosten"})` → `"4663 — Reisekosten Arbeitnehmer, Fahrtkosten"`.
- `AccountTooltip(…, type "expense")` → `"4663 — Reisekosten Arbeitnehmer, Fahrtkosten (Aufwand)"`.
- `AccountTooltip({1200, "Sparkasse", type ""})` → `"1200 — Sparkasse"` (no parenthetical).

---

### 7. Two distinct account concepts: Gegenkonten vs Zahlungskonten

These are **different** concepts and must not be conflated.

**Gegenkonto (expense/revenue counter-account):** the SKR account a booking posts **against** — typically an expense account for an incoming invoice (e.g. 6640 Bewirtung, 4663 Reisekosten) or a revenue account for an outgoing invoice. It is chosen per booking from the full chart via the account search/picker. Picking a Gegenkonto records it as "recently used" (§8) and can be remembered per company (§9).

**Zahlungskonto (payment account / BankAccount):** a user-defined account through which money actually flows — a bank account, a credit card / clearing account, or a cash register. Fields (JSON keys):

| Field | JSON key | Type | Notes |
|---|---|---|---|
| Name | `name` | string | Display name, e.g. `"KSMSE …0712 Sparkasse"`. |
| IBAN | `iban` | string | |
| AccountType | `account_type` | string | One of `bank`, `creditcard`, `cash`. |
| SettlementAccount | `settlement_account` | string | Name of the account that settles a credit card monthly. |
| SKR04Konto | `skr04_konto` | int | The SKR account this payment account maps to (the Haben/credit side). Omitted when 0. |
| IsCreditCard | `is_credit_card` | bool | Legacy flag, retained only for migration. |

**Payment-account → SKR resolution (PaymentAccountSKR04, by name):**
1. Find the BankAccount by name.
2. If its `SKR04Konto != 0`, return that (explicit mapping wins).
3. Else fall back by `AccountType`: `bank → 1800`, `cash → 1600`.
4. `creditcard` (and payroll-type/clearing accounts) have **no** universal default → return "not mapped" (0, false) unless an explicit `SKR04Konto` was set.

In the booking, the Zahlungskonto's SKR account is the credit (Haben) side; the Gegenkonto is the debit (Soll) side (for an expense). The Settings UI lets the user attach an `SKR04Konto` to each payment account via the same account search picker, and renders it with `paymentSKR04Label`: shows the i18n "none" string when 0, the human-readable `accountLabel` when the chart knows the account, or the bare number when it does not.

The "Konten ▾" toggle in the main view lists the configured Zahlungskonten; picking one switches into that account's bank-statement view. (Picking a payment account is **not** a Gegenkonto pick and does **not** record recency.)

---

### 8. Account preferences (recent + favorites)

Per-profile file:
```
<profileConfigDir>/account_prefs.json
```
JSON shape (pretty-printed, 2-space indent, mode `0644`):
```json
{
  "recent": [4663, 4920],
  "favorites": [8400, 8341]
}
```
Both are arrays of account **numbers** (ints). Load tolerates a missing file (no error, empty lists). Save creates the directory if needed.

**RecordUse(konto):** prepend-dedupe-cap:
1. Remove any existing occurrence of `konto`.
2. Prepend `konto` to the front.
3. Cap the list at **8** entries (keep the first 8; drop the oldest beyond that).

Worked examples (from tests):
- Record 4663, 4920, 4663 → recent `[4663, 4920]` (the duplicate 4663 moved to front, length 2).
- Record 4001..4009 (9 distinct) → recent length 8, `recent[0]==4009`, `recent[7]==4002` (4001 dropped).

**Favorites:** `IsFavorite(konto)` linear membership test; `ToggleFavorite(konto)` adds if absent, removes if present (no cap, no ordering guarantees beyond insertion/removal). Favorites are appended in toggle order.

When is recency recorded? Only when a user picks a **Gegenkonto** through the account search (in the invoice edit row and the invoice modal), after which `RecordUse` + `Save` are called. Setting a payment account's SKR mapping does **not** record recency.

**Picker presentation (buildEmptyResults, empty query):** the picker shows sectioned rows with German headers, each section omitted if empty, and de-duplicated across sections (an account shown in an earlier section is not repeated later):
1. **"Zuletzt benutzt"** (recent) — recent numbers resolved against the chart, in recent order.
2. **"Favoriten"** (favorites) — favorites **minus** any already shown in recent.
3. **"Alle Konten"** (all) — all chart accounts **minus** those already shown above, in number-sorted order.

When the user types a query, the flat `Search` (§1.2) is used instead.

---

### 9. Company → account mapping (companymap) and suggestion

Per-profile file:
```
<profileConfigDir>/company_accounts.json
```
JSON shape (pretty-printed, 2-space indent, mode `0644`): a flat object mapping **normalized company name → account number**:
```json
{
  "amazon": 6815,
  "deutsche bahn": 4673
}
```
Load tolerates a missing file (no error, empty map). Save creates the directory if needed.

**Key normalization (NormalizeCompanyName)** — applied on both write (`Set`) and read (`Get`), so keys are stable:
1. Lowercase the whole string.
2. Trim leading/trailing whitespace.
3. Collapse internal runs of whitespace to a single space.
4. Strip **one** trailing legal suffix if present, then re-trim. Suffix list, checked in order, **first match wins**: `" gmbh"`, `" ag"`, `" kg"`, `" ohg"`, `" gbr"`, `" ug"`, `" e.k."`, `" ltd"`, `" inc"`, `" corp"`.

So `"ACME GmbH"`, `"acme  gmbh "`, and `"ACME"` all normalize to `"acme"` and share one mapping.

> Quirk: Suffix stripping removes at most one suffix and only as a literal trailing token preceded by a space. `"acme gmbh & co. kg"` → strips trailing `" kg"` → `"acme gmbh & co."` (the inner `gmbh` is **not** removed). The list order is `gmbh` before `ag`; only one is removed even if multiple could theoretically apply.

**Matching / suggestion (SuggestAccountForCompany(map, companyName, defaultAccount)):**
- Normalize `companyName`, look it up in the map.
- If found → return `(rememberedAccount, true)`.
- If not found → return `(defaultAccount, false)` (the global default account from settings).

The `false`/`true` flag tells the caller whether the suggestion came from a remembered mapping (true) or is just the fallback default (false). The booking flows call `SuggestAccountForCompany(companyMap, meta.Auftraggeber, settings.DefaultAccount)` to pre-fill the Gegenkonto.

**Learning loop:** when the user files an invoice with the "remember mapping" option enabled and a non-empty company name, the app calls `companyMap.Set(company, chosenAccount)` then `Save()`. The next time an invoice from the same (normalized) company is processed, the suggestion returns that account with `found=true`. Save failures are logged as warnings and are **non-fatal** (the invoice still files).

> Quirk: The match is **exact on the normalized key** — there is no fuzzy matching, no substring matching, and the vendor name used is the raw `Auftraggeber` (note: `Auftraggeber` keeps its `&`; only Verwendungszweck text has `&`→`und` normalization applied elsewhere, so an ampersand in a company name is preserved through normalization). A company whose normalized form differs (e.g. a typo, extra token) will not match and falls back to the default account.

---

### Re-implementation checklist

Must-match behaviors for this subsystem:

1. **Account record:** keys `number`(int), `name`(string), `type`(string), `tax_key`(string, omit when empty). `type` is free-form; only `expense/revenue/asset/liability/equity` get tooltip words.
2. **Chart index:** last-write-wins on duplicate `number`; public list de-duplicated and sorted ascending by number. `Search` = case-insensitive substring over number-string OR name; empty query → all.
3. **Bundled seed:** exactly the 8 accounts listed in §2 (1406, 1401, 1600, 1800, 4300, 4400, 6640, 6644). Do not silently ship a full SKR04.
4. **ChartStore:** import path `<profile>/chart_skr04.json`; Load = bundled ++ imported (imported overrides/extends); SaveImport overwrites with 2-space-indented JSON, mode 0644; missing import file is not an error, invalid JSON is.
5. **CSV import:** UTF-8 else ISO-8859-1 decode; `;` vs `,` delimiter by majority count (tie → comma); skip rows whose first cell is not an integer; columns Konto, Bezeichnung, Steuerschlüssel (positional); extra columns ignored.
6. **Standard SKR tables:** reproduce the SKR03/SKR04 numbers in §5.1 exactly. Only `"SKR03"`/`"SKR04"` are valid variant strings.
7. **ApplySKRVariant:** deep copy, never mutate source; **replace** VorsteuerKonten entirely; **merge** UmsatzsteuerKonten (overwrite 19/7) and ErloesKonten (set inland/eu/drittland); update bewirtung & reverse_charge rule accounts; auto-create a reverse_charge rule (`"Reverse Charge §13b"`) if absent; preserve all non-account fields. Applying persists booking rules; it does not change the chart.
8. **ValidateBookingAccounts:** report every referenced non-zero account not in the chart; skip 0; issue string `"<label>: Konto <n> nicht im Kontenrahmen"`; order not guaranteed.
9. **DetectSKRVariant:** 1576 → SKR03 (checked first), else 1406 → SKR04, else `""`.
10. **Prüfen UI strings:** `"Erkannter Kontenrahmen: <v|(unbekannt)>"`, success `"✓ Alle Buchungskonten im Kontenrahmen vorhanden"`, problems list with `"• "` bullets; apply confirm body `"Alle Standard-Buchungskonten werden auf <v> umgestellt.\n\nFortfahren?"`.
11. **Formatting:** ` — ` (space–EM DASH–space) separator; tooltip type words per §6; `accountLabel` drops the dash when name empty.
12. **Gegenkonto vs Zahlungskonto** are distinct. PaymentAccountSKR04 resolution: explicit `skr04_konto` wins; else `bank→1800`, `cash→1600`; `creditcard`/payroll → unmapped unless explicit.
13. **account_prefs.json:** `{recent:[…], favorites:[…]}`; RecordUse = prepend, dedupe, cap 8; favorites toggled with no cap; recency recorded only on Gegenkonto picks. Picker sections "Zuletzt benutzt" / "Favoriten" / "Alle Konten", de-duplicated across sections.
14. **company_accounts.json:** flat `{normalizedName: accountNumber}`. Normalize = lowercase, trim, collapse spaces, strip one trailing legal suffix (gmbh/ag/kg/ohg/gbr/ug/e.k./ltd/inc/corp). Suggestion: exact normalized-key match → `(account, true)`, else `(defaultAccount, false)`. Learning: on "remember" + non-empty company, `Set`+`Save` (non-fatal on failure).

---

## Booking Engine (Double-Entry & VAT)

This subsystem turns a single receipt (an incoming invoice, an outgoing/revenue invoice, or a cash receipt) and its VAT breakdown into a **balanced double-entry booking** (German *Buchung*): a set of postings where the debit total (*Soll*) equals the credit total (*Haben*). It also defines the VAT lines (*Steuerzeilen* / `TaxLine`) that feed the booking, the per-category posting logic (standard, Bewirtung, Geschenke, Reisekosten, Kfz, Reverse-Charge §13b), the Vorsteuer/Umsatzsteuer account resolution by VAT rate, the revenue-account routing by counterparty, and the wire format that links a booking row back to a bank-statement line.

All amounts are decimals representing EUR. **Bookings are always stored in EUR**: foreign-currency invoices are converted to EUR *before* `BuildBooking`/`BuildRevenueBooking` is called (the caller multiplies/divides by the exchange rate). The engine itself is currency-agnostic and never sees a rate.

### 1. Data structures

#### 1.1 TaxLine (one VAT line of a receipt)

A receipt's VAT breakdown is a list of `TaxLine`. Each line:

| Field | Type | Meaning |
|---|---|---|
| `netto` | decimal | Net amount taxed at this rate |
| `satz_prozent` | decimal | VAT rate in percent (e.g. `19`, `7`, `0`) |
| `mwst_betrag` | decimal | VAT amount for this line |

A receipt may carry several lines (e.g. a restaurant bill split into 19% food-to-go and 7% items). Note that `mwst_betrag` is **stored explicitly, not recomputed** from `netto × satz_prozent`; the engine uses the stored `mwst_betrag` verbatim. This matters: a re-implementer must NOT recompute VAT from net×rate when summing — it must trust the per-line `mwst_betrag`.

Derived helpers over a line list:

- **SumNetto** = Σ `netto` over all lines.
- **SumMwSt** = Σ `mwst_betrag` over all lines.
- **ComputeBrutto(lines, trinkgeld)** = SumNetto + SumMwSt + `trinkgeld`. `trinkgeld` (tip) is added **untaxed** (no VAT applies to it).
- **PrimarySatz(lines)** = `satz_prozent` of the first line, or `0` if the list is empty. This is a legacy display value only.

> Quirk: these sums do **no rounding** internally. Rounding to 0.01 happens only inside `BuildBooking`/`BuildRevenueBooking` at the point each posting amount is produced (see §6). So `SumNetto` over many lines can carry sub-cent float noise; only the final posting is rounded.

**Worked example (taxline_test.go):**
Lines `[{14.20, 19, 2.70}, {18.69, 7, 1.31}]`, trinkgeld `2.00`:
- SumNetto = `32.89`
- SumMwSt = `4.01`
- ComputeBrutto = `38.90`
- PrimarySatz = `19`

#### 1.2 TaxLine JSON serialization

`TaxLine` lists are stored as a **compact JSON array** (no indentation), field order `netto, satz_prozent, mwst_betrag`, e.g.:

```json
[{"netto":14.2,"satz_prozent":19,"mwst_betrag":2.7}]
```

Rules:
- An empty/`nil` list marshals to the **empty string `""`** (not `"[]"`).
- The empty string, or any string that fails to parse as JSON, parses back to `nil` (no error raised).

#### 1.3 TaxLine reconstruction from legacy aggregate fields

When a stored row has no detailed `Steuerzeilen` but only the legacy aggregate scalars (`netto`, `satzProzent`, `mwst`, `brutto`), a single `TaxLine` is reconstructed:

1. If `netto == 0 AND satzProzent == 0 AND mwst == 0`:
   - If `brutto > 0` → return one line `{Netto: brutto, SatzProzent: 0, MwStBetrag: 0}` (gross-only fallback: the whole gross is treated as net so the total is preserved).
   - Else → return `nil`.
2. Otherwise → return one line `{Netto: netto, SatzProzent: satzProzent, MwStBetrag: mwst}`.

**Worked example:** `Reconstruct(0,0,0,38.90)` → `[{Netto: 38.90}]`; `Reconstruct(0,0,0,0)` → `nil`; `Reconstruct(14.20,19,2.70,0)` → `[{14.20,19,2.70}]`.

#### 1.4 BookingEntry and Booking

A **BookingEntry** is one posting line:

| Field | Type | Meaning |
|---|---|---|
| `konto` | int | Account number (SKR04 chart) |
| `betrag` | decimal | Posting amount (always positive) |
| `soll` | bool | `true` = Soll/debit, `false` = Haben/credit |
| `steuerschluessel` | string | Optional DATEV tax key; omitted when empty. **Not set by the booking engine** itself — left empty by `BuildBooking`/`BuildRevenueBooking`; populated elsewhere (DATEV export). |

A **Booking** is:

| Field | Type | Meaning |
|---|---|---|
| `entries` | list of BookingEntry | The postings; omitted when empty |
| `info` | string | Free-text rationale (*Buchungswissen*); omitted when empty |
| `manuell` | bool | `true` = hand-edited, not auto-generated; omitted when false |

Booking serialization: compact JSON. An **empty booking** (no entries AND empty info) marshals to `""`. Parsing `""` or invalid JSON yields an empty Booking (no error). The `manuell` flag round-trips through JSON (an auto booking stays `false`).

Derived Booking helpers:
- **SollSum** = Σ `betrag` of entries with `soll == true`.
- **HabenSum** = Σ `betrag` of entries with `soll == false`.
- **Balanced** = there is at least one entry AND `|SollSum − HabenSum| < 0.005`. An empty booking is **not** balanced.
- **IsEmpty** = no entries AND empty info.

### 2. The balance invariant and tolerance

Every booking the engine produces is constructed to balance **by construction**: the engine computes all postings on one side first, then derives the single counter-posting on the other side as the exact sum of that side. So `SollSum == HabenSum` always holds (the balancing entry is `round2(Σ of the other side)`).

The balance check uses an absolute tolerance of **0.005** (half a cent): `|Soll − Haben| < 0.005`. The test helper `almost(a,b)` likewise asserts `−0.005 < a−b < 0.005`. A re-implementer must use this exact tolerance for golden-value comparisons.

> Quirk — balancing entry absorbs dropped VAT: when a VAT rate has **no configured Vorsteuer/Umsatzsteuer account**, that line's VAT is silently **not posted**, and the balancing (payment) entry is the sum of what *was* posted — NOT the raw gross. See §3.6.

### 3. Incoming-invoice booking — `BuildBooking`

Signature (conceptually): `BuildBooking(rules, kategorie, lines, trinkgeld, expenseAccount, paymentAccount, rabatt) → Booking`.

Inputs:
- `kategorie` — booking category key (string). Must exist in the rules base, else error `"unbekannte Buchungskategorie: <kategorie>"`.
- `lines` — the `TaxLine` list.
- `trinkgeld` — untaxed tip, decimal.
- `expenseAccount` — the Soll expense account for the `standard` case (Gegenkonto chosen by the user/suggestion).
- `paymentAccount` — the Haben *Zahlungskonto* (bank/cash account, resolved separately — see §7).
- `rabatt` — a gross discount amount, decimal (only used by `standard`).

First the engine looks up the category `rule`; an unknown key returns an error. It then computes:

```
netTotal = round2(SumNetto(lines) + trinkgeld)
```

and branches by category. For categories that fall through to the shared tail (`geschenke≤Schwelle`, `bewirtung`, `reisekosten`, `kfz`, `standard`), the per-rate Vorsteuer is appended (§3.6) and the payment entry is derived as `round2(Σ Soll)` (§3.7). The §13b and over-threshold-Geschenke branches return their booking **directly** and skip the shared tail.

> Quirk: a category that exists in the rules but is not one of the seven handled keys returns error `"Buchungskategorie ohne Buchungslogik: <kategorie>"`. Only `standard, bewirtung, geschenke, reisekosten, kfz, reverse_charge` have logic.

#### 3.1 `standard`

The plain expense case. One Soll line to `expenseAccount`, reduced by the gross `rabatt`:

```
Soll expenseAccount = round2(netTotal − rabatt)
+ Vorsteuer per rate (§3.6)
Haben paymentAccount = round2(Σ Soll)
```

This is **Method B** for discounts: the expense is reduced by the (gross) `rabatt`, **VAT is posted in full** (not reduced), and the payment equals Brutto − Rabatt.

**Worked example A — simple 119€ invoice (buchung_test.go):**
Lines `[{Netto:100, Satz:19, MwSt:19}]`, trinkgeld 0, expenseAccount `6815`, paymentAccount `1800`, rabatt 0:
- Soll `6815` = `100.00`
- Soll `1406` (Vorsteuer 19%) = `19.00`
- Haben `1800` = `119.00`
- Balanced (Soll 119 = Haben 119).

**Worked example B — with rabatt (buchung_test.go):**
Lines `[{Netto:1116.85, Satz:19, MwSt:212.20}]`, expenseAccount `420`, paymentAccount `1270`, rabatt `50`:
- Soll `420` = `round2(1116.85 − 50)` = `1066.85`
- Soll `1406` (Vorsteuer) = `212.20` (full VAT, **not** reduced by rabatt)
- Haben `1270` = `round2(1066.85 + 212.20)` = `1279.05`

#### 3.2 `bewirtung` (entertainment, §4 Abs. 5 EStG — 70% deductible)

The net total is split into a deductible and a non-deductible part by the rule's `abziehbar_prozent`; **full** Vorsteuer is still claimed (entertainment VAT is fully deductible even though only 70% of the cost is):

```
abz   = round2(netTotal × AbziehbarProzent / 100)
nicht = round2(netTotal − abz)
Soll KontoAbziehbar      = abz
Soll KontoNichtAbziehbar = nicht
+ Vorsteuer per rate (§3.6)   ← full VAT of all lines
Haben paymentAccount = round2(Σ Soll)
```

`nicht` is computed as `netTotal − abz` (so the two parts always re-sum to `netTotal`, avoiding a rounding gap).

**Worked example (buchung_test.go, bundled rule: AbziehbarProzent=70, KontoAbziehbar=6640, KontoNichtAbziehbar=6644):**
Lines `[{6.64, 19, 1.26}, {8.41, 7, 0.59}]`, trinkgeld `3.10`, paymentAccount `1800`:
- netTotal = `round2(6.64 + 8.41 + 3.10)` = `18.15`
- abz = `round2(18.15 × 0.70)` = `12.71` → Soll `6640`
- nicht = `round2(18.15 − 12.71)` = `5.44` → Soll `6644`
- Vorsteuer: `1406` = `1.26` (19%), `1401` = `0.59` (7%)
- Haben `1800` = `round2(12.71 + 5.44 + 1.26 + 0.59)` = `20.00`
- Balanced; Haben = 20.00.

#### 3.3 `geschenke` (gifts, with a per-receipt threshold)

The rule carries `Schwelle` (threshold, e.g. `35` €) and two accounts. The branch decides on `netTotal`:

- **If `netTotal > Schwelle`** (over the limit → gift is fully non-deductible, **no Vorsteuer**): post the **gross** to the non-deductible account and return directly (skips the shared Vorsteuer tail):
  ```
  gross = round2(netTotal + SumMwSt(lines))
  Soll KontoNichtAbziehbar = gross
  Haben paymentAccount     = gross
  ```
- **If `netTotal ≤ Schwelle`** (deductible): post net to the deductible account, then the shared tail adds Vorsteuer and the payment entry:
  ```
  Soll KontoAbziehbar = netTotal
  + Vorsteuer per rate (§3.6)
  Haben paymentAccount = round2(Σ Soll)
  ```

> Quirk: the comparison is strict `>` against the *net* total (`netTotal`, which includes trinkgeld), not the gross. A gift whose net exactly equals the threshold counts as deductible.

**Worked examples (buchung_test.go, rule: Schwelle=35, KontoAbziehbar=6610, KontoNichtAbziehbar=6620):**
- ≤35: Lines `[{20, 19, 3.80}]` → Soll `6610`=`20`, Soll `1406`=`3.80`, Haben `1800`=`23.80`. Balanced.
- >35: Lines `[{40, 19, 7.60}]` → Soll `6620`=`47.60` (gross), **no** Vorsteuer line, Haben `1800`=`47.60`. Balanced.

#### 3.4 `reisekosten` (travel) and `kfz` (vehicle costs)

Both post the entire net total to the rule's `DefaultKonto` (ignoring the caller's `expenseAccount`), then the shared Vorsteuer tail:

```
Soll DefaultKonto = netTotal
+ Vorsteuer per rate (§3.6)
Haben paymentAccount = round2(Σ Soll)
```

> Quirk: the `expenseAccount` argument is **ignored** for `reisekosten`, `kfz`, `geschenke`, `bewirtung`, and `reverse_charge`; only `standard` uses it.

**Worked example (buchung_test.go, reisekosten DefaultKonto=6650):**
Lines `[{100, 19, 19}]`, expenseAccount `9999` (ignored), paymentAccount `1800`:
- Soll `6650` = `100`, Soll `1406` = `19`, Haben `1800` = `119`. Account `9999` is never posted.

Bundled `kfz` rule uses `DefaultKonto = 6520`.

#### 3.5 `reverse_charge` (§13b UStG)

The §13b case: the supplier charges no German VAT; the recipient self-assesses VAT and (if entitled) deducts it as Vorsteuer in the **same** booking, so it nets to zero VAT cash. The VAT is computed from the rule's own `RcSatz` (not from the tax lines' rates), applied to the net total. Returns directly (skips the shared tail):

```
net = round2(SumNetto(lines) + trinkgeld)
vat = round2(net × RcSatz / 100)
Soll expenseAccount = net          ← the expense
Soll KontoVStRC     = vat          ← Vorsteuer §13b (input)
Haben KontoUStRC    = vat          ← Umsatzsteuer §13b (output, self-assessed)
Haben paymentAccount = net         ← only the net is actually paid
```

> Quirk: §13b uses `expenseAccount` (the caller's Gegenkonto). The two VAT legs (`KontoVStRC` Soll and `KontoUStRC` Haben) are equal and cancel in cash terms; the payment Haben is only `net`. Balance: Soll = `net + vat`, Haben = `vat + net`. The tax lines' own `satz_prozent`/`mwst_betrag` are **ignored** for the VAT computation — only `RcSatz` and the net matter. (Reverse-charge tax lines typically carry `satz_prozent: 0, mwst_betrag: 0`.)

**Worked example (buchung_test.go, rule: RcSatz=19, KontoVStRC=1407, KontoUStRC=3837):**
Lines `[{Netto:100, Satz:0, MwSt:0}]`, expenseAccount `6300`, paymentAccount `1800`:
- net = `100`, vat = `round2(100 × 0.19)` = `19`
- Soll `6300` = `100`, Soll `1407` = `19`
- Haben `3837` = `19`, Haben `1800` = `100`
- Balanced: Soll 119 = Haben 119.

#### 3.6 Per-rate Vorsteuer (shared tail)

For the fall-through categories (`standard`, `bewirtung`, `geschenke≤Schwelle`, `reisekosten`, `kfz`), after the expense Soll line(s), iterate the tax lines **in order** and for each line with `mwst_betrag ≠ 0`:

1. Resolve the Vorsteuer account for the line's `satz_prozent` (§5.1).
2. If found, append `Soll <vorsteuerKonto> = round2(mwst_betrag)`.
3. If **not found**, skip the line entirely (no posting, no error).

Lines with `mwst_betrag == 0` are skipped (no zero-VAT Vorsteuer line is produced).

> Quirk — missing-account behavior (buchung_test.go): when a line's rate has no Vorsteuer account, its VAT is dropped and the **payment entry shrinks accordingly**. Example: lines `[{100,19,19}, {50,5,2.50}]` with only `19→1406` configured (5% unmapped), standard, paymentAccount `1800`:
> - Soll `6815` = `150` (net total of both lines), Soll `1406` = `19` (19% only).
> - Haben `1800` = `round2(150 + 19)` = `169` — **NOT** the raw gross `171.50`. The 5% VAT (`2.50`) is silently lost but the booking still balances.

#### 3.7 Payment (balancing) entry

For the fall-through categories, after all Soll lines are built:

```
Haben paymentAccount = round2(Σ betrag of all Soll entries built so far)
```

This guarantees `Soll == Haben` exactly. The payment entry is **appended last** in `entries`, so the Haben (Zahlungskonto) entry is the final entry of the list for these categories.

### 4. Revenue-invoice booking — `BuildRevenueBooking`

The mirror of `BuildBooking` for an **outgoing/sales invoice** (income). Signature: `BuildRevenueBooking(rules, lines, revenueAccount, paymentAccount) → Booking`.

- An empty `lines` list returns error `"keine Steuerzeilen für Erlösbuchung"`.
- Build the Haben side first:
  ```
  Haben revenueAccount = round2(SumNetto(lines))     ← the net revenue (Erlös)
  for each line with mwst_betrag ≠ 0:
      resolve Umsatzsteuer account for satz_prozent (§5.2)
      if found: Haben <ustKonto> = round2(mwst_betrag)
      (if not found: skip — payment shrinks, mirrors §3.6)
  habenSum = Σ Haben built so far
  ```
- The Soll (receivable/payment) account:
  ```
  sollKonto = ForderungsKonto if rules.ForderungsKonto ≠ 0, else paymentAccount
  Soll sollKonto = round2(habenSum)
  ```
- The Soll entry is **prepended** (placed first), so the list reads payment-first: `[Soll, Haben revenue, Haben USt…]`.

This implements two VAT-timing modes:
- **Soll-Besteuerung (accrual):** if `ForderungsKonto` is configured (e.g. `1400`), the receivable account is debited at invoice time; payment settlement later switches it to the bank account (§4.1).
- **Ist-Besteuerung (cash basis):** if `ForderungsKonto == 0`, the Soll posts directly to the bank `paymentAccount`.

**Worked example A — accrual disabled (buchung_test.go):**
`rules.UmsatzsteuerKonten = {19: 1776}`, no ForderungsKonto. Lines `[{6500, 19, 1235}]`, revenueAccount `8400`, paymentAccount `1200`:
- Soll `1200` = `7735` (= 6500 + 1235)
- Haben `8400` = `6500`, Haben `1776` = `1235`
- Exactly 3 entries; balanced.

**Worked example B — accrual enabled (buchung_test.go):**
Same lines, `ForderungsKonto = 1400`. paymentAccount `1200` is **ignored**: Soll `1400` = `7735`. Haben unchanged (`8400`=6500, `1776`=1235).

**Worked example C — cash-basis fallback:** no ForderungsKonto, lines `[{100,19,19}]`, paymentAccount `1200` → Soll posts to `1200`.

#### 4.1 Payment settlement — `WithSettlementAccount`

When an outgoing invoice's incoming payment is reconciled (Forderung → Bank), the revenue booking's **single Soll** entry has its account switched to the actual bank account (`bankKonto`), keeping amount and the entire Haben side unchanged:

- Count Soll entries. If there is **not exactly one** Soll entry → return the booking unchanged (no-op).
- Otherwise return a copy where the one Soll entry's `konto` is set to `bankKonto`; `info`, `manuell`, and all Haben entries are preserved.

**Worked example (buchung_test.go):** revenue booking with Soll `1400`=`7735` → `WithSettlementAccount(1200)` → Soll `1200`=`7735`, Haben side (Erlös + USt) unchanged, still balanced.

### 5. Account resolution from rules base

The rules base (`BookingRules`) holds VAT-account maps keyed by **integer-percent strings** and a list of category `Regeln`. The bundled `assets/buchungsregeln.json` (SKR04) is:

```json
{
  "vorsteuer_konten":    { "19": 1406, "7": 1401 },
  "umsatzsteuer_konten": { "19": 3806, "7": 3801 },
  "erloes_konten":       { "inland": 8400, "eu": 8341, "drittland": 8200 },
  "regeln": [
    { "kategorie": "standard",       "name": "Standard-Aufwand" },
    { "kategorie": "bewirtung",      "name": "Bewirtung (§ 4 Abs. 5 EStG)", "abziehbar_prozent": 70, "konto_abziehbar": 6640, "konto_nicht_abziehbar": 6644 },
    { "kategorie": "reverse_charge", "name": "Reverse-Charge (§ 13b UStG)", "rc_satz": 19, "konto_vst_rc": 1407, "konto_ust_rc": 3837 },
    { "kategorie": "geschenke",      "name": "Geschenke", "schwelle": 35, "konto_abziehbar": 6610, "konto_nicht_abziehbar": 6620 },
    { "kategorie": "reisekosten",    "name": "Reisekosten", "default_konto": 6650 },
    { "kategorie": "kfz",            "name": "Kfz-Kosten",  "default_konto": 6520 }
  ]
}
```

> Quirk: the bundled `umsatzsteuer_konten` are `19→3806`, `7→3801` (SKR04 output-VAT accounts). Some tests pass custom maps (e.g. `19→1776`); the engine uses whatever the rules base provides, so document this as configuration, not a constant.

`Rule(kategorie)` returns the first matching rule by **exact, case-sensitive** key, else not-found.

#### 5.1 VorsteuerKonto(satz)

`VorsteuerKonto(satzProzent)`: the percent is rounded to the nearest integer via `int(satz + 0.5)` and converted to a decimal string, then looked up in `vorsteuer_konten`. So `19.0 → "19"`, `7.0 → "7"`, `6.6 → "7"`, `0 → "0"` (not in map → not found). `VorsteuerKonto(0)` returns not-found with the bundled chart.

> Quirk: the `+0.5` rounding means `satz_prozent` is mapped to the nearest integer key. A genuine non-integer rate (none exist in German VAT today) would be rounded.

#### 5.2 UmsatzsteuerKonto(satz)

Same integer-key lookup, against `umsatzsteuer_konten`.

#### 5.3 ErloesKonto(vatID, mwst) — revenue-account routing

Picks the revenue (Erlös) account for an outgoing invoice from the counterparty's VAT-ID and the total VAT amount:

```
key = "drittland"                       (default: non-EU export)
if mwst > 0.005:        key = "inland"   (German VAT charged → domestic)
else if IsEUVatID(vatID): key = "eu"     (0% VAT + EU VAT-ID → intra-EU §18b)
lookup erloes_konten[key]; (0,false) if unset
```

**EU VAT-ID test (`IsEUVatID`):** uppercase-trim the string; require length ≥ 3; the first two characters must be one of the 26 EU country prefixes **excluding DE** (a domestic customer is never an EU/ZM counterparty). The accepted prefixes are:
`AT, BE, BG, CY, CZ, DK, EE, EL` (Greece), `ES, FI, FR, HR, HU, IE, IT, LT, LU, LV, MT, NL, PL, PT, RO, SE, SI, SK`.
`DE…` and any non-EU prefix → false.

**Worked examples (buchungsregeln_test.go, with `{inland:8400, eu:8341, drittland:8200}`):**
- `ErloesKonto("DE123", 19)` → `8400` (VAT > 0 → inland, regardless of DE prefix).
- `ErloesKonto("FI26378052", 0)` → `8341` (0% + EU prefix → eu).
- `ErloesKonto("", 0)` → `8200` (0% + no EU VAT-ID → drittland).
- Unset `erloes_konten` → not-found.

#### 5.4 SuggestKonto(text) — keyword-based Gegenkonto suggestion

For a new supplier, scans free text (typically `supplier-name + " " + Verwendungszweck`) against the optional `konto_stichwoerter` map (keyword → account):

- If the map is empty or text is blank → no suggestion.
- Lowercase both text and each keyword; a keyword matches if it is a **substring** of the text.
- Among matches, the **longest keyword wins** (most specific). Ties keep the first-found longest (map iteration order — undefined, but only matters on equal-length collisions).
- Returns `(account, true)` of the winner, else `(0, false)`.

**Worked examples (buchungsregeln_test.go):**
With `{tankstelle:4663, aral:4663, hotel:4660, telekom:4920}`:
- `"ARAL Tankstelle München"` → `4663` (both "aral" and "tankstelle" match; same account).
- `"Best Western Hotel"` → `4660`; `"Deutsche Telekom GmbH"` → `4920`.
- `"Unbekannter Lieferant XY"` and `""` → no match.
Longest-match: `{bahn:4671, "deutsche bahn":4670}`, text `"Fahrkarte Deutsche Bahn AG"` → `4670` ("deutsche bahn" is longer than "bahn").

### 6. Rounding rules

The single rounding primitive is **round2(v) = round(v × 100) / 100** — round-half-away-from-zero to 2 decimals (standard arithmetic rounding; .005 rounds up). Applied at exactly these points:
- `netTotal` (net total + trinkgeld) once at entry.
- Each split part (`abz`, `nicht`, RC `vat`, gross-only `gross`).
- Each posted VAT amount: `round2(mwst_betrag)`.
- Each balancing/payment entry: `round2(Σ of the other side)`.
- In `BuildRevenueBooking`: `round2(SumNetto)`, each `round2(mwst_betrag)`, and `round2(habenSum)`.

Tolerance for "balanced" and golden-value comparison is **0.005** (see §2).

### 7. Payment (Zahlungskonto) account resolution — `PaymentAccountSKR04`

The Haben (incoming invoice) / Soll-fallback (revenue, cash basis) account is resolved from the selected bank-account name via the user's settings, **before** the booking is built:

1. Find the configured `BankAccount` by exact `Name`.
2. If it has an explicit `SKR04Konto ≠ 0` → use it.
3. Else by `AccountType`: `bank` → `1800`, `cash` → `1600`.
4. `creditcard` and `payroll` types have **no default** and require an explicit `SKR04Konto`; otherwise → `(0, false)`.
5. Unknown account name → `(0, false)`.

When this returns not-found, the UI marks the receipt **not bookable** with reason "no payment account" rather than producing a booking.

### 8. Category source (how `kategorie` is chosen)

The category fed to `BuildBooking` defaults to `"standard"`, but is overridden by a **learned per-company template** (`BookingTemplate`): keyed by the supplier/`Auftraggeber` name, it stores `{kategorie, expense_konto, autobook}`. If a template exists for the company, its `kategorie` (and, for standard, its `expense_konto`) is used; the user can still override via a category dropdown (labels come from each rule's `name`, falling back to its `kategorie` key). Outgoing invoices (`Ausgangsrechnung` checked) bypass categories entirely and use `BuildRevenueBooking`.

### 9. Booking-reference wire format (`BuchungRef`)

`BuchungRef` links a saved invoice row back to the exact bank-statement line it was matched against. The **wire format** stored in `CSVRow.BuchungRef` is a single string:

```
<statementFilename>|<page>|<lineIdx>
```

- `statementFilename` — basename of the statement PDF (e.g. `Konto_...0002.pdf`).
- `page` — **0-based** PDF page index.
- `lineIdx` — **1-based** line index, restarting per page.

Rules:
- An unset ref is the **empty string**; `String()` of an empty `StatementFilename` returns `""`.
- **Parsing** splits on `|`; it must yield **exactly 3 parts**, and parts 2 and 3 must parse as integers. Any deviation (empty, wrong part count, non-numeric) → **zero value, silently** (no error), to tolerate legacy garbage in the column.
- `Display()` (UI label, never persisted): `"<filename> · S.<page+1> Z.<lineIdx>"` (page shown 1-based).

**Sentinel value — cash-register confirmation:** the constant `kassenbuch|0|0` (`CashConfirmedRef`) marks a cash receipt as confirmed against the generated Kassenbuch. There is no external cash statement to link to, so this sentinel gives cash receipts the same "✓" treatment as bank-matched invoices (any non-empty `BuchungRef` renders as ✓). Re-implementers must treat `kassenbuch|0|0` as a magic confirmed-marker, not a real statement link.

> Quirk: `ParseBuchungRef("kassenbuch|0|0")` parses fine to `{kassenbuch, 0, 0}`, but callers special-case the literal string `CashConfirmedRef` and skip statement-line lookup for it.

A related but **separate** structure, `InvoiceRef`, points the other direction (from a statement line to a saved invoice) and is stored as `{month_folder, filename}`, rendered as `"MonthFolder/Filename"` (or just `Filename` when the folder is empty). Statement lines also carry their own `StatementBooking` shape (`page`, `line_idx`, `date` as `"DD.MM.YYYY"` or `"DD.MM."`, geometry in PDF points, `text`, `betrag`, `gutschrift` flag, optional `invoice_ref`), with display label `"S.<page+1> Z.<lineIdx> — <date>"`.

### 10. Booking decomposition helpers (used by exporters)

- **PaymentEntry()**: returns the single Haben entry (Zahlungskonto), `ok=false` unless there is **exactly one** Haben entry.
- **DebitEntries()**: all Soll entries (the expense/Vorsteuer lines).
- **PaymentAndCounters(isRevenue)**: splits a booking into one *base* entry and ≥1 *counter* entries.
  - Incoming (`isRevenue=false`): base = the single **Haben**; counters = the Soll entries.
  - Revenue (`isRevenue=true`): base = the single **Soll**; counters = the Haben entries.
  - Returns `ok=false` unless the base side has **exactly one** entry **and** there is ≥1 counter. Exporters skip a booking when `ok=false` (e.g. an ambiguous booking with two Soll entries for revenue).

### Re-implementation checklist

Must-match behaviors for the booking engine:

1. **TaxLine**: store/serialize `netto, satz_prozent, mwst_betrag`; trust stored `mwst_betrag` (never recompute from net×rate). Empty list ↔ `""`; invalid JSON → nil. Trinkgeld is untaxed and added to net.
2. **round2** = round-half-away-from-zero to 2 decimals; **balance tolerance 0.005**; `almost()` = `|a−b| < 0.005`.
3. **standard**: Soll `expenseAccount = round2(netTotal − rabatt)`; full VAT; payment = Σ Soll. (Method B discount: VAT not reduced.)
4. **bewirtung**: `abz = round2(netTotal × pct/100)`, `nicht = round2(netTotal − abz)`, full Vorsteuer of all lines.
5. **geschenke**: strict `>` against `Schwelle` on the **net** total. Over → gross to non-deductible account, no Vorsteuer, return directly. ≤ → net to deductible account + Vorsteuer.
6. **reisekosten/kfz**: post net to rule `DefaultKonto` (ignore caller's `expenseAccount`) + Vorsteuer.
7. **reverse_charge (§13b)**: `vat = round2(net × RcSatz/100)` from the **rule's** rate, not the lines; Soll expense+VSt, Haben USt+net; the two VAT legs cancel; payment Haben = net only.
8. **Per-rate VAT posting**: skip lines with `mwst_betrag == 0`; resolve account by `int(satz+0.5)` integer key; **silently drop** VAT for unmapped rates (payment entry shrinks; booking still balances — never post raw gross).
9. **Payment entry** is always `round2(Σ other side)` so Soll == Haben by construction; appended last (incoming) or Soll prepended first (revenue).
10. **BuildRevenueBooking**: Haben `revenueAccount` = net + per-rate USt; Soll = `ForderungsKonto` if set else `paymentAccount`; `WithSettlementAccount` switches the single Soll's account (no-op unless exactly one Soll).
11. **ErloesKonto routing**: `mwst > 0.005 → inland`; else EU VAT-ID → `eu`; else `drittland`. `IsEUVatID` = 2-letter EU prefix (the 26 listed, EL for Greece, **DE excluded**), length ≥ 3.
12. **SuggestKonto**: case-insensitive substring; longest keyword wins.
13. **VorsteuerKonto/UmsatzsteuerKonto**: integer-percent string keys; bundled SKR04 = VSt `19→1406, 7→1401`, USt `19→3806, 7→3801`.
14. **PaymentAccountSKR04**: explicit `SKR04Konto` wins; else `bank→1800`, `cash→1600`; creditcard/payroll need explicit; else not-bookable.
15. **BuchungRef wire format**: `"<filename>|<page0based>|<lineIdx1based>"`; parse requires exactly 3 parts with integer page/line, else silent zero value; sentinel `kassenbuch|0|0` = cash-confirmed marker.
16. Unknown category → error `"unbekannte Buchungskategorie: <k>"`; known-but-unhandled → `"Buchungskategorie ohne Buchungslogik: <k>"`; empty revenue lines → `"keine Steuerzeilen für Erlösbuchung"`.

---

## Revenue & Outgoing Invoices

This chapter specifies how BuchISY records **outgoing invoices** (German *Ausgangsrechnungen*) — invoices the company *issues* to its customers, which generate revenue (*Erlöse*) and output VAT (*Umsatzsteuer*). It covers the `ausgangsrechnung` flag, the revenue booking shape, revenue-account selection (`Erlöskonten`), the Soll-Besteuerung receivable model (*Forderung* booked then cleared to Bank on payment), how revenue rows are exported, and how they feed the derived tax/controlling reports.

### 1. The `Ausgangsrechnung` flag and storage

Every invoice record (a `CSVRow`) carries a boolean field `Ausgangsrechnung`:

- `false` (default) = **incoming** invoice / expense (a supplier billed *us*).
- `true` = **outgoing** invoice / revenue (we billed a *customer*).

This flag is the single switch that selects revenue behavior throughout the system. It persists in the database and survives round-trips (regression-tested: insert a row with `Ausgangsrechnung: true`, list it, the flag is still `true`).

Related fields on the same row that take on revenue meaning when `Ausgangsrechnung == true`:

| Field | Type | Meaning for an outgoing invoice |
|---|---|---|
| `Ausgangsrechnung` | bool | `true` |
| `VATID` | string | the **customer's** VAT-ID (used to classify EU vs. domestic for the ZM and Erlöskonto) |
| `Auftraggeber` | string | the customer name (reused as the booking text / partner) |
| `Bankkonto` | string | the bank account expected to *receive* the payment |
| `Unterordner` | string | `"Ausgangsrechnungen"` when the invoice file is filed in the outgoing-invoice subfolder |
| `BuchungRef` | string | once set (`"<statementFilename>|<page>|<lineIdx>"`), the incoming payment has been reconciled |
| `Bezahldatum` | string (DD.MM.YYYY) | payment date; non-empty drives immediate settlement at entry |

> Quirk: There are **two** signals that an invoice is outgoing: the `Ausgangsrechnung` boolean *and* `Unterordner == "Ausgangsrechnungen"`. The editor UI seeds its checkbox from `row.Ausgangsrechnung || row.Unterordner == "Ausgangsrechnungen"`. A re-implementer should treat either signal as "outgoing" in the UI seed but persist the authoritative boolean.

The invoice-file storage subfolder is chosen by a helper: if `ausgangsrechnung` is true the subfolder is the literal string `"Ausgangsrechnungen"`; otherwise it is derived from the bank account.

### 2. Revenue account selection (`Erlöskonten`)

Outgoing invoices post their net to a **revenue account** (*Erlöskonto*) chosen from the counterparty VAT-ID and the VAT amount. The booking rules carry a map `erloes_konten` keyed by three string keys: `"inland"`, `"eu"`, `"drittland"`.

**Selection algorithm** (`ErloesKonto(vatID, mwst)`), evaluated in this order:

1. If `mwst > 0.005` (i.e. VAT was charged) → key = `"inland"` (domestic taxable sale).
2. Else if the VAT-ID is an EU VAT-ID of *another* member state (`IsEUVatID(vatID)`) → key = `"eu"` (§18b intra-EU supply, 0% VAT).
3. Else → key = `"drittland"` (export to a third country / no VAT).

Return the mapped account, or `(0, false)` if that key is unmapped.

**`IsEUVatID(s)`**: uppercase-trim `s`; return true iff length ≥ 3 and the first two characters are one of these EU country prefixes — explicitly **excluding DE** (a domestic customer is never EU for ZM purposes):

```
AT BE BG CY CZ DK EE EL ES FI FR HR HU IE IT LT LU LV MT NL PL PT RO SE SI SK
```

Note `EL` (Greece), not `GR`. `DE` is intentionally absent.

**Default `erloes_konten` (bundled, SKR03 numbering in the shipped JSON):**

```
inland=8400, eu=8341, drittland=8200
```

**Standard accounts per chart variant** (`StandardSKR`), applied when the user switches chart of accounts:

| Variant | Erlös inland | Erlös EU | Erlös Drittland | USt 19% | USt 7% |
|---|---|---|---|---|---|
| SKR03 | 8400 | 8341 | 8200 | 1776 | 1771 |
| SKR04 | 4400 | 4125 | 4120 | 3806 | 3801 |

> Quirk: The shipped `assets/buchungsregeln.json` mixes conventions — it uses SKR04-style Vorsteuer/Umsatzsteuer accounts (`vorsteuer_konten 19→1406`, `umsatzsteuer_konten 19→3806`) but SKR03-style Erlöskonten (`8400/8341/8200`). A profile only becomes internally consistent after `ApplySKRVariant` is run. Replicate the bundled file verbatim as the un-migrated default.

In the editor, when the user ticks the *Ausgangsrechnung* checkbox the app auto-suggests the Gegenkonto via `ErloesKonto(VATID, SumMwSt(lines))` — but only if the user has **not** already manually picked an account in this dialog session (`accountManuallyPicked` guard). The suggestion also fires on dialog open if the extractor already classified the invoice as outgoing.

### 3. The revenue booking (`BuildRevenueBooking`)

A revenue booking is the **mirror** of an expense booking. Where an expense booking has one Haben (the payment account) and several Soll lines, a revenue booking has one **Soll** (the money owed/received) and several **Haben** lines (revenue + output VAT).

**Inputs:** the rules base, the tax lines, `revenueAccount` (chosen Erlöskonto), `paymentAccount` (the cash-basis fallback account, see §5).

**Algorithm:**

1. If there are no tax lines → error `"keine Steuerzeilen für Erlösbuchung"`.
2. Build the **Haben** side:
   - One entry: `revenueAccount`, amount = `round2(SumNetto(lines))`, `Soll=false`.
   - For each tax line with `MwStBetrag != 0`: look up the **Umsatzsteuer** account for that line's rate (`UmsatzsteuerKonto(satzProzent)`); if found, add a Haben entry of `round2(MwStBetrag)` on that account. If the rate has no configured USt account, **skip** that VAT (it is not posted).
3. `habenSum` = Σ of all Haben entries so far.
4. Choose the **Soll** account (`sollKonto`):
   - If `ForderungsKonto != 0` → `sollKonto = ForderungsKonto` (Soll-Besteuerung; see §4).
   - Else → `sollKonto = paymentAccount` (cash-basis fallback; see §5).
5. **Prepend** one Soll entry: `sollKonto`, amount = `round2(habenSum)`, `Soll=true`. Prepending makes the booking read payment-first.

Because the Soll amount is computed as the sum of the Haben side, the booking **always balances**, even when a rate's USt account is missing.

**Rate→account matching:** `UmsatzsteuerKonto(satzProzent)` keys the `umsatzsteuer_konten` map by `int(satzProzent + 0.5)` rendered as a decimal string, i.e. `19.0 → "19"`, `7.0 → "7"`. Same rounding rule as Vorsteuer.

**Worked example (domestic 19% sale, golden values from tests):**

Tax line: Netto 6500.00, Satz 19%, MwSt 1235.00 (gross 7735.00). Erlöskonto 8400, USt account 1776, paymentAccount 1200, **no** ForderungsKonto.

Resulting booking (exactly 3 entries):

| Konto | Betrag | Soll/Haben | Meaning |
|---|---|---|---|
| 1200 | 7735.00 | **S** | money owed/received (Zahlungskonto, cash-basis) |
| 8400 | 6500.00 | **H** | Erlös (net revenue) |
| 1776 | 1235.00 | **H** | Umsatzsteuer 19% |

`SollSum = HabenSum = 7735.00`; `Balanced() == true`.

**`Balanced()`** is true iff there is ≥ 1 entry and `|SollSum − HabenSum| < 0.005`.

### 4. Soll-Besteuerung: the receivable (Forderung) model

When the profile configures a `forderungskonto` (e.g. **1400**, *Forderungen aus Lieferungen und Leistungen*), BuildRevenueBooking uses **accrual / Soll-Besteuerung**: at invoice entry the Soll side posts to the **receivable** account rather than the bank. The same 6500/1235 example with `ForderungsKonto: 1400` (and paymentAccount 1200 **ignored**):

| Konto | Betrag | Soll/Haben |
|---|---|---|
| 1400 | 7735.00 | **S** (open receivable) |
| 8400 | 6500.00 | H |
| 1776 | 1235.00 | H |

This represents: *the customer owes us 7735.00; revenue 6500 and USt 1235 are recognized now.*

**Settlement on payment (`WithSettlementAccount(bankKonto)`):** when the incoming payment is later reconciled to a bank-statement line, the booking is transformed by switching the single Soll entry's account from the receivable to the actual bank account (Forderung → Bank). Algorithm:

1. Count Soll entries. If **not exactly one**, return the booking unchanged (no-op).
2. Copy the booking; for the single Soll entry, set `Konto = bankKonto` (amount and the Haben side are untouched).

After settlement of the example with `bankKonto = 1200`:

| Konto | Betrag | Soll/Haben |
|---|---|---|
| 1200 | 7735.00 | **S** (now Bank) |
| 8400 | 6500.00 | H |
| 1776 | 1235.00 | H |

The Erlös + USt (Haben) lines are **never** changed by settlement; only the receivable→bank swap happens. The settled booking still balances.

**When settlement fires** (three triggers, all using the row's bank account → `PaymentAccountSKR04`):

- **At entry, if `Bezahldatum`/payment-date is set:** the editor immediately calls `WithSettlementAccount(pay)` so a paid invoice is stored already cleared to the bank. This applies to both the live preview and the persisted booking.
- **During Erlös reconciliation (`ErloesAbgleich`):** when the user confirms a match between an outgoing invoice and an incoming credit line on a bank statement, the app sets `BuchungRef` and calls `WithSettlementAccount(pay)`. This happens in single-confirm, bulk-confirm (★ high-confidence), link-all, and partial-confirm paths.

> Quirk: Settlement is **idempotent only by luck**. `WithSettlementAccount` just sets the Soll account to `bankKonto`. If called twice with different bank accounts the last one wins; if the booking has been hand-edited to ≥ 2 Soll entries it silently no-ops and the receivable is never cleared. A re-implementer must keep the "exactly one Soll entry" guard.

> Quirk: In the **cash-basis** default (no `forderungskonto`), the Soll account is already the bank/cash account, so settlement is a no-op swap (bank→bank). The reconciliation flow still calls it; behavior is unchanged. This is intentional.

### 5. Cash-basis fallback

If `ForderungsKonto == 0` (the bundled default — no `forderungskonto` key), the revenue Soll posts **directly to the payment account** (`paymentAccount`). Test golden: with no ForderungsKonto, the single Soll entry's account equals the passed `paymentAccount` (e.g. 1200). This models *Ist-Besteuerung* (cash receipts), where no separate open-item receivable is tracked.

`paymentAccount` is resolved from the invoice's `Bankkonto` name via **`PaymentAccountSKR04(bankAccountName)`**:

1. Find the configured bank account by name. If none → `(0, false)` and the booking is "not bookable" (reason key `booking.no.payment.account`).
2. If it has an explicit `SKR04Konto != 0` → use it.
3. Else by account type: `Bank → 1800`, `Cash → 1600`.
4. Credit-card and payroll accounts have **no** universal default → `(0, false)` (must set an explicit account).

### 6. Splitting a booking for export: `PaymentAndCounters`

Exports and reconciliation need to identify the "anchor" account (the **Gegenkonto** in DATEV terms) and the lines posted against it. `PaymentAndCounters(isRevenue)` does this **direction-aware**:

- Iterate the entries. An entry is the **base** when `(isRevenue && e.Soll) || (!isRevenue && !e.Soll)`:
  - **Revenue invoice** (`isRevenue == true`): the base is the single **Soll** (the receivable/bank).
  - **Incoming invoice** (`isRevenue == false`): the base is the single **Haben** (the payment account).
- All other entries are **counters**.
- Return `ok = false` (booking skipped by exporters) unless there is **exactly one** base entry **and at least one** counter.

`isRevenue` is supplied as the row's `Ausgangsrechnung` flag everywhere this is called.

Worked split for the revenue example `{1200 S 119, 8400 H 100, 1776 H 19}` with `isRevenue = true`: base = `1200` (119, Soll), counters = `[8400 H 100, 1776 H 19]`, ok = true.

> Quirk / historical bug: An earlier implementation used `PaymentEntry()`, which requires exactly **one Haben** entry. Revenue bookings have *two* Haben (Erlös + USt), so they were wrongly rejected and dropped from exports. `PaymentAndCounters` replaced it. A re-implementer must use the direction-aware split, not "the single credit entry," and the dedicated regression test (`TestClassifyForExport_RevenueNotSkipped`) must pass.

### 7. Export classification

`ClassifyForExport(rows, includeExported)` partitions rows into `Exportable`, `AlreadyExported`, `Skipped` (each skip carries a reason):

For each row:
1. Compute `PaymentAndCounters(r.Ausgangsrechnung)`.
2. If **not** `Balanced()` **or** not `ok` → **Skipped**. Reason = `"keine Buchung"` if the booking has zero entries, else `"nicht ausgeglichen"`.
3. Else if `r.Exportiert` is true → **AlreadyExported** (and also added to **Exportable** only when `includeExported == true`).
4. Else → **Exportable**.

A balanced revenue booking (1 Soll + 2 Haben, `Ausgangsrechnung: true`) lands in **Exportable**, not Skipped.

### 8. Revenue rows in the DATEV export

`BuildDATEVStapel(header, rows)` emits an EXTF *Buchungsstapel*. Per row it calls `PaymentAndCounters(r.Ausgangsrechnung)`; rows that are unbalanced or `ok == false` are **skipped** (counted in the returned `skipped`). Each **counter** entry becomes one data line; the **base** entry's account becomes the line's **Gegenkonto** and is *never itself a data row*.

**Per data line, the Soll/Haben-Kennzeichen comes from the counter entry's own side:** `"S"` if `e.Soll` else `"H"`. For a revenue invoice the counters are Haben (Erlös, USt), so they emit `"H"` lines — the inverse of an expense invoice whose counters emit `"S"`.

**Header line 1** (semicolon-separated, CRLF-terminated), with interpolated header fields:
```
"EXTF";700;21;"Buchungsstapel";13;{ErzeugtAm};;;;;"{BeraterNr}";"{MandantNr}";{WJBeginn};4;{DatumVon};{DatumBis};"";"";"";"";0;"EUR";"";"";"";""
```

**Header line 2** (column captions, verbatim):
```
Umsatz (ohne Soll/Haben-Kz);Soll/Haben-Kennzeichen;WKZ Umsatz;Kurs;Basis-Umsatz;WKZ Basis-Umsatz;Konto;Gegenkonto (ohne BU-Schlüssel);BU-Schlüssel;Belegdatum;Belegfeld 1;Belegfeld 2;Skonto;Buchungstext
```

**Each data line** (in this exact positional order):
```
{amount};"{S|H}";"EUR";;;;{counter.Konto};{base.Konto};;{beleg};"{belegfeld1}";"{belegfeld2}";;"{text}"
```
where:
- `amount` = `datevAmount` = `"%.2f"` with the decimal point replaced by a comma, **unsigned** (e.g. `6500,00`). Columns *WKZ Umsatz/Kurs/Basis-Umsatz/WKZ Basis-Umsatz* and *BU-Schlüssel*, *Skonto* are left empty.
- `beleg` = `datevBeleg(Rechnungsdatum)` = first two dot-separated parts concatenated = **DDMM** (e.g. `10.12.2025 → 1012`); empty if the date has fewer than two parts.
- `belegfeld1` = `Belegnummer` if non-empty, else `Rechnungsnummer`; then `datevClean(..., 36)`.
- `belegfeld2` = `datevClean(Rechnungsnummer, 36)`.
- `text` = `datevClean(trim(Auftraggeber + " " + Verwendungszweck), 60)`.
- `datevClean(s, max)` removes all `"`, replaces CR/LF with spaces, and truncates to `max` **runes** (UTF-8 safe).

**Worked DATEV revenue example** (golden from `TestDATEVRevenueRow`): row `Rechnungsdatum 10.12.2025`, `Belegnummer 2025-0002`, `Auftraggeber Symeo`, `Ausgangsrechnung true`, booking `{1200 S 7735, 8400 H 6500, 1776 H 1235}`. Returns `exported = 2, skipped = 0`. The two data lines (base/Gegenkonto = 1200):
```
6500,00;"H";"EUR";;;;8400;1200;;1012;"2025-0002";"";;"Symeo"
1235,00;"H";"EUR";;;;1776;1200;;1012;"2025-0002";"";;"Symeo"
```

### 9. Revenue rows in the Lexware export

`BuildLexwareCSV(rows)` emits a semicolon-separated CSV with header:
```
Datum;Belegnr;Buchungstext;Betrag;Sollkonto;Habenkonto
```
Per row, same `PaymentAndCounters(r.Ausgangsrechnung)` gate (unbalanced/`!ok` skipped). Each counter → one line. **Soll/Haben assignment is reconstructed per counter:** start `soll = counter.Konto, haben = base.Konto`; if the counter is a **Haben** entry (`!e.Soll`, the revenue case) **swap** them so `soll = base.Konto, haben = counter.Konto`. Net effect for a revenue line: `Sollkonto = base (1200/Forderung/Bank)`, `Habenkonto = Erlös/USt`.

Fields per line:
- `Datum` = `Rechnungsdatum` (DD.MM.YYYY, unchanged).
- `Belegnr` = `Belegnummer` if non-empty else `Rechnungsnummer`, run through `lexClean`.
- `Buchungstext` = `lexClean(trim(Auftraggeber + " " + Verwendungszweck))`.
- `Betrag` = `"%.2f"` with point→comma (e.g. `6500,00`), unsigned.
- `lexClean` replaces `;` with `,` and CR/LF with spaces.

**Worked Lexware revenue example** (golden from `TestLexwareRevenueRow`): same Symeo row. `exported = 2`. The Erlös line:
```
10.12.2025;2025-0002;Symeo;6500,00;1200;8400
```
(Sollkonto = base 1200, Habenkonto = Erlös 8400.)

### 10. Revenue in derived reports

The `Ausgangsrechnung` flag changes how a row contributes to the tax/controlling reports:

**Official UStVA (`ComputeUStVAOfficial`)** — rows are first converted to EUR. For each row classify by Kennzahl:
- If `Ausgangsrechnung`:
  - VAT charged (`SumMwSt > 0.005`): per line, 19% net → **Kz81**, 7% net → **Kz86** (domestic taxable sale, base amounts).
  - Else if EU customer VAT-ID: net → **Kz21** (§18b intra-EU sonstige Leistungen).
  - Else: net → **Kz45** (other non-taxable, place of supply abroad).
- If incoming: §13b → Kz84, else Vorsteuer → Kz66.

Derived: `USt81 = round2(Kz81 × 0.19)`, `USt86 = round2(Kz86 × 0.07)`, `Kz83 (Zahllast) = round2((USt81 + USt86 + Kz85) − (Kz66 + Kz67))`.

**Zusammenfassende Meldung (`ComputeZM`)** — sums net **only** for rows where `Ausgangsrechnung == true` AND `IsEUVatID(VATID)` AND `SumMwSt(TaxLines) == 0` (intra-EU reverse-charge sales). Grouped per uppercased/trimmed customer VAT-ID, rounded, sorted ascending by VAT-ID, with a control total (`Kontrollsumme`).

**Open items / OPOS (`ComputeOpenItems`)** — a row is OPEN iff `Bezahldatum == ""` AND `BuchungRef == ""`. An open `Ausgangsrechnung` is a **Forderung** (receivable); an open incoming invoice is a **Verbindlichkeit** (payable). `Betrag = Bruttobetrag`. Once a payment is reconciled (`BuchungRef` set) or a `Bezahldatum` is entered, the receivable drops off OPOS — consistent with the Forderung→Bank settlement.

**Controlling / GuV** — revenue is recognized from the **Haben** entries of bookings on non-tax, non-payment accounts (i.e. the Erlöskonten). `AggregateControlling` excludes VAT and payment accounts, then treats Haben as Einnahmen and Soll as Ausgaben. So the Erlös Haben line (e.g. 8400, 6500.00) feeds *Einnahmen*; the USt Haben line is excluded (it is a configured Umsatzsteuer account).

### Re-implementation checklist

Must-match behaviors for revenue & outgoing invoices:

1. **`Ausgangsrechnung` boolean** persists and is the master switch; the UI also treats `Unterordner == "Ausgangsrechnungen"` as "outgoing" for the editor checkbox seed, but the boolean is authoritative.
2. **Revenue booking is the mirror of expense:** one Soll (receivable or bank), Haben = Erlös (net) + one Umsatzsteuer line per VAT rate. Soll amount = Σ Haben, so it always balances even when a rate's USt account is unmapped (that VAT is simply not posted).
3. **Erlöskonto selection:** `mwst > 0.005 → inland`; else EU VAT-ID → `eu`; else `drittland`. `IsEUVatID` uses the exact EU prefix set including `EL`, excluding `DE`, min length 3.
4. **Soll-Besteuerung:** with `forderungskonto` set, Soll posts to the receivable (e.g. 1400) at entry; `WithSettlementAccount` swaps that single Soll account to the bank account on payment (no-op unless exactly one Soll entry; Haben side never touched). Without `forderungskonto`, Soll = the bank/cash account directly (cash-basis).
5. **Settlement triggers:** at entry when `Bezahldatum` is set, and on every Erlös-reconciliation confirm path; resolves the bank account via `PaymentAccountSKR04` (explicit account → else Bank=1800 / Cash=1600; credit-card/payroll require explicit).
6. **`PaymentAndCounters(isRevenue)`** must be direction-aware: base = single Soll for revenue (single Haben for incoming); `ok=false` unless exactly one base and ≥ 1 counter. Do **not** use "the single Haben entry" — that drops every revenue invoice.
7. **DATEV:** counter side drives the S/H Kennzeichen (revenue counters emit `"H"`); base account = Gegenkonto and never its own data row; Belegdatum = DDMM; Belegfeld 1 = Belegnummer (fallback Rechnungsnummer), Belegfeld 2 = Rechnungsnummer; comma decimals, unsigned; runes truncated to 36/60; CRLF lines; exact header strings.
8. **Lexware:** revenue lines map `Sollkonto = base`, `Habenkonto = Erlös/USt` (swap when counter is Haben); comma decimals; Belegnummer preferred.
9. **Golden numbers to reproduce exactly:** net 6500 / VAT 1235 / gross 7735 → Soll receivable-or-bank 7735, Haben 8400=6500, Haben USt(1776)=1235; DATEV lines `6500,00;"H";...;8400;1200;;1012;"2025-0002"` and `1235,00;"H";...;1776;1200;...`; Lexware `10.12.2025;2025-0002;Symeo;6500,00;1200;8400`.
10. **Derived reports:** outgoing + VAT>0 → UStVA Kz81/Kz86 (net base); outgoing + EU + 0% VAT → Kz21 and a ZM line; open outgoing → Forderung in OPOS (drops off once `BuchungRef` or `Bezahldatum` is set); Erlös Haben feeds controlling Einnahmen.

---

## VAT Filings: UStVA & ZM

This chapter specifies the two periodic VAT filings BuchISY produces: the **Umsatzsteuer-Voranmeldung (UStVA)** — the German preliminary VAT return — and the **Zusammenfassende Meldung (ZM)** — the EC Sales List of intra-EU supplies. It also specifies the XML export format for both. This is the most correctness-critical subsystem: the numbers fed to the tax authority must reproduce exactly.

There are **two distinct UStVA computations** in the code, kept separately:

1. **Account-based UStVA** (the "legacy"/ledger view) — sums actual VAT amounts posted to the configured Vorsteuer/Umsatzsteuer accounts. Output structure: per-account lines plus totals and Zahllast. Not used by the visible UStVA dialog and not exported to XML; it exists as an alternate cross-check based on booking entries.
2. **Official UStVA** (the ELSTER Kennzahlen view) — classifies each *invoice* into the official ELSTER Kennzahlen from invoice metadata (net bases, derived VAT, Zahllast). This is what the UStVA dialog displays and what the XML/PDF export uses.

A re-implementer must implement **both**, because they consume different inputs (booking entries vs. invoice tax-lines) and a faithful port preserves both code paths.

---

### 1. Shared input data

Both computations operate on a list of invoice rows (`CSVRow`). The relevant fields:

| Field | Type | Meaning |
|---|---|---|
| `Ausgangsrechnung` | bool | `true` = outgoing/revenue invoice (Erlös); `false` = incoming/expense |
| `VATID` | string | Counterparty VAT-ID: customer (if outgoing) or supplier (if incoming) |
| `TaxLines` | list of TaxLine | The VAT breakdown of the receipt |
| `Waehrung` | string | Currency code; blank or `"EUR"` = euro |
| `Wechselkurs` | decimal | Foreign-currency exchange rate (units of foreign per 1 EUR division basis; see currency note) |
| `Buchung.Entries` | list of BookingEntry | The double-entry postings (used only by the account-based UStVA) |

A **TaxLine** has three fields:
- `Netto` (decimal) — net amount
- `SatzProzent` (decimal) — VAT rate in percent (e.g. `19`, `7`, `0`)
- `MwStBetrag` (decimal) — VAT amount for that line

A **BookingEntry** has: `Konto` (int account number), `Betrag` (decimal), `Soll` (bool; `true` = debit/Soll, `false` = credit/Haben).

Helper aggregations over a row's tax lines:
- `SumNetto(lines)` = Σ `Netto`
- `SumMwSt(lines)` = Σ `MwStBetrag`

**Rounding.** A single rounding primitive is used everywhere: `round2(v) = round(v × 100) / 100` using round-half-away-from-zero (standard `math.Round`: ties round away from zero, e.g. 2.5→3, −2.5→−3). All monetary outputs are rounded to 2 decimals via this function.

---

### 2. Currency normalization (applies to Official UStVA and ZM)

Before the official UStVA and the ZM aggregate, rows are normalized to EUR via `RowsEUR`. (The **account-based UStVA does NOT call `RowsEUR`** — it sums booking entries, which are already booked in EUR.)

`RowEUR(row)` rules:
- If `Waehrung` is blank or `"EUR"` → row returned unchanged.
- If foreign (`Waehrung` non-empty and ≠ `"EUR"`) **and** `Wechselkurs ≤ 0` (rate missing) → row passed through **at face value** (amounts NOT converted). This is a deliberate quirk: a foreign invoice with no rate contributes its raw foreign-currency numbers to the filing.
- If foreign **and** `Wechselkurs > 0` → every money field is divided by the rate and `round2`'d. For tax lines: each line's `Netto` and `MwStBetrag` become `round2(value / kurs)`; `SatzProzent` is never changed.

> Quirk: conversion is *division* by `Wechselkurs` (so the rate is "foreign units per EUR"). Each tax line is rounded independently after dividing, so the converted `SumNetto`/`SumMwSt` can differ by ±0.01 from converting the aggregate.

---

### 3. Official UStVA — the ELSTER Kennzahlen

This is the primary, exported UStVA. Output structure `UStVAOfficial` has these fields:

| Field | ELSTER Kz | Meaning |
|---|---|---|
| `Kz81` | 81 | Steuerpflichtige Umsätze 19 % — taxable sales at 19 %, **net base** |
| `Kz86` | 86 | Steuerpflichtige Umsätze 7 % — taxable sales at 7 %, **net base** |
| `Kz21` | 21 | Nicht steuerbare innergem. sonstige Leistungen (§ 18b UStG) — intra-EU services, **net** |
| `Kz45` | 45 | Übrige nicht steuerbare Umsätze (Leistungsort nicht im Inland) — other non-taxable sales abroad, **net** |
| `Kz84` | 84 | § 13b Bemessungsgrundlage — reverse-charge purchase base, **net** |
| `Kz85` | 85 | § 13b Steuer — VAT on the reverse-charge base |
| `Kz66` | 66 | Vorsteuer aus Rechnungen anderer Unternehmer — deductible input VAT from supplier invoices |
| `Kz67` | 67 | Vorsteuer aus § 13b-Leistungen — input VAT on reverse-charge supplies |
| `USt81` | (derived) | = `Kz81 × 19 %` — output VAT on the 19 % base |
| `USt86` | (derived) | = `Kz86 × 7 %` — output VAT on the 7 % base |
| `Kz83` | 83 | Verbleibende Vorauszahlung / Überschuss — the **Zahllast** (positive = owed) or Überschuss (negative = refund) |

`USt81` and `USt86` are computed/displayed values used to derive Kz 83 but are **not** ELSTER Kennzahlen of their own.

#### 3.1 Per-invoice classification algorithm

The reverse-charge rate `rcSatz` defaults to `19.0`. If a booking rule with category `"reverse_charge"` exists and its `RcSatz > 0`, that value is used instead.

For each row (after `RowsEUR`):

Let `net = SumNetto(row.TaxLines)` and `vat = SumMwSt(row.TaxLines)`.

```
if row.Ausgangsrechnung:                      # outgoing / sale
    if vat > 0.005:                           # domestic taxable sale (VAT charged)
        for each tax line l:
            r = round(l.SatzProzent + 0.5)     # integer rate
            if r == 19: Kz81 += l.Netto
            if r == 7:  Kz86 += l.Netto
            # any other rate contributes to NEITHER Kz81 nor Kz86
    else if IsEUVatID(row.VATID):             # 0% sale to an EU customer
        Kz21 += net                            # intra-EU service (§18b)
    else:                                      # 0% sale, non-EU / no EU VAT-ID
        Kz45 += net                            # non-taxable foreign sale
else:                                          # incoming / purchase
    if IsEUVatID(row.VATID) and vat < 0.005:  # §13b reverse-charge purchase
        Kz84 += net
    else:                                      # normal purchase with input VAT
        Kz66 += vat
```

Notes and quirks a re-implementer must replicate:
- The "VAT charged" test uses the **sum of VAT over all tax lines** with threshold `> 0.005` (and `< 0.005` for the zero test). A row whose lines sum to a VAT below half a cent counts as 0 %.
- Integer rate matching uses `round(SatzProzent + 0.5)`, i.e. `floor`-style rounding: `18.5 → 19`, `7.0 → 7`, `7.4 → 7`. Only exactly-19 and exactly-7 (after this rounding) feed Kz 81/86. A 19 %-charged sale with, say, a 16 % line would post that line's net to neither Kz — a silent gap.
- Kz 81/86 sum **per line netto**, but the domestic/foreign branch decision uses the **row-level** `vat`. So a single outgoing row is wholly domestic or wholly non-taxable; the per-line split only chooses between Kz 81 and Kz 86 within a domestic sale.
- `Kz66` accumulates the **VAT** (`vat`), not the net, of every non-reverse-charge incoming invoice — including domestic ones (a `DE` supplier VAT-ID still lands in Kz 66, because it's not an EU-other VAT-ID and/or carries VAT).
- An **incoming** invoice with an EU VAT-ID but VAT > 0 (i.e. `vat ≥ 0.005`) falls into the `else` and goes to Kz 66 — it is *not* treated as §13b.

#### 3.2 Derived values (after the loop)

In order:
```
Kz81 = round2(Kz81)
Kz86 = round2(Kz86)
Kz21 = round2(Kz21)
Kz45 = round2(Kz45)
Kz84 = round2(Kz84)
Kz66 = round2(Kz66)
Kz85 = round2(Kz84 × rcSatz / 100)        # VAT on reverse-charge base
Kz67 = Kz85                                # input VAT equals output VAT (fully deductible) → net zero
USt81 = round2(Kz81 × 0.19)
USt86 = round2(Kz86 × 0.07)
Kz83 = round2( (USt81 + USt86 + Kz85) − (Kz66 + Kz67) )
```

`Kz67 = Kz85` exactly (same rounded value), so §13b is VAT-neutral in the Zahllast: it adds `Kz85` to output and subtracts the identical `Kz67` from input. It still must be reported on both lines.

**Zahllast (Kz 83)** = total output VAT (`USt81 + USt86 + Kz85`) minus total input VAT (`Kz66 + Kz67`). Positive = amount owed; negative = Überschuss (refund). The UI displays "Zahllast: X €" when `Kz83 ≥ 0` and "Überschuss: X €" with the value negated when `Kz83 < 0`.

#### 3.3 Worked example (from `ustva_official_test.go`)

Reverse-charge rate = 19 %. Input invoices:

| # | Direction | VATID | TaxLine (Netto / Satz / MwSt) | Classified as |
|---|---|---|---|---|
| 1 | outgoing | `DE123` | 6500 / 19 / 1235 | Kz81 += 6500 |
| 2 | outgoing | (blank) | 1000 / 0 / 0 | Kz45 += 1000 |
| 3 | outgoing | `FI26378052` | 2000 / 0 / 0 | Kz21 += 2000 |
| 4 | incoming | `IE123` | 462.40 / 0 / 0 | Kz84 += 462.40 |
| 5 | incoming | `DE999` | 164.16 / 19 / 31.19 | Kz66 += 31.19 |

Results:
- `Kz81 = 6500`, `Kz86 = 0`, `Kz21 = 2000`, `Kz45 = 1000`, `Kz84 = 462.40`, `Kz66 = 31.19`
- `Kz85 = round2(462.40 × 0.19) = 87.86`; `Kz67 = 87.86`
- `USt81 = round2(6500 × 0.19) = 1235`; `USt86 = 0`
- `Kz83 = round2((1235 + 0 + 87.86) − (31.19 + 87.86)) = round2(1322.86 − 119.05) = 1203.81`

---

### 4. Account-based UStVA (ledger view)

Computed by `ComputeUStVA(rows, rules)`. This sums actual VAT postings from the double-entry booking, grouped per account. Output `UStVA`:

- `Umsatzsteuer`: list of lines `{Satz, Konto, Betrag}` — output-VAT accounts (credit side)
- `Vorsteuer`: list of lines `{Satz, Konto, Betrag}` — input-VAT accounts (debit side)
- `UmsatzsteuerGesamt`, `VorsteuerGesamt`, `Zahllast` (decimals)

#### 4.1 Account → rate maps

From `rules`:
- For each `(satz, konto)` in `VorsteuerKonten` (a map of string-rate → account, e.g. `"19" → 1576`), build `vstRate[konto] = atoi(satz)`. Non-numeric keys are skipped.
- For each `(satz, konto)` in `UmsatzsteuerKonten`, build `ustRate[konto] = atoi(satz)`.
- If a `"reverse_charge"` rule exists: `rcSatz = round(RcSatz + 0.5)`. If `KontoVStRC ≠ 0`, add `vstRate[KontoVStRC] = rcSatz` (the §13b input-VAT account joins the Vorsteuer side). If `KontoUStRC ≠ 0`, add `ustRate[KontoUStRC] = rcSatz` (the §13b output-VAT account joins the Umsatzsteuer side).

#### 4.2 Summation

For every booking entry of every row:
- If `Soll` (debit) **and** `Konto` is a Vorsteuer account → `vstSum[Konto] += Betrag`.
- If **not** `Soll` (credit) **and** `Konto` is an Umsatzsteuer account → `ustSum[Konto] += Betrag`.

Then build lines:
- For each Umsatzsteuer account: emit `{Satz: ustRate[konto], Konto, Betrag: round2(rawSum)}`; accumulate raw sum into `UmsatzsteuerGesamt`.
- Same for Vorsteuer → `VorsteuerGesamt`.
- `UmsatzsteuerGesamt = round2(rawSum)`, `VorsteuerGesamt = round2(rawSum)`.
- `Zahllast = round2(UmsatzsteuerGesamt − VorsteuerGesamt)`.

> Quirk (totals stability): each *line* `Betrag` is rounded independently, but the section **total** is computed from the *raw* (unrounded) per-account sums and rounded once. This guarantees the printed total never drifts ±0.01 from re-adding the displayed lines for the common case, but for many lines the displayed lines can sum to a value differing from the printed total by a cent. Re-implementers must total the raw values, not the rounded line values.

#### 4.3 Sort order

Both line lists are sorted **ascending by `Satz`, then by `Konto`** within equal rate.

#### 4.4 Worked example (from `ustva_test.go`)

Rules: `VorsteuerKonten = {"19":1576, "7":1571}`, `UmsatzsteuerKonten = {"19":1776}`, reverse_charge `{rc_satz:19, konto_vst_rc:1577, konto_ust_rc:1787}`.

Bookings (entries shown as `Konto/Betrag/side`):
1. Expense 19 %: `4240/100/S, 1576/19/S, 1200/119/H` → VSt 1576 += 19
2. Expense 7 %: `4140/50/S, 1571/3.50/S, 1200/53.50/H` → VSt 1571 += 3.50
3. Revenue 19 %: `1200/7735/S, 8400/6500/H, 1776/1235/H` → USt 1776 += 1235
4. §13b 19 %: `27/462.40/S, 1577/87.86/S, 1787/87.86/H, 1200/462.40/H` → VSt 1577 += 87.86, USt 1787 += 87.86

Results:
- `UmsatzsteuerGesamt = 1235 + 87.86 = 1322.86`
- `VorsteuerGesamt = 19 + 3.50 + 87.86 = 110.36`
- `Zahllast = 1322.86 − 110.36 = 1212.50`
- Umsatzsteuer lines (sorted): `{19, 1776, 1235}`, `{19, 1787, 87.86}`
- Vorsteuer lines (sorted by Satz then Konto): `{7, 1571, 3.50}`, `{19, 1576, 19}`, `{19, 1577, 87.86}`

> Note: the account-based example's Zahllast (1212.50) differs from the official-UStVA Zahllast on comparable data because the two methods sum from different inputs (postings vs. invoice tax-lines) and classify differently. They are independent.

---

### 5. ZM — Zusammenfassende Meldung (EC Sales List)

Computed by `ComputeZM(rows)`. Reports intra-EU reverse-charge supplies, aggregated per customer VAT-ID. Output `ZM`:
- `Zeilen`: list of `{UStIdNr (string), Netto (decimal)}`
- `Kontrollsumme`: decimal control total

#### 5.1 EU VAT-ID test (`IsEUVatID`)

Used by both UStVA-Official and ZM. A string is an EU-other VAT-ID iff, after `trim` + uppercase, it is ≥ 3 chars and its first two characters are in this set of EU member-state prefixes (note **`EL`** for Greece, and **`DE` is intentionally excluded** — a domestic customer is never a ZM counterparty):

```
AT BE BG CY CZ DK EE EL ES FI FR HR HU IE IT
LT LU LV MT NL PL PT RO SE SI SK
```

Test vectors (`zm_test.go`): `FI26378052`→true, `ATU12345678`→true, `FR12345678901`→true, `DE287472874`→false, `CHE123456789`→false (CH not EU), `""`→false, `12345`→false.

> Quirk: the test only checks the 2-letter prefix and length ≥ 3 — it does **not** validate the body. `EL` would match Greece even though Greek IDs also appear as `GR` colloquially (`GR` is **not** in the set). The format-validity regex (`^[A-Z]{2}[0-9A-Za-z]{6,14}$`) is a *separate* advisory warning (§7), not part of `IsEUVatID`.

#### 5.2 Aggregation algorithm

After `RowsEUR(rows)` (EU sales must be reported in EUR):

```
byVat = {}
for each row r:
    if NOT r.Ausgangsrechnung: skip
    if NOT IsEUVatID(r.VATID): skip
    if SumMwSt(r.TaxLines) != 0: skip      # any VAT at all → not a reverse-charge supply
    key = uppercase(trim(r.VATID))
    byVat[key] += SumNetto(r.TaxLines)

for each (vat, netto) in byVat:
    netto = round2(netto)
    add Zeile{UStIdNr: vat, Netto: netto}
    Kontrollsumme += netto

sort Zeilen ascending by UStIdNr (string compare)
Kontrollsumme = round2(Kontrollsumme)
```

Inclusion criteria (ALL must hold): outgoing invoice **and** EU-other VAT-ID **and** VAT exactly 0. The VAT test is **`!= 0`** (exact), not a threshold — differing from the UStVA-Official's `< 0.005`. The VAT-ID key is normalized (trim + uppercase) so the same customer in mixed case aggregates into one line.

#### 5.3 Worked example (from `zm_test.go`)

| # | Direction | VATID | Netto / Satz / MwSt | Effect |
|---|---|---|---|---|
| 1 | outgoing | `FI26378052` | 6500 / 0 / 0 | included |
| 2 | outgoing | `FI26378052` | 1000 / 0 / 0 | accumulates → 7500 |
| 3 | outgoing | `DE123` | 100 / 19 / 19 | excluded (domestic + has VAT) |
| 4 | outgoing | (blank) | 500 / 0 / 0 | excluded (no EU VAT-ID) |
| 5 | incoming | `IE123` | 200 / 0 / 0 | excluded (not outgoing) |

Result: one line `FI26378052 → 7500`; `Kontrollsumme = 7500`.

---

### 6. XML export format

Both XML documents are produced by marshaling with **2-space indentation** and are prefixed with the standard XML header. The XML header used is `<?xml version="1.0" encoding="UTF-8"?>\n`. These are **not ELSTER ERiC transmissions** — they are clean structured exports for the tax advisor. Numbers are rendered as `round2`'d floats (the marshaler prints them with minimal decimals: `6500` not `6500.00`, `1197.21` as-is).

#### 6.1 UStVA XML — `BuildUStVAXML(u, zeitraum, ownVatID)`

Root element `<UmsatzsteuerVoranmeldung>` with attributes:
- `zeitraum` (always present) — the period string
- `ust_idnr` (omitted entirely when `ownVatID == ""`, via `omitempty`)

Children: a sequence of `<kennzahl>` elements, each with attributes `nr` and `bezeichnung` and a child `<wert>`:
```xml
<kennzahl nr="81" bezeichnung="Steuerpflichtige Umsätze 19 %"><wert>6500</wert></kennzahl>
```

**Emission rule:** a Kennzahl is emitted only if its value `≠ 0`, **except Kz 83 which is always emitted** (even when 0). Emission order is fixed:

| nr | bezeichnung | source | always? |
|---|---|---|---|
| 81 | `Steuerpflichtige Umsätze 19 %` | Kz81 | no |
| 86 | `Steuerpflichtige Umsätze 7 %` | Kz86 | no |
| 21 | `Innergem. sonstige Leistungen (§ 18b UStG)` | Kz21 | no |
| 45 | `Übrige nicht steuerbare Umsätze (Ausland)` | Kz45 | no |
| 84 | `§ 13b Bemessungsgrundlage` | Kz84 | no |
| 85 | `§ 13b Steuer` | Kz85 | no |
| 66 | `Vorsteuer aus Rechnungen` | Kz66 | no |
| 67 | `Vorsteuer aus § 13b-Leistungen` | Kz67 | no |
| 83 | `Verbleibende Vorauszahlung / Überschuss` | Kz83 | **yes** |

Each emitted value is `round2`'d again at emit time. `USt81`/`USt86` are **not** in the XML (they are display-only derivations).

Worked example (from `xmlexport_test.go`), `zeitraum="2025"`, `ownVatID="287472874"`, input includes `Kz81=6500, Kz45=1077.60, Kz84=462.40, Kz85=87.86, Kz67=87.86, Kz66=37.79, Kz83=1197.21` (Kz86=0):
- Root has `zeitraum="2025"` and `ust_idnr="287472874"`.
- Contains `<kennzahl nr="81" ...><wert>6500</wert>` and `<kennzahl nr="83" ...><wert>1197.21</wert>`.
- **Does NOT contain `nr="86"`** (zero → omitted).

#### 6.2 ZM XML — `BuildZMXML(z, zeitraum, ownVatID)`

Root element `<ZusammenfassendeMeldung>` with attributes `zeitraum` (always) and `ust_idnr` (omitted when empty). Direct children:
- `<kontrollsumme>` — the control total (decimal), emitted **before** the Meldezeilen
- one `<meldezeile>` per ZM line, in the order they appear in `z.Zeilen` (already sorted ascending by VAT-ID)

Each `<meldezeile>` contains, in order:
- `<ust_idnr>` — the customer EU VAT-ID
- `<summe>` — the net total for that customer
- `<art_der_leistung>` — **hardcoded literal `"Sonstige Leistung"`** (every line; the type of supply is always "other service")

```xml
<ZusammenfassendeMeldung zeitraum="2025-Q2" ust_idnr="287472874">
  <kontrollsumme>44795</kontrollsumme>
  <meldezeile>
    <ust_idnr>FI26378052</ust_idnr>
    <summe>44795</summe>
    <art_der_leistung>Sonstige Leistung</art_der_leistung>
  </meldezeile>
</ZusammenfassendeMeldung>
```
(Worked values from `xmlexport_test.go`: one line `FI26378052 / 44795`, `kontrollsumme 44795`, period `"2025-Q2"`.)

---

### 7. Period selection & the missing-VAT-ID warning (UI behavior)

#### 7.1 Period selection

Both dialogs offer a 3-way radio toggle: **Monat / Quartal / Jahr** (month / quarter / year). The default differs:
- **UStVA** defaults to **Monat** (period 0). UStVA is filed monthly or quarterly.
- **ZM** defaults to **Quartal** (period 1) — the official ZM filing period.

The period is resolved against the app's `currentYear` / `currentMonth`:
- **Month (0):** `fromM = toM = currentMonth`.
- **Quarter (1):** `q = (currentMonth − 1) / 3` (integer div, 0-based); `fromM = q×3 + 1`, `toM = q×3 + 3`. The calendar quarter *containing* the current month.
- **Year (2):** `fromM = 1, toM = 12`.

Rows are gathered by `collectInvoiceRows(fromY, fromM, toY, toM)`, which loads each month's CSV in `[from, to]` inclusive and concatenates. A month whose CSV fails to load is logged as a warning and **skipped** (its rows are simply absent from the filing) — not a hard error.

The exported file's period string (used in filenames and in the XML `zeitraum` attribute):
- Month: `"YYYY-MM"` (e.g. `2025-03`)
- Quarter: `"YYYY-QN"` with `N = (currentMonth − 1)/3 + 1` (e.g. `2025-Q1`)
- Year: `"YYYY"` (e.g. `2025`)

Export filenames: `UStVA_<period>.pdf` / `.xml`, `ZM_<period>.pdf` / `.xml`. The XML `ownVatID` comes from settings `OwnVATID`.

#### 7.2 UStVA dialog display grouping

The official UStVA values are grouped into labeled sections (lines with value 0 are hidden):
- **A. Umsätze** — Kz 81 (with derived USt81 shown as "→ Umsatzsteuer"), Kz 86 (with USt86)
- **E. Nicht steuerbare Umsätze** — Kz 21, Kz 45
- **D. § 13b (Reverse Charge)** — Kz 84, Kz 85
- **F. Vorsteuer** — Kz 66, Kz 67
- A bold trailing line `Kz 83 — Zahllast: X €` (or `Überschuss: X €` with negated value when Kz83 < 0), always shown.

#### 7.3 ZM dialog display

One line per `Zeile`: `<UStIdNr>  <Netto formatted>  Sonstige Leistung`, then a bold `Kontrollsumme: X €`. When there are no lines, it shows the empty-period message ("Keine EU-Umsätze im Zeitraum" / "No intra-EU sales in this period"). If settings `OwnVATID` is non-empty, the header shows `USt-IdNr: <OwnVATID>`.

#### 7.4 The missing-VAT-ID warning

There is **no warning inside the ZM/UStVA computation or dialog** for a missing VAT-ID — rows without an EU VAT-ID are silently excluded. The relevant safeguard is an **advisory, non-blocking plausibility warning raised at invoice-entry time** (in `InvoiceWarnings`):

> Fires when: `Ausgangsrechnung == true` **and** `SteuersatzBetrag == 0` **and** `trim(VATID) == ""`.
> Message (DE): *"Ausgangsrechnung ohne USt und ohne Kunden-USt-IdNr — bei EU-Kunden fehlt sonst der ZM-Eintrag (bei Drittland/Schweiz ok)"* — an outgoing 0 %-VAT invoice without a customer VAT-ID would be missing from the ZM if the customer is an EU customer (harmless for genuine third-country/Switzerland supplies).

This is purely advisory and does not block saving. It is the mechanism by which a user is nudged to add the VAT-ID that the ZM aggregation requires. A separate advisory checks VAT-ID **format** against `^[A-Z]{2}[0-9A-Za-z]{6,14}$` (after removing spaces and uppercasing) and warns "USt-IdNr hat ungültiges Format" on mismatch.

---

### Re-implementation checklist

Must-match behaviors for this subsystem:

1. **Two UStVA computations exist** and consume different inputs: account-based (sums booking entries posted to configured Vorsteuer/Umsatzsteuer accounts, no currency conversion) and official-Kennzahlen (classifies invoices from tax-lines + VAT-ID, after EUR conversion). Implement both.
2. **Official Kennzahlen mapping (exact):** outgoing + VAT>0.005 → per-line Kz81 (rate→19) / Kz86 (rate→7); outgoing + 0% + EU VAT-ID → Kz21; outgoing + 0% + non-EU → Kz45; incoming + EU VAT-ID + VAT<0.005 → Kz84; otherwise incoming → Kz66 (the VAT amount). Integer rate via `round(satz + 0.5)`.
3. **Derived values:** `Kz85 = round2(Kz84 × rcSatz/100)`; `Kz67 = Kz85` exactly; `USt81 = round2(Kz81 × 0.19)`; `USt86 = round2(Kz86 × 0.07)`; `Kz83 = round2((USt81+USt86+Kz85) − (Kz66+Kz67))`. `rcSatz` defaults to 19, overridable by the `reverse_charge` rule's `RcSatz`.
4. **Rounding:** `round2` = round-half-away-from-zero to 2 decimals, applied per line and again at emit. Account-based section totals are summed from **raw** unrounded values and rounded once (totals never re-add the rounded lines).
5. **EU VAT-ID test:** 2-letter prefix in the exact 26-state set (includes `EL`, excludes `DE` and `GR`), length ≥ 3, no body validation. Trim + uppercase first.
6. **ZM aggregation:** outgoing AND EU VAT-ID AND `SumMwSt == 0` (exact), in EUR; aggregate net per uppercased/trimmed VAT-ID; lines sorted ascending by VAT-ID; control total = round2 of the summed rounded line values.
7. **Currency normalization** (Official UStVA + ZM only): foreign with rate → divide each money field/tax-line by `Wechselkurs` and round2; foreign without rate → pass through at face value; EUR/blank → unchanged. Account-based UStVA does NOT convert.
8. **UStVA XML:** root `<UmsatzsteuerVoranmeldung>` with `zeitraum` (always) + `ust_idnr` (omit if empty); `<kennzahl nr="" bezeichnung=""><wert>` in the fixed order 81,86,21,45,84,85,66,67,83; emit only non-zero values, **except Kz 83 always emitted**. Exact `bezeichnung` strings as tabulated. 2-space indent + XML header.
9. **ZM XML:** root `<ZusammenfassendeMeldung>` with `zeitraum` + optional `ust_idnr`; `<kontrollsumme>` first, then one `<meldezeile>` per line with `<ust_idnr>/<summe>/<art_der_leistung>` where `art_der_leistung` is always the literal `"Sonstige Leistung"`.
10. **Period selection:** month/quarter/year toggle; UStVA default = month, ZM default = quarter; quarter = calendar quarter containing the current month; period strings `YYYY-MM`, `YYYY-QN`, `YYYY`; months with unreadable CSVs are skipped, not errored.
11. **Missing-VAT-ID handling:** rows without an EU VAT-ID are silently excluded from ZM; the only warning is the advisory invoice-time check (outgoing + 0% VAT + empty VAT-ID) with the exact wording above — non-blocking.

Source files: `internal/core/ustva.go`, `ustva_official.go`, `zm.go`, `xmlexport.go`, `eur.go`, `taxline.go`, `buchungsregeln.go`, `warnings.go`; UI wiring `internal/ui/ustvaview.go`, `zmview.go`, `csvexport.go`; tests `ustva_test.go`, `ustva_official_test.go`, `zm_test.go`, `xmlexport_test.go`.

---

## Reports: SuSa, GuV, OPOS, Controlling, Overviews

This chapter specifies BuchISY's read-only reporting subsystem: the on-screen aggregations and the PDF documents derived from invoice rows, double-entry bookings and cash books. All amounts are decimals rounded to 2 places. Every report is recomputed from scratch on demand; nothing is persisted.

### Shared concepts and conventions

**Input rows.** All reports below operate on `CSVRow` records (one per invoice/receipt). The on-screen views collect rows via a period filter `collectInvoiceRows(fromYear, fromMonth, toYear, toMonth)`: it loads each month's stored CSV in the inclusive `[from..to]` range and concatenates them in chronological month order. A month whose CSV fails to load is skipped (logged as a warning) and contributes nothing. The default period for SuSa, GuV and OPOS is the **whole current year** (months 1–12). Controlling can be either the current month or the current year (user toggle).

**EUR normalization.** Most report computations begin by mapping rows through `RowsEUR` (see below). For each row:
- A row is "foreign" iff `Waehrung` is non-empty and not `"EUR"`.
- EUR rows (or rows with empty currency) pass through unchanged.
- A foreign row with `Wechselkurs <= 0` passes through at **face value** (rate missing → no conversion).
- A foreign row with `Wechselkurs > 0` has these money fields divided by the rate and re-rounded to 2 places: `BetragNetto`, `SteuersatzBetrag`, `Bruttobetrag`, `Trinkgeld`, `Rabatt`, each `TaxLine.Netto` and `TaxLine.MwStBetrag`; `BetragNetto_EUR` is set equal to the converted net; `Waehrung` → `"EUR"`, `Wechselkurs` → `0`. The bank/FX fee field `Gebuehr` is **not** divided (already EUR).

> Quirk: `RowsEUR` does **not** convert the embedded double-entry `Buchung.Entries`. Booking entry amounts are assumed to already be in EUR (they are produced in EUR at booking time). Therefore SuSa, GuV and Controlling — which iterate over `Buchung.Entries` — are effectively unaffected by the `RowsEUR` call for the booking-derived numbers; the `RowsEUR` step there is a harmless no-op for the booking side. OPOS, the Overview KPIs and the PDF lists, by contrast, read EUR-converted invoice fields (`Bruttobetrag`, `BetragNetto`, `SteuersatzBetrag`) and therefore DO reflect EUR conversion.

**round2.** Round-half-away-from-zero to 2 decimals: `round2(v) = round(v * 100) / 100` (standard arithmetic rounding; `2.675 → 2.68`, `-2.675 → -2.68`).

**Booking helpers** used by reports (a `Booking` is a list of entries, each `{Konto:int, Betrag:decimal, Soll:bool}`):
- `DebitEntries()` = all entries with `Soll == true`.
- `PaymentEntry()` = the single credit (`Soll == false`) entry; returns "not ok" unless there is **exactly one** credit entry.
- `Balanced()` = there is at least one entry AND `|Σ Soll − Σ Haben| < 0.005`.

**Account names** come from a chart of accounts (SKR04). `chart.Find(konto)` returns the account `{Number, Name, Type}` or "not found". `Type` is one of `"revenue"`, `"expense"`, `"asset"`, etc. When `chart` is nil or the account is unknown, the name falls back per report (see each).

**PDF framework** (`newReportPDF`): A4, fpdf, mm units, cp1252 (Windows-1252) encoding via a Unicode→cp1252 translator so `ä ö ü ß €` render with the core Arial font. Every report has:
- Title in bold Arial 14 (left), then a sub-header in Arial 9: `"Erstellt am DD.MM.YYYY"` (today's date), prefixed with `"<company>  ·  "` when a non-blank company/profile name is supplied (`company  ·  Erstellt am ...`).
- A centered footer on every page in italic Arial 8: `"Seite X / N"` (current page / total pages).
- Tables: bold Arial 9 header row drawn with cell borders (`"1"`), 7 mm header height; data rows Arial 9, 6 mm height, bordered cells. Money is right-aligned ("R"), text left-aligned ("L"). A new page is started (and the header re-drawn) whenever the next row would overflow the bottom margin (`pageBreak` check before each row and before each totals row).
- Money formatting in PDFs (`pdfAmount`): `"%.2f"` with the decimal point replaced by a comma, **no thousands separator**, e.g. `1234.5 → "1234,50"`, `-200 → "-200,00"`. (The on-screen tables use the user's configured decimal separator via `formatMoney`.)
- Text truncation (`truncate(s, n)`) is **rune-safe** (counts runes, not bytes) and silently cuts to at most `n` runes; per-column limits are given below.

---

### SuSa — Summen- und Saldenliste (trial balance)

**Function:** `ComputeSuSa(rows, chart) → []AccountBalance`, where each `AccountBalance = {Konto:int, Name:string, SollSumme:decimal, HabenSumme:decimal, Saldo:decimal}`.

**Algorithm:**
1. `rows = RowsEUR(rows)`.
2. For every row, for every booking entry `e`: accumulate per account number `e.Konto`:
   - if `e.Soll`: add `e.Betrag` to that account's running `soll`.
   - else: add `e.Betrag` to its running `haben`.
3. For each account produce a row:
   - `SollSumme = round2(soll)`
   - `HabenSumme = round2(haben)`
   - `Saldo = round2(soll − haben)` (positive = debit excess, negative = credit excess)
   - `Name`: from `chart.Find(konto).Name`; if chart is nil or not found, `Name` = the account number rendered as a string (e.g. `"4663"`).
4. Sort ascending by `Konto`.

> Note: SuSa is built purely from the embedded bookings, not from invoice gross/net fields. Invoices with no booking entries contribute nothing.

**Worked example** (from `TestComputeSuSa`): two bookings —
- Expense: Soll 4663 = 100, Haben 1200 = 100.
- Revenue: Soll 1200 = 200, Haben 8400 = 200.

Result, sorted `[1200, 4663, 8400]`:

| Konto | Soll | Haben | Saldo |
|-------|------|-------|-------|
| 1200  | 200.00 | 100.00 | 100.00 |
| 4663  | 100.00 | 0.00   | 100.00 |
| 8400  | 0.00   | 200.00 | -200.00 |

**SuSa PDF** (`BuildSuSaPDF`, **portrait**). Columns in order, with mm widths and truncation:

| # | Header | Width mm | Align | Source / truncation |
|---|--------|---------|-------|---------------------|
| 1 | Konto | 18 | L | account number |
| 2 | Bezeichnung | 97 | L | `Name`, truncated to 57 runes |
| 3 | Soll | 25 | R | `SollSumme` |
| 4 | Haben | 25 | R | `HabenSumme` |
| 5 | Saldo | 25 | R | `Saldo` |

Totals row (bold): label `"Summe"` right-aligned spanning the first two columns (18+97), then `round2(Σ SollSumme)`, `round2(Σ HabenSumme)`, `round2(Σ Saldo)` in the three amount columns. PDF title (on-screen caller): `"<susa.title> <year>"`.

---

### GuV — Gewinn- und Verlustrechnung (simplified P&L)

**Function:** `ComputeGuV(susa, chart) → GuV`. Input is the **already-computed SuSa list**, partitioned by chart `Type`. `GuV = {ErloesPosten:[]AccountBalance, AufwandPosten:[]AccountBalance, ErloeseGesamt, AufwandGesamt, Ergebnis}`.

**Algorithm:** for each SuSa account balance `b`:
- If `chart` is nil, skip everything (GuV is empty).
- `a = chart.Find(b.Konto)`; if not found, skip.
- If `a.Type == "revenue"`: append `b` to `ErloesPosten`; revenue contribution = `round2(b.HabenSumme − b.SollSumme)`; add to `ErloeseGesamt`.
- If `a.Type == "expense"`: append `b` to `AufwandPosten`; expense contribution = `round2(b.SollSumme − b.HabenSumme)`; add to `AufwandGesamt`.
- Any other type (asset, etc.) is ignored.

Finally: `ErloeseGesamt = round2(Σ revenue contributions)`, `AufwandGesamt = round2(Σ expense contributions)`, `Ergebnis = round2(ErloeseGesamt − AufwandGesamt)`.

Posten preserve SuSa order (ascending by Konto), since the input is the sorted SuSa list.

**Worked example** (from `TestComputeGuV`, chart: 4663=expense "Reisekosten", 8400=revenue "Erlöse 19%", 1200=asset "Bank"; same two bookings as SuSa above):
- Revenue: account 8400 → `HabenSumme − SollSumme = 200 − 0 = 200` → `ErloeseGesamt = 200.00`.
- Expense: account 4663 → `SollSumme − HabenSumme = 100 − 0 = 100` → `AufwandGesamt = 100.00`.
- Account 1200 (asset) appears in neither section.
- `Ergebnis = 200 − 100 = 100.00`.

**GuV PDF** (`BuildGuVPDF`, **portrait**). Two sections, then a bold result line. Each section: a bold Arial 10 heading line, then a 3-column table.

Columns: `Konto` (18 mm, L), `Bezeichnung` (107 mm, L, `Name` truncated to 63 runes), `Betrag` (35 mm, R).

- Section **"Erlöse"**: rows show `round2(HabenSumme − SollSumme)` per account; section total row `"Summe"` (bold, spanning 18+107) + `round2(ErloeseGesamt)`.
- Section **"Aufwand"**: rows show `round2(SollSumme − HabenSumme)` per account; section total `"Summe"` + `round2(AufwandGesamt)`.
- Final bold line (8 mm): `"Ergebnis"` (spanning 18+107) + `Ergebnis`.

Between sections there is a 3 mm vertical gap. PDF title (caller): `"<guv.title> <year>"`.

---

### OPOS — Offene-Posten-Liste (open items / aging)

**Function:** `ComputeOpenItems(rows, asOf:date) → OpenItems`. `OpenItems = {Forderungen:[]OpenItem, Verbindlichkeiten:[]OpenItem, ForderungenGesamt, VerbindlichkeitenGesamt}`. `OpenItem = {Belegnummer, Rechnungsnummer, Datum, Partner, Betrag, AgeDays:int, Bucket:string}`. The on-screen caller passes `asOf = time.Now()` (current wall-clock date).

**What makes an item "open":** a row is open iff **both** `Bezahldatum` is blank (after trimming whitespace) **and** `BuchungRef` is blank (after trimming). If either is non-blank, the row is excluded entirely.

**Algorithm** (after `rows = RowsEUR(rows)`), for each open row:
1. **AgeDays:** parse `Rechnungsdatum` as `DD.MM.YYYY`. If it parses AND `asOf − Rechnungsdatum >= 0`, then `AgeDays = floor((asOf − date) hours / 24)` (integer days). If the date is empty, unparseable, or in the future (negative diff), `AgeDays = 0`.
2. **Bucket** from `AgeDays` (boundaries inclusive on the upper end):
   - `days <= 30` → `"0–30"`
   - `days <= 60` → `"31–60"`
   - `days <= 90` → `"61–90"`
   - else (`> 90`) → `">90"`
   (The dash is an en-dash `–`, U+2013, not a hyphen.)
3. Build the item: `Belegnummer`, `Rechnungsnummer`, `Datum = Rechnungsdatum` (verbatim string), `Partner = Auftraggeber`, `Betrag = Bruttobetrag` (EUR-converted gross), `AgeDays`, `Bucket`.
4. **Classification:** if `Ausgangsrechnung == true` → **Forderung** (receivable / Debitor); else → **Verbindlichkeit** (payable / Kreditor). Add `Betrag` to the corresponding running total (`ForderungenGesamt` / `VerbindlichkeitenGesamt`).

> Quirk: the section totals (`ForderungenGesamt`, `VerbindlichkeitenGesamt`) are accumulated by simple addition and are **not** re-rounded in `ComputeOpenItems`; they are only `round2`-ed at PDF render time. Items appear in input order (no sorting by age or partner).

**Worked example** (from `TestComputeOpenItems`, `asOf = 24.06.2026`):
- Row 1: Ausgangsrechnung, Rechnungsdatum 40 days before asOf, Brutto 1000.00, unpaid, unbooked → Forderung, `AgeDays = 40`, `Bucket = "31–60"`, Partner "Kunde GmbH", Betrag 1000.00.
- Row 2: incoming, 10 days before asOf, Brutto 250.00, unpaid, unbooked → Verbindlichkeit, `AgeDays = 10`, `Bucket = "0–30"`, Betrag 250.00.
- Row 3: incoming, Bezahldatum "10.06.2026" set → **excluded** (paid).

Result: `Forderungen` = [1 item], `Verbindlichkeiten` = [1 item], `ForderungenGesamt = 1000.00`, `VerbindlichkeitenGesamt = 250.00`.

**EUR example** (from `TestComputeOpenItemsEURNormalization`): a USD invoice, `Bruttobetrag = 200.00`, `Waehrung = "USD"`, `Wechselkurs = 1.17`, incoming, open → `Betrag = round2(200.00 / 1.17) = 170.94` EUR (NOT the 200 face value). Confirms OPOS reads EUR-converted gross.

**OPOS PDF** (`BuildOpenItemsPDF`, **landscape**). Two sections — `"Debitoren (Forderungen)"` then `"Kreditoren (Verbindlichkeiten)"` — each a bold Arial 10 heading + table.

Columns in order:

| # | Header | Width mm | Align | Source / truncation |
|---|--------|---------|-------|---------------------|
| 1 | Belegnr. | 30 | L | `Belegnummer`, truncated 18 |
| 2 | Datum | 22 | L | `Datum` (raw string) |
| 3 | Partner | 90 | L | `Partner`, truncated 52 |
| 4 | Betrag | 28 | R | `Betrag` |
| 5 | Alter (Tage) | 28 | R | `AgeDays` (integer) |
| 6 | Bucket | 20 | L | `Bucket` |

Section total (bold): label `"Summe"` right-aligned spanning columns 1+2+3+5+6 (30+22+90+28+20 = 190 mm — note the Betrag column is excluded from the span), then `round2(total)` in the Betrag column. 3 mm gap between sections. PDF title (caller): `"Offene Posten <year>"`.

---

### Controlling — income/expense split per account

There are two related aggregations.

#### AggregateControlling — P&L cash-flow split

**Function:** `AggregateControlling(rows, rules, paymentKonten, chart) → Controlling`. `Controlling = {Einnahmen:[]AccountSum, Ausgaben:[]AccountSum, EinnahmenGesamt, AusgabenGesamt, Saldo}`. `AccountSum = {Konto:int, Name:string, Summe:decimal}`.

**Exclusion set** (these accounts are filtered OUT of both income and expense — they are pure VAT/payment plumbing, not P&L):
- every account in `paymentKonten` (the SKR04 payment accounts of the configured bank accounts, e.g. 1200/1000);
- every account in `rules.VorsteuerKonten` (input-VAT accounts);
- every account in `rules.UmsatzsteuerKonten` (output-VAT accounts);
- if a `"reverse_charge"` rule exists: its `KontoVStRC` and `KontoUStRC` (when non-zero) — the §13b VAT accounts.

**Algorithm** (after `rows = RowsEUR(rows)`):
- For each row, for each booking entry `e`:
  - If `e.Konto` is in the exclusion set → skip.
  - Else if `e.Soll` → add `e.Betrag` to expense map for `e.Konto` (`ausg`).
  - Else (credit) → add to income map for `e.Konto` (`einn`).
- Convert each map to a sorted `[]AccountSum` (ascending by Konto), with `Summe = round2(amount)` and `Name` from chart (empty string if chart nil/unknown), and total `= round2(Σ amounts)`.
- `EinnahmenGesamt`, `AusgabenGesamt` are those rounded totals; `Saldo = round2(EinnahmenGesamt − AusgabenGesamt)`.

**Worked example** (from `TestAggregateControlling`; rules: Vorsteuer 19→1576, Umsatzsteuer 19→1776, reverse_charge VStRC 1577 / UStRC 1787; payment accounts {1000, 1200}):
- Expense booking: Soll 4240=100, Soll 1576=19 (VAT, excluded), Haben 1200=119 (payment, excluded) → expense 4240 += 100.
- Revenue booking: Soll 1200=119 (excluded), Haben 8400=100, Haben 1776=19 (VAT, excluded) → income 8400 += 100.
- §13b booking: Soll 27=50, Soll 1577=9.5 (excluded), Haben 1787=9.5 (excluded), Haben 1200=50 (excluded) → expense 27 += 50.

Result: `Einnahmen` = [{8400, 100.00}], `EinnahmenGesamt = 100.00`; `Ausgaben` = [{27, 50.00}, {4240, 100.00}] (sorted ascending), `AusgabenGesamt = 150.00`; `Saldo = 100 − 150 = -50.00`.

#### AggregateBookingsByAccount — debit-only per-account totals

**Function:** `AggregateBookingsByAccount(rows, chart) → ([]AccountSum, total)`. Sums only the **Soll (debit)** entries across all bookings, grouped by account; no EUR normalization is applied (caller passes already-EUR rows or raw). Each `Summe = round2(account total)`; sorted ascending by Konto; `Name` from chart (empty if nil/unknown); total `= round2(Σ Summe)`. Bookings without entries contribute nothing.

**Worked example** (from `TestAggregateBookingsByAccount`): two bookings with Soll 6640=12.71 + 1406=1.26 (Haben 1800=13.97 ignored), and Soll 6640=7.29 (Haben 1800=7.29 ignored), plus one empty booking → result sorted `[1406, 6640]`: `{1406, 1.26, "Abziehbare Vorsteuer 19%"}`, `{6640, 20.00}` (12.71+7.29); `total = 21.26`.

**Controlling PDF** (`BuildControllingPDF`, **portrait**). Two sections `"Einnahmen"` then `"Ausgaben"` (bold Arial 10 heading each), then a bold Saldo line.

Columns: `Konto` (25 mm, L, number only), `Bezeichnung` (120 mm, L, `Name` truncated 70), `Summe` (35 mm, R).

Each section: per-account rows, then bold `"Summe"` (spanning Konto+Bezeichnung = 145 mm) + `round2(section total)`; 3 mm gap. Final bold line (8 mm): `"Saldo"` (spanning 145 mm) + `Saldo`. PDF title (caller): the localized `controlling.title` (no year/period appended).

The on-screen Controlling view toggles input between the current **month** (`from=to=currentMonth`) and the current **year** (months 1–12) and recomputes on toggle.

---

### Year overview (cash) — ComputeYearOverview

**Function:** `ComputeYearOverview(carriedIn:decimal, months:[]MonthInput) → []MonthSummary`. Rolls a single cash account's balance through 12 calendar months. `MonthInput = {HasStoredBook:bool, Book:CashBook, Invoices:[]CSVRow}`. `MonthSummary = {Month:time.Month, Anfangsbestand, Einnahmen, Ausgaben, Endbestand}`. The input slice is in calendar order: index 0 = January, … index 11 = December; the result carries `Month = i+1`.

**Algorithm:** maintain a running balance `running`, initialized to `carriedIn`, and a flag `anchored = false`. For each month `i`:
1. `anfang = running`.
2. If `HasStoredBook`:
   - take `book = Book`;
   - if not yet `anchored`: `anfang = book.Anfangsbestand` (the **first** stored book's opening sets the anchor), then `anchored = true`;
   - set `book.Anfangsbestand = anfang` (so later months carry forward but keep the book's own deposits/Einlagen).
3. Else: `book = CashBook{Anfangsbestand: anfang}` (empty book opening at the carried balance).
4. Run `ComputeCashReport(book, Invoices)` → `(entries, endBalance)`. `Einnahmen = Σ entries.Einnahme`, `Ausgaben = Σ entries.Ausgabe`.
5. `summary[i] = {Month: i+1, Anfangsbestand: anfang, Einnahmen, Ausgaben, Endbestand: endBalance}`.
6. `running = endBalance`.

**ComputeCashReport** (supporting): combines the book's `Anfangsbestand`, its `Einlagen` (each a cash inflow → `Einnahme = Betrag`), and the cash-paid `Invoices` (each → `Ausgabe = Bruttobetrag`). Items are ordered: datable entries first, sorted ascending by date (invoices order by `Bezahldatum`, falling back to `Rechnungsdatum`; deposits by `Datum`; all `DD.MM.YYYY`); undatable entries last (stable order). Running `Saldo` starts at `Anfangsbestand` and accumulates `+Einnahme − Ausgabe` per entry. Returns the entries (each carrying its running `Saldo`) and the closing balance.

> Quirk (carry semantics): only the **first** stored book in the year anchors an explicit opening balance. Every subsequent month opens with the previous month's closing balance; a later stored book's own `Anfangsbestand` is **ignored** (but its `Einlagen` still apply). This prevents a stray cash book accidentally saved with a 0 opening from resetting the running balance to zero mid-year. If no month has a stored book, `carriedIn` flows through all 12 months untouched (apart from invoices).

**Worked examples:**
- (`TestComputeYearOverview`) Jan: stored book opening 100, one deposit 50; Feb: no book, one cash invoice 30; Mar–Dec empty; `carriedIn = 999` (ignored because Jan has a stored book). → Jan `{Anfangsbestand 100, Einnahmen 50, Ausgaben 0, Endbestand 150}`; Feb `{Anfangsbestand 150, Ausgaben 30, Endbestand 120}`; Mar `{Anfangsbestand 120, Endbestand 120}` (empty month carries forward).
- (`TestComputeYearOverviewStrayLaterBook`) Jan anchors at 3497.35; May spends 16.68 → May Endbestand 3480.67; June has a **stray** stored book with opening 0 plus a 15.16 cash invoice → June opens at 3480.67 (the stray 0 is ignored), Endbestand 3465.51; July opens at 3465.51.
- (`TestComputeYearOverviewCarriedIn`) all months empty, `carriedIn = 3497.35` → every month opens and closes at 3497.35; Dec Endbestand 3497.35.

**Cash year-overview PDF** (`BuildCashYearOverviewPDF`, **portrait**). Title: `"Kassen-Jahresübersicht <year>  ·  <account>"`. Columns:

| # | Header | Width mm | Align | Source |
|---|--------|---------|-------|--------|
| 1 | Monat | 50 | L | German month name + year, e.g. `"Januar 2026"` |
| 2 | Anfangsbestand | 35 | R | `Anfangsbestand` |
| 3 | Einnahmen | 30 | R | `Einnahmen` |
| 4 | Ausgaben | 30 | R | `Ausgaben` |
| 5 | Endbestand | 35 | R | `Endbestand` |

German month names: `Januar, Februar, März, April, Mai, Juni, Juli, August, September, Oktober, November, Dezember`. There is **no** totals row. Twelve data rows always (one per month).

---

### Overview KPIs — per-month dashboard

**Function:** `OverviewKPIs(rows) → OverviewKPI`. Computes aggregate KPIs over a set of invoice rows (the year-overview dialog calls it once per month, then sums selected fields for a totals row). `OverviewKPI = {Count:int, Netto, USt, Brutto, Zahllast, OpenReconcile:int, Warnings:int}`.

> Quirk: `OverviewKPIs` does **not** apply `RowsEUR`. It sums the raw stored `BetragNetto`, `SteuersatzBetrag`, `Bruttobetrag` fields — for foreign-currency rows these are the face-value (foreign) amounts, NOT EUR.

**Algorithm:**
- `Count = len(rows)`.
- For each row `r`:
  - `Netto += r.BetragNetto`
  - `USt += r.SteuersatzBetrag`
  - `Brutto += r.Bruttobetrag`
  - **Zahllast** (rough VAT-payable indicator): if `r.Ausgangsrechnung` then `Zahllast += r.SteuersatzBetrag` else `Zahllast -= r.SteuersatzBetrag`. (Σ tax on outgoing minus Σ tax on incoming; the comment notes this is an approximation — real Zahllast comes from the UStVA.)
  - **OpenReconcile**: increment if `r.Bankkonto != "" AND r.BuchungRef == ""` (linked to a bank/CC account but not yet matched to a statement line).
  - **Warnings**: increment if `InvoiceWarnings(r)` returns ≥ 1 entry (advisory plausibility warnings — see the warnings list below).

None of these totals are re-rounded; they are raw float sums (displayed with the configured separator).

**Worked examples** (from `overview_test.go`):
- (`TestOverviewKPIs`) Row1 `{Netto 100, USt 19, Brutto 119, Bankkonto set, BuchungRef set, Gegenkonto 4920}`; Row2 `{Netto 50, USt 9.50, Brutto 0 (deliberately wrong), Bankkonto set, BuchungRef empty, Gegenkonto 0}`. → `Count = 2`, `Netto = 150.00`, `USt = 28.50`, `Brutto = 119.00` (row2 contributes 0 gross), `OpenReconcile = 1` (row2), `Warnings ≥ 1` (row2: gross 0 and no Gegenkonto).
- (`TestOverviewKPIsZahllast`) outgoing `{USt 38, Ausgangsrechnung true}` + incoming `{USt 19, Ausgangsrechnung false}` → `Zahllast = 38 − 19 = 19.00`.
- (`TestOverviewKPIsEmpty`) nil input → all zero.

**InvoiceWarnings** (the per-row plausibility checks counted by `Warnings`; advisory only, never block). A warning fires when:
1. `Bruttobetrag > 0` AND `|Bruttobetrag − (BetragNetto + SteuersatzBetrag + Trinkgeld)| > 0.02` → "Brutto stimmt nicht mit Netto + MwSt + Trinkgeld überein".
2. `Gegenkonto == 0` → "Kein Gegenkonto gewählt".
3. foreign currency (`Waehrung` non-empty, ≠ "EUR") AND `Wechselkurs <= 0` → "Fremdwährung ohne Wechselkurs".
4. `Ausgangsrechnung` AND `SteuersatzBetrag == 0` AND blank `VATID` → ZM-missing advisory.
5. `Rechnungsdatum` parses AND is after `today` → "Rechnungsdatum liegt in der Zukunft".
6. `Bruttobetrag <= 0` → "Bruttobetrag fehlt oder ist 0".
7. `Gegenkonto ∈ {4855, 6260}` (GWG immediate-write-off account) AND `BetragNetto > 800.0` → over the GWG limit advisory.
8. incoming, `SteuersatzBetrag == 0`, `Bruttobetrag > 0`, no §13b account (1577/1787/1407/3837) in the booking, AND a foreign-supplier signal (non-EUR currency, or a non-DE EU VAT-ID) → §13b reverse-charge advisory.
9. a deductible Bewirtung account (4650/6640) present in the booking with NO matching non-deductible account (4654/6644) → "Bewirtung ohne 70/30-Aufteilung".
10. `VATID` (trimmed) non-empty AND, after removing spaces and upper-casing, fails the pattern `^[A-Z]{2}[0-9A-Za-z]{6,14}$` → "USt-IdNr hat ungültiges Format".

(The GWG net threshold is exactly **800.0 €**; the Brutto tolerance is **0.02**.)

**Year-overview dialog** (`showYearOverviewDialog`): builds 12 rows by calling `collectInvoiceRows(year, m, year, m)` then `OverviewKPIs` per month `m` (1..12). On-screen columns and the totals row use only a subset of the KPI fields:

| Column | Per-month value | Totals row |
|--------|----------------|-----------|
| Monat | `"MM - <German month>"` | `"<overview.total>"` |
| Anzahl | `Count` | `Σ Count` |
| Brutto | `Brutto` (EUR formatted) | `Σ Brutto` |
| #offen | `OpenReconcile` | `Σ OpenReconcile` |
| #Warnungen | `Warnings` | `Σ Warnings` |

(`Netto`, `USt`, `Zahllast` are computed by `OverviewKPIs` but not displayed in this dialog.) Clicking a month row jumps the app to that month and closes the dialog.

---

### Belegliste (invoice list) PDF

**Function:** `BuildInvoiceListPDF(rows, title, company)`, **landscape**. The on-screen caller (`showBelegListePDF`) uses the **current month** only (`from=to=currentMonth`), title `"Belegliste YYYY-MM"`, output filename `"Belegliste_YYYY-MM.pdf"`.

Before EUR normalization, the function captures each row's original `{Waehrung, Bruttobetrag}` when foreign; then `rows = RowsEUR(rows)`. For a foreign row, the Brutto cell shows the EUR value plus a suffix `" (<CUR> <orig gross>)"`, e.g. `"170,65 (USD 200,00)"`.

Columns in order:

| # | Header | Width mm | Align | Source / truncation |
|---|--------|---------|-------|---------------------|
| 1 | Datum | 22 | L | `Rechnungsdatum` |
| 2 | Auftraggeber | 80 | L | `Auftraggeber`, truncated 46 |
| 3 | Rechnungsnr. | 40 | L | `Rechnungsnummer`, truncated 24 |
| 4 | Netto | 28 | R | `BetragNetto` (EUR) |
| 5 | MwSt | 28 | R | `SteuersatzBetrag` (EUR) |
| 6 | Brutto (EUR) | 49 | R | `Bruttobetrag` (EUR) + optional currency suffix |

Totals row (bold): `"Summe Brutto"` right-aligned spanning columns 1–5 (22+80+40+28+28 = 198 mm) + `round2(Σ Bruttobetrag)` in the Brutto column. **All** rows are listed (no filtering by direction or booking status). Empty input renders a header-only PDF without error.

---

### Rechnungsausgangsbuch (sales journal) PDF

**Function:** `BuildSalesJournalPDF(rows, chart, title, company)`, **landscape**. Caller (`showSalesJournalPDF`) uses the **current month**, title `"Rechnungsausgangsbuch YYYY-MM"`, filename `"Rechnungsausgangsbuch_YYYY-MM.pdf"`.

**Filter:** only rows with `Ausgangsrechnung == true` are emitted (incoming invoices are skipped). Foreign-currency suffix handling on the Brutto column is identical to the Belegliste.

Columns in order:

| # | Header | Width mm | Align | Source / truncation |
|---|--------|---------|-------|---------------------|
| 1 | Belegnr. | 24 | L | `Belegnummer` |
| 2 | Rechnungsnr. | 33 | L | `Rechnungsnummer`, truncated 19 |
| 3 | Datum | 20 | L | `Rechnungsdatum` |
| 4 | Kunde | 58 | L | `Auftraggeber`, truncated 33 |
| 5 | Erlöskonto | 42 | L | `kontoLabel(Gegenkonto)`, truncated 24 |
| 6 | Netto | 26 | R | `BetragNetto` (EUR) |
| 7 | USt | 26 | R | `SteuersatzBetrag` (EUR) |
| 8 | Brutto (EUR) | 43 | R | `Bruttobetrag` (EUR) + optional currency suffix |

`kontoLabel(konto)` = `"<number> <Name>"` when found in chart, else just `"<number>"`. Totals row (bold): `"Summe"` spanning columns 1–5 (24+33+20+58+42 = 177 mm) + `round2(Σ Netto)`, `round2(Σ USt)`, `round2(Σ Brutto)` in the three amount columns.

---

### Buchungsjournal (booking journal) PDF

**Function:** `BuildBookingJournalPDF(rows, chart, title, company)`, **landscape**. Driven from the booking-export flow: title `"Buchungsjournal <period>"` where period is `YYYY-MM` (month), `YYYY` (year), or `YYYY-MM_bis_YYYY-MM` (range). Input is the **exportable** rows (those classified for export — i.e. rows whose booking is eligible; already-exported rows are included only if the user opts in).

Foreign-currency originals are captured before `rows = RowsEUR(rows)`; the suffix `" (<CUR> <orig gross>)"` is appended in the **Auftraggeber** column for foreign rows.

**Row generation:** for each row, get `pay = PaymentEntry()` (the single Haben entry). **Skip** the row unless `Buchung.Balanced()` AND `PaymentEntry` is ok (exactly one credit entry). For each **debit entry** of the booking, emit one line, using the payment entry as the counter-account. (So a booking with N debit entries produces N journal lines, all sharing the same Haben-Konto.)

Auftraggeber column: `truncate(Auftraggeber, 40)` normally; for a foreign row, `truncate(Auftraggeber, 27)` + the currency suffix.

Columns in order:

| # | Header | Width mm | Align | Source / truncation |
|---|--------|---------|-------|---------------------|
| 1 | Datum | 20 | L | `Rechnungsdatum` |
| 2 | Beleg | 35 | L | `Rechnungsnummer`, truncated 22 |
| 3 | Auftraggeber | 70 | L | see above (40, or 27 + suffix) |
| 4 | Soll-Konto | 55 | L | `kontoLabel(debit entry Konto)`, truncated 32 |
| 5 | Haben-Konto | 55 | L | `kontoLabel(payment entry Konto)`, truncated 32 |
| 6 | Betrag | 25 | R | the debit entry's `Betrag` |

Totals row (bold): `"Summe"` spanning columns 1–5 (20+35+70+55+55 = 235 mm) + `round2(Σ all emitted debit amounts)`. A row with no booking, an unbalanced booking, or not exactly one credit entry contributes nothing. Empty/nil input renders without error.

> Quirk: the journal sums and lists **debit (Soll) amounts only** as "Betrag"; the total therefore equals the sum of all Soll entries across all balanced bookings, not the gross invoice totals.

---

### Re-implementation checklist

- **EUR step is per-report.** SuSa, GuV, Controlling, OPOS, and the three list PDFs call `RowsEUR` first; Overview KPIs and `AggregateBookingsByAccount` do **not**. `RowsEUR` converts invoice money fields (divide by `Wechselkurs`, re-round) but never touches `Buchung.Entries`. Foreign rows with `Wechselkurs <= 0` pass at face value.
- **SuSa:** accumulate Soll/Haben per `Buchung.Entries[].Konto`; `Saldo = round2(Soll − Haben)`; sort ascending by Konto; name fallback = number-as-string.
- **GuV:** partition SuSa by chart `Type`; revenue contribution `= round2(Haben − Soll)`, expense `= round2(Soll − Haben)`; `Ergebnis = ErloeseGesamt − AufwandGesamt`; nil chart → empty GuV; only `"revenue"`/`"expense"` types count.
- **OPOS open test:** `Bezahldatum` blank AND `BuchungRef` blank (both trimmed). Forderung iff `Ausgangsrechnung`, else Verbindlichkeit. `AgeDays = floor(days)` from `Rechnungsdatum` to `asOf`, 0 if missing/unparseable/future. Buckets with **en-dash**: `0–30 / 31–60 / 61–90 / >90`, upper-inclusive at 30/60/90. `Betrag = Bruttobetrag` (EUR). `asOf = now`.
- **Controlling exclusions:** payment accounts ∪ Vorsteuer ∪ Umsatzsteuer ∪ §13b VStRC/UStRC. On the rest, Soll → Ausgaben, Haben → Einnahmen; `Saldo = Einnahmen − Ausgaben`; per-account `round2`, sorted by Konto.
- **Year overview carry:** first stored book anchors opening; later stored books' openings are ignored (deposits still apply); empty months carry the previous close; `carriedIn` used until the anchor. 12 summaries, January first.
- **Cash report ordering:** datable entries first, ascending by date (invoice `Bezahldatum`→`Rechnungsdatum`); undatable last (stable); running `Saldo` from `Anfangsbestand`; deposit→Einnahme, invoice→Ausgabe (gross).
- **Overview KPIs:** `Zahllast = Σ tax(out) − Σ tax(in)`; `OpenReconcile` = `Bankkonto set AND BuchungRef empty`; `Warnings` = rows with ≥1 `InvoiceWarnings`; **no** EUR conversion; raw float sums. Dialog shows only Count/Brutto/OpenReconcile/Warnings + month label.
- **PDF format:** A4, cp1252, Arial; title 14 bold + `"[company  ·  ]Erstellt am DD.MM.YYYY"` sub-header; `"Seite X / N"` footer; bordered tables; amounts as German comma, **no** thousands separator; rune-safe truncation; auto page-break re-draws the header. Use the exact column orders, widths, truncation limits, totals-span widths, orientation (SuSa/GuV/Controlling/UStVA/ZM/Cash-overview/Anlagen = portrait; OPOS/Belegliste/Sales-journal/Booking-journal = landscape) and titles listed per report.
- **Belegliste** = all rows, current month. **Rechnungsausgangsbuch** = `Ausgangsrechnung` rows only, current month. **Buchungsjournal** = exportable rows, one line per debit entry of each `Balanced()` booking with exactly one credit entry; counter = the single Haben account; total = Σ Soll amounts. Foreign rows append `" (CUR orig,gross)"` in Brutto (lists) / Auftraggeber (journal).

---

## Fixed Assets (AfA) & Cash Book (Kassenbuch)

This chapter specifies two related subsystems that share a JSON-file-per-folder persistence model and the German-bookkeeping conventions of the rest of the app: the **fixed-asset register** (Anlagenbuchhaltung / AfA) and the **cash book** (Kassenbuch / Kassenbericht). Both are pure computed views over stored data — neither writes accounting bookings into the journal.

All dates in stored data and in computed output use the German format **`DD.MM.YYYY`** (parser format `02.01.2006`, leading-zero day/month, 4-digit year). All monetary values are stored as decimals (2 fractional digits semantically) and rounded to 2 places via `round2(v) = round(v*100)/100` (round half away from zero, the standard arithmetic rounding).

---

### 1. Fixed Assets (AfA)

#### 1.1 Asset record

An asset (one fixed asset / Anlagegut) has these fields. JSON keys are given exactly; field order in the persisted JSON array is this order.

| Field | JSON key | Type | Meaning |
|---|---|---|---|
| ID | `id` | string | Stable identifier (see ID generation below) |
| Bezeichnung | `bezeichnung` | string | Asset description / label |
| Anschaffungsdatum | `anschaffungsdatum` | string `DD.MM.YYYY` | Acquisition date |
| Anschaffungswert | `anschaffungswert` | decimal | Acquisition cost (net) = AK / "Bemessungsgrundlage" |
| NutzungsdauerJahre | `nutzungsdauer_jahre` | int | Useful life in whole years (ND) |
| Konto | `konto` | int | Asset account number (Anlagekonto, e.g. SKR03 420) |
| AfaKonto | `afa_konto` | int | Depreciation expense account (e.g. SKR03 4830) |
| BelegRef | `beleg_ref` | string, omitted when empty | Belegnummer of the source invoice if the asset was created from one |

> Quirk: `Konto` and `AfaKonto` are stored and displayed but are **never used in any computation** in this subsystem. There is no AfA booking generation — the asset register produces only computed report values (AfA per year and residual book value), never journal entries. A re-implementer must replicate that AfA is a *reporting-only* feature; the accounts are metadata shown in the Anlagenspiegel context and on the invoice's "In AfA erfasst" note.

**Persistence file: `assets.json`** — a JSON array of asset objects, pretty-printed with 2-space indentation. Load behaviour: a **missing file yields an empty list and no error**. A present-but-unparseable file is an error. Save creates the containing directory (mode 0755) if absent and writes the file (mode 0644). The file path is app-level (one asset register for the whole app/profile), not per-month.

**ID generation** (two code paths, must be reproduced):
- New asset via the manual form: `id = "<N>-<sanitized Bezeichnung>"` where `N = (count of existing assets) + 1` and the sanitizer keeps only ASCII `[A-Za-z0-9]`, truncated to the first 16 characters. (No umlauts, no spaces.)
- Asset created from an invoice: `id = "beleg-" + Belegnummer`, or if Belegnummer is empty `id = "beleg-" + Dateiname`. Creating from an invoice is **idempotent**: any existing asset with the same ID is removed first, then the new one appended (re-create overwrites in place by ID, but the asset moves to the end of the list).

#### 1.2 GWG rule (immediate write-off)

`IsGWG(asset)` is true iff `Anschaffungswert ≤ 800.0` (inclusive). The threshold is the **net** acquisition cost of 800 €.

Threshold is inclusive at exactly 800. Golden values from tests:

| Anschaffungswert | IsGWG |
|---|---|
| 799.99 | true |
| 800.00 | true |
| 800.01 | false |
| 1200.00 | false |

A GWG is fully expensed in the acquisition year: AfA in the acquisition year = the full `Anschaffungswert` (rounded to 2 places); AfA in every other year (before or after) = 0; residual book value at the end of the acquisition year = 0. **No pro-rata-temporis applies to GWG** — even a December acquisition writes off the full amount in that year.

#### 1.3 Linear AfA formula (`LinearAfA(asset, jahr) → decimal`)

Linear depreciation with **pro-rata-temporis (monthly) in the acquisition year only**. Steps, in order:

1. **Guard:** if `NutzungsdauerJahre ≤ 0` OR `Anschaffungswert ≤ 0` → return 0.
2. Parse `Anschaffungsdatum`. If unparseable → return 0. Let `acqYear = year of date`, `acqMonth = month-of-date (1..12)`.
3. If `jahr < acqYear` → return 0.
4. **GWG branch** (`Anschaffungswert ≤ 800`): if `jahr == acqYear` return `round2(Anschaffungswert)`, else return 0.
5. **Annual rate:** `annualRate = Anschaffungswert / NutzungsdauerJahre` (full-precision, not pre-rounded).
6. **First (acquisition) year amount:**
   - `proRataMonths = 13 − acqMonth` (so Jan→12, July→6, Dec→1). This counts the acquisition month *and* every later month of the year.
   - `firstYearAfa = round2( annualRate × proRataMonths / 12 )`.
7. **Determine the last depreciation year:**
   - `remainingAfterFirst = Anschaffungswert − firstYearAfa`.
   - `fullYearsNeeded = ceil( remainingAfterFirst / annualRate )`. If `remainingAfterFirst ≤ 0`, then `fullYearsNeeded = 0`.
   - `lastYear = acqYear + fullYearsNeeded`.
8. If `jahr > lastYear` → return 0.
9. If `jahr == acqYear` → return `firstYearAfa`.
10. Otherwise compute `rbw = Restbuchwert(asset, jahr − 1)` (residual book value at end of the previous year). If `rbw ≤ 0` → return 0.
11. If `jahr == lastYear` → return `round2(rbw)` (whatever remains — absorbs rounding drift so the asset lands exactly on 0).
12. Otherwise (a full mid-life year) → return `round2(annualRate)`.

**Worked example (golden, from `anlagen_test.go`):** Laptop, AK = 1200.00 €, ND = 3 years, Anschaffung = 01.07.2024.
- annualRate = 1200 / 3 = 400.00.
- proRataMonths = 13 − 7 = 6; firstYearAfa = round2(400 × 6/12) = 200.00.
- remainingAfterFirst = 1200 − 200 = 1000; fullYearsNeeded = ceil(1000/400) = ceil(2.5) = 3; lastYear = 2024 + 3 = 2027.

| Year | AfA | Rule applied |
|---|---|---|
| 2023 | 0.00 | before acquisition |
| 2024 | 200.00 | acquisition year (pro-rata 6/12) |
| 2025 | 400.00 | full mid-life year (annualRate) |
| 2026 | 400.00 | full mid-life year |
| 2027 | 200.00 | last year (remaining RBW: 1200 − 200 − 400 − 400 = 200) |
| 2028 | 0.00 | after lastYear |

Total AfA = 200 + 400 + 400 + 200 = 1200 = AK. (Useful-life spans 4 calendar years because the half-year start pushes the tail into a 4th year — this is the expected linear "half-year at both ends" behaviour, time-apportioned monthly.)

**GWG worked example:** Drucker, AK = 700.00 €, ND = 3, Anschaffung = 15.03.2024 → AfA 2024 = 700.00; AfA 2025/2026/2027 = 0; RBW(2024) = 0.

#### 1.4 Residual book value (`Restbuchwert(asset, jahr) → decimal`)

Residual book value at the **end of** the given year = `Anschaffungswert − (sum of LinearAfA for each year from acqYear through jahr)`, floored at 0, then `round2`.

- If the date is unparseable → return `Anschaffungswert` unchanged.
- If `jahr < acqYear` → return `Anschaffungswert`.
- Otherwise iterate `y` from `acqYear` to `jahr` inclusive, accumulate `LinearAfA(asset, y)`, subtract from AK. If the result `< 0`, return 0; else `round2`.

Golden: Laptop RBW(2024) = 1000.00; RBW(2027) = 0.00. GWG Drucker RBW(2024) = 0.00.

> Note: `Restbuchwert` calls `LinearAfA`, and `LinearAfA` (step 10) calls `Restbuchwert(jahr−1)`, which recurses back through earlier years. This mutual recursion is bounded by the acquisition year and is correct, but a re-implementer should be aware it recomputes earlier years repeatedly (O(n²) over the asset's life); behaviour, not performance, is what must match.

#### 1.5 Anlagenspiegel (asset register report)

`Anlagenspiegel(assets, jahr)` returns one row per asset, in the **same order as the assets list**, each row containing:

| Row field | Value |
|---|---|
| Asset | the full asset record |
| AfaJahr | `LinearAfA(asset, jahr)` |
| Restbuchwert | `Restbuchwert(asset, jahr)` |
| GWG | `IsGWG(asset)` |

**Anlagenspiegel PDF** (`BuildAnlagenspiegelPDF(rows, jahr, title, company)`): A4 **portrait**, standard report header (title + company name). One table with columns, in order, and fixed widths (mm):

| # | Header | Width | Align | Content |
|---|---|---|---|---|
| 1 | `Bezeichnung` | 56 | L | description truncated to 32 runes |
| 2 | `Anschaffung` | 24 | L | Anschaffungsdatum (`DD.MM.YYYY`) |
| 3 | `AK (€)` | 26 | R | Anschaffungswert, formatted |
| 4 | `ND (J)` | 14 | R | NutzungsdauerJahre (integer) |
| 5 | `AfA <jahr>` | 26 | R | AfaJahr (header literally embeds the year, e.g. `AfA 2024`) |
| 6 | `Restbuchwert` | 30 | R | Restbuchwert |
| 7 | `GWG` | 12 | C | `"J"` if GWG, else empty |

A bold **totals row** is appended: the first four columns are spanned into one right-aligned cell labelled `Summe`; column 5 shows `round2(sum of AfaJahr)`; column 6 shows `round2(sum of Restbuchwert)`; column 7 is empty. Amounts use the report PDF's amount formatter (`pdfAmount`). The PDF byte stream begins with `%PDF`. Output filename in the UI: `Anlagenspiegel_<jahr>.pdf`.

#### 1.6 Asset creation from an invoice

When an invoice is registered as an asset, the form is pre-filled:
- Bezeichnung = `Auftraggeber + " " + Verwendungszweck` (trimmed).
- Anschaffungsdatum = invoice `Rechnungsdatum`.
- Anschaffungswert (the "AK base", **method B**) = `BetragNetto − Rabatt`; if that is `≤ 0`, fall back to `BetragNetto`. (Net cost reduced by any third-party rebate.)
- NutzungsdauerJahre default = `1` (the BMF-2021 1-year useful life for computers; user edits otherwise). If the user enters `≤ 0`, it is forced to 1.
- Konto default = the invoice's currently selected account; AfaKonto default = `4830` (SKR03 Abschreibungen auf Sachanlagen). Both editable.
- BelegRef = invoice Belegnummer.

`FindAssetByBeleg(assets, belegnummer)` returns the first asset whose `BelegRef == belegnummer`. An **empty** belegnummer never matches (returns not-found immediately). Used to show the "📊 In AfA erfasst: AK …, Nutzungsdauer … J., Anlage … / AfA …" note on the linked invoice.

---

### 2. Cash Book (Kassenbuch)

#### 2.1 Concept and data model

Cash is tracked **per cash-register account** ("Barkasse"). A bank account qualifies as a cash register when its `AccountType == "cash"` (constant `AccountTypeCash = "cash"`). Cash invoices are the invoices whose `Bankkonto` equals the cash account's name and that fall in the relevant month folder.

The cash book is **per month, per account**, stored in `kassenbuch.json` inside that month's folder. A single `kassenbuch.json` is a JSON array; it may contain a separate cash-book object for each cash account.

**CashBook** (one account's book for one month):

| Field | JSON key | Type | Meaning |
|---|---|---|---|
| Konto | `konto` | string | Cash-account name (matches the bank account `Name`) |
| Anfangsbestand | `anfangsbestand` | decimal | Opening balance for the month |
| Einlagen | `einlagen` | array of CashDeposit | Cash inflows (deposits) |

**CashDeposit** (one cash inflow / Bar-Einlage):

| Field | JSON key | Type |
|---|---|---|
| Datum | `datum` | string `DD.MM.YYYY` |
| Beschreibung | `beschreibung` | string |
| Betrag | `betrag` | decimal |

Persisted JSON is pretty-printed, 2-space indent. Load semantics identical to assets: **missing file → empty list, no error**; unparseable → error. Save creates the month folder (0755) and writes mode 0644.

Cash **outflows are not stored in the cash book** — they are derived from cash-paid invoices (their gross amount is the cash expense). Only opening balance and deposits (inflows) are stored.

#### 2.2 The computed cash report (`ComputeCashReport(book, invoices) → (entries, closingBalance)`)

`invoices` must already be filtered to this cash account. The function merges the book's deposits (as **Einnahmen / income**) and the invoices (as **Ausgaben / expense**) into one chronologically ordered list with a running balance.

**Entry construction:**
- Each deposit → `{Datum, Beschreibung, Einnahme = Betrag, Ausgabe = 0, Beleg = ""}`, sort key = parse of `Datum`.
- Each invoice → `{Datum = Bezahldatum or (if empty) Rechnungsdatum, Beschreibung = Auftraggeber, Beleg = Dateiname, Einnahme = 0, Ausgabe = Bruttobetrag}`, sort key = parse of that chosen date string.

> Quirk: the invoice's *gross* amount (`Bruttobetrag`) is used for the cash expense. The cash report itself does no currency conversion — it uses whatever `Bruttobetrag` holds on the filtered rows.

**Ordering** — a stable sort with this comparator:
1. Datable entries (date parsed successfully) sort **before** undatable ones.
2. Among undatable entries, original order is preserved (comparator returns "not less", so stable sort keeps input order).
3. Among datable entries, earlier date first (`t_i < t_j`).
4. Ties (equal dates, or both undatable) preserve insertion order, which is **all deposits first, then all invoices** (deposits are appended before invoices). The sort is *stable*, so this is deterministic and must be reproduced: at the same date a deposit appears before an invoice.

**Running balance:** start `saldo = Anfangsbestand`; for each entry in sorted order `saldo += Einnahme − Ausgabe`, and store that post-update `saldo` on the entry. The function returns the entries (each with its `Saldo`) and the final `saldo` as the closing balance. Note the running balance is **not** rounded per step (it accumulates raw float sums); only display/format applies 2-decimal formatting.

**Worked example (golden, `TestComputeCashReport`):** Anfangsbestand 100; deposit {10.05.2026, "Einlage", 50}; invoices {Auftraggeber "Spät", Dateiname "b.pdf", Brutto 20, Bezahldatum 05.05.2026}, {Auftraggeber "Früh", Dateiname "a.pdf", Brutto 30, Rechnungsdatum 01.05.2026, no Bezahldatum}. Result (3 entries):

| # | Datum | Beschreibung | Einnahme | Ausgabe | Saldo |
|---|---|---|---|---|---|
| 0 | 01.05.2026 | Früh | — | 30 | 70 |
| 1 | 05.05.2026 | Spät | — | 20 | 50 |
| 2 | 10.05.2026 | Einlage | 50 | — | 100 |

Closing balance = 100. (Note "Früh" used its Rechnungsdatum because Bezahldatum was empty.)

**Undatable sorts last (golden, `TestComputeCashReportUndatableSortsLast`):** invoice with `Bezahldatum = "ungültig"` (unparseable) is placed after the datable one regardless of amounts.

#### 2.3 Cash-coverage check (`CashCoverage(book, invoices) → (uncovered, closing)`)

Runs `ComputeCashReport`, then flags every **invoice** entry (an entry whose `Beleg != ""`) where the running `Saldo < −0.005` (i.e. the cash balance went negative at the moment that invoice was booked → not covered by available cash). The returned `uncovered` is a set keyed by invoice `Dateiname` (the `Beleg`). Closing balance is the report's closing balance.

The `−0.005` threshold means a balance of exactly 0 (or any non-negative, or tiny negative within half a cent) is treated as **covered**; only a genuine negative balance flags the invoice.

**Worked example (golden, `TestCashCoverage`):** Anfangsbestand 200; invoices `mueller.pdf` (Brutto 50, Bezahldatum 12.06.2026) and `baecker.pdf` (Brutto 180, Bezahldatum 14.06.2026).
- After Müller: 200 − 50 = 150 (≥ 0 → covered).
- After Bäcker: 150 − 180 = −30 (< 0 → **baecker.pdf uncovered**).
- `uncovered = {baecker.pdf}`; closing = −30.

Adding a deposit {13.06.2026, "Einlage", 100} keeps the balance positive throughout (200 − 50 = 150, +100 = 250, −180 = 70) → `uncovered` is empty.

#### 2.4 Opening-balance carry-over between months

The opening balance of a month is **carried forward** from earlier months. Two code paths implement carry: a single-month carry-in (UI), and the 12-month roll (year overview / core). Both share the rule that **only the first stored cash book in the chain anchors an explicit opening balance; thereafter the balance carries continuously and a later stored book's own `Anfangsbestand` is ignored** (its deposits still count).

**Single-month carry-in (`cashCarryIn(account, year, month)`):**
1. Walk **backwards** month-by-month (up to a `maxLookback = 60` months window), building a chain of preceding (year, month) pairs. At each step, load that month's `kassenbuch.json` and look for a book with `Konto == account`. The first one found is the **anchor**; stop.
2. If no stored book is found within 60 months → return `(0, false)` — caller treats opening as not pre-fillable.
3. Roll forward from the anchor: compute the anchor month's closing balance via `ComputeCashReport(anchorBook, that month's cash invoices)`. Then for every later month up to (but excluding) the target month, build an **empty book seeded with the running balance** (`{Konto: account, Anfangsbestand: balance}`, no deposits) and recompute the closing including that month's cash invoices. The final balance is the carry-in for the target month.

In the cash-book editing view, when an account's book does not yet exist for the current month, it is created on the fly with `Anfangsbestand` pre-filled from `cashCarryIn` (if available).

> Quirk: when rolling forward over months with no stored book, only the cash **invoices** of those months affect the balance — there are no deposits because none are stored. A deposit only exists where someone explicitly saved a `kassenbuch.json` for that month.

#### 2.5 Twelve-month cash-year overview (`ComputeYearOverview(carriedIn, months) → 12 summaries`)

`months` is 12 `MonthInput`s in calendar order (index 0 = January). Each `MonthInput` has: `HasStoredBook` (bool), `Book` (valid only when stored), `Invoices` (that month's cash invoices for the account).

Algorithm:
- `running = carriedIn`; `anchored = false`.
- For each month i (Jan..Dec):
  - `anfang = running` (default opening = carried balance).
  - If `HasStoredBook`: take its `Book`. If not yet anchored, **override** `anfang = Book.Anfangsbestand` and set `anchored = true`. Then **overwrite the book's `Anfangsbestand` with `anfang`** (so later stored books carry the running balance instead of their own opening) while keeping the book's deposits.
  - If no stored book: use an empty book `{Anfangsbestand: anfang}`.
  - Compute the month report; `einnahmen = Σ Einnahme`, `ausgaben = Σ Ausgabe` over its entries; `endbestand = report closing`.
  - Emit `MonthSummary{Month: i+1, Anfangsbestand: anfang, Einnahmen, Ausgaben, Endbestand}`.
  - `running = endbestand` (carries to next month).

`MonthSummary` columns: `Month` (time.Month), `Anfangsbestand`, `Einnahmen`, `Ausgaben`, `Endbestand`.

**Anchoring rule (critical, golden tests):**

`TestComputeYearOverview`: January has a stored book (opening 100, one deposit 50); `carriedIn = 999` is passed but **ignored** because January's stored book anchors the opening. Results:

| Month | Anfangsbestand | Einnahmen | Ausgaben | Endbestand |
|---|---|---|---|---|
| Jan | 100 | 50 | 0 | 150 |
| Feb | 150 | 0 | 30 | 120 |
| Mar | 120 | 0 | 0 | 120 |

(February has no stored book, one cash invoice 30; opening carried from January's close.)

`TestComputeYearOverviewStrayLaterBook`: January anchors at 3497.35 (no deposits/invoices). May has a cash invoice 16.68 → May Endbestand = 3480.67. June has a **stray stored book with opening 0** (saved before the carry chain existed) plus a cash invoice 15.16. The June opening 0 is **ignored** — June opens with May's close (3480.67), not 0, giving June Endbestand = 3480.67 − 15.16 = 3465.51, and July opens at 3465.51. This prevents a stray 0-opening book from resetting the running balance mid-year. A re-implementer MUST replicate that only the first stored book anchors; all later stored books contribute only their deposits.

`TestComputeYearOverviewCarriedIn`: all 12 months empty, no stored books, `carriedIn = 3497.35` → every month opens and closes at 3497.35 (December Endbestand = 3497.35).

For the year-overview UI, the January carry-in is obtained via `cashCarryIn(account, year, January)` (looking into the prior year's months) and passed as `carriedIn`; each month's `MonthInput` is built by loading that month's stored book (matching `Konto`) and that month's cash invoices.

#### 2.6 Kassenbericht PDF (`WriteCashReportPDF`)

One monthly cash report per cash account, written to **landscape A4** (`fpdf.New("L","mm","A4")`). UTF-8 text is translated to cp1252 for the core (Arial) font via a Unicode translator (so umlauts render in the legacy font).

`decimalSep` controls the amount formatter `formatCashAmount(v)`: format `%.2f`, then if `decimalSep == ","` replace the dot with a comma. (No thousands separators in this particular PDF — unlike `FormatAmount` used elsewhere.)

Header block (each on its own line, left-aligned):
- Title, Arial Bold 14: `Kassenbericht - <Konto>`.
- Arial 10: `Monat: <monthLabel>` (the month label, e.g. `2026-05`).
- `Anfangsbestand: <formatted book.Anfangsbestand>`.
- 3 mm vertical gap.

Table — header row Arial Bold 9, body rows Arial 9, all cells bordered, row height 6 mm (header 7 mm). Columns in order with widths (mm):

| # | Header | Width | Align | Content |
|---|---|---|---|---|
| 1 | `Datum` | 25 | L | entry Datum (as-is string) |
| 2 | `Beschreibung` | 80 | L | Beschreibung, truncated to 50 runes |
| 3 | `Beleg` | 75 | L | Beleg (invoice Dateiname), truncated to 48 runes |
| 4 | `Einnahme` | 30 | R | Einnahme if non-zero, else empty |
| 5 | `Ausgabe` | 30 | R | Ausgabe if non-zero, else empty |
| 6 | `Saldo` | 32 | R | running Saldo (always shown) |

Rune-truncation rule (`truncateRunes(s, max)`): if `len(runes) ≤ max` keep as-is; if `max < 4` hard cut to `max` runes; otherwise keep `max−3` runes + `"..."`.

Footer: 3 mm gap, then Arial Bold 11 line `Endbestand: <formatted endbestand>`.

UI filename per account: `Kassenbericht_<SanitizeFilename(account)>_<monthLabel>.pdf`, written into the month folder. Before generating, the cash books are saved.

#### 2.7 Cash-year overview PDF (`BuildCashYearOverviewPDF`)

A4 **portrait**, standard report header. Title literal: `Kassen-Jahresübersicht <year>  ·  <account>`. Columns in order with widths (mm), all bordered, row height 6 mm:

| # | Header | Width | Align | Content |
|---|---|---|---|---|
| 1 | `Monat` | 50 | L | `<German month name> <year>` |
| 2 | `Anfangsbestand` | 35 | R | summary Anfangsbestand |
| 3 | `Einnahmen` | 30 | R | summary Einnahmen |
| 4 | `Ausgaben` | 30 | R | summary Ausgaben |
| 5 | `Endbestand` | 35 | R | summary Endbestand |

One row per month (12 rows), no totals row. Amounts use the report `pdfAmount` formatter. UI filename: `Kassen-Jahresuebersicht_<SanitizeFilename(account)>_<year>.pdf`.

`SanitizeFilename` (used for both PDF filenames): replaces `/` and `\` with `-`; removes `< > : " | ? *` and control chars (0x00–0x1F); collapses runs of whitespace to a single space; trims; **keeps umlauts and spaces**.

---

### Re-implementation checklist

Must-match behaviors for this subsystem:

- **GWG:** `Anschaffungswert ≤ 800.00` (inclusive) → full write-off in acquisition year, 0 in all other years, RBW 0 after; no pro-rata for GWG.
- **Linear AfA pro-rata:** acquisition-year months = `13 − acquisitionMonth`; first-year AfA = `round2(AK/ND × months/12)`; full years = `round2(AK/ND)`; last year = `round2(remaining RBW)` so the asset lands exactly on 0; 0 before acquisition and after the last year. Golden: AK 1200 / ND 3 / 01.07.2024 → 0,200,400,400,200,0 for 2023–2028; RBW(2024)=1000.
- **Restbuchwert** = `max(0, round2(AK − Σ AfA from acqYear..jahr))`; unparseable date or pre-acquisition year → AK unchanged.
- **AfA is reporting-only** — never generates journal bookings; `Konto`/`AfaKonto` are metadata. Anlagenspiegel rows preserve asset order; PDF columns `Bezeichnung·Anschaffung·AK(€)·ND(J)·AfA<year>·Restbuchwert·GWG("J"/"")` with a `Summe` totals row over AfA and RBW.
- **assets.json / kassenbuch.json:** JSON arrays, 2-space indent, exact field keys/order; missing file → empty list + no error; dir created 0755, file 0644.
- **Cash report ordering:** datable before undatable; within datable, earlier date first; stable sort keeps deposits-before-invoices at equal dates and preserves input order for undatable. Date = invoice `Bezahldatum` else `Rechnungsdatum`; expense = `Bruttobetrag`; income = deposit `Betrag`. Running balance starts at `Anfangsbestand`, not per-step rounded.
- **Coverage:** flag invoice entries (Beleg ≠ "") where running balance `< −0.005`; key by invoice Dateiname. Golden: opening 200, −50 then −180 → baecker.pdf uncovered, closing −30.
- **Carry-over:** opening balance carries continuously; **only the first stored cash book anchors** an explicit opening; later stored books' `Anfangsbestand` is ignored (deposits kept). Single-month carry walks back ≤ 60 months to the anchor then rolls forward through invoices. Golden stray-book test: June 0-opening ignored, June opens at May's close.
- **Kassenbericht PDF:** landscape A4, cp1252 translation, no thousands separator (only decimal-sep swap), columns `Datum·Beschreibung·Beleg·Einnahme·Ausgabe·Saldo` (widths 25/80/75/30/30/32), Beschreibung≤50 / Beleg≤48 runes with "..." ellipsis, empty cell for zero Einnahme/Ausgabe, `Endbestand` footer.
- **Year-overview PDF:** portrait A4, columns `Monat·Anfangsbestand·Einnahmen·Ausgaben·Endbestand` (widths 50/35/30/30/35), 12 rows, no totals.
- **Asset-from-invoice:** AK = `BetragNetto − Rabatt` (fallback `BetragNetto` if ≤0); default ND 1 (forced to 1 if ≤0); default AfaKonto 4830; ID `beleg-<Belegnummer>` (or `beleg-<Dateiname>`), idempotent replace-by-ID; `FindAssetByBeleg` never matches empty Belegnummer.

---

## Bank Import & Reconciliation

This chapter specifies how BuchISY ingests bank-statement files (CAMT.053, MT940, Qonto/generic PDF), turns them into a uniform list of transaction *bookings*, and reconciles those bookings against stored receipts/invoices ("Belegabgleich" for expenses, "Erlös-Abgleich" for revenue). All formulas, formats, constants, and quirks below are IST-exact: they describe what the current code does, including its weaknesses.

### 1. Core data model

#### 1.1 StatementBooking (one parsed transaction line)

A booking is the universal output of every parser. Fields (JSON keys in parentheses):

| Field | Type | JSON key | Meaning |
|---|---|---|---|
| Page | int | `page` | 0-based PDF page index. Always `0` for CAMT/MT940/Qonto. |
| LineIdx | int | `line_idx` | 1-based sequence index. For PDF heuristic it restarts per page; for CAMT/MT940/Qonto it is a single monotone counter across the whole file. |
| Date | string | `date` | `DD.MM.YYYY` or the year-less short form `DD.MM.` |
| TopPt | decimal | `top_pt` | Vertical PDF position in points (PDF heuristic only; 0 otherwise). |
| LeftPt | decimal | `left_pt` | Leftmost x position in points (PDF heuristic only; 0 otherwise). |
| Text | string | `text` | Full visible line text / purpose / counterparty. |
| Betrag | decimal | `betrag` (omitempty) | **Absolute** (always ≥ 0) amount of the line. |
| IstGutschrift | bool | `gutschrift` (omitempty) | true = incoming credit (Haben); false = outgoing debit (Soll). |
| InvoiceRef | object/null | `invoice_ref` (omitempty) | Back-pointer to the linked invoice; null = unlinked. |

`InvoiceRef` (line→invoice pointer, the mirror of the invoice's `BuchungRef`):
- `MonthFolder` (string, JSON `month_folder`): storage-root-relative folder, e.g. `"2026/2026-01"` (empty when month-subfolders disabled).
- `Filename` (string, JSON `filename`): basename of the invoice PDF.
- String form: `MonthFolder + "/" + Filename`, or just `Filename` when `MonthFolder` empty.

`Display()` of a booking: `"S.{Page+1} Z.{LineIdx} — {Date}"` (e.g. `"S.1 Z.3 — 14.01.2026"`).

#### 1.2 BuchungRef (invoice→line pointer)

Stored on each invoice/CSVRow as a single string field `BuchungRef`. Wire format:

```
<statementFilename>|<page>|<lineIdx>
```

where page is 0-based and lineIdx is 1-based. Example: `Auszug_2026_0002.pdf|0|3`.

- `String()`: returns the wire format; returns empty string when `StatementFilename` is empty.
- `IsZero()`: true when `StatementFilename` is empty.
- `Display()`: `"{StatementFilename} · S.{Page+1} Z.{LineIdx}"` (e.g. `"Auszug_2026_0002.pdf · S.1 Z.3"`); empty when zero.
- `ParseBuchungRef(s)`: trims whitespace; splits on `|`; requires **exactly 3 parts**; parses parts[1]/parts[2] as integers. **Any** malformation → zero value (no error raised). Test golden: inputs `""`, `"garbage"`, `"file.pdf|abc|def"`, `"file.pdf|0"` all yield zero. Round-trip of `{StatementFilename:"Auszug_2026_0002.pdf", Page:0, LineIdx:3}` → `"Auszug_2026_0002.pdf|0|3"` → parses back identically.

> Quirk: The invoice→line link (`BuchungRef` on CSVRow) and the line→invoice link (`InvoiceRef` on booking) are two **independent** stores. The invoice CSV is the source of truth for "is this line claimed?" during reconciliation (a line is "linked" iff some invoice's `BuchungRef` equals that line's key). `InvoiceRef` on the booking is a separate persisted mirror in the statement's `metadata.json`. A re-implementer must keep both in sync but treat the CSV `BuchungRef` as authoritative for reconcile status.

#### 1.3 Statement metadata + booking cache (metadata.json)

Each account folder contains one `metadata.json` (`StatementMetaPath = accountFolder + "/metadata.json"`), a map keyed by the **statement file's relative path within the account folder** (e.g. `"2026/Auszug-05.pdf"`). Value = `StatementMetadata`:

| Field | Type | JSON key | Meaning |
|---|---|---|---|
| DateFrom | string | `date_from` | `DD.MM.YYYY` period start |
| DateTo | string | `date_to` | `DD.MM.YYYY` period end |
| Number | string | `number` | e.g. `"5/2026"` |
| OpeningBalance | decimal | `opening_balance` | start-of-period balance |
| ClosingBalance | decimal | `closing_balance` | end-of-period balance |
| Reviewed | bool | `reviewed` | user checked off |
| Note | string | `note` | free text |
| BookingsParsedMtime | int64 | `bookings_parsed_mtime` (omitempty) | source PDF mtime (Unix seconds) when `Bookings` was last refreshed — a cache key |
| Bookings | array | `bookings` (omitempty) | cached `StatementBooking` list |

- Serialized as indented JSON (2-space indent). File mode 0644; folder created with 0755 if missing.
- `LoadStatementMeta`: missing file → empty (non-nil) map, no error. Unmarshal failure → empty map **plus** an error returned (caller decides).

**Cache freshness (`EnsureBookingsParsed`)**: given a PDF path and a metadata entry:
1. `stat` the file; read mtime (Unix seconds).
2. If `meta.BookingsParsedMtime == mtime` **and** `len(meta.Bookings) > 0` → no-op, return "not modified".
3. Otherwise re-parse via `ParseStatementBookings`, then **preserve existing InvoiceRef links** by matching old→new bookings on the `(Page, LineIdx)` pair: any old booking that had a non-nil `InvoiceRef` re-attaches it to the new booking with the same `(Page, LineIdx)`. Store new bookings + new mtime; return "modified" (caller must persist).

> Quirk: Link preservation keys solely on `(Page, LineIdx)`. If a re-parse renumbers lines (e.g. a PDF edit inserts a row), links silently migrate to whatever line now occupies that index.

---

### 2. Format auto-detection

`ParseStatementBookings(path)`:
1. Read the whole file. If readable, call `DetectBankFormat(data)`:
   - Returns `"camt"` when the raw text **contains both** the substring `"<Document"` **and** `"BkToCstmrStmt"`.
   - Returns `"mt940"` when the raw text **contains the substring `":61:"`** (the MT940 statement-line tag).
   - Returns `""` otherwise.
2. `"camt"` → `ParseCAMT053`; `"mt940"` → `ParseMT940`.
3. Anything else (including unreadable file) → fall through to `parseStatementPDF`.

> Quirk: Detection is substring-based, order-independent, and not anchored. A `.txt` file that merely contains `:61:` anywhere is treated as MT940. A CAMT file is only detected if both magic substrings are present (namespace-agnostic).

---

### 3. CAMT.053 parser (ISO 20022)

`ParseCAMT053` is a **streaming, namespace-agnostic** XML token walk. It matches elements by **local name only** (namespace URIs are ignored), and treats every charset as UTF-8 (a custom CharsetReader passes bytes through unchanged). One booking is produced per `<Ntry>` element.

#### 3.1 Element → field mapping

While inside an `<Ntry>`, character data is assigned by the **current leaf element name** (and, for dates, its parent):

| Source element (local name) | Maps to | Notes |
|---|---|---|
| `Amt` | raw amount text | dot-decimal string, e.g. `"300.00"` |
| `CdtDbtInd` | credit/debit indicator | `"CRDT"` → credit; anything else → debit |
| `BookgDt` → `Dt` | booking date (preferred) | only first non-empty kept |
| `ValDt` → `Dt` | value date (fallback) | only first non-empty kept |
| `AddtlNtryInf` | text (preferred) | |
| `Ustrd` (each occurrence) | appended to `ustrdParts` | used only if `AddtlNtryInf` empty |

#### 3.2 Per-entry assembly (on `</Ntry>`)

- **Date**: use `BookgDt` if non-empty, else `ValDt`. Then convert `YYYY-MM-DD` → `DD.MM.YYYY` (`camtDateToGerman`: requires exactly length 10 with `-` at positions 4 and 7; otherwise returned unchanged).
- **Text**: `trim(AddtlNtryInf)`; if empty and `ustrdParts` non-empty, join all `Ustrd` parts with a single space.
- **Betrag**: parse the `Amt` text as a float (dot decimal). Parse error → 0. **No abs() is applied** (CAMT amounts are unsigned in the wire format, so this is fine in practice).
- **IstGutschrift**: `CdtDbtInd == "CRDT"`.
- **Page** = 0; **LineIdx** = running counter starting at 1.

#### 3.3 Worked example (test golden)

Input two `<Ntry>` elements (see `camtSample`):

- Entry 1: `Amt=300.00`, `CdtDbtInd=CRDT`, `BookgDt/Dt=2026-01-12`, `AddtlNtryInf="Gutschrift Kunde A"` →
  `{LineIdx:1, Page:0, Date:"12.01.2026", Betrag:300.00, IstGutschrift:true, Text:"Gutschrift Kunde A"}`
- Entry 2: `Amt=55.24`, `CdtDbtInd=DBIT`, no BookgDt but `ValDt/Dt=2026-01-15`, no AddtlNtryInf but `Ustrd="Lieferant B Rechnung 2026-007"` →
  `{LineIdx:2, Page:0, Date:"15.01.2026", Betrag:55.24, IstGutschrift:false, Text:"Lieferant B Rechnung 2026-007"}`

---

### 4. MT940 parser

`ParseMT940` parses the SWIFT MT940 `:tag:` structure. One booking per `:61:` transaction line; its narrative comes from the immediately following `:86:`.

#### 4.1 Tokenizing into tagged fields

- Normalize line endings: `\r\n`→`\n`, `\r`→`\n`; split on `\n`.
- A line **starting with `:`** is a tag start **iff** a second `:` is found within the next 1..5 characters (`end >= 0 && end <= 4` searching in `ln[1:]`). The tag is the text between the two colons (e.g. `20`, `25`, `28C`, `60F`, `61`, `86`); the value is the rest of the line after the second colon, trimmed.
- A lone line `-` ends the MT940 block (flushes the current field).
- Any other non-empty line is appended to the current field's value as `"\n" + line` (multi-line continuation).

#### 4.2 Parsing a `:61:` line

`:61:` value format parsed positionally: `YYMMDD[YYMMDD]<mark><amount><rest>`.
1. Require length ≥ 10, else skip.
2. **Date** = first 6 chars `YYMMDD`. `mt940DateToGerman`: `DD.MM.20YY` (the century is **hard-coded `20`**; YY is taken literally).
3. Skip an **optional entry date**: consume all leading ASCII digits after the value date (this swallows the optional `MMDD` if present).
4. **Mark** (credit/debit): try two-char `RC`/`RD` first, else one-char `C`/`D`; otherwise skip the line. `IstGutschrift = mark starts with "C"` (so `C` and `RC` are credits; `D` and `RD` are debits).
5. **Amount**: consume the run of digits and commas immediately after the mark; if empty, skip. `parseMT940Amount`: replace `,`→`.`, parse float, take absolute value (so always ≥ 0). Parse error → 0.

#### 4.3 Narrative (`:86:`)

If the field **immediately after** the `:61:` field is an `:86:`, its value (trimmed) is the booking text, with embedded newlines collapsed to single spaces (`"\n"`→`" "`, then trim).

> Quirk: Only the *immediately following* `:86:` is consumed. Banks that emit `:61:`/`:61:`/`:86:`/`:86:` blocks, or place `:86:` elsewhere, would lose narratives. `?`-prefixed structured subfields inside `:86:` are **not** decoded — the raw text is kept verbatim.

#### 4.4 Worked example (test golden)

```
:61:2601060106C300,00NTRFNONREF
:86:Gutschrift von Kunde X
:61:2601100110D55,24NTRFNONREF
:86:Lastschrift Lieferant Y
-
```
→
- `{LineIdx:1, Date:"06.01.2026", Betrag:300.00, IstGutschrift:true, Text:"Gutschrift von Kunde X"}` (date `260106`, optional entry date `0106` skipped, mark `C`)
- `{LineIdx:2, Date:"10.01.2026", Betrag:55.24, IstGutschrift:false, Text:"Lastschrift Lieferant Y"}`

---

### 5. PDF parsing (MuPDF positioned-text dependency)

When format detection yields neither CAMT nor MT940, `parseStatementPDF` runs. **Hard dependency: positioned-text extraction.** The current implementation uses MuPDF (`go-fitz`) and asks it for **HTML per page** (`doc.HTML(page, false)`). MuPDF's HTML output emits one `<p style="top:Npt;left:Mpt;...">…</p>` per text run, with **absolute pt coordinates**. A re-implementer must reproduce this: extract each text run **with its absolute (top,left) position in PDF points** — plain `getText()` without coordinates is insufficient because line reassembly and ordering depend on positions.

#### 5.1 HTML run extraction (`splitPTags` + regex)

- `splitPTags`: a zero-dependency string walk that returns each `"<p ...>...</p>"` substring (finds `"<p "`, then the next `"</p>"`).
- For each chunk:
  - `top` = first capture of `top:\s*([\d.]+)pt`; `left` = `left:\s*([\d.]+)pt`. If either is absent, **skip the chunk**.
  - Visible text = substring between the first `>` and the last `</p>`; strip all HTML tags (`<[^>]+>`); HTML-unescape entities (e.g. `&#xf6;`→`ö`); trim. Empty → skip.
- Collect runs as `(top, left, text)` and **sort** by `top` ascending, then `left` ascending (so document reading order is restored even if runs were emitted out of order).

#### 5.2 Generic statement heuristic (`bookingsFromPageHTML`)

Per page, for each sorted run whose text **starts with a date prefix**:
- Date prefix regex (anchored at line start, leading whitespace allowed): `^\s*([0-3]?\d)\.([01]?\d)\.(\d{4}|\d{2}|)` — accepts `DD.MM.` (short, Sparkasse-style), `DD.MM.YY`, and `DD.MM.YYYY`. The captured year may be empty.
- Only runs matching this are emitted as bookings. **The matched date is the trimmed full match `m[0]`** (so a short `05.01.` stays `05.01.`; the year is resolved later by the UI from the statement period).
- `LineIdx` is a 1-based counter restarting per page; `Page` set; `TopPt`/`LeftPt` retained.
- `Betrag` = `ParseLineAmount(text)`; `IstGutschrift` = `ParseLineIsCredit(text)`.

> Quirk (intentional): Because the date must be at the **start** of the line, summary rows like `"Kontostand am 02.01.2026"` (date in the middle) are correctly skipped. This is a load-bearing behavior — replicate the start-anchor.

**`ParseLineAmount(text)`**: finds all German money tokens via `\d{1,3}(?:\.\d{3})*,\d{2}`, takes the **LAST** match (transaction amount sits at line end), removes `.` (thousands), replaces `,`→`.`, parses float; returns the value (no abs needed since the regex has no sign). No match → 0.
- Goldens: `"14.01.2026 AMAZON WEB SERVICES EMEA 78,53"`→`78.53`; `"03.01. Lastschrift Telekom -1.234,56"`→`1234.56`; `"05.01. Gutschrift Kunde 2.000,00 H"`→`2000.00`; `"no amount here"`→`0`.

**`ParseLineIsCredit(text)`** — returns true only when the line is *clearly* a credit, else false (ambiguous lines default to debit so a real expense match is never dropped):
- true if it matches (case-insensitive) `gutschrift|zahlungseingang|geldeingang|überweisungseingang|lohn|gehalt`, **or**
- true if it matches a trailing credit marker: amount followed by ` H` or `+` — regex `\d,\d{2}\s*([H+])\b|\d,\d{2}\s*\+`.
- Goldens credits: `"05.01. Gutschrift Kunde 2.000,00 H"`, `"03.01. Zahlungseingang Müller 500,00"`, `"07.01. SEPA-Gutschrift 80,00 +"`. Goldens debits: `"14.01. AMAZON WEB SERVICES 78,53"`, `"03.01. Lastschrift Telekom -49,99"`, `"02.01. Kartenzahlung REWE 23,40"`.

#### 5.3 Worked PDF example (test golden)

A page with runs `"05.01.2026 LS-Einlösung Adobe"`, `"07.01.2026 ... Google"`, `"07.01.2026 ... Slack"` plus two `"Kontostand am …"` rows yields **3** bookings (LineIdx 1,2,3); the Kontostand rows are skipped; `&#xf6;` unescapes to `ö` so the first text is `"05.01.2026 LS-Einlösung Adobe"`.

---

### 6. Qonto PDF parser

Before the generic heuristic runs, `parseStatementPDF` builds a single plain-text string from all pages (`buildPlainTextFromHTML`, same `<p>` extraction + top/left sort, one run per line). If that text **contains both** `"Qonto"` **and** `"Abrechnungstag"`, the Qonto parser runs; if it returns **≥ 1** booking, that result is used. Otherwise it falls through to the generic heuristic.

`parseQontoStatement(text)` — line-oriented state machine over `\n`-split text:

1. **Year**: from header `Vom DD/MM/YYYY` via `Vom\s+\d{2}/\d{2}/(\d{4})`. Empty if header absent.
2. Skip header/footer lines whose trimmed form matches `^(Kontostand|Eingänge|Ausgänge|Abrechnungstag|Kontoauszüge)`.
3. **New transaction** when a trimmed line matches `^(\d{2})/(\d{2})\b(.*)` → finalize any open transaction, increment `LineIdx`, start a new pending with day=`m[1]`, month=`m[2]`, text = trimmed `m[3]`.
4. Within an open transaction:
   - Skip any line containing `USD` (raw USD amounts **and** FX-rate lines like `1.15220647540039 USD = 1.00 EUR`).
   - If a line matches a standalone EUR amount `^\s*([-+])\s*([\d.,]+)\s+EUR\s*$` **and** no amount has been captured yet: set `Betrag = parseQontoAmount(m[2])`, `IstGutschrift = (sign == "+")`, mark amount captured. (Only the **first** EUR amount counts; later EUR lines are ignored as further description? No — once `hasAmt`, the amount branch is skipped and the line is appended as description.)
   - Otherwise append the trimmed line to the description, joined with `" / "`.
5. **finalize**: emit a booking **only if an amount was captured** (`hasAmt`). Date = `DD.MM.` + year if year present, else `DD.MM.` (year-less). `Page`=0.

**`parseQontoAmount(s)`**: if `s` contains a comma → German format: strip `.` (thousands), replace `,`→`.`; otherwise it is already dot-decimal. Parse float; error → 0.
- Goldens dot: `"108.00"`→108.00, `"75404.05"`→75404.05, `"86.79"`→86.79, `"17850.00"`→17850.00, `"18.00"`→18.00.
- Goldens German: `"1.793,68"`→1793.68, `"2.000,00"`→2000.00, `"1.234,56"`→1234.56.

#### 6.1 Worked example (test golden)

From `sampleQontoText` (period `01/04/2026 … 30/04/2026`), 5 transactions are emitted:

| idx | Date | Betrag | IstGutschrift | notes |
|---|---|---|---|---|
| 0 | 01.04.2026 | 108.00 | false | `01/04 Qonto` (debit) |
| 1 | 02.04.2026 | 18.00 | false | CLAUDE.AI |
| 2 | 09.04.2026 | 86.79 | false | EUR kept, `- 100.00 USD` and the FX-rate line ignored |
| 3 | 23.04.2026 | 75404.05 | true | SW Operations GmbH, `+` → credit |
| 4 | 24.04.2026 | 17850.00 | false | euhost |

Header `Kontostand` lines (`01/04`, `30/04`) are not emitted. Without a `Vom …` header, dates stay year-less (`"01.04."`). `LineIdx` is `i+1`.

---

### 7. Reconciliation matcher (Belegabgleich / Erlös-Abgleich)

The core scorer is `matchToStatement(row, lines, cfg, wantCredit)`. Two public entry points:
- **`MatchInvoiceToStatement`** (expenses): `wantCredit=false` → matches **debit** lines (`IstGutschrift=false`).
- **`MatchRevenueToStatement`** (revenue / Ausgangsrechnung): `wantCredit=true` → matches **incoming credit** lines (`IstGutschrift=true`).

#### 7.1 Target amount (`InvoiceEURAmount(row)`)

`round2(eurRow.Bruttobetrag + eurRow.Gebuehr − eurRow.Rabatt)` where `eurRow = RowEUR(row)`:
- For EUR rows (or blank currency): pass-through (no conversion).
- For foreign rows **with** a valid rate (`Wechselkurs > 0`): money fields divided by the rate and `round2`-ed. `Gebuehr` (bank/CC FX fee) is **already in EUR and NOT divided**. `Rabatt` is converted along with the rest.
- For foreign rows with **missing** rate (`Wechselkurs ≤ 0`): face-value pass-through.
- `round2(v) = round(v*100)/100`.

Worked goldens:
- Plain EUR with discount: `Bruttobetrag=1329.05, Rabatt=50` → `1279.05`. `Rabatt=0` → `1329.05`.
- Foreign (USD): `Bruttobetrag=89.18, Wechselkurs=1.1583, Gebuehr=1.54` → `round2(round2(89.18/1.1583)+1.54) = round2(76.99+1.54) = 78.53`.

#### 7.2 Algorithm (per candidate line)

```
amount = InvoiceEURAmount(row)
if amount <= 0: return MatchNone, []

# amount tolerance
tol = 0.01
if row.Waehrung != "" and row.Waehrung != "EUR" and cfg.ForeignTolerancePct > 0:
    band = amount * cfg.ForeignTolerancePct / 100
    if band > tol: tol = band

invDate = row.Bezahldatum or (if empty) row.Rechnungsdatum
nameTokens  = tokenize(row.Auftraggeber)
aliasTokens = cfg.Aliases[lower(trim(row.Auftraggeber))]
window = cfg.DateWindowDays (if <= 0 → 5)

for each line L:
    if L.IstGutschrift != wantCredit: skip          # type gate
    if abs(L.Betrag - amount) > tol: skip           # amount gate
    days       = dayDistance(invDate, L.Date)
    dateScore  = 1.0 / (1.0 + days)                 # 0d→1.0, decays
    nameScore  = tokenOverlap(nameTokens, lineTokens)
    aliasScore = tokenOverlap(aliasTokens, lineTokens)
    if aliasScore > nameScore: nameScore = aliasScore
    candidate.Score = dateScore*2 + nameScore       # date weighted 2×
```

Candidates are stable-sorted by **Score descending**. Outcome classification:
- No candidates → `MatchNone`.
- **Exactly one** candidate **and** its `dayDistance(invDate, line) ≤ window` → `MatchAuto`.
- Otherwise → `MatchSuggest`.

#### 7.3 Tokenizer & overlap

`tokenize(s)`: lowercase, split on any rune **not** in `[a-z]`, `[0-9]`, or the byte range `ä..ÿ`; keep tokens of **length ≥ 3**.

> Quirk: the "letter" range is the literal Go rune range `'ä'..'ÿ'` (U+00E4..U+00FF), which is narrower than real accented-letter coverage and includes some non-letters; replicate this exact range, not a Unicode letter class.

`tokenOverlap(a, b)`: fraction of `a`'s tokens that appear in `b`, where "appear" means **bidirectional substring** (`b_token contains a_token` OR `a_token contains b_token`). Returns `hits/len(a)`; 0 when `a` empty.

`dayDistance(a, b)`: absolute whole-day difference between two dates, rounded (`int(d+0.5)`). Each date parsed with `parseFlexDate`, which accepts `DD.MM.YYYY` or `DD.MM.` (taking the missing year from the *other* date; 2-digit year → `20`+YY). If **either** date is unparseable → returns **9999** (effectively never within window).

#### 7.4 Default & configured match config

`DefaultMatchConfig`: `DateWindowDays = 5`, `ForeignTolerancePct = 1.5`. `Aliases` empty.

UI override (`matchConfig()`): use settings `MatchDateWindowDays` if `> 0`, else default 5; use `MatchForeignTolerancePct` if `> 0`, else default 1.5; load `Aliases` from the alias store. Settings persist as `matchDateWindowDays` (int) and `matchForeignTolerancePct` (decimal, `0`=use default).

#### 7.5 Worked match-score example

Invoice `AWS, Bezahldatum=14.01.2026, Bruttobetrag=78.53, EUR` (amount 78.53, tol 0.01) against debit lines:
- L1 `12.01.2026 "Lastschrift Telekom 49,99"` (49.99) — amount gate fails (|49.99−78.53|>0.01), skipped.
- L2 `14.01.2026 "AMAZON WEB SERVICES 78,53"` (78.53): days=0 → dateScore=1.0; nameTokens(`AWS`)=`["aws"]`; lineTokens=`["amazon","web","services"]`; no substring overlap → nameScore=0. **Score = 1.0×2 + 0 = 2.0**.
- L3 `20.01.2026 "REWE Markt 78,53"` (78.53): days=6 → dateScore=1/7≈0.1429; nameScore=0. Score ≈ 0.2857.

Using only L1,L2 → one candidate (L2), days 0 ≤ 5 → **MatchAuto**, top=L2.
Using L1,L2,L3 → two amount-matches (L2,L3) → **MatchSuggest**, sorted L2 (2.0) before L3 (0.286).
A `999`-amount invoice → no amount match → **MatchNone**.

**Foreign tolerance example**: USD invoice `Bruttobetrag=91.39, Wechselkurs=1.1583` → EUR ≈ `round2(91.39/1.1583)=78.90`; tol band = `78.90×1.5/100 ≈ 1.18`. A debit line of `78.90` matches (within band), while the `78.53 H` **credit** line is excluded by the type gate. A strict EUR invoice of `78.53` will **not** match a `78.90` line (tol 0.01) → `MatchNone`.

**Alias boost example**: with `cfg.Aliases = {"aws": ["amazon"]}`, invoice `AWS` matches line `"AMAZON WEB SERV 78,53"` because aliasTokens=`["amazon"]` overlap lineTokens (nameScore rescued to 1.0).

#### 7.6 Revenue mirror & cross-direction safety

`MatchRevenueToStatement` is identical but `wantCredit=true`. The type gate guarantees the expense matcher **never** returns a credit line and the revenue matcher **never** returns a debit line, even when amounts/dates coincide (test: invoice `1190` matches the credit line idx1, not the debit line idx2; the expense matcher returns no credit lines).

---

### 8. Grouped (n:1) and partial (1:n) payments

#### 8.1 Grouped payments (`findGroupedPayments`)

Finds one statement line whose `Betrag` equals the **sum of 2 or 3 invoices** within the date window. `FindGroupedPayments` (debits, `wantCredit=false`) and `FindGroupedRevenuePayments` (credits, `wantCredit=true`).

Per line L (skip if `L.IstGutschrift != wantCredit` or `L.Betrag ≤ 0`):
1. Build candidate invoices not yet used (`usedFilenames`), with `InvoiceEURAmount > 0`, and `dayDistance(invDate, L.Date) ≤ window` (invDate = Bezahldatum or Rechnungsdatum).
2. **Pairs first** (nested i<j): if `round2(amt_i + amt_j)` is within `0.01` of `L.Betrag` → emit a `GroupMatch{Dateinamen:[i,j], Line:L}`, mark both filenames used, stop (first match wins).
3. If no pair: **triples** (i<j<k) with the same `0.01` tolerance → emit 3-invoice group.

`GroupMatch`: `Dateinamen` (list of invoice `Dateiname`), `Line` (the statement booking), `File` (source statement filename — left empty by core, **filled by the caller** from the statement cache).

> Quirk: Groups are **disjoint** (an invoice is never reused across groups), but selection is greedy/first-found per line and order-dependent. Sums are limited to size 2 and 3 only.

Goldens: invoices `a=30,b=70,c=999` + line `100` → one 2-group `{a,b}` (not c). `a=20,b=30,c=50` + line `100` → one 3-group. A credit line of `100` produces **no** debit group (and vice-versa). Revenue: two outgoing invoices `100+200` + credit line `300` → one 2-group; a `300` **debit** line produces no revenue group.

#### 8.2 Partial payments (`partialPaymentLines`)

Only when `row.Teilzahlung == true` (else returns nil/empty). `PartialPaymentLines` (debits) and `RevenuePartialPaymentLines` (credits).

For each line L: skip if `L.IstGutschrift != wantCredit`; require `0 < L.Betrag < InvoiceEURAmount(row) − 0.01` (a strict partial). Score `= 1/(1+dayDistance(invDate, L.Date))`. Stable-sort by score descending.

Golden: invoice full `100, Teilzahlung=true` against lines `[50 debit, 50 credit, 100 debit]` → exactly one candidate (the 50 debit, idx1); the 50 credit excluded by type gate, the 100 debit excluded (not < full−0.01). A non-Teilzahlung row → empty.

---

### 9. Alias learning (statementalias)

Persisted at `<configDir>/statement_aliases.json`: a map `lowercase_supplier → [tokens]`, indented JSON, mode 0644.

`Learn(supplier, lineText)`:
- key = `lower(trim(supplier))`; empty key → no-op.
- For each token from `tokenize(lineText)` (the same matcher tokenizer, len ≥ 3), keep it only if: **len ≥ 4**, **not pure ASCII digits**, **not** already one of the supplier-name's own tokens (avoids circular self-matches), and **not** already stored. Union (dedupe) into the supplier's slice.
- `isPureDigits`: true iff non-empty and every rune is `0..9`.

`Load`: reads the JSON if present (missing file → returns current in-memory map, no error); returns a **deep copy** so callers can't mutate internal state. `Save`: writes the in-memory map (creating configDir if needed).

Golden: `Learn("AWS", "14.01. AMAZON WEB SERVICES EMEA 78,53")` adds token `"amazon"` under key `"aws"` (the date token `"14"` and number `"78"` are too short / would be filtered; `"emea"`, `"services"`, `"web"` are also stored). Persists across instances.

**When learning fires (UI):** every time a user confirms/links a match — single-invoice link (`matchInvoiceWithStatement`), bulk-confirm of ★ high-confidence suggestions, individual suggest confirm, grouped, and partial confirmations all call `Learn(supplier, chosenLine.Text)` then `Save()`. The supplier used is the invoice's `Auftraggeber` (counterparty). Aliases are loaded into every subsequent `matchConfig()` so a learned token can rescue a no-shared-word supplier on later runs.

---

### 10. Reconcile status & "open / missing" derivation

#### 10.1 ReconcileSummary

`ReconcileSummary(lines []LineRef, linked map[string]bool)` where `LineRef = {Key, Betrag, IstGutschrift}` and `linked[key]==true` means the line is claimed by some invoice:
- `LinesTotal` = len(lines).
- For each line: if `linked[Key]` → `LinesMatched++`; else `LinesOpen++` and add `Betrag` to **`OpenGutschrift`** if `IstGutschrift`, otherwise to **`OpenBelastung`**.

`ReconcileResult` = `{LinesTotal, LinesMatched, LinesOpen, OpenBelastung, OpenGutschrift}`.

Golden: lines `[100 debit linked, 50 debit open, 200 credit linked, 75 credit open]` → `LinesTotal=4, LinesMatched=2, LinesOpen=2, OpenBelastung=50.0, OpenGutschrift=75.0`. Empty input → all zeros.

#### 10.2 How "linked" and "open/missing" are derived (UI flow)

1. **Parse-once cache** per bank/credit-card account: parse every statement file once; cache lines tagged with their source filename. Only accounts of type **Bank** or **CreditCard** reconcile.
2. **linkedSet**: the set of line keys claimed by the year's invoices = `{ row.BuchungRef : row.BuchungRef != "" }`. (Line key = `BuchungRef{file,page,lineIdx}.String()`.)
3. For each account, build `LineRef` list from cached lines (key/Betrag/IstGutschrift), compute `ReconcileSummary(lines, linkedSet)`, and collect **open lines** = cached lines whose key is **not** in `linkedSet`. These open lines are the **"missing receipts"** display (statement lines with no matching invoice → a receipt is presumably missing). The status string shows `matched/total`, the open total `OpenBelastung+OpenGutschrift`, the most recent `ClosingBalance` (max across the account's metadata entries), and either a "complete" message (LinesOpen==0) or an "N open lines" message.

> Quirk: "missing receipts" is purely the complement of claimed lines — it is line-driven, not invoice-driven. An invoice with no statement line is **not** flagged here (only `MatchNone` in the suggestion list reflects that).

#### 10.3 No silent auto-linking; orchestration specifics

- Even `MatchAuto` results are routed into a **confirm list** (flagged "high-confidence ★"), never linked automatically. The user (or bulk-confirm-all-★) approves each. On confirm: set invoice `BuchungRef = {file,page,lineIdx}`, persist the invoice, `Learn`+`Save` the alias, mark the line `claimed`.
- **Cross-file ambiguity**: per invoice, each statement file is matched independently; if **2+ files** each produce a `MatchAuto` for the same invoice, the result is downgraded to `MatchSuggest` (never auto-link an across-files ambiguity). Suggest candidates from multiple files are **accumulated** (deduped by `{file,page,lineIdx}`) and re-sorted by score descending.
- **Greedy claiming**: a statement line is claimed at most once; auto-results sorted by top-candidate score descending get first pick.
- **Optional Claude re-ranking** (only when `ProcessingMode == "claude"` and an API key exists): for suggestions with ≥2 candidates whose **top-two scores differ by < 0.3**, ask the model to pick the best line by supplier name; on success move that pick to the front. Errors are non-fatal (heuristic order kept).
- Group detection runs once per account over still-unclaimed lines and still-unmatched invoices; partial detection runs per Teilzahlung invoice over unclaimed lines.

---

### Re-implementation checklist

- **Format auto-detect (substring, order-independent):** CAMT iff text contains both `"<Document"` and `"BkToCstmrStmt"`; MT940 iff text contains `":61:"`; else PDF.
- **CAMT.053:** namespace-agnostic by local element name; map `Amt`/`CdtDbtInd`(CRDT=credit)/`BookgDt>Dt` (fallback `ValDt>Dt`)/`AddtlNtryInf` (fallback joined `Ustrd`); date `YYYY-MM-DD`→`DD.MM.YYYY`; one booking per `<Ntry>`; LineIdx monotone from 1; Page 0.
- **MT940:** `:tag:` tokenizer (second colon within 1..5 chars; `-` ends block; continuation lines appended); `:61:` = `YYMMDD` + skip-digits + mark(`RC/RD/C/D`, C-prefixed=credit) + comma-amount (abs); date century hard-coded `20`; narrative from the *immediately following* `:86:` only, newlines→spaces.
- **PDF:** requires **positioned** text runs with absolute (top,left) pt; sort top-then-left; emit a booking only when the line **starts** with a `DD.MM.[YY[YY]]` date (skips mid-line dates like "Kontostand am …"); amount = **last** German money token (abs); credit detection via keyword set or trailing ` H`/`+`.
- **Qonto:** triggered when full text contains both `"Qonto"` and `"Abrechnungstag"`; year from `Vom DD/MM/YYYY`; skip header lines (`Kontostand|Eingänge|Ausgänge|Abrechnungstag|Kontoauszüge`); new tx on `^DD/MM`; ignore any `USD` line; first `±N EUR` sets amount/sign; emit only if an amount was captured.
- **Amount formats:** CAMT/Qonto-plain = dot-decimal; MT940 = comma-decimal; Qonto-German & PDF = `1.234,56`. `parseQontoAmount` branches on presence of comma. All Betrag stored **absolute (≥0)** except CAMT which relies on unsigned wire amounts.
- **Matcher:** target = `round2(Bruttobetrag_EUR + Gebuehr_EUR − Rabatt_EUR)` (Gebuehr never FX-divided); tol = 0.01 EUR, or `amount × ForeignTolerancePct/100` for non-EUR when larger; type gate on IstGutschrift==wantCredit; **Score = (1/(1+days))×2 + tokenOverlap**, alias overlap can replace name overlap if higher; sort by score desc.
- **Outcome:** exactly one candidate within `DateWindowDays` → Auto; else Suggest; no candidates → None. Defaults `DateWindowDays=5`, `ForeignTolerancePct=1.5`.
- **Tokenizer:** lowercase, split on non-`[a-z0-9]`/non-`U+00E4..U+00FF`, keep len≥3; overlap = bidirectional substring fraction; `dayDistance` returns 9999 on unparseable date; flex date fills missing year from the other date.
- **Grouped:** sizes 2 then 3 only; sum within 0.01; disjoint invoices; first-match-per-line wins; `File` filled by caller. **Partial:** only `Teilzahlung`; `0 < Betrag < target−0.01`; ranked by date proximity.
- **Alias learning:** key `lower(trim(supplier))`; learn tokens with len≥4, not pure digits, not in supplier-name tokens, deduped; learn+save on **every** user-confirmed link; load into all later match configs.
- **Status:** a line is "linked" iff its key is in `{ invoice.BuchungRef }`; open/missing = unclaimed lines; `OpenBelastung` (debits) vs `OpenGutschrift` (credits) split; closing balance = max `ClosingBalance` across the account's metadata entries.
- **Links are dual & must stay in sync:** invoice→line `BuchungRef` string `file|page|lineIdx` (authoritative) and line→invoice `InvoiceRef` mirror persisted in `metadata.json`; cache freshness keyed on PDF mtime; link preservation across re-parse keyed on `(Page, LineIdx)`. No silent auto-linking — all matches require confirmation.

---

## Auto-Booking Rules Engine

The auto-booking rules engine lets BuchISY post an incoming receipt to the books **silently, without opening the confirmation modal**, when a per-supplier rule says so and the receipt passes a plausibility gate. It is an explicit, opt-in convenience layer on top of the normal extract → review → save flow. The engine has four cooperating parts:

1. **Booking templates** (`booking_templates.json`) — a learned, per-supplier memory of *which expense account and which booking category* to use, plus an `Autobook` opt-in flag.
2. **Booking rules base** (`buchungsregeln.json`) — the chart-of-accounts-level configuration (Vorsteuer/Umsatzsteuer/Erlös accounts, keyword→account suggestions, category posting rules). This is shared with the manual booking engine; the auto-booker only consumes its category rules indirectly via the booking builder.
3. **The match + plausibility gate** (`MatchAutobookRule`, `AutobookPlausible`) — decides whether a given extracted receipt is eligible for silent booking.
4. **The batch driver** — runs eligible receipts through silent booking, counts them, and reports `"N auto-booked · M for review"`.

> Note: there is no rule list with arbitrary conditions. A "rule" is exactly one stored template per supplier name (`Auftraggeber`), keyed by the *exact* company name string. Matching is by company name only. Keyword matching exists separately and only as a *suggestion* aid for unknown suppliers (see §6); it never triggers auto-booking.

---

### 1. The booking template (per-supplier rule)

A booking template is the remembered booking pattern for one company. Fields:

| Field | Type | JSON key | Meaning |
|---|---|---|---|
| Kategorie | string | `kategorie` | Booking category key, e.g. `"standard"`, `"bewirtung"`. Selects which posting logic the booking builder applies. |
| ExpenseKonto | int | `expense_konto` | The expense (Gegenkonto) account, used for the `"standard"` category. |
| Autobook | bool | `autobook` | Opt-in flag. **Default false.** When false the field is *omitted* from JSON (`omitempty`). |

The store is a flat map `company name → template`, persisted to **`booking_templates.json`** in the profile's config directory, serialized as pretty-printed JSON with 2-space indentation. File mode `0644`. A missing file is **not** an error (empty store). A parse error on load **is** returned as an error (`"failed to parse booking templates: …"`).

Example file content:

```json
{
  "ACME GmbH": {
    "kategorie": "standard",
    "expense_konto": 4920,
    "autobook": true
  },
  "Matcha Rina": {
    "kategorie": "bewirtung",
    "expense_konto": 6640
  }
}
```

(In the second entry `autobook` is absent → `false`.)

Store operations:

- **Get(company)** → `(template, found)`. Exact map-key lookup on the full company name (case-sensitive, no trimming).
- **Set(company, template)** → stores in memory **and immediately writes the whole map to disk**. Every `Set` rewrites the entire file.
- **List()** → all `(company, template)` pairs **sorted ascending by company name** (insertion sort by `<` on the company string). Used to render the Auto-Booking Rules dialog.

> Quirk: company match is an exact string-equality lookup on `Auftraggeber`. `"ACME GmbH"` and `"ACME  GmbH"` (double space) or `"acme gmbh"` are *different* rules. There is no normalization, trimming, or case-folding in the template store (unlike the duplicate check, which does fold/trim — see §5).

#### How a template is learned

Templates are written automatically when a user saves an *incoming* invoice through the review modal via the auto-booking path:

- On a successful save, if `learn == true` and the company name is non-empty, the store records `{Kategorie: <selected category key>, ExpenseKonto: <selected account>}` for that company. The `Autobook` flag is **not** set by learning — it is always written as the zero value (`false`) here.
- `learn` is set to `true` only on the **expense** code path and only when the computed booking was *bookable*. It is **never** set for outgoing invoices (`Ausgangsrechnung`), and it is skipped when the user set a manual booking instead of the auto-computed one.

So: learning happens silently as a side-effect of normal manual bookings; it captures category + account but leaves auto-booking **off**. The user must explicitly turn `Autobook` on per supplier (see §7) before any silent booking can occur.

---

### 2. Matching a rule to an incoming receipt

`MatchAutobookRule(company, store)` → `(template, true)` **iff** both:

1. `store.Get(company)` finds a template for that exact company name, **and**
2. that template's `Autobook` flag is `true`.

Otherwise it returns `(zero template, false)`. So a rule that exists but has `Autobook=false` does **not** match; an unknown company does not match.

The only matching field is `Meta.Auftraggeber` (the supplier/company name). No amount, date, invoice-number, or VAT matching participates in rule selection.

---

### 3. The plausibility gate — `AutobookPlausible(meta)`

Before a matched rule is allowed to book silently, the extracted receipt must pass the plausibility gate. `AutobookPlausible(meta)` returns `true` **only if ALL** of the following hold (evaluated in this order; first failure returns `false`):

1. **At least one tax line:** `meta.TaxLines` is non-empty.
2. **Account set:** `meta.Gegenkonto > 0`.
3. **Positive gross:** `meta.Bruttobetrag > 0`.
4. **Gross reconciles to lines + tip within 2 cents:**
   `|Bruttobetrag − (SumNetto(TaxLines) + SumMwSt(TaxLines) + Trinkgeld)| ≤ 0.02`
   where `SumNetto` = Σ of each line's `Netto`, `SumMwSt` = Σ of each line's `MwStBetrag`. `Trinkgeld` (tip) is added untaxed.
5. **Foreign currency must have a valid rate:** NOT (`Waehrung != ""` AND `Waehrung != "EUR"` AND `Wechselkurs ≤ 0`). I.e. a non-EUR, non-empty currency with a missing/zero/negative exchange rate is rejected. An empty currency string or `"EUR"` is always accepted regardless of `Wechselkurs`.

The tolerance is an **inclusive ≤ 0.02** comparison (so a diff of *exactly* 0.02 passes; 0.03 fails). Currency code comparison is **case-sensitive** — `"EUR"` passes but `"eur"` or `"usd"` would be treated as foreign.

> Quirk: the currency check only fails when the rate is missing for a *foreign* currency. It does **not** verify that the gross was actually converted using that rate — only that a positive rate exists. The reconciliation in rule 4 is performed in the receipt's own currency units, not in EUR.

Worked examples (from the golden tests, base meta = one line `{Netto:100, MwSt:19, Satz:19}`, `Gegenkonto:4920`, `Brutto:119`, `Waehrung:"EUR"`):

| Scenario | Field changes | Expected = (Netto+MwSt+Tip) | Result |
|---|---|---|---|
| Clean EUR invoice | — | 119 | **true** |
| With tip | Brutto=124, Trinkgeld=5 | 119+5=124 | **true** |
| No tax lines | TaxLines=∅ | — | false (rule 1) |
| Gegenkonto=0 | Gegenkonto=0 | — | false (rule 2) |
| Zero gross | Brutto=0, line all-zero | — | false (rule 3) |
| Gross mismatch | Brutto=130 | 119, diff=11 | false (rule 4) |
| Borderline OK | Brutto=119.02 | 119, diff=0.02 | **true** (rule 4, inclusive) |
| Borderline fail | Brutto=119.03 | 119, diff=0.03 | false (rule 4) |
| Foreign, no rate | Waehrung="USD", Wechselkurs=0 | — | false (rule 5) |
| Foreign, with rate | Waehrung="USD", Wechselkurs=1.08 | 119 | **true** |

---

### 4. The silent-booking decision and template expansion

When an incoming file is processed (after extraction), the engine attempts silent auto-booking when **all** of these hold, in order:

1. `MatchAutobookRule(meta.Auftraggeber, store)` returns a matched template (rule exists **and** its `Autobook=true`), **and**
2. `AutobookPlausible(meta)` is `true`, **and**
3. a quick **duplicate pre-check** finds no existing invoice (see §5).

If all three hold, `autoBookInvoice` runs and, on success, increments the batch counter `batchAutoBooked`, reloads the invoice list, and returns *without ever showing the modal*. If `autoBookInvoice` returns an error, the failure is logged and the flow **falls through to the normal confirmation modal** (so the receipt is never lost). If the duplicate pre-check reports a duplicate, silent booking is **skipped** and the modal opens (the modal then does the full duplicate handling).

#### What `autoBookInvoice` does (template → booking expansion)

1. **Target year/month:** parse `meta.Rechnungsdatum` as `DD.MM.YYYY` (split on `"."`). If it splits into 3 parts: year = part[2] (used if > 0), month = part[1] (used if 1..12). Otherwise fall back to the currently-viewed year/month.
2. **Next Belegnummer:** read-only peek of the next sequential receipt number for `<targetYear>` (format `YYYY-NNNN`). On error, Belegnummer is left empty (booking still proceeds).
3. **Resolve expense account:** `account = tpl.ExpenseKonto`; if that is `0`, fall back to `settings.DefaultAccount`.
4. **Bank/payment account:** `bankAccount = settings.DefaultBankAccount`.
5. **Build the double-entry booking** via the shared booking builder using `tpl.Kategorie`, `meta.TaxLines`, `meta.Trinkgeld`, `account`, the bank account, and `meta.Rabatt`. (The category's posting logic — standard/bewirtung/reverse_charge/geschenke/reisekosten/kfz — is the same as for manual bookings; see the Booking Engine chapter.)
6. **Build the filename** from the naming template using a copy of `meta` with `Belegnummer = nextBelegnr` and `Jahr/Monat` taken from the parsed date. If the template errors or yields an empty/whitespace filename, `autoBookInvoice` **errors** (→ fall through to modal).
7. **Save** the invoice with the computed booking. Two flags are **hard-coded false** for silent booking: `partialPayment = false` and `rememberMapping = false` (so silent booking does **not** re-learn/overwrite the template), and `ausgangsrechnung = false` (silent auto-booking only applies to incoming invoices).

> Quirk: silent booking always uses `settings.DefaultBankAccount` and `settings.DefaultAccount` (when the template has no account). It never asks. If the default bank account cannot resolve an SKR04 payment account, the booking builder returns an error and the flow falls back to the modal rather than booking wrongly.

For the `"standard"` category the expense entry posts `round2(netTotal − rabatt)` on the Soll (debit) side, where `netTotal = round2(SumNetto + Trinkgeld)`; Vorsteuer is added per rate from the rules base; the payment account balances on the Haben (credit) side as the sum of all Soll entries.

---

### 5. Duplicate pre-check (gate before silent booking)

Before `autoBookInvoice` runs, a quick duplicate check is performed with a minimal row containing only `Auftraggeber` and `Rechnungsnummer`. `FindDuplicate` logic:

- If `Rechnungsnummer` is blank/whitespace → **not** a duplicate (returns `false`); silent booking proceeds.
- Otherwise it queries existing invoices for a row where
  `LOWER(TRIM(auftraggeber)) = LOWER(TRIM(?))` **AND** `rechnungsnummer = ?` (exact, case-sensitive on the number) **AND** `rechnungsnummer <> ''`, limit 1.
- If found → duplicate; silent booking is skipped and the modal opens. The label returned is the existing `belegnummer` (or `dateiname` if the Belegnummer is empty/null), but the auto-booker only uses the boolean.

> Quirk: the duplicate check folds case and trims whitespace on the *company name* but compares the *invoice number* exactly. The template match (§2) does neither — so a receipt can match an Autobook rule under one casing yet be deduped against an existing invoice stored under a different casing of the same company.

---

### 6. Booking rules base (`buchungsregeln.json`) and keyword suggestion

This is the chart-level rules base, shared with the manual booking engine. The auto-booker uses it only via the booking builder (category posting logic + Vorsteuer accounts). Two parts are worth documenting here because they belong to the same subsystem.

**File:** `buchungsregeln.json` in the profile config dir, pretty JSON (2-space indent), mode `0644`. Loaded by a store that **merges** a saved per-profile file over the **bundled defaults**:

- If the profile file is present and parses: every value from the **saved** file wins, but anything the **bundled** base adds that the saved file lacks is merged in — specifically: Vorsteuer rates (`vorsteuer_konten`), Umsatzsteuer rates (`umsatzsteuer_konten`), Erlös accounts (`erloes_konten`), and category rules (`regeln`) whose `kategorie` key is not already present. Existing categories are **not** duplicated.
- If the profile file is **corrupt/unparseable**: silently fall back to the bundled defaults (a corrupt file must not break all bookings — no error surfaced).
- If no profile file: bundled defaults.

Bundled defaults (current `assets/buchungsregeln.json`):

```
vorsteuer_konten   : { "19": 1406, "7": 1401 }
umsatzsteuer_konten: { "19": 3806, "7": 3801 }
erloes_konten      : { "inland": 8400, "eu": 8341, "drittland": 8200 }
regeln:
  standard       — "Standard-Aufwand"
  bewirtung      — "Bewirtung (§ 4 Abs. 5 EStG)"  abziehbar_prozent=70, konto_abziehbar=6640, konto_nicht_abziehbar=6644
  reverse_charge — "Reverse-Charge (§ 13b UStG)"  rc_satz=19, konto_vst_rc=1407, konto_ust_rc=3837
  geschenke      — "Geschenke"  schwelle=35, konto_abziehbar=6610, konto_nicht_abziehbar=6620
  reisekosten    — "Reisekosten"  default_konto=6650
  kfz            — "Kfz-Kosten"  default_konto=6520
```

Lookups:

- **Rule(kategorie)** — linear scan, **case-sensitive exact** match on the `kategorie` key.
- **VorsteuerKonto(satzPercent)** / **UmsatzsteuerKonto(satzPercent)** — the percent is converted to an integer key via `int(satzPercent + 0.5)` (round-half-up), then looked up as a string. So `19.0 → "19" → 1406`; `7.0 → "7" → 1401`; `0 → "0"` → not found `(0,false)`.
- **ErloesKonto(vatID, mwst)** — for outgoing invoices: `mwst > 0.005` → key `"inland"`; else if `IsEUVatID(vatID)` → key `"eu"` (§18b); else → key `"drittland"`. (Used by revenue bookings, not the auto-booker.)

**Keyword suggestion — `SuggestKonto(text)`** (optional `konto_stichwoerter` map, keyword → account):

- Returns `(0,false)` if the map is empty or `text` is blank/whitespace.
- Lowercases `text`; for each keyword, lowercases+trims it (skipping empty), and tests **case-insensitive substring containment** in the text.
- **Longest matching keyword wins** (most specific), measured by the trimmed-lowercased keyword length. Ties keep the first-found longest.
- Returns `(account, true)` for the winning keyword, else `(0,false)`.

Golden examples (map `{tankstelle:4663, aral:4663, hotel:4660, telekom:4920}`):

| text | result |
|---|---|
| `"ARAL Tankstelle München"` | `(4663, true)` — both `aral` and `tankstelle` match |
| `"Best Western Hotel"` | `(4660, true)` |
| `"Deutsche Telekom GmbH"` | `(4920, true)` |
| `"Unbekannter Lieferant XY"` | `(0, false)` |
| `""` | `(0, false)` |

Longest-match: with `{bahn:4671, "deutsche bahn":4670}`, `"Fahrkarte Deutsche Bahn AG"` → `4670` (the 13-char `"deutsche bahn"` beats the 4-char `"bahn"`).

> Note: `SuggestKonto` only *proposes* a Gegenkonto for unknown suppliers in the modal (populating `Meta.KontoVorschlaege`, a transient field). It is **not** part of the silent auto-booking path and never sets `Autobook`.

---

### 7. The Auto-Booking Rules dialog (opt-in management UI)

Reached from the menu ("Auto-Booking Rules" / "Auto-Buchungs-Regeln"). It lists every learned template via `store.List()` (sorted by supplier name) in four columns: **Supplier, Account, Category, Auto-Book**. The Auto-Book column is a checkbox bound to the template's `Autobook` flag; toggling it immediately `Set`s (persists) the template with the new flag. The Account column shows the chart label for `ExpenseKonto` if found, else the raw account number.

A bold warning banner is always shown:
EN: *"⚠ Enabled rules book receipts WITHOUT review — only enable for suppliers that consistently deliver reliable data."*
DE: *"⚠ Aktivierte Regeln buchen Belege OHNE Prüfung — nur aktivieren, wenn der Lieferant verlässlich bekannte Daten liefert."*

When no templates exist yet, the dialog shows the warning plus an explanatory message that rules are learned automatically when booking through the modal.

---

### 8. The batch driver and result toast ("N gebucht / M zur Prüfung")

Files dropped/selected are filtered to supported types and queued. The driver tracks three counters on the app:

- `batchTotal` — total files in the current batch.
- `batchDone` — files popped/started (used for the modal title `… (done/total)` when total > 1).
- `batchAutoBooked` — files silently auto-booked this batch.

If a batch is already in flight, new files are **appended** to the running queue and `batchTotal` is increased (the running batch is not replaced). Files are processed **sequentially**: each file is extracted, then either silently auto-booked (incrementing `batchAutoBooked`) or routed to the modal; closing the modal (save or cancel) advances to the next file.

When the queue empties and `batchTotal > 0`:

- `autoN = batchAutoBooked`
- `reviewN = batchTotal − autoN`
- If `autoN > 0`, show a toast formatted from the `autobook.result` template:
  - EN: `"%d auto-booked · %d for review"` → e.g. `"3 auto-booked · 2 for review"`
  - DE: `"%d automatisch gebucht · %d zur Prüfung"` → e.g. `"3 automatisch gebucht · 2 zur Prüfung"`
- All three counters reset to 0.

> Quirk: the toast appears only when `autoN > 0`. If nothing was auto-booked, **no** summary toast is shown even though `reviewN` files went through review. `reviewN` counts every non-auto-booked file in the batch — including files the user *cancelled* in the modal, not just files actually saved for review.

---

### 9. End-to-end worked example

Setup: store has a rule `"ACME GmbH" → {Kategorie:"standard", ExpenseKonto:4920, Autobook:true}`. Settings: `DefaultBankAccount` resolves to SKR04 `1800` (bank). Bundled rules: Vorsteuer 19% → `1406`.

A batch of 2 files is dropped:

- **File A** — extracted `Auftraggeber="ACME GmbH"`, one line `{Netto:100, MwSt:19, Satz:19}`, `Brutto:119`, `Waehrung:"EUR"`, `Gegenkonto:4920`, `Rechnungsnummer:"R-001"`, `Rechnungsdatum:"15.03.2026"`.
  1. `MatchAutobookRule` → matched (rule exists, Autobook on).
  2. `AutobookPlausible` → true (line present, Gegenkonto>0, Brutto 119 = 100+19+0).
  3. Duplicate pre-check on `("ACME GmbH","R-001")` → none.
  4. `autoBookInvoice`: targetYear=2026, targetMonth=3 (from `15.03.2026`); account=4920; booking built for `"standard"`:
     - Soll 4920 (expense): `round2(100 − 0) = 100.00`
     - Soll 1406 (Vorsteuer 19%): `19.00`
     - Haben 1800 (bank): `round2(100 + 19) = 119.00`
     - → saved silently. `batchAutoBooked = 1`.
- **File B** — extracted `Auftraggeber="Unknown Café"` (no rule). → not matched → opens the modal for manual review.

When the queue empties: `batchTotal=2`, `autoN=1`, `reviewN=1`. Toast: `"1 automatisch gebucht · 1 zur Prüfung"` (DE) / `"1 auto-booked · 1 for review"` (EN).

---

### Re-implementation checklist

Must-match behaviors for this subsystem:

- **Rule = exact-company-name template.** Key on full `Auftraggeber` string, case-sensitive, no trimming. Storage file `booking_templates.json`, flat `{company: {kategorie, expense_konto, autobook}}`, pretty JSON 2-space, mode 0644; missing file ok, parse error fatal. `autobook` omitted when false.
- **Default Autobook = OFF.** Learning records `{kategorie, expense_konto}` only, never sets `autobook`. The flag is set only by the user in the rules dialog.
- **Match requires rule exists AND `autobook==true`.** A disabled or absent rule never matches.
- **Plausibility gate, all of:** ≥1 tax line; `Gegenkonto>0`; `Bruttobetrag>0`; `|Brutto − (ΣNetto + ΣMwSt + Trinkgeld)| ≤ 0.02` (inclusive 2-cent tolerance, in receipt currency); NOT (non-empty, non-`"EUR"` currency with `Wechselkurs ≤ 0`). Currency comparison case-sensitive.
- **Silent-booking precondition is match AND plausible AND not a duplicate**, evaluated in that order; any failure routes to the modal (never drops the file).
- **Duplicate pre-check:** blank invoice number ⇒ not duplicate; else match on `LOWER(TRIM(auftraggeber))` + exact non-empty `rechnungsnummer`.
- **autoBookInvoice details:** date parsed `DD.MM.YYYY`; year used if >0, month if 1..12, else current view; expense account = `tpl.ExpenseKonto` or `DefaultAccount` if 0; bank = `DefaultBankAccount`; `partialPayment`, `rememberMapping`, `ausgangsrechnung` all forced false; empty/error filename ⇒ error ⇒ modal fallback.
- **Booking expansion** uses the same category posting logic as manual bookings (standard subtracts `rabatt` on the expense leg; Vorsteuer per rate via round-half-up percent→account lookup; payment leg balances as Σ Soll).
- **Rules base merge:** saved values win; bundled additions (Vorsteuer/Umsatzsteuer/Erlös rates and new category keys) are merged in without duplicating existing categories; corrupt profile file silently falls back to bundled.
- **Keyword suggestion (`SuggestKonto`)** is a separate, suggestion-only feature: case-insensitive substring, longest keyword wins; never triggers auto-booking.
- **Batch result:** `reviewN = batchTotal − batchAutoBooked`; toast shown only when `batchAutoBooked > 0`; format `"%d automatisch gebucht · %d zur Prüfung"` (DE) / `"%d auto-booked · %d for review"` (EN). `reviewN` includes cancelled-in-modal files. Counters reset after the toast.

---

## Exports, GoBD Compliance & Filename/Storage Rules

This chapter specifies every artefact BuchISY produces for external consumption (CSV journals, DATEV/Lexware bookkeeping imports, the GoBD audit-export ZIP, backups, the Verfahrensdokumentation PDF) and every internal mechanism that makes the data GoBD-defensible (audit log, period locking / Festschreibung, gap-free receipt numbers, deduplication). All formats are IST-exact: column orders, separators, encodings, rounding, date masks and the golden numbers from the test suite are reproduced verbatim. A re-implementation on any stack must match these byte-for-byte where noted.

### Common conventions

- **Date display format** everywhere user-facing and inside CSV/manifest is `DD.MM.YYYY` (e.g. `18.06.2026`). The DB stores it as this same string. `Jahr` is `YYYYYY` (4-digit string), `Monat` is `MM` (2-digit string).
- **Two amount-decimal conventions coexist** and must not be confused:
  - The **invoices.csv** money columns use the *configured decimal separator* — default comma `,`. (Quirk: the documentation calls it "always `.`", but the live default is `,`; see §1.)
  - The **DATEV** and **Lexware** booking files always use a **comma** decimal regardless of settings.
- **Rounding** of all money is half-away-from-zero to 2 decimals (`round2(v) = round(v*100)/100`). Exchange rates and fee percentages are stored to **4** decimals.
- **EUR is the booking currency.** Foreign-currency rows are converted to EUR before any booking export or CSV export (§10).

---

### 1. CSV journal `invoices.csv`

One `invoices.csv` lives per month folder (`<StorageRoot>/<YYYY>/<YYYY-MM>/invoices.csv`). It is regenerated from the SQLite database (the DB is the leading system; CSV is a redundant GoBD backup format). It is also read on legacy import.

#### 1.1 Column order (exact, in order)

```
Belegnummer, Dateiname, Rechnungsdatum, Jahr, Monat, Auftraggeber,
Verwendungszweck, Rechnungsnummer, VATID, BetragNetto, Steuersatz_Prozent,
Steuersatz_Betrag, Bruttobetrag, Waehrung, Gegenkonto, Bankkonto, Bezahldatum,
Teilzahlung, Ausgangsrechnung, Kommentar, BewirtungAnlass, BewirtungTeilnehmer,
BetragNetto_EUR, Gebuehr, Rabatt, Wechselkurs, GebuehrProzent, HatAnhaenge,
AnzahlAnhaenge, Unterordner, BuchungRef, Trinkgeld, Steuerzeilen, Buchung,
Exportiert, Originalwaehrung, Originalbetrag_Brutto
```

37 columns. The header row uses these exact IDs (not the German display names). A user-configured `column_order` may reorder columns, but any column from the default set that is missing from a saved order is **appended** in default order, so newer columns always appear even on legacy orders.

#### 1.2 Quoting, separators, encoding

- **Every field** (header included) is wrapped in double-quotes. Embedded `"` is escaped by doubling (`"` → `""`).
- **Field separator**: configurable (`CSVSeparator`): `,` (default), `;`, or tab (`\t` / literal `\t`). Anything else falls back to `,`.
- **Line terminator on write**: `\n` (LF only). (Note: DATEV/Lexware use CRLF; only invoices.csv uses LF.)
- **Encoding**: configurable (`CSVEncoding`): default `ISO-8859-1` (Latin-1); alternative `UTF-8`. On ISO-8859-1, content is transcoded to Latin-1 on write and decoded from Latin-1 on read.
- **Decimal separator in money columns**: configured `DecimalSeparator`, default `,`. Money columns are formatted `%.2f` then `.`→sep. `Wechselkurs` and `GebuehrProzent` are formatted `%.4f` then `.`→sep.
- **Bool columns** (`Teilzahlung`, `Ausgangsrechnung`, `HatAnhaenge`, `Exportiert`) are the literal strings `true` / `false`.
- **Int columns**: `Gegenkonto`, `AnzahlAnhaenge` are plain integers.
- **Steuerzeilen** and **Buchung** columns hold embedded JSON (see §1.4).

#### 1.3 Reading rules & backward compatibility

The reader is lenient (`LazyQuotes` on). It builds a header→index map from the first row, recognising only known column names. Recognised legacy aliases when the canonical column is empty/absent:

- `Firmenname` → read into `Auftraggeber` (when `Auftraggeber` empty).
- `Kurzbezeichnung` → read into `Verwendungszweck` (when empty).
- `UStIdNr` → read into `VATID` (when `VATID` empty).
- When a parsed header contains **zero** recognised columns, the reader assumes the **default column order** positionally and treats the first row as data (no header skip).
- Float parsing accepts both `,` and `.` (first `,`→`.`).
- **Backfills on read:**
  - If `HatAnhaenge` is false but `AnzahlAnhaenge > 0`, set `HatAnhaenge = true`.
  - If `Steuerzeilen` is empty, reconstruct one tax line from the aggregate fields (`BetragNetto`, `Steuersatz_Prozent`, `Steuersatz_Betrag`); if those are all zero but `Bruttobetrag > 0`, emit a single line with `Netto = Bruttobetrag` (so gross-only legacy rows keep their total).
  - `Exportiert` parsed case-insensitively as `== "true"`.

On `Append`, if the on-disk header does not exactly match the current expected header, the whole file is reloaded and rewritten in the current order before appending.

#### 1.4 Embedded JSON columns

- **Steuerzeilen** = JSON array of `{ "netto":<float>, "satz_prozent":<float>, "mwst_betrag":<float> }`. Empty array → `""`.
- **Buchung** = JSON `{ "entries":[{ "konto":<int>, "betrag":<float>, "soll":<bool>, "steuerschluessel":"<opt>" }], "info":"<opt>", "manuell":<opt bool> }`. Empty booking → `""`.

---

### 2. DATEV-EXTF Buchungsstapel

Produced by `BuildDATEVStapel(header, rows)`. Output is built as a UTF-8 string with **CRLF** line endings, then (in the UI export path) **re-encoded to Windows-1252** before writing to disk (falls back to UTF-8 if encoding fails). File name: `DATEV-EXTF_<period>.csv` (period forms in §2.5).

#### 2.1 Header line (line 1) — exact template

```
"EXTF";700;21;"Buchungsstapel";13;<ErzeugtAm>;;;;;"<BeraterNr>";"<MandantNr>";<WJBeginn>;4;<DatumVon>;<DatumBis>;"";"";"";"";0;"EUR";"";"";"";""
```

- `EXTF` format marker; version `700`; category `21` (Buchungsstapel); format name `Buchungsstapel`; format version `13`.
- `ErzeugtAm` = `YYYYMMDDHHMMSSmmm` (17 chars; UI passes `time.Now()` as `YYYYMMDDHHMMSS` + `"000"`).
- `BeraterNr`, `MandantNr` (DATEV consultant/client numbers) — empty strings allowed.
- `WJBeginn` = fiscal-year start `YYYYMMDD`.
- Fixed `4` (Sachkontenlänge), then `DatumVon`/`DatumBis` = `YYYYMMDD`.
- Default currency `EUR`.

#### 2.2 Column header line (line 2) — exact, semicolon-separated, **unquoted**

```
Umsatz (ohne Soll/Haben-Kz);Soll/Haben-Kennzeichen;WKZ Umsatz;Kurs;Basis-Umsatz;WKZ Basis-Umsatz;Konto;Gegenkonto (ohne BU-Schlüssel);BU-Schlüssel;Belegdatum;Belegfeld 1;Belegfeld 2;Skonto;Buchungstext
```

#### 2.3 Data row layout (per booking counter-entry), in order

Format string: `%s;"%s";"EUR";;;;%d;%d;;%s;"%s";"%s";;"%s"`

| # | Field | Source / rule |
|---|-------|---------------|
| 1 | Umsatz | `datevAmount(entry.Betrag)` = `%.2f` with `.`→`,`, unsigned |
| 2 | Soll/Haben-Kz | `"S"` if entry.Soll else `"H"` (quoted) |
| 3 | WKZ Umsatz | `"EUR"` |
| 4 | Kurs | empty |
| 5 | Basis-Umsatz | empty |
| 6 | WKZ Basis-Umsatz | empty |
| 7 | Konto | `entry.Konto` (the counter account) |
| 8 | Gegenkonto | `base.Konto` (the payment/base account) |
| 9 | BU-Schlüssel | empty |
| 10 | Belegdatum | `DDMM` (day+month of `Rechnungsdatum`, no year) |
| 11 | Belegfeld 1 | `Belegnummer`, else `Rechnungsnummer`; cleaned, max 36 runes (quoted) |
| 12 | Belegfeld 2 | `Rechnungsnummer`; cleaned, max 36 runes (quoted) |
| 13 | Skonto | empty |
| 14 | Buchungstext | `trim(Auftraggeber + " " + Verwendungszweck)`; cleaned, max 60 runes (quoted) |

#### 2.4 Booking split, `datevClean`, Belegfeld logic

- **Which rows export:** a row contributes rows only if its booking is `Balanced()` (≥1 entry, `|ΣSoll − ΣHaben| < 0.005`) **and** `PaymentAndCounters(isRevenue)` returns ok. `isRevenue = row.Ausgangsrechnung`. The **base** = the single entry on the base side (Haben for expense, Soll for revenue); **counters** = all other entries. Ok requires exactly one base entry and ≥1 counter. Otherwise the whole invoice is **skipped** (counted in `skipped`). The base account is never its own data row; it only appears as Gegenkonto (field 8).
- One output line is written **per counter entry**; `exported` counts output lines.
- **`datevClean(s, max)`**: remove all `"`; replace `\r` and `\n` with a space; truncate to `max` **runes** (UTF-8 safe — never splits a multibyte char).
- **Belegfeld 1/2 split:** Belegfeld 1 is the internal sequential receipt number (primary DATEV sort/find key); if `Belegnummer` is empty (pre-Belegnummer rows) it falls back to `Rechnungsnummer`. Belegfeld 2 always carries the supplier `Rechnungsnummer`.

#### 2.5 Period string forms (file-name suffix)

- Current month: `YYYY-MM` (e.g. `2026-06`).
- Whole year: `YYYY` (e.g. `2026`). DatumVon = `YYYY0101`, DatumBis = `YYYY1231`.
- Date range: `YYYY-MM_bis_YYYY-MM`.
- `DatumVon` = 1st of from-month; `DatumBis` = real last day of to-month (28/29/30/31 handled).

#### 2.6 Worked examples (golden)

Expense, Bewirtung mixed split, paid from 1800; `Rechnungsdatum 18.06.2026`, `Rechnungsnummer MC9C7PFZ-103052`, no Belegnummer, Auftraggeber "Matcha Rina". Entries: 6640/12.71 S, 6644/5.44 S, 1406/1.26 S, 1401/0.59 S, 1800/20.00 H. Base = 1800 (single Haben). Four data lines result; the 6640 line:

```
12,71;"S";"EUR";;;;6640;1800;;1806;"MC9C7PFZ-103052";"MC9C7PFZ-103052";;"Matcha Rina"
```

(`exported=4, skipped=1` — a row with no booking is skipped.)

With a Belegnummer (`2026-0014`), Belegfeld 1/2 split is visible:

```
…;1755;;0606;"2026-0014";"MC9C7PFZ-103052";;"Matcha Rina"
```

Revenue (`Ausgangsrechnung=true`), Belegnummer `2025-0002`, `Rechnungsdatum 10.12.2025`. Entries: 1200/7735 S (base), 8400/6500 H, 1776/1235 H → two lines:

```
6500,00;"H";"EUR";;;;8400;1200;;1012;"2025-0002";"";;"Symeo"
1235,00;"H";"EUR";;;;1776;1200;;1012;"2025-0002";"";;"Symeo"
```

(`exported=2, skipped=0`.)

> Quirk: when `Belegnummer` is set and `Rechnungsnummer` empty, Belegfeld 1 = Belegnummer and Belegfeld 2 = `""` (both fall back to the same single source only when Belegnummer is absent).

---

### 3. Lexware import CSV

Produced by `BuildLexwareCSV(rows)`. UTF-8, **CRLF** line endings, **semicolon**-separated, **no quoting**. File name `Lexware-Buchungen_<period>.csv`.

Header (line 1):

```
Datum;Belegnr;Buchungstext;Betrag;Sollkonto;Habenkonto
```

Row layout (one per counter entry), format `%s;%s;%s;%s;%d;%d`:

| Field | Rule |
|-------|------|
| Datum | `Rechnungsdatum` as-is (`DD.MM.YYYY`) |
| Belegnr | `Belegnummer` else `Rechnungsnummer`; `lexClean`ed |
| Buchungstext | `trim(Auftraggeber + " " + Verwendungszweck)`; `lexClean`ed |
| Betrag | `%.2f` with `.`→`,` (unsigned) |
| Sollkonto | if entry.Soll: `entry.Konto`; else `base.Konto` |
| Habenkonto | if entry.Soll: `base.Konto`; else `entry.Konto` |

`lexClean(s)`: replace `;`→`,`, `\r`/`\n`→space. Same Balanced/PaymentAndCounters skip rule as DATEV; same per-counter expansion. Lexware orients Soll/Haben per entry side (DATEV puts the entry account in `Konto` and base in `Gegenkonto`).

Golden examples:

```
18.06.2026;R-1;Matcha Rina Bewirtung;12,71;6640;1800
06.06.2026;2026-0014;Matcha Rina Bewirtung;12,71;4650;1755
10.12.2025;2025-0002;Symeo;6500,00;1200;8400    (revenue: Soll=base 1200, Haben=8400)
```

---

### 4. GoBD / DATEV-Belegpaket ZIP

Produced by `BuildExportPackage(rows, datevCSV, belege, period)`. Built with stdlib zip + XML. The UI builds it for the **whole current year** (months 1–12, period `YYYY`) and saves it as `GoBD-Export_<period>.zip`. Contents:

| ZIP entry | Content |
|-----------|---------|
| `DATEV-EXTF_<period>.csv` | the DATEV EXTF Buchungsstapel bytes (§2) — note: NOT re-encoded to Win-1252 inside the ZIP; the raw `BuildDATEVStapel` UTF-8 bytes are stored |
| `belege/<sanitized>.pdf` | one entry per `BelegFile` (the original receipt) |
| `manifest.csv` | receipt-to-booking index |
| `index.xml` | GoBD-oriented (DTD-uncertified) data-set description |

#### 4.1 Beleg file naming (`belegZipName`)

Base = `Belegnummer` if non-empty, else `Dateiname`. Strip a trailing `.pdf`, run `SanitizeFilename` (§8), then if empty → `"beleg"`, then re-append `.pdf`. So `2026-0001` → `belege/2026-0001.pdf`. Rows whose source PDF cannot be read are skipped (logged) and not added to `belege`.

#### 4.2 `manifest.csv`

Semicolon-separated; **LF** line endings; **conditional** quoting. Header:

```
Belegnummer;Dateiname;Auftraggeber;Rechnungsdatum;Bruttobetrag;Gegenkonto
```

Per row, in order: `Belegnummer`, `Dateiname`, `Auftraggeber`, `Rechnungsdatum`, `Bruttobetrag` (`%.2f`, dot decimal), `Gegenkonto` (`%d`). A field is quoted **only if** it contains `;`, `"`, `\n` or `\r`; inside a quoted field `"`→`""`.

Example data row: `2026-0001;2026-01-15_Acme_GmbH_R001_EUR_119.00.pdf;Acme GmbH;15.01.2026;119.00;4400`

#### 4.3 `index.xml` (GDPdU/GoBD-oriented)

XML declaration header + indented (`  `) marshalled tree. Structure:

```
<DataSet Version="1.0">
  <Media>
    <Name>GoBD-Export <period></Name>
    <Table>
      <URL>manifest.csv</URL>
      <Name>Belegmanifest</Name>
      <Description>Zuordnung Belegnummer zu Belegdatei und Buchungsdaten (GoBD-orientiert)</Description>
      <VariableLength>
        <ColumnDelimiter>;</ColumnDelimiter>
        <TextEncapsulator>"</TextEncapsulator>
        <Column><Name>Belegnummer</Name><DataType>Alphanumerisch</DataType></Column>
        <Column><Name>Dateiname</Name><DataType>Alphanumerisch</DataType></Column>
        <Column><Name>Auftraggeber</Name><DataType>Alphanumerisch</DataType></Column>
        <Column><Name>Rechnungsdatum</Name><DataType>Alphanumerisch</DataType></Column>
        <Column><Name>Bruttobetrag</Name><DataType>Numerisch</DataType></Column>
        <Column><Name>Gegenkonto</Name><DataType>Numerisch</DataType></Column>
      </VariableLength>
    </Table>
    <Table>
      <URL><datevFileName></URL>
      <Name>DATEV-EXTF-Buchungsstapel</Name>
      <Description>DATEV EXTF Buchungsstapel fuer Zeitraum <period></Description>
      <VariableLength>
        <ColumnDelimiter>;</ColumnDelimiter>
        <TextEncapsulator>"</TextEncapsulator>
        <Column><Name>Umsatz</Name><DataType>Numerisch</DataType></Column>
        <Column><Name>Soll-Haben-Kennzeichen</Name><DataType>Alphanumerisch</DataType></Column>
        <Column><Name>Konto</Name><DataType>Numerisch</DataType></Column>
        <Column><Name>Gegenkonto</Name><DataType>Numerisch</DataType></Column>
        <Column><Name>Belegdatum</Name><DataType>Alphanumerisch</DataType></Column>
        <Column><Name>Belegnummer</Name><DataType>Alphanumerisch</DataType></Column>
        <Column><Name>Buchungstext</Name><DataType>Alphanumerisch</DataType></Column>
      </VariableLength>
    </Table>
  </Media>
</DataSet>
```

> Quirk: the index.xml column list for the DATEV table is a simplified subset (7 columns) and does **not** mirror the real 14-column EXTF layout — it is documentation-grade, not a re-import schema. Re-implementers should keep it as-is.

---

### 5. Backup ZIP

Produced by `WriteBackupZip(writer, files)` where `files` is a map of `zipEntryName → sourcePath`. Sources that cannot be opened are **silently skipped**; the function returns the count of files actually written. Map iteration order is unspecified (entry order in the ZIP is non-deterministic). Default file name `BuchISY-Backup.zip`. The UI assembles `files` as:

- `invoices.db` ← the global SQLite DB.
- `config/settings.json`, `config/chart_skr04.json`, `config/buchungsregeln.json`, `config/booking_templates.json`, `config/company_accounts.json` ← the profile config dir.
- `csv/<relpath>` ← **every** `invoices.csv` found under the storage root, keyed by its slash-normalised path relative to the root.

---

### 6. GoBD mechanisms

#### 6.1 Audit log (`audit_log` table)

Schema: `id` (autoinc), `ts` (`DATETIME DEFAULT CURRENT_TIMESTAMP`), `aktion`, `entitaet`, `schluessel`, `details`. `AuditLog(limit)` returns newest-first (`ORDER BY ts DESC, id DESC LIMIT ?`). Logging is **best-effort**: a failure logs a warning and never aborts the underlying operation.

Entries written:

| Trigger | aktion | entitaet | schluessel | details |
|---------|--------|----------|------------|---------|
| Invoice insert | `create` | `invoice` | `<Belegnummer> <Dateiname>` (space-joined; leading space if no Belegnummer) | `""` |
| Invoice update | `update` | `invoice` | `<Belegnummer> <oldDateiname>` | JSON field diff (see below); `"{}"` if before-image unavailable |
| Invoice delete | `delete` | `invoice` | `<Dateiname>` (no Belegnummer) | `""` |
| Lock period | `lock` | `period` | `<jahr>-<monat>` (e.g. `2026-06`) | `""` |
| Unlock period | `unlock` | `period` | `<jahr>-<monat>` | `""` |

**Update diff (`DiffFields`)**: JSON object of only the changed fields, each `{"alt":<old>,"neu":<new>}`. Compared fields, in this order: `Auftraggeber, Rechnungsnummer, Rechnungsdatum, BetragNetto, SteuersatzBetrag, Bruttobetrag, Gegenkonto, Bankkonto, Bezahldatum, BuchungRef, Belegnummer, Ausgangsrechnung`. Equality compared via string formatting (`%v`). No changes → `"{}"`.

> Quirk: only those 12 fields are diffed; changes to e.g. `Verwendungszweck`, `Waehrung`, tax lines, or the booking itself are **not** recorded in `details`.

#### 6.2 Period locking / Festschreibung (`period_locks` table)

Schema: `(jahr TEXT, monat TEXT, locked_at, PRIMARY KEY(jahr,monat))`.

- `LockPeriod(jahr, monat)`: `INSERT OR IGNORE` (idempotent) + audit `lock`.
- `UnlockPeriod(jahr, monat)`: `DELETE` + audit `unlock`.
- `IsPeriodLocked(jahr, monat)`: count > 0.
- `LockedPeriods()`: all locks as `"YYYY-MM"`, ordered by jahr, monat.

**What a lock blocks** (returns error `ErrPeriodLocked` = "Periode ist festgeschrieben"):

- `Insert` into a locked `(jahr, monat)`.
- `Delete` from a locked `(jahr, monat)`.
- `Update` where the **old** `(jahr, monat)` is locked.
- **Cross-month move:** `Update` where the row's **new** `(row.Jahr, row.Monat)` differs from old and the **new** period is locked — blocked too. So moving an invoice *out of* or *into* a locked month both fail.

Locking is per-month and reversible only by explicit unlock (both audited). After Festschreibung the documented policy (Verfahrensdokumentation §6) is that only Stornobuchungen (reversing entries via a new booking in an open period) are permitted; direct edits to locked records are refused at the repository layer.

> Quirk — the "Storno guard": there is **no dedicated Storno code path**. The guard is purely the `ErrPeriodLocked` rejection above. A reversal is achieved by booking a counter-entry in an *open* period; the app does not auto-generate storno rows. A re-implementer must replicate the rejection, not invent a storno feature.

The UI tracks `currentMonthLocked` (refreshed in `loadInvoices`) to disable edit/delete affordances and shows a "locked" indicator; whole-year view forces the flag false.

#### 6.3 Gap-free Belegnummer assignment (`NextBelegnummer`)

Format `YYYY-NNNN` (year + 4-digit zero-padded sequence). Per **database (= profile)** and per **year**. Algorithm:

1. `SELECT MAX(belegnummer) FROM invoices WHERE belegnummer LIKE '<jahr>-%'`.
2. If found, parse the integer suffix after the first `-`; else `n = 0`.
3. Return `sprintf("%s-%04d", jahr, n+1)`.

Keys purely on the `YYYY-` prefix of stored numbers, **not** the `jahr` column. The value is *read, not reserved* — cancelling a dialog leaves no gap, and the same number is handed out until a row actually persists it. Lexical MAX equals numeric MAX because of zero-padding (valid up to 9999/year).

Golden: empty DB → `2026-0001`; after inserting `2026-0001` → next `2026-0002`; year 2025 is independent (`2025-0001`); inserting a row whose Belegnummer is `2026-0009` (even filed under `jahr=2025`) advances the 2026 sequence → next `2026-0010`.

#### 6.4 Renumbering (`RenumberBelegnummern`)

One-shot SQL rewrite: for every invoice, assign `printf('%s-%04d', jahr, ROW_NUMBER() OVER (PARTITION BY jahr ORDER BY <sortkey>, id))`. The chronological sort key is the date reassembled to sortable `YYYYMMDD` from `Rechnungsdatum` (`substr(...,7,4)||substr(...,4,2)||substr(...,1,2)`), ties broken by `id`. Returns the total invoice count. Backfills empty numbers **and** closes gaps left by deletions; overwrites existing numbers (even "wrong" ones).

> Quirk: partitions by the `jahr` **column**, while `NextBelegnummer` keys on the **prefix**. After a renumber, a row with `jahr=2025` but a `2026-` number from a prior insert gets re-partitioned under 2025 — the two functions use different keys.

Golden: rows (Mar 2026 numbered `2026-0099`, Jan 2026 unnumbered, Dec 2025 unnumbered) → after renumber: Jan→`2026-0001`, Mar→`2026-0002` (stale 0099 replaced), Dec 2025→`2025-0001`; count returned = 3. After renumber the UI re-exports the current month's CSV.

#### 6.5 Export-eligibility classification (`ClassifyForExport`)

Partitions a period's rows into `Exportable`, `AlreadyExported`, `Skipped{Dateiname, Grund}`:

- Not balanced or `PaymentAndCounters` not ok → Skipped, reason `"nicht ausgeglichen"`, or `"keine Buchung"` if there are zero entries.
- Else if `Exportiert` already true → AlreadyExported (and added to Exportable **only** if `includeExported`).
- Else → Exportable.

After a successful booking export the UI calls `MarkExported(jahr, monat, dateiname)` per exported row (`exportiert = 1`). Note: any **Update** to a row resets `exportiert = 0` (the SQL hard-sets it), so an edited invoice becomes re-exportable.

---

### 7. Dedupe algorithm (`IsDuplicate`)

Code-based, **no DB unique constraint**. Two flavours:

**Core `IsDuplicate(existingRows, newRow)`** — true if ANY existing row matches ALL of:

- `NormalizeCompanyName(Auftraggeber)` equal (lowercase, trim, collapse spaces, strip one trailing legal suffix from: ` gmbh, ag, kg, ohg, gbr, ug, e.k., ltd, inc, corp`),
- `Rechnungsnummer` exactly equal,
- `Rechnungsdatum` exactly equal,
- `Bruttobetrag` approximately equal (`|a−b| < 0.01`),
- `Teilzahlung` equal.

**DB `Repository.IsDuplicate(jahr, monat, row)`** — same fields scoped to one month: `LOWER(TRIM(auftraggeber))` equal, `rechnungsnummer` equal, `rechnungsdatum` equal, `ABS(bruttobetrag - ?) < 0.01`, `teilzahlung` equal. (Used during CSV→DB import to skip dupes.)

**`FindDuplicate(row)`** (early cross-month signal): blank `Rechnungsnummer` → not found. Else first row across all months with matching `LOWER(TRIM(auftraggeber))` and identical non-empty `rechnungsnummer`; returns the existing `Belegnummer` (or `Dateiname` fallback) as a label.

> Quirk: company normalization strips only ONE suffix and only the listed ones; "GmbH & Co. KG" is not specially handled. Float tolerance differs subtly: core uses `< 0.01`, the DB uses `< 0.01` as well, both exclusive.

---

### 8. Filename template & sanitization

#### 8.1 Tokens (`ApplyTemplate`)

German aliases are replaced first (verbatim string replace), then canonical tokens. **Case-sensitive.**

German alias → canonical:

| Alias | Canonical |
|-------|-----------|
| `${Firma}` | `${Company}` |
| `${Belegnummer}` | `${Belegnr}` |
| `${Rechnungsnummer}` | `${InvoiceNumber}` |
| `${Kurzbez}` | `${Kurzbezeichnung}` |
| `${BetragNetto}` | `${NetAmount}` |
| `${SteuersatzProzent}` | `${TaxPercent}` |
| `${Steuerbetrag}` | `${TaxAmount}` |
| `${Bruttobetrag}` | `${GrossAmount}` |
| `${Waehrung}` | `${Currency}` |
| `${Jahr}` | `${YYYY}` |
| `${Monat}` | `${MM}` |

Canonical tokens → value:

| Token | Value |
|-------|-------|
| `${Belegnr}` | `Belegnummer` |
| `${YYYY}` | `Jahr` |
| `${MM}` | `Monat` |
| `${DD}` | day = first part of `Rechnungsdatum` split on `.` (empty if not 3 parts) |
| `${Company}` | `Auftraggeber` |
| `${InvoiceNumber}` | `Rechnungsnummer` |
| `${Kurzbezeichnung}` | `Verwendungszweck` (legacy name) |
| `${Verwendungszweck}` | `Verwendungszweck` |
| `${Kurzbez8}` | first 8 runes of `Verwendungszweck` |
| `${NetAmount}` | `FormatAmount(BetragNetto, sep)` |
| `${TaxPercent}` | `FormatAmount(SteuersatzProzent, sep)` |
| `${TaxAmount}` | `FormatAmount(SteuersatzBetrag, sep)` |
| `${GrossAmount}` | `FormatAmount(Bruttobetrag, sep)` |
| `${Currency}` | `Waehrung` |
| `${OriginalName}` | `""` (filled by caller if needed) |

Amounts via `FormatAmount(value, decimalSep)`: 2 decimals, given decimal sep (`,` or `.`), with a thousands separator (whichever of `.`/`,` is not the decimal); negative sign kept in front. Default naming template:

```
${YYYY}-${MM}-${DD}_${Company}_${Kurzbez8}_${InvoiceNumber}_${Currency}_${GrossAmount}.pdf
```

After substitution the result is run through `SanitizeFilename`. Golden: `${Kurzbez8}` of "Software Projekt Entwicklung" → `Software`; `${Belegnr}_${Company}` and `${Belegnummer}_${Company}` both → `2026-0014_Matcha Rina`.

> Quirk: unknown tokens like `${YYYY-MM-DD}` are **not** recognised and pass through as literal text (`${YYYY-MM-DD}` appears verbatim in the output). `${OriginalName}` always resolves to empty unless the caller pre-substitutes.

#### 8.2 `SanitizeFilename`

In order: `/`→`-`; `\`→`-`; **remove** any of `< > : " | ? *` and control chars (`\x00`–`\x1f`) via regex `[<>:"|?*\x00-\x1f]`; collapse runs of whitespace to a single space; trim ends. **Spaces and commas are preserved**; umlauts preserved. Golden: `2026-05-21_Foo_EUR_15,23.pdf` unchanged; `a<b>c:d"e|f?g*h.pdf` → `abcdefgh.pdf`.

`NormalizeVerwendungszweck` (applied to purposes only, not company names): replace `&` (with surrounding whitespace) by ` und ` and trim. Golden: `A&B`→`A und B`; `A&B&C`→`A und B und C`; `& Anfang`→`und Anfang`; `Ende &`→`Ende und`.

#### 8.3 Collision handling (`prepareTarget`)

When the target file already exists, append `_2`, `_3`, … **before the extension** until free: `base.pdf` → `base_2.pdf` → `base_3.pdf`. The counter starts at 2. Used by `MoveAndRename` and `CopyAndRename`.

#### 8.4 Storage layout & attachments

- Month folder: `<StorageRoot>/<YYYY>/<YYYY-MM>` when `UseMonthSubfolders` (default true); else the bare storage root.
- Invoice main file: `<monthFolder>/<Unterordner?>/<Dateiname>`; category subfolders e.g. `Bar/` for cash.
- Attachments are **numbered siblings** next to the main file: `<base>_Anhang<N>.<ext>` (1-based), where `<base>` = main name without extension. Parsed/sorted by index; there is no separate attachments folder. Renaming the invoice renames attachments to keep the new base + same `_Anhang<N>.<ext>` suffix.
- Move/copy fall back to read+write copy if `os.Rename` fails (cross-device or locked source); a locked source that cannot be removed after copy still counts as success.

---

### 9. Verfahrensdokumentation PDF

`BuildVerfahrensdokumentationPDF(settings, chartAccounts, profilName, datum)` → PDF bytes (A4 portrait, Arial, cp1252 translator so umlauts/€ render; "Seite X / N" footer; sub-header `<profil> · Erstellt am <today DD.MM.YYYY>`). Title "Verfahrensdokumentation BuchISY". Ten numbered sections (`N. Heading` bold + wrapped body). Dynamic values:

- `modus` = "KI-Extraktion via Claude (Anthropic API)" if `ProcessingMode == "claude"`, else "Lokale Mustererkennung (offline)".
- Empty `profilName`/`datum`/`StorageRoot`/`NamingTemplate` rendered as `-`.
- `chartAccounts` integer interpolated into §5.

Sections: (1) Allgemeines (program, profile, GoBD reference BMF 28.11.2019), (2) Belegerfassung (extraction mode, channels, E-Rechnung XRechnung/ZUGFeRD, OCR/Vision), (3) Belegfluss & Ablage (StorageRoot, `YYYY-MM` month folders, naming template, `<Dateiname>-files/` for attachments — note this text says `-files/`, the actual code uses `_Anhang<N>` siblings; the PDF text is descriptive, not authoritative), (4) Belegnummernkreis (`YYYY-NNNN` gap-free), (5) Buchung (Soll/Haben, chartAccounts count, tax keys 19%/7%/steuerfrei/§13b, DATEV+Lexware export), (6) Unveränderbarkeit & Festschreibung (month lock; audit trail; "nach Festschreibung nur Stornobuchungen"), (7) Aufbewahrung (10 years §147 AO; SQLite `invoices.db` leading, CSV redundant), (8) Datensicherung (backup ZIP), (9) GoBD-Datenexport (EXTF + Belegbilder + index.xml; §147 Abs. 6 AO), (10) Verantwortlichkeiten (blank signature lines + the document date). Output must begin with `%PDF` and be non-trivial in size.

---

### 10. Currency normalization & EUR conversion

#### 10.1 Currency catalogue (`currencies.go`)

~160 ISO-4217 entries `{Code, Name}`. Dropdown order: top four `EUR, USD, CAD, AUD` first (in that order), then the rest sorted by code. Option string format `"<CODE> — <Name>"` (em-dash with surrounding spaces). `CurrencyCodeFromOption` parses the code before `" — "`; unknown codes pass through unchanged. There is **no `€`→`EUR` symbol mapping in this layer** — the catalogue is code-based; symbol normalization happens upstream in extraction (out of scope here). The stored `Waehrung` is always a 3-letter code (or empty = treated as EUR).

#### 10.2 Foreign → EUR row conversion (`RowEUR` / `RowsEUR`)

A row is "foreign" iff `Waehrung` is non-empty and ≠ `EUR`. For a foreign row with a valid `Wechselkurs > 0` (rate is **foreign units per 1 EUR**):

- `BetragNetto`, `SteuersatzBetrag`, `Bruttobetrag`, `Trinkgeld`, `Rabatt` each = `round2(value / kurs)`.
- `TaxLines` converted via `TaxLinesEUR`: each line's `Netto` and `MwStBetrag` = `round2(/kurs)`; `SatzProzent` unchanged.
- `BetragNetto_EUR` set equal to the converted net.
- **`Gebuehr` (bank/CC FX fee) is left unchanged** — it is already booked in EUR (NOT divided).
- `Waehrung` → `EUR`, `Wechselkurs` → `0`.

EUR rows or blank-currency rows: returned unchanged, `rateMissing=false`. Foreign row with `Wechselkurs ≤ 0`: returned unchanged at face value, `rateMissing=true`.

Golden (USD, kurs 1.1720): net 168.09→`round2(168.09/1.1720)=143.42`; gross 200.00→`170.65`; Trinkgeld 1.17→`1.00`; Rabatt 5.86→`5.00`; Gebuehr 2.34→**2.34 (unchanged)**.

#### 10.3 CSV export normalization (`ExportToCSV`)

Before writing `invoices.csv`, for each foreign row the **documentation columns** are stamped from the *original* values (BEFORE conversion):

- `Originalwaehrung` = original `Waehrung`.
- `Originalbetrag_Brutto` = original `Bruttobetrag`.

Then `RowsEUR` converts all money to EUR. So the on-disk CSV carries EUR amounts in the primary columns plus the original currency/gross preserved in the two documentation columns. Rows with a missing rate pass through unconverted (and still get documentation columns stamped, since the stamp only checks `Waehrung != EUR`).

#### 10.4 Payment-conversion helper (`ConvertForeignPayment`)

For a foreign **payment** with a CC fee: `kurs ≤ 0` → all-zero (no divide-by-zero). Else `brutto = round2(bruttoForeign/kurs)`, `netto = round2(nettoForeign/kurs)`, `gebuehr = round2(brutto * gebuehrProzent/100)`, `gesamt = round2(brutto + gebuehr)`. Golden: USD 89.18 gross at 1.1583, 2% fee → BruttoEUR 76.99, GebuehrEUR 1.54, GesamtEUR 78.53.

---

### 11. Advisory warnings (non-blocking, `warnings.go`)

These never block save/export; they surface plausibility hints (German strings). Conditions (as-of a reference date):

- Gross mismatch: `Bruttobetrag > 0` and `|Brutto − (Netto + Steuerbetrag + Trinkgeld)| > 0.02` → "Brutto stimmt nicht mit Netto + MwSt + Trinkgeld überein".
- `Gegenkonto == 0` → "Kein Gegenkonto gewählt".
- Foreign currency with `Wechselkurs ≤ 0` → "Fremdwährung ohne Wechselkurs".
- Outgoing + 0 tax + blank `VATID` → ZM-gap warning.
- Future `Rechnungsdatum` (parsed `02.01.2006`, after today) → "...liegt in der Zukunft".
- `Bruttobetrag ≤ 0` → "Bruttobetrag fehlt oder ist 0".
- GWG accounts **4855** (SKR03) / **6260** (SKR04) with `BetragNetto > 800.0` → "kein GWG ... abschreiben (AfA)". (Threshold exactly `> 800` €.)
- Incoming + 0 tax + gross > 0 + no §13b account (1577/1787/1407/3837) in booking + foreign signal (non-EUR currency OR a non-DE EU VAT-ID) → §13b reverse-charge hint.
- Bewirtung deductible account (4650/6640) present without the matching non-deductible (4654/6644) → "Bewirtung ohne 70/30-Aufteilung".
- VAT-ID format: after uppercasing and removing spaces, must match `^[A-Z]{2}[0-9A-Za-z]{6,14}$`, else "USt-IdNr hat ungültiges Format".

#### Config hints (`MissingConfigHints`)

Returns i18n keys: `hint.no_api_key` when `ProcessingMode == "claude"` and no stored API key; `hint.no_storage` when `StorageRoot` is blank/whitespace.

---

### Re-implementation checklist

- **invoices.csv**: emit the 37 columns in the exact order in §1.1; quote every field (double-quote, `""` escaping); default encoding ISO-8859-1, default field sep `,`, default decimal sep `,`, LF line endings; `Wechselkurs`/`GebuehrProzent` to 4 decimals, money to 2; embed Steuerzeilen/Buchung as the documented JSON. On read, support `Firmenname`/`Kurzbezeichnung`/`UStIdNr` aliases, header-less positional fallback, attachment/tax-line backfills.
- **DATEV EXTF**: reproduce the header line and the 14-field column line verbatim; CRLF; per-counter rows with `Konto=entry`, `Gegenkonto=base`; comma decimals; Belegdatum=`DDMM`; Belegfeld 1 = Belegnummer-or-Rechnungsnummer, Belegfeld 2 = Rechnungsnummer; `datevClean` (strip `"`, CR/LF→space, rune-truncate to 36/36/60); skip unbalanced/invalid bookings; re-encode file to Windows-1252 on disk.
- **Lexware CSV**: header `Datum;Belegnr;Buchungstext;Betrag;Sollkonto;Habenkonto`; semicolons, no quotes, CRLF; entry-oriented Soll/Haben; `;`→`,` cleaning.
- **GoBD ZIP**: entries `DATEV-EXTF_<period>.csv`, `belege/<sanitized>.pdf` (Belegnummer-based, `.pdf` re-appended), `manifest.csv` (6 columns, conditional quoting, LF), and the GDPdU-style `index.xml` (XML decl + the exact DataSet/Media/Table tree); skip unreadable belege.
- **Backup ZIP**: `invoices.db`, `config/*.json` (5 named files), `csv/<relpath>` for every `invoices.csv` under the root; skip unreadable sources; count written.
- **Audit log**: write create/update/delete/lock/unlock with the exact aktion/entitaet/schluessel/details rules; update-diff covers only the 12 listed fields; best-effort (never abort the op).
- **Festschreibung**: month-scoped locks block Insert/Delete on the locked month and Update when old OR new period is locked (cross-month moves blocked both directions); error message "Periode ist festgeschrieben"; reversal only via new booking in an open period (no auto-storno).
- **Belegnummer**: `YYYY-NNNN` per profile+year, keyed on the `YYYY-` prefix of MAX, read-not-reserved; renumber partitions by `jahr` column chronologically (date→`YYYYMMDD`, tie by id), gap-free, overwrites.
- **Dedupe**: code-based match on normalized Auftraggeber + Rechnungsnummer + Rechnungsdatum + Bruttobetrag (`<0.01`) + Teilzahlung; no DB constraint; one stripped legal suffix in normalization.
- **Filename template**: alias-then-canonical replacement (case-sensitive), the full token table, `FormatAmount` with thousands grouping, then `SanitizeFilename` (remove `<>:"|?*` + control chars; keep spaces/commas/umlauts), then `_2/_3` collision suffixing before the extension.
- **Currency/EUR**: 3-letter codes; foreign→EUR divides net/tax/gross/Trinkgeld/Rabatt/tax-lines by `Wechselkurs` (round2) but leaves `Gebuehr` untouched; CSV export stamps `Originalwaehrung`/`Originalbetrag_Brutto` before conversion; missing rate → face value passthrough.
