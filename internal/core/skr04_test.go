package core

import "testing"

func TestParseChartJSON(t *testing.T) {
	data := []byte(`[{"number":6640,"name":"Bewirtungskosten","type":"expense"},{"number":1800,"name":"Bank","type":"asset"}]`)
	accs, err := ParseChartJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(accs) != 2 || accs[0].Number != 6640 {
		t.Fatalf("got %+v", accs)
	}
	if got, _ := ParseChartJSON([]byte("  ")); len(got) != 0 {
		t.Error("blank input should parse to empty slice")
	}
}

func TestParseChartCSV(t *testing.T) {
	csv := "Konto;Bezeichnung;Steuerschlüssel\n6640;Bewirtungskosten;\n1800;Bank;\nnicht;eine Zahl;\n"
	accs, err := ParseChartCSV([]byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(accs) != 2 {
		t.Fatalf("got %d accounts, want 2: %+v", len(accs), accs)
	}
	if accs[0].Number != 6640 || accs[0].Name != "Bewirtungskosten" {
		t.Errorf("row 0 = %+v", accs[0])
	}
}

func TestChartFindSearch(t *testing.T) {
	c := NewChartOfAccounts([]SKRAccount{
		{Number: 6644, Name: "Nicht abziehbare Bewirtungskosten", Type: "expense"},
		{Number: 6640, Name: "Bewirtungskosten", Type: "expense"},
		{Number: 1800, Name: "Bank", Type: "asset"},
	})
	got, ok := c.Find(6640)
	if !ok || got.Name != "Bewirtungskosten" {
		t.Fatalf("Find(6640) = %+v, %v", got, ok)
	}
	if _, ok := c.Find(9999); ok {
		t.Error("Find(9999) should be false")
	}
	all := c.All()
	if len(all) != 3 || all[0].Number != 1800 {
		t.Fatalf("All() not sorted by number: %+v", all)
	}
	// Search by name fragment (case-insensitive) and by number text.
	if r := c.Search("bewirt"); len(r) != 2 {
		t.Errorf("Search(bewirt) = %d, want 2", len(r))
	}
	if r := c.Search("1800"); len(r) != 1 || r[0].Number != 1800 {
		t.Errorf("Search(1800) = %+v", r)
	}
}
