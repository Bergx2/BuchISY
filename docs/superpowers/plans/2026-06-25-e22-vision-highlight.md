# E22 — Vision/OCR-Highlight für Scan-Belege

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** When a preview PDF has NO text layer (scanned receipt → `HighlightRects` finds nothing), locate the booking amount with Claude Vision, cache the box, and draw the highlight overlay — so scanned receipts also show the amount marked.

**Architecture:** Reuse the existing Claude Vision pipeline (`client.SendWithImages`) + page rendering (`core.PDFAllPagesToBase64`). Async at preview time with a per-profile cache; UI updated via `fyne.Do`.

**Tech:** Go 1.25, Fyne. Branch `feat/e22-vision-highlight`.

## Global Constraints
- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer.
- Normalized box coords are top-left origin, 0..1. `core.Rect{X,Y,W,H float32}` is in IMAGE PIXELS.
- Only act when `a.hasAPIKey()` is true. Cache "not found" too, so a miss is never re-queried.
- Existing helpers: `core.PDFAllPagesToBase64(path)([]string,string,error)`; `(*anthropic.Client).SendWithImages(ctx,apiKey,model,systemPrompt,userMessage,imagesBase64,mediaType)(string,error)`; `(*anthropic.Extractor)` holds an unexported `client`. The strip is `pdfPreviewStrip` (fields `rects [][]core.Rect`, `rectObjs [][]*canvas.Rectangle`, `pageNative []fyne.Size`); `previewHighlight{fill,stroke,strokeWidth,fullWidth,values}` + presets `hlYellowFill`/`hlGreenFrame`. Per-profile JSON stores mirror `internal/core/companymap.go` (LoadX/SaveX(path)).

---

### Task 1: anthropic LocateValue (vision → normalized box)

**Files:** `internal/anthropic/locate.go` (new), `internal/anthropic/locate_test.go` (new).

**Interface:**
```go
// LocatedBox is a normalized (0..1, top-left origin) bounding box on a page.
type LocatedBox struct { Found bool; Page int; X0, Y0, X1, Y1 float64 }
func (e *Extractor) LocateValue(ctx context.Context, apiKey, model string, imagesBase64 []string, mediaType, value string) (LocatedBox, error)
```
- Build a vision system prompt: the model receives page image(s) and must find the money amount `value`. Return ONLY JSON `{"found":true,"page":<0-based int>,"box":[x0,y0,x1,y1]}` (normalized 0..1, top-left origin, tightly around the amount) or `{"found":false}`.
- Call `e.client.SendWithImages(ctx, apiKey, model, systemPrompt, "Finde den Betrag "+value, imagesBase64, mediaType)`.
- Parse the JSON (strip any markdown fences first, like the other extractors do) into LocatedBox. Clamp coords to [0,1]; if x1<=x0 or y1<=y0 → Found=false.

- [ ] **Step 1: test** `internal/anthropic/locate_test.go`: a pure helper `parseLocateJSON(s string)(LocatedBox,error)` (extract it so it's testable without the API). Cases: `{"found":true,"page":1,"box":[0.1,0.2,0.3,0.25]}` → Found, Page 1, coords; fenced ```json … ``` variant; `{"found":false}` → Found=false; degenerate box (x1<=x0) → Found=false; garbage → error. Run → fail.
- [ ] **Step 2:** implement `parseLocateJSON` + `LocateValue`. Run → pass.
- [ ] **Step 3:** `go build ./... && go test ./internal/anthropic/`. Commit `E22: anthropic LocateValue (vision amount localization)`.

---

### Task 2: core locate cache (sidecar JSON)

**Files:** `internal/core/locatecache.go` (new), `internal/core/locatecache_test.go` (new). Mirror `companymap.go`.

**Interface:**
```go
type LocateCacheEntry struct { Found bool; Page int; X0, Y0, X1, Y1 float64 }
type LocateCache map[string]LocateCacheEntry // key = "<relpath>|<value>"
func LocateCacheKey(relpath, value string) string
func LoadLocateCache(path string) (LocateCache, error) // missing file → empty map, no error
func SaveLocateCache(path string, c LocateCache) error
```
- [ ] **Step 1: test:** Save a cache with one entry to a temp path, Load it back, assert equality; Load of a non-existent path → empty map + nil error; `LocateCacheKey("2026/x.pdf","573.15")` stable. Run → fail.
- [ ] **Step 2:** implement (JSON marshal/indent, `os.WriteFile`, `os.ReadFile`; `os.IsNotExist` → empty). Run → pass + full core.
- [ ] **Step 3:** Commit `E22: core LocateCache (sidecar for vision boxes)`.

---

### Task 3: UI — async vision highlight fallback

**Files:** `internal/ui/documentpreview.go` (strip method), `internal/ui/visionhighlight.go` (new), `internal/ui/tableedit.go` (wire), `internal/ui/app.go` (profile path helper if needed).

**Context:** READ `tableedit.go` `swapPreview` (builds `strip` via `renderPreviewContent(path, meta, hl)`; `statementPreviewPath`, `row.Bruttobetrag` in scope) and how the app gets the API key (`keyring.Get("BuchISY", a.keyringAccount())`, `a.settings.AnthropicModel`) and the profile dir (mirror an existing per-profile JSON path helper, e.g. company map).

- [ ] **Step 1:** Add `func (s *pdfPreviewStrip) addHighlight(page int, rc core.Rect, hl previewHighlight)` — bounds-check `page`; append `rc` to `s.rects[page]`, create a `*canvas.Rectangle` styled from `hl` (same fill/stroke logic as `newPdfPreviewStrip`), append to `s.rectObjs[page]`; then `s.Refresh()`. (Used from the main thread only.)
- [ ] **Step 2:** `internal/ui/visionhighlight.go`: `func (a *App) visionHighlight(strip *pdfPreviewStrip, path, value string, hl previewHighlight)`:
  1. If `!a.hasAPIKey()` or `value==""` or strip nil → return.
  2. If `core.HighlightRects(path, []string{value, strings.ReplaceAll(value, ".", ",")}, previewDPI)` already yields ≥1 rect → text layer covered it, return (no vision needed).
  3. `go func(){ ... }()`: load `LocateCache` (profile path); key = `LocateCacheKey(rel(path), value)`. If hit: use it. Else: `imgs, mt, err := core.PDFAllPagesToBase64(path)`; `apiKey := keyring.Get(...)`; `box, err := a.anthropicExtractor.LocateValue(ctx, apiKey, model, imgs, mt, value)`; store result (found or not) in cache + `SaveLocateCache`. If `!box.Found` → return.
  4. Convert normalized box → pixel `core.Rect` using `strip.pageNative[box.Page]` (W=native.Width, H=native.Height): `rc := core.Rect{X: float32(box.X0)*W, Y: float32(box.Y0)*H, W: float32(box.X1-box.X0)*W, H: float32(box.Y1-box.Y0)*H}`. Pad slightly.
  5. `fyne.Do(func(){ strip.addHighlight(box.Page, rc, hl) })`.
  Guard against `box.Page` out of range of `strip.pageNative`.
- [ ] **Step 3:** In `tableedit.go` `swapPreview`, after `previewStrip = strip` (strip built), call `a.visionHighlight(strip, path, fmt.Sprintf("%.2f", row.Bruttobetrag), hl)` — so a scanned receipt gets its amount marked (yellow for the receipt, green frame when it's the statement). Use the same `hl` already chosen in that closure.
- [ ] **Step 4:** `go build ./... && go test ./...`. Commit `E22: async Claude-Vision highlight fallback for scanned PDFs`.

## Self-Review
Vision localization (Task 1) + cache (Task 2) + async UI overlay (Task 3). Only fires when there is no text layer and an API key exists; cached (incl. negatives) so each receipt is queried at most once. Coordinates are approximate (Claude Vision) — acceptable for a "find roughly here" marker.
