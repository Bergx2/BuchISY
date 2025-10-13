# BuchISY - Project Architecture

## What is BuchISY?

BuchISY is a **desktop invoice management application** (macOS/Windows) that helps small businesses and freelancers organize invoices for bookkeeping. It extracts metadata from PDF invoices using either **Claude AI** (high accuracy) or **local heuristics** (offline), then organizes files into month-based folders with structured CSV records.

**Published by:** Bergx2 GmbH

## Core Features

- **Dual extraction modes:** Claude API (AI-powered) or local pattern matching
- **Smart organization:** Automatic file naming and month-based folder structure (YYYY-MM)
- **CSV tracking:** Maintains `invoices.csv` per month with all invoice metadata
- **Account mapping:** Remembers company→account assignments for faster processing
- **Multi-language:** German (primary) and English with i18n support
- **Privacy-first:** API keys stored in OS keychain, local processing available

## Architecture Overview

### Tech Stack

- **Language:** Go 1.25.2
- **UI Framework:** [Fyne](https://fyne.io/) (cross-platform native GUI)
- **PDF Parsing:** [ledongthuc/pdf](https://github.com/ledongthuc/pdf)
- **AI Integration:** Anthropic Claude API (Messages API)
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
│   │   ├── views.go        # UI layout builders
│   │   ├── invoicemodal.go # Invoice confirmation dialog
│   │   ├── settings.go     # Settings dialog
│   │   ├── table.go        # Invoice table display
│   │   ├── filepicker.go   # PDF file selection
│   │   └── ...
│   ├── core/               # Business logic (domain layer)
│   │   ├── types.go        # Domain models (Meta, Settings, CSVRow)
│   │   ├── storage.go      # File system operations
│   │   ├── csvrepo.go      # CSV read/write with custom column order
│   │   ├── localextract.go # Heuristic-based extraction
│   │   ├── pdftext.go      # PDF text extraction
│   │   ├── template.go     # Filename template engine
│   │   ├── settings.go     # Settings persistence
│   │   ├── companymap.go   # Company→Account mapping
│   │   ├── dedupe.go       # Duplicate detection
│   │   └── sanitize.go     # Filename sanitization
│   ├── anthropic/          # Claude API integration
│   │   ├── extractor.go    # Metadata extraction via Claude
│   │   └── client.go       # HTTP client for Messages API
│   ├── i18n/               # Internationalization
│   │   └── i18n.go         # Translation loading (German/English)
│   └── logging/            # Structured logging
│       └── logger.go       # File-based logger with levels
├── assets/
│   ├── i18n/               # Translation files (de.toml, en.toml)
│   └── icon.png            # App icon
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

- **InvoiceTable (`table.go`)**: Displays monthly invoices
  - Custom Fyne table with sortable columns
  - Supports delete, edit, duplicate operations
  - Configurable column order

- **InvoiceModal (`invoicemodal.go`)**: Confirmation dialog for extracted data
  - Shows all extracted fields for review
  - Account selection with remembered mappings
  - Validates and sanitizes input before saving

- **SettingsDialog (`settings.go`)**: Application configuration
  - Storage paths, filename templates, currency
  - Processing mode (Claude vs Local)
  - API key management (keyring integration)
  - Account definitions, language selection

### 2. Core Layer (`internal/core/`)

**Responsibility:** Business logic, data models, file operations

- **Types (`types.go`)**: Domain models
  - `Meta`: Invoice metadata structure
  - `Settings`: Application settings with JSON persistence
  - `CSVRow`: CSV record structure
  - `Account`, `BankAccount`: User-defined accounts

- **StorageManager (`storage.go`)**: File system operations
  - Creates month folders (YYYY-MM)
  - Handles file moving/renaming with collision detection
  - Scans for all CSV files in storage root

- **CSVRepository (`csvrepo.go`)**: CSV operations
  - Reads/writes invoices.csv with configurable column order
  - Handles decimal formatting (always `.` in CSV)
  - Validates required columns

- **LocalExtractor (`localextract.go`)**: Heuristic-based extraction
  - Pattern matching for German invoices
  - Regex-based field extraction (amounts, dates, invoice numbers)
  - Returns confidence score

- **TemplateEngine (`template.go`)**: Filename generation
  - Token replacement: `${Company}`, `${InvoiceNumber}`, `${YYYY-MM-DD}`, etc.
  - Supports German aliases: `${Firma}`, `${Rechnungsnummer}`
  - Decimal separator configuration (`,` or `.`)

- **SettingsManager (`settings.go`)**: Persists settings to JSON
  - Location: `~/Library/Application Support/BuchISY/settings.json` (macOS)

- **CompanyAccountMap (`companymap.go`)**: Remembers company→account assignments
  - Location: `~/Library/Application Support/BuchISY/company_accounts.json`

- **Deduplication (`dedupe.go`)**: Prevents duplicate invoice entries
  - Checks invoice number, date, and amount

### 3. Anthropic Integration (`internal/anthropic/`)

**Responsibility:** Claude API communication for AI extraction

- **Extractor (`extractor.go`)**: Metadata extraction
  - Uses Claude with structured JSON output
  - System prompt enforces strict JSON schema (German field names)
  - Preprocesses text to prioritize invoice-relevant content (10k char limit)
  - Returns `Meta` with 0.9 confidence

- **Client (`client.go`)**: HTTP client for Claude Messages API
  - Sends text + system prompt to Claude
  - Parses streaming/non-streaming responses
  - Error handling for API failures

### 4. i18n (`internal/i18n/`)

**Responsibility:** Localization support

- Loads `.toml` translation files from `assets/i18n/`
- Supports German (de) and English (en)
- Fallback to German if translations missing

### 5. Logging (`internal/logging/`)

**Responsibility:** Structured file-based logging

- Log levels: DEBUG, INFO, WARN, ERROR
- Location: `~/Library/Application Support/BuchISY/logs/` (macOS)
- Log rotation by date

## Data Flow

### Invoice Processing Flow

1. **User selects PDF** → `ui/filepicker.go`
2. **Extract text from PDF** → `core/pdftext.go` (uses ledongthuc/pdf)
3. **Extract metadata:**
   - **Claude mode:** → `anthropic/extractor.go` → Claude API → JSON response
   - **Local mode:** → `core/localextract.go` → Pattern matching
4. **Suggest account** → `core/companymap.go` (remembers past assignments)
5. **Show confirmation modal** → `ui/invoicemodal.go`
6. **User confirms/edits** → Validation + sanitization
7. **Check for duplicates** → `core/dedupe.go`
8. **Generate filename** → `core/template.go`
9. **Move file** → `core/storage.go` (handles collisions)
10. **Append to CSV** → `core/csvrepo.go`
11. **Remember mapping** → `core/companymap.go` (if enabled)
12. **Reload table** → `ui/table.go`

### Settings Persistence

- **Read:** `core/settings.go` → JSON file → `Settings` struct
- **Write:** User edits settings → Validate → Write JSON
- **API Key:** Separate keyring storage (never in JSON)

### CSV Format

Each month folder contains `invoices.csv` with these columns (default order):

```
Dateiname, Rechnungsdatum, Jahr, Monat, Firmenname,
Kurzbezeichnung, Rechnungsnummer, BetragNetto, Steuersatz_Prozent,
Steuersatz_Betrag, Bruttobetrag, Waehrung, Gegenkonto, Bankkonto,
Bezahldatum, Teilzahlung
```

Column order is configurable via `Settings.ColumnOrder`.

## Build System

**Makefile targets:**
- `make build` - Build for current platform
- `make package-macos` - Create `.app` bundle (uses `fyne package`)
- `make run` - Run in dev mode
- `make test` - Run unit tests
- `make deps` - Update Go dependencies

**Deployment:**
- macOS: `.app` bundle with embedded assets in `Contents/Resources/`
- Windows: `.exe` with bundled assets

## Configuration Locations

- **macOS:**
  - Settings: `~/Library/Application Support/BuchISY/settings.json`
  - Company mappings: `~/Library/Application Support/BuchISY/company_accounts.json`
  - Logs: `~/Library/Application Support/BuchISY/logs/`
  - API key: macOS Keychain (service: `BuchISY`)
  - Default storage: `~/Documents/BuchISY/`

- **Windows:**
  - Settings: `%APPDATA%\BuchISY\settings.json`
  - Company mappings: `%APPDATA%\BuchISY\company_accounts.json`
  - Logs: `%APPDATA%\BuchISY\logs\`
  - API key: Windows Credential Manager
  - Default storage: `%USERPROFILE%\Documents\BuchISY\`

## Important Notes

- **No OCR:** BuchISY requires PDFs with embedded text (not scanned images)
- **Claude Prompt:** German-first prompt in `anthropic/extractor.go` with strict JSON schema
- **Currency:** Always normalized to ISO codes (€ → EUR)
- **Dates:** German format (DD.MM.YYYY) everywhere - both in CSV storage and display
- **Decimal Handling:** CSV always uses `.`, UI respects user preference (`,` or `.`)
- **Thread Safety:** PDF extraction runs in background goroutine, UI updates via main thread
- **Error Handling:** No automatic retries; users can re-process files

## Development Tips

- UI code lives in `internal/ui/`, business logic in `internal/core/`
- All UI updates must happen on main thread (Fyne requirement)
- Logger is thread-safe, use throughout codebase
- Settings changes require app restart (no hot reload)
- Debug mode (`Settings.DebugMode`) enables verbose logging
- Column order affects both table display and CSV output
