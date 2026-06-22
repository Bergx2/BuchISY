package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// belegSuggestion is a single MatchSuggest entry collected during reconciliation.
type belegSuggestion struct {
	row       core.CSVRow
	candidate core.ScoredLine
	fileName  string
}

// matchConfig builds a core.MatchConfig from the current settings,
// falling back to defaults for any field left at zero.
func (a *App) matchConfig() core.MatchConfig {
	cfg := core.DefaultMatchConfig()
	if a.settings.MatchDateWindowDays > 0 {
		cfg.DateWindowDays = a.settings.MatchDateWindowDays
	}
	if a.settings.MatchForeignTolerancePct > 0 {
		cfg.ForeignTolerancePct = a.settings.MatchForeignTolerancePct
	}
	return cfg
}

// showBelegabgleich runs the reconciliation for the current month:
// auto-links unambiguous matches and presents the rest as a confirm-list.
func (a *App) showBelegabgleich() {
	rows, err := a.dbRepo.List(fmt.Sprintf("%04d", a.currentYear), fmt.Sprintf("%02d", int(a.currentMonth)))
	if err != nil {
		a.showError("Belegabgleich", err.Error())
		return
	}

	accountType := func(name string) string {
		for _, ba := range a.settings.BankAccounts {
			if ba.Name == name {
				return ba.AccountType
			}
		}
		return ""
	}

	autoLinked := 0
	var suggestions []belegSuggestion

	for _, row := range rows {
		// Skip already-linked and cash accounts.
		if row.BuchungRef != "" {
			continue
		}
		at := accountType(row.Bankkonto)
		if at != core.AccountTypeBank && at != core.AccountTypeCreditCard {
			continue
		}

		// Per-file matching: iterate statements, keep best outcome across files.
		bestKind := core.MatchNone
		var bestCandidate core.ScoredLine
		bestFile := ""
		autoCount := 0

		for _, name := range a.listStatements(row.Bankkonto) {
			fullPath := filepath.Join(a.statementFolder(row.Bankkonto), name)
			lines, err := core.ParseStatementBookings(fullPath)
			if err != nil {
				a.logger.Warn("Belegabgleich: parse statement %s: %v", name, err)
				continue
			}
			kind, cands := core.MatchInvoiceToStatement(row, lines, a.matchConfig())
			if kind == core.MatchNone || len(cands) == 0 {
				continue
			}
			top := cands[0]

			if kind == core.MatchAuto {
				autoCount++
			}

			// Prefer MatchAuto with a single candidate; else best MatchSuggest by top score.
			switch {
			case kind == core.MatchAuto && bestKind != core.MatchAuto:
				bestKind = kind
				bestCandidate = top
				bestFile = name
			case kind == core.MatchAuto && bestKind == core.MatchAuto:
				// Multiple MatchAuto matches across files — keep highest score.
				if top.Score > bestCandidate.Score {
					bestCandidate = top
					bestFile = name
				}
			case kind == core.MatchSuggest && bestKind == core.MatchNone:
				bestKind = kind
				bestCandidate = top
				bestFile = name
			case kind == core.MatchSuggest && bestKind == core.MatchSuggest:
				if top.Score > bestCandidate.Score {
					bestCandidate = top
					bestFile = name
				}
			}
		}

		// Two+ statement files each produced an unambiguous match for the same
		// invoice — that is ambiguous across files; never silently auto-link.
		if bestKind == core.MatchAuto && autoCount >= 2 {
			bestKind = core.MatchSuggest
		}

		switch bestKind {
		case core.MatchAuto:
			row.BuchungRef = core.BuchungRef{
				StatementFilename: bestFile,
				Page:              bestCandidate.Line.Page,
				LineIdx:           bestCandidate.Line.LineIdx,
			}.String()
			if err := a.dbRepo.Update(row.Jahr, row.Monat, row.Dateiname, row); err != nil {
				a.logger.Warn("Belegabgleich auto-link Update %s: %v", row.Dateiname, err)
			}
			autoLinked++
		case core.MatchSuggest:
			suggestions = append(suggestions, belegSuggestion{
				row:       row,
				candidate: bestCandidate,
				fileName:  bestFile,
			})
		}
	}

	// Refresh table so auto-linked rows show their new BuchungRef.
	a.loadInvoices()

	// Build Barkasse summary block (informational only).
	cashAccounts := a.cashAccounts()
	var cashBox *fyne.Container
	if len(cashAccounts) > 0 {
		cashBox = container.NewVBox()
		for _, acct := range cashAccounts {
			books, _ := core.LoadCashBooks(filepath.Join(a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth), "kassenbuch.json"))
			var book core.CashBook
			for _, b := range books {
				if b.Konto == acct {
					book = b
					break
				}
			}
			unc, closing := core.CashCoverage(book, a.cashInvoicesForMonth(acct, a.currentYear, a.currentMonth))
			closingStr := strings.Replace(fmt.Sprintf("%.2f", closing), ".", ",", 1)
			line := fmt.Sprintf("%s: %s %s €", acct, a.bundle.T("reconcile.cashBalance"), closingStr)
			if len(unc) > 0 {
				line += "  " + fmt.Sprintf(a.bundle.T("reconcile.cashUncovered"), len(unc))
			} else {
				line += "  " + a.bundle.T("reconcile.cashOk")
			}
			cashBox.Add(widget.NewLabel(line))
		}
	}

	// Build dialog content.
	var content fyne.CanvasObject

	if autoLinked == 0 && len(suggestions) == 0 {
		vbox := container.NewVBox(widget.NewLabel(a.bundle.T("reconcile.none")))
		if cashBox != nil {
			heading := widget.NewLabel(a.bundle.T("reconcile.cashHeading"))
			heading.TextStyle = fyne.TextStyle{Bold: true}
			vbox.Add(heading)
			vbox.Add(cashBox)
		}
		content = container.NewVScroll(vbox)
	} else {
		headerText := a.bundle.T("reconcile.summary", autoLinked, len(suggestions))
		header := widget.NewLabel(headerText)
		header.TextStyle = fyne.TextStyle{Bold: true}

		vbox := container.NewVBox(header)

		for i, s := range suggestions {
			// Capture loop vars for closure.
			idx := i
			sug := s

			amountVal := core.InvoiceEURAmount(sug.row)
			amountStr := strings.Replace(fmt.Sprintf("%.2f", amountVal), ".", ",", 1)
			rowLabel := fmt.Sprintf("%s  %s €  →  %s  (%s)",
				sug.row.Auftraggeber,
				amountStr,
				sug.candidate.Line.Display(),
				sug.fileName,
			)
			lbl := widget.NewLabel(rowLabel)
			lbl.Wrapping = fyne.TextWrapWord

			confirmBtn := widget.NewButton(a.bundle.T("reconcile.confirm"), nil)
			_ = idx // idx used via sug to avoid stale pointer

			confirmBtn.OnTapped = func() {
				sug.row.BuchungRef = core.BuchungRef{
					StatementFilename: sug.fileName,
					Page:              sug.candidate.Line.Page,
					LineIdx:           sug.candidate.Line.LineIdx,
				}.String()
				if err := a.dbRepo.Update(sug.row.Jahr, sug.row.Monat, sug.row.Dateiname, sug.row); err != nil {
					a.logger.Warn("Belegabgleich confirm Update %s: %v", sug.row.Dateiname, err)
				}
				confirmBtn.Disable()
				lbl.SetText("✓ " + rowLabel)
				a.loadInvoices()
			}

			row := container.NewBorder(nil, nil, nil, confirmBtn, lbl)
			vbox.Add(row)
		}

		if cashBox != nil {
			heading := widget.NewLabel(a.bundle.T("reconcile.cashHeading"))
			heading.TextStyle = fyne.TextStyle{Bold: true}
			vbox.Add(heading)
			vbox.Add(cashBox)
		}

		content = container.NewVScroll(vbox)
	}

	dlg := dialog.NewCustom(
		a.bundle.T("reconcile.title"),
		a.bundle.T("common.close"),
		content,
		a.window,
	)
	dlg.Resize(fyne.NewSize(640, 480))
	dlg.Show()
}
