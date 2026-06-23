package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBookingRules(t *testing.T) {
	data, _ := os.ReadFile(filepath.Join("..", "..", "assets", "buchungsregeln.json"))
	r, err := ParseBookingRules(data)
	if err != nil {
		t.Fatal(err)
	}
	bew, ok := r.Rule("bewirtung")
	if !ok || bew.AbziehbarProzent != 70 || bew.KontoAbziehbar != 6640 || bew.KontoNichtAbziehbar != 6644 {
		t.Fatalf("bewirtung rule = %+v", bew)
	}
	if _, ok := r.Rule("standard"); !ok {
		t.Error("standard rule missing")
	}
	if k, ok := r.VorsteuerKonto(19); !ok || k != 1406 {
		t.Errorf("VorsteuerKonto(19) = %d,%v", k, ok)
	}
	if k, ok := r.VorsteuerKonto(7); !ok || k != 1401 {
		t.Errorf("VorsteuerKonto(7) = %d,%v", k, ok)
	}
	if _, ok := r.VorsteuerKonto(0); ok {
		t.Error("VorsteuerKonto(0) should be false")
	}
}

func TestBundledChartHasNewCategories(t *testing.T) {
	data, _ := os.ReadFile(filepath.Join("..", "..", "assets", "buchungsregeln.json"))
	r, err := ParseBookingRules(data)
	if err != nil {
		t.Fatal(err)
	}
	rc, ok := r.Rule("reverse_charge")
	if !ok || rc.RcSatz != 19 || rc.KontoVStRC != 1407 || rc.KontoUStRC != 3837 {
		t.Errorf("reverse_charge rule = %+v", rc)
	}
	g, ok := r.Rule("geschenke")
	if !ok || g.Schwelle != 35 || g.KontoAbziehbar != 6610 || g.KontoNichtAbziehbar != 6620 {
		t.Errorf("geschenke rule = %+v", g)
	}
	rk, ok := r.Rule("reisekosten")
	if !ok || rk.DefaultKonto != 6650 {
		t.Errorf("reisekosten rule = %+v", rk)
	}
	kfz, ok := r.Rule("kfz")
	if !ok || kfz.DefaultKonto != 6520 {
		t.Errorf("kfz rule = %+v", kfz)
	}
}

func TestErloesKonto(t *testing.T) {
	r := &BookingRules{ErloesKonten: map[string]int{"inland": 8400, "eu": 8341, "drittland": 8200}}
	if k, _ := r.ErloesKonto("DE123", 19); k != 8400 {
		t.Errorf("domestic (with VAT) = %d, want 8400", k)
	}
	if k, _ := r.ErloesKonto("FI26378052", 0); k != 8341 {
		t.Errorf("EU 0%% = %d, want 8341", k)
	}
	if k, _ := r.ErloesKonto("", 0); k != 8200 {
		t.Errorf("Drittland 0%% = %d, want 8200", k)
	}
	if _, ok := (&BookingRules{}).ErloesKonto("DE", 19); ok {
		t.Error("unset config must return ok=false")
	}
}
