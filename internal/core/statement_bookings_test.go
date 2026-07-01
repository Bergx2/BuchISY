package core

import "testing"

const samplePageHTML = `<!DOCTYPE html>
<html><body>
<div id="page0" style="width:595.3pt;height:841.9pt">
<p style="top:288.6pt;left:68.0pt;line-height:10.0pt"><span>Kontoauszug 1/2026</span></p>
<p style="top:338.2pt;left:123.1pt;line-height:10.0pt"><span>Kontostand am 02.01.2026</span></p>
<p style="top:351.9pt;left:69.9pt;line-height:10.0pt"><span>05.01.2026 LS-Einl&#xf6;sung Adobe</span></p>
<p style="top:394.1pt;left:69.9pt;line-height:10.0pt"><span>07.01.2026 LS-Einl&#xf6;sung Google</span></p>
<p style="top:427.8pt;left:69.9pt;line-height:10.0pt"><span>07.01.2026 LS-Einl&#xf6;sung Slack</span></p>
<p style="top:503.2pt;left:123.1pt;line-height:10.0pt"><span>Kontostand am 09.01.2026</span></p>
</div></body></html>`

func TestBookingsFromPageHTML_NumbersInOrder(t *testing.T) {
	got := bookingsFromPageHTML(samplePageHTML, 0)
	if len(got) != 3 {
		t.Fatalf("want 3 bookings, got %d: %+v", len(got), got)
	}
	for i, b := range got {
		if b.LineIdx != i+1 {
			t.Errorf("booking %d: LineIdx=%d want %d", i, b.LineIdx, i+1)
		}
		if b.Page != 0 {
			t.Errorf("booking %d: Page=%d want 0", i, b.Page)
		}
	}
	if got[0].Date != "05.01.2026" {
		t.Errorf("first date = %q want 05.01.2026", got[0].Date)
	}
	if got[2].Date != "07.01.2026" {
		t.Errorf("third date = %q want 07.01.2026", got[2].Date)
	}
}

func TestBookingsFromPageHTML_SkipsKontostandRows(t *testing.T) {
	got := bookingsFromPageHTML(samplePageHTML, 0)
	for _, b := range got {
		if b.Text == "Kontostand am 02.01.2026" {
			t.Error("Kontostand row must not be treated as a booking")
		}
	}
}

func TestBookingsFromPageHTML_UnescapesEntities(t *testing.T) {
	got := bookingsFromPageHTML(samplePageHTML, 0)
	want := "05.01.2026 LS-Einlösung Adobe"
	if got[0].Text != want {
		t.Errorf("first text = %q want %q", got[0].Text, want)
	}
}

// amountRunPageHTML mimics a classic Sparkasse statement where each booking's
// amount is a SEPARATE right-aligned run on the same row as the date line, and
// the running balance sits on its own Kontostand row.
const amountRunPageHTML = `<!DOCTYPE html><html><body>
<div id="page0" style="width:595.3pt;height:841.9pt">
<p style="top:338.2pt;left:498.3pt"><span>34.337,91</span></p>
<p style="top:338.2pt;left:123.1pt"><span>Kontostand am 02.01.2025</span></p>
<p style="top:351.9pt;left:69.9pt"><span>02.01.2025 LS-Einl&#xf6;sung SEPA</span></p>
<p style="top:351.9pt;left:504.2pt"><span>-217,71</span></p>
<p style="top:478.5pt;left:69.9pt"><span>02.01.2025 Zahlungseingang</span></p>
<p style="top:478.5pt;left:501.8pt"><span>1.520,25</span></p>
</div></body></html>`

// detailDatePageHTML mimics a Sparkasse statement where a booking's wrapped
// description line in the Erläuterung column (left≈123) starts with a date
// ("08.06.2026, 14.23 UHR") — it must NOT be counted as its own booking. Real
// booking dates sit in the left Datum column (left≈70).
const detailDatePageHTML = `<!DOCTYPE html><html><body>
<div id="page0" style="width:595.3pt;height:841.9pt">
<p style="top:323.3pt;left:70.9pt"><span>Datum</span></p>
<p style="top:351.9pt;left:69.9pt"><span>08.06.2026 SEPA-Auftrag Online</span></p>
<p style="top:351.9pt;left:504.2pt"><span>-309,39</span></p>
<p style="top:372.0pt;left:123.3pt"><span>DATUM 08.06.2026, 13.06 UHR</span></p>
<p style="top:385.6pt;left:69.9pt"><span>08.06.2026 SEPA-Auftrag Online</span></p>
<p style="top:385.6pt;left:504.2pt"><span>-119,00</span></p>
<p style="top:405.7pt;left:123.3pt"><span>08.06.2026, 14.23 UHR</span></p>
<p style="top:419.3pt;left:69.9pt"><span>10.06.2026 Dauerauftrag &#xdc;berw.</span></p>
<p style="top:419.3pt;left:504.2pt"><span>-434,35</span></p>
<p style="top:452.9pt;left:69.9pt"><span>10.06.2026 LS-Einl&#xf6;sung SEPA</span></p>
<p style="top:452.9pt;left:507.7pt"><span>-56,18</span></p>
</div></body></html>`

func TestBookingsFromPageHTML_SkipsDetailColumnDates(t *testing.T) {
	got := bookingsFromPageHTML(detailDatePageHTML, 0)
	if len(got) != 4 {
		t.Fatalf("want 4 bookings (detail-column date excluded), got %d: %+v", len(got), got)
	}
	amts := []float64{309.39, 119.00, 434.35, 56.18}
	for i, w := range amts {
		if got[i].Betrag != w {
			t.Errorf("booking %d betrag = %.2f, want %.2f (%q)", i, got[i].Betrag, w, got[i].Text)
		}
	}
}

func TestBookingsFromPageHTML_AmountOnSeparateRun(t *testing.T) {
	got := bookingsFromPageHTML(amountRunPageHTML, 0)
	if len(got) != 2 {
		t.Fatalf("want 2 bookings (Kontostand row excluded), got %d: %+v", len(got), got)
	}
	if got[0].Betrag != 217.71 || got[0].IstGutschrift {
		t.Errorf("booking 0: betrag=%.2f credit=%v, want 217.71 debit", got[0].Betrag, got[0].IstGutschrift)
	}
	if got[1].Betrag != 1520.25 || !got[1].IstGutschrift {
		t.Errorf("booking 1: betrag=%.2f credit=%v, want 1520.25 credit", got[1].Betrag, got[1].IstGutschrift)
	}
}

func TestParseBuchungRef_RoundTrip(t *testing.T) {
	ref := BuchungRef{StatementFilename: "Auszug_2026_0002.pdf", Page: 0, LineIdx: 3}
	parsed := ParseBuchungRef(ref.String())
	if parsed != ref {
		t.Errorf("round trip mismatch: got %+v want %+v", parsed, ref)
	}
}

func TestParseBuchungRef_MalformedReturnsZero(t *testing.T) {
	for _, in := range []string{"", "garbage", "file.pdf|abc|def", "file.pdf|0"} {
		if got := ParseBuchungRef(in); !got.IsZero() {
			t.Errorf("ParseBuchungRef(%q) = %+v, want zero", in, got)
		}
	}
}

func TestStatementCacheStale(t *testing.T) {
	fresh := &StatementMetadata{BookingsParsedMtime: 100, BookingsParserVersion: StatementParserVersion, Bookings: []StatementBooking{{}}}
	if statementCacheStale(fresh, 100) {
		t.Error("fresh cache (matching mtime + version + non-empty) should NOT be stale")
	}
	if !statementCacheStale(fresh, 101) {
		t.Error("changed file mtime should be stale")
	}
	oldVer := &StatementMetadata{BookingsParsedMtime: 100, BookingsParserVersion: StatementParserVersion - 1, Bookings: []StatementBooking{{}}}
	if !statementCacheStale(oldVer, 100) {
		t.Error("cache from an older parser version should be stale (forces re-parse after a parser fix)")
	}
	empty := &StatementMetadata{BookingsParsedMtime: 100, BookingsParserVersion: StatementParserVersion}
	if !statementCacheStale(empty, 100) {
		t.Error("empty bookings should be stale")
	}
}

func TestParseLineAmount(t *testing.T) {
	cases := []struct {
		text string
		want float64
	}{
		{"14.01.2026 AMAZON WEB SERVICES EMEA 78,53", 78.53},
		{"03.01. Lastschrift Telekom -1.234,56", 1234.56},
		{"05.01. Gutschrift Kunde 2.000,00 H", 2000.00},
		{"no amount here", 0},
	}
	for _, c := range cases {
		if got := ParseLineAmount(c.text); got != c.want {
			t.Errorf("ParseLineAmount(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestParseLineIsCredit(t *testing.T) {
	credits := []string{
		"05.01. Gutschrift Kunde 2.000,00 H",
		"03.01. Zahlungseingang Müller 500,00",
		"07.01. SEPA-Gutschrift 80,00 +",
	}
	debits := []string{
		"14.01. AMAZON WEB SERVICES 78,53",
		"03.01. Lastschrift Telekom -49,99",
		"02.01. Kartenzahlung REWE 23,40",
	}
	for _, c := range credits {
		if !ParseLineIsCredit(c) {
			t.Errorf("expected credit: %q", c)
		}
	}
	for _, d := range debits {
		if ParseLineIsCredit(d) {
			t.Errorf("expected debit: %q", d)
		}
	}
}
