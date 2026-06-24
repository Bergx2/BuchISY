package core

import (
	"testing"
)

// --- DetectBankFormat ---

func TestDetectBankFormat_CAMT(t *testing.T) {
	data := []byte(`<?xml version="1.0"?><Document><BkToCstmrStmt></BkToCstmrStmt></Document>`)
	got := DetectBankFormat(data)
	if got != "camt" {
		t.Fatalf("DetectBankFormat: want \"camt\", got %q", got)
	}
}

func TestDetectBankFormat_MT940(t *testing.T) {
	data := []byte(":20:STARTUMSE\r\n:25:DE12345678901234567890\r\n:61:2601100106C300,00NTRFNONREF\r\n:86:Gutschrift\r\n-\r\n")
	got := DetectBankFormat(data)
	if got != "mt940" {
		t.Fatalf("DetectBankFormat: want \"mt940\", got %q", got)
	}
}

func TestDetectBankFormat_Unknown(t *testing.T) {
	data := []byte("just some random text")
	got := DetectBankFormat(data)
	if got != "" {
		t.Fatalf("DetectBankFormat: want \"\", got %q", got)
	}
}

// --- ParseCAMT053 ---

const camtSample = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.02">
  <BkToCstmrStmt>
    <Stmt>
      <Ntry>
        <Amt Ccy="EUR">300.00</Amt>
        <CdtDbtInd>CRDT</CdtDbtInd>
        <BookgDt>
          <Dt>2026-01-12</Dt>
        </BookgDt>
        <AddtlNtryInf>Gutschrift Kunde A</AddtlNtryInf>
      </Ntry>
      <Ntry>
        <Amt Ccy="EUR">55.24</Amt>
        <CdtDbtInd>DBIT</CdtDbtInd>
        <ValDt>
          <Dt>2026-01-15</Dt>
        </ValDt>
        <NtryDtls>
          <TxDtls>
            <RmtInf>
              <Ustrd>Lieferant B Rechnung 2026-007</Ustrd>
            </RmtInf>
          </TxDtls>
        </NtryDtls>
      </Ntry>
    </Stmt>
  </BkToCstmrStmt>
</Document>`

func TestParseCAMT053(t *testing.T) {
	bookings, err := ParseCAMT053([]byte(camtSample))
	if err != nil {
		t.Fatalf("ParseCAMT053 error: %v", err)
	}
	if len(bookings) != 2 {
		t.Fatalf("want 2 bookings, got %d", len(bookings))
	}

	// First: credit
	b0 := bookings[0]
	if b0.IstGutschrift != true {
		t.Errorf("b0.IstGutschrift: want true, got false")
	}
	if b0.Betrag != 300.00 {
		t.Errorf("b0.Betrag: want 300.00, got %f", b0.Betrag)
	}
	if b0.Date != "12.01.2026" {
		t.Errorf("b0.Date: want \"12.01.2026\", got %q", b0.Date)
	}
	if b0.Text != "Gutschrift Kunde A" {
		t.Errorf("b0.Text: want \"Gutschrift Kunde A\", got %q", b0.Text)
	}
	if b0.Page != 0 {
		t.Errorf("b0.Page: want 0, got %d", b0.Page)
	}
	if b0.LineIdx != 1 {
		t.Errorf("b0.LineIdx: want 1, got %d", b0.LineIdx)
	}

	// Second: debit, date from ValDt, text from Ustrd
	b1 := bookings[1]
	if b1.IstGutschrift != false {
		t.Errorf("b1.IstGutschrift: want false, got true")
	}
	if b1.Betrag != 55.24 {
		t.Errorf("b1.Betrag: want 55.24, got %f", b1.Betrag)
	}
	if b1.Date != "15.01.2026" {
		t.Errorf("b1.Date: want \"15.01.2026\", got %q", b1.Date)
	}
	if b1.Text != "Lieferant B Rechnung 2026-007" {
		t.Errorf("b1.Text: want \"Lieferant B Rechnung 2026-007\", got %q", b1.Text)
	}
	if b1.LineIdx != 2 {
		t.Errorf("b1.LineIdx: want 2, got %d", b1.LineIdx)
	}
}

// --- ParseMT940 ---

const mt940Sample = ":20:STARTUMSE\r\n" +
	":25:DE12345678901234567890/EUR\r\n" +
	":28C:00001/001\r\n" +
	":60F:C260101EUR1000,00\r\n" +
	":61:2601060106C300,00NTRFNONREF\r\n" +
	":86:Gutschrift von Kunde X\r\n" +
	":61:2601100110D55,24NTRFNONREF\r\n" +
	":86:Lastschrift Lieferant Y\r\n" +
	"-\r\n"

func TestParseMT940(t *testing.T) {
	bookings, err := ParseMT940([]byte(mt940Sample))
	if err != nil {
		t.Fatalf("ParseMT940 error: %v", err)
	}
	if len(bookings) != 2 {
		t.Fatalf("want 2 bookings, got %d", len(bookings))
	}

	// First: credit C, date YYMMDD=260106 → 06.01.2026
	b0 := bookings[0]
	if b0.IstGutschrift != true {
		t.Errorf("b0.IstGutschrift: want true, got false")
	}
	if b0.Betrag != 300.00 {
		t.Errorf("b0.Betrag: want 300.00, got %f", b0.Betrag)
	}
	if b0.Date != "06.01.2026" {
		t.Errorf("b0.Date: want \"06.01.2026\", got %q", b0.Date)
	}
	if b0.Text != "Gutschrift von Kunde X" {
		t.Errorf("b0.Text: want \"Gutschrift von Kunde X\", got %q", b0.Text)
	}
	if b0.Page != 0 {
		t.Errorf("b0.Page: want 0, got %d", b0.Page)
	}
	if b0.LineIdx != 1 {
		t.Errorf("b0.LineIdx: want 1, got %d", b0.LineIdx)
	}

	// Second: debit D, date 260110 → 10.01.2026
	b1 := bookings[1]
	if b1.IstGutschrift != false {
		t.Errorf("b1.IstGutschrift: want false, got true")
	}
	if b1.Betrag != 55.24 {
		t.Errorf("b1.Betrag: want 55.24, got %f", b1.Betrag)
	}
	if b1.Date != "10.01.2026" {
		t.Errorf("b1.Date: want \"10.01.2026\", got %q", b1.Date)
	}
	if b1.Text != "Lastschrift Lieferant Y" {
		t.Errorf("b1.Text: want \"Lastschrift Lieferant Y\", got %q", b1.Text)
	}
	if b1.LineIdx != 2 {
		t.Errorf("b1.LineIdx: want 2, got %d", b1.LineIdx)
	}
}
