package ui

import (
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// showCustomFilePicker shows a custom multi-select file picker with search.
func (a *App) showCustomFilePicker() {
	startFolder := a.getStartingFolder()
	currentPath := startFolder
	allFiles := []os.DirEntry{}
	filteredFiles := []os.DirEntry{}

	// Persistent selection across folder navigation.
	selected := []string{} // absolute paths, in selection order
	mainPath := ""         // which selected path is the invoice main file

	isSelected := func(path string) bool {
		for _, s := range selected {
			if s == path {
				return true
			}
		}
		return false
	}

	// Clickable breadcrumb path bar. loadFiles is forward-declared so the
	// breadcrumb segment buttons can navigate.
	var loadFiles func(path string)
	breadcrumb := container.NewHBox()
	breadcrumbScroll := container.NewHScroll(breadcrumb)

	updateBreadcrumb := func(path string) {
		breadcrumb.RemoveAll()
		segments := splitPathSegments(path)
		cumulative := ""
		for i, seg := range segments {
			if i == 0 {
				cumulative = seg
			} else {
				cumulative = filepath.Join(cumulative, seg)
			}
			target := cumulative
			if i == len(segments)-1 {
				// Current folder: render as a button too (NOT a label), so it
				// sits on the same baseline as the other segments instead of
				// floating higher. Highlighted to mark "you are here".
				cur := widget.NewButton(seg, func() { loadFiles(target) })
				cur.Importance = widget.HighImportance
				breadcrumb.Add(cur)
				continue
			}
			segBtn := widget.NewButton(seg, func() { loadFiles(target) })
			segBtn.Importance = widget.LowImportance
			breadcrumb.Add(segBtn)
			sep := widget.NewLabel("›")
			breadcrumb.Add(container.NewCenter(sep)) // vertically center the separator to the buttons
		}
		breadcrumb.Refresh()
		breadcrumbScroll.ScrollToOffset(fyne.NewPos(breadcrumb.MinSize().Width, 0))
	}

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Dateiname suchen (z.B. rechnung, 2025-10, GmbH)...")

	// Selection tray (rebuilt on every change). Rows can be reordered by
	// dragging; the order determines attachment numbering.
	selectionList := container.NewVBox()
	var fileList *widget.List
	var trayRows []*draggableRow
	var refreshSelection func()

	// swapColumns-style in-place adjacent swap, used during a drag so the
	// dragged widget stays attached (no full rebuild mid-drag).
	swapSelected := func(a, b int) {
		n := len(selected)
		if a < 0 || b < 0 || a >= n || b >= n || a == b {
			return
		}
		selected[a], selected[b] = selected[b], selected[a]
		trayRows[a], trayRows[b] = trayRows[b], trayRows[a]
		selectionList.Objects[a], selectionList.Objects[b] = selectionList.Objects[b], selectionList.Objects[a]
		trayRows[a].index = a
		trayRows[b].index = b
		selectionList.Refresh()
	}

	onTrayDragEnd := func(row *draggableRow) { row.dragAccum = 0 }

	onTrayDrag := func(row *draggableRow, dy float32) {
		row.dragAccum += dy

		pitch := row.Size().Height
		if idx := row.index; idx+1 < len(trayRows) {
			pitch = trayRows[idx+1].Position().Y - row.Position().Y
		} else if idx > 0 {
			pitch = row.Position().Y - trayRows[idx-1].Position().Y
		}
		if pitch <= 0 {
			return
		}

		for row.dragAccum > pitch/2 && row.index < len(trayRows)-1 {
			swapSelected(row.index, row.index+1)
			row.dragAccum -= pitch
		}
		for row.dragAccum < -pitch/2 && row.index > 0 {
			swapSelected(row.index, row.index-1)
			row.dragAccum += pitch
		}
	}

	refreshSelection = func() {
		selectionList.RemoveAll()
		trayRows = nil
		if len(selected) == 0 {
			selectionList.Add(widget.NewLabel("Noch keine Dateien ausgewählt."))
			selectionList.Refresh()
			return
		}
		for i, p := range selected {
			path := p
			isMain := path == mainPath

			star := "☆"
			if isMain {
				star = "★"
			}
			starBtn := widget.NewButton(star, func() {
				mainPath = path
				refreshSelection()
			})
			if isMain {
				starBtn.Importance = widget.HighImportance
			} else {
				starBtn.Importance = widget.LowImportance
			}

			removeBtn := widget.NewButton("Entfernen", func() {
				next := make([]string, 0, len(selected))
				for _, s := range selected {
					if s != path {
						next = append(next, s)
					}
				}
				selected = next
				if mainPath == path {
					mainPath = ""
					if len(selected) > 0 {
						mainPath = selected[0]
					}
				}
				refreshSelection()
				if fileList != nil {
					fileList.Refresh()
				}
			})
			removeBtn.Importance = widget.LowImportance

			grip := widget.NewLabel("↕")
			nameLabel := widget.NewLabel(filepath.Base(path))
			nameLabel.Truncation = fyne.TextTruncateEllipsis

			content := container.NewBorder(
				nil, nil,
				container.NewHBox(grip, starBtn),
				removeBtn,
				nameLabel,
			)
			row := newDraggableRow(content, i)
			row.onDrag = onTrayDrag
			row.onDragEnd = onTrayDragEnd
			trayRows = append(trayRows, row)
			selectionList.Add(row)
		}
		selectionList.Refresh()
	}

	toggleSelected := func(path string, checked bool) {
		if checked {
			if !isSelected(path) {
				selected = append(selected, path)
				if mainPath == "" {
					mainPath = path
				}
			}
		} else {
			next := make([]string, 0, len(selected))
			for _, s := range selected {
				if s != path {
					next = append(next, s)
				}
			}
			selected = next
			if mainPath == path {
				mainPath = ""
				if len(selected) > 0 {
					mainPath = selected[0]
				}
			}
		}
		refreshSelection()
	}

	fileList = widget.NewList(
		func() int { return len(filteredFiles) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewCheck("", nil),
				widget.NewIcon(nil),
				widget.NewLabel("template"),
			)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id >= len(filteredFiles) {
				return
			}
			entry := filteredFiles[id]
			box := item.(*fyne.Container)
			check := box.Objects[0].(*widget.Check)
			icon := box.Objects[1].(*widget.Icon)
			label := box.Objects[2].(*widget.Label)
			fullPath := filepath.Join(currentPath, entry.Name())

			if entry.IsDir() {
				check.OnChanged = nil
				check.SetChecked(false)
				check.Hide()
				icon.SetResource(theme.FolderIcon())
				label.SetText("📁 " + entry.Name())
				return
			}

			check.Show()
			check.OnChanged = nil // avoid SetChecked firing the handler
			check.SetChecked(isSelected(fullPath))
			check.OnChanged = func(checked bool) {
				toggleSelected(fullPath, checked)
			}
			icon.SetResource(theme.FileIcon())
			label.SetText("📄 " + entry.Name())
		},
	)

	loadFiles = func(path string) {
		entries, err := os.ReadDir(path)
		if err != nil {
			a.logger.Error("Failed to read directory: %v", err)
			return
		}
		allFiles = []os.DirEntry{}
		for _, entry := range entries {
			if entry.IsDir() || core.IsSupportedFile(entry.Name()) {
				allFiles = append(allFiles, entry)
			}
		}
		filteredFiles = allFiles
		currentPath = path
		updateBreadcrumb(currentPath)
		searchEntry.SetText("")
		fileList.Refresh()
	}

	applyFilter := func(query string) {
		query = strings.ToLower(strings.TrimSpace(query))
		if query == "" {
			filteredFiles = allFiles
		} else {
			filteredFiles = []os.DirEntry{}
			for _, entry := range allFiles {
				if strings.Contains(strings.ToLower(entry.Name()), query) {
					filteredFiles = append(filteredFiles, entry)
				}
			}
		}
		fileList.Refresh()
	}
	searchEntry.OnChanged = func(query string) { applyFilter(query) }

	upBtn := widget.NewButton("⬆️ Übergeordneter Ordner", func() {
		parent := filepath.Dir(currentPath)
		if parent != currentPath {
			loadFiles(parent)
		}
	})
	homeBtn := widget.NewButton("🏠 Dokumente", func() {
		if docsDir, err := core.GetDocumentsDir(); err == nil {
			loadFiles(docsDir)
		}
	})

	fileList.OnSelected = func(id widget.ListItemID) {
		if id >= len(filteredFiles) {
			fileList.UnselectAll()
			return
		}
		entry := filteredFiles[id]
		if entry.IsDir() {
			loadFiles(filepath.Join(currentPath, entry.Name()))
			fileList.UnselectAll()
			return
		}
		// Left-click on a file row toggles its checkbox.
		fullPath := filepath.Join(currentPath, entry.Name())
		toggleSelected(fullPath, !isSelected(fullPath))
		fileList.Refresh()
		fileList.UnselectAll()
	}

	// Desktop button
	desktopBtn := widget.NewButton("🖥️ Desktop", func() {
		home, err := os.UserHomeDir()
		if err == nil {
			desktopPath := filepath.Join(home, "Desktop")
			if _, err := os.Stat(desktopPath); err == nil {
				loadFiles(desktopPath)
			}
		}
	})

	// Downloads button
	downloadsBtn := widget.NewButton("📥 Downloads", func() {
		home, err := os.UserHomeDir()
		if err == nil {
			downloadsPath := filepath.Join(home, "Downloads")
			if _, err := os.Stat(downloadsPath); err == nil {
				loadFiles(downloadsPath)
			}
		}
	})

	loadFiles(startFolder)
	refreshSelection()

	selectionScroll := container.NewVScroll(selectionList)
	selectionScroll.SetMinSize(fyne.NewSize(0, 150))

	// Compact theme override: reduced padding so more rows fit on screen.
	compactList := container.NewThemeOverride(fileList, newCompactListTheme(a.theme))
	// Compact breadcrumb: reduced padding so the path segments sit tightly.
	compactBreadcrumb := container.NewThemeOverride(breadcrumbScroll, newCompactListTheme(a.theme))

	content := container.NewBorder(
		container.NewVBox(
			container.NewHBox(upBtn, homeBtn, desktopBtn, downloadsBtn),
			compactBreadcrumb,
			searchEntry,
			widget.NewSeparator(),
		),
		container.NewVBox(
			widget.NewSeparator(),
			widget.NewLabelWithStyle(
				"Auswahl (★ = Hauptdatei)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
			),
			selectionScroll,
		),
		nil, nil,
		compactList,
	)

	// Separate, user-resizable window (a Fyne dialog cannot be drag-resized).
	pickerWin := a.app.NewWindow("Dateien auswählen")

	abbrechenBtn := widget.NewButton("Abbrechen", func() {
		pickerWin.Close()
	})
	oeffnenBtn := widget.NewButton("Öffnen", nil)
	oeffnenBtn.Importance = widget.HighImportance
	oeffnenBtn.OnTapped = func() {
		if len(selected) == 0 {
			dialog.ShowInformation("Fehler", "Bitte mindestens eine Datei auswählen.", pickerWin)
			return
		}
		if mainPath == "" {
			dialog.ShowInformation("Fehler", "Bitte eine Hauptdatei markieren (★).", pickerWin)
			return
		}
		attachments := make([]string, 0, len(selected))
		for _, s := range selected {
			if s != mainPath {
				attachments = append(attachments, s)
			}
		}
		a.settings.LastUsedFolder = currentPath
		if err := a.settingsMgr.Save(a.settings); err != nil {
			a.logger.Warn("Failed to save last used folder: %v", err)
		}
		pickerWin.Close()
		a.processSubmission(mainPath, attachments, nil)
	}

	buttonBar := container.NewBorder(nil, nil, nil,
		container.NewHBox(abbrechenBtn, oeffnenBtn),
	)

	pickerWin.SetContent(container.NewBorder(nil, buttonBar, nil, nil, content))
	pickerWin.Resize(fyne.NewSize(1000, 760))
	pickerWin.CenterOnScreen()
	pickerWin.Show()
}

// getStartingFolder determines the best starting folder for the file picker.
func (a *App) getStartingFolder() string {
	// Priority: last used > storage root > Documents
	if a.settings.LastUsedFolder != "" {
		if _, err := os.Stat(a.settings.LastUsedFolder); err == nil {
			return a.settings.LastUsedFolder
		}
	}

	if a.settings.StorageRoot != "" {
		if _, err := os.Stat(a.settings.StorageRoot); err == nil {
			return a.settings.StorageRoot
		}
	}

	docsDir, err := core.GetDocumentsDir()
	if err == nil {
		return docsDir
	}

	home, _ := os.UserHomeDir()
	return home
}

// splitPathSegments breaks an absolute path into its components, with the
// first element being the root (e.g. "C:\"). Each element joined with the
// preceding ones yields a navigable folder path.
func splitPathSegments(p string) []string {
	p = filepath.Clean(p)
	var parts []string
	for {
		parent := filepath.Dir(p)
		if parent == p {
			// Reached the root (e.g. "C:\" or "/").
			parts = append([]string{p}, parts...)
			break
		}
		parts = append([]string{filepath.Base(p)}, parts...)
		p = parent
	}
	return parts
}
