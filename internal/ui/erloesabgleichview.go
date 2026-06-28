package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/zalando/go-keyring"

	"github.com/bergx2/buchisy/internal/core"
)

// showErloesAbgleich runs revenue reconciliation for the current month:
// auto-links unambiguous incoming credit matches and presents ambiguous ones
// as a confirm-list, with grouped (n:1) and partial (1:n) payment detection,
// alias learning, and Claude re-ranking for close-call suggestions.
// This mirrors showBelegabgleich but is filtered to Ausgangsrechnungen and
// bank CREDIT lines (IstGutschrift==true).
func (a *App) showErloesAbgleich() {
	// Reconcile the WHOLE current year (consistent with showBelegabgleich); the
	// matcher's date window keeps same-amount credits in different months apart.
	rows := a.collectInvoiceRows(a.currentYear, 1, a.currentYear, 12)

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

	// ── Step 3: Route all matches into the confirm list (no silent auto-linking) ─
	// E18 confirm-each: every match (auto or ambiguous) requires explicit user
	// confirmation. High-confidence (formerly MatchAuto) entries are flagged with
	// highConfidence=true so they appear with a ★ prefix and sort to the top.

	refKey := func(file string, page, lineIdx int) string {
		return core.BuchungRef{StatementFilename: file, Page: page, LineIdx: lineIdx}.String()
	}

	claimed := map[string]bool{}

	// E18: autoLinkedSet is always empty — nothing is silently linked.
	autoLinkedSet := map[string]bool{}

	type erloessSuggestion struct {
		row            core.CSVRow
		candidates     []scoredWithFile
		highConfidence bool
	}

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

	var suggestions []erloessSuggestion

	// Former-auto results become high-confidence suggestions (no claiming here).
	for _, r := range autoResults {
		suggestions = append(suggestions, erloessSuggestion{
			row:            r.row,
			candidates:     r.candidates,
			highConfidence: true,
		})
	}

	// Ambiguous/suggest results — all candidates available (nothing claimed yet).
	for _, r := range suggestResults {
		if len(r.candidates) == 0 {
			continue
		}
		suggestions = append(suggestions, erloessSuggestion{
			row:            r.row,
			candidates:     r.candidates,
			highConfidence: false,
		})
	}

	// Sort: high-confidence (★) first, then by top-candidate score descending.
	sort.SliceStable(suggestions, func(i, j int) bool {
		if suggestions[i].highConfidence != suggestions[j].highConfidence {
			return suggestions[i].highConfidence
		}
		return suggestions[i].candidates[0].scored.Score > suggestions[j].candidates[0].scored.Score
	})

	// ── Step 4: Claude re-ranking for close-call ambiguous suggestions ────────
	// Mechanism 4: when processing mode is "claude" and the top-2 single-line
	// candidates are within a small score margin (< 0.3), ask Claude to pick the
	// best matching credit line. Errors are non-fatal.
	if a.settings.ProcessingMode == "claude" {
		apiKey, keyErr := keyring.Get("BuchISY", a.keyringAccount())
		if keyErr == nil && apiKey != "" {
			for i := range suggestions {
				sug := &suggestions[i]
				if len(sug.candidates) < 2 {
					continue
				}
				if sug.candidates[0].scored.Score-sug.candidates[1].scored.Score >= 0.3 {
					continue
				}
				lineTexts := make([]string, len(sug.candidates))
				for j, c := range sug.candidates {
					lineTexts[j] = c.scored.Line.Text
				}
				idx, rankErr := a.anthropicExtractor.RankStatementLine(
					context.Background(),
					apiKey,
					a.settings.AnthropicModel,
					sug.row.Auftraggeber,
					lineTexts,
				)
				if rankErr != nil {
					a.logger.Warn("ErloesAbgleich RankStatementLine %s: %v", sug.row.Auftraggeber, rankErr)
					continue
				}
				if idx > 0 && idx < len(sug.candidates) {
					// Move Claude's pick to the front.
					chosen := sug.candidates[idx]
					copy(sug.candidates[1:idx+1], sug.candidates[0:idx])
					sug.candidates[0] = chosen
				}
			}
		}
	}

	// ── Step 5: Grouped (n:1) and partial (1:n) detection ────────────────────
	// Mechanism 1: FindGroupedRevenuePayments — one credit line covering N invoices.
	// Mechanism 2: RevenuePartialPaymentLines — one Teilzahlung invoice with partial credits.
	// Both run over unclaimed credit lines and unlinked Ausgangsrechnungen.

	type groupSuggestion struct {
		group core.GroupMatch
	}
	type partialSuggestion struct {
		row        core.CSVRow
		candidates []scoredWithFile
	}

	var groupSuggestions []groupSuggestion
	var partialSuggestions []partialSuggestion

	// Track which accounts we've already processed for group detection.
	groupProcessedAcct := map[string]bool{}

	for _, row := range rows {
		// Only unlinked Ausgangsrechnungen on bank accounts.
		if !row.Ausgangsrechnung || row.BuchungRef != "" || autoLinkedSet[row.Dateiname] {
			continue
		}
		if accountType(row.Bankkonto) != core.AccountTypeBank {
			continue
		}

		// Mechanism 2: partial payment suggestions for Teilzahlung invoices.
		// Match per file so each candidate's source file is unambiguous.
		if row.Teilzahlung {
			var swf []scoredWithFile
			for _, sl := range unclaimedByFile(stmtCache[row.Bankkonto], claimed, refKey) {
				for _, c := range core.RevenuePartialPaymentLines(row, sl.lines) {
					swf = append(swf, scoredWithFile{scored: c, file: sl.file})
				}
			}
			if len(swf) > 0 {
				sort.SliceStable(swf, func(i, j int) bool { return swf[i].scored.Score > swf[j].scored.Score })
				partialSuggestions = append(partialSuggestions, partialSuggestion{row: row, candidates: swf})
			}
		}

		// Mechanism 1: group detection per account (run once per account).
		if !groupProcessedAcct[row.Bankkonto] {
			groupProcessedAcct[row.Bankkonto] = true

			// Collect all unlinked Ausgangsrechnungen for this account.
			var unmatchedInvoices []core.CSVRow
			for _, r2 := range rows {
				if !r2.Ausgangsrechnung || r2.Bankkonto != row.Bankkonto {
					continue
				}
				if r2.BuchungRef != "" || autoLinkedSet[r2.Dateiname] {
					continue
				}
				if accountType(r2.Bankkonto) != core.AccountTypeBank {
					continue
				}
				unmatchedInvoices = append(unmatchedInvoices, r2)
			}

			// Detect groups per file so each group's source file is unambiguous,
			// and don't reuse an invoice across files' groups.
			groupedInvoices := map[string]bool{}
			for _, sl := range unclaimedByFile(stmtCache[row.Bankkonto], claimed, refKey) {
				var avail []core.CSVRow
				for _, inv := range unmatchedInvoices {
					if !groupedInvoices[inv.Dateiname] {
						avail = append(avail, inv)
					}
				}
				for _, g := range core.FindGroupedRevenuePayments(avail, sl.lines, cfg) {
					g.File = sl.file
					for _, dn := range g.Dateinamen {
						groupedInvoices[dn] = true
					}
					groupSuggestions = append(groupSuggestions, groupSuggestion{group: g})
				}
			}
		}
	}

	// Refresh table before showing the dialog.
	a.loadInvoices()

	// dlg is declared here so the bulk-confirm closure can reference it to
	// close and reopen the dialog after linking all ★ rows. It is assigned
	// below after dialog.NewCustom returns.
	var dlg dialog.Dialog

	// ── Step 6: Build dialog content ──────────────────────────────────────────
	var content fyne.CanvasObject

	if len(suggestions) == 0 && len(groupSuggestions) == 0 && len(partialSuggestions) == 0 {
		content = container.NewVScroll(
			container.NewVBox(widget.NewLabel(a.bundle.T("erloesabgleich.none"))),
		)
	} else {
		vbox := container.NewVBox()

		// Show summary header only when there are actual suggestions to confirm.
		if len(suggestions) > 0 || len(groupSuggestions) > 0 || len(partialSuggestions) > 0 {
			headerText := fmt.Sprintf(a.bundle.T("erloesabgleich.summary"), len(suggestions))
			header := widget.NewLabel(headerText)
			header.TextStyle = fyne.TextStyle{Bold: true}
			vbox.Add(header)
		}

		// ── "Alle ★ bestätigen" bulk-confirm button ────────────────────────────
		// Count high-confidence suggestions whose top candidate's line is not yet
		// claimed. The button is only shown when at least one such row exists.
		starCount := 0
		for _, s := range suggestions {
			if !s.highConfidence {
				continue
			}
			top := s.candidates[0]
			key := refKey(top.file, top.scored.Line.Page, top.scored.Line.LineIdx)
			if !claimed[key] {
				starCount++
			}
		}
		if starCount > 0 {
			bulkBtn := widget.NewButton(a.bundle.T("reconcile.confirmAllStar", starCount), nil)
			bulkBtn.OnTapped = func() {
				dialog.ShowConfirm(
					a.bundle.T("reconcile.confirmAllStar", starCount),
					a.bundle.T("reconcile.confirmAllAsk", starCount),
					func(ok bool) {
						if !ok {
							return
						}
						for i := range suggestions {
							sug := &suggestions[i]
							if !sug.highConfidence {
								continue
							}
							top := sug.candidates[0]
							key := refKey(top.file, top.scored.Line.Page, top.scored.Line.LineIdx)
							if claimed[key] {
								continue
							}
							sug.row.BuchungRef = core.BuchungRef{
								StatementFilename: top.file,
								Page:              top.scored.Line.Page,
								LineIdx:           top.scored.Line.LineIdx,
							}.String()
							if pay, ok := a.settings.PaymentAccountSKR04(sug.row.Bankkonto); ok {
								sug.row.Buchung = sug.row.Buchung.WithSettlementAccount(pay)
							}
							if err := a.dbRepo.Update(sug.row.Jahr, sug.row.Monat, sug.row.Dateiname, sug.row); err != nil {
								a.logger.Warn("ErloesAbgleich bulkConfirm Update %s: %v", sug.row.Dateiname, err)
							}
							if a.statementAliases != nil {
								a.statementAliases.Learn(sug.row.Auftraggeber, top.scored.Line.Text)
								if err := a.statementAliases.Save(); err != nil {
									a.logger.Warn("ErloesAbgleich bulkConfirm: save aliases: %v", err)
								}
							}
							claimed[key] = true
						}
						a.loadInvoices()
						// Rebuild the dialog so the now-linked ★ rows are gone from the
						// suggestion list and no stale "Bestätigen" buttons remain visible.
						if dlg != nil {
							dlg.Hide()
						}
						a.showErloesAbgleich()
					},
					a.window,
				)
			}
			vbox.Add(bulkBtn)
		}

		// ── Single-line suggestions ────────────────────────────────────────────
		for _, s := range suggestions {
			// Capture loop variable for closure safety.
			sug := s
			selIdx := 0

			top := sug.candidates[0]

			invEUR := core.InvoiceEURAmount(sug.row)
			invAmtStr := formatMoney(invEUR, "EUR", a.settings.DecimalSeparator)

			lineDate := top.scored.Line.Date
			lineBetragStr := formatMoney(top.scored.Line.Betrag, "EUR", a.settings.DecimalSeparator)

			lineRunes := []rune(top.scored.Line.Text)
			if len(lineRunes) > 60 {
				lineRunes = append(lineRunes[:57], []rune("…")...)
			}

			baseName := filepath.Base(top.file)

			// Label: [★ ]<Auftraggeber>  <amount>  →  S.<page+1> Z.<lineIdx> · <date> · <betrag> · <text> (<file>)
			// ★ prefix indicates a high-confidence (formerly auto-linked) match.
			prefix := ""
			if sug.highConfidence {
				prefix = "★ "
			}
			rowLabel := fmt.Sprintf("%s%s  %s  →  S.%d Z.%d · %s · %s · %s  (%s)",
				prefix,
				sug.row.Auftraggeber,
				invAmtStr,
				top.scored.Line.Page+1,
				top.scored.Line.LineIdx,
				lineDate,
				lineBetragStr,
				string(lineRunes),
				baseName,
			)
			lbl := newCopyableLabel(a.bundle, rowLabel)
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
				if pay, ok := a.settings.PaymentAccountSKR04(sug.row.Bankkonto); ok {
					sug.row.Buchung = sug.row.Buchung.WithSettlementAccount(pay)
				}
				if err := a.dbRepo.Update(sug.row.Jahr, sug.row.Monat, sug.row.Dateiname, sug.row); err != nil {
					a.logger.Warn("ErloesAbgleich confirm Update %s: %v", sug.row.Dateiname, err)
				}
				// Mechanism 3: alias learning on confirm.
				if a.statementAliases != nil {
					a.statementAliases.Learn(sug.row.Auftraggeber, chosen.scored.Line.Text)
					if err := a.statementAliases.Save(); err != nil {
						a.logger.Warn("ErloesAbgleich confirm: save aliases: %v", err)
					}
				}
				claimed[key] = true
				confirmBtn.Disable()
				lbl.SetText("✓ " + rowLabel)
				a.loadInvoices()
			}

			// Build row widget. When ≥2 candidates exist, add a Select dropdown
			// so the user can choose which credit line to confirm.
			var rowWidget fyne.CanvasObject
			if len(sug.candidates) >= 2 {
				options := make([]string, len(sug.candidates))
				for i, c := range sug.candidates {
					runes := []rune(c.scored.Line.Text)
					if len(runes) > 60 {
						runes = append(runes[:57], []rune("…")...)
					}
					bStr := formatMoney(c.scored.Line.Betrag, "EUR", a.settings.DecimalSeparator)
					// 1-based index prefix ensures option labels are unique even when
					// two candidates render identically (mirrors expense dialog E12 fix).
					options[i] = fmt.Sprintf("[%d] S.%d Z.%d · %s · %s · %s",
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
					// Update the label to reflect the newly selected candidate.
					chosen := sug.candidates[selIdx]
					newLineRunes := []rune(chosen.scored.Line.Text)
					if len(newLineRunes) > 60 {
						newLineRunes = append(newLineRunes[:57], []rune("…")...)
					}
					newBStr := formatMoney(chosen.scored.Line.Betrag, "EUR", a.settings.DecimalSeparator)
					newBase := filepath.Base(chosen.file)
					lbl.SetText(fmt.Sprintf("%s%s  %s  →  S.%d Z.%d · %s · %s · %s  (%s)",
						prefix,
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

		// ── Mechanism 1: Group (n:1) payment rows ─────────────────────────────
		for _, gs := range groupSuggestions {
			// Capture for closure safety.
			grp := gs.group

			lineRunes := []rune(grp.Line.Text)
			if len(lineRunes) > 60 {
				lineRunes = append(lineRunes[:57], []rune("…")...)
			}
			betragStr := formatMoney(grp.Line.Betrag, "EUR", a.settings.DecimalSeparator)
			filePart := ""
			if grp.File != "" {
				filePart = " (" + filepath.Base(grp.File) + ")"
			}
			countLabel := fmt.Sprintf(a.bundle.T("reconcile.group"), len(grp.Dateinamen))
			rowLabel := fmt.Sprintf("%s (%s) = S.%d Z.%d · %s · %s · %s%s",
				countLabel,
				strings.Join(grp.Dateinamen, ", "),
				grp.Line.Page+1,
				grp.Line.LineIdx,
				grp.Line.Date,
				betragStr,
				string(lineRunes),
				filePart,
			)
			lbl := newCopyableLabel(a.bundle, rowLabel)
			lbl.Wrapping = fyne.TextWrapWord

			linkAllBtn := widget.NewButton(a.bundle.T("reconcile.linkAll"), nil)
			linkAllBtn.OnTapped = func() {
				// Guard against double-linking the same statement line.
				grpKey := refKey(grp.File, grp.Line.Page, grp.Line.LineIdx)
				if claimed[grpKey] {
					lbl.SetText("⚠ " + a.bundle.T("reconcile.lineTaken"))
					linkAllBtn.Disable()
					return
				}
				lineRef := core.BuchungRef{
					StatementFilename: grp.File,
					Page:              grp.Line.Page,
					LineIdx:           grp.Line.LineIdx,
				}.String()
				for _, name := range grp.Dateinamen {
					for _, r2 := range rows {
						if r2.Dateiname != name {
							continue
						}
						r2.BuchungRef = lineRef
						if pay, ok := a.settings.PaymentAccountSKR04(r2.Bankkonto); ok {
							r2.Buchung = r2.Buchung.WithSettlementAccount(pay)
						}
						if err := a.dbRepo.Update(r2.Jahr, r2.Monat, r2.Dateiname, r2); err != nil {
							a.logger.Warn("ErloesAbgleich linkAll Update %s: %v", r2.Dateiname, err)
						}
						// Mechanism 3: alias learning for each linked invoice in the group.
						if a.statementAliases != nil {
							a.statementAliases.Learn(r2.Auftraggeber, grp.Line.Text)
							if err := a.statementAliases.Save(); err != nil {
								a.logger.Warn("ErloesAbgleich linkAll: save aliases: %v", err)
							}
						}
						break
					}
				}
				// Claim the line so no other row in this session can reuse it.
				claimed[grpKey] = true
				linkAllBtn.Disable()
				lbl.SetText("✓ " + rowLabel)
				a.loadInvoices()
			}

			vbox.Add(container.NewBorder(nil, nil, nil, linkAllBtn, lbl))
		}

		// ── Mechanism 2: Partial (1:n) payment rows ───────────────────────────
		for _, ps := range partialSuggestions {
			// Capture for closure safety.
			psug := ps
			pSelIdx := 0

			if len(psug.candidates) == 0 {
				continue
			}
			top := psug.candidates[0]
			invEUR := core.InvoiceEURAmount(psug.row)
			invAmtStr := formatMoney(invEUR, "EUR", a.settings.DecimalSeparator)
			lineBetragStr := formatMoney(top.scored.Line.Betrag, "EUR", a.settings.DecimalSeparator)
			lineRunes := []rune(top.scored.Line.Text)
			if len(lineRunes) > 60 {
				lineRunes = append(lineRunes[:57], []rune("…")...)
			}
			baseName := filepath.Base(top.file)
			rowLabel := fmt.Sprintf("[%s] %s  %s  →  S.%d Z.%d · %s · %s · %s  (%s)",
				a.bundle.T("reconcile.partial"),
				psug.row.Auftraggeber,
				invAmtStr,
				top.scored.Line.Page+1,
				top.scored.Line.LineIdx,
				top.scored.Line.Date,
				lineBetragStr,
				string(lineRunes),
				baseName,
			)
			lbl := newCopyableLabel(a.bundle, rowLabel)
			lbl.Wrapping = fyne.TextWrapWord

			confirmBtn := widget.NewButton(a.bundle.T("erloesabgleich.confirm"), nil)
			confirmBtn.OnTapped = func() {
				// Guard against double-linking the same statement line.
				chosen := psug.candidates[pSelIdx]
				key := refKey(chosen.file, chosen.scored.Line.Page, chosen.scored.Line.LineIdx)
				if claimed[key] {
					lbl.SetText("⚠ " + a.bundle.T("reconcile.lineTaken"))
					confirmBtn.Disable()
					return
				}
				psug.row.BuchungRef = core.BuchungRef{
					StatementFilename: chosen.file,
					Page:              chosen.scored.Line.Page,
					LineIdx:           chosen.scored.Line.LineIdx,
				}.String()
				if pay, ok := a.settings.PaymentAccountSKR04(psug.row.Bankkonto); ok {
					psug.row.Buchung = psug.row.Buchung.WithSettlementAccount(pay)
				}
				if err := a.dbRepo.Update(psug.row.Jahr, psug.row.Monat, psug.row.Dateiname, psug.row); err != nil {
					a.logger.Warn("ErloesAbgleich partial confirm Update %s: %v", psug.row.Dateiname, err)
				}
				// Mechanism 3: alias learning on partial confirm.
				if a.statementAliases != nil {
					a.statementAliases.Learn(psug.row.Auftraggeber, chosen.scored.Line.Text)
					if err := a.statementAliases.Save(); err != nil {
						a.logger.Warn("ErloesAbgleich partial confirm: save aliases: %v", err)
					}
				}
				claimed[key] = true
				confirmBtn.Disable()
				lbl.SetText("✓ " + rowLabel)
				a.loadInvoices()
			}

			var rowWidget fyne.CanvasObject
			if len(psug.candidates) >= 2 {
				options := make([]string, len(psug.candidates))
				for i, c := range psug.candidates {
					runes := []rune(c.scored.Line.Text)
					if len(runes) > 60 {
						runes = append(runes[:57], []rune("…")...)
					}
					bStr := formatMoney(c.scored.Line.Betrag, "EUR", a.settings.DecimalSeparator)
					options[i] = fmt.Sprintf("[%d] S.%d Z.%d · %s · %s · %s",
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
							pSelIdx = i
							break
						}
					}
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

	dlg = dialog.NewCustom(
		fmt.Sprintf("%s — %d (%s)", a.bundle.T("erloesabgleich.title"), a.currentYear, a.bundle.T("reconcile.wholeYear")),
		a.bundle.T("common.close"),
		content,
		a.window,
	)
	dlg.Resize(fyne.NewSize(640, 480))
	dlg.Show()
}
