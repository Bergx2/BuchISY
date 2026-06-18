package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Neo accent palette — picked to read well in both light and dark mode.
var (
	accentBelege = color.NRGBA{R: 36, G: 99, B: 167, A: 255}   // calm blue
	accentKonten = color.NRGBA{R: 38, G: 132, B: 84, A: 255}   // warm green
	stripeLight  = color.NRGBA{R: 248, G: 250, B: 252, A: 255} // very pale blue
	stripeDark   = color.NRGBA{R: 30, G: 34, B: 38, A: 80}     // semi-transparent dark
	cardLight    = color.NRGBA{R: 252, G: 252, B: 253, A: 255}
	cardDark     = color.NRGBA{R: 40, G: 44, B: 50, A: 255}
)

// stripeColor returns the alternating row background for the current
// theme variant.
func stripeColor() color.Color {
	if fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantDark {
		return stripeDark
	}
	return stripeLight
}

// cardBackgroundColor returns the subtle card-tinted background.
func cardBackgroundColor() color.Color {
	if fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantDark {
		return cardDark
	}
	return cardLight
}

// Bounds and step for the user-controlled UI zoom.
const (
	MinUIScale  float32 = 0.6
	MaxUIScale  float32 = 2.5
	UIScaleStep float32 = 0.1
)

// buchisyTheme wraps the default theme to (a) enlarge the scrollbars so
// the horizontal scrollbar at the bottom of the invoice table stays
// easily visible, (b) apply a user-controlled zoom factor to all size
// metrics, and (c) swap the primary accent colour to a Neo blue/green
// depending on which mode the app is in.
type buchisyTheme struct {
	fyne.Theme
	scale  float32
	accent color.Color
}

func newBuchisyTheme(scale float32) *buchisyTheme {
	return &buchisyTheme{
		Theme:  theme.DefaultTheme(),
		scale:  clampUIScale(scale),
		accent: accentBelege,
	}
}

// SetAccent swaps the primary colour to the given accent.
func (t *buchisyTheme) SetAccent(c color.Color) { t.accent = c }

// Color overrides the primary swatch with our accent so HighImportance
// buttons / progress bars / sliders pick it up automatically.
func (t *buchisyTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNamePrimary && t.accent != nil {
		return t.accent
	}
	return t.Theme.Color(name, variant)
}

// Scale returns the current zoom factor.
func (t *buchisyTheme) Scale() float32 {
	return t.scale
}

// SetScale updates the zoom factor in place. Caller must trigger a
// theme refresh (e.g. fyne.CurrentApp().Settings().SetTheme(t)) for
// the change to become visible.
func (t *buchisyTheme) SetScale(s float32) {
	t.scale = clampUIScale(s)
}

func (t *buchisyTheme) Size(name fyne.ThemeSizeName) float32 {
	var base float32
	switch name {
	case theme.SizeNameScrollBar:
		base = 20
	case theme.SizeNameScrollBarSmall:
		base = 10
	default:
		base = t.Theme.Size(name)
	}
	return base * t.scale
}

func clampUIScale(s float32) float32 {
	if s < MinUIScale {
		return MinUIScale
	}
	if s > MaxUIScale {
		return MaxUIScale
	}
	return s
}

// compactListTheme wraps another theme with reduced padding. Applied via
// container.NewThemeOverride to the file picker's list so its rows take
// less vertical space and more files/folders fit on screen.
type compactListTheme struct {
	fyne.Theme
}

func newCompactListTheme(base fyne.Theme) fyne.Theme {
	return &compactListTheme{Theme: base}
}

func (t *compactListTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return t.Theme.Size(name) * 0.15
	case theme.SizeNameInnerPadding:
		return t.Theme.Size(name) * 0.2
	}
	return t.Theme.Size(name)
}
