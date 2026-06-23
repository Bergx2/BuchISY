package core

import "testing"

func TestAccountFormat(t *testing.T) {
	a := SKRAccount{Number: 4663, Name: "Reisekosten Arbeitnehmer, Fahrtkosten", Type: "expense"}
	if got := AccountDisplay(a); got != "4663 — Reisekosten Arbeitnehmer, Fahrtkosten" {
		t.Errorf("AccountDisplay = %q", got)
	}
	if got := AccountTooltip(a); got != "4663 — Reisekosten Arbeitnehmer, Fahrtkosten (Aufwand)" {
		t.Errorf("AccountTooltip = %q", got)
	}
	// unknown type → no parenthetical
	b := SKRAccount{Number: 1200, Name: "Sparkasse", Type: ""}
	if got := AccountTooltip(b); got != "1200 — Sparkasse" {
		t.Errorf("AccountTooltip(no type) = %q", got)
	}
}
