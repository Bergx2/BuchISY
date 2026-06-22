package core

import (
	"archive/zip"
	"io"
	"os"
)

// WriteBackupZip writes a ZIP to w containing each files[zipName]=sourcePath
// whose source is readable. Unreadable/missing sources are skipped. Returns the
// number of files written.
func WriteBackupZip(w io.Writer, files map[string]string) (int, error) {
	zw := zip.NewWriter(w)
	count := 0
	for name, path := range files {
		src, err := os.Open(path)
		if err != nil {
			continue // skip missing/unreadable
		}
		fw, err := zw.Create(name)
		if err != nil {
			_ = src.Close()
			_ = zw.Close()
			return count, err
		}
		if _, err := io.Copy(fw, src); err != nil {
			_ = src.Close()
			_ = zw.Close()
			return count, err
		}
		_ = src.Close()
		count++
	}
	if err := zw.Close(); err != nil {
		return count, err
	}
	return count, nil
}
