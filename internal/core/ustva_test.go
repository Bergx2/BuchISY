package core

import "testing"

func TestComputeUStVA(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{
		"vorsteuer_konten":{"19":1576,"7":1571},
		"umsatzsteuer_konten":{"19":1776},
		"regeln":[{"kategorie":"reverse_charge","rc_satz":19,"konto_vst_rc":1577,"konto_ust_rc":1787}]
	}`))
	rows := []CSVRow{
		// expense 19%: Vorsteuer 1576 = 19
		{Buchung: Booking{Entries: []BookingEntry{{Konto: 4240, Betrag: 100, Soll: true}, {Konto: 1576, Betrag: 19, Soll: true}, {Konto: 1200, Betrag: 119, Soll: false}}}},
		// expense 7%: Vorsteuer 1571 = 3.50
		{Buchung: Booking{Entries: []BookingEntry{{Konto: 4140, Betrag: 50, Soll: true}, {Konto: 1571, Betrag: 3.50, Soll: true}, {Konto: 1200, Betrag: 53.50, Soll: false}}}},
		// revenue 19%: Umsatzsteuer 1776 = 1235
		{Buchung: Booking{Entries: []BookingEntry{{Konto: 1200, Betrag: 7735, Soll: true}, {Konto: 8400, Betrag: 6500, Soll: false}, {Konto: 1776, Betrag: 1235, Soll: false}}}},
		// §13b 19%: Vorsteuer 1577 = 87.86 (Soll), Umsatzsteuer 1787 = 87.86 (Haben)
		{Buchung: Booking{Entries: []BookingEntry{{Konto: 27, Betrag: 462.40, Soll: true}, {Konto: 1577, Betrag: 87.86, Soll: true}, {Konto: 1787, Betrag: 87.86, Soll: false}, {Konto: 1200, Betrag: 462.40, Soll: false}}}},
	}
	u := ComputeUStVA(rows, rules)

	// Umsatzsteuer geschuldet: 1235 (1776) + 87.86 (1787, §13b) = 1322.86
	if !almost(u.UmsatzsteuerGesamt, 1322.86) {
		t.Errorf("UmsatzsteuerGesamt = %v, want 1322.86", u.UmsatzsteuerGesamt)
	}
	// Vorsteuer abziehbar: 19 + 3.50 + 87.86 (1577) = 110.36
	if !almost(u.VorsteuerGesamt, 110.36) {
		t.Errorf("VorsteuerGesamt = %v, want 110.36", u.VorsteuerGesamt)
	}
	// Zahllast = 1322.86 - 110.36 = 1212.50
	if !almost(u.Zahllast, 1212.50) {
		t.Errorf("Zahllast = %v, want 1212.50", u.Zahllast)
	}

	// Umsatzsteuer: one line per account (1776, 1787) — both 19%.
	if len(u.Umsatzsteuer) != 2 {
		t.Fatalf("want 2 USt lines, got %d: %+v", len(u.Umsatzsteuer), u.Umsatzsteuer)
	}
	if u.Umsatzsteuer[0].Konto != 1776 || !almost(u.Umsatzsteuer[0].Betrag, 1235) {
		t.Errorf("USt[0] = %+v (want 1776 / 1235)", u.Umsatzsteuer[0])
	}
	if u.Umsatzsteuer[1].Konto != 1787 || !almost(u.Umsatzsteuer[1].Betrag, 87.86) {
		t.Errorf("USt[1] = %+v (want 1787 / 87.86)", u.Umsatzsteuer[1])
	}

	// Vorsteuer: 1571 (7%), 1576 (19%), 1577 (19%) — sorted by Satz then Konto.
	if len(u.Vorsteuer) != 3 {
		t.Fatalf("want 3 VSt lines, got %d: %+v", len(u.Vorsteuer), u.Vorsteuer)
	}
	if u.Vorsteuer[0].Satz != 7 || u.Vorsteuer[0].Konto != 1571 || !almost(u.Vorsteuer[0].Betrag, 3.50) {
		t.Errorf("VSt[0] = %+v (want 7%% 1571 3.50)", u.Vorsteuer[0])
	}
	if u.Vorsteuer[1].Konto != 1576 || !almost(u.Vorsteuer[1].Betrag, 19) {
		t.Errorf("VSt[1] = %+v (want 1576 19)", u.Vorsteuer[1])
	}
	if u.Vorsteuer[2].Konto != 1577 || !almost(u.Vorsteuer[2].Betrag, 87.86) {
		t.Errorf("VSt[2] = %+v (want 1577 87.86)", u.Vorsteuer[2])
	}
}
