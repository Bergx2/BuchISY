package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// fileEntry holds file information for the picker.
type fileEntry struct {
	entry   os.DirEntry
	modTime time.Time
	name    string
	isDir   bool
}

// showCustomFilePicker shows a custom file picker with search functionality.
func (a *App) showCustomFilePicker() {
	// Determine starting folder
	startFolder := a.getStartingFolder()

	currentPath := startFolder
	allFiles := []fileEntry{}
	filteredFiles := []fileEntry{}
	selectedIndex := -1

	// Sort state: 0 = name asc, 1 = name desc, 2 = date asc, 3 = date desc
	sortState := 0

	// Path label
	pathLabel := widget.NewLabel(currentPath)
	pathLabel.Wrapping = fyne.TextWrapBreak

	// Search entry
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Dateiname suchen (z.B. rechnung, 2025-10, GmbH)...")

	// Sort function
	sortFiles := func() {
		switch sortState {
		case 0: // Name ascending
			sort.Slice(filteredFiles, func(i, j int) bool {
				// Directories first
				if filteredFiles[i].isDir != filteredFiles[j].isDir {
					return filteredFiles[i].isDir
				}
				return strings.ToLower(filteredFiles[i].name) < strings.ToLower(filteredFiles[j].name)
			})
		case 1: // Name descending
			sort.Slice(filteredFiles, func(i, j int) bool {
				if filteredFiles[i].isDir != filteredFiles[j].isDir {
					return filteredFiles[i].isDir
				}
				return strings.ToLower(filteredFiles[i].name) > strings.ToLower(filteredFiles[j].name)
			})
		case 2: // Date ascending (oldest first)
			sort.Slice(filteredFiles, func(i, j int) bool {
				if filteredFiles[i].isDir != filteredFiles[j].isDir {
					return filteredFiles[i].isDir
				}
				return filteredFiles[i].modTime.Before(filteredFiles[j].modTime)
			})
		case 3: // Date descending (newest first)
			sort.Slice(filteredFiles, func(i, j int) bool {
				if filteredFiles[i].isDir != filteredFiles[j].isDir {
					return filteredFiles[i].isDir
				}
				return filteredFiles[i].modTime.After(filteredFiles[j].modTime)
			})
		}
	}

	// File table
	fileTable := widget.NewTable(
		func() (int, int) {
			return len(filteredFiles) + 1, 2 // +1 for header row, 2 columns
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			label := cell.(*widget.Label)

			if id.Row == 0 {
				// Header row
				label.TextStyle.Bold = true
				if id.Col == 0 {
					label.SetText("Dateiname")
				} else {
					label.SetText("Datum")
				}
			} else {
				// Data rows
				label.TextStyle.Bold = false
				dataRow := id.Row - 1
				if dataRow >= len(filteredFiles) {
					label.SetText("")
					return
				}

				file := filteredFiles[dataRow]
				if id.Col == 0 {
					// Filename column
					if file.isDir {
						label.SetText("📁 " + file.name)
					} else {
						label.SetText("📄 " + file.name)
					}
				} else {
					// Date column
					if file.isDir {
						label.SetText("")
					} else {
						// Shorter date format to fit in 80px
						label.SetText(file.modTime.Format("02.01.06"))
					}
				}
			}
		},
	)

	// Set column widths
	fileTable.SetColumnWidth(0, 920) // Filename - takes most space
	fileTable.SetColumnWidth(1, 80)  // Date - very compact (80px as requested)

	// Define loadFiles function FIRST (before it's used in handlers)
	// Note: must be var because it's recursive
	var loadFiles func(string)
	loadFiles = func(path string) {
		entries, err := os.ReadDir(path)
		if err != nil {
			a.logger.Error("Failed to read directory: %v", err)
			return
		}

		// Filter: only directories and PDF files, and get mod times
		allFiles = []fileEntry{}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasSuffix(strings.ToLower(entry.Name()), ".pdf") {
				info, err := entry.Info()
				modTime := time.Time{}
				if err == nil {
					modTime = info.ModTime()
				}

				allFiles = append(allFiles, fileEntry{
					entry:   entry,
					modTime: modTime,
					name:    entry.Name(),
					isDir:   entry.IsDir(),
				})
			}
		}

		filteredFiles = allFiles
		sortFiles()
		currentPath = path
		pathLabel.SetText(currentPath)
		searchEntry.SetText("") // Clear search
		fileTable.Refresh()
	}

	// Header click handling for sorting
	fileTable.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			// Header clicked - toggle sort
			if id.Col == 0 {
				// Name column
				if sortState == 0 {
					sortState = 1 // Switch to descending
				} else {
					sortState = 0 // Switch to ascending
				}
			} else {
				// Date column
				if sortState == 2 {
					sortState = 3 // Switch to descending
				} else {
					sortState = 2 // Switch to ascending
				}
			}
			sortFiles()
			fileTable.Refresh()
			fileTable.UnselectAll()
		} else {
			// Data row clicked
			selectedIndex = id.Row - 1
			dataRow := selectedIndex
			if dataRow >= 0 && dataRow < len(filteredFiles) {
				file := filteredFiles[dataRow]
				fullPath := filepath.Join(currentPath, file.name)

				if file.isDir {
					// Navigate into directory
					loadFiles(fullPath)
					fileTable.UnselectAll()
					selectedIndex = -1
				}
			}
		}
	}

	// Apply search filter
	applyFilter := func(query string) {
		query = strings.ToLower(strings.TrimSpace(query))
		if query == "" {
			filteredFiles = allFiles
		} else {
			filteredFiles = []fileEntry{}
			for _, file := range allFiles {
				if strings.Contains(strings.ToLower(file.name), query) {
					filteredFiles = append(filteredFiles, file)
				}
			}
		}
		sortFiles()
		fileTable.Refresh()
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

	// Initial load
	loadFiles(startFolder)

	// Layout
	content := container.NewBorder(
		container.NewVBox(
			container.NewHBox(upBtn, homeBtn, desktopBtn, downloadsBtn),
			pathLabel,
			searchEntry,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		fileTable,
	)

	// Custom dialog
	customDialog := dialog.NewCustomConfirm(
		"Datei auswählen",
		"Öffnen",
		"Abbrechen",
		content,
		func(open bool) {
			if !open {
				return
			}

			// Get selected file
			if selectedIndex < 0 || selectedIndex >= len(filteredFiles) {
				a.showError("Fehler", "Bitte eine Datei auswählen.")
				return
			}

			file := filteredFiles[selectedIndex]
			if file.isDir {
				a.showError("Fehler", "Bitte eine Datei auswählen (kein Ordner).")
				return
			}

			fullPath := filepath.Join(currentPath, file.name)

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
