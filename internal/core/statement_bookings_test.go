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
