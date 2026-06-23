package core

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// BookingRule describes how a booking category posts a receipt's net amounts.
// AbziehbarProzent / the two Konto fields are only used by categories that
// split (e.g. Bewirtung); they are zero for the plain "standard" rule.
type BookingRule struct {
	Kategorie           string  `json:"kategorie"`
	Name                string  `json:"name"`
	AbziehbarProzent    float64 `json:"abziehbar_prozent,omitempty"`
	KontoAbziehbar      int     `json:"konto_abziehbar,omitempty"`
	KontoNichtAbziehbar int     `json:"konto_nicht_abziehbar,omitempty"`
	RcSatz              float64 `json:"rc_satz,omitempty"`
	KontoVStRC          int     `json:"konto_vst_rc,omitempty"`
	KontoUStRC          int     `json:"konto_ust_rc,omitempty"`
	Schwelle            float64 `json:"schwelle,omitempty"`
	DefaultKonto        int     `json:"default_konto,omitempty"`
}

// BookingRules is the bundled rules base: Vorsteuer accounts keyed by integer
// percent ("19","7") and the list of category rules.
type BookingRules struct {
	VorsteuerKonten   map[string]int `json:"vorsteuer_konten"`
	UmsatzsteuerKonten map[string]int `json:"umsatzsteuer_konten,omitempty"`
	Regeln            []BookingRule  `json:"regeln"`
}

// ParseBookingRules decodes the rules base JSON.
func ParseBookingRules(data []byte) (*BookingRules, error) {
	var r BookingRules
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("failed to parse booking rules: %w", err)
	}
	return &r, nil
}

// Rule returns the rule for a category (case-sensitive key match).
func (r *BookingRules) Rule(kategorie string) (BookingRule, bool) {
	for _, rule := range r.Regeln {
		if rule.Kategorie == kategorie {
			return rule, true
		}
	}
	return BookingRule{}, false
}

// VorsteuerKonto returns the Vorsteuer account for a VAT rate (percent). The
// rate is matched as an integer key, so 19.0 → "19".
func (r *BookingRules) VorsteuerKonto(satzProzent float64) (int, bool) {
	k, ok := r.VorsteuerKonten[strconv.Itoa(int(satzProzent+0.5))]
	return k, ok
}

// UmsatzsteuerKonto returns the output-VAT account for a VAT rate (percent).
func (r *BookingRules) UmsatzsteuerKonto(satzProzent float64) (int, bool) {
	k, ok := r.UmsatzsteuerKonten[strconv.Itoa(int(satzProzent+0.5))]
	return k, ok
}
