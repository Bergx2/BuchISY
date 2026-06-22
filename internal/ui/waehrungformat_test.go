package ui

import "testing"

// TestFormatDecimalRoundTripComma verifies that formatDecimal + parseFloat form
// a correct round-trip when the decimal separator is "," (German default).
// Before the fix, netEUREntry and feeEntry were written with fmt.Sprintf("%.2f",
// x) — always a literal dot — but read back with parseFloat(text, ",") which
// strips dots as thousands separators, corrupting e.g. 76.99 → 7699.
func TestFormatDecimalRoundTripComma(t *testing.T) {
	sep := ","

	cases := []struct {
		input float64
		want  string
	}{
		{76.99, "76,99"},
		{1.54, "1,54"},
		{0.00, "0,00"},
		// FormatAmount inserts thousands separator: 1234.56 → "1.234,56"
		// parseFloat strips the "." thousands sep on read-back → correct round-trip.
		{1234.56, "1.234,56"},
	}

	for _, tc := range cases {
		got := formatDecimal(tc.input, sep)
		if got != tc.want {
			t.Errorf("formatDecimal(%v, %q) = %q, want %q", tc.input, sep, got, tc.want)
		}

		// Round-trip: what parseFloat reads back on save must equal the original.
		roundTrip := parseFloat(got, sep)
		if roundTrip != tc.input {
			t.Errorf("round-trip: formatDecimal(%v, %q)=%q parseFloat=%v, want %v",
				tc.input, sep, got, roundTrip, tc.input)
		}
	}
}

// TestFormatDecimalRoundTripDot verifies the round-trip with "." separator.
func TestFormatDecimalRoundTripDot(t *testing.T) {
	sep := "."

	cases := []struct {
		input float64
		want  string
	}{
		{76.99, "76.99"},
		{1.54, "1.54"},
	}

	for _, tc := range cases {
		got := formatDecimal(tc.input, sep)
		if got != tc.want {
			t.Errorf("formatDecimal(%v, %q) = %q, want %q", tc.input, sep, got, tc.want)
		}

		roundTrip := parseFloat(got, sep)
		if roundTrip != tc.input {
			t.Errorf("round-trip: formatDecimal(%v, %q)=%q parseFloat=%v, want %v",
				tc.input, sep, got, roundTrip, tc.input)
		}
	}
}
