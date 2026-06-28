package ui

import (
	"context"
	"encoding/base64"
	"fmt"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/zalando/go-keyring"

	"github.com/bergx2/buchisy/internal/core"
)

// base64StdEncode is a thin alias used inside the package so callers
// don't need to import encoding/base64 themselves.
func base64StdEncode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// extractStatementMetadata runs Claude Vision on the given statement
// file and merges the extracted period/balances into metadata.json
// (preserving the user's Reviewed flag and Note).
// For CAMT.053 XML and MT940 files the vision step is skipped because
// all transaction data is already extracted structurally by ParseStatementBookings.
func (a *App) extractStatementMetadata(folder, rel string) error {
	fullPath := filepath.Join(folder, rel)

	// E20.6: structured bank-statement files (CAMT.053, MT940) do not need
	// Claude Vision — their bookings are parsed directly from bytes.
	// We still need to touch metadata.json so the row appears in the list.
	{
		data, readErr := os.ReadFile(fullPath)
		if readErr == nil && core.DetectBankFormat(data) != "" {
			format := core.DetectBankFormat(data)
			a.logger.Info("Structured bank statement detected (%s): %s — skipping Vision extraction", format, rel)
			// Ensure the entry exists in metadata.json (with empty period/balance).
			// We only add it when absent so we don't overwrite a previously edited entry.
			metaMap, _ := core.LoadStatementMeta(folder)
			if metaMap == nil {
				metaMap = core.StatementMetadataMap{}
			}
			if _, exists := metaMap[rel]; !exists {
				metaMap[rel] = core.StatementMetadata{}
				_ = core.SaveStatementMeta(folder, metaMap)
			}
			return nil
		}
	}

	var (
		images    []string
		mediaType string
		err       error
	)
	switch {
	case core.IsPDF(fullPath):
		// Send every page — the closing balance is often on the last
		// page, not the first.
		images, mediaType, err = core.PDFAllPagesToBase64(fullPath)
		if err != nil {
			return fmt.Errorf("PDF konnte nicht in Bilder umgewandelt werden: %w", err)
		}
	case core.ImageMediaType(fullPath) != "":
		mediaType = core.ImageMediaType(fullPath)
		data, ferr := os.ReadFile(fullPath)
		if ferr != nil {
			return fmt.Errorf("Bilddatei nicht lesbar: %w", ferr)
		}
		images = []string{base64StdEncode(data)}
	default:
		return fmt.Errorf("Dateiformat wird nicht unterstützt: %s", filepath.Ext(fullPath))
	}

	apiKey, err := keyring.Get("BuchISY", a.keyringAccount())
	if err != nil {
		return fmt.Errorf("API-Key nicht verfügbar (in den Einstellungen hinterlegen): %w", err)
	}

	extracted, err := a.anthropicExtractor.ExtractStatementFromImages(
		context.Background(), apiKey, a.settings.AnthropicModel,
		images, mediaType,
	)
	if err != nil {
		return err
	}

	metaMap, err := core.LoadStatementMeta(folder)
	if err != nil {
		return fmt.Errorf("Metadaten konnten nicht geladen werden: %w", err)
	}
	existing := metaMap[rel]
	// Preserve manual fields the user maintains by hand.
	extracted.Reviewed = existing.Reviewed
	extracted.Note = existing.Note
	metaMap[rel] = extracted
	if err := core.SaveStatementMeta(folder, metaMap); err != nil {
		return fmt.Errorf("Metadaten konnten nicht gespeichert werden: %w", err)
	}
	a.logger.Info("Statement metadata auto-extracted for %s: %s–%s, opening=%.2f, closing=%.2f",
		rel, extracted.DateFrom, extracted.DateTo, extracted.OpeningBalance, extracted.ClosingBalance)
	return nil
}

// autoFillOneStatement runs extractStatementMetadata on a single file
// with a progress spinner; refreshes the view on success.
func (a *App) autoFillOneStatement(folder, rel string) {
	progress := dialog.NewProgressInfinite("Metadaten extrahieren",
		fmt.Sprintf("Lese %s …", rel), a.window)
	progress.Show()
	go func() {
		err := a.extractStatementMetadata(folder, rel)
		fyne.DoAndWait(func() {
			progress.Hide()
			if err != nil {
				dialog.ShowError(err, a.window)
				return
			}
			a.window.SetContent(a.buildUI())
		})
	}()
}

// autoFillAllStatements runs extractStatementMetadata over every
// statement of the given account sequentially, surfacing per-file
// errors as a single summary at the end.
func (a *App) autoFillAllStatements(account string) {
	folder := a.statementFolder(account)
	statements := a.listStatements(account)
	if len(statements) == 0 {
		return
	}
	progress := dialog.NewProgressInfinite("Metadaten extrahieren",
		fmt.Sprintf("0 / %d", len(statements)), a.window)
	progress.Show()
	go func() {
		var failures []string
		for _, rel := range statements {
			if err := a.extractStatementMetadata(folder, rel); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", rel, err))
				a.logger.Warn("Auto-fill failed for %s: %v", rel, err)
			}
		}
		fyne.DoAndWait(func() {
			progress.Hide()
			if len(failures) > 0 {
				dialog.ShowInformation("Auto-füllen abgeschlossen",
					fmt.Sprintf("%d / %d erfolgreich.\n\nFehler:\n%s",
						len(statements)-len(failures), len(statements),
						strings.Join(failures, "\n")),
					a.window)
			} else {
				dialog.ShowInformation("Auto-füllen abgeschlossen",
					fmt.Sprintf("%d / %d erfolgreich.", len(statements), len(statements)),
					a.window)
			}
			a.window.SetContent(a.buildUI())
		})
	}()
}

// fileStatement copies srcPath directly into the currently selected
// account's folder and immediately triggers metadata extraction so the
// new row shows up with the period / number / balances already filled.
// The folder is created if missing.
func (a *App) fileStatement(srcPath string) {
	if a.kontenAccount == "" {
		dialog.ShowInformation("Kontoauszug",
			"Bitte zuerst ein Zahlungskonto auswählen.", a.window)
		return
	}
	folder := a.statementFolder(a.kontenAccount)
	if err := os.MkdirAll(folder, 0755); err != nil {
		dialog.ShowError(err, a.window)
		return
	}
	finalName, err := a.placeFile(srcPath, folder, filepath.Base(srcPath))
	if err != nil {
		dialog.ShowError(err, a.window)
		return
	}
	a.autoFillNewStatements(folder, []string{finalName})
}

// autoFillNewStatements runs Claude vision extraction over a list of
// freshly placed statement file names and refreshes the UI when done.
// Used after both single-file (drag-drop) and multi-file uploads so the
// user doesn't have to click "Alle auto-füllen" manually.
func (a *App) autoFillNewStatements(folder string, names []string) {
	if len(names) == 0 {
		a.window.SetContent(a.buildUI())
		return
	}
	progress := dialog.NewProgressInfinite("Metadaten extrahieren",
		fmt.Sprintf("Verarbeite %d Datei(en) …", len(names)), a.window)
	progress.Show()
	go func() {
		var failures []string
		for _, name := range names {
			if err := a.extractStatementMetadata(folder, name); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", name, err))
				a.logger.Warn("Auto-extract after upload failed for %s: %v", name, err)
			}
		}
		fyne.DoAndWait(func() {
			progress.Hide()
			if len(failures) > 0 {
				dialog.ShowInformation("Metadaten extrahieren",
					fmt.Sprintf("Hochgeladen: %d. Mit Fehlern: %d.\n\n%s",
						len(names), len(failures), strings.Join(failures, "\n")),
					a.window)
			}
			a.window.SetContent(a.buildUI())
			// E18.2: a freshly imported statement is the trigger for
			// reconciliation — open the (confirm-each) Belegabgleich right away
			// when at least one statement was imported successfully.
			if len(names) > len(failures) {
				a.showBelegabgleich()
			}
		})
	}()
}

// switchToBelege returns to the Belege (invoice) view. Triggered from the
// workflow sidebar's "Belege" entry.
func (a *App) switchToBelege() {
	a.viewMode = ""
	a.window.SetContent(a.buildUI())
}

// openKontenPicker shows a popup listing the configured Zahlungskonten;
// picking one switches into that account's statement view. Triggered from
// the workflow sidebar's "Konten" entry. The sidebar has no anchor button,
// so the popup is shown as a modal centered on the window canvas.
func (a *App) openKontenPicker() {
	accounts := a.bankAccountOptionList()
	if len(accounts) == 0 {
		dialog.ShowInformation("Konten",
			"Noch kein Zahlungskonto konfiguriert.\n"+
				"Bitte in den Einstellungen anlegen.",
			a.window)
		return
	}
	// Custom popup with full-width buttons — fyne's NewMenuItem popup
	// truncated long account names like "KSMSE …0712 Sparkasse".
	var pop *widget.PopUp
	list := container.NewVBox()
	for _, name := range accounts {
		n := name
		btn := widget.NewButton(n, func() {
			if pop != nil {
				pop.Hide()
			}
			a.kontenAccount = n
			a.viewMode = "konten"
			a.window.SetContent(a.buildUI())
		})
		btn.Alignment = widget.ButtonAlignLeading
		btn.Importance = widget.LowImportance
		list.Add(btn)
	}
	// Modal popup auto-centers on the canvas — the sidebar has no button to
	// anchor against, so we don't position it manually.
	pop = widget.NewModalPopUp(list, a.window.Canvas())
	pop.Show()
}

// statementFolder returns the per-account root folder used to store
// bank statements: <StorageRoot>/<sanitized-accountName>. Statements
// are stored directly inside (flat layout — no year subfolders).
func (a *App) statementFolder(accountName string) string {
	root := a.settings.StorageRoot
	if root == "" {
		return ""
	}
	return filepath.Join(root, core.SanitizeFilename(accountName))
}

// ensureAccountFolders creates the per-account root folder for every
// configured Zahlungskonto and migrates any legacy year subfolders
// (created by an earlier app version) by flattening their contents
// back into the account root. Also runs a one-time rename migration
// for the deprecated Standard-Zahlungskonto. Idempotent.
func (a *App) ensureAccountFolders() {
	if a.settings.StorageRoot == "" {
		return
	}
	a.migrateLegacyDefaultBankAccount()
	for _, ba := range a.settings.BankAccounts {
		if ba.Name == "" {
			continue
		}
		folder := a.statementFolder(ba.Name)
		if folder == "" {
			continue
		}
		if err := os.MkdirAll(folder, 0755); err != nil {
			if a.logger != nil {
				a.logger.Warn("Failed to create account folder %s: %v", folder, err)
			}
			continue
		}
		a.flattenYearSubfolders(folder)
	}
}

// migrateLegacyDefaultBankAccount handles the leftover state from the
// removed "Standard-Zahlungskonto" feature: if `DefaultBankAccount` is
// still set and matches a BankAccount by IBAN with a different name
// (i.e. the user renamed the account), rename the on-disk folder to
// the new account name and bring its metadata.json along. Clears the
// legacy fields after a successful migration. Idempotent.
func (a *App) migrateLegacyDefaultBankAccount() {
	oldName := a.settings.DefaultBankAccount
	oldIBAN := a.settings.DefaultBankAccountIBAN
	if oldName == "" {
		return
	}
	// Find the new (renamed) account by IBAN.
	if oldIBAN != "" {
		for _, ba := range a.settings.BankAccounts {
			if ba.IBAN == oldIBAN && ba.Name != "" && ba.Name != oldName {
				a.renameAccountFolder(oldName, ba.Name)
				break
			}
		}
	}
	// Whether or not the rename happened, the field has no UI now —
	// clear it so subsequent loads stop running this migration.
	a.settings.DefaultBankAccount = ""
	a.settings.DefaultBankAccountIBAN = ""
	if a.settingsMgr != nil {
		if err := a.settingsMgr.Save(a.settings); err != nil && a.logger != nil {
			a.logger.Warn("Failed to clear legacy DefaultBankAccount: %v", err)
		}
	}
}

// renameAccountFolder moves <Storage>/<oldName>/ to <Storage>/<newName>/.
// If the destination already exists with the same name, the source is
// folded in (only when files don't collide). No-op if source missing.
func (a *App) renameAccountFolder(oldName, newName string) {
	oldFolder := a.statementFolder(oldName)
	newFolder := a.statementFolder(newName)
	if oldFolder == "" || newFolder == "" || oldFolder == newFolder {
		return
	}
	srcInfo, err := os.Stat(oldFolder)
	if err != nil || !srcInfo.IsDir() {
		return
	}

	if _, err := os.Stat(newFolder); err != nil {
		// Destination does not exist — simple rename.
		if err := os.Rename(oldFolder, newFolder); err != nil && a.logger != nil {
			a.logger.Warn("Renaming %s → %s failed: %v", oldFolder, newFolder, err)
		} else if a.logger != nil {
			a.logger.Info("Migrated legacy account folder: %s → %s", oldFolder, newFolder)
		}
		return
	}

	// Both exist — move files individually, skipping collisions.
	if a.logger != nil {
		a.logger.Info("Folding %s into existing %s", oldFolder, newFolder)
	}
	entries, _ := os.ReadDir(oldFolder)
	for _, e := range entries {
		src := filepath.Join(oldFolder, e.Name())
		dst := filepath.Join(newFolder, e.Name())
		if e.Name() == "metadata.json" {
			// Merge metadata maps; new wins on key collision.
			a.mergeStatementMetadata(oldFolder, newFolder)
			_ = os.Remove(src)
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			if a.logger != nil {
				a.logger.Warn("Skip migrating %s — %s already exists", src, dst)
			}
			continue
		}
		if err := os.Rename(src, dst); err != nil && a.logger != nil {
			a.logger.Warn("Move %s → %s failed: %v", src, dst, err)
		}
	}
	_ = os.Remove(oldFolder) // only succeeds if empty
}

// mergeStatementMetadata merges oldFolder/metadata.json into the one
// in newFolder. Existing entries in newFolder win.
func (a *App) mergeStatementMetadata(oldFolder, newFolder string) {
	oldMap, err := core.LoadStatementMeta(oldFolder)
	if err != nil || len(oldMap) == 0 {
		return
	}
	newMap, _ := core.LoadStatementMeta(newFolder)
	if newMap == nil {
		newMap = core.StatementMetadataMap{}
	}
	for k, v := range oldMap {
		if _, exists := newMap[k]; !exists {
			newMap[k] = v
		}
	}
	if err := core.SaveStatementMeta(newFolder, newMap); err != nil && a.logger != nil {
		a.logger.Warn("Merging metadata.json failed: %v", err)
	}
}

// flattenYearSubfolders moves files out of YYYY-named subfolders into
// the account root and rewrites metadata.json keys accordingly. Skips
// files where a same-named file already exists at the root.
func (a *App) flattenYearSubfolders(accountFolder string) {
	entries, err := os.ReadDir(accountFolder)
	if err != nil {
		return
	}
	metaMap, _ := core.LoadStatementMeta(accountFolder)
	metaChanged := false

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !looksLikeYear(name) {
			continue
		}
		yearFolder := filepath.Join(accountFolder, name)
		subEntries, err := os.ReadDir(yearFolder)
		if err != nil {
			continue
		}
		for _, f := range subEntries {
			if f.IsDir() {
				continue
			}
			src := filepath.Join(yearFolder, f.Name())
			dst := filepath.Join(accountFolder, f.Name())
			if _, err := os.Stat(dst); err == nil {
				if a.logger != nil {
					a.logger.Warn("Skipping flatten of %s — %s already exists",
						src, dst)
				}
				continue
			}
			if err := os.Rename(src, dst); err != nil {
				if a.logger != nil {
					a.logger.Warn("Flatten move failed for %s: %v", src, err)
				}
				continue
			}
			// Rewrite the metadata.json key from "2026/x.pdf" → "x.pdf".
			oldKey := filepath.ToSlash(filepath.Join(name, f.Name()))
			newKey := f.Name()
			if m, ok := metaMap[oldKey]; ok {
				metaMap[newKey] = m
				delete(metaMap, oldKey)
				metaChanged = true
			}
		}
		// Drop the now-empty year folder; harmless if it still has
		// non-statement children (Remove won't delete in that case).
		_ = os.Remove(yearFolder)
	}

	if metaChanged {
		if err := core.SaveStatementMeta(accountFolder, metaMap); err != nil &&
			a.logger != nil {
			a.logger.Warn("Failed to rewrite metadata.json after flatten: %v", err)
		}
	}
}

// looksLikeYear returns true for 4-digit names in the 1900–2099 range.
func looksLikeYear(name string) bool {
	if len(name) != 4 {
		return false
	}
	n, err := strconv.Atoi(name)
	if err != nil {
		return false
	}
	return n >= 1900 && n <= 2099
}

// listStatements walks the per-account root and every year-subfolder
// and returns the statements as relative paths (e.g. "2026/Auszug.pdf"),
// newest-first by modification time. Files dropped at the account
// root level (without a year folder) are also included.
func (a *App) listStatements(accountName string) []string {
	root := a.statementFolder(accountName)
	if root == "" {
		return nil
	}
	type item struct {
		rel string
		mod int64
	}
	var items []item
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		// Skip our own metadata sidecar — it isn't a statement.
		if strings.EqualFold(d.Name(), "metadata.json") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		// Normalise to forward slashes for stable display.
		rel = filepath.ToSlash(rel)
		items = append(items, item{rel: rel, mod: info.ModTime().Unix()})
		return nil
	})
	sort.Slice(items, func(i, j int) bool { return items[i].mod > items[j].mod })
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.rel
	}
	return out
}

// accountRichLabel renders a payment account for the Konten picker as
// "<full name> · <SKR-Konto> <Kontoname>", so both the full account name and
// the booking account number are visible. Falls back to the plain name when no
// SKR account is assigned.
func (a *App) accountRichLabel(name string) string {
	for _, ba := range a.settings.BankAccounts {
		if ba.Name != name {
			continue
		}
		if ba.SKR04Konto != 0 {
			chartName := ""
			if acc, ok := a.chart.Find(ba.SKR04Konto); ok && acc.Name != "" {
				chartName = " " + acc.Name
			}
			return fmt.Sprintf("%s  ·  %d%s", name, ba.SKR04Konto, chartName)
		}
		return name
	}
	return name
}

// buildKontenContent builds the bank-statement browser BODY only: the
// account picker, the Konten-specific action buttons (upload / auto-fill /
// missing receipts) and the statement list + preview split. It returns NO
// chrome — the outer shell (buildUI) provides the period header, sidebar,
// view switching and status bar — so nothing double-renders.
func (a *App) buildKontenContent() fyne.CanvasObject {
	accounts := a.bankAccountOptionList()
	if a.kontenAccount == "" && len(accounts) > 0 {
		a.kontenAccount = accounts[0]
	}

	// Account picker: chip row when ≤ 6 accounts (quick visual switch),
	// dropdown fallback above that count to avoid wrapping.
	var accountPicker fyne.CanvasObject
	if len(accounts) > 0 && len(accounts) <= 6 {
		chips := make([]fyne.CanvasObject, 0, len(accounts))
		for _, name := range accounts {
			n := name
			b := widget.NewButton(a.accountRichLabel(n), func() { // show name + Kontonummer
				if n == a.kontenAccount {
					return
				}
				a.kontenAccount = n
				a.window.SetContent(a.buildUI())
			})
			if n == a.kontenAccount {
				b.Importance = widget.HighImportance
			} else {
				b.Importance = widget.LowImportance
			}
			chips = append(chips, b)
		}
		accountPicker = container.NewHBox(chips...)
	} else {
		// Rich options "<name> · <Konto> <Name>" with a map back to the plain
		// account name (which is the stored key / statement-folder name).
		richOptions := make([]string, 0, len(accounts))
		richToName := make(map[string]string, len(accounts))
		nameToRich := make(map[string]string, len(accounts))
		for _, name := range accounts {
			rich := a.accountRichLabel(name)
			richOptions = append(richOptions, rich)
			richToName[rich] = name
			nameToRich[name] = rich
		}
		accountSelect := widget.NewSelect(richOptions, func(sel string) {
			name := richToName[sel]
			if name == a.kontenAccount {
				return
			}
			a.kontenAccount = name
			a.window.SetContent(a.buildUI())
		})
		if a.kontenAccount != "" {
			accountSelect.SetSelected(nameToRich[a.kontenAccount])
		}
		// Give the dropdown more width so the full label fits.
		wide := container.NewGridWrap(fyne.NewSize(460, accountSelect.MinSize().Height), accountSelect)
		accountPicker = container.NewHBox(widget.NewLabel("Konto:"), wide)
	}

	// Body — fileList + preview HSplit. Built per render so reload
	// after upload picks up the new file.
	var body fyne.CanvasObject
	uploadBtn := widget.NewButtonWithIcon("Kontoauszug hochladen",
		theme.UploadIcon(), func() {
			if a.kontenAccount == "" {
				dialog.ShowInformation("Kontoauszug",
					"Bitte zuerst ein Zahlungskonto auswählen.", a.window)
				return
			}
			a.showFilesPickerFor(pickerKontoauszug, func(paths []string) {
				folder := a.statementFolder(a.kontenAccount)
				if err := os.MkdirAll(folder, 0755); err != nil {
					dialog.ShowError(err, a.window)
					return
				}
				var failures []string
				var newNames []string
				for _, p := range paths {
					finalName, err := a.placeFile(p, folder, filepath.Base(p))
					if err != nil {
						failures = append(failures,
							fmt.Sprintf("%s: %v", filepath.Base(p), err))
						a.logger.Warn("Statement upload failed for %s: %v", p, err)
						continue
					}
					newNames = append(newNames, finalName)
				}
				if len(failures) > 0 {
					dialog.ShowError(fmt.Errorf(
						"%d Datei(en) konnten nicht abgelegt werden:\n%s",
						len(failures), strings.Join(failures, "\n")), a.window)
				}
				a.autoFillNewStatements(folder, newNames)
			})
		})
	uploadBtn.Importance = widget.HighImportance

	autoFillAllBtn := widget.NewButtonWithIcon("Alle auto-füllen",
		theme.SearchReplaceIcon(), func() {
			if a.kontenAccount == "" {
				return
			}
			a.autoFillAllStatements(a.kontenAccount)
		})
	autoFillAllBtn.Importance = widget.LowImportance

	// E20.6: "Fehlende Belege" — lists debit statement lines with no linked invoice.
	missingBtn := widget.NewButtonWithIcon(a.bundle.T("missing.title"),
		theme.WarningIcon(), func() {
			if a.kontenAccount == "" {
				dialog.ShowInformation(a.bundle.T("missing.title"),
					"Bitte zuerst ein Zahlungskonto auswählen.", a.window)
				return
			}
			a.showMissingReceipts(a.kontenAccount)
		})
	missingBtn.Importance = widget.LowImportance

	// Content-local header: account picker on the left, the three
	// Konten-specific actions on the right. No view toggles, no global
	// settings gear — those live in the outer shell.
	contentHeader := container.NewBorder(nil, nil,
		accountPicker,
		container.NewHBox(missingBtn, autoFillAllBtn, uploadBtn))

	switch {
	case len(accounts) == 0:
		settingsCTA := widget.NewButtonWithIcon("Konto anlegen",
			theme.SettingsIcon(), func() { a.showSettingsView() })
		settingsCTA.Importance = widget.HighImportance
		body = emptyState(
			theme.AccountIcon(),
			"Noch kein Zahlungskonto",
			"Lege in den Einstellungen dein erstes Zahlungskonto an, "+
				"damit du Kontoauszüge hochladen kannst.",
			settingsCTA)
	case a.kontenAccount == "":
		body = emptyState(
			theme.AccountIcon(),
			"Bitte ein Konto wählen",
			"Klick oben auf 'Konten ▾' um ein Zahlungskonto zu öffnen.",
			nil)
	default:
		body = a.buildKontenSplit()
	}

	return container.NewBorder(contentHeader, nil, nil, nil, body)
}

// buildStatementStats renders three small stat cards above the
// statement table: count, period covered, latest closing balance.
func (a *App) buildStatementStats(metaMap core.StatementMetadataMap, all []string) fyne.CanvasObject {
	sep := ","
	if a.settings.DecimalSeparator != "" {
		sep = a.settings.DecimalSeparator
	}

	// Period: earliest DateFrom → latest DateTo across metadata.
	var earliest, latest time.Time
	var latestClosing float64
	var latestClosingDate time.Time
	for _, name := range all {
		m := metaMap[name]
		if m.DateFrom != "" {
			if t := parseGermanDate(m.DateFrom); !t.IsZero() {
				if earliest.IsZero() || t.Before(earliest) {
					earliest = t
				}
			}
		}
		if m.DateTo != "" {
			if t := parseGermanDate(m.DateTo); !t.IsZero() {
				if latest.IsZero() || t.After(latest) {
					latest = t
				}
				if t.After(latestClosingDate) || latestClosingDate.IsZero() {
					latestClosingDate = t
					latestClosing = m.ClosingBalance
				}
			}
		}
	}

	period := "—"
	if !earliest.IsZero() && !latest.IsZero() {
		period = fmt.Sprintf("%s  –  %s",
			earliest.Format("02.01.2006"), latest.Format("02.01.2006"))
	}
	closing := "—"
	if !latestClosingDate.IsZero() {
		closing = formatDecimal(latestClosing, sep)
	}

	statCard := func(title, value string) fyne.CanvasObject {
		t := widget.NewLabelWithStyle(title,
			fyne.TextAlignLeading, fyne.TextStyle{})
		v := widget.NewLabelWithStyle(value,
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		bg := canvas.NewRectangle(cardBackgroundColor())
		bg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
		bg.StrokeWidth = 1
		bg.CornerRadius = 6
		return container.NewStack(bg,
			container.NewPadded(container.NewVBox(t, v)))
	}

	return container.NewGridWithColumns(3,
		statCard("Anzahl Auszüge", fmt.Sprintf("%d", len(all))),
		statCard("Zeitraum gesamt", period),
		statCard("Letzter Endsaldo", closing),
	)
}

// kontenColumn describes one column of the Konten statement table.
type kontenColumn struct {
	key       string
	title     string
	width     float32
	alignment fyne.TextAlign
	cell      func(rel string, m core.StatementMetadata, sep string) string
	less      func(am, bm core.StatementMetadata, ar, br string) bool
}

func (a *App) kontenColumns() []kontenColumn {
	return []kontenColumn{
		{
			key: "datei", title: "Datei", width: 240, alignment: fyne.TextAlignLeading,
			// Just the bare filename; the year folder is captured by the
			// "Zeitraum von/bis" columns anyway.
			cell: func(rel string, _ core.StatementMetadata, _ string) string {
				return filepath.Base(rel)
			},
			less: func(_, _ core.StatementMetadata, a, b string) bool {
				return strings.ToLower(filepath.Base(a)) < strings.ToLower(filepath.Base(b))
			},
		},
		{
			key: "von", title: "Zeitraum von", width: 110, alignment: fyne.TextAlignLeading,
			cell: func(_ string, m core.StatementMetadata, _ string) string { return m.DateFrom },
			less: func(a, b core.StatementMetadata, _, _ string) bool {
				return parseGermanDate(a.DateFrom).Before(parseGermanDate(b.DateFrom))
			},
		},
		{
			key: "bis", title: "Zeitraum bis", width: 110, alignment: fyne.TextAlignLeading,
			cell: func(_ string, m core.StatementMetadata, _ string) string { return m.DateTo },
			less: func(a, b core.StatementMetadata, _, _ string) bool {
				return parseGermanDate(a.DateTo).Before(parseGermanDate(b.DateTo))
			},
		},
		{
			key: "nr", title: "Auszugsnummer", width: 120, alignment: fyne.TextAlignLeading,
			cell: func(_ string, m core.StatementMetadata, _ string) string { return m.Number },
			less: func(a, b core.StatementMetadata, _, _ string) bool {
				return strings.ToLower(a.Number) < strings.ToLower(b.Number)
			},
		},
		{
			key: "anfang", title: "Anfangssaldo", width: 120, alignment: fyne.TextAlignTrailing,
			cell: func(_ string, m core.StatementMetadata, sep string) string {
				return formatDecimal(m.OpeningBalance, sep)
			},
			less: func(a, b core.StatementMetadata, _, _ string) bool {
				return a.OpeningBalance < b.OpeningBalance
			},
		},
		{
			key: "ende", title: "Endsaldo", width: 120, alignment: fyne.TextAlignTrailing,
			cell: func(_ string, m core.StatementMetadata, sep string) string {
				return formatDecimal(m.ClosingBalance, sep)
			},
			less: func(a, b core.StatementMetadata, _, _ string) bool {
				return a.ClosingBalance < b.ClosingBalance
			},
		},
		{
			key: "geprueft", title: "Geprüft", width: 80, alignment: fyne.TextAlignCenter,
			cell: func(_ string, m core.StatementMetadata, _ string) string {
				if m.Reviewed {
					return "✓"
				}
				return ""
			},
			less: func(a, b core.StatementMetadata, _, _ string) bool {
				if a.Reviewed == b.Reviewed {
					return false
				}
				return !a.Reviewed
			},
		},
		{
			key: "notiz", title: "Notiz", width: 220, alignment: fyne.TextAlignLeading,
			cell: func(_ string, m core.StatementMetadata, _ string) string { return m.Note },
			less: func(a, b core.StatementMetadata, _, _ string) bool {
				return strings.ToLower(a.Note) < strings.ToLower(b.Note)
			},
		},
	}
}

// buildKontenSplit renders the statement table + preview for the
// active Zahlungskonto. Columns are sortable (click header), rows
// support right-click "Bearbeiten" / "Löschen", and click selects a
// row for preview on the right.
func (a *App) buildKontenSplit() fyne.CanvasObject {
	folder := a.statementFolder(a.kontenAccount)
	metaMap, _ := core.LoadStatementMeta(folder)
	all := a.listStatements(a.kontenAccount)
	filtered := append([]string(nil), all...)
	statsBar := a.buildStatementStats(metaMap, all)

	cols := a.kontenColumns()
	sep := a.settings.DecimalSeparator
	if sep == "" {
		sep = ","
	}

	previewHolder := container.NewStack(container.NewCenter(
		widget.NewLabel("Bitte einen Kontoauszug auswählen.")))

	countLabel := widget.NewLabel("")
	updateCount := func() {
		if len(filtered) == len(all) {
			countLabel.SetText(fmt.Sprintf("%d Kontoauszüge", len(all)))
		} else {
			countLabel.SetText(fmt.Sprintf("%d / %d Kontoauszüge", len(filtered), len(all)))
		}
	}

	applySort := func() {
		if a.kontenSortCol == "" {
			return
		}
		var col *kontenColumn
		for i := range cols {
			if cols[i].key == a.kontenSortCol {
				col = &cols[i]
				break
			}
		}
		if col == nil || col.less == nil {
			return
		}
		sort.SliceStable(filtered, func(i, j int) bool {
			ar, br := filtered[i], filtered[j]
			am, bm := metaMap[ar], metaMap[br]
			if a.kontenSortAsc {
				return col.less(am, bm, ar, br)
			}
			return col.less(bm, am, br, ar)
		})
	}

	var selectedRow int = -1
	var table *widget.Table

	updatePreview := func(rel string) {
		statementPath := filepath.Join(folder, rel)
		preview, _ := buildDocumentPreview(statementPath, core.Meta{})

		// Build numbered booking sidebar; refresh metaMap if the
		// sidebar's call to EnsureBookingsParsed mutated the entry.
		stm := metaMap[rel]
		sidebar := a.buildBookingSidebar(statementPath, &stm,
			func(b core.StatementBooking) {
				a.onBookingTapped(folder, rel, b)
			})
		metaMap[rel] = stm

		// Sidebar on the left, document on the right.
		split := container.NewHSplit(sidebar, preview)
		split.SetOffset(0.25) // sidebar ≈ 25 %, preview ≈ 75 %
		previewHolder.Objects = []fyne.CanvasObject{split}
		previewHolder.Refresh()
	}

	// Helpers used by the two leading action cells.
	openEdit := func(rel string) {
		a.showStatementEditDialog(folder, rel, metaMap[rel])
	}
	confirmDelete := func(rel string) {
		dialog.ShowConfirm("Kontoauszug löschen",
			fmt.Sprintf("Datei %q wirklich löschen?", rel),
			func(ok bool) {
				if !ok {
					return
				}
				target := filepath.Join(folder, rel)
				if err := os.Remove(target); err != nil {
					dialog.ShowError(err, a.window)
					return
				}
				if _, ok := metaMap[rel]; ok {
					delete(metaMap, rel)
					_ = core.SaveStatementMeta(folder, metaMap)
				}
				a.window.SetContent(a.buildUI())
			}, a.window)
	}

	// Total columns = 2 leading action columns (edit, delete) + data.
	const editCol, deleteCol = 0, 1
	dataOffset := 2

	table = widget.NewTable(
		func() (int, int) { return len(filtered), len(cols) + dataOffset },
		func() fyne.CanvasObject {
			bg := canvas.NewRectangle(color.Transparent)
			hl := newHoverLabel(nil, nil)
			hl.Truncation = fyne.TextTruncateEllipsis
			return container.NewStack(bg, hl)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			stack := cell.(*fyne.Container)
			bg := stack.Objects[0].(*canvas.Rectangle)
			hl := stack.Objects[1].(*hoverLabel)
			hl.onTap = nil
			hl.TextStyle.Bold = false
			if id.Row%2 == 0 {
				bg.FillColor = stripeColor()
			} else {
				bg.FillColor = color.Transparent
			}
			bg.Refresh()
			if id.Row >= len(filtered) {
				hl.SetText("")
				return
			}
			rel := filtered[id.Row]
			switch id.Col {
			case editCol:
				hl.Alignment = fyne.TextAlignCenter
				hl.SetText("✏️")
				hl.onTap = func() { openEdit(rel) }
			case deleteCol:
				hl.Alignment = fyne.TextAlignCenter
				hl.SetText("🗑")
				hl.onTap = func() { confirmDelete(rel) }
			default:
				idx := id.Col - dataOffset
				if idx < 0 || idx >= len(cols) {
					hl.SetText("")
					return
				}
				col := cols[idx]
				hl.Alignment = col.alignment
				hl.SetText(col.cell(rel, metaMap[rel], sep))
				// Clicking any data cell loads the preview — Fyne's
				// table only fires OnSelected when the cell widget
				// itself doesn't consume the tap, and hoverLabel does.
				rowRel := rel
				hl.onTap = func() {
					selectedRow = id.Row
					updatePreview(rowRel)
				}
			}
		},
	)

	// Column widths: action columns first, then data columns.
	table.SetColumnWidth(editCol, 40)
	table.SetColumnWidth(deleteCol, 40)
	for i, c := range cols {
		table.SetColumnWidth(i+dataOffset, c.width)
	}

	// Sortable header row
	table.ShowHeaderRow = true
	table.CreateHeader = func() fyne.CanvasObject {
		bg := canvas.NewRectangle(headerBackgroundColor)
		h := newHoverLabel(nil, nil)
		return container.NewStack(bg, h)
	}
	table.UpdateHeader = func(id widget.TableCellID, cell fyne.CanvasObject) {
		stack := cell.(*fyne.Container)
		h := stack.Objects[1].(*hoverLabel)
		h.onTap = nil
		h.TextStyle.Bold = true
		// Action columns first.
		switch id.Col {
		case editCol:
			h.Alignment = fyne.TextAlignCenter
			h.SetText("✏️")
			h.Refresh()
			return
		case deleteCol:
			h.Alignment = fyne.TextAlignCenter
			h.SetText("🗑")
			h.Refresh()
			return
		}
		idx := id.Col - dataOffset
		if idx < 0 || idx >= len(cols) {
			h.SetText("")
			h.Refresh()
			return
		}
		col := cols[idx]
		h.Alignment = col.alignment
		text := col.title
		bold := false
		if a.kontenSortCol == col.key {
			if a.kontenSortAsc {
				text += " ▲"
			} else {
				text += " ▼"
			}
			bold = true
		} else if col.less != nil {
			text += " ▴▾"
		}
		h.SetText(text)
		h.TextStyle.Bold = bold
		colKey := col.key
		hasSort := col.less != nil
		if hasSort {
			h.onTap = func() {
				if a.kontenSortCol == colKey {
					a.kontenSortAsc = !a.kontenSortAsc
				} else {
					a.kontenSortCol = colKey
					a.kontenSortAsc = true
				}
				a.persistKontenSort()
				a.window.SetContent(a.buildUI())
			}
		}
		h.Refresh()
	}

	table.OnSelected = func(id widget.TableCellID) {
		if id.Row < 0 || id.Row >= len(filtered) {
			return
		}
		selectedRow = id.Row
		updatePreview(filtered[id.Row])
	}

	filterEntry := widget.NewEntry()
	filterEntry.SetPlaceHolder("Filtern …")
	filterEntry.OnChanged = func(q string) {
		q = strings.ToLower(strings.TrimSpace(q))
		if q == "" {
			filtered = append([]string(nil), all...)
		} else {
			filtered = filtered[:0]
			for _, n := range all {
				if strings.Contains(strings.ToLower(n), q) {
					filtered = append(filtered, n)
					continue
				}
				m := metaMap[n]
				blob := strings.ToLower(strings.Join([]string{
					m.DateFrom, m.DateTo, m.Number, m.Note,
				}, " "))
				if strings.Contains(blob, q) {
					filtered = append(filtered, n)
				}
			}
		}
		applySort()
		updateCount()
		table.Refresh()
	}

	tableWrap := newContextMenuWrap(table, func(e *fyne.PointEvent) {
		if selectedRow < 0 || selectedRow >= len(filtered) {
			return
		}
		rel := filtered[selectedRow]
		menu := fyne.NewMenu("",
			fyne.NewMenuItem("Bearbeiten", func() { openEdit(rel) }),
			fyne.NewMenuItem("Metadaten auto-füllen", func() {
				a.autoFillOneStatement(folder, rel)
			}),
			fyne.NewMenuItem("Löschen", func() { confirmDelete(rel) }),
		)
		widget.ShowPopUpMenuAtPosition(menu, a.window.Canvas(), e.AbsolutePosition)
	})

	openFolderBtn := widget.NewButton("Ordner öffnen", func() {
		if err := os.MkdirAll(folder, 0755); err != nil {
			dialog.ShowError(err, a.window)
			return
		}
		a.openFolderInOS(folder)
	})
	openFolderBtn.Importance = widget.LowImportance

	applySort()
	updateCount()

	leftHeader := container.NewBorder(nil, nil, countLabel, openFolderBtn, filterEntry)
	leftHead := container.NewVBox(statsBar, leftHeader)
	left := container.NewBorder(leftHead, nil, nil, nil, tableWrap)

	split := container.NewHSplit(left, previewHolder)
	splitOffset := a.settings.PreviewSplitOffset
	if splitOffset < 0.1 || splitOffset > 0.85 {
		splitOffset = 0.55
	}
	split.SetOffset(splitOffset)
	return split
}

// showStatementEditDialog opens a form to edit one statement's
// metadata and persists it on save.
func (a *App) showStatementEditDialog(folder, rel string, current core.StatementMetadata) {
	fromEntry := widget.NewEntry()
	fromEntry.SetText(current.DateFrom)
	fromEntry.SetPlaceHolder("DD.MM.YYYY")

	toEntry := widget.NewEntry()
	toEntry.SetText(current.DateTo)
	toEntry.SetPlaceHolder("DD.MM.YYYY")

	numEntry := widget.NewEntry()
	numEntry.SetText(current.Number)
	numEntry.SetPlaceHolder("z. B. 5/2026")

	sep := a.settings.DecimalSeparator
	if sep == "" {
		sep = ","
	}
	openEntry := widget.NewEntry()
	openEntry.SetText(formatDecimal(current.OpeningBalance, sep))
	openEntry.SetPlaceHolder("0" + sep + "00")

	closeEntry := widget.NewEntry()
	closeEntry.SetText(formatDecimal(current.ClosingBalance, sep))
	closeEntry.SetPlaceHolder("0" + sep + "00")

	reviewedCheck := widget.NewCheck("Geprüft", nil)
	reviewedCheck.SetChecked(current.Reviewed)

	noteEntry := widget.NewMultiLineEntry()
	noteEntry.SetText(current.Note)
	noteEntry.SetMinRowsVisible(3)
	noteEntry.SetPlaceHolder("Optionaler Vermerk")

	items := []*widget.FormItem{
		widget.NewFormItem("Datei", newCopyableLabel(a.bundle, rel)),
		widget.NewFormItem("Zeitraum von", fromEntry),
		widget.NewFormItem("Zeitraum bis", toEntry),
		widget.NewFormItem("Auszugsnummer", numEntry),
		widget.NewFormItem("Anfangssaldo", openEntry),
		widget.NewFormItem("Endsaldo", closeEntry),
		widget.NewFormItem("", reviewedCheck),
		widget.NewFormItem("Notiz", noteEntry),
	}

	dialog.ShowForm("Kontoauszug bearbeiten", "Speichern", "Abbrechen", items,
		func(ok bool) {
			if !ok {
				return
			}
			updated := core.StatementMetadata{
				DateFrom:       strings.TrimSpace(fromEntry.Text),
				DateTo:         strings.TrimSpace(toEntry.Text),
				Number:         strings.TrimSpace(numEntry.Text),
				OpeningBalance: parseFloat(openEntry.Text, sep),
				ClosingBalance: parseFloat(closeEntry.Text, sep),
				Reviewed:       reviewedCheck.Checked,
				Note:           strings.TrimSpace(noteEntry.Text),
			}
			metaMap, err := core.LoadStatementMeta(folder)
			if err != nil {
				dialog.ShowError(err, a.window)
				return
			}
			metaMap[rel] = updated
			if err := core.SaveStatementMeta(folder, metaMap); err != nil {
				dialog.ShowError(err, a.window)
				return
			}
			a.window.SetContent(a.buildUI())
		}, a.window)
}

// showMissingReceipts lists DEBIT statement lines for the given account
// that are not linked to any invoice via BuchungRef. This surfaces
// unmatched bank transactions so the user can find missing receipts.
//
// E20.6: uses the same parse-once approach as showBelegabgleich — every
// statement file for the account is scanned; debit lines whose ref key
// does not appear in any invoice's BuchungRef are collected and shown.
func (a *App) showMissingReceipts(account string) {
	// ── collect linked BuchungRef keys from ALL invoices across ALL months ──
	// We pull all rows from the database and build a set of claimed keys.
	// Using ListAll (or iterating all months) is overkill; instead we scan the
	// current year ± one for simplicity — the debit lines are also from the
	// currently stored statements.
	linkedKeys := map[string]bool{}
	for yr := a.currentYear - 1; yr <= a.currentYear+1; yr++ {
		for mo := 1; mo <= 12; mo++ {
			rows, err := a.dbRepo.List(fmt.Sprintf("%04d", yr), fmt.Sprintf("%02d", mo))
			if err != nil {
				continue
			}
			for _, row := range rows {
				if row.BuchungRef != "" {
					linkedKeys[row.BuchungRef] = true
				}
			}
		}
	}

	// ── parse all statement files for this account ──
	folder := a.statementFolder(account)
	type missingLine struct {
		Date   string
		Betrag float64
		Text   string
	}
	var missing []missingLine

	for _, name := range a.listStatements(account) {
		fullPath := filepath.Join(folder, name)
		lines, err := core.ParseStatementBookings(fullPath)
		if err != nil {
			a.logger.Warn("showMissingReceipts: parse %s: %v", name, err)
			continue
		}
		for _, l := range lines {
			if l.IstGutschrift {
				continue // only debit (expense) lines
			}
			key := core.BuchungRef{
				StatementFilename: name,
				Page:              l.Page,
				LineIdx:           l.LineIdx,
			}.String()
			if linkedKeys[key] {
				continue // already linked to an invoice
			}
			missing = append(missing, missingLine{
				Date:   l.Date,
				Betrag: l.Betrag,
				Text:   l.Text,
			})
		}
	}

	// ── build dialog ──
	sep := a.settings.DecimalSeparator
	if sep == "" {
		sep = ","
	}

	var content fyne.CanvasObject
	if len(missing) == 0 {
		content = container.NewVScroll(container.NewVBox(
			newCopyableLabel(a.bundle, a.bundle.T("missing.none")),
		))
	} else {
		// Header row
		hDate := widget.NewLabelWithStyle(a.bundle.T("missing.col.date"),
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		hAmt := widget.NewLabelWithStyle(a.bundle.T("missing.col.amount"),
			fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})
		hText := widget.NewLabelWithStyle(a.bundle.T("missing.col.text"),
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		headerRow := container.NewGridWithColumns(3, hDate, hAmt, hText)

		vbox := container.NewVBox(headerRow, widget.NewSeparator())
		for _, m := range missing {
			m := m // capture
			amtStr := formatDecimal(m.Betrag, sep)
			lDate := newCopyableLabel(a.bundle, m.Date)
			lAmt := newCopyableLabel(a.bundle, amtStr)
			lAmt.Alignment = fyne.TextAlignTrailing
			lText := newCopyableLabel(a.bundle, m.Text)
			lText.Wrapping = fyne.TextWrapWord
			vbox.Add(container.NewGridWithColumns(3, lDate, lAmt, lText))
		}
		content = container.NewVScroll(vbox)
	}

	dlg := dialog.NewCustom(
		a.bundle.T("missing.title"),
		a.bundle.T("common.close"),
		content,
		a.window,
	)
	dlg.Resize(fyne.NewSize(700, 420))
	dlg.Show()
}
