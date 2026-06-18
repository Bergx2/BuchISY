package ui

import (
	"fyne.io/fyne/v2/widget"
)

// wireAmountAutoCompute hooks OnChanged into the four amount entries
// (Netto / MwSt% / MwSt-Betrag / Brutto) so that as soon as two
// of them have plausible (>0) values, the missing ones are filled in.
//
// Cases handled:
//   - Netto + MwSt%        → MwSt-Betrag + Brutto
//   - Brutto + MwSt%       → Netto + MwSt-Betrag
//   - Netto + Brutto       → MwSt-Betrag + MwSt% (from delta)
//   - Netto + MwSt-Betrag  → MwSt% + Brutto
//   - Brutto + MwSt-Betrag → Netto + MwSt% (from delta)
//
// A `suppress` flag prevents the cascade of SetText() calls from
// firing recompute again. Each entry's pre-existing OnChanged
// callback (e.g. filename-preview update) is preserved.
func wireAmountAutoCompute(net, vatPercent, vatAmount, gross *widget.Entry, sep string) {
	var suppress bool

	set := func(e *widget.Entry, v float64) {
		e.SetText(formatDecimal(v, sep))
	}

	recompute := func() {
		if suppress {
			return
		}
		n := parseFloat(net.Text, sep)
		p := parseFloat(vatPercent.Text, sep)
		a := parseFloat(vatAmount.Text, sep)
		g := parseFloat(gross.Text, sep)

		suppress = true
		defer func() { suppress = false }()

		switch {
		case n > 0 && p > 0 && (a == 0 || g == 0):
			newA := n * p / 100
			newG := n + newA
			if a == 0 {
				set(vatAmount, newA)
			}
			if g == 0 {
				set(gross, newG)
			}
		case g > 0 && p > 0 && (n == 0 || a == 0):
			newN := g / (1 + p/100)
			newA := g - newN
			if n == 0 {
				set(net, newN)
			}
			if a == 0 {
				set(vatAmount, newA)
			}
		case n > 0 && g > 0 && (a == 0 || p == 0) && g > n:
			newA := g - n
			if a == 0 {
				set(vatAmount, newA)
			}
			if p == 0 {
				set(vatPercent, (newA/n)*100)
			}
		case n > 0 && a > 0 && (g == 0 || p == 0):
			if g == 0 {
				set(gross, n+a)
			}
			if p == 0 {
				set(vatPercent, (a/n)*100)
			}
		case g > 0 && a > 0 && (n == 0 || p == 0) && g > a:
			newN := g - a
			if n == 0 {
				set(net, newN)
			}
			if p == 0 && newN > 0 {
				set(vatPercent, (a/newN)*100)
			}
		}
	}

	chain := func(e *widget.Entry) {
		prev := e.OnChanged
		e.OnChanged = func(s string) {
			if prev != nil {
				prev(s)
			}
			recompute()
		}
	}
	chain(net)
	chain(vatPercent)
	chain(vatAmount)
	chain(gross)
}
