# BuchISY

**BuchISY** is a desktop application for Windows and macOS that helps you centralize and structure company invoices for easy transfer to your bookkeeping agency.

**Published by:** Bergx2 GmbH

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
- **Invoice Table Display**: View, sort, and manage all invoices for the selected month
- **Edit Functionality**: Modify existing invoice data with automatic file renaming
- **Delete & Cleanup**: Remove unwanted entries with automatic file management
- **Duplicate Detection**: Smart prevention of duplicate entries based on invoice number, date, and amount
- **CSV Export**: Maintains a monthly `invoices.csv` file with all invoice metadata
- **Customizable Columns**: Configure which columns to display and their order in both table and CSV

### Account & Company Management
- **Account Management**: Define up to 10 custom accounts (Gegenkonten) with codes and descriptions
- **Smart Company Mapping**: Automatically remembers and suggests accounts for repeat vendors
- **Bank Account Tracking**: Optional bank account field for payment processing
- **Payment Date Tracking**: Record when invoices were paid with date picker interface

### User Interface
- **Multi-language**: German (primary) and English interface with instant switching
- **Tooltips**: Hover tooltips for long text in company names, filenames, and descriptions
- **Date Pickers**: Calendar widgets for easy date selection (German format DD.MM.YYYY)
- **Responsive Design**: Native desktop experience on macOS and Windows
- **Window State Memory**: Remembers window size and position between sessions

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

1. Download the latest release for your platform from the [Releases](https://github.com/bergx2/buchisy/releases) page
2. **macOS**: Open the `.app` bundle
3. **Windows**: Run the `.exe` file

### Building from Source

#### Requirements

- Go 1.22 or later
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
MACOSX_DEPLOYMENT_TARGET=15.0 make build-windows    # Windows

# Or apps
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
2. **Add Invoices**: Drag-and-drop PDF files or click "PDF auswählen" to select files
3. **Review Data**: A modal will appear with extracted invoice data. Review and edit as needed:
   - Company name
   - Invoice number
   - Invoice date
   - Amounts (net, tax, gross)
   - Currency
   - Account code (Gegenkonto)
   - Short description
   - Payment date (optional)
   - Bank account (optional)
4. **Save**: Click "Speichern" to save the invoice. The file will be renamed according to your template and moved to the month folder.

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

**Advanced**:
- **Debug Mode**: Enable verbose logging for troubleshooting (logs stored in Application Support folder)

#### Getting a Claude API Key

1. Sign up at [console.anthropic.com](https://console.anthropic.com/)
2. Navigate to API Keys
3. Create a new key
4. Copy the key and paste it into BuchISY Settings
5. The key will be securely stored in your system keychain (macOS Keychain or Windows Credential Manager)

## CSV Format

Each month folder contains an `invoices.csv` file with the following columns (default order):

```
Dateiname,Rechnungsdatum,Jahr,Monat,Firmenname,Kurzbezeichnung,Rechnungsnummer,BetragNetto,Steuersatz_Prozent,Steuersatz_Betrag,Bruttobetrag,Waehrung,Gegenkonto,Bankkonto,Bezahldatum,Teilzahlung
```

- **Column order** can be customized in Settings
- All amounts use `.` as decimal separator in CSV (regardless of UI settings)
- Dates (Rechnungsdatum and Bezahldatum) are in German format `DD.MM.YYYY`
- Currency codes are normalized (€ → EUR, $ → USD, etc.)

## File Structure

```
~/Documents/BuchISY/              # Default storage root
  2024-09/                        # Month folder
    invoices.csv                  # CSV with invoice metadata
    2024-09-15_Acme-GmbH_119,00_EUR.pdf
    2024-09-20_Example-AG_250,50_EUR.pdf
  2024-10/
    invoices.csv
    ...

~/Library/Application Support/BuchISY/  # macOS config
  settings.json                   # App settings
  company_accounts.json           # Company→Account mappings
  logs/                           # Application logs
```

## Privacy & Security

- **API Key**: Stored in OS keychain (not in plain text)
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
    anthropic/          # Claude API integration
    i18n/               # Internationalization
    logging/            # Logging
  assets/
    i18n/               # Translation files
    icon.png            # App icon
  Makefile              # Build automation
```

### Running Tests

```bash
make test
make test-coverage  # With coverage report
```

### Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Code Style

- Run `make fmt` before committing
- Run `make lint` to check for issues (requires [golangci-lint](https://golangci-lint.run/))

## Roadmap

### Completed Features ✅
- [x] Drag-and-drop support for PDF files
- [x] Settings dialog with all configuration options
- [x] Full invoice confirmation modal with all fields
- [x] Edit functionality for existing invoices
- [x] Claude Vision support for scanned PDFs (OCR alternative)
- [x] E-invoice support (XRechnung, ZUGFeRD)
- [x] Tooltips for long text fields
- [x] Date picker with calendar widget
- [x] Company-to-account mapping memory
- [x] Customizable CSV column order

### Planned Features
- [ ] Open folder in system file manager
- [ ] Batch processing with progress indicator
- [ ] Export to other formats (Excel, JSON, QuickBooks)
- [ ] Search and filter in invoice table
- [ ] Keyboard shortcuts for common operations
- [ ] Dark mode support
- [ ] Additional language support (French, Italian, Spanish)
- [ ] Cloud backup/sync support
- [ ] Multi-user collaboration features
- [ ] Invoice templates for recurring vendors
- [ ] Automatic categorization with machine learning
- [ ] Email invoice import
- [ ] Receipt scanning via mobile app

## License

MIT License - Copyright © 2025 Bergx2 GmbH

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

You are free to use, modify, distribute, and sell this software without restriction.

## Support

For issues, questions, or feature requests, please open an issue on [GitHub](https://github.com/bergx2/buchisy/issues).

## Acknowledgments

- Built with [Fyne](https://fyne.io/) - Cross-platform GUI toolkit for Go
- PDF extraction via [ledongthuc/pdf](https://github.com/ledongthuc/pdf)
- Secure key storage via [go-keyring](https://github.com/zalando/go-keyring)
- AI extraction powered by [Anthropic Claude](https://www.anthropic.com/)

---

**BuchISY** - Simplifying invoice management for small businesses.
