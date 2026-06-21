package ui

import "testing"

func TestDatevPeriod(t *testing.T) {
	cases := []struct{ fy, fm, ty, tm int; von, bis string }{
		{2026, 6, 2026, 6, "20260601", "20260630"}, // June → 30 days
		{2026, 2, 2026, 2, "20260201", "20260228"}, // Feb 2026 → 28
		{2024, 2, 2024, 2, "20240201", "20240229"}, // Feb 2024 leap → 29
		{2026, 1, 2026, 12, "20260101", "20261231"}, // whole year
	}
	for _, c := range cases {
		von, bis := datevPeriod(c.fy, c.fm, c.ty, c.tm)
		if von != c.von || bis != c.bis {
			t.Errorf("datevPeriod(%d,%d,%d,%d) = %s,%s want %s,%s", c.fy, c.fm, c.ty, c.tm, von, bis, c.von, c.bis)
		}
	}
}
