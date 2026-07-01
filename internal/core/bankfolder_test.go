package core

import "testing"

func TestReconcileAccountFolders(t *testing.T) {
	accts := []BankAccount{
		{Name: "KSKMSE ...0712 Sparkasse", FolderName: "KSMSE ...0712 Sparkasse"}, // drifted → move
		{Name: "Wise ...9503", FolderName: "Wise ...9503"},                         // aligned → ensure only
		{Name: "Barkasse"},                                                         // empty pointer → backfill
		{Name: ""},                                                                 // no name → skipped
	}
	got := ReconcileAccountFolders(accts)
	if len(got) != 3 {
		t.Fatalf("want 3 actions (empty-name skipped), got %d: %+v", len(got), got)
	}
	// [0] drifted → move From→To
	want0 := SanitizeFilename("KSKMSE ...0712 Sparkasse")
	if got[0].Index != 0 || !got[0].Move || got[0].From != "KSMSE ...0712 Sparkasse" || got[0].To != want0 {
		t.Errorf("action0 = %+v; want move from %q to %q", got[0], "KSMSE ...0712 Sparkasse", want0)
	}
	// [1] aligned → no move
	if got[1].Move || got[1].From != "" || got[1].To != SanitizeFilename("Wise ...9503") {
		t.Errorf("action1 = %+v; want no-move ensure", got[1])
	}
	// [2] backfill → no move, To = name-derived
	if got[2].Move || got[2].From != "" || got[2].To != SanitizeFilename("Barkasse") {
		t.Errorf("action2 = %+v; want backfill", got[2])
	}
}
