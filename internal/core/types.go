// Package core contains the core business logic and data types for BuchISY.
package core

import (
	"fmt"
	"time"
)

// Meta represents the invoice metadata extracted from a PDF.
type Meta struct {
	Firmenname          string  // Company name
	Kurzbezeichnung     string  // Short description (max 80 chars)
	Rechnungsnummer     string  // Invoice number
	BetragNetto         float64 // Net amount
	SteuersatzProzent   float64 // Tax rate in percent
	SteuersatzBetrag    float64 // Tax amount
	Bruttobetrag        float64 // Gross amount
	Waehrung            string  // Currency (EUR, USD, etc.)
	Rechnungsdatum      string  // Invoice date DD.MM.YYYY
	Jahr                string  // Year YYYY
	Monat               string  // Month MM
	Gegenkonto          int     // Account code
	Bankkonto           string  // Bank account
	Bezahldatum         string  // Payment date DD.MM.YYYY
	Teilzahlung         bool    // Partial payment flag
	Dateiname           string  // Final filename
	Kommentar           string  // Comment/note for this invoice
	BetragNetto_EUR     float64 // Net amount in default currency (EUR) for foreign currency invoices
	Gebuehr             float64 // Fee (e.g., currency exchange fee)
	HatAnhaenge         bool    // Indicates if invoice has additional file attachments
}

// Account represents a user-defined account (Gegenkonto).
type Account struct {
	Code  int    `json:"code"`
	Label string `json:"label"`
}

// BankAccount represents a user-defined bank account (Bankkonto).
type BankAccount struct {
	Name string `json:"name"`
}

// Settings represents the application settings.
type Settings struct {
	StorageRoot            string        `json:"storage_root"`
	UseMonthSubfolders     bool          `json:"use_month_subfolders"`
	NamingTemplate         string        `json:"naming_template"`
	DecimalSeparator       string        `json:"decimal_separator"`
	CurrencyDefault        string        `json:"currency_default"`
	AnthropicModel         string        `json:"anthropic_model"`
	AnthropicAPIKeyRef     string        `json:"anthropic_api_key_ref"`
	Language               string        `json:"language"`
	ProcessingMode         string        `json:"processing_mode"` // "claude" or "local"
	DefaultAccount         int           `json:"default_account"`
	Accounts               []Account     `json:"accounts"`
	DefaultBankAccount     string        `json:"default_bank_account"`
	BankAccounts           []BankAccount `json:"bank_accounts"`
	RememberCompanyAccount bool          `json:"remember_company_account"`
	AutoSelectAccount      bool          `json:"auto_select_account"`
	LastUsedFolder         string        `json:"last_used_folder"` // Last folder used in file picker
	DebugMode              bool          `json:"debug_mode"`       // Enable verbose debug logging
	WindowWidth            int           `json:"window_width"`     // Window width in pixels
	WindowHeight           int           `json:"window_height"`    // Window height in pixels
	WindowX                int           `json:"window_x"`         // Window X position
	WindowY                int           `json:"window_y"`         // Window Y position
	ColumnOrder            []string      `json:"column_order"`     // Order of columns in table and CSV
}

// DefaultSettings returns the default application settings.
func DefaultSettings() Settings {
	return Settings{
		StorageRoot:        "", // Will be set to Documents/BuchISY on first run
		UseMonthSubfolders: true,
		NamingTemplate:     "${YYYY}-${MM}-${DD}_${Company}_${GrossAmount}_${Currency}.pdf",
		DecimalSeparator:   ",",
		CurrencyDefault:    "EUR",
		AnthropicModel:     "claude-sonnet-4-5",
		AnthropicAPIKeyRef: "claude", // keyring account name
		Language:           "de",
		ProcessingMode:     "claude",
		DefaultAccount:     2000,
		Accounts: []Account{
			{Code: 2000, Label: "Ausgaben"},
		},
		DefaultBankAccount: "Sparkasse",
		BankAccounts: []BankAccount{
			{Name: "Sparkasse"},
		},
		RememberCompanyAccount: true,
		AutoSelectAccount:      true,
		DebugMode:              false,
		WindowWidth:            1500,
		WindowHeight:           875,
		WindowX:                -1, // -1 means center on screen
		WindowY:                -1,
		ColumnOrder:            DefaultCSVColumns,
	}
}

// ProcessingResult represents the result of processing a PDF.
type ProcessingResult struct {
	Meta       Meta
	Confidence float64 // Confidence score (0-1), mainly for local extraction
	Error      error
}

// PDFExtractor is the interface for extracting text from PDFs.
type PDFExtractor interface {
	ExtractText(path string) (string, error)
}

// MetaExtractor is the interface for extracting metadata from invoice text.
type MetaExtractor interface {
	Extract(text string) (Meta, float64, error)
}

// CSVRow represents a row in the invoices CSV file.
type CSVRow struct {
	Dateiname         string
	Rechnungsdatum    string
	Jahr              string
	Monat             string
	Firmenname        string
	Kurzbezeichnung   string
	Rechnungsnummer   string
	BetragNetto       float64
	SteuersatzProzent float64
	SteuersatzBetrag  float64
	Bruttobetrag      float64
	Waehrung          string
	Gegenkonto        int
	Bankkonto         string
	Bezahldatum       string
	Teilzahlung       bool
	Kommentar         string
	BetragNetto_EUR   float64
	Gebuehr           float64
	HatAnhaenge       bool
}

// ToCSVRow converts Meta to CSVRow.
func (m Meta) ToCSVRow() CSVRow {
	return CSVRow{
		Dateiname:         m.Dateiname,
		Rechnungsdatum:    m.Rechnungsdatum,
		Jahr:              m.Jahr,
		Monat:             m.Monat,
		Firmenname:        m.Firmenname,
		Kurzbezeichnung:   m.Kurzbezeichnung,
		Rechnungsnummer:   m.Rechnungsnummer,
		BetragNetto:       m.BetragNetto,
		SteuersatzProzent: m.SteuersatzProzent,
		SteuersatzBetrag:  m.SteuersatzBetrag,
		Bruttobetrag:      m.Bruttobetrag,
		Waehrung:          m.Waehrung,
		Gegenkonto:        m.Gegenkonto,
		Bankkonto:         m.Bankkonto,
		Bezahldatum:       m.Bezahldatum,
		Teilzahlung:       m.Teilzahlung,
		Kommentar:         m.Kommentar,
		BetragNetto_EUR:   m.BetragNetto_EUR,
		Gebuehr:           m.Gebuehr,
		HatAnhaenge:       m.HatAnhaenge,
	}
}

// ToMeta converts CSVRow to Meta.
func (r CSVRow) ToMeta() Meta {
	return Meta{
		Dateiname:         r.Dateiname,
		Rechnungsdatum:    r.Rechnungsdatum,
		Jahr:              r.Jahr,
		Monat:             r.Monat,
		Firmenname:        r.Firmenname,
		Kurzbezeichnung:   r.Kurzbezeichnung,
		Rechnungsnummer:   r.Rechnungsnummer,
		BetragNetto:       r.BetragNetto,
		SteuersatzProzent: r.SteuersatzProzent,
		SteuersatzBetrag:  r.SteuersatzBetrag,
		Bruttobetrag:      r.Bruttobetrag,
		Waehrung:          r.Waehrung,
		Gegenkonto:        r.Gegenkonto,
		Bankkonto:         r.Bankkonto,
		Bezahldatum:       r.Bezahldatum,
		Teilzahlung:       r.Teilzahlung,
		Kommentar:         r.Kommentar,
		BetragNetto_EUR:   r.BetragNetto_EUR,
		Gebuehr:           r.Gebuehr,
		HatAnhaenge:       r.HatAnhaenge,
	}
}

// MonthContext represents the currently selected year-month.
type MonthContext struct {
	Year  int
	Month time.Month
}

// String returns YYYY-MM format.
func (mc MonthContext) String() string {
	return fmt.Sprintf("%04d-%02d", mc.Year, mc.Month)
}

// FolderName returns the folder name for this month (YYYY-MM).
func (mc MonthContext) FolderName() string {
	return fmt.Sprintf("%04d-%02d", mc.Year, mc.Month)
}
