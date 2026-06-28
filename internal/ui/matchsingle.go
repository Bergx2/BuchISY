package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// matchInvoiceWithStatement reconciles a SINGLE invoice against its bank
// account's statements: it parses the statement lines, finds candidate matches,
// and presents them so the user can link one. On link it sets BuchungRef, saves,
// learns the alias and calls onLinked with the updated row. Used from the edit
// dialog so a receipt can be matched without opening the full Belegabgleich.
func (a *App) matchInvoiceWithStatement(row core.CSVRow, parent fyne.Window, onLinked func(core.CSVRow)) {
	// Only bank / credit-card receipts reconcile against a statement.
	at := ""
	for _, ba := range a.settings.BankAccounts {
		if ba.Name == row.Bankkonto {
			at = ba.AccountType
		}
	}
	if at != core.AccountTypeBank && at != core.AccountTypeCreditCard {
		a.showInfo("Abgleich", "Nur Bank-/Kreditkarten-Belege lassen sich mit einem Kontoauszug abgleichen.")
		return
	}

	cfg := a.matchConfig()
	sep := a.settings.DecimalSeparator

	type cand struct {
		file  string
		line  core.StatementBooking
		score float64
	}
	// splitCand is a 1→N match: several statement lines from one file whose sum
	// equals the receipt's gross (e.g. a fee statement settled as separate debits).
	type splitCand struct {
		file  string
		lines []core.StatementBooking
	}
	var cands []cand
	var splits []splitCand
	for _, name := range a.listStatements(row.Bankkonto) {
		lines, err := core.ParseStatementBookings(filepath.Join(a.statementFolder(row.Bankkonto), name))
		if err != nil {
			a.logger.Warn("Einzelabgleich: parse %s: %v", name, err)
			continue
		}
		if kind, scored := core.MatchInvoiceToStatement(row, lines, cfg); kind != core.MatchNone {
			for _, sc := range scored {
				cands = append(cands, cand{file: name, line: sc.Line, score: sc.Score})
			}
		}
		// 1→N: the receipt's gross may equal the sum of several debits.
		for _, sm := range core.FindSplitPayments([]core.CSVRow{row}, lines, cfg) {
			splits = append(splits, splitCand{file: name, lines: sm.Lines})
		}
	}

	if len(cands) == 0 && len(splits) == 0 {
		a.showInfo("Abgleich", fmt.Sprintf(
			"Keine passende Auszugszeile gefunden (%s, %s).\n\nPrüfe, ob der Kontoauszug für „%s\" importiert ist.",
			row.Rechnungsdatum, formatMoney(core.InvoiceEURAmount(row), "EUR", sep), row.Bankkonto))
		return
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })

	var dlg dialog.Dialog
	box := container.NewVBox(widget.NewLabel("Passende Auszugszeile wählen und verknüpfen:"))
	for _, c := range cands {
		c := c
		sign := "−"
		if c.line.IstGutschrift {
			sign = "+"
		}
		text := c.line.Text
		if r := []rune(text); len(r) > 60 {
			text = string(r[:60]) + "…"
		}
		label := fmt.Sprintf("%s   %s%s   %s", c.line.Date, sign, formatDecimal(c.line.Betrag, sep), text)
		linkBtn := widget.NewButton("Verknüpfen", func() {
			row.BuchungRef = core.BuchungRef{
				StatementFilename: c.file,
				Page:              c.line.Page,
				LineIdx:           c.line.LineIdx,
			}.String()
			if err := a.dbRepo.Update(row.Jahr, row.Monat, row.Dateiname, row); err != nil {
				a.showError("Abgleich", err.Error())
				return
			}
			if a.statementAliases != nil {
				a.statementAliases.Learn(row.Auftraggeber, c.line.Text)
				if err := a.statementAliases.Save(); err != nil {
					a.logger.Warn("Einzelabgleich: save aliases: %v", err)
				}
			}
			a.loadInvoices()
			a.showToast("✓ Mit Kontoauszug verknüpft")
			if dlg != nil {
				dlg.Hide()
			}
			if onLinked != nil {
				onLinked(row)
			}
		})
		linkBtn.Importance = widget.LowImportance
		box.Add(container.NewBorder(nil, nil, nil, linkBtn, newCopyableLabel(a.bundle, label)))
	}

	// 1→N split options: one button links ALL lines of the combination at once.
	if len(splits) > 0 {
		box.Add(widget.NewSeparator())
		box.Add(widget.NewLabel(fmt.Sprintf(
			"Aufteilung (mehrere Abbuchungen, Summe = %s):",
			formatMoney(core.InvoiceEURAmount(row), "EUR", sep))))
	}
	for _, s := range splits {
		s := s
		parts := make([]string, len(s.lines))
		refs := make([]core.BuchungRef, len(s.lines))
		for i, l := range s.lines {
			refs[i] = core.BuchungRef{StatementFilename: s.file, Page: l.Page, LineIdx: l.LineIdx}
			parts[i] = fmt.Sprintf("%s −%s", l.Date, formatDecimal(l.Betrag, sep))
		}
		label := fmt.Sprintf("%d Zeilen: %s  (%s)",
			len(s.lines), strings.Join(parts, " + "), filepath.Base(s.file))
		linkAllBtn := widget.NewButton("Alle verknüpfen", func() {
			row.BuchungRef = core.JoinBuchungRefs(refs)
			if err := a.dbRepo.Update(row.Jahr, row.Monat, row.Dateiname, row); err != nil {
				a.showError("Abgleich", err.Error())
				return
			}
			if a.statementAliases != nil && len(s.lines) > 0 {
				a.statementAliases.Learn(row.Auftraggeber, s.lines[0].Text)
				if err := a.statementAliases.Save(); err != nil {
					a.logger.Warn("Einzelabgleich: save aliases: %v", err)
				}
			}
			a.loadInvoices()
			a.showToast(fmt.Sprintf("✓ %d Auszugszeilen verknüpft", len(s.lines)))
			if dlg != nil {
				dlg.Hide()
			}
			if onLinked != nil {
				onLinked(row)
			}
		})
		linkAllBtn.Importance = widget.LowImportance
		box.Add(container.NewBorder(nil, nil, nil, linkAllBtn, newCopyableLabel(a.bundle, label)))
	}

	dlg = dialog.NewCustom("Mit Kontoauszug abgleichen", a.bundle.T("common.close"),
		container.NewVScroll(box), parent)
	dlg.Resize(fyne.NewSize(560, 420))
	dlg.Show()
}
