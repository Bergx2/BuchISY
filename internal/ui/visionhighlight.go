package ui

import (
	"context"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"github.com/zalando/go-keyring"

	"github.com/bergx2/buchisy/internal/core"
)

// visionHighlight triggers an async Claude Vision lookup when the text layer
// of a PDF does not already contain a highlight for value. On success it
// appends a highlight rectangle to strip via addHighlight (on the main thread).
//
// Call after building the strip; skips silently when:
//   - strip is nil
//   - value is empty
//   - no API key is configured
//   - the text layer already provided ≥1 matching rect (HighlightRects hit)
func (a *App) visionHighlight(strip *pdfPreviewStrip, path, value string, hl previewHighlight) {
	if strip == nil || value == "" || !a.hasAPIKey() {
		return
	}
	if !core.IsPDF(path) {
		return
	}

	// Check whether the text layer already covers this value. Include the
	// comma-variant so e.g. "1234,56" is treated the same as "1234.56".
	commaVariant := strings.ReplaceAll(value, ".", ",")
	existing, _ := core.HighlightRects(path, []string{value, commaVariant}, previewDPI)
	for _, rects := range existing {
		if len(rects) > 0 {
			return // text layer found it — no vision needed
		}
	}

	// Derive the profile config dir to locate the cache file.
	configDir, err := core.GetProfileConfigDir(a.profile)
	if err != nil {
		a.logger.Warn("visionHighlight: cannot derive config dir: %v", err)
		return
	}
	cachePath := filepath.Join(configDir, "locate_cache.json")

	// Use the base filename as the cache key rel-path so it is stable across
	// storage-root changes and platform path differences.
	relpath := filepath.Base(path)
	cacheKey := core.LocateCacheKey(relpath, value)

	go func() {
		// Load (or create) the cache.
		cache, err := core.LoadLocateCache(cachePath)
		if err != nil {
			a.logger.Warn("visionHighlight: load cache: %v", err)
			cache = make(core.LocateCache)
		}

		// Cache hit — use the stored result.
		if entry, ok := cache[cacheKey]; ok {
			if !entry.Found {
				return // previously queried, not found — skip
			}
			applyVisionBox(strip, entry.Page, entry.X0, entry.Y0, entry.X1, entry.Y1, hl)
			return
		}

		// Render all pages to base64 for the Vision API.
		imgs, mt, err := core.PDFAllPagesToBase64(path)
		if err != nil {
			a.logger.Warn("visionHighlight: PDFAllPagesToBase64: %v", err)
			return
		}

		// Fetch the API key from the keyring.
		apiKey, err := keyring.Get("BuchISY", a.keyringAccount())
		if err != nil || apiKey == "" {
			return
		}

		model := a.settings.AnthropicModel
		if model == "" {
			model = "claude-opus-4-5"
		}

		box, err := a.anthropicExtractor.LocateValue(context.Background(), apiKey, model, imgs, mt, value)
		if err != nil {
			a.logger.Warn("visionHighlight: LocateValue: %v", err)
			return
		}

		// Store result (including negative) so the receipt is never re-queried.
		cache[cacheKey] = core.LocateCacheEntry{
			Found: box.Found,
			Page:  box.Page,
			X0:    box.X0,
			Y0:    box.Y0,
			X1:    box.X1,
			Y1:    box.Y1,
		}
		if err := core.SaveLocateCache(cachePath, cache); err != nil {
			a.logger.Warn("visionHighlight: SaveLocateCache: %v", err)
		}

		if !box.Found {
			return
		}
		applyVisionBox(strip, box.Page, box.X0, box.Y0, box.X1, box.Y1, hl)
	}()
}

// applyVisionBox converts a normalized Vision box to pixel coords and
// appends it to the strip on the main thread.
func applyVisionBox(strip *pdfPreviewStrip, page int, x0, y0, x1, y1 float64, hl previewHighlight) {
	if page < 0 || page >= len(strip.pageNative) {
		return
	}
	native := strip.pageNative[page]
	W := native.Width
	H := native.Height

	// Slight padding (2 % of page dimension) so the highlight is easy to spot.
	padX := W * 0.02
	padY := H * 0.01

	rc := core.Rect{
		X: float32(x0)*W - padX,
		Y: float32(y0)*H - padY,
		W: float32(x1-x0)*W + 2*padX,
		H: float32(y1-y0)*H + 2*padY,
	}

	fyne.Do(func() {
		strip.addHighlight(page, rc, hl)
	})
}
