package core

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestBuildUStVAXML(t *testing.T) {
	u := UStVAOfficial{Kz81: 6500, USt81: 1235, Kz45: 1077.60, Kz84: 462.40, Kz85: 87.86, Kz67: 87.86, Kz66: 37.79, Kz83: 1197.21}
	data, err := BuildUStVAXML(u, "2025", "287472874")
	if err != nil {
		t.Fatal(err)
	}
	// Valid XML.
	var probe struct{}
	if err := xml.Unmarshal(data, &probe); err != nil {
		t.Fatalf("not valid XML: %v", err)
	}
	s := string(data)
	for _, want := range []string{`zeitraum="2025"`, `ust_idnr="287472874"`, `nr="81"`, "<wert>6500</wert>", `nr="83"`, "<wert>1197.21</wert>"} {
		if !strings.Contains(s, want) {
			t.Errorf("XML missing %q:\n%s", want, s)
		}
	}
	// A zero Kennzahl (Kz86) is omitted; Kz83 is always present.
	if strings.Contains(s, `nr="86"`) {
		t.Error("zero Kz86 should be omitted")
	}
}

func TestBuildZMXML(t *testing.T) {
	z := ZM{Zeilen: []ZMZeile{{UStIdNr: "FI26378052", Netto: 44795}}, Kontrollsumme: 44795}
	data, err := BuildZMXML(z, "2025-Q2", "287472874")
	if err != nil {
		t.Fatal(err)
	}
	var probe struct{}
	if err := xml.Unmarshal(data, &probe); err != nil {
		t.Fatalf("not valid XML: %v", err)
	}
	s := string(data)
	for _, want := range []string{`zeitraum="2025-Q2"`, "<ust_idnr>FI26378052</ust_idnr>", "<summe>44795</summe>", "<kontrollsumme>44795</kontrollsumme>", "Sonstige Leistung"} {
		if !strings.Contains(s, want) {
			t.Errorf("ZM XML missing %q:\n%s", want, s)
		}
	}
}
