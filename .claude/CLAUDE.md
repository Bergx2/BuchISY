# BuchISY - Project Architecture

## What is BuchISY?

BuchISY is a **GoBD-compliant German bookkeeping pre-system** (Buchhaltungs-Vorsystem) desktop app (macOS/Windows) for small businesses and freelancers. It started as an invoice organizer (extract PDF metadata via **Claude AI** or **local heuristics**, file into month folders, SQLite + CSV) and has grown into a full bookkeeping suite: double-entry booking with VAT logic against an SKR04 chart, German VAT filings (UStVA/ZM), reports (SuSa/GuV/OPOS), DATEV/Lexware/GoBD exports, bank-statement import, and the GoBD trio of audit trail, period locking, and Verfahrensdokumentation.

It is deliberately a **Vorsystem**: capture, book, lock, file VAT, and export a clean DATEV/receipt package for the tax advisor. It does **not** do a full annual close (HGB-Bilanz/Jahresabschluss) — SuSa/GuV are evaluations, not a Jahresabschluss.

**Published by:** Bergx2 GmbH
**Website:** [www.buchisy.de](https://www.buchisy.de)
**Current Version:** v2.16

> **Note on this document:** the architecture below was written for an earlier "v2.3 invoice organizer" and still accurately describes the extraction/storage core. The accounting modules (booking, UStVA/ZM, SuSa/GuV/OPOS, Anlagen, Kassenbuch, bank import, audit, Festschreibung, exports) are summarized in **Core Features** and **Bookkeeping modules** below; many individual `internal/core/` and `internal/ui/` files for those features are not yet enumerated in the per-file lists.

## Core Features

**Capture & extraction**
- **Dual extraction modes:** Claude API (AI-powered) or local pattern matching
- **Vision extraction:** Claude Vision API for scanned PDFs (platform-specific rendering)
- **E-Invoice support:** XRechnung and ZUGFeRD (CII) structured data extraction
- **Multiple ingest paths:** drag-and-drop, clipboard paste, batch picker, watched scan inbox
- **PDFs and images:** receipts can be PDF or image (JPG/PNG/GIF/WebP); document preview pane with zoom/pan/highlights
- **Smart organization:** Automatic file naming and month-based folder structure (YYYY-MM)

**Bookkeeping & tax**
- **Double-entry booking:** multiple VAT lines, SKR04 chart, §13b reverse-charge, auto-booking rules engine (opt-in)
- **VAT filings:** UStVA (official ELSTER Kennzahlen, PDF + XML), ZM (PDF + XML)
- **Reports:** SuSa, GuV, OPOS (aging), Controlling, year overview
- **Fixed assets (Anlagen):** linear AfA, Anlagenspiegel; **Kassenbuch:** per-Barkasse cash book
- **Bank import:** CAMT.053 + MT940 + PDF/Qonto statements; Konten view + Belegabgleich/Erlös-Abgleich
- **Reports/PDF:** Belegliste, Rechnungsausgangsbuch, Buchungsjournal, Kassenbericht, cash-year overview, Verfahrensdokumentation

**GoBD & data**
- **SQLite database:** Single source of truth (tables: `invoices`, `audit_log`, `period_locks`)
- **Audit trail** (Änderungsprotokoll), **period locking** (Festschreibung), gap-free Belegnummern
- **Multi-profile (Mandanten):** per-profile chart + booking rules under `profiles/<name>/`
- **Exports:** auto CSV, DATEV-EXTF, Lexware, GoBD/DATEV-Belegpaket ZIP, Verfahrensdokumentation
- **Multi-language:** German (primary) and English with i18n support
- **Privacy-first:** API keys stored in OS keychain, local processing available

## Architecture Overview

### Tech Stack

- **Language:** Go 1.25+
- **UI Framework:** [Fyne](https://fyne.io/) (cross-platform native GUI)
- **Database:** [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (Pure Go SQLite)
- **PDF Parsing:** [ledongthuc/pdf](https://github.com/ledongthuc/pdf)
- **PDF Rendering:** [go-fitz](https://github.com/gen2brain/go-fitz) (MuPDF) + external commands for ARM64
- **E-Invoice:** [pdfcpu](https://pdfcpu.io/) for XML extraction
- **AI Integration:** Anthropic Claude API (Messages API + Vision API)
- **Secure Storage:** [go-keyring](https://github.com/zalando/go-keyring) (macOS Keychain/Windows Credential Manager)
- **i18n:** [go-i18n/v2](https://github.com/nicksnyder/go-i18n)

### Project Structure

```
buchisy/
├── cmd/buchisy/              # Application entry point
│   └── main.go              # Initializes app, handles asset paths
├── internal/
│   ├── ui/                  # Fyne-based UI layer
│   │   ├── app.go          # Main app initialization, window management
│   │   ├── invoicemodal.go # Invoice confirmation dialog (new/edit)
│   │   ├── tableedit.go    # Edit dialog for existing invoices
│   │   ├── tabledelete.go  # Delete confirmation and cleanup
│   │   ├── settings.go     # Settings dialog
│   │   ├── table.go        # Invoice table display
│   │   ├── filepicker.go   # PDF file selection logic
│   │   ├── custompicker.go # Custom file picker with search
│   │   ├── dialogs.go      # Helper dialogs (error, info, date picker)
│   │   ├── openfolder.go   # OS folder opening utilities
│   │   └── views.go        # UI layout builders
│   ├── core/               # Business logic (domain layer)
│   │   ├── types.go        # Domain models (Meta, Settings, CSVRow)
│   │   ├── storage.go      # File system operations
│   │   ├── csvrepo.go      # CSV read/write with custom column order
│   │   ├── localextract.go # Heuristic-based extraction
│   │   ├── pdftext.go      # PDF text extraction
│   │   ├── pdfimage.go     # PDF to image conversion (platform-specific)
│   │   ├── einvoice.go     # E-invoice (XRechnung/ZUGFeRD) extraction
│   │   ├── template.go     # Filename template engine
│   │   ├── settings.go     # Settings persistence
│   │   ├── companymap.go   # Company→Account mapping
│   │   ├── dedupe.go       # Duplicate detection
│   │   └── sanitize.go     # Filename sanitization
│   ├── db/                 # SQLite database layer (v2.0+)
│   │   ├── repository.go   # Database operations (CRUD)
│   │   ├── schema.go       # Database schema and indexes
│   │   ├── migration.go    # Schema migrations
│   │   ├── export.go       # CSV export from database
│   │   ├── maintenance.go  # Wipe/delete DB, re-book foreign→EUR (backup is in core/)
│   │   └── paths.go        # Database path utilities
│   ├── anthropic/          # Claude API integration
│   │   ├── extractor.go    # Metadata extraction via Claude (text + vision)
│   │   └── client.go       # HTTP client for Messages API
│   ├── i18n/               # Internationalization
│   │   ├── i18n.go         # Translation loading (German/English)
│   │   └── embed.go        # Embedded translation files
│   └── logging/            # Structured logging
│       └── logger.go       # File-based logger with levels
├── assets/
│   ├── i18n/               # Translation files (de.json, en.json)
│   ├── icon.png            # App icon
│   └── embed.go            # Asset embedding
├── .github/                # GitHub configuration
├── CODE_OF_CONDUCT.md      # Contributor Covenant
├── CONTRIBUTING.md         # Contributing guidelines
├── LICENSE                 # MIT License
├── go.mod                  # Go dependencies
├── Makefile                # Build automation
└── README.md               # User documentation
```

## Key Components

### 1. UI Layer (`internal/ui/`)

**Responsibility:** User interaction, event handling, dialog management

- **App (`app.go`)**: Main application controller
  - Manages Fyne window and app lifecycle
  - Coordinates UI components and business logic
  - Handles month/year selection state
  - Saves/restores window dimensions
  - Orchestrates PDF processing (text, e-invoice, vision)

- **InvoiceTable (`table.go`)**: Displays monthly invoices
  - Custom Fyne table with sortable columns
  - Loads data from SQLite database (v2.0+)
  - Supports delete, edit, duplicate operations
  - Configurable column order

- **InvoiceModal (`invoicemodal.go`)**: Confirmation dialog for new invoices
  - Optimized header: Original file + New filename preview (v2.1)
  - Shows all extracted fields for review
  - Real-time filename preview updates (v2.1)
  - Account selection with remembered mappings
  - Attachment file selection
  - Currency conversion fields (for non-EUR invoices)
  - Validates and sanitizes input before saving

- **TableEdit (`tableedit.go`)**: Edit dialog for existing invoices
  - Same optimized header as new invoice modal (v2.1)
  - Updates database and renames file if needed
  - Handles attachment additions
  - CSV auto-regenerated after updates

- **TableDelete (`tabledelete.go`)**: Delete confirmation
  - Removes from SQLite database
  - Deletes physical file
  - Deletes attachment folder if exists
  - Regenerates CSV

- **CustomPicker (`custompicker.go`)**: Advanced file picker (v2.0)
  - Search functionality
  - Sortable columns (filename, date)
  - Quick folder shortcuts (Desktop, Documents, Downloads)
  - Optimized column widths (880px + 80px, v2.1)

- **Settings (`settings.go`)**: Application configuration
  - Storage paths, filename templates, currency
  - Processing mode (Claude vs Local)
  - API key management (keyring integration)
  - Account definitions, language selection
  - CSV format configuration

- **Dialogs (`dialogs.go`)**: Reusable dialog helpers
  - Error dialogs
  - Info dialogs
  - Date picker with calendar widget

### 2. Core Layer (`internal/core/`)

**Responsibility:** Business logic, data models, file operations

- **Types (`types.go`)**: Domain models
  - `Meta`: Invoice metadata structure (20+ fields)
  - `Settings`: Application settings with JSON persistence
  - `CSVRow`: CSV record structure
  - `Account`, `BankAccount`: User-defined accounts

- **StorageManager (`storage.go`)**: File system operations
  - Creates month folders (YYYY-MM)
  - Handles file moving/renaming with collision detection
  - Manages attachment folders (`filename-files/`)
  - Scans for all CSV files in storage root

- **CSVRepository (`csvrepo.go`)**: CSV operations
  - Reads/writes invoices.csv with configurable column order
  - Handles decimal formatting (always `.` in CSV)
  - Validates required columns
  - Backward compatible with old column names (Firmenname/Kurzbezeichnung)

- **LocalExtractor (`localextract.go`)**: Heuristic-based extraction
  - Pattern matching for German invoices
  - Regex-based field extraction (amounts, dates, invoice numbers)
  - Returns confidence score

- **PDFTextExtractor (`pdftext.go`)**: Text extraction
  - Uses ledongthuc/pdf library
  - Extracts embedded text from PDF
  - Returns raw text for processing

- **PDFImageConverter (`pdfimage.go`)**: PDF to image conversion (v2.1)
  - Platform-specific rendering to avoid ARM64 crashes
  - **macOS ARM64**: External commands (sips → convert → gs)
  - **Windows/Linux/macOS Intel**: go-fitz (MuPDF) at 144 DPI
  - Returns base64-encoded PNG for Claude Vision API

- **EInvoiceExtractor (`einvoice.go`)**: Structured e-invoice extraction (v2.0)
  - Detects XRechnung and ZUGFeRD formats
  - Extracts XML data from PDF
  - Parses structured invoice data
  - Higher priority than text extraction

- **TemplateEngine (`template.go`)**: Filename generation
  - Token replacement: `${Company}`, `${InvoiceNumber}`, `${YYYY-MM-DD}`, etc.
  - Supports German aliases: `${Firma}`, `${Rechnungsnummer}`
  - Decimal separator configuration (`,` or `.`)

- **SettingsManager (`settings.go`)**: Persists settings to JSON
  - Location: `~/Library/Application Support/BuchISY/settings.json` (macOS)

- **CompanyAccountMap (`companymap.go`)**: Remembers company→account assignments
  - Location: `~/Library/Application Support/BuchISY/company_accounts.json`

- **Deduplication (`dedupe.go`)**: Prevents duplicate invoice entries
  - Checks invoice number, date, and amount in database

- **Sanitize (`sanitize.go`)**: Filename sanitization
  - Removes invalid characters for cross-platform compatibility

### 3. Database Layer (`internal/db/`) - v2.0+

**Responsibility:** SQLite database operations, persistence layer

- **Repository (`repository.go`)**: Main database interface
  - CRUD operations (Create, Read, Update, Delete)
  - List invoices by month
  - Duplicate detection
  - Transaction support

- **Schema (`schema.go`)**: Database schema definition
  - `invoices` table with all metadata fields
  - Indexes for fast queries (Jahr+Monat)
  - Timestamps (created_at, updated_at)

- **Migration (`migration.go`)**: Schema migrations
  - Creates initial schema
  - Handles version upgrades
  - Ensures backward compatibility

- **Export (`export.go`)**: CSV export from database
  - Generates `invoices.csv` from SQLite data
  - Respects column order and formatting settings
  - Called automatically after every change

- **Maintenance (`maintenance.go`)**: Database maintenance
  - `WipeDatabase` (delete all rows)
  - `DeleteDatabase` (remove the DB file)
  - `RebookForeignToEUR` (idempotent re-booking of foreign-currency receipts to EUR)
  - (Backup is NOT here — it lives in `core/backup.go` → `WriteBackupZip`, surfaced via `ui/backup.go`)

- **Paths (`paths.go`)**: Database path utilities
  - Global database path: `~/Library/Application Support/BuchISY/invoices.db`

### 4. Anthropic Integration (`internal/anthropic/`)

**Responsibility:** Claude API communication for AI extraction

- **Extractor (`extractor.go`)**: Metadata extraction
  - **Text extraction**: Uses Claude with structured JSON output
  - **Vision extraction**: Sends base64 PNG to Claude Vision API (v2.0)
  - System prompt enforces strict JSON schema (German field names)
  - Preprocesses text to prioritize invoice-relevant content (10k char limit)
  - Returns `Meta` with 0.9 confidence

- **Client (`client.go`)**: HTTP client for Claude Messages API
  - Sends text/image + system prompt to Claude
  - Parses streaming/non-streaming responses
  - Error handling for API failures

### 5. i18n (`internal/i18n/`)

**Responsibility:** Localization support

- Loads `.json` translation files from `assets/i18n/` (`de.json`, `en.json`)
- Supports German (de) and English (en)
- Fallback to German if translations missing
- Embedded in binary via `embed.go`

### 6. Logging (`internal/logging/`)

**Responsibility:** Structured file-based logging

- Log levels: DEBUG, INFO, WARN, ERROR
- Location: `~/Library/Application Support/BuchISY/logs/` (macOS)
- Log rotation by date
- Thread-safe

## Data Flow

### Invoice Processing Flow (v2.1)

1. **User selects PDF** → `ui/custompicker.go` (custom picker with search)
2. **Determine PDF type:**
   - Check for **E-Invoice** (XRechnung/ZUGFeRD) → `core/einvoice.go`
   - If E-Invoice: Extract structured XML data → Skip to step 5
3. **Extract text from PDF** → `core/pdftext.go` (uses ledongthuc/pdf)
4. **Check if text exists:**
   - **If text found:** Extract metadata based on mode
     - **Claude mode:** → `anthropic/extractor.go` → Claude API → JSON response
     - **Local mode:** → `core/localextract.go` → Pattern matching
   - **If no text (scanned PDF) AND Claude mode:**
     - Convert to image → `core/pdfimage.go` (platform-specific)
     - Extract via Vision → `anthropic/extractor.go` → Claude Vision API
5. **Suggest account** → `core/companymap.go` (remembers past assignments)
6. **Show confirmation modal** → `ui/invoicemodal.go` (optimized header, v2.1)
7. **User confirms/edits** → Real-time filename preview updates
8. **Check for duplicates** → `db/repository.go` (SQL-based)
9. **Generate filename** → `core/template.go`
10. **Move file** → `core/storage.go` (handles collisions)
11. **Copy attachments** → `core/storage.go` (if any selected)
12. **Insert to database** → `db/repository.go`
13. **Export to CSV** → `db/export.go` (auto-generated)
14. **Remember mapping** → `core/companymap.go` (if enabled)
15. **Reload table** → `ui/table.go` (loads from database)

### Platform-Specific PDF Rendering (v2.1)

**Vision extraction for scanned PDFs:**

| Platform | Method | Tool |
|----------|--------|------|
| macOS ARM64 | External commands | `sips` → `convert` → `gs` (fallback chain) |
| macOS Intel | go-fitz (MuPDF) | Native library at 144 DPI |
| Windows | go-fitz (MuPDF) | Native library at 144 DPI |
| Linux | go-fitz (MuPDF) | Native library at 144 DPI |

**Why different for ARM64?**
- go-fitz has signal handling issues on macOS ARM64 (crashes with "signal 16 not on stack")
- External commands avoid this by running in separate process
- sips is built-in to macOS, always available

### Database Architecture (v2.0+)

**Single Global Database:**
- Location: `~/Library/Application Support/BuchISY/invoices.db`
- Contains ALL invoices across all months
- SQLite with indexes for fast filtering

**Schema (3 tables):**

The `invoices` table has grown well beyond the original ~20 fields. Newer columns support booking and GoBD features: `steuerzeilen` (JSON tax lines), `buchung` / `buchung_ref` (booking + statement-line reference), `belegnummer`, `ausgangsrechnung` (outgoing-invoice flag), `exportiert`, `wechselkurs`, `rabatt`, `trinkgeld`, `bewirtung_anlass`/`bewirtung_teilnehmer`. See `internal/db/schema.go` for the authoritative column list.

```sql
CREATE TABLE IF NOT EXISTS invoices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    dateiname TEXT NOT NULL,
    rechnungsdatum TEXT, jahr TEXT, monat TEXT,
    auftraggeber TEXT, verwendungszweck TEXT DEFAULT '-', rechnungsnummer TEXT,
    betrag_netto REAL, steuersatz_prozent REAL, steuersatz_betrag REAL, bruttobetrag REAL,
    waehrung TEXT, gegenkonto INTEGER, bankkonto TEXT, bezahldatum TEXT, teilzahlung BOOLEAN DEFAULT 0,
    kommentar TEXT, betrag_netto_eur REAL, gebuehr REAL, hat_anhaenge BOOLEAN DEFAULT 0, ustidnr TEXT,
    steuerzeilen TEXT, buchung TEXT, buchung_ref TEXT DEFAULT '', belegnummer TEXT DEFAULT '',
    ausgangsrechnung INTEGER DEFAULT 0, exportiert INTEGER DEFAULT 0, wechselkurs REAL DEFAULT 0,
    -- ...plus rabatt, trinkgeld, gebuehr_prozent, bewirtung_* (see schema.go)
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
-- NOTE: there is NO UNIQUE constraint on invoices. Duplicate detection is done
-- in code (core/dedupe.go), not by the schema. Indexes:
CREATE INDEX idx_invoices_monat          ON invoices(jahr, monat);
CREATE INDEX idx_invoices_datum          ON invoices(rechnungsdatum);
CREATE INDEX idx_invoices_auftraggeber   ON invoices(auftraggeber);
CREATE INDEX idx_invoices_rechnungsnummer ON invoices(rechnungsnummer);
CREATE INDEX idx_invoices_dateiname      ON invoices(dateiname);

-- GoBD audit trail (append-only), written by core/audit.go via the repository
CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ts DATETIME DEFAULT CURRENT_TIMESTAMP,
    aktion TEXT NOT NULL, entitaet TEXT, schluessel TEXT, details TEXT
);
CREATE INDEX idx_audit_ts ON audit_log(ts);

-- Period locking / Festschreibung; CRUD is guarded against locked (jahr, monat)
CREATE TABLE IF NOT EXISTS period_locks (
    jahr TEXT NOT NULL, monat TEXT NOT NULL,
    locked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(jahr, monat)
);
```

**Data Flow:**
- SQLite = Source of Truth
- CSV = Auto-generated export (for backward compatibility)
- Every change triggers CSV regeneration

### Settings Persistence

- **Read:** `core/settings.go` → JSON file → `Settings` struct
- **Write:** User edits settings → Validate → Write JSON
- **API Key:** Separate keyring storage (never in JSON)

### CSV Format (v2.1)

Each month folder contains an auto-generated `invoices.csv` with these columns (default order):

```
Dateiname, Rechnungsdatum, Jahr, Monat, Auftraggeber, Verwendungszweck,
Rechnungsnummer, BetragNetto, Steuersatz_Prozent, Steuersatz_Betrag,
Bruttobetrag, Waehrung, Gegenkonto, Bankkonto, Bezahldatum, Teilzahlung,
Kommentar, BetragNetto_EUR, Gebuehr, HatAnhaenge, UStIdNr
```

**Notes:**
- CSV is **auto-generated** from database after every change
- Column order is configurable via `Settings.ColumnOrder`
- Backward compatible: Reads old CSVs with Firmenname/Kurzbezeichnung columns
- All fields quoted with double quotes
- Decimal separator: Always `.` in CSV (regardless of UI setting)

## Build System

**Makefile targets:**
- `make build` - Build for current platform
- `make build-macos` - Build for macOS (Universal binary)
- `make build-windows` - Build for Windows
- `make package-macos` - Create `.app` bundle (uses `fyne package`)
- `make run` - Run in dev mode
- `make test` - Run unit tests
- `make test-coverage` - Run tests with coverage
- `make deps` - Update Go dependencies
- `make fmt` - Format code with gofmt
- `make lint` - Run golangci-lint

**Deployment:**
- macOS: `.app` bundle with embedded assets in `Contents/Resources/`
- Windows: `.exe` with bundled assets

## Configuration Locations

### macOS
- **Database:** `~/Library/Application Support/BuchISY/invoices.db` (v2.0+)
- **Settings:** `~/Library/Application Support/BuchISY/settings.json`
- **Company mappings:** `~/Library/Application Support/BuchISY/company_accounts.json`
- **Logs:** `~/Library/Application Support/BuchISY/logs/`
- **API key:** macOS Keychain (service: `BuchISY`)
- **Default storage:** `~/Documents/BuchISY/`

### Windows
- **Database:** `%APPDATA%\BuchISY\invoices.db` (v2.0+)
- **Settings:** `%APPDATA%\BuchISY\settings.json`
- **Company mappings:** `%APPDATA%\BuchISY\company_accounts.json`
- **Logs:** `%APPDATA%\BuchISY\logs\`
- **API key:** Windows Credential Manager
- **Default storage:** `%USERPROFILE%\Documents\BuchISY\`

## Important Notes

### PDF Processing
- **E-Invoice priority:** XRechnung/ZUGFeRD is checked FIRST (highest accuracy)
- **Vision support:** Claude Vision API for scanned PDFs (requires Claude mode)
- **Platform-specific rendering:** ARM64 uses external commands, others use go-fitz
- **No OCR:** BuchISY requires PDFs with embedded text OR uses Claude Vision for scans

### Data Management
- **SQLite is source of truth** (v2.0+)
- **CSV is auto-generated** after every database change
- **Field names:** Auftraggeber (not Firmenname), Verwendungszweck (not Kurzbezeichnung)
- **Backward compatibility:** Can read old CSVs with old column names

### Technical Details
- **Claude Prompt:** German-first prompt in `anthropic/extractor.go` with strict JSON schema
- **Currency:** Always normalized to ISO codes (€ → EUR)
- **Dates:** German format (DD.MM.YYYY) everywhere - both in database and display
- **Decimal Handling:** CSV always uses `.`, UI respects user preference (`,` or `.`)
- **Thread Safety:** PDF extraction runs in background goroutine, UI updates via main thread
- **Error Handling:** No automatic retries; users can re-process files

### UI Specifics (v2.1)
- **Optimized modal header:** Original file + new filename preview on same row (right-aligned)
- **Attachment handling:** Shows "Keine" or count, lists files when added
- **Real-time preview:** Filename updates as user edits company, amount, date, etc.
- **Consistent design:** Same header across new invoice and edit dialogs

## Development Tips

### Code Organization
- **UI code** lives in `internal/ui/` - No business logic in UI
- **Business logic** lives in `internal/core/` - No UI dependencies
- **Database operations** in `internal/db/` - Use repository pattern
- All UI updates must happen on main thread (Fyne requirement)
- Logger is thread-safe, use throughout codebase

### Testing
- Add tests for new features in `*_test.go` files
- Run `make test` before committing
- Test on both macOS and Windows if possible
- Manual testing: Check both languages, edge cases

### Settings & State
- Settings changes require app restart (no hot reload)
- Debug mode (`Settings.DebugMode`) enables verbose logging
- Column order affects both table display and CSV output
- Window/dialog sizes are persisted automatically

### Common Patterns
- **Error handling:** Always check errors, log them, show user-friendly messages
- **Decimal parsing:** Use `parseFloat()` helper (handles both `,` and `.`)
- **Date format:** Always DD.MM.YYYY (German format)
- **i18n:** Use `a.bundle.T("key")` for all user-facing strings
- **Goroutines:** Only for background work (PDF processing), UI updates on main thread

### File Naming
- Template tokens are case-sensitive: `${Company}` not `${company}`
- Sanitization removes invalid characters: `/ \ : * ? " < > |`
- Collision handling: Appends `_2`, `_3`, etc.

## Common Issues

### macOS ARM64 (Apple Silicon)
- **Vision extraction crash:** Fixed in v2.1 using external commands
- **Requires sips:** Built-in to macOS, always available
- **Fallback available:** ImageMagick (convert) or Ghostscript (gs) if installed

### Database
- **Single database for all months:** Not per-month like CSVs
- **Unique constraint:** (jahr, monat, dateiname) prevents duplicates
- **CSV regeneration:** Automatic after INSERT, UPDATE, DELETE

### CSV Export
- **Always uses `.` for decimals** regardless of UI setting
- **Backward compatible reading:** Accepts Firmenname/Kurzbezeichnung columns
- **Forward compatible writing:** Uses Auftraggeber/Verwendungszweck columns

## Version History

### v2.4 – v2.16 (Current): the bookkeeping buildout
A rapid series of feature epics (internally numbered Phase A–E20) turned BuchISY from an
invoice organizer into a GoBD-compliant bookkeeping pre-system. Highlights:
- **Booking engine:** double-entry, multiple VAT lines, SKR04 chart, §13b reverse-charge
- **Revenue / Ausgangsrechnungen:** Erlöskonten, Soll-Besteuerung, revenue export classification
- **VAT filings:** UStVA with official ELSTER Kennzahlen (PDF + XML), ZM (PDF + XML)
- **Reports:** SuSa, GuV, OPOS (aging), Controlling, year overview
- **Fixed assets / AfA** (Anlagen, Anlagenspiegel) and **Kassenbuch**
- **Bank import:** CAMT.053 + MT940 with Belegabgleich/Erlös-Abgleich reconciliation
- **Auto-booking rules engine** (opt-in, default off)
- **Exports:** DATEV-EXTF, Lexware, GoBD/DATEV-Belegpaket ZIP
- **GoBD:** audit trail (Änderungsprotokoll), period locking (Festschreibung), gap-free
  Belegnummern, multi-profile (Mandanten), Verfahrensdokumentation
- New DB tables `audit_log` and `period_locks`; `invoices` extended with booking/export columns

### v2.3
- Improved landing page (clearer hero messaging, benefit-focused)
- Added Code of Conduct (Contributor Covenant)
- Added Contributing guidelines
- Added sample PDFs (XRechnung, ZUGFeRD) for testing
- Updated dependencies (security patches)
- Website integration (www.buchisy.de)
- Documentation improvements

### v2.1
- Fixed PDF vision extraction crash on macOS ARM64
- Platform-specific PDF rendering implementation
- Optimized invoice modal UI with unified header
- Real-time filename preview updates
- Fixed file picker horizontal scrolling
- Comprehensive documentation

### v2.0
- SQLite database as source of truth (breaking change)
- File attachments support
- Currency conversion fields
- Comments/notes field
- USt-IdNr (VAT ID) extraction
- Sortable file picker with date column
- Resizable dialogs with saved dimensions
- Improved CSV export (configurable)

### v1.x
- CSV-based storage (no database)
- Basic PDF text extraction
- Claude API integration
- Local extraction mode

## External Resources

- **Website:** [www.buchisy.de](https://www.buchisy.de)
- **Repository:** [github.com/Bergx2/BuchISY](https://github.com/Bergx2/BuchISY)
- **Issues:** [GitHub Issues](https://github.com/Bergx2/BuchISY/issues)
- **Releases:** [GitHub Releases](https://github.com/Bergx2/BuchISY/releases)
- **Contributing:** See [CONTRIBUTING.md](../CONTRIBUTING.md)
- **Code of Conduct:** See [CODE_OF_CONDUCT.md](../CODE_OF_CONDUCT.md)
- **Functional spec (for rebuild on any stack):** See [docs/FUNCTIONAL_SPEC.md](../docs/FUNCTIONAL_SPEC.md) — IST-exact data model, formulas, file formats, and algorithms
- **Changelog:** See [CHANGELOG.md](../CHANGELOG.md)
- **Company:** [Bergx2 GmbH](https://www.bergx2.de)
