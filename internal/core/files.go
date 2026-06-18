package core

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// supportedExtensions is the set of file extensions BuchISY accepts as an
// invoice main file or as an attachment (lower-case, leading dot).
var supportedExtensions = map[string]struct{}{
	".pdf": {},
	".doc": {}, ".docx": {},
	".xls": {}, ".xlsx": {},
	".ppt": {}, ".pptx": {},
	".odt": {}, ".ods": {}, ".odp": {},
	".jpg": {}, ".jpeg": {}, ".png": {}, ".gif": {},
	".bmp": {}, ".tif": {}, ".tiff": {}, ".webp": {}, ".heic": {}, ".svg": {},
}

// IsSupportedFile reports whether the file name has an extension BuchISY
// accepts as an invoice main file or attachment.
func IsSupportedFile(name string) bool {
	_, ok := supportedExtensions[strings.ToLower(filepath.Ext(name))]
	return ok
}

// IsPDF reports whether the file name is a PDF.
func IsPDF(name string) bool {
	return strings.ToLower(filepath.Ext(name)) == ".pdf"
}

// ImageMediaType returns the Claude-Vision-compatible media type for an
// image file based on its extension. Returns "" for non-image / non-
// vision-supported files. Used to decide whether to route a non-PDF
// submission through the Claude image-vision extractor.
func ImageMediaType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	}
	return ""
}

// ReplaceExtension returns name with its extension replaced by newExt.
// newExt must include the leading dot (or be empty to strip the extension).
func ReplaceExtension(name, newExt string) string {
	ext := filepath.Ext(name)
	return name[:len(name)-len(ext)] + newExt
}

// AttachmentName builds the stored name for the index-th attachment (1-based)
// of an invoice whose final file name is mainName. attachmentExt is the
// attachment's own extension, including the leading dot.
func AttachmentName(mainName string, index int, attachmentExt string) string {
	return fmt.Sprintf("%s_Anhang%d%s", ReplaceExtension(mainName, ""), index, attachmentExt)
}

// ParseAttachmentName reports whether name is a numbered attachment of the
// invoice whose main file is mainName — i.e. "<base>_Anhang<N>.<ext>" — and
// returns the 1-based index N. Returns (0, false) for anything else.
func ParseAttachmentName(name, mainName string) (int, bool) {
	prefix := ReplaceExtension(mainName, "") + "_Anhang"
	if !strings.HasPrefix(name, prefix) {
		return 0, false
	}
	rest := name[len(prefix):] // "<N>.<ext>" or "<N>"
	if dot := strings.IndexByte(rest, '.'); dot >= 0 {
		rest = rest[:dot]
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
