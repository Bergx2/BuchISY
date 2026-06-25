package ui

import (
	"fmt"
	"path/filepath"
	"sort"

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
	var cands []cand
	for _, name := range a.listStatements(row.Bankkonto) {
		lines, err := core.ParseStatementBookings(filepath.Join(a.statementFolder(row.Bankkonto), name))
		if err != nil {
			a.logger.Warn("Einzelabgleich: parse %s: %v", name, err)
			continue
		}
		kind, scored := core.MatchInvoiceToStatement(row, lines, cfg)
		if kind == core.MatchNone {
			continue
		}
		for _, sc := range scored {
			cands = append(cands, cand{file: name, line: sc.Line, score: sc.Score})
		}
	}

	if len(cands) == 0 {
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

	dlg = dialog.NewCustom("Mit Kontoauszug abgleichen", a.bundle.T("common.close"),
		container.NewVScroll(box), parent)
	dlg.Resize(fyne.NewSize(560, 420))
	dlg.Show()
}
