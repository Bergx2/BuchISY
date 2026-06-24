package core

import (
	"bytes"
	"fmt"
)

// BuildVerfahrensdokumentationPDF generates the GoBD-required
// Verfahrensdokumentation for the given profile settings as a PDF.
// chartAccounts is the total number of accounts in the active chart of
// accounts (len(chart.All())), profilName is the profile/Mandant name and
// datum is the document date formatted as "DD.MM.YYYY".
func BuildVerfahrensdokumentationPDF(s Settings, chartAccounts int, profilName, datum string) ([]byte, error) {
	title := "Verfahrensdokumentation BuchISY"
	pdf, tr := newReportPDF(title, "P")

	// section renders a numbered GoBD heading + wrapped body text.
	section := func(num int, heading, body string) {
		pdf.Ln(3)
		pdf.SetFont("Arial", "B", 11)
		pdf.CellFormat(0, 8, tr(fmt.Sprintf("%d. %s", num, heading)), "", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "", 9)
		pdf.MultiCell(0, 5, tr(body), "", "L", false)
	}

	// Resolve dynamic values.
	modus := "Lokale Mustererkennung (offline)"
	if s.ProcessingMode == "claude" {
		modus = "KI-Extraktion via Claude (Anthropic API)"
	}
	if profilName == "" {
		profilName = "-"
	}
	if datum == "" {
		datum = "-"
	}
	storageRoot := s.StorageRoot
	if storageRoot == "" {
		storageRoot = "-"
	}
	namingTemplate := s.NamingTemplate
	if namingTemplate == "" {
		namingTemplate = "-"
	}
	chartStr := fmt.Sprintf("%d", chartAccounts)

	// 1. Allgemeines
	section(1, "Allgemeines",
		"Programm: BuchISY (Bergx2 GmbH, www.buchisy.de)\n"+
			"Profil/Mandant: "+profilName+"\n"+
			"Stand dieser Dokumentation: "+datum+"\n\n"+
			"BuchISY ist eine Desktop-Anwendung fuer macOS und Windows, die kleinen "+
			"Unternehmen und Freiberuflern das Erfassen, Ablegen und Verbuchen von "+
			"Eingangs- und Ausgangsrechnungen erleichtert. Die Anwendung genuegt den "+
			"Anforderungen der GoBD (BMF-Schreiben vom 28.11.2019).")

	// 2. Belegerfassung
	section(2, "Belegerfassung",
		"Extraktionsmodus: "+modus+"\n\n"+
			"Eingangskanäle:\n"+
			"  - PDF-Upload (Drag & Drop oder Dateiauswahl)\n"+
			"  - Scan-Ordner (automatische Erkennung neuer Dateien)\n"+
			"  - Mehrfachimport (Batch-Verarbeitung)\n\n"+
			"E-Rechnung: XRechnung und ZUGFeRD werden automatisch erkannt und "+
			"strukturiert verarbeitet (hoehere Prioritaet als Texterkennung).\n\n"+
			"Gescannte PDFs ohne eingebetteten Text werden per Vision-API (Claude) "+
			"oder durch optische Zeichenerkennung (OCR) verarbeitet.")

	// 3. Belegfluss & Ablage
	section(3, "Belegfluss & Ablage",
		"Speicherpfad: "+storageRoot+"\n"+
			"Ordnerstruktur: Monatsordner YYYY-MM unterhalb des Speicherpfads.\n"+
			"Dateibenennung: "+namingTemplate+"\n\n"+
			"Anhänge (z.B. Quittungen, Vertraege) werden im Unterordner "+
			"<Dateiname>-files/ abgelegt. Die Originaldatei wird unveraendert "+
			"gespeichert; BuchISY benennt sie nach dem konfigurierten Template um "+
			"und verschiebt sie in den zugehoerigen Monatsordner.")

	// 4. Belegnummernkreis
	section(4, "Belegnummernkreis",
		"BuchISY vergibt fortlaufende, lueckenlose Belegnummern im Format "+
			"YYYY-NNNN (Jahr + vierstellige Sequenznummer). Die Vergabe erfolgt "+
			"automatisch beim Speichern eines Belegs. Bestehende Nummern koennen "+
			"durch die Funktion Belegnummern neu vergeben korrigiert werden.")

	// 5. Buchung
	section(5, "Buchung",
		"Buchungssystematik: Soll/Haben gemaess deutschem Kontenrahmen.\n"+
			"Kontenrahmen: "+chartStr+" Konten konfiguriert.\n\n"+
			"Unterstuetzte Steuerschluessel: Voller Steuersatz (19 %), ermaessigter "+
			"Steuersatz (7 %), steuerfrei, §13b UStG (Umkehrung der "+
			"Steuerschuldnerschaft).\n\n"+
			"Export: DATEV-Belegpaket (EXTF-Format), Lexware-CSV-Format. "+
			"Die erzeugten Exportdateien koennen direkt in DATEV Unternehmen Online "+
			"oder Lexware importiert werden.")

	// 6. Unveraenderbarkeit & Festschreibung
	section(6, "Unveraenderbarkeit & Festschreibung",
		"Periodenabschluss: Einzelne Monate koennen gesperrt werden "+
			"(Funktion: Monat sperren). Gesperrte Perioden koennen nur durch "+
			"explizites Entsperren wieder geoeffnet werden.\n\n"+
			"Aenderungsprotokoll (Audit-Trail): Jede Anlage, Aenderung und "+
			"Loeschung wird mit Zeitstempel und Aktion in einem Audit-Log "+
			"festgehalten.\n\n"+
			"Nach Festschreibung sind nur noch Stornobuchungen moeglich; "+
			"direkte Aenderungen an gesperrten Datensaetzen werden abgewiesen.")

	// 7. Aufbewahrung
	section(7, "Aufbewahrung",
		"Aufbewahrungsfrist: 10 Jahre gemaess §147 AO.\n\n"+
			"Original-PDFs: Die Originaldateien werden unveraendert im Monatsordner "+
			"abgelegt und duerfen waehrend der Aufbewahrungsfrist nicht geaendert "+
			"oder geloescht werden.\n\n"+
			"Fuehrendes System: SQLite-Datenbank (invoices.db). CSV-Dateien "+
			"(invoices.csv) pro Monatsordner werden automatisch aus der Datenbank "+
			"erzeugt und dienen als zusaetzliches Sicherungsformat.")

	// 8. Datensicherung
	section(8, "Datensicherung",
		"BuchISY bietet eine integrierte Backup-Funktion "+
			"(Funktion: Backup erstellen), die Datenbank, Konfiguration und "+
			"CSV-Exporte als ZIP-Archiv sichert.\n\n"+
			"Empfohlen wird eine taeglich automatisierte Sicherung des gesamten "+
			"Speicherpfads auf ein externes Medium oder in eine Cloud-Loesung "+
			"(z.B. Time Machine, Windows-Sicherung, Backblaze).")

	// 9. GoBD-Datenexport
	section(9, "GoBD-Datenexport",
		"Auf Anforderung durch die Finanzverwaltung (Betriebspruefung) kann ein "+
			"GoBD-konformes Exportpaket erzeugt werden "+
			"(Funktion: GoBD-/StB-Paket exportieren).\n\n"+
			"Das Paket enthaelt:\n"+
			"  - EXTF-Exportdatei (DATEV-Format) mit allen Buchungsdaten\n"+
			"  - Belegbilder (Original-PDFs)\n"+
			"  - index.xml mit Belegverzeichnis\n\n"+
			"Das Paket genuegt den Anforderungen des §147 Abs. 6 AO (Datenzugriff).")

	// 10. Verantwortlichkeiten
	section(10, "Verantwortlichkeiten",
		"Die folgenden Angaben sind vom Betrieb auszufuellen:\n\n"+
			"Verantwortliche Person fuer die IT-gestuetzte Buchfuehrung:\n"+
			"  Name:     _________________________________\n"+
			"  Funktion: _________________________________\n\n"+
			"Datum der Erstellung dieser Dokumentation: "+datum+"\n\n"+
			"Datum der letzten Pruefung / Aktualisierung:\n"+
			"  _________________________________\n\n"+
			"Unterschrift:\n"+
			"  _________________________________")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
