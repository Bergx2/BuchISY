# BuchISY

**BuchISY** is a desktop application for Windows and macOS: a GoBD-compliant German bookkeeping pre-system (Buchhaltungs-Vorsystem) for small businesses and freelancers. It captures invoices and receipts, books them with proper double-entry VAT logic against an SKR04 chart of accounts, locks periods immutably, produces the German VAT filings, and hands your tax advisor a clean DATEV/receipt package.

**Published by:** Bergx2 GmbH
**Website:** [www.buchisy.de](https://www.buchisy.de)
**Current Version:** v2.16

> **Note:** The codebase is under heavy development and the documentation is constantly evolving. Give it a try and let us know what you think by [creating an issue](https://github.com/Bergx2/BuchISY/issues). Watch releases of this repo to get notified of updates. And give us a star if you like it!

## What BuchISY does

BuchISY started as an invoice organizer (drag a PDF in, extract the metadata, file it into a month folder) and has grown into a full bookkeeping pre-system. It is deliberately a **Vorsystem**: it does the day-to-day capture, booking, VAT, and export, then hands a clean package to your Steuerberater. It does **not** attempt a full annual close (HGB-Bilanz/Jahresabschluss) — SuSa/GuV are evaluations, not a Jahresabschluss.

**Capture → Book → Lock → File → Export:**
- **Capture:** drag-and-drop, clipboard paste, a watched scan inbox, or batch import. Extraction via Claude AI, local heuristics, or structured XRechnung/ZUGFeRD e-invoice data.
- **Book:** double-entry bookings with multiple VAT lines, SKR04 accounts, §13b reverse-charge, and an opt-in auto-booking rules engine.
- **Lock:** GoBD audit trail (Änderungsprotokoll) plus period locking (Festschreibung) and gap-free sequential receipt numbers.
- **File:** UStVA and ZM tax filings with official ELSTER Kennzahlen, plus SuSa/GuV, OPOS, and Controlling reports.
- **Export:** DATEV-EXTF, Lexware, and a GoBD/DATEV receipt package (Belegpaket) ready for the tax advisor.

> **Scope note:** earlier sections of this README described a "v2.3 invoice organizer." That framing is out of date. The feature list below reflects the current v2.16 product. See the [CHANGELOG](CHANGELOG.md) for the full release history.

## Features

### Capture & Extraction
- **Multiple ingest paths**: Drag-and-drop, clipboard paste (file path or raw image/screenshot), file/batch picker, and an auto-watched scan-inbox folder
- **Dual Processing Modes**:
  - **Claude API**: High-accuracy extraction using Anthropic's Claude AI with Vision support for scanned PDFs
  - **Local Heuristics**: Offline extraction using pattern matching and rules for standard German invoices
- **E-Invoice Support**: Process XRechnung and ZUGFeRD (CII) electronic invoices with structured XML data extraction
- **Automatic Organization**: Organizes receipts by year-month (YYYY-MM) with customizable folder structure
- **Smart Naming**: Flexible filename templates with tokens like `${Company}`, `${InvoiceNumber}`, `${GrossAmount}`, etc.
- **Batch Processing**: Process multiple invoices in sequence with automatic metadata extraction

### Bookkeeping & Tax
- **Double-entry booking engine**: Multiple VAT lines, SKR04 chart of accounts, §13b reverse-charge, gift/travel/Kfz rules
- **Revenue / outgoing invoices**: Erlöskonten, Soll-Besteuerung (Forderung 1400 → Bank on payment), revenue export classification
- **VAT filings**: UStVA (Umsatzsteuer-Voranmeldung) with official ELSTER Kennzahlen, month/quarter/year, PDF + ELSTER XML; ZM (Zusammenfassende Meldung) per customer VAT-ID, PDF + XML
- **Reports**: SuSa (trial balance), GuV (P&L), OPOS (open items with aging buckets), Controlling (income/expense per account), per-year KPI overview
- **Fixed assets (Anlagen)**: Asset register, linear AfA (time-apportioned), GWG hint at ≤800 €, Anlagenspiegel PDF
- **Cash book (Kassenbuch)**: Per-Barkasse ledger with opening-balance carry-over and cash-coverage checks
- **Auto-booking rules engine**: Per-supplier/keyword booking templates, opt-in autobook (default off) with a plausibility gate

### Bank reconciliation
- **Statement import**: In-house CAMT.053 (ISO 20022) and MT940 parsers with automatic format detection
- **Belegabgleich / Erlös-Abgleich**: Scored matching of invoices to bank-statement lines, grouped/partial payments, alias learning, and a "missing receipts" list

### GoBD compliance
- **Audit trail**: Append-only Änderungsprotokoll for create/update/delete/lock/unlock events
- **Period locking (Festschreibung)**: Lock a month immutably; locked periods are guarded across edit/delete, including cross-month moves
- **Gap-free receipt numbers**: Sequential Belegnummern per profile/year with renumbering
- **Multi-profile (Mandanten)**: Separate chart, rules, and data per company profile
- **Verfahrensdokumentation**: GoBD-required process documentation generated as a PDF from the profile's own settings

### Exports
- **CSV**: Auto-generated `invoices.csv` per month from the database
- **DATEV-EXTF** Buchungsstapel and **Lexware** CSV
- **GoBD/DATEV-Belegpaket**: ZIP bundling the EXTF Stapel + linked receipt images + `manifest.csv` + GoBD `index.xml` (GDPdU/Z3)
- **Reports as PDF**: Belegliste, Rechnungsausgangsbuch, Verfahrensdokumentation, and a full backup ZIP

### Data Management
- **SQLite Database**: Fast, reliable SQLite database as source-of-truth
- **Invoice Table Display**: View, sort, and manage all invoices for the selected month
- **Edit Functionality**: Modify existing invoice data with automatic file renaming
- **Delete & Cleanup**: Remove unwanted entries with automatic file and database management
- **Duplicate Detection**: SQL-based prevention of duplicate entries
- **CSV Auto-Export**: Automatically generates `invoices.csv` after every change
- **Customizable Columns**: Configure which columns to display and their order
- **File Attachments**: Upload additional files (receipts, contracts) per invoice
- **Comments**: Add notes and comments to invoices
- **Currency Conversion**: Track foreign currency amounts with EUR conversion and fees

### Account & Company Management
- **Account Management**: Define up to 10 custom accounts (Gegenkonten) with codes and descriptions
- **Smart Company Mapping**: Automatically remembers and suggests accounts for repeat vendors
- **Bank Account Tracking**: Optional bank account field for payment processing
- **Payment Date Tracking**: Record when invoices were paid with date picker interface

### User Interface
- **Multi-language**: German (primary) and English interface with instant switching
- **Sortable File Picker**: Click column headers (Dateiname, Datum) to sort files
- **Quick Shortcuts**: Desktop, Documents, and Downloads folder buttons
- **Resizable Dialogs**: Invoice dialogs remember your preferred size
- **Tooltips**: Hover tooltips for long text fields
- **Date Pickers**: Calendar widgets for easy date selection (German format DD.MM.YYYY)
- **Full-Width Table**: Maximized space for invoice list (no sidebar)
- **Responsive Design**: Native desktop experience on macOS and Windows
- **Window State Memory**: Remembers window and dialog sizes between sessions

### Security & Privacy
- **Privacy-First**: All processing happens locally except when using Claude API
- **Secure Key Storage**: API keys stored in OS keychain (macOS Keychain/Windows Credential Manager)
- **No Telemetry**: No usage tracking or data collection
- **Local Logs**: Debug logs stored locally for troubleshooting

## Screenshots

_Coming soon_

## Installation

### Prerequisites

- macOS 11+ or Windows 10+
- (Optional) Claude API key from [Anthropic Console](https://console.anthropic.com/) for AI-powered extraction

### Download

1. Download the latest release for your platform:
   - **Website**: [www.buchisy.de](https://www.buchisy.de)
   - **GitHub**: [Releases](https://github.com/bergx2/buchisy/releases) page
2. **macOS**: Open the `.app` bundle
3. **Windows**: Run the `.exe` file

### Building from Source

#### Requirements

- Go 1.25 or later
- Git

#### Steps

```bash
# Clone the repository
git clone https://github.com/bergx2/buchisy.git
cd buchisy

# Download dependencies
make deps

# Build for your platform
make build

# Or build for specific platforms
MACOSX_DEPLOYMENT_TARGET=15.0 make build-macos      # macOS (Intel + ARM)
make build-windows                                  # Windows

# Or create macOS app bundle
MACOSX_DEPLOYMENT_TARGET=15.0 make package-macos

# Run the application
./build/buchisy
```

For detailed build instructions (Windows cross-compilation, Docker, CI) see [BUILDING.md](BUILDING.md). For setting up the app and dev environment on a new machine, see [SETUP.md](SETUP.md).

## Usage

### First Run

1. **Language**: Choose your preferred language (German or English) in Settings
2. **Storage Folder**: BuchISY will propose `~/Documents/BuchISY` as the default storage location. You can change this in Settings.
3. **Processing Mode**: Choose between:
   - **Claude API**: Requires an API key (see Configuration below)
   - **Local (Heuristics)**: Works offline, best for standard German invoices

### Processing Invoices

1. **Select Month**: Use the year-month selector at the top to choose the target month (defaults to last month)
2. **Add Invoices**: Click "Datei auswählen" in the header to select files
3. **Review Data**: A resizable modal will appear with extracted invoice data. Review and edit:
   - Auftraggeber (payer/client)
   - Verwendungszweck (purpose/description)
   - Invoice number
   - USt-IdNr (VAT ID) - auto-extracted
   - Invoice date
   - Amounts (net, VAT, gross) with currency
   - Currency conversion (for non-EUR invoices)
   - Account code (Gegenkonto)
   - Payment date (optional)
   - Bank account (optional)
   - Comments/notes
   - File attachments (receipts, contracts, etc.)
4. **Save**: Click "Speichern" to save. The invoice is stored in SQLite and exported to CSV automatically.

### Extraction Modes Explained

**Claude API Mode** (Recommended):
- Uses Anthropic's Claude AI for intelligent extraction
- Supports both native PDFs and scanned documents (via Claude Vision)
- Handles complex layouts and multiple languages
- Requires internet connection and API key
- Higher accuracy for non-standard invoice formats

**Local Mode** (Offline):
- Uses pattern matching and heuristics
- Works completely offline - no internet required
- Best for standard German invoices
- Does not support scanned PDFs
- Faster processing for simple invoices

### PDF Processing Details

#### Processing Flow
1. **E-Invoice Detection**: Check for XRechnung/ZUGFeRD structured XML data
2. **Text Extraction**: Extract embedded text using native Go libraries
3. **Metadata Extraction**: Process with Claude API or local pattern matching
4. **Vision Fallback**: If no text found, convert to image for Claude Vision API

#### Text Extraction (All Platforms)
- **E-Invoice formats**: Native Go XML parsing for XRechnung/ZUGFeRD
- **Regular PDFs**: `ledongthuc/pdf` library for text extraction
- **Processing modes**:
  - Claude API for AI-powered analysis
  - Local mode for regex-based pattern matching

#### Vision Extraction (Scanned PDFs)

| Platform | Method | Implementation |
|----------|--------|----------------|
| **Windows** | go-fitz (MuPDF) | Native library rendering at 144 DPI |
| **Linux** | go-fitz (MuPDF) | Native library rendering at 144 DPI |
| **macOS Intel** | go-fitz (MuPDF) | Native library rendering at 144 DPI |
| **macOS ARM64** | External commands | `sips` → `convert` → `gs` (fallback chain) |

**Note**: macOS ARM64 (Apple Silicon) uses external commands to avoid signal handling issues with go-fitz. All other platforms use the go-fitz library for reliable PDF-to-image conversion.

### Configuration

#### Settings Dialog

Access via the "Einstellungen" button.

**Storage (Ablage)**:
- **Target Folder**: Where invoices are stored
- **Use Month Subfolders**: Organize invoices into `YYYY-MM` folders (recommended)

**Filenames (Dateinamen)**:
- **Template**: Customize filename format using tokens:
  - `${YYYY}`, `${MM}`, `${DD}`: Year, month, day
  - `${Company}`: Company name
  - `${InvoiceNumber}`: Invoice number
  - `${GrossAmount}`: Gross amount
  - `${Currency}`: Currency code
  - German aliases supported: `${Firma}`, `${Rechnungsnummer}`, etc.
- **Decimal Separator**: Choose `,` or `.` for amounts in filenames
- **Default Currency**: Default currency for new invoices

**Processing (Verarbeitung)**:
- **Mode**: Claude API or Local
- **Model**: Claude model to use (default: `claude-sonnet-4-6`)
- **API Key**: Your Anthropic API key (stored securely in OS keychain)

**Accounts (Konten)**:
- **Chart of accounts**: SKR04-style accounts; the account picker searches by code and name
- **Default Account**: Default counter-account (Gegenkonto) for new bookings
- **Custom Accounts**: Add custom accounts with code and label
- **Remember Mappings**: Automatically remember which account you assign to each company

**Language (Sprache)**:
- Switch between German and English

**CSV Format**:
- **CSV Separator**: Choose comma (default), semicolon, or tab
- **CSV Encoding**: Choose ISO-8859-1 (default) or UTF-8
- All CSV files auto-generated from database

**Advanced**:
- **Column Order**: Customize table and CSV column order
- **Debug Mode**: Enable verbose logging for troubleshooting
- **Wipe Database**: Delete all invoice data (with confirmation)

#### Getting a Claude API Key

1. Sign up at [console.anthropic.com](https://console.anthropic.com/)
2. Navigate to API Keys
3. Create a new key
4. Copy the key and paste it into BuchISY Settings
5. The key will be securely stored in your system keychain (macOS Keychain or Windows Credential Manager)

## Data Storage

### SQLite Database (Source-of-Truth)
- **Location**: `~/Library/Application Support/BuchISY/invoices.db` (macOS) or `%APPDATA%\BuchISY\invoices.db` (Windows)
- **Single database** for all invoices across all months
- **Automatic indexes** for fast searching and filtering
- **Timestamps**: Tracks when invoices were created and last modified

### CSV Export (Auto-Generated)

Each month folder contains an auto-generated `invoices.csv` file with the following columns (default order):

```
"Dateiname","Rechnungsdatum","Jahr","Monat","Auftraggeber","Verwendungszweck","Rechnungsnummer","BetragNetto","Steuersatz_Prozent","Steuersatz_Betrag","Bruttobetrag","Waehrung","Gegenkonto","Bankkonto","Bezahldatum","Teilzahlung","Kommentar","BetragNetto_EUR","Gebuehr","HatAnhaenge","UStIdNr"
```

**CSV Features:**
- **Auto-generated** from SQLite database after every change
- **All fields quoted** with double quotes
- **Configurable separator**: Comma (default), semicolon, or tab
- **Configurable encoding**: ISO-8859-1 (default) or UTF-8
- **Column order** customizable in Settings
- **Decimal separator**: Comma or dot (matches your preference)
- **Backward compatible**: Can read old CSV files with Firmenname/Kurzbezeichnung columns

## File Structure

```
~/Documents/BuchISY/              # Default storage root
  2024-09/                        # Month folder
    invoices.csv                  # Auto-generated CSV export
    2024-09-15_Acme-Corp_240,00_USD.pdf
    2024-09-15_Acme-Corp_240,00_USD-files/  # Attachments folder (NEW!)
      receipt.jpg
      contract.pdf
    2024-09-20_Example-AG_500,00_EUR.pdf
  2024-10/
    invoices.csv
    ...

~/Library/Application Support/BuchISY/  # macOS config
  invoices.db                     # SQLite database (SOURCE-OF-TRUTH!)
  settings.json                   # App settings
  company_accounts.json           # Company→Account mappings
  profiles/                       # Per-profile (Mandant) chart + booking rules
    Bergx2/
    ...
  logs/                           # Application logs

# Windows equivalent:
%APPDATA%\BuchISY\
  invoices.db
  settings.json
  company_accounts.json
  profiles\
  logs\
```

## Upgrading

### Within the v2.x series

Releases in the v2.x series are **non-breaking** for your data. Download the latest version and replace your existing installation — `invoices.db` and your config in `~/Library/Application Support/BuchISY/` (macOS) or `%APPDATA%\BuchISY\` (Windows) stay compatible and are migrated automatically on startup when needed.

### From v1.x to v2.0+

**Important Notes:**
- v2.0+ uses SQLite database instead of CSV as primary storage
- Old CSV files are backward compatible and can be read
- Start fresh with v2.0+ (recommended) or manually import old invoices

**Migration Options:**

**Option 1: Fresh Start (Recommended)**
1. Install the latest version
2. Start adding new invoices
3. Old CSV files remain accessible for reference

**Option 2: Keep Old Data**
- Your existing PDF files and CSV files remain untouched
- Old CSVs can still be read (backward compatible column names)
- Manually re-process old invoices if you want them in the database

**What Changes (v2.0+):**
- Database location: `~/Library/Application Support/BuchISY/invoices.db`
- CSV files become exports (regenerated automatically)
- Field names in database: Auftraggeber, Verwendungszweck (CSV backward compatible)

## Privacy & Security

- **API Key**: Stored in OS keychain (not in plain text)
- **Database**: Stored locally, fully under your control
- **No Telemetry**: BuchISY does not send any telemetry or usage data
- **Local Processing**: When using local mode, no data leaves your machine
- **Claude API**: When using Claude mode, invoice text is sent to Anthropic's API over HTTPS. See [Anthropic's Privacy Policy](https://www.anthropic.com/privacy)
- **Logs**: Application logs (timestamps, file sizes, errors) are stored locally in `~/Library/Application Support/BuchISY/logs/` (macOS) or `%APPDATA%\BuchISY\logs\` (Windows)

## Troubleshooting

### "No text detected" error

**Cause**: The PDF is image-based (scanned document) without embedded text.

**Solution**:
- **Claude API Mode**: BuchISY supports scanned PDFs through Claude Vision API. Ensure you're using Claude mode with a valid API key.
- **Local Mode**: Does not support scanned PDFs. Options:
  1. Switch to Claude API mode for automatic extraction from scanned PDFs
  2. Use a PDF with embedded text (native PDF, not scanned)
  3. Pre-process scanned PDFs with OCR software (e.g., Adobe Acrobat, ABBYY FineReader)
  4. Enter invoice data manually

### "API key missing" error

**Cause**: Claude API mode is selected but no API key is configured.

**Solution**:
1. Open Settings
2. Enter your Claude API key
3. Or switch to Local mode

### Incorrect extraction results

**Claude Mode**: Try adjusting the prompt or updating to a newer Claude model.

**Local Mode**: Local extraction uses heuristics and works best with standard German invoices. For complex layouts, Claude mode is recommended.

### Application won't start

**macOS**: Check System Preferences > Security & Privacy. You may need to allow BuchISY.

**Windows**: Check Windows Defender or antivirus software.

## Development

### Project Structure

```
buchisy/
  cmd/buchisy/          # Main entry point
  internal/
    ui/                 # Fyne UI components
    core/               # Core business logic
    db/                 # SQLite database layer (NEW!)
    anthropic/          # Claude API integration
    i18n/               # Internationalization
    logging/            # Logging
  assets/
    i18n/               # Translation files
    icon.png            # App icon
  .github/
    workflows/          # GitHub Actions for automated builds
  Makefile              # Build automation
```

### Running Tests

```bash
make test
make test-coverage  # With coverage report
```

### Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) and [Code of Conduct](CODE_OF_CONDUCT.md) before submitting pull requests.

**Quick start:**
1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run `make fmt && make lint && make test`
5. Submit a pull request

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

## Roadmap

### Shipped ✅

**Capture & data foundation:**
- [x] **SQLite database** as source-of-truth, with auto-generated CSV export
- [x] **Claude Vision** support for scanned PDFs (platform-specific rendering, ARM64 fix)
- [x] **E-invoice support** (XRechnung, ZUGFeRD / CII)
- [x] **Multiple ingest paths**: drag-and-drop, clipboard paste, batch, watched scan inbox
- [x] **File attachments**, currency conversion, comments, USt-IdNr extraction
- [x] **Global search** across months, sortable file picker, keyboard navigation

**Bookkeeping & tax:**
- [x] **Double-entry booking engine** with multiple VAT lines, SKR04, §13b reverse-charge
- [x] **Revenue / outgoing invoices** (Ausgangsrechnungen, Soll-Besteuerung)
- [x] **UStVA** with official ELSTER Kennzahlen (PDF + XML) and **ZM** (PDF + XML)
- [x] **SuSa, GuV, OPOS, Controlling** reports
- [x] **Fixed assets / AfA** (Anlagen, Anlagenspiegel) and **Kassenbuch**
- [x] **Auto-booking rules engine** (opt-in)

**Bank & exports:**
- [x] **CAMT.053 + MT940** bank-statement import with Belegabgleich/Erlös-Abgleich
- [x] **DATEV-EXTF**, **Lexware**, and **GoBD/DATEV-Belegpaket** export

**GoBD compliance:**
- [x] **Audit trail** (Änderungsprotokoll), **period locking** (Festschreibung)
- [x] **Gap-free Belegnummern**, **multi-profile (Mandanten)**, **Verfahrensdokumentation**

### Possible next steps
- [ ] Shared multi-user database (concurrent access for a second team member)
- [ ] Full HGB balance sheet / Jahresabschluss (currently out of scope by design)
- [ ] Degressive AfA automation
- [ ] Additional language support
- [ ] DATEV online API integration

## License

MIT License - Copyright © 2025 Bergx2 GmbH

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

You are free to use, modify, distribute, and sell this software without restriction.

## Support

- **Issues & Feature Requests**: [GitHub Issues](https://github.com/bergx2/buchisy/issues)
- **Website**: [www.buchisy.de](https://www.buchisy.de)
- **Enterprise Solutions**: Contact [info@bergx2.de](mailto:info@bergx2.de) or visit [www.bergx2.de](https://www.bergx2.de)

## Acknowledgments

- Built with [Fyne](https://fyne.io/) - Cross-platform GUI toolkit for Go
- Database powered by [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) - Pure Go SQLite
- PDF extraction via [ledongthuc/pdf](https://github.com/ledongthuc/pdf)
- E-invoice processing via [pdfcpu](https://pdfcpu.io/) and [go-fitz](https://github.com/gen2brain/go-fitz)
- Secure key storage via [go-keyring](https://github.com/zalando/go-keyring)
- AI extraction powered by [Anthropic Claude](https://www.anthropic.com/)

---

**BuchISY** - GoBD-compliant bookkeeping for small businesses and freelancers.

🌐 [www.buchisy.de](https://www.buchisy.de) | 💻 [GitHub](https://github.com/Bergx2/BuchISY) | 🏢 [Bergx2 GmbH](https://www.bergx2.de)
