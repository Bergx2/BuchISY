package db

const schemaSQL = `
CREATE TABLE IF NOT EXISTS invoices (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	dateiname TEXT NOT NULL,
	rechnungsdatum TEXT,
	jahr TEXT,
	monat TEXT,
	auftraggeber TEXT,
	verwendungszweck TEXT DEFAULT '-',
	rechnungsnummer TEXT,
	betrag_netto REAL,
	steuersatz_prozent REAL,
	steuersatz_betrag REAL,
	bruttobetrag REAL,
	waehrung TEXT,
	gegenkonto INTEGER,
	bankkonto TEXT,
	bezahldatum TEXT,
	teilzahlung BOOLEAN DEFAULT 0,
	kommentar TEXT,
	betrag_netto_eur REAL,
	gebuehr REAL,
	hat_anhaenge BOOLEAN DEFAULT 0,
	ustidnr TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_invoices_monat ON invoices(jahr, monat);
CREATE INDEX IF NOT EXISTS idx_invoices_datum ON invoices(rechnungsdatum);
CREATE INDEX IF NOT EXISTS idx_invoices_auftraggeber ON invoices(auftraggeber);
CREATE INDEX IF NOT EXISTS idx_invoices_rechnungsnummer ON invoices(rechnungsnummer);
CREATE INDEX IF NOT EXISTS idx_invoices_dateiname ON invoices(dateiname);

-- Trigger to auto-update updated_at timestamp
CREATE TRIGGER IF NOT EXISTS update_invoices_timestamp
AFTER UPDATE ON invoices
FOR EACH ROW
BEGIN
	UPDATE invoices SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
`

// CurrentSchemaVersion is the current database schema version.
const CurrentSchemaVersion = 1
