package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeBankAccounts(t *testing.T) {
	in := []BankAccount{
		{Name: "A", IsCreditCard: true},           // legacy card -> creditcard
		{Name: "B"},                               // empty type -> bank
		{Name: "C", AccountType: AccountTypeCash}, // valid -> kept
		{Name: "D", AccountType: "weird"},         // unknown -> bank
		{Name: "E", AccountType: AccountTypeCreditCard, IsCreditCard: true}, // valid kept
	}
	want := []string{
		AccountTypeCreditCard,
		AccountTypeBank,
		AccountTypeCash,
		AccountTypeBank,
		AccountTypeCreditCard,
	}

	got := normalizeBankAccounts(in)
	if len(got) != len(want) {
		t.Fatalf("got %d accounts, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].AccountType != w {
			t.Errorf("account %d: AccountType = %q, want %q", i, got[i].AccountType, w)
		}
		if got[i].IsCreditCard {
			t.Errorf("account %d: legacy IsCreditCard should be cleared", i)
		}
	}
}

func TestGetProfileConfigDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("APPDATA", tmp)
	got, err := GetProfileConfigDir("Bergx2")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(tmp, "BuchISY", "profiles", "Bergx2")
	if got != want {
		t.Errorf("GetProfileConfigDir = %q, want %q", got, want)
	}
}

func TestListProfiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("APPDATA", tmp)

	names, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles on missing dir: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected no profiles, got %v", names)
	}

	base := filepath.Join(tmp, "BuchISY", "profiles")
	if err := os.MkdirAll(filepath.Join(base, "Bergx2"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "Boomstraat"), 0755); err != nil {
		t.Fatal(err)
	}
	names, err = ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 profiles, got %v", names)
	}
}
