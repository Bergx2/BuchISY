# BuchISY

**BuchISY** is a desktop application for Windows and macOS that helps you centralize and structure company invoices for easy transfer to your bookkeeping agency.

**Published by:** Bergx2 GmbH
**Website:** [www.buchisy.de](https://www.buchisy.de)

> **Note:** The codebase is under heavy development and the documentation is constantly evolving. Give it a try and let us know what you think by [creating an issue](https://github.com/Bergx2/BuchISY/issues). Watch releases of this repo to get notified of updates. And give us a star if you like it!

## 🆕 Version 2.3 - Polish & Refinements

**New in v2.3:**
- **Improved landing page** - Clearer hero messaging and benefit-focused copy
- **Community foundation** - Code of Conduct and Contributing guidelines
- **Sample PDFs** - XRechnung and ZUGFeRD examples for testing
- **Security updates** - Updated dependencies (golang.org/x/crypto, net, sys, image)
- **Website integration** - www.buchisy.de domain references throughout
- **Documentation improvements** - Updated architecture docs and build instructions

## Version 2.1 - ARM64 Fix & UI Improvements

**New in v2.1:**
- **Fixed PDF vision extraction crash** on macOS ARM64 (Apple Silicon)
- Platform-specific PDF rendering (external commands for ARM64, go-fitz for others)
- **Optimized invoice modal header** with unified file information layout
- Real-time filename preview updates in new/edit dialogs
- Fixed file picker horizontal scrolling
- Comprehensive PDF processing documentation

## 📦 Version 2.0 - Major Update!

**⚠️ Breaking Changes (v2.0):**
- SQLite database replaces CSV as source-of-truth
- Field renames: Firmenname → Auftraggeber, Kurzbezeichnung → Verwendungszweck
- CSV files are now auto-generated exports (backward compatible reading)

**Features added in v2.0:**
- SQLite database for faster, more reliable data management
- USt-IdNr (VAT ID) field with automatic extraction
- File attachments support
- Currency conversion fields for foreign invoices
- Comments/notes field
- Sortable file picker with date column
- Resizable dialogs with saved dimensions
- Improved CSV export (configurable separator, encoding, quotes)

## Features

### Invoice Processing
- **PDF Invoice Processing**: Drag-and-drop or select PDF invoices for automatic data extraction
- **Dual Processing Modes**:
  - **Claude API**: High-accuracy extraction using Anthropic's Claude AI with Vision support for scanned PDFs
  - **Local Heuristics**: Offline extraction using pattern matching and rules for standard German invoices
- **E-Invoice Support**: Process XRechnung and ZUGFeRD electronic invoices with structured XML data extraction
- **Automatic Organization**: Organizes invoices by year-month (YYYY-MM) with customizable folder structure
- **Smart Naming**: Flexible filename templates with tokens like `${Company}`, `${InvoiceNumber}`, `${GrossAmount}`, etc.
- **Batch Processing**: Process multiple invoices in sequence with automatic metadata extraction

### Data Management
- **SQLite Database**: Fast, reliable SQLite database as source-of-truth (v2.0!)
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
- **Model**: Claude model to use (e.g., `claude-3-5-sonnet-20241022`)
- **API Key**: Your Anthropic API key (stored securely in OS keychain)

**Accounts (Konten)**:
- **Default Account**: Default account code (default: 2000 - Ausgaben)
- **Custom Accounts**: Add up to 10 custom accounts with code and label
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

## Data Storage (v2.0)

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

## File Structure (v2.0)

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
  invoices.db                     # SQLite database (SOURCE-OF-TRUTH!) (NEW!)
  settings.json                   # App settings
  company_accounts.json           # Company→Account mappings
  logs/                           # Application logs

# Windows equivalent:
%APPDATA%\BuchISY\
  invoices.db
  settings.json
  company_accounts.json
  logs\
```

## Upgrading

### From v2.x to v2.3

**Good news:** v2.3 is a **non-breaking update** with community improvements and documentation enhancements. Simply download the latest version and replace your existing installation. All your data remains compatible.

### From v1.x to v2.0+

**Important Notes:**
- v2.0+ uses SQLite database instead of CSV as primary storage
- Old CSV files are backward compatible and can be read
- Start fresh with v2.0+ (recommended) or manually import old invoices

**Migration Options:**

**Option 1: Fresh Start (Recommended)**
1. Install v2.1
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

### Completed Features ✅

**v2.3:**
- [x] **Improved landing page** - Clearer hero messaging, benefit-focused copy
- [x] **Code of Conduct** - Contributor Covenant for open community
- [x] **Contributing guidelines** - Comprehensive guide for contributors
- [x] **Sample PDFs** - XRechnung and ZUGFeRD examples included
- [x] **Security updates** - Updated all dependencies with security patches
- [x] **Website integration** - www.buchisy.de domain throughout docs
- [x] **Documentation improvements** - Updated architecture and build docs

**v2.1:**
- [x] **Fixed PDF vision crash** on macOS ARM64 (Apple Silicon)
- [x] **Platform-specific PDF rendering** (external commands for ARM64, go-fitz for others)
- [x] **Optimized invoice modal UI** with unified header layout
- [x] **Real-time filename preview** in new/edit dialogs
- [x] **Fixed file picker scrolling** with optimized column widths

**v2.0:**
- [x] **SQLite database** as source-of-truth
- [x] **File attachments** support
- [x] **Currency conversion** fields
- [x] **Comments/notes** field
- [x] **USt-IdNr** (VAT ID) extraction
- [x] **Sortable file picker** with date column
- [x] **Resizable dialogs** with saved dimensions
- [x] Drag-and-drop support for PDF files
- [x] Settings dialog with all configuration options
- [x] Full invoice confirmation modal
- [x] Edit functionality for existing invoices
- [x] Claude Vision support for scanned PDFs
- [x] E-invoice support (XRechnung, ZUGFeRD)
- [x] Tooltips for long text fields
- [x] Date picker with calendar widget
- [x] Company-to-account mapping memory
- [x] Customizable CSV column order
- [x] Configurable CSV format (separator, encoding, quotes)

### Planned Features
- [ ] SQL-based search and filtering (coming in v2.4)
- [ ] Batch processing with progress indicator
- [ ] Advanced reporting and statistics
- [ ] Export to Excel/JSON formats
- [ ] Keyboard shortcuts for common operations
- [ ] Dark mode support
- [ ] Additional language support (French, Italian, Spanish)
- [ ] Cloud backup/sync support
- [ ] Invoice templates for recurring vendors
- [ ] Email invoice import
- [ ] Mobile companion app

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

**BuchISY** - Simplifying invoice management for small businesses.

🌐 [www.buchisy.de](https://www.buchisy.de) | 💻 [GitHub](https://github.com/Bergx2/BuchISY) | 🏢 [Bergx2 GmbH](https://www.bergx2.de)
