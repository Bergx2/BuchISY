package core

import "strings"

// categoryKeywords maps booking categories to built-in, chart-independent
// detection keywords (case-insensitive substring match). They are deliberately
// conservative to avoid false positives; anything more supplier-specific is
// better handled per-profile via konto_stichwoerter (which the account-based
// path below picks up automatically). Order matters: the first category whose
// keyword matches AND that exists in the profile's rules wins, so more specific
// categories come first (e.g. a "Hotel-Restaurant" bill → Bewirtung).
var categoryKeywords = []struct {
	kategorie string
	keywords  []string
}{
	{"bewirtung", []string{"bewirtung", "restaurant", "gaststätte", "gaststaette", "geschäftsessen", "geschaeftsessen"}},
	{"reisekosten", []string{"übernachtung", "uebernachtung", "hotel", "bahnticket", "flugticket"}},
	{"kfz", []string{"tankstelle", "tankquittung"}},
	{"geschenke", []string{"geschenk"}},
}

// SuggestCategory proposes a booking category for a NEW invoice (or one being
// re-opened) so the entry dialog can pre-select e.g. "Bewirtung" — which
// triggers the automatic 70/30 split (§ 4 Abs. 5 EStG) — instead of defaulting
// to "standard". The user still sees and confirms the choice in the dropdown
// before saving. Returns "" when no special category is evident (caller keeps
// its default / learned template).
//
// Detection signals, in priority order:
//
//   - gegenkonto is the profile's Bewirtung account (abziehbar or nicht
//     abziehbar). Chart-aware (reads the "bewirtung" rule), so any
//     konto_stichwoerter keyword that maps to the Bewirtung account also
//     triggers it.
//   - the text (supplier name + Verwendungszweck) contains a built-in keyword
//     for a category that the profile actually has configured.
//
// Only categories present in rules are ever suggested, so the returned key
// always exists in the dropdown.
func (r *BookingRules) SuggestCategory(gegenkonto int, text string) (string, bool) {
	if r == nil {
		return "", false
	}

	// Account-based: the Gegenkonto is the Bewirtung account.
	if rule, ok := r.Rule("bewirtung"); ok && gegenkonto != 0 &&
		(gegenkonto == rule.KontoAbziehbar || gegenkonto == rule.KontoNichtAbziehbar) {
		return "bewirtung", true
	}

	// Keyword-based: scan the text for built-in category keywords.
	lower := strings.ToLower(text)
	for _, ck := range categoryKeywords {
		if _, ok := r.Rule(ck.kategorie); !ok {
			continue // category not configured in this profile → skip
		}
		for _, kw := range ck.keywords {
			if strings.Contains(lower, kw) {
				return ck.kategorie, true
			}
		}
	}
	return "", false
}
