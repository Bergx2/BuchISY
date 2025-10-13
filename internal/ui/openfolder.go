package ui

import (
	"os/exec"
	"runtime"
)

// openFolderInOS opens a folder in the system file manager.
func (a *App) openFolderInOS(path string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin": // macOS
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("explorer", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	default:
		a.showError(
			a.bundle.T("error.processing.title"),
			"Unsupported operating system",
		)
		return
	}

	if err := cmd.Start(); err != nil {
		a.logger.Error("Failed to open folder: %v", err)
		a.showError(
			a.bundle.T("error.processing.title"),
			"Failed to open folder: "+err.Error(),
		)
	} else {
		a.logger.Info("Opened folder: %s", path)
	}
}
