package core

import "testing"

func TestBookingBalance(t *testing.T) {
	b := Booking{Entries: []BookingEntry{
		{Konto: 6640, Betrag: 12.71, Soll: true},
		{Konto: 6644, Betrag: 5.44, Soll: true},
		{Konto: 1406, Betrag: 1.26, Soll: true},
		{Konto: 1401, Betrag: 0.59, Soll: true},
		{Konto: 1800, Betrag: 20.00, Soll: false},
	}}
	if !almost(b.SollSum(), 20.00) || !almost(b.HabenSum(), 20.00) {
		t.Fatalf("sums: soll=%v haben=%v", b.SollSum(), b.HabenSum())
	}
	if !b.Balanced() {
		t.Error("should be balanced")
	}
	if (Booking{}).Balanced() {
		t.Error("empty booking is not balanced")
	}
	if !(Booking{}).IsEmpty() {
		t.Error("zero booking should be empty")
	}
}

func TestBuildBookingBewirtung(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{"vorsteuer_konten":{"19":1406,"7":1401},"regeln":[{"kategorie":"standard","name":"Standard"},{"kategorie":"bewirtung","name":"Bewirtung","abziehbar_prozent":70,"konto_abziehbar":6640,"konto_nicht_abziehbar":6644}]}`))
	lines := []TaxLine{
		{Netto: 6.64, SatzProzent: 19, MwStBetrag: 1.26},
		{Netto: 8.41, SatzProzent: 7, MwStBetrag: 0.59},
	}
	b, err := BuildBooking(rules, "bewirtung", lines, 3.10, 0, 1800)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Balanced() || !almost(b.HabenSum(), 20.00) {
		t.Fatalf("not balanced / haben != 20: %+v (haben=%v)", b, b.HabenSum())
	}
	got := map[int]float64{}
	for _, e := range b.Entries {
		if e.Soll {
			got[e.Konto] += e.Betrag
		}
	}
	// net+trinkgeld = 18.15; 70% = 12.71 (6640), remainder 5.44 (6644); VSt 1.26/0.59.
	if !almost(got[6640], 12.71) || !almost(got[6644], 5.44) {
		t.Errorf("split wrong: 6640=%v 6644=%v", got[6640], got[6644])
	}
	if !almost(got[1406], 1.26) || !almost(got[1401], 0.59) {
		t.Errorf("vorsteuer wrong: 1406=%v 1401=%v", got[1406], got[1401])
	}
}

func TestBuildBookingStandard(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{"vorsteuer_konten":{"19":1406},"regeln":[{"kategorie":"standard","name":"Standard"}]}`))
	lines := []TaxLine{{Netto: 100, SatzProzent: 19, MwStBetrag: 19}}
	b, err := BuildBooking(rules, "standard", lines, 0, 6815, 1800)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]float64{}
	for _, e := range b.Entries {
		if e.Soll {
			got[e.Konto] += e.Betrag
		}
	}
	if !almost(got[6815], 100) || !almost(got[1406], 19) || !almost(b.HabenSum(), 119) {
		t.Errorf("standard booking wrong: %+v", b)
	}
	if !b.Balanced() {
		t.Errorf("standard booking not balanced: %+v", b)
	}
	if _, err := BuildBooking(rules, "unbekannt", lines, 0, 6815, 1800); err == nil {
		t.Error("unknown category should error")
	}
}

// TestBuildBookingBalancesWithMissingVorsteuerAccount verifies that a booking
// still balances when a tax line's rate has no registered Vorsteuer account.
// In this case its VAT is not posted to Soll, so Haben must equal the sum of
// the actual Soll entries (not the raw gross which would be higher).
func TestBuildBookingBalancesWithMissingVorsteuerAccount(t *testing.T) {
	// Only 19% has a Vorsteuer account; 5% does NOT.
	rules, _ := ParseBookingRules([]byte(`{"vorsteuer_konten":{"19":1406},"regeln":[{"kategorie":"standard","name":"S"}]}`))
	lines := []TaxLine{
		{Netto: 100, SatzProzent: 19, MwStBetrag: 19},
		{Netto: 50, SatzProzent: 5, MwStBetrag: 2.50},
	}
	b, err := BuildBooking(rules, "standard", lines, 0, 6815, 1800)
	if err != nil {
		t.Fatal(err)
	}
	// Soll: 6815=150 (net total), 1406=19 (19% VAT only; 5% VAT has no account)
	// Haben must equal Σ Soll = 169, NOT the raw gross 171.50.
	if !b.Balanced() {
		t.Errorf("booking with missing Vorsteuer account not balanced: soll=%v haben=%v entries=%+v",
			b.SollSum(), b.HabenSum(), b.Entries)
	}
	if !almost(b.HabenSum(), 169) {
		t.Errorf("haben = %v, want 169 (= Σ Soll, not raw gross 171.50)", b.HabenSum())
	}
}

func TestBookingPaymentSplit(t *testing.T) {
	b := Booking{Entries: []BookingEntry{
		{Konto: 6640, Betrag: 12.71, Soll: true},
		{Konto: 6644, Betrag: 5.44, Soll: true},
		{Konto: 1800, Betrag: 18.15, Soll: false},
	}}
	pay, ok := b.PaymentEntry()
	if !ok || pay.Konto != 1800 {
		t.Fatalf("payment = %+v ok=%v", pay, ok)
	}
	if len(b.DebitEntries()) != 2 {
		t.Errorf("want 2 debit entries, got %d", len(b.DebitEntries()))
	}
	// two Haben → not a clean single-payment booking
	b2 := Booking{Entries: []BookingEntry{{Konto: 1, Betrag: 1, Soll: false}, {Konto: 2, Betrag: 1, Soll: false}}}
	if _, ok := b2.PaymentEntry(); ok {
		t.Error("two Haben entries should yield ok=false")
	}
}

func TestBookingManuellRoundTrip(t *testing.T) {
	b := Booking{Manuell: true, Entries: []BookingEntry{{Konto: 6640, Betrag: 10, Soll: true}, {Konto: 1800, Betrag: 10, Soll: false}}}
	got := ParseBooking(MarshalBooking(b))
	if !got.Manuell {
		t.Error("Manuell flag did not round-trip")
	}
	if len(got.Entries) != 2 {
		t.Errorf("entries lost: %+v", got)
	}
	// an auto booking (Manuell=false) stays false
	if ParseBooking(MarshalBooking(Booking{Entries: b.Entries})).Manuell {
		t.Error("non-manual booking should stay false")
	}
}

func TestBuildBookingNewCategories(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{"vorsteuer_konten":{"19":1406,"7":1401},"regeln":[
		{"kategorie":"standard","name":"S"},
		{"kategorie":"reverse_charge","name":"RC","rc_satz":19,"konto_vst_rc":1407,"konto_ust_rc":3837},
		{"kategorie":"geschenke","name":"G","schwelle":35,"konto_abziehbar":6610,"konto_nicht_abziehbar":6620},
		{"kategorie":"reisekosten","name":"R","default_konto":6650}]}`))

	// reverse_charge: net 100 → expense 100, VSt§13b 19, USt§13b 19, payment 100; balanced.
	rc, err := BuildBooking(rules, "reverse_charge", []TaxLine{{Netto: 100, SatzProzent: 0, MwStBetrag: 0}}, 0, 6300, 1800)
	if err != nil || !rc.Balanced() {
		t.Fatalf("rc not balanced: %+v err=%v", rc, err)
	}
	got := sollByKonto(rc)
	if !almost(got[6300], 100) || !almost(got[1407], 19) {
		t.Errorf("rc soll: %+v", got)
	}
	if !almost(habenByKonto(rc)[3837], 19) || !almost(habenByKonto(rc)[1800], 100) {
		t.Errorf("rc haben: %+v", habenByKonto(rc))
	}

	// geschenke ≤ 35: net 20, VAT 3.80 → 6610=20, 1406=3.80, payment 23.80.
	g1, _ := BuildBooking(rules, "geschenke", []TaxLine{{Netto: 20, SatzProzent: 19, MwStBetrag: 3.80}}, 0, 0, 1800)
	if !g1.Balanced() || !almost(sollByKonto(g1)[6610], 20) || !almost(sollByKonto(g1)[1406], 3.80) {
		t.Errorf("geschenke≤35: %+v", g1)
	}

	// geschenke > 35: net 40, VAT 7.60 → 6620 = 47.60 (gross), no Vorsteuer, payment 47.60.
	g2, _ := BuildBooking(rules, "geschenke", []TaxLine{{Netto: 40, SatzProzent: 19, MwStBetrag: 7.60}}, 0, 0, 1800)
	if !g2.Balanced() || !almost(sollByKonto(g2)[6620], 47.60) || sollByKonto(g2)[1406] != 0 {
		t.Errorf("geschenke>35: %+v", g2)
	}

	// reisekosten: net 100, VAT 19 → 6650=100, 1406=19, payment 119 (ignores passed expenseAccount).
	r, _ := BuildBooking(rules, "reisekosten", []TaxLine{{Netto: 100, SatzProzent: 19, MwStBetrag: 19}}, 0, 9999, 1800)
	if !r.Balanced() || !almost(sollByKonto(r)[6650], 100) || sollByKonto(r)[9999] != 0 {
		t.Errorf("reisekosten: %+v", r)
	}
}

func sollByKonto(b Booking) map[int]float64 {
	m := map[int]float64{}
	for _, e := range b.Entries {
		if e.Soll {
			m[e.Konto] += e.Betrag
		}
	}
	return m
}

func habenByKonto(b Booking) map[int]float64 {
	m := map[int]float64{}
	for _, e := range b.Entries {
		if !e.Soll {
			m[e.Konto] += e.Betrag
		}
	}
	return m
}
