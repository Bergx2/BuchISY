package core

import (
	"strings"
	"testing"
)

func TestStandardSKR_SKR03(t *testing.T) {
	accs, ok := StandardSKR("SKR03")
	if !ok {
		t.Fatal("StandardSKR(SKR03) returned ok=false")
	}
	if accs.Vorsteuer["19"] != 1576 {
		t.Errorf("Vorsteuer 19 = %d, want 1576", accs.Vorsteuer["19"])
	}
	if accs.Vorsteuer["7"] != 1571 {
		t.Errorf("Vorsteuer 7 = %d, want 1571", accs.Vorsteuer["7"])
	}
	if accs.Umsatzsteuer["19"] != 1776 {
		t.Errorf("Umsatzsteuer 19 = %d, want 1776", accs.Umsatzsteuer["19"])
	}
	if accs.Umsatzsteuer["7"] != 1771 {
		t.Errorf("Umsatzsteuer 7 = %d, want 1771", accs.Umsatzsteuer["7"])
	}
	if accs.VStRC != 1577 {
		t.Errorf("VStRC = %d, want 1577", accs.VStRC)
	}
	if accs.UStRC != 1787 {
		t.Errorf("UStRC = %d, want 1787", accs.UStRC)
	}
	if accs.BewAbz != 4650 {
		t.Errorf("BewAbz = %d, want 4650", accs.BewAbz)
	}
	if accs.BewNicht != 4654 {
		t.Errorf("BewNicht = %d, want 4654", accs.BewNicht)
	}
	if accs.ErloesInland != 8400 {
		t.Errorf("ErloesInland = %d, want 8400", accs.ErloesInland)
	}
	if accs.ErloesEU != 8341 {
		t.Errorf("ErloesEU = %d, want 8341", accs.ErloesEU)
	}
	if accs.ErloesDrittland != 8200 {
		t.Errorf("ErloesDrittland = %d, want 8200", accs.ErloesDrittland)
	}
}

func TestStandardSKR_SKR04(t *testing.T) {
	accs, ok := StandardSKR("SKR04")
	if !ok {
		t.Fatal("StandardSKR(SKR04) returned ok=false")
	}
	if accs.Vorsteuer["19"] != 1406 {
		t.Errorf("Vorsteuer 19 = %d, want 1406", accs.Vorsteuer["19"])
	}
	if accs.Vorsteuer["7"] != 1401 {
		t.Errorf("Vorsteuer 7 = %d, want 1401", accs.Vorsteuer["7"])
	}
	if accs.Umsatzsteuer["19"] != 3806 {
		t.Errorf("Umsatzsteuer 19 = %d, want 3806", accs.Umsatzsteuer["19"])
	}
	if accs.Umsatzsteuer["7"] != 3801 {
		t.Errorf("Umsatzsteuer 7 = %d, want 3801", accs.Umsatzsteuer["7"])
	}
	if accs.VStRC != 1407 {
		t.Errorf("VStRC = %d, want 1407", accs.VStRC)
	}
	if accs.UStRC != 3837 {
		t.Errorf("UStRC = %d, want 3837", accs.UStRC)
	}
	if accs.BewAbz != 6640 {
		t.Errorf("BewAbz = %d, want 6640", accs.BewAbz)
	}
	if accs.BewNicht != 6644 {
		t.Errorf("BewNicht = %d, want 6644", accs.BewNicht)
	}
	if accs.ErloesInland != 4400 {
		t.Errorf("ErloesInland = %d, want 4400", accs.ErloesInland)
	}
	if accs.ErloesEU != 4125 {
		t.Errorf("ErloesEU = %d, want 4125", accs.ErloesEU)
	}
	if accs.ErloesDrittland != 4120 {
		t.Errorf("ErloesDrittland = %d, want 4120", accs.ErloesDrittland)
	}
}

func TestStandardSKR_Unknown(t *testing.T) {
	_, ok := StandardSKR("SKR99")
	if ok {
		t.Error("StandardSKR(SKR99) should return ok=false")
	}
}

// makeTestRulesSKR04 returns a minimal BookingRules with SKR04 accounts and
// bewirtung + reverse_charge rules present.
func makeTestRulesSKR04() *BookingRules {
	return &BookingRules{
		VorsteuerKonten: map[string]int{
			"19": 1406,
			"7":  1401,
		},
		UmsatzsteuerKonten: map[string]int{
			"19": 3806,
			"7":  3801,
		},
		ErloesKonten: map[string]int{
			"inland":    4400,
			"eu":        4125,
			"drittland": 4120,
		},
		Regeln: []BookingRule{
			{
				Kategorie:           "bewirtung",
				Name:                "Bewirtungskosten",
				AbziehbarProzent:    70,
				KontoAbziehbar:      6640,
				KontoNichtAbziehbar: 6644,
			},
			{
				Kategorie:  "reverse_charge",
				Name:       "Reverse Charge §13b",
				RcSatz:     19,
				KontoVStRC: 1407,
				KontoUStRC: 3837,
			},
		},
	}
}

func TestApplySKRVariant_SKR03(t *testing.T) {
	rules := makeTestRulesSKR04()
	applied := ApplySKRVariant(rules, "SKR03")

	// Vorsteuer maps should be updated.
	if applied.VorsteuerKonten["19"] != 1576 {
		t.Errorf("VorsteuerKonten[19] = %d, want 1576", applied.VorsteuerKonten["19"])
	}
	if applied.VorsteuerKonten["7"] != 1571 {
		t.Errorf("VorsteuerKonten[7] = %d, want 1571", applied.VorsteuerKonten["7"])
	}

	// Umsatzsteuer maps.
	if applied.UmsatzsteuerKonten["19"] != 1776 {
		t.Errorf("UmsatzsteuerKonten[19] = %d, want 1776", applied.UmsatzsteuerKonten["19"])
	}

	// Erloes maps.
	if applied.ErloesKonten["inland"] != 8400 {
		t.Errorf("ErloesKonten[inland] = %d, want 8400", applied.ErloesKonten["inland"])
	}
	if applied.ErloesKonten["eu"] != 8341 {
		t.Errorf("ErloesKonten[eu] = %d, want 8341", applied.ErloesKonten["eu"])
	}
	if applied.ErloesKonten["drittland"] != 8200 {
		t.Errorf("ErloesKonten[drittland] = %d, want 8200", applied.ErloesKonten["drittland"])
	}

	// Bewirtung rule.
	bew, ok := applied.Rule("bewirtung")
	if !ok {
		t.Fatal("bewirtung rule missing after apply")
	}
	if bew.KontoAbziehbar != 4650 {
		t.Errorf("bewirtung.KontoAbziehbar = %d, want 4650", bew.KontoAbziehbar)
	}
	if bew.KontoNichtAbziehbar != 4654 {
		t.Errorf("bewirtung.KontoNichtAbziehbar = %d, want 4654", bew.KontoNichtAbziehbar)
	}
	// Non-account fields preserved.
	if bew.AbziehbarProzent != 70 {
		t.Errorf("bewirtung.AbziehbarProzent = %f, want 70", bew.AbziehbarProzent)
	}

	// Reverse charge rule.
	rc, ok := applied.Rule("reverse_charge")
	if !ok {
		t.Fatal("reverse_charge rule missing after apply")
	}
	if rc.KontoVStRC != 1577 {
		t.Errorf("reverse_charge.KontoVStRC = %d, want 1577", rc.KontoVStRC)
	}
	if rc.KontoUStRC != 1787 {
		t.Errorf("reverse_charge.KontoUStRC = %d, want 1787", rc.KontoUStRC)
	}
	// Non-account field preserved.
	if rc.RcSatz != 19 {
		t.Errorf("reverse_charge.RcSatz = %f, want 19", rc.RcSatz)
	}

	// Original must NOT be mutated.
	if rules.VorsteuerKonten["19"] != 1406 {
		t.Error("original rules.VorsteuerKonten[19] was mutated")
	}
}

func TestApplySKRVariant_CreatesReverseChargeIfAbsent(t *testing.T) {
	rules := &BookingRules{
		VorsteuerKonten:    map[string]int{"19": 1406},
		UmsatzsteuerKonten: map[string]int{"19": 3806},
		ErloesKonten:       map[string]int{"inland": 4400},
		Regeln:             []BookingRule{},
	}
	applied := ApplySKRVariant(rules, "SKR03")
	rc, ok := applied.Rule("reverse_charge")
	if !ok {
		t.Fatal("reverse_charge rule should be created when absent")
	}
	if rc.KontoVStRC != 1577 {
		t.Errorf("KontoVStRC = %d, want 1577", rc.KontoVStRC)
	}
	if rc.KontoUStRC != 1787 {
		t.Errorf("KontoUStRC = %d, want 1787", rc.KontoUStRC)
	}
}

func TestValidateBookingAccounts(t *testing.T) {
	// Small chart: only has a few accounts.
	chart := NewChartOfAccounts([]SKRAccount{
		{Number: 1576, Name: "Vorsteuer 19%"},
		{Number: 1571, Name: "Vorsteuer 7%"},
		{Number: 8400, Name: "Erlöse Inland"},
	})

	rules := &BookingRules{
		VorsteuerKonten: map[string]int{
			"19": 1576,
			"7":  1571,
		},
		UmsatzsteuerKonten: map[string]int{
			"19": 1776, // NOT in chart
		},
		ErloesKonten: map[string]int{
			"inland": 8400,
			"eu":     8341, // NOT in chart
		},
		Regeln: []BookingRule{
			{
				Kategorie:           "bewirtung",
				KontoAbziehbar:      4650, // NOT in chart
				KontoNichtAbziehbar: 0,    // zero = skip
			},
			{
				Kategorie:  "reverse_charge",
				KontoVStRC: 1576, // in chart
				KontoUStRC: 1787, // NOT in chart
			},
		},
	}

	issues := ValidateBookingAccounts(rules, chart)
	// Expect: 1776 (Umsatzsteuer 19), 8341 (Erlös eu), 4650 (bewirtung KontoAbziehbar), 1787 (reverse_charge KontoUStRC)
	if len(issues) != 4 {
		t.Errorf("expected 4 issues, got %d: %v", len(issues), issues)
	}
	// Check that each missing account number appears in at least one issue.
	for _, missing := range []int{1776, 8341, 4650, 1787} {
		found := false
		for _, iss := range issues {
			if strings.Contains(iss, "1776") && missing == 1776 {
				found = true
				break
			}
			if strings.Contains(iss, "8341") && missing == 8341 {
				found = true
				break
			}
			if strings.Contains(iss, "4650") && missing == 4650 {
				found = true
				break
			}
			if strings.Contains(iss, "1787") && missing == 1787 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("account %d not mentioned in issues: %v", missing, issues)
		}
	}
}

func TestValidateBookingAccounts_NoIssues(t *testing.T) {
	chart := NewChartOfAccounts([]SKRAccount{
		{Number: 1576, Name: "Vorsteuer 19%"},
		{Number: 1571, Name: "Vorsteuer 7%"},
		{Number: 1776, Name: "Umsatzsteuer 19%"},
	})
	rules := &BookingRules{
		VorsteuerKonten:    map[string]int{"19": 1576, "7": 1571},
		UmsatzsteuerKonten: map[string]int{"19": 1776},
	}
	issues := ValidateBookingAccounts(rules, chart)
	if len(issues) != 0 {
		t.Errorf("expected no issues, got: %v", issues)
	}
}

func TestDetectSKRVariant(t *testing.T) {
	// Chart with SKR03 marker account 1576.
	skr03Chart := NewChartOfAccounts([]SKRAccount{
		{Number: 1576, Name: "Vorsteuer 19%"},
		{Number: 8400, Name: "Erlöse Inland"},
	})
	if got := DetectSKRVariant(skr03Chart); got != "SKR03" {
		t.Errorf("DetectSKRVariant(skr03) = %q, want SKR03", got)
	}

	// Chart with SKR04 marker account 1406.
	skr04Chart := NewChartOfAccounts([]SKRAccount{
		{Number: 1406, Name: "Vorsteuer 19%"},
		{Number: 4400, Name: "Erlöse Inland"},
	})
	if got := DetectSKRVariant(skr04Chart); got != "SKR04" {
		t.Errorf("DetectSKRVariant(skr04) = %q, want SKR04", got)
	}

	// Unknown chart.
	unknownChart := NewChartOfAccounts([]SKRAccount{
		{Number: 9999, Name: "Irgendwas"},
	})
	if got := DetectSKRVariant(unknownChart); got != "" {
		t.Errorf("DetectSKRVariant(unknown) = %q, want empty", got)
	}
}
