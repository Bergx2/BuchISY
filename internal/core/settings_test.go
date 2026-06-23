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
		{Name: "F", AccountType: AccountTypePayroll},                        // valid -> kept
	}
	want := []string{
		AccountTypeCreditCard,
		AccountTypeBank,
		AccountTypeCash,
		AccountTypeBank,
		AccountTypeCreditCard,
		AccountTypePayroll,
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

func TestPaymentAccountSKR04(t *testing.T) {
	s := Settings{BankAccounts: []BankAccount{
		{Name: "Sparkasse", AccountType: AccountTypeBank, SKR04Konto: 1800},
		{Name: "Barkasse", AccountType: AccountTypeCash},        // no explicit → fallback 1600
		{Name: "Visa", AccountType: AccountTypeCreditCard},      // no mapping → (0,false)
		{Name: "Gehaltserstattung", AccountType: AccountTypePayroll, SKR04Konto: 1755}, // payroll liability
		{Name: "Lohn ohne Konto", AccountType: AccountTypePayroll},                     // payroll, no account
	}}
	if k, ok := s.PaymentAccountSKR04("Sparkasse"); !ok || k != 1800 {
		t.Errorf("Sparkasse = %d,%v", k, ok)
	}
	if k, ok := s.PaymentAccountSKR04("Barkasse"); !ok || k != 1600 {
		t.Errorf("Barkasse (cash fallback) = %d,%v", k, ok)
	}
	if _, ok := s.PaymentAccountSKR04("Visa"); ok {
		t.Error("Visa without mapping should be (0,false)")
	}
	if k, ok := s.PaymentAccountSKR04("Gehaltserstattung"); !ok || k != 1755 {
		t.Errorf("payroll with explicit liability account = %d,%v, want 1755,true", k, ok)
	}
	if _, ok := s.PaymentAccountSKR04("Lohn ohne Konto"); ok {
		t.Error("payroll without explicit account should be (0,false)")
	}
	if _, ok := s.PaymentAccountSKR04("Unbekannt"); ok {
		t.Error("unknown account name should be (0,false)")
	}
}
