package ui

import "github.com/bergx2/buchisy/internal/core"

// bookingCategoryOptions returns display labels for the rule categories plus a
// map from label back to the category key.
func (a *App) bookingCategoryOptions() ([]string, map[string]string) {
	labels := make([]string, 0, len(a.bookingRules.Regeln))
	byLabel := map[string]string{}
	for _, r := range a.bookingRules.Regeln {
		label := r.Name
		if label == "" {
			label = r.Kategorie
		}
		labels = append(labels, label)
		byLabel[label] = r.Kategorie
	}
	return labels, byLabel
}

// bookingCategoryLabel maps a category key to its display label.
func (a *App) bookingCategoryLabel(kategorie string) string {
	for _, r := range a.bookingRules.Regeln {
		if r.Kategorie == kategorie {
			if r.Name != "" {
				return r.Name
			}
			return r.Kategorie
		}
	}
	return kategorie
}

// computeRevenueBooking builds the revenue booking for an outgoing invoice:
// Soll Zahlungskonto, Haben Erlöskonto + Umsatzsteuer. Returns (booking, ok, msg).
func (a *App) computeRevenueBooking(lines []core.TaxLine, revenueAccount int, bankAccountName string) (core.Booking, bool, string) {
	if len(lines) == 0 {
		return core.Booking{}, false, a.bundle.T("booking.no.lines")
	}
	payment, ok := a.settings.PaymentAccountSKR04(bankAccountName)
	if !ok {
		return core.Booking{}, false, a.bundle.T("booking.no.payment.account")
	}
	b, err := core.BuildRevenueBooking(a.bookingRules, lines, revenueAccount, payment)
	if err != nil {
		return core.Booking{}, false, err.Error()
	}
	return b, true, ""
}

// computeInvoiceBooking resolves the payment account and builds the booking.
// Returns (booking, bookable, reasonIfNotBookable).
func (a *App) computeInvoiceBooking(kategorie string, lines []core.TaxLine, trinkgeld float64, expenseAccount int, bankAccountName string, rabatt float64) (core.Booking, bool, string) {
	if len(lines) == 0 {
		return core.Booking{}, false, a.bundle.T("booking.no.lines")
	}
	payment, ok := a.settings.PaymentAccountSKR04(bankAccountName)
	if !ok {
		return core.Booking{}, false, a.bundle.T("booking.no.payment.account")
	}
	b, err := core.BuildBooking(a.bookingRules, kategorie, lines, trinkgeld, expenseAccount, payment, rabatt)
	if err != nil {
		return core.Booking{}, false, err.Error()
	}
	return b, true, ""
}
