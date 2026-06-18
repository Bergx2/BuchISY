package ui

import "testing"

func TestScanFileReady(t *testing.T) {
	cases := []struct {
		name       string
		prevSize   int64
		seenBefore bool
		curSize    int64
		handled    bool
		want       bool
	}{
		{"never seen before", 0, false, 100, false, false},
		{"size still changing", 80, true, 100, false, false},
		{"stable and unhandled", 100, true, 100, false, true},
		{"stable but already handled", 100, true, 100, true, false},
	}
	for _, c := range cases {
		got := scanFileReady(c.prevSize, c.seenBefore, c.curSize, c.handled)
		if got != c.want {
			t.Errorf("%s: scanFileReady() = %v, want %v", c.name, got, c.want)
		}
	}
}
