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
