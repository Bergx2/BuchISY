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
// carriedIn is the opening balance entering the first month, used when that
// month has no stored cash book. months are in calendar order, January
// first; each summary carries the matching time.Month (index 0 -> January).
// A month with a stored book uses that book's own opening balance; a month
// without one opens with the previous month's closing balance.
func ComputeYearOverview(carriedIn float64, months []MonthInput) []MonthSummary {
	summaries := make([]MonthSummary, len(months))
	running := carriedIn
	for i, mi := range months {
		anfang := running
		var book CashBook
		if mi.HasStoredBook {
			book = mi.Book
			anfang = book.Anfangsbestand
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
