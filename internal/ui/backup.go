package ui

import (
	"bytes"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"github.com/bergx2/buchisy/internal/core"
	"github.com/bergx2/buchisy/internal/db"
)

// showBackup collects the app's data (database, profile config JSONs, and the
// month CSVs under the storage root) into a ZIP and asks where to save it.
func (a *App) showBackup() {
	configDir, err := core.GetProfileConfigDir(a.profile)
	if err != nil {
		a.showError(a.bundle.T("error.processing.title"), err.Error())
		return
	}
	files := map[string]string{}
	files["invoices.db"] = db.GetGlobalDBPath(configDir)
	for _, name := range []string{"settings.json", "chart_skr04.json", "buchungsregeln.json", "booking_templates.json", "company_accounts.json"} {
		files["config/"+name] = filepath.Join(configDir, name)
	}
	// All invoices.csv under the storage root, keyed by their relative path.
	root := a.settings.StorageRoot
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() || d.Name() != "invoices.csv" {
			return nil
		}
		if rel, rerr := filepath.Rel(root, path); rerr == nil {
			files["csv/"+filepath.ToSlash(rel)] = path
		}
		return nil
	})

	var buf bytes.Buffer
	n, err := core.WriteBackupZip(&buf, files)
	if err != nil {
		a.showError(a.bundle.T("error.processing.title"), err.Error())
		return
	}
	d := dialog.NewFileSave(func(wc fyne.URIWriteCloser, ferr error) {
		if wc == nil {
			return
		}
		defer wc.Close()
		if ferr != nil {
			a.showError(a.bundle.T("error.processing.title"), ferr.Error())
			return
		}
		if _, werr := wc.Write(buf.Bytes()); werr != nil {
			a.showError(a.bundle.T("error.processing.title"), werr.Error())
			return
		}
		a.showInfo(a.bundle.T("backup.title"), a.bundle.T("backup.done", n))
	}, a.window)
	d.SetFileName("BuchISY-Backup.zip")
	d.Show()
}
