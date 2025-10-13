# BuchISY

**BuchISY** is a desktop application for Windows and macOS that helps you centralize and structure company invoices for easy transfer to your bookkeeping agency.

**Published by:** Bergx2 GmbH

## Features

- **PDF Invoice Processing**: Drag-and-drop or select PDF invoices for automatic data extraction
- **Dual Processing Modes**:
  - **Claude API**: High-accuracy extraction using Anthropic's Claude AI
  - **Local Heuristics**: Offline extraction using pattern matching and rules
- **Automatic Organization**: Organizes invoices by year-month (YYYY-MM) with customizable folder structure
- **Smart Naming**: Flexible filename templates with tokens like `${Company}`, `${InvoiceNumber}`, `${GrossAmount}`, etc.
- **Account Management**: Define up to 10 custom accounts (Gegenkonten) and automatically remember company-to-account mappings
- **CSV Export**: Maintains a monthly `invoices.csv` file with all invoice metadata
- **Duplicate Detection**: Prevents accidental duplicate entries based on invoice number, date, and amount
- **Multi-language**: German (default) and English interface with easy switching
- **Privacy-First**: All processing happens locally except Claude API calls; API keys stored securely in OS keychain

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
make build-macos      # macOS (Intel + ARM)
make build-windows    # Windows

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
4. **Save**: Click "Speichern" to save the invoice. The file will be renamed according to your template and moved to the month folder.

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

#### Getting a Claude API Key

1. Sign up at [console.anthropic.com](https://console.anthropic.com/)
2. Navigate to API Keys
3. Create a new key
4. Copy the key and paste it into BuchISY Settings
5. The key will be securely stored in your system keychain (macOS Keychain or Windows Credential Manager)

## CSV Format

Each month folder contains an `invoices.csv` file with the following columns:

```
Dateiname,Rechnungsdatum,Datum_Deutsch,Jahr,Monat,Firmenname,Kurzbezeichnung,Rechnungsnummer,BetragNetto,Steuersatz_Prozent,Steuersatz_Betrag,Bruttobetrag,Waehrung,Gegenkonto
```

- All amounts use `.` as decimal separator in CSV (regardless of UI settings)
- Dates are in `YYYY-MM-DD` format (ISO) for Rechnungsdatum
- Datum_Deutsch uses German format `dd.MM.yyyy`

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

**Solution**: BuchISY currently does not support OCR. Options:
1. Use a PDF with embedded text (native PDF, not scanned)
2. Pre-process scanned PDFs with OCR software (e.g., Adobe Acrobat, ABBYY FineReader)
3. Enter invoice data manually

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

- [ ] Drag-and-drop support for PDF files
- [ ] Settings dialog with all configuration options
- [ ] Full invoice confirmation modal
- [ ] Open folder in system file manager
- [ ] OCR support for scanned PDFs
- [ ] Batch processing with progress indicator
- [ ] Export to other formats (Excel, JSON)
- [ ] Search and filter in invoice table
- [ ] Dark mode support
- [ ] Additional language support (French, Italian, Spanish)

## License

Copyright © 2025 Bergx2 GmbH. All rights reserved.

_License details TBD_

## Support

For issues, questions, or feature requests, please open an issue on [GitHub](https://github.com/bergx2/buchisy/issues).

## Acknowledgments

- Built with [Fyne](https://fyne.io/) - Cross-platform GUI toolkit for Go
- PDF extraction via [ledongthuc/pdf](https://github.com/ledongthuc/pdf)
- Secure key storage via [go-keyring](https://github.com/zalando/go-keyring)
- AI extraction powered by [Anthropic Claude](https://www.anthropic.com/)

---

**BuchISY** - Simplifying invoice management for small businesses.
