package core

import "fmt"

// accountTypeWord maps an SKR account type to a German label for tooltips.
var accountTypeWord = map[string]string{
	"expense":   "Aufwand",
	"revenue":   "Erlös",
	"asset":     "Aktiva",
	"liability": "Passiva",
	"equity":    "Eigenkapital",
}

// AccountDisplay renders an account as "Nummer: Name" for compact cells
// (e.g. the Gegenkonto table column).
func AccountDisplay(a SKRAccount) string {
	return fmt.Sprintf("%d: %s", a.Number, a.Name)
}

// AccountTooltip renders "Nummer — Name (Typ)" for hover, omitting the type
// when unknown.
func AccountTooltip(a SKRAccount) string {
	if w, ok := accountTypeWord[a.Type]; ok {
		return fmt.Sprintf("%d — %s (%s)", a.Number, a.Name, w)
	}
	return fmt.Sprintf("%d — %s", a.Number, a.Name)
}
