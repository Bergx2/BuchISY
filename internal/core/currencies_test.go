package core

import "testing"

func TestCurrencyOptions(t *testing.T) {
	opts := CurrencyOptions()
	if len(opts) < 140 {
		t.Fatalf("want >=140 currencies, got %d", len(opts))
	}
	// EUR, USD, CAD, AUD must be the first four, in that order.
	wantTop := []string{"EUR", "USD", "CAD", "AUD"}
	for i, code := range wantTop {
		if CurrencyCodeFromOption(opts[i]) != code {
			t.Errorf("opts[%d] code = %q, want %q", i, CurrencyCodeFromOption(opts[i]), code)
		}
	}
	// round-trip
	if CurrencyCodeFromOption(CurrencyOptionForCode("USD")) != "USD" {
		t.Error("USD round-trip failed")
	}
	// unknown code passes through
	if CurrencyCodeFromOption(CurrencyOptionForCode("XXX")) != "XXX" {
		t.Error("unknown code should pass through")
	}
}
