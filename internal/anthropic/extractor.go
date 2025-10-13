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
  "firmenname": "string oder null",
  "kurzbezeichnung": "string oder null",
  "rechnungsnummer": "string oder null",
  "betragnetto": 0.0,
  "steuersatz_prozent": 0.0,
  "steuersatz_betrag": 0.0,
  "bruttobetrag": 0.0,
  "waehrung": "EUR|USD|andere ISO4217 oder null",
  "rechnungsdatum": "YYYY-MM-DD oder null",
  "datum_deutsch": "dd.MM.yyyy oder null",
  "jahr": "YYYY oder null",
  "monat": "MM oder null"
}

Regeln:

- Antworte nur mit JSON, ohne Prosa.
- Wenn unsicher: Feld auf null setzen, nicht raten.
- firmenname: Verwende den Aussteller (Vendor), nicht den Rechnungsempfänger.
- rechnungsdatum: Bevorzuge das Feld nahe "Rechnung/Rechnungsdatum/Datum". Normalisiere nach YYYY-MM-DD.
- datum_deutsch: gleiche Bedeutung wie rechnungsdatum, aber formatiert als dd.MM.yyyy.
- betragnetto / steuersatz_prozent / steuersatz_betrag / bruttobetrag:
  - bruttobetrag = Gesamtbetrag / Total / Rechnungsbetrag.
  - Nutze Punkt als Dezimaltrennzeichen in JSON (56.23) und entferne Tausendertrennzeichen.
  - Wenn möglich, konsistenzprüfen: netto + steuer ≈ brutto (kleine Abweichung zulässig).
- waehrung: Verwende ISO-Code (z. B. EUR, USD). "€" ⇒ EUR.
- jahr / monat: aus rechnungsdatum ableiten (YYYY, MM).
- kurzbezeichnung: kurze menschliche Zusammenfassung (max. ~80 Zeichen), z. B. "Cloud-Abo Oktober 2025".

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

	// Parse JSON response
	var result struct {
		Firmenname        *string  `json:"firmenname"`
		Kurzbezeichnung   *string  `json:"kurzbezeichnung"`
		Rechnungsnummer   *string  `json:"rechnungsnummer"`
		BetragNetto       *float64 `json:"betragnetto"`
		SteuersatzProzent *float64 `json:"steuersatz_prozent"`
		SteuersatzBetrag  *float64 `json:"steuersatz_betrag"`
		Bruttobetrag      *float64 `json:"bruttobetrag"`
		Waehrung          *string  `json:"waehrung"`
		Rechnungsdatum    *string  `json:"rechnungsdatum"`
		DatumDeutsch      *string  `json:"datum_deutsch"`
		Jahr              *string  `json:"jahr"`
		Monat             *string  `json:"monat"`
	}

	// Clean response (remove any markdown code blocks if present)
	response = cleanJSONResponse(response)

	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return core.Meta{}, 0, fmt.Errorf("failed to parse JSON response: %w (response: %s)", err, response)
	}

	// Convert to Meta
	meta := core.Meta{}

	if result.Firmenname != nil {
		meta.Firmenname = *result.Firmenname
	}
	if result.Kurzbezeichnung != nil {
		meta.Kurzbezeichnung = *result.Kurzbezeichnung
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
	if result.DatumDeutsch != nil {
		meta.DatumDeutsch = *result.DatumDeutsch
	}
	if result.Jahr != nil {
		meta.Jahr = *result.Jahr
	}
	if result.Monat != nil {
		meta.Monat = *result.Monat
	}

	// Confidence is high for Claude (assume 0.9 if we got a response)
	confidence := 0.9

	return meta, confidence, nil
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
