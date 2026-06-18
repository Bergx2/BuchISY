package ui

import "testing"

func TestJoinRowValues(t *testing.T) {
	if got := joinRowValues([]string{"AWS", "37.64", "EUR"}); got != "AWS\t37.64\tEUR" {
		t.Errorf("joinRowValues = %q, want tab-joined", got)
	}
	if got := joinRowValues(nil); got != "" {
		t.Errorf("joinRowValues(nil) = %q, want empty", got)
	}
	if got := joinRowValues([]string{"only"}); got != "only" {
		t.Errorf("joinRowValues single = %q", got)
	}
}
