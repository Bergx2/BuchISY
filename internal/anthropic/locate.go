package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
)

// LocatedBox is a normalized (0..1, top-left origin) bounding box on a page.
type LocatedBox struct {
	Found      bool
	Page       int
	X0, Y0, X1, Y1 float64
}

const locateSystemPrompt = `Du erhältst ein oder mehrere Seitenbilder eines Belegs (Seite 0, Seite 1, …).
Deine Aufgabe: Finde den angegebenen Geldbetrag auf einer der Seiten und liefere seine Bounding-Box.

Antworte AUSSCHLIESSLICH mit einem JSON-Objekt — KEINE Prosa, KEINE Erklärungen, KEIN Markdown:

Wenn der Betrag gefunden wurde:
{"found":true,"page":<0-basierter Seitenindex>,"box":[x0,y0,x1,y1]}

x0,y0 = obere linke Ecke; x1,y1 = untere rechte Ecke.
Alle Koordinaten sind normalisiert (0.0 = linker/oberer Rand, 1.0 = rechter/unterer Rand der Seite).
Die Box soll den GESAMTEN Betrag vollständig umschließen — alle Ziffern, Tausenderpunkte, Komma und ggf. Währungssymbol — mit etwas Rand. Lieber etwas zu groß als zu klein; der Betrag darf NICHT halb außerhalb liegen.

Wenn der Betrag NICHT gefunden wurde:
{"found":false}

Keine weiteren Felder. Nur dieses JSON-Objekt.`

// LocateValue asks Claude Vision to find the bounding box of a money amount
// on one of the provided page images. imagesBase64 are base64-encoded page
// renders, mediaType is e.g. "image/png". The returned LocatedBox uses
// normalized coordinates (0..1, top-left origin).
func (e *Extractor) LocateValue(ctx context.Context, apiKey, model string, imagesBase64 []string, mediaType, value string) (LocatedBox, error) {
	userMessage := "Finde den Betrag " + value

	if e.debug && e.logger != nil {
		e.logger.Debug("=== CLAUDE LOCATE VALUE REQUEST (%d pages, value=%q) ===", len(imagesBase64), value)
	}

	response, err := e.client.SendWithImages(ctx, apiKey, model, locateSystemPrompt, userMessage, imagesBase64, mediaType)
	if err != nil {
		return LocatedBox{}, fmt.Errorf("LocateValue API request failed: %w", err)
	}

	if e.debug && e.logger != nil {
		e.logger.Debug("=== CLAUDE LOCATE VALUE RESPONSE ===\n%s", response)
	}

	box, err := parseLocateJSON(response)
	if err != nil {
		return LocatedBox{}, fmt.Errorf("LocateValue parse failed: %w", err)
	}
	return box, nil
}

// parseLocateJSON parses Claude's JSON reply into a LocatedBox.
// It strips markdown fences (like cleanJSONResponse does for other extractors),
// clamps coordinates to [0,1], and returns Found=false for degenerate boxes.
// Exported for unit-testability.
func parseLocateJSON(s string) (LocatedBox, error) {
	// Strip markdown fences the same way cleanJSONResponse does.
	s = cleanJSONResponse(s)

	var raw struct {
		Found bool        `json:"found"`
		Page  int         `json:"page"`
		Box   [4]float64  `json:"box"`
	}
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return LocatedBox{}, fmt.Errorf("parseLocateJSON: %w", err)
	}

	if !raw.Found {
		return LocatedBox{Found: false}, nil
	}

	// Clamp coords to [0,1].
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return v
	}
	x0 := clamp(raw.Box[0])
	y0 := clamp(raw.Box[1])
	x1 := clamp(raw.Box[2])
	y1 := clamp(raw.Box[3])

	// Degenerate box → treat as not found.
	if x1 <= x0 || y1 <= y0 {
		return LocatedBox{Found: false}, nil
	}

	return LocatedBox{
		Found: true,
		Page:  raw.Page,
		X0:    x0,
		Y0:    y0,
		X1:    x1,
		Y1:    y1,
	}, nil
}
