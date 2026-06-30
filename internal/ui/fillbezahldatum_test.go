package ui

import (
	"testing"

	"github.com/bergx2/buchisy/internal/core"
)

func TestFillBezahldatumIfEmpty(t *testing.T) {
	cases := []struct {
		name     string
		jahr     string
		existing string
		lineDate string
		want     string
	}{
		{"empty filled from full date", "2026", "", "24.02.2026", "24.02.2026"},
		{"already set stays", "2026", "01.01.2026", "24.02.2026", "01.01.2026"},
		{"missing year completed", "2026", "", "03.03", "03.03.2026"},
		{"trailing-dot year completed", "2026", "", "03.03.", "03.03.2026"},
		{"empty line keeps empty", "2026", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := core.CSVRow{Jahr: c.jahr, Bezahldatum: c.existing}
			fillBezahldatumIfEmpty(&r, c.lineDate)
			if r.Bezahldatum != c.want {
				t.Errorf("Bezahldatum = %q, want %q", r.Bezahldatum, c.want)
			}
		})
	}
}
