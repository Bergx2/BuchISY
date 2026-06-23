# E17.2 — Stichwort→Konto-Vorschlag für neue Lieferanten (#3)

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** For a supplier not yet in the company→account memory, suggest a Gegenkonto from keywords found in the supplier name + Verwendungszweck (instead of falling straight to the placeholder default).

**Tech:** Go 1.25, Fyne. `internal/core`, `internal/ui`. Branch `feat/e17-2-stichwort-konto`.

## Global Constraints

- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- Keyword→account is per-profile (chart-specific). Do NOT seed accounts into the bundled `assets/buchungsregeln.json` (would pollute other-chart profiles). Empty/absent config → `SuggestKonto` returns `(0,false)` → existing DefaultAccount behaviour (backward-compatible).
- Matching is case-insensitive substring; the LONGEST matching keyword wins (most specific), deterministic regardless of map order.

---

### Task 1: Core — `KontoStichwoerter` config + `SuggestKonto`

**Files:** `internal/core/buchungsregeln.go`, test `internal/core/buchungsregeln_test.go`.

**Interface:** `BookingRules.KontoStichwoerter map[string]int json:"konto_stichwoerter,omitempty"`; `BookingRules.SuggestKonto(text string) (int, bool)`.

- [ ] **Step 1: Test** (append):

```go
func TestSuggestKonto(t *testing.T) {
	r := &BookingRules{KontoStichwoerter: map[string]int{"tankstelle": 4663, "aral": 4663, "hotel": 4660, "telekom": 4920}}
	cases := []struct {
		text string
		want int
		ok   bool
	}{
		{"ARAL Tankstelle München", 4663, true},
		{"Best Western Hotel", 4660, true},
		{"Deutsche Telekom GmbH", 4920, true},
		{"Unbekannter Lieferant XY", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		k, ok := r.SuggestKonto(c.text)
		if k != c.want || ok != c.ok {
			t.Errorf("SuggestKonto(%q) = (%d,%v), want (%d,%v)", c.text, k, ok, c.want, c.ok)
		}
	}
	// longest keyword wins: "deutsche bahn" (4670) over "bahn" (4671)
	r2 := &BookingRules{KontoStichwoerter: map[string]int{"bahn": 4671, "deutsche bahn": 4670}}
	if k, _ := r2.SuggestKonto("Fahrkarte Deutsche Bahn AG"); k != 4670 {
		t.Errorf("longest-match: got %d, want 4670", k)
	}
	// unset config → no suggestion, no panic
	if _, ok := (&BookingRules{}).SuggestKonto("ARAL"); ok {
		t.Error("nil map must return ok=false")
	}
}
```

- [ ] **Step 2:** run → fail.
- [ ] **Step 3:** Add field `KontoStichwoerter map[string]int json:"konto_stichwoerter,omitempty"` to `BookingRules`. Implement:

```go
// SuggestKonto proposes a Gegenkonto for a new supplier by scanning text
// (supplier name + Verwendungszweck) for configured keywords. Case-insensitive
// substring match; the longest matching keyword wins (most specific). Returns
// (0,false) when nothing matches or no keywords are configured.
func (r *BookingRules) SuggestKonto(text string) (int, bool) {
	if len(r.KontoStichwoerter) == 0 || strings.TrimSpace(text) == "" {
		return 0, false
	}
	lower := strings.ToLower(text)
	bestKw, bestKonto := "", 0
	for kw, konto := range r.KontoStichwoerter {
		k := strings.ToLower(strings.TrimSpace(kw))
		if k == "" {
			continue
		}
		if strings.Contains(lower, k) && len(k) > len(bestKw) {
			bestKw, bestKonto = k, konto
		}
	}
	if bestKw == "" {
		return 0, false
	}
	return bestKonto, true
}
```

Add `"strings"` to imports if not present.

- [ ] **Step 4:** run → pass + full core. Commit `E17.2: konto_stichwoerter config + BookingRules.SuggestKonto`.

## Self-Review

Coverage: #3 → Task 1 (the selector); the modal wiring + the Bergx2 keyword data are applied by the controller after merge. No bundled seed (per constraints). DRY/deterministic longest-match.
