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

// showCustomFilePicker shows a custom file picker with search functionality.
func (a *App) showCustomFilePicker() {
	// Determine starting folder
	startFolder := a.getStartingFolder()

	currentPath := startFolder
	allFiles := []os.DirEntry{}
	filteredFiles := []os.DirEntry{}
	selectedIndex := -1

	// Path label
	pathLabel := widget.NewLabel(currentPath)
	pathLabel.Wrapping = fyne.TextWrapBreak

	// Search entry
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Dateiname suchen (z.B. rechnung, 2025-10, GmbH)...")

	// File list
	var fileList *widget.List
	fileList = widget.NewList(
		func() int {
			return len(filteredFiles)
		},
		func() fyne.CanvasObject {
			icon := widget.NewIcon(nil)
			label := widget.NewLabel("template")
			return container.NewHBox(icon, label)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id >= len(filteredFiles) {
				return
			}
			entry := filteredFiles[id]
			box := item.(*fyne.Container)
			icon := box.Objects[0].(*widget.Icon)
			label := box.Objects[1].(*widget.Label)

			if entry.IsDir() {
				icon.SetResource(theme.FolderIcon())
				label.SetText("📁 " + entry.Name())
			} else {
				icon.SetResource(theme.FileIcon())
				label.SetText("📄 " + entry.Name())
			}
		},
	)

	// Load files function
	loadFiles := func(path string) {
		entries, err := os.ReadDir(path)
		if err != nil {
			a.logger.Error("Failed to read directory: %v", err)
			return
		}

		// Filter: only directories and PDF files
		allFiles = []os.DirEntry{}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasSuffix(strings.ToLower(entry.Name()), ".pdf") {
				allFiles = append(allFiles, entry)
			}
		}

		filteredFiles = allFiles
		currentPath = path
		pathLabel.SetText(currentPath)
		searchEntry.SetText("") // Clear search
		fileList.Refresh()
	}

	// Apply search filter
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

	searchEntry.OnChanged = func(query string) {
		applyFilter(query)
	}

	// Up button (parent folder)
	upBtn := widget.NewButton("⬆️ Übergeordneter Ordner", func() {
		parent := filepath.Dir(currentPath)
		if parent != currentPath {
			loadFiles(parent)
		}
	})

	// Home button
	homeBtn := widget.NewButton("🏠 Dokumente", func() {
		docsDir, err := core.GetDocumentsDir()
		if err == nil {
			loadFiles(docsDir)
		}
	})

	// Selection handler
	fileList.OnSelected = func(id widget.ListItemID) {
		if id >= len(filteredFiles) {
			return
		}
		selectedIndex = id
		entry := filteredFiles[id]
		fullPath := filepath.Join(currentPath, entry.Name())

		if entry.IsDir() {
			// Navigate into directory
			loadFiles(fullPath)
			fileList.UnselectAll()
			selectedIndex = -1
		}
		// For files, keep selected (user will click Open button)
	}

	// Initial load
	loadFiles(startFolder)

	// Layout
	content := container.NewBorder(
		container.NewVBox(
			container.NewHBox(upBtn, homeBtn),
			pathLabel,
			searchEntry,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		fileList,
	)

	// Custom dialog
	customDialog := dialog.NewCustomConfirm(
		"PDF auswählen",
		"Öffnen",
		"Abbrechen",
		content,
		func(open bool) {
			if !open {
				return
			}

			// Get selected file
			if selectedIndex < 0 || selectedIndex >= len(filteredFiles) {
				a.showError("Fehler", "Bitte eine PDF-Datei auswählen.")
				return
			}

			entry := filteredFiles[selectedIndex]
			if entry.IsDir() {
				a.showError("Fehler", "Bitte eine PDF-Datei auswählen (kein Ordner).")
				return
			}

			fullPath := filepath.Join(currentPath, entry.Name())

			// Remember folder
			a.settings.LastUsedFolder = currentPath
			if err := a.settingsMgr.Save(a.settings); err != nil {
				a.logger.Warn("Failed to save last used folder: %v", err)
			}

			// Process the PDF
			a.processPDFAsync(fullPath)
		},
		a.window,
	)

	customDialog.Resize(fyne.NewSize(1000, 700))
	customDialog.Show()
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
