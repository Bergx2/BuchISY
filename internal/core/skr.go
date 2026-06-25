package core

import "fmt"

// SKRAccounts holds the standard tax/booking accounts of a chart variant.
type SKRAccounts struct {
	Vorsteuer    map[string]int // "19","7"
	Umsatzsteuer map[string]int
	VStRC, UStRC int // §13b reverse charge
	BewAbz, BewNicht int // Bewirtung abziehbar / nicht abziehbar
	ErloesInland, ErloesEU, ErloesDrittland int
}

// StandardSKR returns the standard account numbers for the given variant
// ("SKR03" or "SKR04"). Returns (zero, false) for unknown variants.
func StandardSKR(variant string) (SKRAccounts, bool) {
	switch variant {
	case "SKR03":
		return SKRAccounts{
			Vorsteuer:       map[string]int{"19": 1576, "7": 1571},
			Umsatzsteuer:    map[string]int{"19": 1776, "7": 1771},
			VStRC:           1577,
			UStRC:           1787,
			BewAbz:          4650,
			BewNicht:        4654,
			ErloesInland:    8400,
			ErloesEU:        8341,
			ErloesDrittland: 8200,
		}, true
	case "SKR04":
		return SKRAccounts{
			Vorsteuer:       map[string]int{"19": 1406, "7": 1401},
			Umsatzsteuer:    map[string]int{"19": 3806, "7": 3801},
			VStRC:           1407,
			UStRC:           3837,
			BewAbz:          6640,
			BewNicht:        6644,
			ErloesInland:    4400,
			ErloesEU:        4125,
			ErloesDrittland: 4120,
		}, true
	}
	return SKRAccounts{}, false
}

// ApplySKRVariant returns a deep copy of rules with all standard accounts set
// to the variant's: Vorsteuer/Umsatzsteuer/Erloes maps, plus the bewirtung
// and reverse_charge rule accounts. Other rule fields (percentages, names,
// DefaultKonto, etc.) are preserved from the original.
// Returns nil if the variant is unknown.
func ApplySKRVariant(rules *BookingRules, variant string) *BookingRules {
	accs, ok := StandardSKR(variant)
	if !ok {
		return nil
	}

	// Deep-copy VorsteuerKonten.
	vorsteuer := make(map[string]int, len(accs.Vorsteuer))
	for k, v := range accs.Vorsteuer {
		vorsteuer[k] = v
	}

	// Deep-copy UmsatzsteuerKonten (start from existing for unknown keys, then overwrite known).
	umsatzsteuer := make(map[string]int, len(rules.UmsatzsteuerKonten))
	for k, v := range rules.UmsatzsteuerKonten {
		umsatzsteuer[k] = v
	}
	for k, v := range accs.Umsatzsteuer {
		umsatzsteuer[k] = v
	}

	// Deep-copy ErloesKonten.
	erloes := make(map[string]int, len(rules.ErloesKonten))
	for k, v := range rules.ErloesKonten {
		erloes[k] = v
	}
	erloes["inland"] = accs.ErloesInland
	erloes["eu"] = accs.ErloesEU
	erloes["drittland"] = accs.ErloesDrittland

	// Deep-copy KontoStichwoerter.
	var stichwoerter map[string]int
	if rules.KontoStichwoerter != nil {
		stichwoerter = make(map[string]int, len(rules.KontoStichwoerter))
		for k, v := range rules.KontoStichwoerter {
			stichwoerter[k] = v
		}
	}

	// Deep-copy Regeln, updating bewirtung and reverse_charge accounts.
	regeln := make([]BookingRule, len(rules.Regeln))
	for i, r := range rules.Regeln {
		regeln[i] = r // shallow copy of value type is fine (no pointer fields)
		switch r.Kategorie {
		case "bewirtung":
			regeln[i].KontoAbziehbar = accs.BewAbz
			regeln[i].KontoNichtAbziehbar = accs.BewNicht
		case "reverse_charge":
			regeln[i].KontoVStRC = accs.VStRC
			regeln[i].KontoUStRC = accs.UStRC
		}
	}

	// If reverse_charge rule was absent, create it.
	hasRC := false
	for _, r := range rules.Regeln {
		if r.Kategorie == "reverse_charge" {
			hasRC = true
			break
		}
	}
	if !hasRC {
		regeln = append(regeln, BookingRule{
			Kategorie:  "reverse_charge",
			Name:       "Reverse Charge §13b",
			KontoVStRC: accs.VStRC,
			KontoUStRC: accs.UStRC,
		})
	}

	return &BookingRules{
		VorsteuerKonten:    vorsteuer,
		UmsatzsteuerKonten: umsatzsteuer,
		ErloesKonten:       erloes,
		ForderungsKonto:    rules.ForderungsKonto,
		KontoStichwoerter:  stichwoerter,
		Regeln:             regeln,
	}
}

// ValidateBookingAccounts returns human-readable issues for every account
// referenced by the rules that is NOT present in chart.
func ValidateBookingAccounts(rules *BookingRules, chart *ChartOfAccounts) []string {
	if rules == nil || chart == nil {
		return nil
	}

	var issues []string

	check := func(label string, konto int) {
		if konto == 0 {
			return
		}
		if _, ok := chart.Find(konto); !ok {
			issues = append(issues, fmt.Sprintf("%s: Konto %d nicht im Kontenrahmen", label, konto))
		}
	}

	// Vorsteuer map.
	for rate, konto := range rules.VorsteuerKonten {
		check(fmt.Sprintf("Vorsteuer %s%%", rate), konto)
	}

	// Umsatzsteuer map.
	for rate, konto := range rules.UmsatzsteuerKonten {
		check(fmt.Sprintf("Umsatzsteuer %s%%", rate), konto)
	}

	// Erloes map.
	for key, konto := range rules.ErloesKonten {
		check(fmt.Sprintf("Erlöskonto %s", key), konto)
	}

	// Per-rule konto fields.
	for _, r := range rules.Regeln {
		label := r.Kategorie
		if r.Name != "" {
			label = r.Name
		}
		check(fmt.Sprintf("%s KontoAbziehbar", label), r.KontoAbziehbar)
		check(fmt.Sprintf("%s KontoNichtAbziehbar", label), r.KontoNichtAbziehbar)
		check(fmt.Sprintf("%s KontoVStRC", label), r.KontoVStRC)
		check(fmt.Sprintf("%s KontoUStRC", label), r.KontoUStRC)
		check(fmt.Sprintf("%s DefaultKonto", label), r.DefaultKonto)
	}

	return issues
}

// DetectSKRVariant guesses the chart's variant from marker accounts:
// SKR03 has account 1576 (Vorsteuer 19%); SKR04 has account 1406.
// Returns "" if the variant cannot be determined.
func DetectSKRVariant(chart *ChartOfAccounts) string {
	if chart == nil {
		return ""
	}
	if _, ok := chart.Find(1576); ok {
		return "SKR03"
	}
	if _, ok := chart.Find(1406); ok {
		return "SKR04"
	}
	return ""
}
