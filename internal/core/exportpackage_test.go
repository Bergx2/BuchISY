package core

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
)

// probeDataSet is a minimal struct to verify the index.xml is well-formed GoBD-oriented XML.
type probeDataSet struct {
	XMLName xml.Name    `xml:"DataSet"`
	Media   []probeMedia `xml:"Media"`
}

type probeMedia struct {
	Table []probeTable `xml:"Table"`
}

type probeTable struct {
	URL     string        `xml:"URL"`
	Columns []probeColumn `xml:"VariableLength>Column"`
}

type probeColumn struct {
	Name string `xml:"Name"`
}

func TestBuildExportPackage(t *testing.T) {
	period := "2026-01"
	datevCSV := []byte("EXTF;700;21;Buchungsstapel;9;20260101;;\n")

	rows := []CSVRow{
		{
			Belegnummer:    "2026-0001",
			Dateiname:      "2026-01-15_Acme_GmbH_R001_EUR_119.00.pdf",
			Rechnungsdatum: "15.01.2026",
			Auftraggeber:   "Acme GmbH",
			Bruttobetrag:   119.00,
			Gegenkonto:     4400,
		},
		{
			Belegnummer:    "2026-0002",
			Dateiname:      "2026-01-20_Beta_Corp_R002_EUR_59.50.pdf",
			Rechnungsdatum: "20.01.2026",
			Auftraggeber:   "Beta Corp",
			Bruttobetrag:   59.50,
			Gegenkonto:     4300,
		},
	}

	belege := []BelegFile{
		{
			Belegnummer: "2026-0001",
			Dateiname:   "2026-01-15_Acme_GmbH_R001_EUR_119.00.pdf",
			Bytes:       []byte("%PDF-1.4 fake pdf content for acme"),
		},
		{
			Belegnummer: "2026-0002",
			Dateiname:   "2026-01-20_Beta_Corp_R002_EUR_59.50.pdf",
			Bytes:       []byte("%PDF-1.4 fake pdf content for beta"),
		},
	}

	zipBytes, err := BuildExportPackage(rows, datevCSV, belege, period)
	if err != nil {
		t.Fatalf("BuildExportPackage returned error: %v", err)
	}
	if len(zipBytes) == 0 {
		t.Fatal("BuildExportPackage returned empty zip")
	}

	// Open the zip and inspect entries
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader failed: %v", err)
	}

	// Collect all entry names
	entryNames := make(map[string]bool)
	entryContents := make(map[string][]byte)
	for _, f := range r.File {
		entryNames[f.Name] = true
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed to open zip entry %q: %v", f.Name, err)
		}
		buf := new(bytes.Buffer)
		buf.ReadFrom(rc)
		rc.Close()
		entryContents[f.Name] = buf.Bytes()
	}

	// 1. DATEV CSV must be present
	datevEntry := "DATEV-EXTF_" + period + ".csv"
	if !entryNames[datevEntry] {
		t.Errorf("missing zip entry %q; got: %v", datevEntry, entryNames)
	}

	// 2. manifest.csv must be present
	if !entryNames["manifest.csv"] {
		t.Errorf("missing zip entry manifest.csv; got: %v", entryNames)
	} else {
		manifestContent := string(entryContents["manifest.csv"])
		lines := strings.Split(strings.TrimRight(manifestContent, "\n"), "\n")
		// header + 2 data rows = 3 lines minimum
		if len(lines) < 3 {
			t.Errorf("manifest.csv: expected at least 3 lines (header+2 rows), got %d:\n%s", len(lines), manifestContent)
		}
		// Check header
		expectedHeader := "Belegnummer;Dateiname;Auftraggeber;Rechnungsdatum;Bruttobetrag;Gegenkonto"
		if lines[0] != expectedHeader {
			t.Errorf("manifest.csv header = %q, want %q", lines[0], expectedHeader)
		}
		// Check that both belegnummern appear
		if !strings.Contains(manifestContent, "2026-0001") {
			t.Error("manifest.csv missing Belegnummer 2026-0001")
		}
		if !strings.Contains(manifestContent, "2026-0002") {
			t.Error("manifest.csv missing Belegnummer 2026-0002")
		}
	}

	// 3. index.xml must be present and well-formed
	if !entryNames["index.xml"] {
		t.Errorf("missing zip entry index.xml; got: %v", entryNames)
	} else {
		var ds probeDataSet
		if err := xml.Unmarshal(entryContents["index.xml"], &ds); err != nil {
			t.Errorf("index.xml is not well-formed XML: %v\ncontent: %s", err, entryContents["index.xml"])
		}
		if len(ds.Media) == 0 {
			t.Error("index.xml: no <Media> elements found")
		}
	}

	// 4. belege/<sanitized>.pdf entries must be present for each BelegFile
	belegeFound := 0
	for name := range entryNames {
		if strings.HasPrefix(name, "belege/") && strings.HasSuffix(name, ".pdf") {
			belegeFound++
		}
	}
	if belegeFound != len(belege) {
		t.Errorf("expected %d belege/ entries, got %d; entries: %v", len(belege), belegeFound, entryNames)
	}
}
