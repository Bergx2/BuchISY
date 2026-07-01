package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CashDeposit is a single cash inflow into a cash register (Bar-Einlage).
type CashDeposit struct {
	Datum        string  `json:"datum"` // DD.MM.YYYY
	Beschreibung string  `json:"beschreibung"`
	Betrag       float64 `json:"betrag"`
}

// CashBook is the monthly cash book of one cash-register account.
type CashBook struct {
	Konto          string        `json:"konto"`
	Anfangsbestand float64       `json:"anfangsbestand"`
	Einlagen       []CashDeposit `json:"einlagen"`
}

// LoadCashBooks reads the cash books stored in a month folder's
// kassenbuch.json. A missing file yields an empty slice and no error.
func LoadCashBooks(path string) ([]CashBook, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []CashBook{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read cash book: %w", err)
	}
	var books []CashBook
	if err := json.Unmarshal(data, &books); err != nil {
		return nil, fmt.Errorf("failed to parse cash book: %w", err)
	}
	return books, nil
}

// SaveCashBooks writes the cash books to kassenbuch.json, creating the
// containing month folder if it does not exist yet.
func SaveCashBooks(path string, books []CashBook) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create cash book directory: %w", err)
	}
	data, err := json.MarshalIndent(books, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cash book: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write cash book: %w", err)
	}
	return nil
}

// CashEntry is one line of a computed cash report.
type CashEntry struct {
	Datum        string
	Beschreibung string
	Beleg        string // invoice Dateiname (links the row to its receipt)
	Belegnummer  string // sequential receipt number ("YYYY-NNNN"), empty for deposits
	Einnahme     float64
	Ausgabe      float64
	Saldo        float64
}

// ComputeCashReport combines the cash book's opening balance and deposits
// with the cash-paid invoices into a chronologically ordered running
// balance. invoices must already be filtered to this cash account.
// Invoices use Bezahldatum for ordering, falling back to Rechnungsdatum;
// entries with an unparseable date are sorted last.
func ComputeCashReport(book CashBook, invoices []CSVRow) ([]CashEntry, float64) {
	type dated struct {
		entry CashEntry
		t     time.Time
		ok    bool
	}
	items := make([]dated, 0, len(book.Einlagen)+len(invoices))

	for _, d := range book.Einlagen {
		t, ok := parseGermanDate(d.Datum)
		items = append(items, dated{
			entry: CashEntry{Datum: d.Datum, Beschreibung: d.Beschreibung, Einnahme: d.Betrag},
			t:     t, ok: ok,
		})
	}
	for _, inv := range invoices {
		dateStr := inv.Bezahldatum
		if dateStr == "" {
			dateStr = inv.Rechnungsdatum
		}
		t, ok := parseGermanDate(dateStr)
		items = append(items, dated{
			entry: CashEntry{Datum: dateStr, Beschreibung: inv.Auftraggeber, Beleg: inv.Dateiname, Belegnummer: inv.Belegnummer, Ausgabe: inv.Bruttobetrag},
			t:     t, ok: ok,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ok != items[j].ok {
			return items[i].ok // datable entries first
		}
		if !items[i].ok {
			return false
		}
		return items[i].t.Before(items[j].t)
	})

	saldo := book.Anfangsbestand
	entries := make([]CashEntry, len(items))
	for i, it := range items {
		saldo += it.entry.Einnahme - it.entry.Ausgabe
		e := it.entry
		e.Saldo = saldo
		entries[i] = e
	}
	return entries, saldo
}

// CashCoverage runs the cash report and reports which cash-paid invoices are
// booked while the running cash balance is negative (i.e. not covered by
// available cash), plus the closing balance. The map key is the invoice
// Dateiname. invoices must already be filtered to this cash account.
func CashCoverage(book CashBook, invoices []CSVRow) (uncovered map[string]bool, closing float64) {
	entries, closing := ComputeCashReport(book, invoices)
	uncovered = map[string]bool{}
	for _, e := range entries {
		if e.Beleg != "" && e.Saldo < -0.005 {
			uncovered[e.Beleg] = true
		}
	}
	return uncovered, closing
}

// parseGermanDate parses a DD.MM.YYYY date.
func parseGermanDate(s string) (time.Time, bool) {
	t, err := time.Parse("02.01.2006", strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
