package ui

import (
	"testing"
	"time"
)

func TestParseFilingYearMonth(t *testing.T) {
	cases := []struct {
		date   string
		wantY  int
		wantM  time.Month
		wantOK bool
	}{
		{"15.05.2026", 2026, time.May, true},
		{" 01.12.2025 ", 2025, time.December, true},
		{"31.1.2026", 2026, time.January, true}, // single-digit month
		{"", 0, 0, false},
		{"2026-05-15", 0, 0, false}, // wrong format
		{"15.13.2026", 0, 0, false}, // month out of range
		{"15.00.2026", 0, 0, false}, // month zero
		{"15.05", 0, 0, false},      // missing year
	}
	for _, c := range cases {
		y, m, ok := parseFilingYearMonth(c.date)
		if ok != c.wantOK || (ok && (y != c.wantY || m != c.wantM)) {
			t.Errorf("parseFilingYearMonth(%q) = (%d,%v,%v), want (%d,%v,%v)",
				c.date, y, m, ok, c.wantY, c.wantM, c.wantOK)
		}
	}
}
