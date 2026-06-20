package core

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// DefaultCSVColumns defines the default column order for the CSV file.
var DefaultCSVColumns = []string{
	"Dateiname",
	"Rechnungsdatum",
	"Jahr",
	"Monat",
	"Auftraggeber",
	"Verwendungszweck",
	"Rechnungsnummer",
	"VATID",
	"BetragNetto",
	"Steuersatz_Prozent",
	"Steuersatz_Betrag",
	"Bruttobetrag",
	"Waehrung",
	"Gegenkonto",
	"Bankkonto",
	"Bezahldatum",
	"Teilzahlung",
	"Kommentar",
	"BetragNetto_EUR",
	"Gebuehr",
	"HatAnhaenge",
	"AnzahlAnhaenge",
	"Unterordner",
	"BuchungRef",
	"Trinkgeld",
	"Steuerzeilen",
}

// ColumnDisplayNames maps column IDs to German display names.
var ColumnDisplayNames = map[string]string{
	"Dateiname":          "Dateiname",
	"Rechnungsdatum":     "Rechnungsdatum",
	"Jahr":               "Jahr",
	"Monat":              "Monat",
	"Auftraggeber":       "Auftraggeber",
	"Verwendungszweck":   "Verwendungszweck",
	"Rechnungsnummer":    "Rechnungsnummer",
	"VATID":              "USt-IdNr.",
	"BetragNetto":        "Betrag Netto",
	"Steuersatz_Prozent": "Steuersatz %",
	"Steuersatz_Betrag":  "Steuerbetrag",
	"Bruttobetrag":       "Bruttobetrag",
	"Waehrung":           "Währung",
	"Gegenkonto":         "Gegenkonto",
	"Bankkonto":          "Zahlungskonto",
	"Bezahldatum":        "Bezahldatum",
	"Teilzahlung":        "Teilzahlung",
	"Kommentar":          "Kommentar",
	"BetragNetto_EUR":    "Betrag Netto (EUR)",
	"Gebuehr":            "Gebühr",
	"HatAnhaenge":        "Anhänge",
	"AnzahlAnhaenge":     "Anzahl Anhänge",
	"Unterordner":        "Unterordner",
	"BuchungRef":         "Buchungs-Ref",
	"Trinkgeld":          "Trinkgeld",
	"Steuerzeilen":       "Steuerzeilen (Detail)",
}

// ColumnTranslationKeys maps column IDs to translation keys.
var ColumnTranslationKeys = map[string]string{
	"Dateiname":          "table.col.filename",
	"Rechnungsdatum":     "table.col.date",
	"Jahr":               "table.col.year",
	"Monat":              "table.col.month",
	"Auftraggeber":       "table.col.auftraggeber",
	"Verwendungszweck":   "table.col.verwendungszweck",
	"Rechnungsnummer":    "table.col.invoicenumber",
	"VATID":              "table.col.vatid",
	"BetragNetto":        "table.col.net",
	"Steuersatz_Prozent": "table.col.vatPercent",
	"Steuersatz_Betrag":  "table.col.vatAmount",
	"Bruttobetrag":       "table.col.gross",
	"Waehrung":           "table.col.currency",
	"Gegenkonto":         "table.col.account",
	"Bankkonto":          "table.col.bankaccount",
	"Bezahldatum":        "table.col.paymentdate",
	"Teilzahlung":        "table.col.partialpayment",
	"Kommentar":          "table.col.comment",
	"BetragNetto_EUR":    "table.col.net_eur",
	"Gebuehr":            "table.col.fee",
	"HatAnhaenge":        "table.col.hasattachments",
	"AnzahlAnhaenge":     "table.col.attachmentcount",
	"Unterordner":        "table.col.unterordner",
	"BuchungRef":         "table.col.buchungref",
	"Trinkgeld":          "table.col.trinkgeld",
	"Steuerzeilen":       "table.col.taxlines",
}

var validColumns = func() map[string]struct{} {
	m := make(map[string]struct{}, len(DefaultCSVColumns)+2)
	for _, col := range DefaultCSVColumns {
		m[col] = struct{}{}
	}
	// Add old column names for backward compatibility
	m["Firmenname"] = struct{}{}
	m["Kurzbezeichnung"] = struct{}{}
	return m
}()

// CSVRepository manages reading and writing invoice CSV files.
type CSVRepository struct {
	columnOrder      []string // Custom column order
	separator        rune     // CSV separator
	encoding         string   // CSV encoding
	decimalSeparator string   // Decimal separator for amounts
}

// NewCSVRepository creates a new CSV repository.
func NewCSVRepository() *CSVRepository {
	return &CSVRepository{
		columnOrder:      DefaultCSVColumns,
		separator:        ',',
		encoding:         "ISO-8859-1",
		decimalSeparator: ",",
	}
}

// SetColumnOrder sets the column order for CSV operations. Any column
// from DefaultCSVColumns missing from order is appended, so a legacy
// saved order still includes columns added in newer versions.
func (r *CSVRepository) SetColumnOrder(order []string) {
	if len(order) == 0 {
		r.columnOrder = append([]string{}, DefaultCSVColumns...)
		return
	}
	result := append([]string{}, order...)
	present := make(map[string]struct{}, len(result))
	for _, c := range result {
		present[c] = struct{}{}
	}
	for _, c := range DefaultCSVColumns {
		if _, ok := present[c]; !ok {
			result = append(result, c)
		}
	}
	r.columnOrder = result
}

// SetSeparator sets the CSV field separator.
func (r *CSVRepository) SetSeparator(sep string) {
	switch sep {
	case ";":
		r.separator = ';'
	case "\t", "\\t":
		r.separator = '\t'
	default:
		r.separator = ','
	}
}

// SetEncoding sets the CSV file encoding.
func (r *CSVRepository) SetEncoding(enc string) {
	r.encoding = enc
}

// SetDecimalSeparator sets the decimal separator for amounts.
func (r *CSVRepository) SetDecimalSeparator(sep string) {
	r.decimalSeparator = sep
}

// GetHeader returns the header based on current column order.
func (r *CSVRepository) GetHeader() []string {
	if len(r.columnOrder) > 0 {
		return append([]string{}, r.columnOrder...)
	}
	return DefaultCSVColumns
}

// Load reads all rows from a CSV file.
func (r *CSVRepository) Load(path string) ([]CSVRow, error) {
	// If file doesn't exist, return empty
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []CSVRow{}, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Create reader with encoding transformation
	var reader *csv.Reader
	if r.encoding == "ISO-8859-1" {
		// Decode from ISO-8859-1 to UTF-8
		decoder := charmap.ISO8859_1.NewDecoder()
		transformedReader := transform.NewReader(file, decoder)
		reader = csv.NewReader(transformedReader)
	} else {
		// UTF-8 (default)
		reader = csv.NewReader(file)
	}

	// Set separator
	reader.Comma = r.separator
	reader.LazyQuotes = true // Be lenient with quotes
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	if len(records) == 0 {
		return []CSVRow{}, nil
	}

	headerMap, start := parseHeader(records[0])

	var rows []CSVRow
	for i := start; i < len(records); i++ {
		record := records[i]
		if len(record) == 0 {
			continue
		}

		// Read with backward compatibility for old column names
		auftraggeber := valueForColumn(record, headerMap, "Auftraggeber")
		if auftraggeber == "" {
			// Fallback to old column name
			auftraggeber = valueForColumn(record, headerMap, "Firmenname")
		}

		verwendungszweck := valueForColumn(record, headerMap, "Verwendungszweck")
		if verwendungszweck == "" {
			// Fallback to old column name
			verwendungszweck = valueForColumn(record, headerMap, "Kurzbezeichnung")
		}

		// VATID and the former separate "UStIdNr" column are the same concept
		// (the issuer's VAT-ID); read the legacy column when VATID is empty.
		vatID := valueForColumn(record, headerMap, "VATID")
		if vatID == "" {
			vatID = valueForColumn(record, headerMap, "UStIdNr")
		}

		row := CSVRow{
			Dateiname:         valueForColumn(record, headerMap, "Dateiname"),
			Rechnungsdatum:    valueForColumn(record, headerMap, "Rechnungsdatum"),
			Jahr:              valueForColumn(record, headerMap, "Jahr"),
			Monat:             valueForColumn(record, headerMap, "Monat"),
			Auftraggeber:      auftraggeber,
			Verwendungszweck:  verwendungszweck,
			Rechnungsnummer:   valueForColumn(record, headerMap, "Rechnungsnummer"),
			VATID:             vatID,
			BetragNetto:       parseFloat(valueForColumn(record, headerMap, "BetragNetto")),
			SteuersatzProzent: parseFloat(valueForColumn(record, headerMap, "Steuersatz_Prozent")),
			SteuersatzBetrag:  parseFloat(valueForColumn(record, headerMap, "Steuersatz_Betrag")),
			Bruttobetrag:      parseFloat(valueForColumn(record, headerMap, "Bruttobetrag")),
			Waehrung:          valueForColumn(record, headerMap, "Waehrung"),
			Gegenkonto:        parseInt(valueForColumn(record, headerMap, "Gegenkonto")),
			Bankkonto:         valueForColumn(record, headerMap, "Bankkonto"),
			Bezahldatum:       valueForColumn(record, headerMap, "Bezahldatum"),
			Teilzahlung:       parseBool(valueForColumn(record, headerMap, "Teilzahlung")),
			Kommentar:         valueForColumn(record, headerMap, "Kommentar"),
			BetragNetto_EUR:   parseFloat(valueForColumn(record, headerMap, "BetragNetto_EUR")),
			Gebuehr:           parseFloat(valueForColumn(record, headerMap, "Gebuehr")),
			HatAnhaenge:       parseBool(valueForColumn(record, headerMap, "HatAnhaenge")),
			AnzahlAnhaenge:    parseInt(valueForColumn(record, headerMap, "AnzahlAnhaenge")),
			Unterordner:       valueForColumn(record, headerMap, "Unterordner"),
			BuchungRef:        valueForColumn(record, headerMap, "BuchungRef"),
		}
		// Backfill HatAnhaenge from AnzahlAnhaenge for rows written before the
		// HatAnhaenge column existed (legacy CSVs where only the count was stored).
		if !row.HatAnhaenge && row.AnzahlAnhaenge > 0 {
			row.HatAnhaenge = true
		}
		row.Trinkgeld = parseFloat(valueForColumn(record, headerMap, "Trinkgeld"))
		row.TaxLines = ParseTaxLines(valueForColumn(record, headerMap, "Steuerzeilen"))
		if len(row.TaxLines) == 0 {
			// Legacy row without detail: reconstruct one line from aggregates.
			// Pass brutto as the 4th arg so gross-only rows still get a usable line.
			row.TaxLines = ReconstructTaxLines(row.BetragNetto, row.SteuersatzProzent, row.SteuersatzBetrag, row.Bruttobetrag)
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// Append adds a new row to the CSV file, creating it with a header if necessary.
func (r *CSVRepository) Append(path string, row CSVRow) error {
	// Check if file exists
	fileExists := false
	if _, err := os.Stat(path); err == nil {
		fileExists = true

		matches, err := r.headerMatches(path)
		if err != nil {
			return fmt.Errorf("failed to verify CSV header: %w", err)
		}
		if !matches {
			existingRows, err := r.Load(path)
			if err != nil {
				return fmt.Errorf("failed to load CSV for reordering: %w", err)
			}
			if err := r.Rewrite(path, existingRows); err != nil {
				return fmt.Errorf("failed to reorder CSV: %w", err)
			}
		}
	}

	// Open file for appending with encoding
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open CSV: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Create write target with encoding transformation
	var writeTarget io.Writer = file
	if r.encoding == "ISO-8859-1" {
		encoder := charmap.ISO8859_1.NewEncoder()
		writeTarget = transform.NewWriter(file, encoder)
	}

	// Write header if new file (with quotes)
	if !fileExists {
		header := r.GetHeader()
		quotedHeader := make([]string, len(header))
		for i, field := range header {
			escaped := strings.ReplaceAll(field, "\"", "\"\"")
			quotedHeader[i] = "\"" + escaped + "\""
		}
		headerLine := strings.Join(quotedHeader, string(r.separator)) + "\n"
		if _, err := writeTarget.Write([]byte(headerLine)); err != nil {
			return fmt.Errorf("failed to write CSV header: %w", err)
		}
	}

	// Build row in the configured column order
	record := r.rowToRecord(row)

	// Wrap all fields in quotes (force quoting)
	quotedRecord := make([]string, len(record))
	for i, field := range record {
		// Escape existing quotes by doubling them (CSV standard)
		escaped := strings.ReplaceAll(field, "\"", "\"\"")
		// Wrap in quotes
		quotedRecord[i] = "\"" + escaped + "\""
	}

	// Join with separator and write
	line := strings.Join(quotedRecord, string(r.separator)) + "\n"
	if _, err := writeTarget.Write([]byte(line)); err != nil {
		return fmt.Errorf("failed to write CSV row: %w", err)
	}

	return nil
}

// WriteTo writes the header and all rows as CSV to w, using the current
// column order. Honors the configured encoding (ISO-8859-1 by default) and
// quotes every field, matching the on-disk invoices.csv format.
func (r *CSVRepository) WriteTo(w io.Writer, rows []CSVRow) error {
	// Determine write target based on encoding
	var writeTarget io.Writer = w
	if r.encoding == "ISO-8859-1" {
		encoder := charmap.ISO8859_1.NewEncoder()
		writeTarget = transform.NewWriter(w, encoder)
	}

	// Write header with quotes
	header := r.GetHeader()
	quotedHeader := make([]string, len(header))
	for i, field := range header {
		escaped := strings.ReplaceAll(field, "\"", "\"\"")
		quotedHeader[i] = "\"" + escaped + "\""
	}
	headerLine := strings.Join(quotedHeader, string(r.separator)) + "\n"
	if _, err := writeTarget.Write([]byte(headerLine)); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write rows with quotes
	for _, row := range rows {
		record := r.rowToRecord(row)
		quotedRecord := make([]string, len(record))
		for i, field := range record {
			escaped := strings.ReplaceAll(field, "\"", "\"\"")
			quotedRecord[i] = "\"" + escaped + "\""
		}
		line := strings.Join(quotedRecord, string(r.separator)) + "\n"
		if _, err := writeTarget.Write([]byte(line)); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}
	return nil
}

// Rewrite overwrites the CSV file with the provided rows using the current column order.
func (r *CSVRepository) Rewrite(path string, rows []CSVRow) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to recreate CSV: %w", err)
	}
	defer func() { _ = file.Close() }()
	return r.WriteTo(file, rows)
}

// parseFloat parses a float from a string, returning 0 on error.
// Accepts both comma and dot as decimal separator.
func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	// Replace comma with dot for parsing
	s = strings.Replace(s, ",", ".", 1)
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// parseInt parses an int from a string, returning 0 on error.
func parseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

// parseBool parses a bool from a string, returning false on error.
func parseBool(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
}

// formatFloat formats a float64 as a string with 2 decimal places using the configured decimal separator.
func (r *CSVRepository) formatFloat(f float64) string {
	formatted := fmt.Sprintf("%.2f", f)
	// Replace dot with configured decimal separator
	if r.decimalSeparator == "," {
		formatted = strings.Replace(formatted, ".", ",", 1)
	}
	return formatted
}

// rowToRecord converts a CSVRow to a string slice based on column order.
func (r *CSVRepository) rowToRecord(row CSVRow) []string {
	// Map of column name to value
	valueMap := map[string]string{
		"Dateiname":          row.Dateiname,
		"Rechnungsdatum":     row.Rechnungsdatum,
		"Jahr":               row.Jahr,
		"Monat":              row.Monat,
		"Auftraggeber":       row.Auftraggeber,
		"Verwendungszweck":   row.Verwendungszweck,
		"Rechnungsnummer":    row.Rechnungsnummer,
		"VATID":              row.VATID,
		"BetragNetto":        r.formatFloat(row.BetragNetto),
		"Steuersatz_Prozent": r.formatFloat(row.SteuersatzProzent),
		"Steuersatz_Betrag":  r.formatFloat(row.SteuersatzBetrag),
		"Bruttobetrag":       r.formatFloat(row.Bruttobetrag),
		"Waehrung":           row.Waehrung,
		"Gegenkonto":         strconv.Itoa(row.Gegenkonto),
		"Bankkonto":          row.Bankkonto,
		"Bezahldatum":        row.Bezahldatum,
		"Teilzahlung":        formatBool(row.Teilzahlung),
		"Kommentar":          row.Kommentar,
		"BetragNetto_EUR":    r.formatFloat(row.BetragNetto_EUR),
		"Gebuehr":            r.formatFloat(row.Gebuehr),
		"HatAnhaenge":        formatBool(row.HatAnhaenge),
		"AnzahlAnhaenge":     strconv.Itoa(row.AnzahlAnhaenge),
		"Unterordner":        row.Unterordner,
		"BuchungRef":         row.BuchungRef,
		"Trinkgeld":          r.formatFloat(row.Trinkgeld),
		"Steuerzeilen":       MarshalTaxLines(row.TaxLines),
	}

	// Build record in configured order
	header := r.GetHeader()
	record := make([]string, len(header))
	for i, col := range header {
		if val, ok := valueMap[col]; ok {
			record[i] = val
		} else {
			record[i] = ""
		}
	}

	return record
}

// formatBool formats a bool as a string ("true" or "false").
func formatBool(b bool) string {
	return strconv.FormatBool(b)
}

func (r *CSVRepository) headerMatches(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("failed to open CSV for header check: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return true, nil
		}
		return false, fmt.Errorf("failed to read CSV header: %w", err)
	}

	expected := r.GetHeader()
	if len(header) != len(expected) {
		return false, nil
	}

	for idx, val := range header {
		if val != expected[idx] {
			return false, nil
		}
	}

	return true, nil
}

func parseHeader(header []string) (map[string]int, int) {
	if len(header) == 0 {
		return defaultHeaderMap(), 0
	}

	headerMap := make(map[string]int)
	validCount := 0

	for idx, col := range header {
		col = strings.TrimSpace(col)
		if _, ok := validColumns[col]; ok {
			headerMap[col] = idx
			validCount++
		}
	}

	if validCount == 0 {
		return defaultHeaderMap(), 0
	}

	// Ensure all known columns exist in map even if missing from header.
	for col := range validColumns {
		if _, ok := headerMap[col]; !ok {
			headerMap[col] = -1
		}
	}

	return headerMap, 1
}

func defaultHeaderMap() map[string]int {
	m := make(map[string]int, len(DefaultCSVColumns))
	for idx, col := range DefaultCSVColumns {
		m[col] = idx
	}
	return m
}

func valueForColumn(record []string, header map[string]int, col string) string {
	if header == nil {
		return ""
	}
	idx, ok := header[col]
	if !ok {
		return ""
	}
	if idx < 0 || idx >= len(record) {
		return ""
	}
	return record[idx]
}
