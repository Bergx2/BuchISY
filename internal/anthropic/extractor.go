package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bergx2/buchisy/internal/core"
	"github.com/bergx2/buchisy/internal/logging"
)

const systemPromptBase = `Du bist ein sorgfältiger Daten-Extractor für deutsche Rechnungen (und simple englische/US-Invoices). Du erhältst reinen Text, extrahiert aus einer PDF-Rechnung.

Ziel: Liefere ausschließlich ein strenges JSON-Objekt mit genau diesen Schlüsseln (deutsche Bezeichner, snake-case):

{
  "auftraggeber": "string oder null",
  "verwendungszweck": "string oder null",
  "rechnungsnummer": "string oder null",
  "vat_id": "USt-IdNr. des Rechnungsstellers oder null",
  "steuerzeilen": [{"satz": 0.0, "netto": 0.0, "mwst": 0.0}],
  "trinkgeld": 0.0,
  "bruttobetrag": 0.0,
  "waehrung": "EUR|USD|andere ISO4217 oder null",
  "rechnungsdatum": "dd.MM.yyyy oder null",
  "jahr": "YYYY oder null",
  "monat": "MM oder null",
  "ausgangsrechnung": false
}

Regeln:

- Antworte nur mit JSON, ohne Prosa.
- Wenn unsicher: Feld auf null setzen, nicht raten.
- auftraggeber: Verwende immer den GESCHÄFTSPARTNER, nicht den App-Nutzer selbst. Bei einer Eingangsrechnung ist das der Aussteller (Vendor/Lieferant). Bei einer Ausgangsrechnung (siehe ausgangsrechnung=true, der Nutzer ist Aussteller) ist es der RechnungsEMPFÄNGER (Kunde) — nicht der Aussteller.
- rechnungsnummer: Suche auf dem gesamten Beleg (auch Folgeseiten). Auch als "Belegnummer", "Beleg-Nr.", "Quittungsnummer", "Bon-Nr.", "Receipt No." oder "Transaktionsnummer" gekennzeichnete Nummern zählen als rechnungsnummer.
- rechnungsdatum: Bevorzuge das Feld nahe "Rechnung/Rechnungsdatum/Datum". Normalisiere nach dd.MM.yyyy (deutsches Format).
- steuerzeilen: je MwSt.-Satz eine Zeile (satz %, netto, mwst). Bei nur einem Satz genau eine Zeile. trinkgeld: separat ohne MwSt., 0 wenn keins.
- bruttobetrag = Gesamtbetrag / Total / Rechnungsbetrag.
  - Nutze Punkt als Dezimaltrennzeichen in JSON (56.23) und entferne Tausendertrennzeichen.
  - Wenn möglich, konsistenzprüfen: summe(netto) + summe(mwst) + trinkgeld ≈ brutto (kleine Abweichung zulässig).
- waehrung: Verwende ISO-Code (z. B. EUR, USD). "€" ⇒ EUR.
- jahr / monat: aus rechnungsdatum ableiten (YYYY, MM).
- verwendungszweck: kurze menschliche Zusammenfassung (max. ~80 Zeichen), z. B. "Cloud-Abo Oktober 2025".

vat_id (Umsatzsteuer-Identifikationsnummer des Rechnungsstellers):
- Beispiele für gültige Formate: "DE123456789", "ATU12345678", "FR12345678901", "GB123456789", "VAT-Nr.", "USt-IdNr.", "VAT-ID", "TAX-ID", "Tax Number".
- Bevorzuge das Feld, das explizit als "USt-IdNr." / "VAT" / "VAT-ID" / "Tax ID" der ausstellenden Firma gekennzeichnet ist.
- Wenn der Rechnungsaussteller eine Umsatzsteuer-Identifikationsnummer angibt, ist das die richtige.
- Steuernummer (z. B. "112/197/12644") ist NICHT die VAT-ID.

Ausgabe: Nur das JSON-Objekt.`

// systemPromptFor returns the base prompt with optional exclusions for
// the user's own VAT-IDs (so the extractor doesn't accidentally pick
// the receiver's number instead of the sender's).
func systemPromptFor(ownVATIDs []string) string {
	clean := cleanVATIDList(ownVATIDs)
	if len(clean) == 0 {
		return systemPromptBase
	}
	ausgangsBlock := "\n\n=== AUSGANGSRECHNUNG-ERKENNUNG ===\n" +
		"ausgangsrechnung = true, wenn die Rechnung vom App-Nutzer AUSGESTELLT wurde — d.h. wenn der Rechnungs-AUSSTELLER eine der folgenden eigenen USt-IdNrn trägt (Nutzer = Verkäufer/Aussteller):\n" +
		"  • " + strings.Join(clean, "\n  • ") + "\n" +
		"Bei einer normalen Eingangsrechnung (Nutzer = Empfänger) ist ausgangsrechnung = false. Wenn keine eigene USt-IdNr sichtbar ist, ausgangsrechnung = false.\n" +
		"=== ENDE AUSGANGSRECHNUNG-ERKENNUNG ==="
	verbotBlock := "\n\n=== STRENG VERBOTEN ===\n" +
		"Folgende VAT-IDs gehören dem App-Nutzer selbst und dürfen ABSOLUT NIEMALS als vat_id zurückgegeben werden — auch wenn sie auf der Rechnung stehen, auch wenn sie der ersten Firma zugeordnet sind, auch wenn die Rechnung eine Ausgangsrechnung des Nutzers ist:\n" +
		"  • " + strings.Join(clean, "\n  • ") + "\n\n" +
		"Wenn die einzige sichtbare VAT-ID einer dieser Werte ist, MUSS vat_id auf null gesetzt werden. Suche stattdessen nach der VAT-ID des Geschäftspartners (bei Eingangsrechnungen: Aussteller; bei Ausgangsrechnungen: Kunde). Wenn keine vorhanden, null.\n=== ENDE STRENG VERBOTEN ==="
	return systemPromptBase + ausgangsBlock + verbotBlock
}

// cleanVATIDList trims whitespace and drops empty entries from a list
// of VAT-IDs.
func cleanVATIDList(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, v := range ids {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// normalizeVATID strips formatting so we can compare two VAT-IDs
// loosely (case, spaces, dots, dashes, slashes are all ignored).
// "DE 287 472 874" ≡ "de287472874" ≡ "DE-287472874".
func normalizeVATID(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case ' ', '\t', '-', '.', '/', ' ':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isOwnVATID reports whether extracted matches any of ownVATIDs after
// normalization. Used as defense-in-depth after extraction: even if
// Claude ignored the prompt instruction, never store the user's own
// VAT-ID.
func isOwnVATID(extracted string, ownVATIDs []string) bool {
	norm := normalizeVATID(extracted)
	if norm == "" {
		return false
	}
	for _, own := range ownVATIDs {
		if normalizeVATID(own) == norm {
			return true
		}
	}
	return false
}

// Extractor extracts invoice metadata using Claude API.
type Extractor struct {
	client       *Client
	logger       *logging.Logger
	debug        bool
	accountHints []core.SKRAccount
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

// SetAccountHints provides the profile's chart so the extractor can propose
// fitting Gegenkonten for unknown suppliers.
func (e *Extractor) SetAccountHints(accounts []core.SKRAccount) {
	e.accountHints = accounts
}

// accountHintSection renders the chart + the suggestion instruction appended to
// the extraction prompt. Empty when no hints are set.
func (e *Extractor) accountHintSection() string {
	if len(e.accountHints) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n=== VERFÜGBARE GEGENKONTEN (SKR04) ===\n")
	b.WriteString("Schlage zusätzlich 1–3 am besten passende Gegenkonten für diese Rechnung vor. Verwende AUSSCHLIESSLICH Kontonummern aus der folgenden Liste und gib sie als Feld \"gegenkonto_vorschlaege\" (JSON-Array von Zahlen, beste zuerst) zurück. Wenn keines passt, ein leeres Array.\n")
	for _, a := range e.accountHints {
		b.WriteString(fmt.Sprintf("%d %s\n", a.Number, a.Name))
	}
	return b.String()
}

// statementSystemPrompt instructs Claude to extract bank-statement
// header data from one page image. Strictly JSON, German date format,
// dot as decimal separator.
const statementSystemPrompt = `Du erhältst die Seiten eines Bankkontoauszugs als Bilder (Seite 1, dann Seite 2 usw.). Extrahiere die folgenden Felder als strenges JSON-Objekt:

{
  "date_from": "Erstes Datum der Berichtsperiode im Format dd.MM.yyyy, oder null",
  "date_to": "Letztes Datum der Berichtsperiode im Format dd.MM.yyyy, oder null",
  "number": "Auszugsnummer, oder null",
  "opening_balance": 0.0,
  "closing_balance": 0.0
}

Regeln:
- Deine gesamte Antwort MUSS mit "{" beginnen und mit "}" enden. KEINE
  Einleitung, KEINE Erklärungen, KEINE Markdown-Codeblöcke, KEINE Listen.
  Wenn ein Wert nicht erkennbar ist, schreibe einfach null bzw. 0.0 — gib
  KEINEN Erklärungstext aus.
- Bei Unsicherheit: null bzw. 0.0 verwenden.
- Beträge: Punkt als Dezimaltrennzeichen, ohne Tausenderzeichen.
- date_from / date_to im deutschen Format dd.MM.yyyy.
- "Opening balance" / "Anfangssaldo" / "Saldo Vortrag" / "Alter Kontostand" → opening_balance.
- "Closing balance" / "Endsaldo" / "Neuer Saldo" → closing_balance.
- Wenn "Kontostand am DD.MM.YYYY" mehrmals vorkommt: das **frühere** Datum
  (typischerweise vor Beginn der Berichtsperiode oder direkt am Anfang) ist
  der opening_balance, das **spätere** (am oder nach dem Periodenende) ist
  der closing_balance. Beispiel: "Kontostand am 30.12.2025 = 33.884,98"
  und später "Kontostand am 31.01.2026 = 12.345,67" → opening_balance
  = 33884.98, closing_balance = 12345.67.

Auszugsnummer (number) — typische Formate, die du erkennen sollst:
  - "Kontoauszug N/YYYY"  → übernimm exakt "N/YYYY" (z. B. "Kontoauszug 1/2026" → "1/2026").
  - "Auszug Nr. N" oder "Auszugsnummer N"  → "N".
  - "Statement No. N" / "Statement Number N"  → "N".
  - "Kontoauszug Nr. N vom DD.MM.YYYY"  → "N".
  Wenn mehrere Kandidaten auf dem Auszug stehen, bevorzuge das "N/YYYY"-Format
  (das ist die monatliche/periodische Nummer). Pure Jahreskumulationen wie
  "Auszug Nr. 53" sind das zweite Wahl, falls kein N/YYYY vorhanden ist.
`

// ExtractStatementFromImages extracts bank-statement header metadata
// from multiple page images. Use this for PDFs where the closing
// balance commonly lives on the last page, not the first.
func (e *Extractor) ExtractStatementFromImages(ctx context.Context, apiKey, model string, imagesBase64 []string, mediaType string) (core.StatementMetadata, error) {
	visionPrompt := "Bitte extrahiere die Kontoauszug-Metadaten anhand aller mitgesendeten Seiten."

	if e.debug && e.logger != nil {
		e.logger.Debug("=== CLAUDE STATEMENT VISION REQUEST (%d pages) ===", len(imagesBase64))
		e.logger.Debug("Model: %s, media type: %s", model, mediaType)
	}

	response, err := e.client.SendWithImages(ctx, apiKey, model,
		statementSystemPrompt, visionPrompt, imagesBase64, mediaType)
	if err != nil {
		return core.StatementMetadata{}, fmt.Errorf("Vision API request failed: %w", err)
	}

	if e.debug && e.logger != nil {
		e.logger.Debug("=== CLAUDE STATEMENT VISION RESPONSE ===")
		e.logger.Debug("Response: %s", response)
	}

	return parseStatementResponse(response)
}

// ExtractStatementFromImage is the single-image convenience wrapper
// (e.g. for JPG/PNG statement scans).
func (e *Extractor) ExtractStatementFromImage(ctx context.Context, apiKey, model, imageBase64, mediaType string) (core.StatementMetadata, error) {
	return e.ExtractStatementFromImages(ctx, apiKey, model, []string{imageBase64}, mediaType)
}

// parseStatementResponse converts Claude's JSON response into a
// StatementMetadata. Tolerates missing fields.
func parseStatementResponse(response string) (core.StatementMetadata, error) {
	rawHead := response
	if len(rawHead) > 200 {
		rawHead = rawHead[:200] + "…"
	}
	response = cleanJSONResponse(response)
	var result struct {
		DateFrom       *string  `json:"date_from"`
		DateTo         *string  `json:"date_to"`
		Number         *string  `json:"number"`
		OpeningBalance *float64 `json:"opening_balance"`
		ClosingBalance *float64 `json:"closing_balance"`
	}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return core.StatementMetadata{}, fmt.Errorf(
			"failed to parse statement JSON: %w\nRaw response head: %s\nCleaned: %s",
			err, rawHead, response)
	}
	out := core.StatementMetadata{}
	if result.DateFrom != nil {
		out.DateFrom = *result.DateFrom
	}
	if result.DateTo != nil {
		out.DateTo = *result.DateTo
	}
	if result.Number != nil {
		out.Number = *result.Number
	}
	if result.OpeningBalance != nil {
		out.OpeningBalance = *result.OpeningBalance
	}
	if result.ClosingBalance != nil {
		out.ClosingBalance = *result.ClosingBalance
	}
	return out, nil
}

// ExtractFromImage extracts invoice metadata from a PDF image using Claude Vision API.
// ownVATIDs lists the user's own company VAT-IDs that should be excluded
// from extraction (the receiver of the invoice).
func (e *Extractor) ExtractFromImage(ctx context.Context, apiKey, model, imageBase64, mediaType string, ownVATIDs ...string) (core.Meta, float64, error) {
	// Simplified prompt for vision - Claude can see the invoice directly
	visionPrompt := "Bitte extrahiere die Rechnungsinformationen aus diesem Dokument."
	prompt := systemPromptFor(ownVATIDs) + e.accountHintSection()

	// Debug logging: log request
	if e.debug && e.logger != nil {
		e.logger.Debug("=== CLAUDE VISION API REQUEST ===")
		e.logger.Debug("Model: %s", model)
		e.logger.Debug("Image size: %d bytes (base64)", len(imageBase64))
		e.logger.Debug("Media type: %s", mediaType)
		e.logger.Debug("Own VAT-IDs to exclude: %v", ownVATIDs)
	}

	// Send request with image
	response, err := e.client.SendWithImage(ctx, apiKey, model, prompt, visionPrompt, imageBase64, mediaType)
	if err != nil {
		if e.debug && e.logger != nil {
			e.logger.Debug("=== CLAUDE VISION API ERROR ===")
			e.logger.Debug("Error: %v", err)
		}
		return core.Meta{}, 0, fmt.Errorf("vision API request failed: %w", err)
	}

	// Debug logging: log response
	if e.debug && e.logger != nil {
		e.logger.Debug("=== CLAUDE VISION API RESPONSE ===")
		e.logger.Debug("Response length: %d chars", len(response))
		e.logger.Debug("Full response: %s", response)
	}

	// Parse the JSON response (same as text extraction)
	meta, err := parseExtractionResponse(response, ownVATIDs)
	if err != nil {
		return core.Meta{}, 0, err
	}
	if e.debug && e.logger != nil && meta.VATID == "" && len(ownVATIDs) > 0 {
		e.logger.Debug("VAT-ID either not detected or matched an own VAT-ID and was filtered out")
	}

	// Confidence is high for Claude Vision (assume 0.95 for vision)
	confidence := 0.95

	return meta, confidence, nil
}

// Extract extracts invoice metadata from text using Claude API.
// ownVATIDs lists the user's own company VAT-IDs that should be excluded
// from extraction (the receiver of the invoice).
func (e *Extractor) Extract(ctx context.Context, apiKey, model, text string, ownVATIDs ...string) (core.Meta, float64, error) {
	// Limit text length to avoid token limits
	// Keep first 10000 chars, prioritizing invoice-relevant content
	text = preprocessText(text, 10000)
	prompt := systemPromptFor(ownVATIDs) + e.accountHintSection()

	// Debug logging: log request
	if e.debug && e.logger != nil {
		e.logger.Debug("=== CLAUDE API REQUEST ===")
		e.logger.Debug("Model: %s", model)
		e.logger.Debug("Text length: %d chars", len(text))
		e.logger.Debug("Text preview (first 500 chars): %s", truncate(text, 500))
		e.logger.Debug("System prompt length: %d chars", len(prompt))
		e.logger.Debug("Own VAT-IDs to exclude: %v", ownVATIDs)
	}

	// Send request
	response, err := e.client.Send(ctx, apiKey, model, prompt, text)
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
	meta, err := parseExtractionResponse(response, ownVATIDs)
	if err != nil {
		return core.Meta{}, 0, err
	}
	if e.debug && e.logger != nil && meta.VATID == "" && len(ownVATIDs) > 0 {
		e.logger.Debug("VAT-ID either not detected or matched an own VAT-ID and was filtered out")
	}

	// Confidence is high for Claude (assume 0.9 if we got a response)
	confidence := 0.9

	return meta, confidence, nil
}

// ExtractMultimodal extracts invoice metadata from BOTH the extracted text and
// the rendered page images. Many receipts (POS / SumUp / restaurant bills) carry
// their VAT table as an image that text extraction cannot read; sending the page
// images alongside the text lets Claude read the form text and the image table
// together. imagesBase64 are PNG-encoded pages, mediaType e.g. "image/png".
func (e *Extractor) ExtractMultimodal(ctx context.Context, apiKey, model, text string, imagesBase64 []string, mediaType string, ownVATIDs ...string) (core.Meta, float64, error) {
	text = preprocessText(text, 10000)
	prompt := systemPromptFor(ownVATIDs) + e.accountHintSection()
	userMessage := "Dokumenttext (kann unvollständig sein — Tabellen/Beträge stehen oft nur in den beigefügten Seitenbildern, dann diese auswerten):\n\n" + text

	if e.debug && e.logger != nil {
		e.logger.Debug("=== CLAUDE MULTIMODAL API REQUEST ===")
		e.logger.Debug("Model: %s, text: %d chars, images: %d", model, len(text), len(imagesBase64))
	}

	response, err := e.client.SendWithImages(ctx, apiKey, model, prompt, userMessage, imagesBase64, mediaType)
	if err != nil {
		if e.debug && e.logger != nil {
			e.logger.Debug("=== CLAUDE MULTIMODAL API ERROR ===\nError: %v", err)
		}
		return core.Meta{}, 0, fmt.Errorf("multimodal API request failed: %w", err)
	}

	if e.debug && e.logger != nil {
		e.logger.Debug("=== CLAUDE MULTIMODAL API RESPONSE ===\nFull response: %s", response)
	}

	meta, err := parseExtractionResponse(response, ownVATIDs)
	if err != nil {
		return core.Meta{}, 0, err
	}
	return meta, 0.95, nil
}

// parseExtractionJSON parses the JSON response from Claude API into a Meta.
// ownVATIDs is the list of the user's own VAT-IDs; if Claude returned
// one of them as vat_id, we drop it (defense-in-depth, since the LLM
// can ignore prompt instructions).
// This is exported for unit-testability.
func parseExtractionJSON(response string, ownVATIDs []string) (core.Meta, error) {
	// Clean response (remove any markdown code blocks if present)
	response = cleanJSONResponse(response)

	// Parse JSON response
	var result struct {
		Auftraggeber     *string `json:"auftraggeber"`
		Verwendungszweck *string `json:"verwendungszweck"`
		Rechnungsnummer  *string `json:"rechnungsnummer"`
		VATID            *string `json:"vat_id"`
		Steuerzeilen     []struct {
			Satz  float64 `json:"satz"`
			Netto float64 `json:"netto"`
			MwSt  float64 `json:"mwst"`
		} `json:"steuerzeilen"`
		Trinkgeld               *float64 `json:"trinkgeld"`
		Bruttobetrag            *float64 `json:"bruttobetrag"`
		Waehrung                *string  `json:"waehrung"`
		Rechnungsdatum          *string  `json:"rechnungsdatum"`
		Jahr                    *string  `json:"jahr"`
		Monat                   *string  `json:"monat"`
		GegenkontoVorschlaege   []int    `json:"gegenkonto_vorschlaege"`
		Ausgangsrechnung        *bool    `json:"ausgangsrechnung"`
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
		meta.Verwendungszweck = core.NormalizeVerwendungszweck(*result.Verwendungszweck)
	}
	if result.Rechnungsnummer != nil {
		meta.Rechnungsnummer = *result.Rechnungsnummer
	}
	if result.VATID != nil {
		val := strings.TrimSpace(*result.VATID)
		if isOwnVATID(val, ownVATIDs) {
			// Defense-in-depth: Claude occasionally returns the user's
			// own VAT-ID despite the prompt forbidding it (especially
			// on Ausgangsrechnungen where the user IS the sender).
			val = ""
		}
		meta.VATID = val
	}
	for _, z := range result.Steuerzeilen {
		meta.TaxLines = append(meta.TaxLines, core.TaxLine{Netto: z.Netto, SatzProzent: z.Satz, MwStBetrag: z.MwSt})
	}
	if result.Trinkgeld != nil {
		meta.Trinkgeld = *result.Trinkgeld
	}
	meta.BetragNetto = core.SumNetto(meta.TaxLines)
	meta.SteuersatzBetrag = core.SumMwSt(meta.TaxLines)
	meta.SteuersatzProzent = core.PrimarySatz(meta.TaxLines)
	if result.Bruttobetrag != nil && *result.Bruttobetrag > 0 {
		meta.Bruttobetrag = *result.Bruttobetrag
	} else {
		meta.Bruttobetrag = core.ComputeBrutto(meta.TaxLines, meta.Trinkgeld)
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
	meta.KontoVorschlaege = result.GegenkontoVorschlaege
	if result.Ausgangsrechnung != nil {
		meta.Ausgangsrechnung = *result.Ausgangsrechnung
	}

	return meta, nil
}

// parseExtractionResponse is an alias for parseExtractionJSON kept for
// backward compatibility with existing tests that call it directly.
func parseExtractionResponse(response string, ownVATIDs []string) (core.Meta, error) {
	return parseExtractionJSON(response, ownVATIDs)
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

// cleanJSONResponse strips markdown code fences and prose around the
// JSON object Claude returned, so the unmarshaller sees just the
// "{...}" payload. Claude occasionally narrates the image content
// before emitting JSON, especially for ambiguous screenshots — this
// recovers gracefully from that.
func cleanJSONResponse(response string) string {
	response = strings.TrimSpace(response)

	// Remove ```json ... ``` blocks if the response starts with one.
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
		response = strings.TrimSuffix(response, "```")
		response = strings.TrimSpace(response)
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
		response = strings.TrimSuffix(response, "```")
		response = strings.TrimSpace(response)
	}

	// If anything else (prose, list items, etc.) precedes the JSON,
	// fall back to the substring between the first '{' and the last '}'.
	if !strings.HasPrefix(response, "{") {
		start := strings.Index(response, "{")
		end := strings.LastIndex(response, "}")
		if start >= 0 && end > start {
			return response[start : end+1]
		}
	}
	return response
}

// RankStatementLine asks Claude which of the provided statement-line texts
// best matches the given supplier name. It returns the 0-based index of the
// best match, or -1 if none matches. Only the integer (or -1) is expected in
// Claude's reply. Errors are returned as (-1, err); the caller should treat
// them as non-fatal and fall back to heuristic ordering.
func (e *Extractor) RankStatementLine(ctx context.Context, apiKey, model, supplier string, lineTexts []string) (int, error) {
	if len(lineTexts) == 0 {
		return -1, fmt.Errorf("no line texts provided")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Welche dieser Kontoauszug-Zeilen gehört am ehesten zum Lieferanten %q? Antworte NUR mit der Zahl (0-basierter Index) oder -1, wenn keine passt. Keine Erklärung.\n\n", supplier))
	for i, t := range lineTexts {
		sb.WriteString(fmt.Sprintf("%d: %s\n", i, t))
	}

	reply, err := e.client.Send(ctx, apiKey, model, "Du hilfst beim Belegabgleich.", sb.String())
	if err != nil {
		return -1, fmt.Errorf("RankStatementLine API call failed: %w", err)
	}

	// Parse integer from reply — trim whitespace, handle leading/trailing noise.
	reply = strings.TrimSpace(reply)
	// Extract just the first run of digits (possibly with a leading minus sign).
	numStr := ""
	for _, r := range reply {
		if (r >= '0' && r <= '9') || (r == '-' && numStr == "") {
			numStr += string(r)
		} else if numStr != "" {
			break
		}
	}
	if numStr == "" || numStr == "-" {
		return -1, fmt.Errorf("RankStatementLine: could not parse integer from reply %q", reply)
	}

	var idx int
	if _, err := fmt.Sscanf(numStr, "%d", &idx); err != nil {
		return -1, fmt.Errorf("RankStatementLine: parse %q: %w", numStr, err)
	}

	// Clamp to valid range.
	if idx < -1 {
		idx = -1
	}
	if idx >= len(lineTexts) {
		idx = len(lineTexts) - 1
	}

	return idx, nil
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
