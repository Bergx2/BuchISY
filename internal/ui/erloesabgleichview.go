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

// showErloesAbgleich runs revenue reconciliation for the current month:
// auto-links unambiguous incoming credit matches and presents ambiguous ones
// as a confirm-list. This is the mirror of showBelegabgleich, but filtered to
// Ausgangsrechnungen and bank CREDIT lines (IstGutschrift==true).
//
// SCOPE NOTE: grouped (n:1) and partial (1:n) payment detection, as well as
// Claude re-ranking of ambiguous suggestions, are intentionally deferred to a
// future iteration. Auto-link + confirm-suggestions is the deliverable here.
func (a *App) showErloesAbgleich() {
	year := fmt.Sprintf("%04d", a.currentYear)
	month := fmt.Sprintf("%02d", int(a.currentMonth))

	rows, err := a.dbRepo.List(year, month)
	if err != nil {
		a.showError(a.bundle.T("erloesabgleich.title"), err.Error())
		return
	}

	// accountType returns the AccountType string for a named bank/cash account.
	accountType := func(name string) string {
		for _, ba := range a.settings.BankAccounts {
			if ba.Name == name {
				return ba.AccountType
			}
		}
		return ""
	}

	// Hoist matchConfig so it is computed once and reused in all inner loops.
	cfg := a.matchConfig()

	// ── Step 1: Build parse-once cache per account ───────────────────────────
	// For each unique bank account referenced by the unlinked Ausgangsrechnungen,
	// parse every statement file exactly once and cache all lines tagged with
	// their source file.

	type matchResult struct {
		row        core.CSVRow
		kind       core.MatchKind
		candidates []scoredWithFile
	}

	stmtCache := map[string][]stmtLine{}
	cacheBuilt := map[string]bool{}

	ensureCache := func(acct string) {
		if cacheBuilt[acct] {
			return
		}
		cacheBuilt[acct] = true
		for _, name := range a.listStatements(acct) {
			fullPath := filepath.Join(a.statementFolder(acct), name)
			lines, err := core.ParseStatementBookings(fullPath)
			if err != nil {
				a.logger.Warn("ErloesAbgleich: parse statement %s: %v", name, err)
				continue
			}
			for _, l := range lines {
				stmtCache[acct] = append(stmtCache[acct], stmtLine{File: name, Line: l})
			}
		}
	}

	// ── Step 2: Compute match results for every unlinked Ausgangsrechnung ────
	var allResults []matchResult

	for _, row := range rows {
		// Only outgoing invoices, not yet linked, on a bank account.
		if !row.Ausgangsrechnung || row.BuchungRef != "" {
			continue
		}
		if accountType(row.Bankkonto) != core.AccountTypeBank {
			continue
		}

		ensureCache(row.Bankkonto)
		cached := stmtCache[row.Bankkonto]

		// Group cached lines by file, preserving encounter order.
		fileLines := map[string][]core.StatementBooking{}
		fileOrder := []string{}
		for _, sl := range cached {
			if _, seen := fileLines[sl.File]; !seen {
				fileOrder = append(fileOrder, sl.File)
			}
			fileLines[sl.File] = append(fileLines[sl.File], sl.Line)
		}

		bestKind := core.MatchNone
		var bestCandidates []scoredWithFile
		autoCount := 0

		for _, name := range fileOrder {
			linesForFile := fileLines[name]
			kind, cands := core.MatchRevenueToStatement(row, linesForFile, cfg)
			if kind == core.MatchNone || len(cands) == 0 {
				continue
			}

			if kind == core.MatchAuto {
				autoCount++
			}

			// Convert []ScoredLine → []scoredWithFile, tagging each with its file.
			swf := make([]scoredWithFile, len(cands))
			for i, c := range cands {
				swf[i] = scoredWithFile{scored: c, file: name}
			}

			switch {
			case kind == core.MatchAuto && bestKind != core.MatchAuto:
				bestKind = kind
				bestCandidates = swf
			case kind == core.MatchAuto && bestKind == core.MatchAuto:
				if cands[0].Score > bestCandidates[0].scored.Score {
					bestCandidates = swf
				}
			case kind == core.MatchSuggest && bestKind == core.MatchNone:
				bestKind = kind
				bestCandidates = swf
			case kind == core.MatchSuggest && bestKind == core.MatchSuggest:
				// Accumulate candidates from multiple suggest files; deduplicate.
				seen := map[string]bool{}
				for _, c := range bestCandidates {
					key := core.BuchungRef{StatementFilename: c.file, Page: c.scored.Line.Page, LineIdx: c.scored.Line.LineIdx}.String()
					seen[key] = true
				}
				for _, c := range swf {
					key := core.BuchungRef{StatementFilename: c.file, Page: c.scored.Line.Page, LineIdx: c.scored.Line.LineIdx}.String()
					if !seen[key] {
						bestCandidates = append(bestCandidates, c)
						seen[key] = true
					}
				}
			}
		}

		// Two+ statement files each returned an unambiguous auto match for the
		// same invoice — that is cross-file ambiguity; downgrade to suggest.
		if bestKind == core.MatchAuto && autoCount >= 2 {
			bestKind = core.MatchSuggest
		}

		// Sort accumulated suggest candidates best-first.
		if bestKind == core.MatchSuggest && len(bestCandidates) > 1 {
			sort.SliceStable(bestCandidates, func(i, j int) bool {
				return bestCandidates[i].scored.Score > bestCandidates[j].scored.Score
			})
		}

		if bestKind == core.MatchNone || len(bestCandidates) == 0 {
			continue
		}

		allResults = append(allResults, matchResult{
			row:        row,
			kind:       bestKind,
			candidates: bestCandidates,
		})
	}

	// ── Step 3: Greedy auto-link by score; claim each credit line at most once ─
	refKey := func(file string, page, lineIdx int) string {
		return core.BuchungRef{StatementFilename: file, Page: page, LineIdx: lineIdx}.String()
	}

	claimed := map[string]bool{}

	var autoResults []matchResult
	var suggestResults []matchResult
	for _, r := range allResults {
		if r.kind == core.MatchAuto {
			autoResults = append(autoResults, r)
		} else {
			suggestResults = append(suggestResults, r)
		}
	}
	sort.SliceStable(autoResults, func(i, j int) bool {
		return autoResults[i].candidates[0].scored.Score > autoResults[j].candidates[0].scored.Score
	})

	autoLinked := 0

	for _, r := range autoResults {
		top := r.candidates[0]
		key := refKey(top.file, top.scored.Line.Page, top.scored.Line.LineIdx)
		if claimed[key] {
			// Line already claimed by a higher-scored invoice — demote to suggest.
			suggestResults = append(suggestResults, matchResult{
				row:        r.row,
				kind:       core.MatchSuggest,
				candidates: r.candidates,
			})
			continue
		}
		r.row.BuchungRef = core.BuchungRef{
			StatementFilename: top.file,
			Page:              top.scored.Line.Page,
			LineIdx:           top.scored.Line.LineIdx,
		}.String()
		if err := a.dbRepo.Update(r.row.Jahr, r.row.Monat, r.row.Dateiname, r.row); err != nil {
			a.logger.Warn("ErloesAbgleich auto-link Update %s: %v", r.row.Dateiname, err)
		}
		claimed[key] = true
		autoLinked++
	}

	// Build suggestion list: drop candidates whose line was already claimed.
	type erloessSuggestion struct {
		row        core.CSVRow
		candidates []scoredWithFile
	}
	var suggestions []erloessSuggestion
	for _, r := range suggestResults {
		var remaining []scoredWithFile
		for _, c := range r.candidates {
			k := refKey(c.file, c.scored.Line.Page, c.scored.Line.LineIdx)
			if !claimed[k] {
				remaining = append(remaining, c)
			}
		}
		if len(remaining) == 0 {
			continue
		}
		suggestions = append(suggestions, erloessSuggestion{
			row:        r.row,
			candidates: remaining,
		})
	}

	// Refresh table so auto-linked rows show their new BuchungRef.
	a.loadInvoices()

	// ── Step 4: Build dialog content ─────────────────────────────────────────
	var content fyne.CanvasObject

	if autoLinked == 0 && len(suggestions) == 0 {
		content = container.NewVScroll(
			container.NewVBox(widget.NewLabel(a.bundle.T("erloesabgleich.none"))),
		)
	} else {
		headerText := fmt.Sprintf(a.bundle.T("erloesabgleich.autolinked"), autoLinked)
		header := widget.NewLabel(headerText)
		header.TextStyle = fyne.TextStyle{Bold: true}

		vbox := container.NewVBox(header)

		for _, s := range suggestions {
			// Capture loop variable for closure safety.
			sug := s
			selIdx := 0

			top := sug.candidates[0]

			invEUR := core.InvoiceEURAmount(sug.row)
			invAmtStr := strings.Replace(fmt.Sprintf("%.2f", invEUR), ".", ",", 1)

			lineDate := top.scored.Line.Date
			lineBetragStr := strings.Replace(fmt.Sprintf("%.2f", top.scored.Line.Betrag), ".", ",", 1)

			lineRunes := []rune(top.scored.Line.Text)
			if len(lineRunes) > 60 {
				lineRunes = append(lineRunes[:57], []rune("…")...)
			}

			baseName := filepath.Base(top.file)

			// Label: <Auftraggeber>  <amount> €  →  S.<page+1> Z.<lineIdx> · <date> · <betrag> · <text> (<file>)
			rowLabel := fmt.Sprintf("%s  %s €  →  S.%d Z.%d · %s · %s € · %s  (%s)",
				sug.row.Auftraggeber,
				invAmtStr,
				top.scored.Line.Page+1,
				top.scored.Line.LineIdx,
				lineDate,
				lineBetragStr,
				string(lineRunes),
				baseName,
			)
			lbl := widget.NewLabel(rowLabel)
			lbl.Wrapping = fyne.TextWrapWord

			confirmBtn := widget.NewButton(a.bundle.T("erloesabgleich.confirm"), nil)

			confirmBtn.OnTapped = func() {
				// Guard against double-linking the same statement line.
				chosen := sug.candidates[selIdx]
				key := refKey(chosen.file, chosen.scored.Line.Page, chosen.scored.Line.LineIdx)
				if claimed[key] {
					lbl.SetText("⚠ " + a.bundle.T("reconcile.lineTaken"))
					confirmBtn.Disable()
					return
				}
				sug.row.BuchungRef = core.BuchungRef{
					StatementFilename: chosen.file,
					Page:              chosen.scored.Line.Page,
					LineIdx:           chosen.scored.Line.LineIdx,
				}.String()
				if err := a.dbRepo.Update(sug.row.Jahr, sug.row.Monat, sug.row.Dateiname, sug.row); err != nil {
					a.logger.Warn("ErloesAbgleich confirm Update %s: %v", sug.row.Dateiname, err)
				}
				claimed[key] = true
				confirmBtn.Disable()
				lbl.SetText("✓ " + rowLabel)
				a.loadInvoices()
			}

			// Build row widget. When ≥2 candidates exist, add a Select dropdown
			// so the user can choose which credit line to confirm.
			// Mirror the expense dialog's E12 fix: the label updates when the
			// dropdown selection changes to reflect the newly selected candidate.
			var rowWidget fyne.CanvasObject
			if len(sug.candidates) >= 2 {
				options := make([]string, len(sug.candidates))
				for i, c := range sug.candidates {
					runes := []rune(c.scored.Line.Text)
					if len(runes) > 60 {
						runes = append(runes[:57], []rune("…")...)
					}
					bStr := strings.Replace(fmt.Sprintf("%.2f", c.scored.Line.Betrag), ".", ",", 1)
					// 1-based index prefix ensures option labels are unique even when
					// two candidates render identically (E12 fix mirrors expense dialog).
					options[i] = fmt.Sprintf("[%d] S.%d Z.%d · %s · %s € · %s",
						i+1,
						c.scored.Line.Page+1,
						c.scored.Line.LineIdx,
						c.scored.Line.Date,
						bStr,
						string(runes),
					)
				}

				sel := widget.NewSelect(options, func(selected string) {
					for i, opt := range options {
						if opt == selected {
							selIdx = i
							break
						}
					}
					// Update the label to reflect the newly selected candidate
					// (E12 fix: dropdown change updates the label, mirroring expense dialog).
					chosen := sug.candidates[selIdx]
					newLineRunes := []rune(chosen.scored.Line.Text)
					if len(newLineRunes) > 60 {
						newLineRunes = append(newLineRunes[:57], []rune("…")...)
					}
					newBStr := strings.Replace(fmt.Sprintf("%.2f", chosen.scored.Line.Betrag), ".", ",", 1)
					newBase := filepath.Base(chosen.file)
					lbl.SetText(fmt.Sprintf("%s  %s €  →  S.%d Z.%d · %s · %s € · %s  (%s)",
						sug.row.Auftraggeber,
						invAmtStr,
						chosen.scored.Line.Page+1,
						chosen.scored.Line.LineIdx,
						chosen.scored.Line.Date,
						newBStr,
						string(newLineRunes),
						newBase,
					))
				})
				sel.SetSelected(options[0])

				rowWidget = container.NewBorder(nil, nil, nil, confirmBtn,
					container.NewVBox(lbl, sel))
			} else {
				rowWidget = container.NewBorder(nil, nil, nil, confirmBtn, lbl)
			}

			vbox.Add(rowWidget)
		}

		content = container.NewVScroll(vbox)
	}

	dlg := dialog.NewCustom(
		a.bundle.T("erloesabgleich.title"),
		a.bundle.T("common.close"),
		content,
		a.window,
	)
	dlg.Resize(fyne.NewSize(640, 480))
	dlg.Show()
}
