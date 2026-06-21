package ui

import (
	"strings"
	"testing"

	"github.com/bergx2/buchisy/internal/core"
)

func TestFormatBookingLines(t *testing.T) {
	chart := core.NewChartOfAccounts([]core.SKRAccount{
		{Number: 6640, Name: "Bewirtungskosten (abziehbar)"},
		{Number: 1800, Name: "Bank"},
	})
	b := core.Booking{Entries: []core.BookingEntry{
		{Konto: 6640, Betrag: 12.71, Soll: true},
		{Konto: 1800, Betrag: 12.71, Soll: false},
	}}
	lines := formatBookingLines(b, chart)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "Soll") || !strings.Contains(lines[0], "6640") ||
		!strings.Contains(lines[0], "Bewirtungskosten") || !strings.Contains(lines[0], "12,71") {
		t.Errorf("soll line wrong: %q", lines[0])
	}
	if !strings.Contains(lines[1], "Haben") || !strings.Contains(lines[1], "1800") {
		t.Errorf("haben line wrong: %q", lines[1])
	}
}
