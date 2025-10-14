# XRechnung and ZUGFeRD Integration

## Overview

This document describes the integration of **XRechnung** and **ZUGFeRD** structured electronic invoice formats into BuchISY. These formats embed machine-readable XML data directly into PDF files, allowing for perfect data extraction without OCR, text parsing, or AI inference.

## What are XRechnung and ZUGFeRD?

### XRechnung

**XRechnung** is the German standard for electronic invoices (e-invoicing) in public procurement.

- **Specification**: Based on EU standard EN 16931
- **Format**: CII (Cross Industry Invoice) XML embedded in PDF
- **Mandatory**: Required for invoices to German government entities since November 2020
- **Scope**: B2G (Business-to-Government) primarily, increasingly B2B
- **Standard**: Semantic data model with strict validation rules

**Key Benefits**:
- 100% accurate data extraction
- Machine-readable structured data
- Legally compliant for government invoicing
- Automated processing without manual intervention

### ZUGFeRD

**ZUGFeRD** (Zentraler User Guide des Forums elektronische Rechnung Deutschland) is a hybrid invoice format combining human-readable PDF with machine-readable XML.

- **Format**: PDF/A-3 with embedded XML attachment
- **Profiles**:
  - **MINIMUM**: Basic invoice data
  - **BASIC WL**: Without line items (simplified)
  - **BASIC**: With line items
  - **COMFORT**: Extended data for automated booking
  - **EXTENDED**: Full feature set including delivery notes
  - **XRECHNUNG**: XRechnung-compliant profile
- **Versions**: ZUGFeRD 1.0, 2.0, 2.1, 2.2 (latest)
- **Scope**: B2B (Business-to-Business), widely adopted in Germany

**Key Benefits**:
- Dual format: PDF for humans, XML for machines
- No special software needed to view invoice (standard PDF reader)
- Automated data extraction and accounting system integration
- Reduces manual data entry errors to zero

## Technical Architecture

### PDF Structure

Both XRechnung and ZUGFeRD embed XML data as attachments in PDF files:

```
PDF File
├── Visual Content (pages with invoice layout)
└── Embedded Files
    └── factur-x.xml / zugferd-invoice.xml / xrechnung.xml
        └── XML data with invoice details
```

**File Naming Conventions**:
- ZUGFeRD 2.x: `factur-x.xml` (Factur-X is the French/German joint standard)
- ZUGFeRD 1.x: `ZUGFeRD-invoice.xml`
- XRechnung: `xrechnung.xml` or embedded as CII XML

### XML Structure

Both formats use CII (Cross Industry Invoice) XML schema based on UN/CEFACT standards:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<rsm:CrossIndustryInvoice xmlns:...>
  <!-- Header Information -->
  <rsm:ExchangedDocumentContext>
    <ram:GuidelineSpecifiedDocumentContextParameter>
      <ram:ID>urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0</ram:ID>
    </ram:GuidelineSpecifiedDocumentContextParameter>
  </rsm:ExchangedDocumentContext>

  <!-- Document Details -->
  <rsm:ExchangedDocument>
    <ram:ID>RE-2024-001</ram:ID>  <!-- Invoice Number -->
    <ram:TypeCode>380</ram:TypeCode>  <!-- Commercial Invoice -->
    <ram:IssueDateTime>
      <udt:DateTimeString format="102">20240115</udt:DateTimeString>
    </ram:IssueDateTime>
  </rsm:ExchangedDocument>

  <!-- Transaction Details -->
  <rsm:SupplyChainTradeTransaction>
    <!-- Line Items -->
    <ram:IncludedSupplyChainTradeLineItem>...</ram:IncludedSupplyChainTradeLineItem>

    <!-- Parties (Seller/Buyer) -->
    <ram:ApplicableHeaderTradeAgreement>
      <ram:SellerTradeParty>
        <ram:Name>Acme GmbH</ram:Name>
        <ram:SpecifiedTaxRegistration>
          <ram:ID schemeID="VA">DE123456789</ram:ID>
        </ram:SpecifiedTaxRegistration>
      </ram:SellerTradeParty>
    </ram:ApplicableHeaderTradeAgreement>

    <!-- Delivery Info -->
    <ram:ApplicableHeaderTradeDelivery>...</ram:ApplicableHeaderTradeDelivery>

    <!-- Payment and Amounts -->
    <ram:ApplicableHeaderTradeSettlement>
      <ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>

      <!-- Tax Breakdown -->
      <ram:ApplicableTradeTax>
        <ram:CalculatedAmount>19.00</ram:CalculatedAmount>
        <ram:TypeCode>VAT</ram:TypeCode>
        <ram:BasisAmount>100.00</ram:BasisAmount>
        <ram:CategoryCode>S</ram:CategoryCode>
        <ram:RateApplicablePercent>19.00</ram:RateApplicablePercent>
      </ram:ApplicableTradeTax>

      <!-- Totals -->
      <ram:SpecifiedTradeSettlementHeaderMonetarySummation>
        <ram:LineTotalAmount>100.00</ram:LineTotalAmount>
        <ram:TaxBasisTotalAmount>100.00</ram:TaxBasisTotalAmount>
        <ram:TaxTotalAmount currencyID="EUR">19.00</ram:TaxTotalAmount>
        <ram:GrandTotalAmount>119.00</ram:GrandTotalAmount>
        <ram:DuePayableAmount>119.00</ram:DuePayableAmount>
      </ram:SpecifiedTradeSettlementHeaderMonetarySummation>

      <!-- Payment Terms -->
      <ram:SpecifiedTradePaymentTerms>
        <ram:DueDateDateTime>
          <udt:DateTimeString format="102">20240215</udt:DateTimeString>
        </ram:DueDateDateTime>
      </ram:SpecifiedTradePaymentTerms>
    </ram:ApplicableHeaderTradeSettlement>
  </rsm:SupplyChainTradeTransaction>
</rsm:CrossIndustryInvoice>
```

## Field Mapping to BuchISY Meta Structure

| BuchISY Field | XRechnung/ZUGFeRD XML Path | Notes |
|---------------|----------------------------|-------|
| `Rechnungsnummer` | `/rsm:CrossIndustryInvoice/rsm:ExchangedDocument/ram:ID` | Invoice number |
| `Rechnungsdatum` | `/rsm:ExchangedDocument/ram:IssueDateTime/udt:DateTimeString` | Format: YYYYMMDD → convert to DD.MM.YYYY |
| `Firmenname` | `/rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeAgreement/ram:SellerTradeParty/ram:Name` | Seller name |
| `BetragNetto` | `/ram:ApplicableHeaderTradeSettlement/ram:SpecifiedTradeSettlementHeaderMonetarySummation/ram:TaxBasisTotalAmount` | Net amount (sum of tax basis) |
| `Bruttobetrag` | `/ram:SpecifiedTradeSettlementHeaderMonetarySummation/ram:GrandTotalAmount` | Gross total including tax |
| `SteuersatzProzent` | `/ram:ApplicableTradeTax/ram:RateApplicablePercent` | VAT rate (e.g., 19.00) |
| `SteuersatzBetrag` | `/ram:ApplicableTradeTax/ram:CalculatedAmount` | Tax amount |
| `Waehrung` | `/ram:InvoiceCurrencyCode` | Currency code (EUR, USD, etc.) |
| `Bezahldatum` | `/ram:SpecifiedTradePaymentTerms/ram:DueDateDateTime/udt:DateTimeString` | Payment due date (optional) |

**Additional Available Data** (not currently used but available):
- Buyer information
- Line items with quantities and unit prices
- Payment instructions (bank account, IBAN)
- Delivery address
- Tax ID / VAT registration numbers
- Notes and references

## Implementation Strategy

### 1. Detection Logic

```
┌─────────────────────────────────────┐
│  PDF File Selected                  │
└──────────┬──────────────────────────┘
           │
           ▼
┌─────────────────────────────────────┐
│  Check for XRechnung/ZUGFeRD        │
│  - Look for XML attachments          │
│  - Check filenames: factur-x.xml,   │
│    zugferd-invoice.xml, etc.        │
└──────────┬──────────────────────────┘
           │
      ┌────┴────┐
      │ Found?  │
      └────┬────┘
           │
    ┌──────┴──────┐
    │ YES         │ NO
    │             │
    ▼             ▼
┌────────┐   ┌──────────────┐
│ Extract│   │ Fall back to │
│ XML &  │   │ text/vision  │
│ Parse  │   │ extraction   │
└────┬───┘   └──────────────┘
     │
     ▼
┌────────────────────┐
│ Map to Meta struct │
│ Confidence = 1.0   │
└────────────────────┘
```

### 2. Implementation Details

**Chosen Approach**: Custom implementation using **pdfcpu** + **encoding/xml**

#### PDF Attachment Extraction
We use **github.com/pdfcpu/pdfcpu** (v0.11.0) to extract embedded XML files from PDF/A-3 documents:

- **Library**: Pure Go PDF processor with excellent PDF/A-3 support
- **License**: Apache-2.0 (open source, production-ready)
- **API**: Simple `api.Attachments()` function extracts all embedded files
- **Performance**: Fast, efficient, no external dependencies beyond Go stdlib

#### XML Parsing
Standard library **encoding/xml** for parsing CII (Cross Industry Invoice) XML:

- **Approach**: Custom struct definitions matching CII schema
- **Namespaces**: Handled transparently by xml.Unmarshal
- **Maintenance**: XML structure is stable (UN/CEFACT standard)

#### Detection Strategy
The extractor searches for XML attachments in this order:

1. **Known filenames**: factur-x.xml, zugferd-invoice.xml, xrechnung.xml, ZUGFeRD-invoice.xml
2. **Fallback**: Any .xml file attachment
3. **Content validation**: XML content checked for format indicators (urn:cen.eu:en16931, urn:ferd:, etc.)

### 3. Integration Points

**File**: `internal/core/einvoice.go` (new)
```go
type EInvoiceExtractor struct {
    logger *logging.Logger
}

// DetectFormat checks if PDF contains XRechnung/ZUGFeRD data
func (e *EInvoiceExtractor) DetectFormat(pdfPath string) (string, bool)

// Extract extracts metadata from XRechnung/ZUGFeRD XML
func (e *EInvoiceExtractor) Extract(pdfPath string) (Meta, float64, error)
```

**File**: `internal/ui/app.go` (modify)
```go
func (a *App) extractPDFData(ctx context.Context, path string) (core.Meta, error) {
    // 1. NEW: Try XRechnung/ZUGFeRD first
    if format, ok := a.eInvoiceExtractor.DetectFormat(path); ok {
        a.logger.Info("Detected %s format, using structured data extraction", format)
        meta, confidence, err := a.eInvoiceExtractor.Extract(path)
        if err == nil {
            return meta, nil
        }
        a.logger.Warn("E-invoice extraction failed: %v, falling back to text", err)
    }

    // 2. EXISTING: Extract text and use Claude/local
    text, err := a.pdfExtractor.ExtractText(path)
    // ... rest of existing logic
}
```

### 4. Code Structure

```
internal/core/
└── einvoice.go          # XRechnung/ZUGFeRD extractor (implemented)
                         # - EInvoiceExtractor struct
                         # - DetectFormat() - format detection
                         # - Extract() - metadata extraction
                         # - extractXMLAttachment() - PDF attachment extraction
                         # - CII XML struct definitions
                         # - mapToMeta() - XML to Meta mapping

testdata/                # TODO: Add test files
├── xrechnung_sample.pdf
├── zugferd_basic.pdf
└── zugferd_comfort.pdf
```

## Testing Strategy

### Test Cases

1. **XRechnung Detection**
   - PDF with embedded xrechnung.xml → Detected ✓
   - PDF with factur-x.xml → Detected ✓
   - Regular PDF without XML → Not detected, fallback works ✓

2. **Data Extraction Accuracy**
   - Compare extracted data against known values
   - Test with various XRechnung profiles
   - Test with ZUGFeRD 1.0, 2.0, 2.1, 2.2

3. **Edge Cases**
   - Multiple tax rates (split invoices)
   - Foreign currency invoices
   - Credit notes (negative amounts)
   - Partial payments

4. **Performance**
   - Measure extraction time vs. text extraction
   - Measure extraction time vs. Claude API
   - Expected: < 50ms for XML extraction

### Sample Files

Obtain test files from:
- https://www.xrechnung.de/downloads - Official XRechnung samples
- https://www.ferd-net.de/standards/zugferd-2.2/ - ZUGFeRD samples
- Generate own test invoices using free tools

## Benefits

### For Users
- ✅ **Perfect Accuracy**: No OCR errors, no AI hallucinations
- ✅ **Instant Processing**: No API latency (< 50ms vs. 2-5s for Claude)
- ✅ **Cost Savings**: No Claude API usage for e-invoices
- ✅ **Future-Proof**: German B2G mandatory, B2B increasing adoption

### For System
- ✅ **Lower Complexity**: No text parsing heuristics needed
- ✅ **Better Data Quality**: Structured data with validation
- ✅ **Reduced Load**: Fewer Claude API calls
- ✅ **Compliance**: Ready for mandatory e-invoicing requirements

## Adoption Timeline in Germany

- **2020**: XRechnung mandatory for B2G (government invoices)
- **2025**: E-invoicing becomes mandatory for B2B transactions (planned)
- **Current**: ZUGFeRD widely adopted voluntarily in B2B sector
- **Trend**: Increasing adoption, especially among larger companies

## Future Enhancements

1. **Validate Against Schema**: Add XML schema validation
2. **Support Line Items**: Extract individual invoice lines for detailed analysis
3. **Export as E-Invoice**: Generate XRechnung/ZUGFeRD from CSV data
4. **Bank Account Extraction**: Use IBAN from XML for payment setup
5. **Multi-Currency Support**: Handle foreign currency with exchange rates
6. **Batch Processing Indicator**: Show e-invoice badge in UI

## References

### Specifications
- [XRechnung Standard](https://www.xrechnung.de/)
- [ZUGFeRD Specification](https://www.ferd-net.de/standards/)
- [EN 16931 (EU Standard)](https://ec.europa.eu/digital-building-blocks/wikis/display/DIGITAL/Obtaining+a+copy+of+the+European+standard+on+eInvoicing)
- [CII (Cross Industry Invoice)](https://unece.org/trade/uncefact/xml-schemas)

### Tools and Validators
- [XRechnung Validator](https://www.itzbund.de/SharedDocs/Downloads/DE/E-Rechnung/KoSIT/xrechnung-testsuite.html)
- [ZUGFeRD Validator](https://www.ferd-net.de/front_content.php?idcat=231&lang=2)
- [Mustangproject](https://www.mustangproject.org/) - Open source Java library

### German E-Invoicing Law
- [Wachstumschancengesetz](https://www.bundesfinanzministerium.de/Content/DE/Gesetzestexte/Gesetze_Gesetzesvorhaben/Abteilungen/Abteilung_IV/20_Legislaturperiode/2023-08-30-Wachstumschancengesetz/0-Gesetz.html) - Growth Opportunities Act mandating B2B e-invoicing

## Implementation Checklist

- [x] Create `internal/core/einvoice.go` with detection logic
- [x] Implement XML extraction from PDF attachments (using pdfcpu)
- [x] Parse CII XML and map to Meta structure
- [x] Add date format conversion (YYYYMMDD → DD.MM.YYYY)
- [x] Integrate into `app.go` extraction pipeline
- [x] Add logging for format detection
- [ ] Write unit tests with sample files
- [ ] Test fallback to text/vision extraction
- [ ] Update UI to show e-invoice indicator (optional)
- [ ] Performance benchmark vs. existing methods
- [ ] Documentation in user manual
