package core

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// StorageManager handles file system operations for invoices.
type StorageManager struct {
	settings *Settings
}

// NewStorageManager creates a new storage manager.
func NewStorageManager(settings *Settings) *StorageManager {
	return &StorageManager{
		settings: settings,
	}
}

// GetMonthFolder returns the folder path for a given year-month.
// If useMonthSubfolders is false, returns the root storage path.
func (sm *StorageManager) GetMonthFolder(year int, month time.Month) string {
	if !sm.settings.UseMonthSubfolders {
		return sm.settings.StorageRoot
	}

	yearName := fmt.Sprintf("%04d", year)
	monthName := fmt.Sprintf("%04d-%02d", year, month)
	return filepath.Join(sm.settings.StorageRoot, yearName, monthName)
}

// InvoiceFilePath returns the full path of an invoice's main file inside
// its month folder, honouring the row's category subfolder.
func InvoiceFilePath(monthFolder string, row CSVRow) string {
	if row.Unterordner == "" {
		return filepath.Join(monthFolder, row.Dateiname)
	}
	return filepath.Join(monthFolder, row.Unterordner, row.Dateiname)
}

// EnsureMonthFolder creates the month folder if it doesn't exist.
func (sm *StorageManager) EnsureMonthFolder(year int, month time.Month) error {
	folder := sm.GetMonthFolder(year, month)
	return os.MkdirAll(folder, 0755)
}

// GetCSVPath returns the path to the invoices.csv file for a given month.
func (sm *StorageManager) GetCSVPath(year int, month time.Month) string {
	folder := sm.GetMonthFolder(year, month)
	return filepath.Join(folder, "invoices.csv")
}

// prepareTarget ensures targetFolder exists and returns a collision-free
// final name and full path within it (appending _2, _3, … as needed).
func prepareTarget(targetFolder, newName string) (finalName, targetPath string, err error) {
	if err := os.MkdirAll(targetFolder, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create target folder: %w", err)
	}
	finalName = newName
	targetPath = filepath.Join(targetFolder, finalName)
	counter := 2
	for {
		if _, statErr := os.Stat(targetPath); statErr != nil {
			if os.IsNotExist(statErr) {
				break
			}
			return "", "", fmt.Errorf("failed to check target path: %w", statErr)
		}
		ext := filepath.Ext(newName)
		base := newName[:len(newName)-len(ext)]
		finalName = fmt.Sprintf("%s_%d%s", base, counter, ext)
		targetPath = filepath.Join(targetFolder, finalName)
		counter++
	}
	return finalName, targetPath, nil
}

// MoveAndRename moves a file to the target location with a new name,
// handling collisions with _2, _3, … suffixes. If the source cannot be
// removed after a fallback copy (e.g. it is locked by another program),
// the operation still counts as successful — the file is already at its
// destination.
func (sm *StorageManager) MoveAndRename(sourcePath, targetFolder, newName string) (string, error) {
	finalName, targetPath, err := prepareTarget(targetFolder, newName)
	if err != nil {
		return "", err
	}
	if err := os.Rename(sourcePath, targetPath); err != nil {
		// Rename failed (cross-device, or source locked): copy instead.
		if err := copyFile(sourcePath, targetPath); err != nil {
			return "", fmt.Errorf("failed to copy file: %w", err)
		}
		// Best-effort source removal; a locked source must not fail the op.
		_ = os.Remove(sourcePath)
	}
	return finalName, nil
}

// CopyAndRename copies a file to the target location with a new name,
// leaving the source file untouched. Collisions get _2, _3, … suffixes.
func (sm *StorageManager) CopyAndRename(sourcePath, targetFolder, newName string) (string, error) {
	finalName, targetPath, err := prepareTarget(targetFolder, newName)
	if err != nil {
		return "", err
	}
	if err := copyFile(sourcePath, targetPath); err != nil {
		return "", fmt.Errorf("failed to copy file: %w", err)
	}
	return finalName, nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// FileExists checks if a file exists.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ListAllCSVPaths returns all invoices.csv files under the storage root.
func (sm *StorageManager) ListAllCSVPaths() ([]string, error) {
	root := sm.settings.StorageRoot
	if root == "" {
		return []string{}, nil
	}

	if _, err := os.Stat(root); os.IsNotExist(err) {
		return []string{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to access storage root: %w", err)
	}

	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if strings.EqualFold(d.Name(), "invoices.csv") {
			paths = append(paths, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan storage root: %w", err)
	}

	return paths, nil
}

// monthFolderPattern matches a bare YYYY-MM folder name.
var monthFolderPattern = regexp.MustCompile(`^(\d{4})-(\d{2})$`)

// MigrateToYearFolders moves bare YYYY-MM folders directly under the
// storage root into a YYYY year folder. Idempotent: a second run finds
// nothing to move. warn is called with a message for each skipped folder.
func (sm *StorageManager) MigrateToYearFolders(warn func(string)) error {
	if !sm.settings.UseMonthSubfolders {
		return nil
	}
	root := sm.settings.StorageRoot
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read storage root: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m := monthFolderPattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		yearDir := filepath.Join(root, m[1])
		if err := os.MkdirAll(yearDir, 0755); err != nil {
			return fmt.Errorf("failed to create year folder %s: %w", yearDir, err)
		}
		target := filepath.Join(yearDir, e.Name())
		if _, err := os.Stat(target); err == nil {
			warn(fmt.Sprintf("Monatsordner %s übersprungen — Ziel existiert bereits", e.Name()))
			continue
		}
		if err := os.Rename(filepath.Join(root, e.Name()), target); err != nil {
			return fmt.Errorf("failed to move %s: %w", e.Name(), err)
		}
	}
	return nil
}

// MigrateCashToBar moves already-filed invoices that are booked to a cash
// account into a Bar/ subfolder and sets their Unterordner. cashAccounts
// is the set of cash-account names. Idempotent: rows with a non-empty
// Unterordner are skipped. warn is called for each unmovable file.
func (sm *StorageManager) MigrateCashToBar(repo *CSVRepository, cashAccounts map[string]struct{}, warn func(string)) error {
	if !sm.settings.UseMonthSubfolders || len(cashAccounts) == 0 {
		return nil
	}
	csvPaths, err := sm.ListAllCSVPaths()
	if err != nil {
		return err
	}
	for _, csvPath := range csvPaths {
		monthFolder := filepath.Dir(csvPath)
		rows, err := repo.Load(csvPath)
		if err != nil {
			warn(fmt.Sprintf("CSV %s übersprungen: %v", csvPath, err))
			continue
		}
		// Is there anything to move in this folder?
		needsMove := false
		for i := range rows {
			if rows[i].Unterordner != "" {
				continue
			}
			if _, isCash := cashAccounts[rows[i].Bankkonto]; isCash {
				needsMove = true
				break
			}
		}
		if !needsMove {
			continue
		}
		// Probe: confirm the CSV is writable BEFORE moving any file, so a
		// write failure cannot leave files relocated but the CSV stale.
		if err := repo.Rewrite(csvPath, rows); err != nil {
			warn(fmt.Sprintf("CSV %s nicht schreibbar — übersprungen: %v", csvPath, err))
			continue
		}
		barDir := filepath.Join(monthFolder, "Bar")
		for i := range rows {
			if rows[i].Unterordner != "" {
				continue
			}
			if _, isCash := cashAccounts[rows[i].Bankkonto]; !isCash {
				continue
			}
			src := filepath.Join(monthFolder, rows[i].Dateiname)
			if _, err := os.Stat(src); err != nil {
				warn(fmt.Sprintf("Beleg %s nicht gefunden — übersprungen", src))
				continue
			}
			finalName, err := sm.MoveAndRename(src, barDir, rows[i].Dateiname)
			if err != nil {
				warn(fmt.Sprintf("Verschieben von %s fehlgeschlagen: %v", src, err))
				continue
			}
			rows[i].Dateiname = finalName
			rows[i].Unterordner = "Bar"
		}
		if err := repo.Rewrite(csvPath, rows); err != nil {
			return fmt.Errorf("failed to rewrite %s: %w", csvPath, err)
		}
	}
	return nil
}

// AttachmentPathsIn returns the file paths of an invoice's numbered
// "<base>_Anhang<N>.<ext>" attachments located in folder, ordered by index.
// The numbered siblings live next to the invoice's main file — there is no
// separate attachments folder.
func AttachmentPathsIn(folder, mainName string) []string {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil
	}
	type indexed struct {
		idx  int
		path string
	}
	var found []indexed
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if n, ok := ParseAttachmentName(e.Name(), mainName); ok {
			found = append(found, indexed{n, filepath.Join(folder, e.Name())})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].idx < found[j].idx })
	paths := make([]string, len(found))
	for i, f := range found {
		paths[i] = f.path
	}
	return paths
}

// CountAttachmentsIn returns how many numbered attachments the invoice
// mainName has in folder.
func CountAttachmentsIn(folder, mainName string) int {
	return len(AttachmentPathsIn(folder, mainName))
}

// MoveInvoiceAttachments moves the numbered "_Anhang<N>" sibling files of an
// invoice when its main file is renamed to newName and/or moved to newFolder.
// Each attachment keeps its index and extension but adopts the invoice's new
// base name, so it stays associated. Best-effort per file (copy fallback if
// rename fails, e.g. across volumes or with a locked source).
func (sm *StorageManager) MoveInvoiceAttachments(oldFolder, oldName, newFolder, newName string) error {
	oldBase := ReplaceExtension(oldName, "")
	newBase := ReplaceExtension(newName, "")
	if oldFolder == newFolder && oldBase == newBase {
		return nil
	}
	for _, src := range AttachmentPathsIn(oldFolder, oldName) {
		suffix := filepath.Base(src)[len(oldBase):] // "_Anhang<N>.<ext>"
		target := filepath.Join(newFolder, newBase+suffix)
		if err := os.Rename(src, target); err != nil {
			if cerr := copyFile(src, target); cerr != nil {
				return fmt.Errorf("failed to move attachment %s: %w", filepath.Base(src), cerr)
			}
			_ = os.Remove(src)
		}
	}
	return nil
}
