package core

import (
	"fmt"
	"math"
	"strings"
)

// FormatAmount formats a monetary value with two decimal places, the given
// decimal separator ("," or "."), and a thousands separator grouping the
// integer part in threes. The thousands separator is whichever of "." / ","
// is not the decimal separator. A negative sign is kept in front.
func FormatAmount(value float64, decimalSep string) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Sprintf("%.2f", value)
	}
	thousandsSep := "."
	if decimalSep != "," {
		decimalSep = "."
		thousandsSep = ","
	}

	s := fmt.Sprintf("%.2f", value) // always "-?<digits>.dd"
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart := s[:len(s)-3]  // digits before ".dd"
	fracPart := s[len(s)-2:] // the two decimals

	var b strings.Builder
	for i, d := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteString(thousandsSep)
		}
		b.WriteRune(d)
	}

	result := b.String() + decimalSep + fracPart
	if neg {
		result = "-" + result
	}
	return result
}
