package core

import "testing"

func TestApplyTemplateKurzbez8(t *testing.T) {
	opts := TemplateOpts{DecimalSeparator: ","}
	cases := []struct {
		kurzbez string
		want    string
	}{
		{"Software Projekt Entwicklung", "Software"},
		{"Abc", "Abc"},
		{"", ""},
	}
	for _, c := range cases {
		got, err := ApplyTemplate("${Kurzbez8}", Meta{Verwendungszweck: c.kurzbez}, opts)
		if err != nil {
			t.Fatalf("ApplyTemplate(%q): %v", c.kurzbez, err)
		}
		if got != c.want {
			t.Errorf("ApplyTemplate(${Kurzbez8}) with %q = %q, want %q", c.kurzbez, got, c.want)
		}
	}
}

func TestApplyTemplateBelegnr(t *testing.T) {
	opts := TemplateOpts{DecimalSeparator: ","}
	meta := Meta{Belegnummer: "2026-0014", Auftraggeber: "Matcha Rina"}
	// Both the canonical token and the German alias resolve to the number.
	for _, tmpl := range []string{"${Belegnr}_${Company}", "${Belegnummer}_${Company}"} {
		got, err := ApplyTemplate(tmpl, meta, opts)
		if err != nil {
			t.Fatalf("ApplyTemplate(%q): %v", tmpl, err)
		}
		if got != "2026-0014_Matcha Rina" {
			t.Errorf("ApplyTemplate(%q) = %q, want 2026-0014_Matcha Rina", tmpl, got)
		}
	}
}
