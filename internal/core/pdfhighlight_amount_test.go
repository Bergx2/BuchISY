package core

import "testing"

func TestAmountDigits(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOk bool
	}{
		{"1234.56", "123456", true},  // dot decimal (our search form)
		{"1.234,56", "123456", true}, // German thousands + comma decimal
		{"573,15", "57315", true},    // comma decimal
		{"573.15", "57315", true},    // dot decimal
		{" 573.15 ", "57315", true},  // surrounding space
		{"0,00", "000", true},        // zero
		{"123.456", "", false},       // 3 decimals → NOT a 2-decimal amount (e.g. invoice no.)
		{"123456", "", false},        // no decimal separator
		{"abc", "", false},           // not numeric
		{"", "", false},              // empty
		{"12,5", "", false},          // 1 decimal
	}
	for _, c := range cases {
		got, ok := amountDigits(c.in)
		if ok != c.wantOk || got != c.want {
			t.Errorf("amountDigits(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.wantOk)
		}
	}
}
