package core

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// EInvoiceFormat represents the detected e-invoice format.
type EInvoiceFormat string

const (
	FormatXRechnung EInvoiceFormat = "XRechnung"
	FormatZUGFeRD   EInvoiceFormat = "ZUGFeRD"
	FormatNone      EInvoiceFormat = ""
)

// EInvoiceExtractor extracts structured data from XRechnung and ZUGFeRD PDFs.
type EInvoiceExtractor struct{}

// NewEInvoiceExtractor creates a new e-invoice extractor.
func NewEInvoiceExtractor() *EInvoiceExtractor {
	return &EInvoiceExtractor{}
}

// DetectFormat checks if a PDF contains XRechnung or ZUGFeRD data.
// Returns the format type and true if detected, empty string and false otherwise.
func (e *EInvoiceExtractor) DetectFormat(pdfPath string) (EInvoiceFormat, bool) {
	xmlData, attachmentName, err := e.extractXMLAttachment(pdfPath)
	if err != nil || xmlData == nil {
		return FormatNone, false
	}

	// Check filename to determine format
	lowerName := strings.ToLower(attachmentName)
	if strings.Contains(lowerName, "factur-x") || strings.Contains(lowerName, "zugferd") {
		return FormatZUGFeRD, true
	}
	if strings.Contains(lowerName, "xrechnung") {
		return FormatXRechnung, true
	}

	// Check XML content for format indicators
	xmlContent := string(xmlData)
	if strings.Contains(xmlContent, "xrechnung") || strings.Contains(xmlContent, "urn:cen.eu:en16931") {
		return FormatXRechnung, true
	}
	if strings.Contains(xmlContent, "zugferd") || strings.Contains(xmlContent, "urn:ferd:") {
		return FormatZUGFeRD, true
	}

	// If we found XML but can't determine format, assume ZUGFeRD (more common)
	return FormatZUGFeRD, true
}

// Extract extracts invoice metadata from XRechnung/ZUGFeRD XML.
// Returns Meta with confidence 1.0 (perfect accuracy for structured data).
func (e *EInvoiceExtractor) Extract(pdfPath string) (Meta, float64, error) {
	// Extract XML attachment
	xmlData, _, err := e.extractXMLAttachment(pdfPath)
	if err != nil {
		return Meta{}, 0, fmt.Errorf("failed to extract XML: %w", err)
	}
	if xmlData == nil {
		return Meta{}, 0, fmt.Errorf("no XML attachment found")
	}

	// Parse XML
	invoice, err := e.parseXML(xmlData)
	if err != nil {
		return Meta{}, 0, fmt.Errorf("failed to parse XML: %w", err)
	}

	// Map to Meta structure
	meta := e.mapToMeta(invoice)

	// Confidence is always 1.0 for structured data
	return meta, 1.0, nil
}

// extractXMLAttachment extracts the first XML attachment from a PDF.
// Returns the XML data, attachment filename, and error.
// Uses pdfcpu library to extract embedded files from PDF/A-3 documents.
func (e *EInvoiceExtractor) extractXMLAttachment(pdfPath string) ([]byte, string, error) {
	// Create temporary directory for extraction
	tempDir, err := os.MkdirTemp("", "einvoice-*")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir) // Clean up temp directory

	// Extract all attachments to temp directory
	// nil = extract all attachments, nil = use default configuration
	err = api.ExtractAttachmentsFile(pdfPath, tempDir, nil, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to extract attachments: %w", err)
	}

	// Look for XML files in the temp directory
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read temp dir: %w", err)
	}

	if len(files) == 0 {
		return nil, "", fmt.Errorf("no attachments found in PDF")
	}

	// Common XRechnung/ZUGFeRD XML filenames
	commonNames := []string{
		"factur-x.xml",
		"zugferd-invoice.xml",
		"xrechnung.xml",
		"zugferd-invoice.xml",
	}

	// First, try to find files with known names
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		lowerName := strings.ToLower(file.Name())
		for _, commonName := range commonNames {
			if strings.Contains(lowerName, strings.ToLower(commonName)) {
				xmlPath := filepath.Join(tempDir, file.Name())
				data, err := os.ReadFile(xmlPath)
				if err != nil {
					return nil, "", fmt.Errorf("failed to read attachment %s: %w", file.Name(), err)
				}
				return data, file.Name(), nil
			}
		}
	}

	// If no known name found, look for any .xml file
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(file.Name()), ".xml") {
			xmlPath := filepath.Join(tempDir, file.Name())
			data, err := os.ReadFile(xmlPath)
			if err != nil {
				return nil, "", fmt.Errorf("failed to read attachment %s: %w", file.Name(), err)
			}
			return data, file.Name(), nil
		}
	}

	return nil, "", fmt.Errorf("no XML attachment found in PDF")
}

// CrossIndustryInvoice represents the root element of CII XML.
type CrossIndustryInvoice struct {
	XMLName              xml.Name             `xml:"CrossIndustryInvoice"`
	ExchangedDocument    ExchangedDocument    `xml:"ExchangedDocument"`
	SupplyChainTradeTransaction SupplyChainTradeTransaction `xml:"SupplyChainTradeTransaction"`
}

// ExchangedDocument contains document-level information.
type ExchangedDocument struct {
	ID            string        `xml:"ID"`
	IssueDateTime IssueDateTime `xml:"IssueDateTime"`
}

// IssueDateTime represents the invoice date.
type IssueDateTime struct {
	DateTimeString DateTimeString `xml:"DateTimeString"`
}

// DateTimeString represents a date in YYYYMMDD format.
type DateTimeString struct {
	Format string `xml:"format,attr"`
	Value  string `xml:",chardata"`
}

// SupplyChainTradeTransaction contains the main transaction details.
type SupplyChainTradeTransaction struct {
	ApplicableHeaderTradeAgreement  HeaderTradeAgreement  `xml:"ApplicableHeaderTradeAgreement"`
	ApplicableHeaderTradeSettlement HeaderTradeSettlement `xml:"ApplicableHeaderTradeSettlement"`
}

// HeaderTradeAgreement contains party information.
type HeaderTradeAgreement struct {
	SellerTradeParty TradeParty `xml:"SellerTradeParty"`
}

// TradeParty represents a party (seller or buyer).
type TradeParty struct {
	Name string `xml:"Name"`
}

// HeaderTradeSettlement contains payment and amount information.
type HeaderTradeSettlement struct {
	InvoiceCurrencyCode                           string                    `xml:"InvoiceCurrencyCode"`
	ApplicableTradeTax                            []TradeTax                `xml:"ApplicableTradeTax"`
	SpecifiedTradePaymentTerms                    *TradePaymentTerms        `xml:"SpecifiedTradePaymentTerms"`
	SpecifiedTradeSettlementHeaderMonetarySummation MonetarySummation         `xml:"SpecifiedTradeSettlementHeaderMonetarySummation"`
}

// TradeTax represents tax information.
type TradeTax struct {
	CalculatedAmount      Amount `xml:"CalculatedAmount"`
	BasisAmount           Amount `xml:"BasisAmount"`
	RateApplicablePercent string `xml:"RateApplicablePercent"`
}

// Amount represents a monetary amount.
type Amount struct {
	Value string `xml:",chardata"`
}

// TradePaymentTerms contains payment terms.
type TradePaymentTerms struct {
	DueDateDateTime *IssueDateTime `xml:"DueDateDateTime"`
}

// MonetarySummation contains total amounts.
type MonetarySummation struct {
	LineTotalAmount    Amount `xml:"LineTotalAmount"`
	TaxBasisTotalAmount Amount `xml:"TaxBasisTotalAmount"`
	TaxTotalAmount     Amount `xml:"TaxTotalAmount"`
	GrandTotalAmount   Amount `xml:"GrandTotalAmount"`
}

// parseXML parses the CII XML into a structured format.
func (e *EInvoiceExtractor) parseXML(xmlData []byte) (*CrossIndustryInvoice, error) {
	var invoice CrossIndustryInvoice
	err := xml.Unmarshal(xmlData, &invoice)
	if err != nil {
		return nil, fmt.Errorf("XML unmarshal error: %w", err)
	}
	return &invoice, nil
}

// mapToMeta maps the parsed XML to our Meta structure.
func (e *EInvoiceExtractor) mapToMeta(invoice *CrossIndustryInvoice) Meta {
	meta := Meta{}

	// Invoice number
	meta.Rechnungsnummer = invoice.ExchangedDocument.ID

	// Invoice date (convert YYYYMMDD to DD.MM.YYYY)
	dateStr := invoice.ExchangedDocument.IssueDateTime.DateTimeString.Value
	meta.Rechnungsdatum = e.convertDateFormat(dateStr)

	// Extract year and month from date
	if len(dateStr) >= 6 {
		meta.Jahr = dateStr[0:4]   // YYYY
		meta.Monat = dateStr[4:6]  // MM
	}

	// Company name (seller)
	meta.Firmenname = invoice.SupplyChainTradeTransaction.ApplicableHeaderTradeAgreement.SellerTradeParty.Name

	// Currency
	meta.Waehrung = invoice.SupplyChainTradeTransaction.ApplicableHeaderTradeSettlement.InvoiceCurrencyCode

	// Amounts
	settlement := invoice.SupplyChainTradeTransaction.ApplicableHeaderTradeSettlement
	meta.BetragNetto = parseXMLAmount(settlement.SpecifiedTradeSettlementHeaderMonetarySummation.TaxBasisTotalAmount.Value)
	meta.Bruttobetrag = parseXMLAmount(settlement.SpecifiedTradeSettlementHeaderMonetarySummation.GrandTotalAmount.Value)

	// Tax information (use first tax entry)
	if len(settlement.ApplicableTradeTax) > 0 {
		tax := settlement.ApplicableTradeTax[0]
		meta.SteuersatzProzent = parseXMLAmount(tax.RateApplicablePercent)
		meta.SteuersatzBetrag = parseXMLAmount(tax.CalculatedAmount.Value)
	}

	// Payment due date (optional)
	if settlement.SpecifiedTradePaymentTerms != nil &&
		settlement.SpecifiedTradePaymentTerms.DueDateDateTime != nil {
		dueDateStr := settlement.SpecifiedTradePaymentTerms.DueDateDateTime.DateTimeString.Value
		meta.Bezahldatum = e.convertDateFormat(dueDateStr)
	}

	// Kurzbezeichnung: Create a short description from the invoice number
	// This can be customized based on needs
	meta.Kurzbezeichnung = fmt.Sprintf("Rechnung %s", meta.Rechnungsnummer)

	return meta
}

// convertDateFormat converts YYYYMMDD to DD.MM.YYYY.
func (e *EInvoiceExtractor) convertDateFormat(dateStr string) string {
	// Remove any spaces and validate length
	dateStr = strings.TrimSpace(dateStr)
	if len(dateStr) != 8 {
		return dateStr // Return as-is if not valid format
	}

	year := dateStr[0:4]
	month := dateStr[4:6]
	day := dateStr[6:8]

	return fmt.Sprintf("%s.%s.%s", day, month, year)
}

// parseXMLAmount converts a string amount to float64.
func parseXMLAmount(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", ".") // Handle European format
	var amount float64
	fmt.Sscanf(s, "%f", &amount)
	return amount
}
