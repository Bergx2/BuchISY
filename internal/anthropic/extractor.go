package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bergx2/buchisy/internal/core"
	"github.com/bergx2/buchisy/internal/logging"
)

const systemPrompt = `Du bist ein sorgfältiger Daten-Extractor für deutsche Rechnungen (und simple englische/US-Invoices). Du erhältst reinen Text, extrahiert aus einer PDF-Rechnung.

Ziel: Liefere ausschließlich ein strenges JSON-Objekt mit genau diesen Schlüsseln (deutsche Bezeichner, snake-case):

{
  "auftraggeber": "string oder null",
  "verwendungszweck": "string oder null",
  "rechnungsnummer": "string oder null",
  "betragnetto": 0.0,
  "steuersatz_prozent": 0.0,
  "steuersatz_betrag": 0.0,
  "bruttobetrag": 0.0,
  "waehrung": "EUR|USD|andere ISO4217 oder null",
  "rechnungsdatum": "dd.MM.yyyy oder null",
  "jahr": "YYYY oder null",
  "monat": "MM oder null",
  "ustidnr": "string oder null"
}

Regeln:

- Antworte nur mit JSON, ohne Prosa.
- Wenn unsicher: Feld auf null setzen, nicht raten.
- auftraggeber: Verwende den Aussteller (Vendor), nicht den Rechnungsempfänger.
- rechnungsdatum: Bevorzuge das Feld nahe "Rechnung/Rechnungsdatum/Datum". Normalisiere nach dd.MM.yyyy (deutsches Format).
- betragnetto / steuersatz_prozent / steuersatz_betrag / bruttobetrag:
  - bruttobetrag = Gesamtbetrag / Total / Rechnungsbetrag.
  - Nutze Punkt als Dezimaltrennzeichen in JSON (56.23) und entferne Tausendertrennzeichen.
  - Wenn möglich, konsistenzprüfen: netto + steuer ≈ brutto (kleine Abweichung zulässig).
- waehrung: Verwende ISO-Code (z. B. EUR, USD). "€" ⇒ EUR.
- jahr / monat: aus rechnungsdatum ableiten (YYYY, MM).
- verwendungszweck: kurze menschliche Zusammenfassung (max. ~80 Zeichen), z. B. "Cloud-Abo Oktober 2025".
- ustidnr: Die Umsatzsteuer-Identifikationsnummer des Rechnungsausstellers (Format: 2 Buchstaben Ländercode + 8-12 Ziffern, z.B. "DE123456789"). Falls nicht vorhanden: null.

Ausgabe: Nur das JSON-Objekt.`

// Extractor extracts invoice metadata using Claude API.
type Extractor struct {
	client *Client
	logger *logging.Logger
	debug  bool
}

// NewExtractor creates a new Anthropic extractor.
func NewExtractor(logger *logging.Logger, debug bool) *Extractor {
	return &Extractor{
		client: NewClient(),
		logger: logger,
		debug:  debug,
	}
}

// SetDebug enables or disables debug logging.
func (e *Extractor) SetDebug(debug bool) {
	e.debug = debug
}

// ExtractFromImage extracts invoice metadata from a PDF image using Claude Vision API.
func (e *Extractor) ExtractFromImage(ctx context.Context, apiKey, model, imageBase64, mediaType string) (core.Meta, float64, error) {
	// Simplified prompt for vision - Claude can see the invoice directly
	visionPrompt := "Bitte extrahiere die Rechnungsinformationen aus diesem Dokument."

	// Debug logging: log request
	if e.debug && e.logger != nil {
		e.logger.Debug("=== CLAUDE VISION API REQUEST ===")
		e.logger.Debug("Model: %s", model)
		e.logger.Debug("Image size: %d bytes (base64)", len(imageBase64))
		e.logger.Debug("Media type: %s", mediaType)
	}

	// Send request with image
	response, err := e.client.SendWithImage(ctx, apiKey, model, systemPrompt, visionPrompt, imageBase64, mediaType)
	if err != nil {
		if e.debug && e.logger != nil {
			e.logger.Debug("=== CLAUDE VISION API ERROR ===")
			e.logger.Debug("Error: %v", err)
		}
		return core.Meta{}, 0, fmt.Errorf("Vision API request failed: %w", err)
	}

	// Debug logging: log response
	if e.debug && e.logger != nil {
		e.logger.Debug("=== CLAUDE VISION API RESPONSE ===")
		e.logger.Debug("Response length: %d chars", len(response))
		e.logger.Debug("Full response: %s", response)
	}

	// Parse the JSON response (same as text extraction)
	meta, err := parseExtractionResponse(response)
	if err != nil {
		return core.Meta{}, 0, err
	}

	// Confidence is high for Claude Vision (assume 0.95 for vision)
	confidence := 0.95

	return meta, confidence, nil
}

// Extract extracts invoice metadata from text using Claude API.
func (e *Extractor) Extract(ctx context.Context, apiKey, model, text string) (core.Meta, float64, error) {
	// Limit text length to avoid token limits
	// Keep first 10000 chars, prioritizing invoice-relevant content
	text = preprocessText(text, 10000)

	// Debug logging: log request
	if e.debug && e.logger != nil {
		e.logger.Debug("=== CLAUDE API REQUEST ===")
		e.logger.Debug("Model: %s", model)
		e.logger.Debug("Text length: %d chars", len(text))
		e.logger.Debug("Text preview (first 500 chars): %s", truncate(text, 500))
		e.logger.Debug("System prompt length: %d chars", len(systemPrompt))
	}

	// Send request
	response, err := e.client.Send(ctx, apiKey, model, systemPrompt, text)
	if err != nil {
		if e.debug && e.logger != nil {
			e.logger.Debug("=== CLAUDE API ERROR ===")
			e.logger.Debug("Error: %v", err)
		}
		return core.Meta{}, 0, fmt.Errorf("API request failed: %w", err)
	}

	// Debug logging: log response
	if e.debug && e.logger != nil {
		e.logger.Debug("=== CLAUDE API RESPONSE ===")
		e.logger.Debug("Response length: %d chars", len(response))
		e.logger.Debug("Full response: %s", response)
	}

	// Parse the JSON response
	meta, err := parseExtractionResponse(response)
	if err != nil {
		return core.Meta{}, 0, err
	}

	// Confidence is high for Claude (assume 0.9 if we got a response)
	confidence := 0.9

	return meta, confidence, nil
}

// parseExtractionResponse parses the JSON response from Claude API.
func parseExtractionResponse(response string) (core.Meta, error) {
	// Clean response (remove any markdown code blocks if present)
	response = cleanJSONResponse(response)

	// Parse JSON response
	var result struct {
		Auftraggeber      *string  `json:"auftraggeber"`
		Verwendungszweck  *string  `json:"verwendungszweck"`
		Rechnungsnummer   *string  `json:"rechnungsnummer"`
		BetragNetto       *float64 `json:"betragnetto"`
		SteuersatzProzent *float64 `json:"steuersatz_prozent"`
		SteuersatzBetrag  *float64 `json:"steuersatz_betrag"`
		Bruttobetrag      *float64 `json:"bruttobetrag"`
		Waehrung          *string  `json:"waehrung"`
		Rechnungsdatum    *string  `json:"rechnungsdatum"`
		Jahr              *string  `json:"jahr"`
		Monat             *string  `json:"monat"`
		UStIdNr           *string  `json:"ustidnr"`
	}

	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return core.Meta{}, fmt.Errorf("failed to parse JSON response: %w (response: %s)", err, response)
	}

	// Convert to Meta
	meta := core.Meta{}

	if result.Auftraggeber != nil {
		meta.Auftraggeber = *result.Auftraggeber
	}
	if result.Verwendungszweck != nil {
		meta.Verwendungszweck = *result.Verwendungszweck
	}
	if result.Rechnungsnummer != nil {
		meta.Rechnungsnummer = *result.Rechnungsnummer
	}
	if result.BetragNetto != nil {
		meta.BetragNetto = *result.BetragNetto
	}
	if result.SteuersatzProzent != nil {
		meta.SteuersatzProzent = *result.SteuersatzProzent
	}
	if result.SteuersatzBetrag != nil {
		meta.SteuersatzBetrag = *result.SteuersatzBetrag
	}
	if result.Bruttobetrag != nil {
		meta.Bruttobetrag = *result.Bruttobetrag
	}
	if result.Waehrung != nil {
		meta.Waehrung = *result.Waehrung
	}
	if result.Rechnungsdatum != nil {
		meta.Rechnungsdatum = *result.Rechnungsdatum
	}
	if result.Jahr != nil {
		meta.Jahr = *result.Jahr
	}
	if result.Monat != nil {
		meta.Monat = *result.Monat
	}
	if result.UStIdNr != nil {
		meta.UStIdNr = *result.UStIdNr
	}

	return meta, nil
}

// preprocessText limits and prioritizes invoice-relevant content.
func preprocessText(text string, maxChars int) string {
	if len(text) <= maxChars {
		return text
	}

	// Find lines with invoice-relevant keywords
	keywords := []string{
		"rechnung", "invoice", "datum", "date", "netto", "net",
		"brutto", "gross", "mwst", "ust", "vat", "total", "gesamt",
		"rechnungsnr", "invoice no", "betrag", "amount", "eur", "€", "usd", "$",
	}

	lines := strings.Split(text, "\n")
	var priorityLines []string
	var otherLines []string

	for _, line := range lines {
		lowerLine := strings.ToLower(line)
		isPriority := false
		for _, kw := range keywords {
			if strings.Contains(lowerLine, kw) {
				isPriority = true
				break
			}
		}
		if isPriority {
			priorityLines = append(priorityLines, line)
		} else {
			otherLines = append(otherLines, line)
		}
	}

	// Combine priority lines first, then others until we hit the limit
	combined := strings.Join(priorityLines, "\n")
	if len(combined) < maxChars {
		remaining := maxChars - len(combined)
		others := strings.Join(otherLines, "\n")
		if len(others) <= remaining {
			combined += "\n" + others
		} else {
			combined += "\n" + others[:remaining]
		}
	}

	return combined[:min(len(combined), maxChars)]
}

// cleanJSONResponse removes markdown code blocks if present.
func cleanJSONResponse(response string) string {
	response = strings.TrimSpace(response)

	// Remove ```json ... ``` blocks
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
		response = strings.TrimSuffix(response, "```")
		response = strings.TrimSpace(response)
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
		response = strings.TrimSuffix(response, "```")
		response = strings.TrimSpace(response)
	}

	return response
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// truncate returns the first n characters of a string.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
