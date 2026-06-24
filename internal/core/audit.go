// Package core contains the core business logic and data types for BuchISY.
package core

import (
	"encoding/json"
	"fmt"
)

// AuditEntry represents a single entry in the audit log.
type AuditEntry struct {
	TS        string // RFC3339 / DATETIME timestamp
	Aktion    string // "create", "update", "delete"
	Entitaet  string // "invoice"
	Schluessel string // e.g. "2026-0001 2026-01-Firma-..."
	Details   string // JSON diff for updates, empty for create/delete
}

// diffEntry holds the old and new value for a changed field.
type diffEntry struct {
	Alt any `json:"alt"`
	Neu any `json:"neu"`
}

// diffableFields defines the ordered list of CSVRow fields compared by DiffFields.
// Adding a field here is the only change needed to include it in the diff.
var diffableFields = []struct {
	name   string
	getter func(r CSVRow) any
}{
	{"Auftraggeber", func(r CSVRow) any { return r.Auftraggeber }},
	{"Rechnungsnummer", func(r CSVRow) any { return r.Rechnungsnummer }},
	{"Rechnungsdatum", func(r CSVRow) any { return r.Rechnungsdatum }},
	{"BetragNetto", func(r CSVRow) any { return r.BetragNetto }},
	{"SteuersatzBetrag", func(r CSVRow) any { return r.SteuersatzBetrag }},
	{"Bruttobetrag", func(r CSVRow) any { return r.Bruttobetrag }},
	{"Gegenkonto", func(r CSVRow) any { return r.Gegenkonto }},
	{"Bankkonto", func(r CSVRow) any { return r.Bankkonto }},
	{"Bezahldatum", func(r CSVRow) any { return r.Bezahldatum }},
	{"BuchungRef", func(r CSVRow) any { return r.BuchungRef }},
	{"Belegnummer", func(r CSVRow) any { return r.Belegnummer }},
	{"Ausgangsrechnung", func(r CSVRow) any { return r.Ausgangsrechnung }},
}

// DiffFields compares two CSVRow values and returns a JSON string containing
// only the fields that changed, in the form:
//
//	{"FieldName":{"alt":<oldValue>,"neu":<newValue>}, ...}
//
// If no diffable fields changed, it returns "{}".
func DiffFields(old, new CSVRow) string {
	changes := make(map[string]diffEntry)
	for _, f := range diffableFields {
		oldVal := f.getter(old)
		newVal := f.getter(new)
		if fmt.Sprintf("%v", oldVal) != fmt.Sprintf("%v", newVal) {
			changes[f.name] = diffEntry{Alt: oldVal, Neu: newVal}
		}
	}
	if len(changes) == 0 {
		return "{}"
	}
	b, err := json.Marshal(changes)
	if err != nil {
		return "{}"
	}
	return string(b)
}
