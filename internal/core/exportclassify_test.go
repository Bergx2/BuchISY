package core

import "testing"

func TestClassifyForExport(t *testing.T) {
	good := Booking{Entries: []BookingEntry{{Konto: 6640, Betrag: 10, Soll: true}, {Konto: 1800, Betrag: 10, Soll: false}}}
	rows := []CSVRow{
		{Dateiname: "neu.pdf", Buchung: good},
		{Dateiname: "alt.pdf", Buchung: good, Exportiert: true},
		{Dateiname: "leer.pdf"},
		{Dateiname: "schief.pdf", Buchung: Booking{Entries: []BookingEntry{{Konto: 6640, Betrag: 10, Soll: true}}}},
	}
	c := ClassifyForExport(rows, false)
	if len(c.Exportable) != 1 || c.Exportable[0].Dateiname != "neu.pdf" {
		t.Errorf("exportable = %+v", c.Exportable)
	}
	if len(c.AlreadyExported) != 1 {
		t.Errorf("alreadyExported = %+v", c.AlreadyExported)
	}
	if len(c.Skipped) != 2 {
		t.Fatalf("skipped = %+v", c.Skipped)
	}
	// includeExported puts the already-exported row back into Exportable.
	if len(ClassifyForExport(rows, true).Exportable) != 2 {
		t.Error("includeExported should yield 2 exportable")
	}
}
