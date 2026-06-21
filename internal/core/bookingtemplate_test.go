package core

import (
	"path/filepath"
	"testing"
)

func TestBookingTemplateStore(t *testing.T) {
	dir := t.TempDir()
	s := NewBookingTemplateStore(dir)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("Matcha Rina"); ok {
		t.Error("expected no template yet")
	}
	if err := s.Set("Matcha Rina", BookingTemplate{Kategorie: "bewirtung", ExpenseKonto: 6640}); err != nil {
		t.Fatal(err)
	}
	// A fresh store over the same dir must read the persisted template.
	s2 := NewBookingTemplateStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get("Matcha Rina")
	if !ok || got.Kategorie != "bewirtung" || got.ExpenseKonto != 6640 {
		t.Fatalf("template not persisted: %+v %v", got, ok)
	}
	_ = filepath.Join(dir, "booking_templates.json")
}
