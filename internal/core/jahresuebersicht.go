package core

import "time"

// MonthInput is the per-month data fed to ComputeYearOverview.
type MonthInput struct {
	HasStoredBook bool     // whether a cash book is stored for this month
	Book          CashBook // valid only when HasStoredBook
	Invoices      []CSVRow // cash invoices booked to the account this month
}

// MonthSummary is one month's row of a year overview.
type MonthSummary struct {
	Month          time.Month
	Anfangsbestand float64
	Einnahmen      float64
	Ausgaben       float64
	Endbestand     float64
}

// ComputeYearOverview rolls the cash balance through a year's months.
// carriedIn is the opening balance entering the first month, used until a
// stored cash book anchors the balance. months are in calendar order, January
// first; each summary carries the matching time.Month (index 0 -> January).
//
// Cash carries continuously: only the FIRST stored book sets an explicit
// opening balance (the anchor). Every later month opens with the previous
// month's closing balance — a later stored book's own Anfangsbestand is
// ignored (its deposits/Einlagen still apply). This prevents a stray book
// saved with a 0 opening (e.g. created before the carry chain existed) from
// resetting the running balance to zero mid-year.
func ComputeYearOverview(carriedIn float64, months []MonthInput) []MonthSummary {
	summaries := make([]MonthSummary, len(months))
	running := carriedIn
	anchored := false
	for i, mi := range months {
		anfang := running
		var book CashBook
		if mi.HasStoredBook {
			book = mi.Book
			if !anchored {
				anfang = book.Anfangsbestand // first stored book anchors the balance
				anchored = true
			}
			book.Anfangsbestand = anfang // later months carry; keep the book's Einlagen
		} else {
			book = CashBook{Anfangsbestand: anfang}
		}
		entries, end := ComputeCashReport(book, mi.Invoices)
		var einnahmen, ausgaben float64
		for _, e := range entries {
			einnahmen += e.Einnahme
			ausgaben += e.Ausgabe
		}
		summaries[i] = MonthSummary{
			Month:          time.Month(i + 1),
			Anfangsbestand: anfang,
			Einnahmen:      einnahmen,
			Ausgaben:       ausgaben,
			Endbestand:     end,
		}
		running = end
	}
	return summaries
}
