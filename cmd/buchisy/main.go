package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bergx2/buchisy/internal/ui"
)

func main() {
	// Determine assets directory
	// In development: relative to working directory
	// In production: bundled with the application
	assetsDir := getAssetsDir()

	// Create and run the application
	app, err := ui.New(assetsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize application: %v\n", err)
		os.Exit(1)
	}

	app.Run()
}

// getAssetsDir returns the path to the assets directory.
func getAssetsDir() string {
	// Try current working directory first (development)
	if _, err := os.Stat("assets"); err == nil {
		return "assets"
	}

	// Get executable path
	execPath, err := os.Executable()
	if err != nil {
		return "."
	}

	execDir := filepath.Dir(execPath)

	// Check for macOS .app bundle structure
	// executable is at: BuchISY.app/Contents/MacOS/buchisy
	// assets should be at: BuchISY.app/Contents/Resources/assets
	if filepath.Base(execDir) == "MacOS" {
		contentsDir := filepath.Dir(execDir)
		if filepath.Base(contentsDir) == "Contents" {
			// We're in a .app bundle
			resourcesPath := filepath.Join(contentsDir, "Resources", "assets")
			if _, err := os.Stat(resourcesPath); err == nil {
				return resourcesPath
			}
		}
	}

	// Try relative to executable (production, non-bundle)
	assetsPath := filepath.Join(execDir, "assets")
	if _, err := os.Stat(assetsPath); err == nil {
		return assetsPath
	}

	// Fallback to current directory
	return "."
}
