package core

// InferBookingCategory guesses the booking category from a stored booking's
// accounts, so reopening an existing invoice restores the correct category
// instead of defaulting to "standard" (which would recompute a wrong booking
// and overwrite the stored one on save). Returns "" when no special category is
// evident (caller keeps its default / learned template).
//
//   - §13b reverse-charge accounts (SKR03 1577/1787, SKR04 1407/3837) → "reverse_charge"
//   - both a Bewirtung deductible (4650/6640) AND non-deductible (4654/6644)
//     entry → "bewirtung"
func InferBookingCategory(b Booking) string {
	var hasRC, hasBewAbz, hasBewNicht bool
	for _, e := range b.Entries {
		switch e.Konto {
		case 1577, 1787, 1407, 3837:
			hasRC = true
		case 4650, 6640:
			hasBewAbz = true
		case 4654, 6644:
			hasBewNicht = true
		}
	}
	if hasRC {
		return "reverse_charge"
	}
	if hasBewAbz && hasBewNicht {
		return "bewirtung"
	}
	return ""
}
