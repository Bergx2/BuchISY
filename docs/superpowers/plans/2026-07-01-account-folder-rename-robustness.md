# Account-Folder Rename Robustness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Renaming a Zahlungskonto must never orphan its bank-statement folder; the on-disk folder stays human-readable and realigns to the account name automatically.

**Architecture:** Give each `BankAccount` a `FolderName` pointer recording where its statements actually live. A pure `ReconcileAccountFolders` decides, per account, whether the folder must move to match a renamed account. `ensureAccountFolders` runs the decision + `core.MoveStatementFolder` on startup and after every account save, converging folder ⇄ name in both directions (UI rename and direct `settings.json` edits).

**Tech Stack:** Go 1.25, standard library only. Tests via `go test`.

## Global Constraints

- Go standard library only; no new dependencies.
- Statement folders remain named `<StorageRoot>/<SanitizeFilename(account.Name)>/` (human-readable). `statementFolder(name)` and its callers stay unchanged.
- Metadata file is `metadata.json` (see `core.StatementMetaPath`). Merge semantics: existing destination entries win.
- Decimal/format/other app conventions unaffected (no behavioural change to parsing).

---

### Task 1: Core — `FolderName` field + pure `ReconcileAccountFolders`

**Files:**
- Modify: `internal/core/types.go` (BankAccount struct, ~line 86)
- Create: `internal/core/bankfolder.go`
- Test: `internal/core/bankfolder_test.go`

**Interfaces:**
- Produces:
  - `BankAccount.FolderName string` (json `folder_name,omitempty`)
  - `type AccountFolderAction struct { Index int; From string; To string; Move bool }`
  - `func ReconcileAccountFolders(accounts []BankAccount) []AccountFolderAction`

- [ ] **Step 1: Write the failing test**

Create `internal/core/bankfolder_test.go`:

```go
package core

import "testing"

func TestReconcileAccountFolders(t *testing.T) {
	accts := []BankAccount{
		{Name: "KSKMSE ...0712 Sparkasse", FolderName: "KSMSE ...0712 Sparkasse"}, // drifted → move
		{Name: "Wise ...9503", FolderName: "Wise ...9503"},                          // aligned → ensure only
		{Name: "Barkasse"},                                                          // empty pointer → backfill
		{Name: ""},                                                                  // no name → skipped
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core -run TestReconcileAccountFolders -v`
Expected: FAIL — `undefined: ReconcileAccountFolders` (and `BankAccount.FolderName`).

- [ ] **Step 3: Add the `FolderName` field**

In `internal/core/types.go`, inside `type BankAccount struct`, add after `IsCreditCard`:

```go
	FolderName string `json:"folder_name,omitempty"` // folder currently holding this account's statements ("" = uninitialised)
```

- [ ] **Step 4: Implement `ReconcileAccountFolders`**

Create `internal/core/bankfolder.go`:

```go
package core

// AccountFolderAction is the folder reconcile needed for one account so its
// on-disk statement folder matches its (possibly renamed) Name.
type AccountFolderAction struct {
	Index int    // index into the accounts slice passed to ReconcileAccountFolders
	From  string // current sanitized folder to move away from ("" when no move)
	To    string // desired sanitized folder = SanitizeFilename(Name)
	Move  bool   // true → move From → To before ensuring To exists
}

// ReconcileAccountFolders decides, per account, how to align its on-disk
// statement folder with its Name. Name is the desired state; FolderName records
// where the data currently lives:
//
//   - FolderName == ""      → backfill: assume data already lives in To (no move)
//   - FolderName == To      → aligned: ensure-exists only (no move)
//   - FolderName != To      → drifted: move From → To
//
// Accounts with an empty Name are skipped. The caller performs the moves and
// writes To back into each account's FolderName.
func ReconcileAccountFolders(accounts []BankAccount) []AccountFolderAction {
	var actions []AccountFolderAction
	for i, acc := range accounts {
		if acc.Name == "" {
			continue
		}
		act := AccountFolderAction{Index: i, To: SanitizeFilename(acc.Name)}
		if acc.FolderName != "" && acc.FolderName != act.To {
			act.From = acc.FolderName
			act.Move = true
		}
		actions = append(actions, act)
	}
	return actions
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/core -run TestReconcileAccountFolders -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/core/types.go internal/core/bankfolder.go internal/core/bankfolder_test.go
git commit -m "Account folders: add FolderName pointer + ReconcileAccountFolders"
```

---

### Task 2: Core — `MoveStatementFolder` (move/merge)

**Files:**
- Modify: `internal/core/bankfolder.go`
- Test: `internal/core/bankfolder_test.go`

**Interfaces:**
- Consumes: `LoadStatementMeta`, `SaveStatementMeta`, `StatementMetadataMap`, `StatementMetaPath` (existing, `internal/core/statement_meta.go`).
- Produces: `func MoveStatementFolder(fromDir, toDir string) error`

- [ ] **Step 1: Write the failing test**

Append to `internal/core/bankfolder_test.go`:

```go
import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestMoveStatementFolder_SimpleRename(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "old")
	to := filepath.Join(root, "new")
	writeFile(t, filepath.Join(from, "a.pdf"), "x")
	writeFile(t, filepath.Join(from, "metadata.json"), `{"a.pdf":{"note":"n"}}`)

	if err := MoveStatementFolder(from, to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(to, "a.pdf")); err != nil {
		t.Errorf("a.pdf not moved: %v", err)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Errorf("source folder should be gone, err=%v", err)
	}
}

func TestMoveStatementFolder_MergeIntoExisting(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "old")
	to := filepath.Join(root, "new")
	writeFile(t, filepath.Join(from, "a.pdf"), "fromA")
	writeFile(t, filepath.Join(from, "shared.pdf"), "fromShared")
	writeFile(t, filepath.Join(from, "metadata.json"), `{"a.pdf":{"note":"fa"},"shared.pdf":{"note":"fs"}}`)
	writeFile(t, filepath.Join(to, "shared.pdf"), "toShared")
	writeFile(t, filepath.Join(to, "metadata.json"), `{"shared.pdf":{"note":"ts"}}`)

	if err := MoveStatementFolder(from, to); err != nil {
		t.Fatal(err)
	}
	// a.pdf folded in
	if b, _ := os.ReadFile(filepath.Join(to, "a.pdf")); string(b) != "fromA" {
		t.Errorf("a.pdf = %q, want fromA", b)
	}
	// collision keeps destination
	if b, _ := os.ReadFile(filepath.Join(to, "shared.pdf")); string(b) != "toShared" {
		t.Errorf("shared.pdf = %q, want toShared (destination wins)", b)
	}
	// metadata merged, destination wins on collision
	m, _ := LoadStatementMeta(to)
	if m["a.pdf"].Note != "fa" {
		t.Errorf("metadata a.pdf note = %q, want fa", m["a.pdf"].Note)
	}
	if m["shared.pdf"].Note != "ts" {
		t.Errorf("metadata shared.pdf note = %q, want ts (destination wins)", m["shared.pdf"].Note)
	}
}

func TestMoveStatementFolder_MissingSourceNoop(t *testing.T) {
	root := t.TempDir()
	if err := MoveStatementFolder(filepath.Join(root, "nope"), filepath.Join(root, "to")); err != nil {
		t.Errorf("missing source should be a no-op, got %v", err)
	}
}
```

Note: confirm `StatementMetadata` has a `Note` field (it does — `metadata.json` entries carry `"note"`). If the JSON tag differs, adjust the literals to a field that exists.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core -run TestMoveStatementFolder -v`
Expected: FAIL — `undefined: MoveStatementFolder`.

- [ ] **Step 3: Implement `MoveStatementFolder`**

Append to `internal/core/bankfolder.go` (add `"os"` and `"path/filepath"` to its imports):

```go
// MoveStatementFolder moves a statement folder fromDir → toDir. If toDir does
// not exist it is a plain rename; if it exists, files are folded in (skipping
// name collisions — the destination wins) and metadata.json maps are merged
// (existing destination entries win). A missing fromDir is a no-op; the emptied
// fromDir is removed when possible.
func MoveStatementFolder(fromDir, toDir string) error {
	if fromDir == "" || toDir == "" || fromDir == toDir {
		return nil
	}
	info, err := os.Stat(fromDir)
	if err != nil || !info.IsDir() {
		return nil // nothing to move
	}
	if _, err := os.Stat(toDir); err != nil {
		return os.Rename(fromDir, toDir) // destination absent → simple rename
	}
	entries, _ := os.ReadDir(fromDir)
	for _, e := range entries {
		src := filepath.Join(fromDir, e.Name())
		dst := filepath.Join(toDir, e.Name())
		if e.Name() == filepath.Base(StatementMetaPath(fromDir)) { // "metadata.json"
			if err := mergeStatementMeta(fromDir, toDir); err != nil {
				return err
			}
			_ = os.Remove(src)
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			continue // collision → keep destination
		}
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	_ = os.Remove(fromDir) // only succeeds if now empty
	return nil
}

// mergeStatementMeta folds fromDir/metadata.json into toDir/metadata.json.
// Existing toDir entries win on key collision.
func mergeStatementMeta(fromDir, toDir string) error {
	oldMap, err := LoadStatementMeta(fromDir)
	if err != nil || len(oldMap) == 0 {
		return nil
	}
	newMap, _ := LoadStatementMeta(toDir)
	if newMap == nil {
		newMap = StatementMetadataMap{}
	}
	for k, v := range oldMap {
		if _, exists := newMap[k]; !exists {
			newMap[k] = v
		}
	}
	return SaveStatementMeta(toDir, newMap)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/core -run TestMoveStatementFolder -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/core/bankfolder.go internal/core/bankfolder_test.go
git commit -m "Account folders: MoveStatementFolder (rename or fold-in merge)"
```

---

### Task 3: UI — reconcile in `ensureAccountFolders`, refactor `renameAccountFolder`

**Files:**
- Modify: `internal/ui/kontenview.go` (`ensureAccountFolders` ~389-410, `renameAccountFolder` ~444-493, `mergeStatementMetadata` ~495-515)

**Interfaces:**
- Consumes: `core.ReconcileAccountFolders`, `core.MoveStatementFolder` (Tasks 1-2).

- [ ] **Step 1: Refactor `renameAccountFolder` to delegate to core**

Replace the whole body of `renameAccountFolder` (keep the signature — the legacy migration at ~line 428 still calls it) with:

```go
// renameAccountFolder moves <Storage>/<oldName>/ to <Storage>/<newName>/,
// delegating the move/merge to core.MoveStatementFolder.
func (a *App) renameAccountFolder(oldName, newName string) {
	oldFolder := a.statementFolder(oldName)
	newFolder := a.statementFolder(newName)
	if oldFolder == "" || newFolder == "" || oldFolder == newFolder {
		return
	}
	if err := core.MoveStatementFolder(oldFolder, newFolder); err != nil && a.logger != nil {
		a.logger.Warn("Renaming %s → %s failed: %v", oldFolder, newFolder, err)
	}
}
```

Then delete the now-unused `mergeStatementMetadata` method (its logic now lives in `core.mergeStatementMeta`). If any other caller of `mergeStatementMetadata` exists, leave it and skip this deletion.

- [ ] **Step 2: Verify nothing else references the deleted method**

Run: `grep -rn "mergeStatementMetadata" internal/`
Expected: no matches (safe to have deleted it). If matches remain, restore the method.

- [ ] **Step 3: Rewrite `ensureAccountFolders` as a reconcile**

Replace the body of `ensureAccountFolders` with:

```go
// ensureAccountFolders realigns every Zahlungskonto's on-disk statement folder
// with its (possibly renamed) name, then makes sure the folder exists. The
// per-account FolderName pointer records where the data currently lives, so a
// rename — via the settings UI or a direct settings.json edit — moves the
// folder instead of orphaning it. Idempotent; runs on startup and after every
// account save.
func (a *App) ensureAccountFolders() {
	if a.settings.StorageRoot == "" {
		return
	}
	a.migrateLegacyDefaultBankAccount()

	root := a.settings.StorageRoot
	changed := false
	for _, act := range core.ReconcileAccountFolders(a.settings.BankAccounts) {
		toDir := filepath.Join(root, act.To)
		if act.Move {
			fromDir := filepath.Join(root, act.From)
			if err := core.MoveStatementFolder(fromDir, toDir); err != nil {
				if a.logger != nil {
					a.logger.Warn("Realign account folder %s → %s failed: %v", act.From, act.To, err)
				}
			} else if a.logger != nil {
				a.logger.Info("Realigned account folder: %s → %s", act.From, act.To)
			}
		}
		if err := os.MkdirAll(toDir, 0755); err != nil {
			if a.logger != nil {
				a.logger.Warn("Failed to create account folder %s: %v", toDir, err)
			}
			continue
		}
		a.flattenYearSubfolders(toDir)
		if a.settings.BankAccounts[act.Index].FolderName != act.To {
			a.settings.BankAccounts[act.Index].FolderName = act.To
			changed = true
		}
	}
	if changed && a.settingsMgr != nil {
		if err := a.settingsMgr.Save(a.settings); err != nil && a.logger != nil {
			a.logger.Warn("Persisting account folder pointers failed: %v", err)
		}
	}
}
```

- [ ] **Step 4: Build + vet + full test suite**

Run: `go build ./... && go vet ./internal/... && go test ./...`
Expected: build succeeds; vet clean (pre-existing `clipboard_windows.go` unsafe.Pointer note is unrelated); all tests PASS.

- [ ] **Step 5: Manual smoke test (glue verification)**

`ensureAccountFolders` depends on live `App` state (settings/logger/settingsMgr) and isn't unit-tested directly — the risky logic is covered by Tasks 1-2. Verify the glue by hand:

1. Close the app (`tasklist | grep -i buchisy` → none).
2. In a scratch storage root, create `Test Konto/` with a file, set a profile `bank_accounts` entry `{name:"Test Konto", folder_name:"Test Konto"}`, then rename it in settings.json to `{name:"Test Konto NEU", folder_name:"Test Konto"}`.
3. Launch the app; confirm `Test Konto NEU/` now holds the file and `folder_name` became `Test Konto NEU`, and `Test Konto/` is gone.

Alternatively rename an account via Settings → Konten in the running app and confirm the statements follow.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/kontenview.go
git commit -m "Account folders: reconcile folder to name on load/save (no more orphans on rename)"
```

---

## Notes for the implementer

- `SanitizeFilename` is idempotent for already-clean names, so `FolderName` (a sanitized string) round-trips through `statementFolder`. Tests use `SanitizeFilename(...)` in expectations rather than hard-coding, to stay independent of its internals.
- Do NOT change `statementFolder` or its ~15 callers; after reconcile the on-disk folder always equals `SanitizeFilename(Name)`.
- Two accounts that sanitise to the same folder would merge — names are assumed unique (pre-existing assumption). The fold-in merge keeps destination files/metadata, so no silent data loss, but it is not a supported configuration.
