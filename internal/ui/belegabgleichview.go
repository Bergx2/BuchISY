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

// scoredWithFile pairs a candidate ScoredLine with the statement file it came from.
type scoredWithFile struct {
	scored core.ScoredLine
	file   string
}

// belegSuggestion is a single MatchSuggest entry collected during reconciliation.
// candidates is ranked best-first; candidates[0] is the default selection.
// highConfidence is true for former MatchAuto entries (unambiguous single match)
// that are now routed to the confirm list instead of being silently linked.
type belegSuggestion struct {
	row            core.CSVRow
	candidates     []scoredWithFile
	highConfidence bool
}

// stmtLine pairs a parsed StatementBooking with its source file (base name).
type stmtLine struct {
	File string
	Line core.StatementBooking
}

// fileLineGroup holds one statement file's lines for per-file matching.
type fileLineGroup struct {
	file  string
	lines []core.StatementBooking
}

// unclaimedByFile groups a per-account cache's not-yet-claimed lines by source
// file (preserving encounter order), so per-file matching keeps each line's
// file unambiguous — a (page,lineIdx) pair repeats across statement files.
func unclaimedByFile(cache []stmtLine, claimed map[string]bool, refKey func(string, int, int) string) []fileLineGroup {
	idxOf := map[string]int{}
	var out []fileLineGroup
	for _, sl := range cache {
		if claimed[refKey(sl.File, sl.Line.Page, sl.Line.LineIdx)] {
			continue
		}
		idx, seen := idxOf[sl.File]
		if !seen {
			idx = len(out)
			idxOf[sl.File] = idx
			out = append(out, fileLineGroup{file: sl.File})
		}
		out[idx].lines = append(out[idx].lines, sl.Line)
	}
	return out
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
	if a.statementAliases != nil {
		cfg.Aliases, _ = a.statementAliases.Load()
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

	// FIX 3: Hoist matchConfig out of hot loops — computed once here and
	// passed to every MatchInvoiceToStatement / FindGroupedPayments call.
	cfg := a.matchConfig()

	// ── Step 1: Build parse-once cache per account ──────────────────────────
	// For each unique bank/creditcard account referenced by the unlinked rows,
	// parse every statement file exactly once and cache all lines tagged with
	// their source file. Reuse this cache for all invoices of that account.

	type matchResult struct {
		row        core.CSVRow
		kind       core.MatchKind
		candidates []scoredWithFile
	}

	stmtCache := map[string][]stmtLine{} // account name → all cached lines across files
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
				a.logger.Warn("Belegabgleich: parse statement %s: %v", name, err)
				continue
			}
			for _, l := range lines {
				stmtCache[acct] = append(stmtCache[acct], stmtLine{File: name, Line: l})
			}
		}
	}

	// ── Step 2: Compute match results for every unlinked invoice ────────────
	// Match per file so each candidate's file is known unambiguously.
	var allResults []matchResult

	for _, row := range rows {
		// Skip already-linked and non-bank/creditcard accounts.
		if row.BuchungRef != "" {
			continue
		}
		at := accountType(row.Bankkonto)
		if at != core.AccountTypeBank && at != core.AccountTypeCreditCard {
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
			// FIX 3: use hoisted cfg instead of calling a.matchConfig() per iteration.
			kind, cands := core.MatchInvoiceToStatement(row, linesForFile, cfg)
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
				// FIX 2: first suggest file — start accumulation.
				bestKind = kind
				bestCandidates = swf
			case kind == core.MatchSuggest && bestKind == core.MatchSuggest:
				// FIX 2: additional suggest file — ACCUMULATE candidates instead of
				// replacing with only the higher-scored file's slice.
				// Deduplicate by (file, page, lineIdx) before appending.
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

		// Two+ statement files each produced an unambiguous match for the same
		// invoice — that is ambiguous across files; never silently auto-link.
		if bestKind == core.MatchAuto && autoCount >= 2 {
			bestKind = core.MatchSuggest
		}

		// FIX 2: After the file loop, if final bestKind is MatchSuggest, sort
		// all accumulated candidates by score descending so best appears first.
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

	// ── Step 3: Greedy auto-link by score; claim each statement line at most once ──
	claimed := map[string]bool{}

	refKey := func(file string, page, lineIdx int) string {
		return core.BuchungRef{StatementFilename: file, Page: page, LineIdx: lineIdx}.String()
	}

	// Separate auto from suggest results; sort autos DESC by top-candidate score.
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

	// E18.1: No silent auto-linking. Every match (auto or ambiguous) is routed
	// into the confirm list so the user approves each one explicitly.
	// High-confidence (former MatchAuto) entries are flagged and sorted to the top.
	var suggestions []belegSuggestion

	// Route former-auto results as high-confidence suggestions (no claiming here).
	for _, r := range autoResults {
		suggestions = append(suggestions, belegSuggestion{
			row:            r.row,
			candidates:     r.candidates,
			highConfidence: true,
		})
	}

	// Build suggestions for ambiguous/suggest results; no claimed lines to filter yet.
	for _, r := range suggestResults {
		if len(r.candidates) == 0 {
			continue
		}
		suggestions = append(suggestions, belegSuggestion{
			row:            r.row,
			candidates:     r.candidates,
			highConfidence: false,
		})
	}

	// Sort: high-confidence (★) entries first, then by top-candidate score descending.
	sort.SliceStable(suggestions, func(i, j int) bool {
		if suggestions[i].highConfidence != suggestions[j].highConfidence {
			return suggestions[i].highConfidence
		}
		return suggestions[i].candidates[0].scored.Score > suggestions[j].candidates[0].scored.Score
	})

	// ── Step 4: Claude ranking for close-call ambiguous suggestions ────────
	// For each suggestion with ≥2 candidates whose top-two scores are within
	// 0.3 of each other, ask Claude which statement line best matches the
	// supplier and reorder candidates so Claude's pick is first.
	// Only runs when processing mode is "claude" and an API key is available.
	// Errors are non-fatal: we just log and keep the heuristic order.
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
					a.logger.Warn("Belegabgleich RankStatementLine %s: %v", sug.row.Auftraggeber, rankErr)
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

	// ── Step 5: Grouped (n:1) and partial (1:n) payment detection ─────────
	// Per bank/creditcard account: collect still-unclaimed lines from the
	// parse-once cache and the still-unmatched invoices; run group + partial
	// detection. Results are rendered in the dialog below.

	// E18.1: Nothing is auto-linked any more — autoLinkedSet is always empty.
	// The group/partial detection loop uses it to skip already-linked invoices;
	// with no silent linking, every invoice with BuchungRef=="" is a candidate.
	autoLinkedSet := map[string]bool{}

	type groupSuggestion struct {
		group core.GroupMatch
	}
	type partialSuggestion struct {
		row        core.CSVRow
		candidates []scoredWithFile
	}

	var groupSuggestions []groupSuggestion
	var partialSuggestions []partialSuggestion

	// Track which accounts we've already processed to avoid duplicate group detection.
	groupProcessedAcct := map[string]bool{}

	for _, row := range rows {
		if row.BuchungRef != "" || autoLinkedSet[row.Dateiname] {
			continue
		}
		at := accountType(row.Bankkonto)
		if at != core.AccountTypeBank && at != core.AccountTypeCreditCard {
			continue
		}

		// Partial payment suggestions for this invoice (Teilzahlung=true).
		// Match per file so each candidate's source file is unambiguous (a
		// (page,lineIdx) pair repeats across statement files).
		if row.Teilzahlung {
			var swf []scoredWithFile
			for _, sl := range unclaimedByFile(stmtCache[row.Bankkonto], claimed, refKey) {
				for _, c := range core.PartialPaymentLines(row, sl.lines) {
					swf = append(swf, scoredWithFile{scored: c, file: sl.file})
				}
			}
			if len(swf) > 0 {
				sort.SliceStable(swf, func(i, j int) bool { return swf[i].scored.Score > swf[j].scored.Score })
				partialSuggestions = append(partialSuggestions, partialSuggestion{row: row, candidates: swf})
			}
		}

		// Group detection per account (run once per account).
		if !groupProcessedAcct[row.Bankkonto] {
			groupProcessedAcct[row.Bankkonto] = true

			// Unmatched invoices for this account.
			var unmatchedInvoices []core.CSVRow
			for _, r2 := range rows {
				if r2.Bankkonto != row.Bankkonto {
					continue
				}
				if r2.BuchungRef != "" || autoLinkedSet[r2.Dateiname] {
					continue
				}
				at2 := accountType(r2.Bankkonto)
				if at2 != core.AccountTypeBank && at2 != core.AccountTypeCreditCard {
					continue
				}
				unmatchedInvoices = append(unmatchedInvoices, r2)
			}

			// Detect groups PER FILE so each group's source file is unambiguous,
			// and don't reuse an invoice across files' groups.
			groupedInvoices := map[string]bool{}
			for _, sl := range unclaimedByFile(stmtCache[row.Bankkonto], claimed, refKey) {
				var avail []core.CSVRow
				for _, inv := range unmatchedInvoices {
					if !groupedInvoices[inv.Dateiname] {
						avail = append(avail, inv)
					}
				}
				// FIX 3: use hoisted cfg instead of calling a.matchConfig() per iteration.
				for _, g := range core.FindGroupedPayments(avail, sl.lines, cfg) {
					g.File = sl.file
					for _, dn := range g.Dateinamen {
						groupedInvoices[dn] = true
					}
					groupSuggestions = append(groupSuggestions, groupSuggestion{group: g})
				}
			}
		}
	}

	// Refresh table (in case any rows changed state before the dialog opens).
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

	if len(suggestions) == 0 && len(groupSuggestions) == 0 && len(partialSuggestions) == 0 {
		vbox := container.NewVBox(widget.NewLabel(a.bundle.T("reconcile.none")))
		if cashBox != nil {
			heading := widget.NewLabel(a.bundle.T("reconcile.cashHeading"))
			heading.TextStyle = fyne.TextStyle{Bold: true}
			vbox.Add(heading)
			vbox.Add(cashBox)
		}
		content = container.NewVScroll(vbox)
	} else {
		headerText := a.bundle.T("reconcile.summary", len(suggestions))
		header := widget.NewLabel(headerText)
		header.TextStyle = fyne.TextStyle{Bold: true}

		vbox := container.NewVBox(header)

		for _, s := range suggestions {
			// Capture loop variable for closure safety.
			sug := s

			// selIdx tracks which candidate is currently selected for THIS suggestion.
			// Declared inside the per-iteration scope so each row has its own index.
			selIdx := 0

			top := sug.candidates[0]

			// Invoice EUR amount: prefer BetragNetto_EUR if set, else Bruttobetrag.
			invEUR := core.InvoiceEURAmount(sug.row)
			invAmtStr := strings.Replace(fmt.Sprintf("%.2f", invEUR), ".", ",", 1)

			lineDate := top.scored.Line.Date
			lineBetragStr := strings.Replace(fmt.Sprintf("%.2f", top.scored.Line.Betrag), ".", ",", 1)

			// Truncate line text to ~60 runes.
			lineRunes := []rune(top.scored.Line.Text)
			if len(lineRunes) > 60 {
				lineRunes = append(lineRunes[:57], []rune("…")...)
			}

			baseName := filepath.Base(top.file)

			// Label format:
			// [★ ]<Auftraggeber>  <invEUR> €  →  S.<p> Z.<l> · <date> · <betrag> € · <text>  (<file>)
			// ★ prefix indicates a high-confidence (formerly auto-linked) match.
			prefix := ""
			if sug.highConfidence {
				prefix = "★ "
			}
			rowLabel := fmt.Sprintf("%s%s  %s €  →  S.%d Z.%d · %s · %s € · %s  (%s)",
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
			lbl := widget.NewLabel(rowLabel)
			lbl.Wrapping = fyne.TextWrapWord

			confirmBtn := widget.NewButton(a.bundle.T("reconcile.confirm"), nil)

			confirmBtn.OnTapped = func() {
				// FIX 1: Guard against double-linking the same statement line.
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
					a.logger.Warn("Belegabgleich confirm Update %s: %v", sug.row.Dateiname, err)
				}
				if a.statementAliases != nil {
					a.statementAliases.Learn(sug.row.Auftraggeber, chosen.scored.Line.Text)
					if err := a.statementAliases.Save(); err != nil {
						a.logger.Warn("Belegabgleich confirm: save aliases: %v", err)
					}
				}
				// Mark this line claimed so other confirms in the same dialog session
				// cannot reuse it.
				claimed[key] = true
				confirmBtn.Disable()
				lbl.SetText("✓ " + rowLabel)
				a.loadInvoices()
			}

			// Build the row container. If there are 2+ candidates, add a Select widget
			// so the user can pick which line to confirm. A single candidate keeps the
			// current minimal UI (label + button only).
			var rowWidget fyne.CanvasObject
			if len(sug.candidates) >= 2 {
				// Build option labels: "S.<p> Z.<l> · <date> · <betrag> € · <short text>"
				options := make([]string, len(sug.candidates))
				for i, c := range sug.candidates {
					runes := []rune(c.scored.Line.Text)
					if len(runes) > 60 {
						runes = append(runes[:57], []rune("…")...)
					}
					bStr := strings.Replace(fmt.Sprintf("%.2f", c.scored.Line.Betrag), ".", ",", 1)
					// Prefix with a 1-based index so option labels are unique even
					// when two candidate lines render identically — the OnChanged
					// string lookup then always resolves to the right candidate.
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
				})
				sel.SetSelected(options[0])

				rowWidget = container.NewBorder(nil, nil, nil, confirmBtn,
					container.NewVBox(lbl, sel))
			} else {
				rowWidget = container.NewBorder(nil, nil, nil, confirmBtn, lbl)
			}

			vbox.Add(rowWidget)
		}

		// ── Group (n:1) payment rows ────────────────────────────────────────
		for _, gs := range groupSuggestions {
			// Capture for closure safety.
			grp := gs.group

			lineRunes := []rune(grp.Line.Text)
			if len(lineRunes) > 60 {
				lineRunes = append(lineRunes[:57], []rune("…")...)
			}
			betragStr := strings.Replace(fmt.Sprintf("%.2f", grp.Line.Betrag), ".", ",", 1)
			filePart := ""
			if grp.File != "" {
				filePart = " (" + filepath.Base(grp.File) + ")"
			}
			countLabel := fmt.Sprintf(a.bundle.T("reconcile.group"), len(grp.Dateinamen))
			rowLabel := fmt.Sprintf("%s (%s) = S.%d Z.%d · %s · %s € · %s%s",
				countLabel,
				strings.Join(grp.Dateinamen, ", "),
				grp.Line.Page+1,
				grp.Line.LineIdx,
				grp.Line.Date,
				betragStr,
				string(lineRunes),
				filePart,
			)
			lbl := widget.NewLabel(rowLabel)
			lbl.Wrapping = fyne.TextWrapWord

			linkAllBtn := widget.NewButton(a.bundle.T("reconcile.linkAll"), nil)
			linkAllBtn.OnTapped = func() {
				// FIX 1: Guard against double-linking the same statement line.
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
					// Find the matching row.
					for _, r2 := range rows {
						if r2.Dateiname != name {
							continue
						}
						r2.BuchungRef = lineRef
						if err := a.dbRepo.Update(r2.Jahr, r2.Monat, r2.Dateiname, r2); err != nil {
							a.logger.Warn("Belegabgleich linkAll Update %s: %v", r2.Dateiname, err)
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

		// ── Partial (1:n) payment rows ───────────────────────────────────────
		for _, ps := range partialSuggestions {
			// Capture for closure safety.
			psug := ps
			pSelIdx := 0

			if len(psug.candidates) == 0 {
				continue
			}
			top := psug.candidates[0]
			invEUR := core.InvoiceEURAmount(psug.row)
			invAmtStr := strings.Replace(fmt.Sprintf("%.2f", invEUR), ".", ",", 1)
			lineBetragStr := strings.Replace(fmt.Sprintf("%.2f", top.scored.Line.Betrag), ".", ",", 1)
			lineRunes := []rune(top.scored.Line.Text)
			if len(lineRunes) > 60 {
				lineRunes = append(lineRunes[:57], []rune("…")...)
			}
			baseName := filepath.Base(top.file)
			rowLabel := fmt.Sprintf("[%s] %s  %s €  →  S.%d Z.%d · %s · %s € · %s  (%s)",
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
			lbl := widget.NewLabel(rowLabel)
			lbl.Wrapping = fyne.TextWrapWord

			confirmBtn := widget.NewButton(a.bundle.T("reconcile.confirm"), nil)
			confirmBtn.OnTapped = func() {
				// FIX 1: Guard against double-linking the same statement line.
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
				if err := a.dbRepo.Update(psug.row.Jahr, psug.row.Monat, psug.row.Dateiname, psug.row); err != nil {
					a.logger.Warn("Belegabgleich partial confirm Update %s: %v", psug.row.Dateiname, err)
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
					bStr := strings.Replace(fmt.Sprintf("%.2f", c.scored.Line.Betrag), ".", ",", 1)
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
