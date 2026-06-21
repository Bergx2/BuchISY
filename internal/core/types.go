// Package core contains the core business logic and data types for BuchISY.
package core

import (
	"fmt"
	"time"
)

// Meta represents the invoice metadata extracted from a PDF.
type Meta struct {
	Auftraggeber      string     // Company name (previously Firmenname)
	Verwendungszweck  string     // Purpose/description (previously Kurzbezeichnung)
	Rechnungsnummer   string     // Invoice number
	VATID             string     // VAT-ID Nr. of the SENDER (e.g. DE123456789)
	BetragNetto       float64    // Net amount
	SteuersatzProzent float64    // Tax rate in percent
	SteuersatzBetrag  float64    // Tax amount
	Bruttobetrag      float64    // Gross amount
	TaxLines          []TaxLine  // VAT lines; aggregates above are their sums
	Trinkgeld         float64    // tip, no VAT, only part of Bruttobetrag
	Waehrung          string     // Currency (EUR, USD, etc.)
	Rechnungsdatum    string     // Invoice date DD.MM.YYYY
	Jahr              string     // Year YYYY
	Monat             string     // Month MM
	Gegenkonto        int        // Account code
	Bankkonto         string     // Bank account
	Bezahldatum       string     // Payment date DD.MM.YYYY
	Teilzahlung       bool       // Partial payment flag
	Dateiname         string     // Final filename
	Kommentar         string     // Comment/note for this invoice
	BetragNetto_EUR   float64    // Net amount in default currency (EUR) for foreign currency invoices
	Gebuehr           float64    // Fee (e.g., currency exchange fee)
	HatAnhaenge       bool       // Indicates if invoice has additional file attachments
	// BuchungRef is "<statementFilename>|<page>|<lineIdx>" pointing to
	// a booking on a bank statement; empty when this invoice is not
	// linked to a statement. The statement is identified within the
	// invoice's Bankkonto (Zahlungskonto) folder.
	BuchungRef string
	Buchung    Booking // double-entry booking for this invoice
}

// Account represents a user-defined account (Gegenkonto).
type Account struct {
	Code  int    `json:"code"`
	Label string `json:"label"`
}

// Account type values for BankAccount.AccountType.
const (
	AccountTypeBank       = "bank"
	AccountTypeCreditCard = "creditcard"
	AccountTypeCash       = "cash"
)

// BankAccount represents a user-defined payment account (Zahlungskonto):
// a bank account, a credit card / clearing account, or a cash register.
type BankAccount struct {
	Name              string `json:"name"`
	IBAN              string `json:"iban"`
	AccountType       string `json:"account_type"`       // bank | creditcard | cash
	SettlementAccount string `json:"settlement_account"` // account that settles a credit card monthly
	SKR04Konto        int    `json:"skr04_konto,omitempty"`
	IsCreditCard      bool   `json:"is_credit_card"`     // legacy flag, kept only for migration
}

// Settings represents the application settings.
type Settings struct {
	StorageRoot            string        `json:"storage_root"`
	ScanInboxFolder        string        `json:"scan_inbox_folder"`
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
	DefaultBankAccountIBAN string        `json:"default_bank_account_iban"`
	BankAccounts           []BankAccount `json:"bank_accounts"`
	RememberCompanyAccount bool          `json:"remember_company_account"`
	AutoSelectAccount      bool          `json:"auto_select_account"`
	LastUsedFolder         string        `json:"last_used_folder"`      // Last folder for Belege / attachments
	LastStatementFolder    string        `json:"last_statement_folder"` // Last folder for Kontoauszüge
	OwnVATID               string        `json:"own_vat_id"`            // The user's own company VAT-ID — excluded during auto-extract
	DebugMode              bool          `json:"debug_mode"`            // Enable verbose debug logging
	WindowWidth            int           `json:"window_width"`          // Window width in pixels
	WindowHeight           int           `json:"window_height"`         // Window height in pixels
	WindowX                int           `json:"window_x"`              // Window X position
	WindowY                int           `json:"window_y"`              // Window Y position
	DialogWidth            int           `json:"dialog_width"`          // Invoice dialog width in pixels
	DialogHeight           int           `json:"dialog_height"`         // Invoice dialog height in pixels
	CSVSeparator           string        `json:"csv_separator"`         // CSV field separator: "," (comma), ";" (semicolon), "\t" (tab)
	CSVEncoding            string        `json:"csv_encoding"`          // CSV file encoding: "ISO-8859-1" or "UTF-8"
	ColumnOrder            []string      `json:"column_order"`          // Order of columns in table and CSV
	UIScale                float32       `json:"ui_scale"`              // UI zoom factor (1.0 = 100%)
	PreviewSplitOffset     float64       `json:"preview_split_offset"`  // Divider position in the confirmation window (0..1)
}

// DefaultSettings returns the default application settings.
func DefaultSettings() Settings {
	return Settings{
		StorageRoot:        "", // Will be set to Documents/BuchISY on first run
		UseMonthSubfolders: true,
		NamingTemplate:     "${YYYY}-${MM}-${DD}_${Company}_${Kurzbez8}_${InvoiceNumber}_${Currency}_${GrossAmount}.pdf",
		DecimalSeparator:   ",",
		CurrencyDefault:    "EUR",
		AnthropicModel:     "claude-sonnet-4-6",
		AnthropicAPIKeyRef: "claude", // keyring account name
		Language:           "de",
		ProcessingMode:     "claude",
		DefaultAccount:     2000,
		Accounts: []Account{
			{Code: 2000, Label: "Ausgaben"},
		},
		DefaultBankAccount: "Sparkasse",
		BankAccounts: []BankAccount{
			{Name: "Sparkasse", AccountType: AccountTypeBank},
		},
		RememberCompanyAccount: true,
		AutoSelectAccount:      true,
		DebugMode:              false,
		WindowWidth:            1500,
		WindowHeight:           875,
		WindowX:                -1, // -1 means center on screen
		WindowY:                -1,
		DialogWidth:            850,
		DialogHeight:           700,
		CSVSeparator:           ",",
		CSVEncoding:            "ISO-8859-1",
		ColumnOrder:            DefaultCSVColumns,
		UIScale:                1.0,
		PreviewSplitOffset:     0.33,
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
	Auftraggeber      string
	Verwendungszweck  string
	Rechnungsnummer   string
	VATID             string // VAT-ID Nr. of the sender
	BetragNetto       float64
	SteuersatzProzent float64
	SteuersatzBetrag  float64
	Bruttobetrag      float64
	TaxLines          []TaxLine
	Trinkgeld         float64
	Waehrung          string
	Gegenkonto        int
	Bankkonto         string
	Bezahldatum       string
	Teilzahlung       bool
	Kommentar         string
	BetragNetto_EUR   float64
	Gebuehr           float64
	HatAnhaenge       bool
	AnzahlAnhaenge    int
	Unterordner       string // "" | "Bar" | "Ausgangsrechnungen"
	BuchungRef        string // statementFilename|page|lineIdx (within the Bankkonto's folder)
	Buchung           Booking
}

// ToCSVRow converts Meta to CSVRow.
func (m Meta) ToCSVRow() CSVRow {
	// Set Verwendungszweck to "-" if empty
	verwendungszweck := m.Verwendungszweck
	if verwendungszweck == "" {
		verwendungszweck = "-"
	}

	return CSVRow{
		Dateiname:         m.Dateiname,
		Rechnungsdatum:    m.Rechnungsdatum,
		Jahr:              m.Jahr,
		Monat:             m.Monat,
		Auftraggeber:      m.Auftraggeber,
		Verwendungszweck:  verwendungszweck,
		Rechnungsnummer:   m.Rechnungsnummer,
		VATID:             m.VATID,
		BetragNetto:       m.BetragNetto,
		SteuersatzProzent: m.SteuersatzProzent,
		SteuersatzBetrag:  m.SteuersatzBetrag,
		Bruttobetrag:      m.Bruttobetrag,
		TaxLines:          m.TaxLines,
		Trinkgeld:         m.Trinkgeld,
		Waehrung:          m.Waehrung,
		Gegenkonto:        m.Gegenkonto,
		Bankkonto:         m.Bankkonto,
		Bezahldatum:       m.Bezahldatum,
		Teilzahlung:       m.Teilzahlung,
		Kommentar:         m.Kommentar,
		BetragNetto_EUR:   m.BetragNetto_EUR,
		Gebuehr:           m.Gebuehr,
		HatAnhaenge:       m.HatAnhaenge,
		BuchungRef:        m.BuchungRef,
		Buchung:           m.Buchung,
	}
}

// ToMeta converts CSVRow to Meta.
func (r CSVRow) ToMeta() Meta {
	return Meta{
		Dateiname:         r.Dateiname,
		Rechnungsdatum:    r.Rechnungsdatum,
		Jahr:              r.Jahr,
		Monat:             r.Monat,
		Auftraggeber:      r.Auftraggeber,
		Verwendungszweck:  r.Verwendungszweck,
		Rechnungsnummer:   r.Rechnungsnummer,
		VATID:             r.VATID,
		BetragNetto:       r.BetragNetto,
		SteuersatzProzent: r.SteuersatzProzent,
		SteuersatzBetrag:  r.SteuersatzBetrag,
		Bruttobetrag:      r.Bruttobetrag,
		TaxLines:          r.TaxLines,
		Trinkgeld:         r.Trinkgeld,
		Waehrung:          r.Waehrung,
		Gegenkonto:        r.Gegenkonto,
		Bankkonto:         r.Bankkonto,
		Bezahldatum:       r.Bezahldatum,
		Teilzahlung:       r.Teilzahlung,
		Kommentar:         r.Kommentar,
		BetragNetto_EUR:   r.BetragNetto_EUR,
		Gebuehr:           r.Gebuehr,
		HatAnhaenge:       r.HatAnhaenge,
		BuchungRef:        r.BuchungRef,
		Buchung:           r.Buchung,
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

// PaymentAccountSKR04 returns the SKR04 account that the Haben (credit) side of
// a booking should post to for a given Zahlungskonto, looked up by name. An
// explicit BankAccount.SKR04Konto wins; otherwise it falls back by account type
// (bank→1800, cash→1600). Returns (0,false) when nothing maps.
func (s Settings) PaymentAccountSKR04(bankAccountName string) (int, bool) {
	for _, ba := range s.BankAccounts {
		if ba.Name != bankAccountName {
			continue
		}
		if ba.SKR04Konto != 0 {
			return ba.SKR04Konto, true
		}
		switch ba.AccountType {
		case AccountTypeBank:
			return 1800, true
		case AccountTypeCash:
			return 1600, true
		}
		return 0, false
	}
	return 0, false
}
