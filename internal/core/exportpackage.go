package core

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// BelegFile holds the raw PDF bytes for a single receipt (Beleg) to include in the export package.
type BelegFile struct {
	Belegnummer string // e.g. "2026-0001"
	Dateiname   string // original filename (with or without .pdf)
	Bytes       []byte // raw PDF content
}

// BuildExportPackage builds a ZIP containing:
//   - "DATEV-EXTF_<period>.csv"  — the DATEV Buchungsstapel
//   - "belege/<sanitized>.pdf"   — one entry per BelegFile
//   - "manifest.csv"             — semicolon-separated: Belegnummer;Dateiname;Auftraggeber;Rechnungsdatum;Bruttobetrag;Gegenkonto
//   - "index.xml"                — GoBD-orientiert (nicht DTD-zertifiziert): DataSet/Media/Table describing manifest.csv + DATEV file
//
// It uses only stdlib (archive/zip, encoding/xml).
func BuildExportPackage(rows []CSVRow, datevCSV []byte, belege []BelegFile, period string) ([]byte, error) {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// 1. DATEV EXTF CSV
	datevName := "DATEV-EXTF_" + period + ".csv"
	if err := addZipEntry(w, datevName, datevCSV); err != nil {
		return nil, fmt.Errorf("exportpackage: writing datev csv: %w", err)
	}

	// 2. Beleg PDFs under belege/
	for _, b := range belege {
		safeName := belegZipName(b)
		if err := addZipEntry(w, "belege/"+safeName, b.Bytes); err != nil {
			return nil, fmt.Errorf("exportpackage: writing beleg %q: %w", safeName, err)
		}
	}

	// 3. manifest.csv
	manifestBytes := buildManifest(rows)
	if err := addZipEntry(w, "manifest.csv", manifestBytes); err != nil {
		return nil, fmt.Errorf("exportpackage: writing manifest.csv: %w", err)
	}

	// 4. index.xml
	indexBytes, err := buildIndexXML(datevName, period)
	if err != nil {
		return nil, fmt.Errorf("exportpackage: building index.xml: %w", err)
	}
	if err := addZipEntry(w, "index.xml", indexBytes); err != nil {
		return nil, fmt.Errorf("exportpackage: writing index.xml: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("exportpackage: closing zip: %w", err)
	}
	return buf.Bytes(), nil
}

// addZipEntry writes content to a named entry inside the zip.
func addZipEntry(w *zip.Writer, name string, content []byte) error {
	f, err := w.Create(name)
	if err != nil {
		return err
	}
	_, err = f.Write(content)
	return err
}

// belegZipName returns a sanitized filename for the beleg's zip entry.
// Uses Belegnummer if set, falls back to Dateiname; ensures .pdf suffix.
func belegZipName(b BelegFile) string {
	base := b.Belegnummer
	if base == "" {
		base = b.Dateiname
	}
	// Strip .pdf suffix before sanitizing, then re-add.
	base = strings.TrimSuffix(base, ".pdf")
	base = SanitizeFilename(base)
	if base == "" {
		base = "beleg"
	}
	return base + ".pdf"
}

// buildManifest generates the semicolon-separated manifest.csv content.
// Header: Belegnummer;Dateiname;Auftraggeber;Rechnungsdatum;Bruttobetrag;Gegenkonto
func buildManifest(rows []CSVRow) []byte {
	var sb strings.Builder
	sb.WriteString("Belegnummer;Dateiname;Auftraggeber;Rechnungsdatum;Bruttobetrag;Gegenkonto\n")
	for _, r := range rows {
		sb.WriteString(csvEscapeSemicolon(r.Belegnummer))
		sb.WriteByte(';')
		sb.WriteString(csvEscapeSemicolon(r.Dateiname))
		sb.WriteByte(';')
		sb.WriteString(csvEscapeSemicolon(r.Auftraggeber))
		sb.WriteByte(';')
		sb.WriteString(csvEscapeSemicolon(r.Rechnungsdatum))
		sb.WriteByte(';')
		sb.WriteString(fmt.Sprintf("%.2f", r.Bruttobetrag))
		sb.WriteByte(';')
		sb.WriteString(fmt.Sprintf("%d", r.Gegenkonto))
		sb.WriteByte('\n')
	}
	return []byte(sb.String())
}

// csvEscapeSemicolon wraps the value in double-quotes if it contains a semicolon or quote.
func csvEscapeSemicolon(s string) string {
	if strings.ContainsAny(s, ";\"\n\r") {
		s = strings.ReplaceAll(s, "\"", "\"\"")
		return "\"" + s + "\""
	}
	return s
}

// --- GoBD-oriented index.xml structures ---

type gdpduDataSet struct {
	XMLName xml.Name     `xml:"DataSet"`
	Version string       `xml:"Version,attr"`
	Media   []gdpduMedia `xml:"Media"`
}

type gdpduMedia struct {
	Name  string       `xml:"Name"`
	Table []gdpduTable `xml:"Table"`
}

type gdpduTable struct {
	URL           string        `xml:"URL"`
	Name          string        `xml:"Name"`
	Description   string        `xml:"Description"`
	VariableLength gdpduVarLen  `xml:"VariableLength"`
}

type gdpduVarLen struct {
	ColumnDelimiter string        `xml:"ColumnDelimiter"`
	TextEncapsulator string       `xml:"TextEncapsulator"`
	Columns         []gdpduColumn `xml:"Column"`
}

type gdpduColumn struct {
	Name     string `xml:"Name"`
	DataType string `xml:"DataType"`
}

// buildIndexXML creates a GoBD-orientiert (nicht DTD-zertifiziert) index.xml describing
// the contents of the export package: manifest.csv and the DATEV EXTF file.
func buildIndexXML(datevFileName, period string) ([]byte, error) {
	manifestColumns := []gdpduColumn{
		{Name: "Belegnummer", DataType: "Alphanumerisch"},
		{Name: "Dateiname", DataType: "Alphanumerisch"},
		{Name: "Auftraggeber", DataType: "Alphanumerisch"},
		{Name: "Rechnungsdatum", DataType: "Alphanumerisch"},
		{Name: "Bruttobetrag", DataType: "Numerisch"},
		{Name: "Gegenkonto", DataType: "Numerisch"},
	}

	ds := gdpduDataSet{
		Version: "1.0",
		Media: []gdpduMedia{
			{
				Name: "GoBD-Export " + period,
				Table: []gdpduTable{
					{
						URL:         "manifest.csv",
						Name:        "Belegmanifest",
						Description: "Zuordnung Belegnummer zu Belegdatei und Buchungsdaten (GoBD-orientiert)",
						VariableLength: gdpduVarLen{
							ColumnDelimiter:  ";",
							TextEncapsulator: "\"",
							Columns:          manifestColumns,
						},
					},
					{
						URL:         datevFileName,
						Name:        "DATEV-EXTF-Buchungsstapel",
						Description: "DATEV EXTF Buchungsstapel fuer Zeitraum " + period,
						VariableLength: gdpduVarLen{
							ColumnDelimiter:  ";",
							TextEncapsulator: "\"",
							Columns: []gdpduColumn{
								{Name: "Umsatz", DataType: "Numerisch"},
								{Name: "Soll-Haben-Kennzeichen", DataType: "Alphanumerisch"},
								{Name: "Konto", DataType: "Numerisch"},
								{Name: "Gegenkonto", DataType: "Numerisch"},
								{Name: "Belegdatum", DataType: "Alphanumerisch"},
								{Name: "Belegnummer", DataType: "Alphanumerisch"},
								{Name: "Buchungstext", DataType: "Alphanumerisch"},
							},
						},
					},
				},
			},
		},
	}

	out, err := xml.MarshalIndent(ds, "", "  ")
	if err != nil {
		return nil, err
	}
	// Prepend XML declaration
	result := append([]byte(xml.Header), out...)
	return result, nil
}
