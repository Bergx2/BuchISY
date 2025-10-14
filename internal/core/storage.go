package core

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

	folderName := fmt.Sprintf("%04d-%02d", year, month)
	return filepath.Join(sm.settings.StorageRoot, folderName)
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

// MoveAndRename moves a file to the target location with a new name.
// It handles collisions by appending _2, _3, etc.
func (sm *StorageManager) MoveAndRename(sourcePath, targetFolder, newName string) (string, error) {
	// Ensure target folder exists
	if err := os.MkdirAll(targetFolder, 0755); err != nil {
		return "", fmt.Errorf("failed to create target folder: %w", err)
	}

	// Handle filename collisions
	finalName := newName
	targetPath := filepath.Join(targetFolder, finalName)
	counter := 2

	for {
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			break
		}

		// File exists, try with counter
		ext := filepath.Ext(newName)
		base := newName[:len(newName)-len(ext)]
		finalName = fmt.Sprintf("%s_%d%s", base, counter, ext)
		targetPath = filepath.Join(targetFolder, finalName)
		counter++
	}

	// Move the file
	if err := os.Rename(sourcePath, targetPath); err != nil {
		// If rename fails (e.g., cross-device), try copy + delete
		if err := copyFile(sourcePath, targetPath); err != nil {
			return "", fmt.Errorf("failed to copy file: %w", err)
		}
		if err := os.Remove(sourcePath); err != nil {
			return "", fmt.Errorf("failed to remove source file: %w", err)
		}
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

// GetAttachmentsFolderName returns the attachments folder name for a given invoice filename.
// e.g., "invoice.pdf" -> "invoice-files"
func GetAttachmentsFolderName(filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	return base + "-files"
}

// GetAttachmentsFolder returns the full path to the attachments folder for a given invoice.
func (sm *StorageManager) GetAttachmentsFolder(invoiceFilePath string) string {
	dir := filepath.Dir(invoiceFilePath)
	filename := filepath.Base(invoiceFilePath)
	folderName := GetAttachmentsFolderName(filename)
	return filepath.Join(dir, folderName)
}

// HasAttachments checks if an invoice has attachments.
func (sm *StorageManager) HasAttachments(invoiceFilePath string) bool {
	attachmentsFolder := sm.GetAttachmentsFolder(invoiceFilePath)
	info, err := os.Stat(attachmentsFolder)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// EnsureAttachmentsFolder creates the attachments folder if it doesn't exist.
func (sm *StorageManager) EnsureAttachmentsFolder(invoiceFilePath string) error {
	attachmentsFolder := sm.GetAttachmentsFolder(invoiceFilePath)
	return os.MkdirAll(attachmentsFolder, 0755)
}

// CopyFileToAttachments copies a file to the attachments folder, preserving the original filename.
// Returns the final filename (which may be different if there was a collision).
func (sm *StorageManager) CopyFileToAttachments(sourcePath, invoiceFilePath string) (string, error) {
	attachmentsFolder := sm.GetAttachmentsFolder(invoiceFilePath)

	// Ensure attachments folder exists
	if err := sm.EnsureAttachmentsFolder(invoiceFilePath); err != nil {
		return "", fmt.Errorf("failed to create attachments folder: %w", err)
	}

	// Get original filename
	filename := filepath.Base(sourcePath)
	targetPath := filepath.Join(attachmentsFolder, filename)

	// Handle filename collisions
	finalName := filename
	counter := 2
	for {
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			break
		}

		ext := filepath.Ext(filename)
		base := filename[:len(filename)-len(ext)]
		finalName = fmt.Sprintf("%s_%d%s", base, counter, ext)
		targetPath = filepath.Join(attachmentsFolder, finalName)
		counter++
	}

	// Copy the file
	if err := copyFile(sourcePath, targetPath); err != nil {
		return "", fmt.Errorf("failed to copy attachment: %w", err)
	}

	return finalName, nil
}

// ListAttachments returns a list of attachment filenames for an invoice.
func (sm *StorageManager) ListAttachments(invoiceFilePath string) ([]string, error) {
	attachmentsFolder := sm.GetAttachmentsFolder(invoiceFilePath)

	if !sm.HasAttachments(invoiceFilePath) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(attachmentsFolder)
	if err != nil {
		return nil, fmt.Errorf("failed to read attachments folder: %w", err)
	}

	var filenames []string
	for _, entry := range entries {
		if !entry.IsDir() {
			filenames = append(filenames, entry.Name())
		}
	}

	return filenames, nil
}
