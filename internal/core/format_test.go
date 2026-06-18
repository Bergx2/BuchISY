package core

import "testing"

func TestFormatAmount(t *testing.T) {
	cases := []struct {
		value float64
		sep   string
		want  string
	}{
		{15000, ",", "15.000,00"},
		{1234567.5, ",", "1.234.567,50"},
		{42, ",", "42,00"},
		{999, ",", "999,00"},
		{-1234, ",", "-1.234,00"},
		{0, ",", "0,00"},
		{15000, ".", "15,000.00"},
		{1234567.5, ".", "1,234,567.50"},
	}
	for _, c := range cases {
		got := FormatAmount(c.value, c.sep)
		if got != c.want {
			t.Errorf("FormatAmount(%v, %q) = %q, want %q", c.value, c.sep, got, c.want)
		}
	}
}
