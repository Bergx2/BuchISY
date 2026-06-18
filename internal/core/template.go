package core

import (
	"strings"
)

// TokenAliases maps German token names to their canonical English equivalents.
var TokenAliases = map[string]string{
	"${Firma}":             "${Company}",
	"${Rechnungsnummer}":   "${InvoiceNumber}",
	"${Kurzbez}":           "${Kurzbezeichnung}",
	"${BetragNetto}":       "${NetAmount}",
	"${SteuersatzProzent}": "${TaxPercent}",
	"${Steuerbetrag}":      "${TaxAmount}",
	"${Bruttobetrag}":      "${GrossAmount}",
	"${Waehrung}":          "${Currency}",
	"${Jahr}":              "${YYYY}",
	"${Monat}":             "${MM}",
}

// TemplateOpts holds options for template rendering.
type TemplateOpts struct {
	DecimalSeparator string // "," or "."
}

// ApplyTemplate applies the naming template to generate a filename.
// Supports both canonical tokens and German aliases.
func ApplyTemplate(template string, meta Meta, opts TemplateOpts) (string, error) {
	result := template

	// First, replace aliases with canonical tokens
	for alias, canonical := range TokenAliases {
		result = strings.ReplaceAll(result, alias, canonical)
	}

	// Format amounts with decimal + thousands separators.
	formatAmount := func(amount float64) string {
		return FormatAmount(amount, opts.DecimalSeparator)
	}

	// Replace canonical tokens
	replacements := map[string]string{
		"${YYYY}":             meta.Jahr,
		"${MM}":               meta.Monat,
		"${DD}":               extractDay(meta.Rechnungsdatum),
		"${Company}":          meta.Auftraggeber,
		"${InvoiceNumber}":    meta.Rechnungsnummer,
		"${Kurzbezeichnung}":  meta.Verwendungszweck, // Keep old token name for backward compatibility
		"${Verwendungszweck}": meta.Verwendungszweck, // New token name
		"${Kurzbez8}":         first8(meta.Verwendungszweck),
		"${NetAmount}":        formatAmount(meta.BetragNetto),
		"${TaxPercent}":       formatAmount(meta.SteuersatzProzent),
		"${TaxAmount}":        formatAmount(meta.SteuersatzBetrag),
		"${GrossAmount}":      formatAmount(meta.Bruttobetrag),
		"${Currency}":         meta.Waehrung,
		"${OriginalName}":     "", // Will be filled in by caller if needed
	}

	for token, value := range replacements {
		result = strings.ReplaceAll(result, token, value)
	}

	// Sanitize the filename
	result = SanitizeFilename(result)

	return result, nil
}

// extractDay extracts the day from a DD.MM.YYYY date string.
func extractDay(date string) string {
	parts := strings.Split(date, ".")
	if len(parts) == 3 {
		return parts[0] // Day is the first part in DD.MM.YYYY format
	}
	return ""
}

// FormatGermanDate is kept for backward compatibility but dates are already in DD.MM.YYYY format.
// This function now just returns the input unchanged.
func FormatGermanDate(ddMmYyyy string) string {
	return ddMmYyyy
}

// ParseGermanDate is kept for backward compatibility.
// Since dates are now stored in DD.MM.YYYY format, this returns the input unchanged.
func ParseGermanDate(ddMmYyyy string) string {
	return ddMmYyyy
}

// first8 returns the first 8 runes of s (fewer if s is shorter).
func first8(s string) string {
	r := []rune(s)
	if len(r) > 8 {
		r = r[:8]
	}
	return string(r)
}
