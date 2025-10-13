package core

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// DefaultCSVColumns defines the default column order for the CSV file.
var DefaultCSVColumns = []string{
	"Dateiname",
	"Rechnungsdatum",
	"Jahr",
	"Monat",
	"Firmenname",
	"Kurzbezeichnung",
	"Rechnungsnummer",
	"BetragNetto",
	"Steuersatz_Prozent",
	"Steuersatz_Betrag",
	"Bruttobetrag",
	"Waehrung",
	"Gegenkonto",
	"Bankkonto",
	"Bezahldatum",
	"Teilzahlung",
}

// ColumnDisplayNames maps column IDs to German display names.
var ColumnDisplayNames = map[string]string{
	"Dateiname":          "Dateiname",
	"Rechnungsdatum":     "Rechnungsdatum",
	"Jahr":               "Jahr",
	"Monat":              "Monat",
	"Firmenname":         "Firmenname",
	"Kurzbezeichnung":    "Kurzbezeichnung",
	"Rechnungsnummer":    "Rechnungsnummer",
	"BetragNetto":        "Betrag Netto",
	"Steuersatz_Prozent": "Steuersatz %",
	"Steuersatz_Betrag":  "Steuerbetrag",
	"Bruttobetrag":       "Bruttobetrag",
	"Waehrung":           "Währung",
	"Gegenkonto":         "Gegenkonto",
	"Bankkonto":          "Bankkonto",
	"Bezahldatum":        "Bezahldatum",
	"Teilzahlung":        "Teilzahlung",
}

// ColumnTranslationKeys maps column IDs to translation keys.
var ColumnTranslationKeys = map[string]string{
	"Dateiname":          "table.col.filename",
	"Rechnungsdatum":     "table.col.date",
	"Jahr":               "table.col.year",
	"Monat":              "table.col.month",
	"Firmenname":         "table.col.company",
	"Kurzbezeichnung":    "table.col.shortdesc",
	"Rechnungsnummer":    "table.col.invoicenumber",
	"BetragNetto":        "table.col.net",
	"Steuersatz_Prozent": "table.col.vatPercent",
	"Steuersatz_Betrag":  "table.col.vatAmount",
	"Bruttobetrag":       "table.col.gross",
	"Waehrung":           "table.col.currency",
	"Gegenkonto":         "table.col.account",
	"Bankkonto":          "table.col.bankaccount",
	"Bezahldatum":        "table.col.paymentdate",
	"Teilzahlung":        "table.col.partialpayment",
}

var validColumns = func() map[string]struct{} {
	m := make(map[string]struct{}, len(DefaultCSVColumns))
	for _, col := range DefaultCSVColumns {
		m[col] = struct{}{}
	}
	return m
}()

// CSVRepository manages reading and writing invoice CSV files.
type CSVRepository struct {
	columnOrder []string // Custom column order
}

// NewCSVRepository creates a new CSV repository.
func NewCSVRepository() *CSVRepository {
	return &CSVRepository{
		columnOrder: DefaultCSVColumns,
	}
}

// SetColumnOrder sets the column order for CSV operations.
func (r *CSVRepository) SetColumnOrder(order []string) {
	if len(order) == 0 {
		r.columnOrder = append([]string{}, DefaultCSVColumns...)
		return
	}
	r.columnOrder = append([]string{}, order...)
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
	defer file.Close()

	reader := csv.NewReader(file)
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

		row := CSVRow{
			Dateiname:         valueForColumn(record, headerMap, "Dateiname"),
			Rechnungsdatum:    valueForColumn(record, headerMap, "Rechnungsdatum"),
			Jahr:              valueForColumn(record, headerMap, "Jahr"),
			Monat:             valueForColumn(record, headerMap, "Monat"),
			Firmenname:        valueForColumn(record, headerMap, "Firmenname"),
			Kurzbezeichnung:   valueForColumn(record, headerMap, "Kurzbezeichnung"),
			Rechnungsnummer:   valueForColumn(record, headerMap, "Rechnungsnummer"),
			BetragNetto:       parseFloat(valueForColumn(record, headerMap, "BetragNetto")),
			SteuersatzProzent: parseFloat(valueForColumn(record, headerMap, "Steuersatz_Prozent")),
			SteuersatzBetrag:  parseFloat(valueForColumn(record, headerMap, "Steuersatz_Betrag")),
			Bruttobetrag:      parseFloat(valueForColumn(record, headerMap, "Bruttobetrag")),
			Waehrung:          valueForColumn(record, headerMap, "Waehrung"),
			Gegenkonto:        parseInt(valueForColumn(record, headerMap, "Gegenkonto")),
			Bankkonto:         valueForColumn(record, headerMap, "Bankkonto"),
			Bezahldatum:       valueForColumn(record, headerMap, "Bezahldatum"),
			Teilzahlung:       parseBool(valueForColumn(record, headerMap, "Teilzahlung")),
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

	// Open file for appending
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open CSV: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header if new file
	if !fileExists {
		header := r.GetHeader()
		if err := writer.Write(header); err != nil {
			return fmt.Errorf("failed to write CSV header: %w", err)
		}
	}

	// Build row in the configured column order
	record := r.rowToRecord(row)

	if err := writer.Write(record); err != nil {
		return fmt.Errorf("failed to write CSV row: %w", err)
	}

	return nil
}

// Rewrite overwrites the CSV file with the provided rows using the current column order.
func (r *CSVRepository) Rewrite(path string, rows []CSVRow) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to recreate CSV: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write(r.GetHeader()); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	for _, row := range rows {
		if err := writer.Write(r.rowToRecord(row)); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	if err := writer.Error(); err != nil {
		return fmt.Errorf("failed to flush CSV writer: %w", err)
	}

	return nil
}

// parseFloat parses a float from a string, returning 0 on error.
func parseFloat(s string) float64 {
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

// formatFloat formats a float64 as a string with 2 decimal places.
// Always uses dot as decimal separator for CSV.
func formatFloat(f float64) string {
	return fmt.Sprintf("%.2f", f)
}

// rowToRecord converts a CSVRow to a string slice based on column order.
func (r *CSVRepository) rowToRecord(row CSVRow) []string {
	// Map of column name to value
	valueMap := map[string]string{
		"Dateiname":          row.Dateiname,
		"Rechnungsdatum":     row.Rechnungsdatum,
		"Jahr":               row.Jahr,
		"Monat":              row.Monat,
		"Firmenname":         row.Firmenname,
		"Kurzbezeichnung":    row.Kurzbezeichnung,
		"Rechnungsnummer":    row.Rechnungsnummer,
		"BetragNetto":        formatFloat(row.BetragNetto),
		"Steuersatz_Prozent": formatFloat(row.SteuersatzProzent),
		"Steuersatz_Betrag":  formatFloat(row.SteuersatzBetrag),
		"Bruttobetrag":       formatFloat(row.Bruttobetrag),
		"Waehrung":           row.Waehrung,
		"Gegenkonto":         strconv.Itoa(row.Gegenkonto),
		"Bankkonto":          row.Bankkonto,
		"Bezahldatum":        row.Bezahldatum,
		"Teilzahlung":        formatBool(row.Teilzahlung),
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
