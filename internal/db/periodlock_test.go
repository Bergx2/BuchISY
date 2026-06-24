package db

import (
	"errors"
	"testing"

	"github.com/bergx2/buchisy/internal/core"
)

// sampleRow returns a minimal CSVRow for the given period.
func sampleRow(jahr, monat, dateiname string) core.CSVRow {
	return core.CSVRow{
		Dateiname:    dateiname,
		Jahr:         jahr,
		Monat:        monat,
		Auftraggeber: "Testfirma",
		Bruttobetrag: 119,
	}
}

// TestLockPeriod_IsPeriodLocked verifies that LockPeriod causes IsPeriodLocked to return true.
func TestLockPeriod_IsPeriodLocked(t *testing.T) {
	repo := newTestRepo(t)

	locked, err := repo.IsPeriodLocked("2026", "06")
	if err != nil {
		t.Fatalf("IsPeriodLocked before lock: %v", err)
	}
	if locked {
		t.Fatal("expected unlocked before LockPeriod")
	}

	if err := repo.LockPeriod("2026", "06"); err != nil {
		t.Fatalf("LockPeriod: %v", err)
	}

	locked, err = repo.IsPeriodLocked("2026", "06")
	if err != nil {
		t.Fatalf("IsPeriodLocked after lock: %v", err)
	}
	if !locked {
		t.Fatal("expected locked after LockPeriod")
	}
}

// TestLockedPeriod_InsertBlocked verifies that Insert on a locked period returns ErrPeriodLocked.
func TestLockedPeriod_InsertBlocked(t *testing.T) {
	repo := newTestRepo(t)

	if err := repo.LockPeriod("2026", "06"); err != nil {
		t.Fatalf("LockPeriod: %v", err)
	}

	_, err := repo.Insert(sampleRow("2026", "06", "invoice.pdf"))
	if !errors.Is(err, ErrPeriodLocked) {
		t.Fatalf("Insert on locked period: got %v, want ErrPeriodLocked", err)
	}
}

// TestLockedPeriod_UpdateBlocked verifies that Update on a locked old period returns ErrPeriodLocked.
func TestLockedPeriod_UpdateBlocked(t *testing.T) {
	repo := newTestRepo(t)

	// Insert into unlocked period first.
	row := sampleRow("2026", "05", "inv.pdf")
	if _, err := repo.Insert(row); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// Lock the period.
	if err := repo.LockPeriod("2026", "05"); err != nil {
		t.Fatalf("LockPeriod: %v", err)
	}

	// Update should be blocked.
	row.Auftraggeber = "Changed"
	err := repo.Update("2026", "05", "inv.pdf", row)
	if !errors.Is(err, ErrPeriodLocked) {
		t.Fatalf("Update on locked period: got %v, want ErrPeriodLocked", err)
	}
}

// TestLockedPeriod_DeleteBlocked verifies that Delete on a locked period returns ErrPeriodLocked.
func TestLockedPeriod_DeleteBlocked(t *testing.T) {
	repo := newTestRepo(t)

	if _, err := repo.Insert(sampleRow("2026", "06", "inv.pdf")); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := repo.LockPeriod("2026", "06"); err != nil {
		t.Fatalf("LockPeriod: %v", err)
	}

	err := repo.Delete("2026", "06", "inv.pdf")
	if !errors.Is(err, ErrPeriodLocked) {
		t.Fatalf("Delete on locked period: got %v, want ErrPeriodLocked", err)
	}
}

// TestUnlockedPeriod_OpsSucceed verifies that Insert/Update/Delete succeed on an unlocked period.
func TestUnlockedPeriod_OpsSucceed(t *testing.T) {
	repo := newTestRepo(t)

	row := sampleRow("2026", "07", "ok.pdf")
	if _, err := repo.Insert(row); err != nil {
		t.Fatalf("Insert on unlocked period: %v", err)
	}

	row.Auftraggeber = "Updated"
	if err := repo.Update("2026", "07", "ok.pdf", row); err != nil {
		t.Fatalf("Update on unlocked period: %v", err)
	}

	if err := repo.Delete("2026", "07", "ok.pdf"); err != nil {
		t.Fatalf("Delete on unlocked period: %v", err)
	}
}

// TestUnlockPeriod_OpsSucceedAgain verifies that after UnlockPeriod operations succeed.
func TestUnlockPeriod_OpsSucceedAgain(t *testing.T) {
	repo := newTestRepo(t)

	if _, err := repo.Insert(sampleRow("2026", "06", "doc.pdf")); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := repo.LockPeriod("2026", "06"); err != nil {
		t.Fatalf("LockPeriod: %v", err)
	}

	// Verify locked.
	if locked, _ := repo.IsPeriodLocked("2026", "06"); !locked {
		t.Fatal("expected locked")
	}

	if err := repo.UnlockPeriod("2026", "06"); err != nil {
		t.Fatalf("UnlockPeriod: %v", err)
	}

	// Verify unlocked.
	if locked, _ := repo.IsPeriodLocked("2026", "06"); locked {
		t.Fatal("expected unlocked after UnlockPeriod")
	}

	// Insert on same period should succeed now.
	if _, err := repo.Insert(sampleRow("2026", "06", "doc2.pdf")); err != nil {
		t.Fatalf("Insert after unlock: %v", err)
	}
}

// TestLockedPeriods_ContainsPeriod verifies that LockedPeriods returns the locked period as "YYYY-MM".
func TestLockedPeriods_ContainsPeriod(t *testing.T) {
	repo := newTestRepo(t)

	if err := repo.LockPeriod("2026", "06"); err != nil {
		t.Fatalf("LockPeriod: %v", err)
	}
	if err := repo.LockPeriod("2026", "07"); err != nil {
		t.Fatalf("LockPeriod: %v", err)
	}

	periods, err := repo.LockedPeriods()
	if err != nil {
		t.Fatalf("LockedPeriods: %v", err)
	}

	found06, found07 := false, false
	for _, p := range periods {
		if p == "2026-06" {
			found06 = true
		}
		if p == "2026-07" {
			found07 = true
		}
	}
	if !found06 {
		t.Errorf("LockedPeriods missing 2026-06, got %v", periods)
	}
	if !found07 {
		t.Errorf("LockedPeriods missing 2026-07, got %v", periods)
	}
}

// TestCrossMonthUpdate_OldPeriodLocked verifies that moving an invoice OUT of a locked period is blocked.
func TestCrossMonthUpdate_OldPeriodLocked(t *testing.T) {
	repo := newTestRepo(t)

	// Insert in Jan (will be locked).
	if _, err := repo.Insert(sampleRow("2026", "01", "x.pdf")); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := repo.LockPeriod("2026", "01"); err != nil {
		t.Fatalf("LockPeriod: %v", err)
	}

	// Try moving to Feb (unlocked) — old period is locked → block.
	newRow := sampleRow("2026", "02", "x.pdf")
	err := repo.Update("2026", "01", "x.pdf", newRow)
	if !errors.Is(err, ErrPeriodLocked) {
		t.Fatalf("cross-month Update with locked old period: got %v, want ErrPeriodLocked", err)
	}
}

// TestCrossMonthUpdate_NewPeriodLocked verifies that moving an invoice INTO a locked period is blocked.
func TestCrossMonthUpdate_NewPeriodLocked(t *testing.T) {
	repo := newTestRepo(t)

	// Insert in Jan (unlocked).
	if _, err := repo.Insert(sampleRow("2026", "01", "y.pdf")); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// Lock Feb (target period).
	if err := repo.LockPeriod("2026", "02"); err != nil {
		t.Fatalf("LockPeriod: %v", err)
	}

	// Try moving from Jan (unlocked) to Feb (locked) → block.
	newRow := sampleRow("2026", "02", "y.pdf")
	err := repo.Update("2026", "01", "y.pdf", newRow)
	if !errors.Is(err, ErrPeriodLocked) {
		t.Fatalf("cross-month Update with locked new period: got %v, want ErrPeriodLocked", err)
	}
}

// TestLockPeriod_AuditEntry verifies that LockPeriod writes an audit log entry.
func TestLockPeriod_AuditEntry(t *testing.T) {
	repo := newTestRepo(t)

	if err := repo.LockPeriod("2026", "06"); err != nil {
		t.Fatalf("LockPeriod: %v", err)
	}
	if err := repo.UnlockPeriod("2026", "06"); err != nil {
		t.Fatalf("UnlockPeriod: %v", err)
	}

	entries, err := repo.AuditLog(10)
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}

	foundLock, foundUnlock := false, false
	for _, e := range entries {
		if e.Aktion == "lock" && e.Schluessel == "2026-06" {
			foundLock = true
		}
		if e.Aktion == "unlock" && e.Schluessel == "2026-06" {
			foundUnlock = true
		}
	}
	if !foundLock {
		t.Error("expected audit entry for lock action")
	}
	if !foundUnlock {
		t.Error("expected audit entry for unlock action")
	}
}
