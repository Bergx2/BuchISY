package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// formatBookingLines renders each booking entry as one human-readable line.
// sep is the decimal separator ("," or ".").
func formatBookingLines(b core.Booking, chart *core.ChartOfAccounts, sep string) []string {
	lines := make([]string, 0, len(b.Entries))
	for _, e := range b.Entries {
		side := "Soll "
		if !e.Soll {
			side = "Haben"
		}
		name := ""
		if chart != nil {
			if acc, ok := chart.Find(e.Konto); ok {
				name = acc.Name
			}
		}
		amount := formatMoney(e.Betrag, "EUR", sep)
		lines = append(lines, strings.TrimSpace(fmt.Sprintf("%s  %d  %s  %s", side, e.Konto, name, amount)))
	}
	return lines
}

// bookingPreview is a read-only widget that shows the proposed Buchungssatz.
type bookingPreview struct {
	app       *App
	container *fyne.Container
	last      core.Booking // most recently shown booking (for warning checks)
}

func newBookingPreview(a *App) *bookingPreview {
	return &bookingPreview{app: a, container: container.NewVBox()}
}

// set replaces the preview content. When bookable is false, reason is shown.
func (p *bookingPreview) set(b core.Booking, bookable bool, reason string) {
	p.last = b
	p.container.RemoveAll()
	if !bookable {
		hint := widget.NewLabel(reason)
		hint.Wrapping = fyne.TextWrapWord
		p.container.Add(hint)
		p.container.Refresh()
		return
	}
	for _, line := range formatBookingLines(b, p.app.chart, p.app.settings.DecimalSeparator) {
		p.container.Add(widget.NewLabel(line))
	}
	status := p.app.bundle.T("booking.balanced")
	if !b.Balanced() {
		status = p.app.bundle.T("booking.unbalanced")
	}
	p.container.Add(widget.NewLabel(status))
	p.container.Refresh()
}
