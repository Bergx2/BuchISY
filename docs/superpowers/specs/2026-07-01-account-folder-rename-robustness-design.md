# Account-Folder Rename Robustness

**Date:** 2026-07-01
**Status:** Approved (design)

## Problem

A Zahlungskonto's bank-statement storage lives at
`<StorageRoot>/<SanitizeFilename(account.Name)>/`. The account's **identity is
its name**, so renaming an account changes the derived folder path. When a user
renames an account in Settings, `persistBankAccounts` → `ensureAccountFolders()`
**creates a new empty folder** for the new name but never moves the old one, so
the statements are orphaned in the old folder and the account appears empty.

Observed: renaming `KSMSE …` → `KSKMSE …` (fixing a typo) orphaned 26 main-account
statements + 6 credit-card statements in `KSMSE …` folders while the app looked in
new empty `KSKMSE …` folders. A `renameAccountFolder(old,new)` helper already
exists but is wired **only** into the legacy default-account migration, not the
normal rename path.

Direct edits to `settings.json` (a workflow this user uses) rename the account
without any folder move at all.

## Goal

Renaming an account must never orphan its statements. Storage stays
**human-readable** (folders named after the account, browsable in the user's
Finance directory) but becomes **robust against name changes** — including
renames made by editing `settings.json` directly.

## Design: per-account folder pointer

`Name` is the *desired* state; a new field records where the data *actually*
lives. A reconcile step converges them.

### Data model

Add one field to `core.BankAccount` (`internal/core/types.go`):

```go
FolderName string `json:"folder_name,omitempty"` // folder currently holding this account's statements
```

`FolderName` is the sanitized folder name that currently holds the account's
statements. Empty means "not yet initialised" (pre-existing accounts on first
load after the upgrade).

### Reconcile (replaces the body of `ensureAccountFolders`)

`internal/core/` gains a **pure** decision helper so the logic is unit-testable
without the filesystem, and `ui/kontenview.go` performs the resulting moves.

Pure helper (new, in `internal/core`, e.g. `bankfolder.go`):

```go
// AccountFolderAction is one folder move/backfill the reconcile wants.
type AccountFolderAction struct {
    Index    int    // index into the accounts slice
    From     string // sanitized source folder ("" = none / no move, just ensure)
    To       string // sanitized destination folder = SanitizeFilename(Name)
    Backfill bool   // FolderName was empty → just record To, no move
}

// ReconcileAccountFolders returns, per account, the folder move needed so each
// account's on-disk folder matches its (possibly renamed) Name. folderExists
// reports whether a sanitized folder currently exists under the storage root.
func ReconcileAccountFolders(accounts []BankAccount, folderExists func(string) bool) []AccountFolderAction
```

Rules per account (`want := SanitizeFilename(Name)`; skip empty names):

- `FolderName == ""` → **Backfill**: record `To = want`, no move.
- `FolderName == want` → no action (ensure-exists only).
- `FolderName != want` → **Move**: `From = FolderName`, `To = want`.

The UI (`ensureAccountFolders`) then, for each action:
1. If `From != "" && From != To` → move `<root>/<From>` → `<root>/<To>`
   (simple rename, or fold-in merge if the destination already exists; no-op if
   the source is missing).
2. `os.MkdirAll(<root>/<To>)`.
3. Set `account.FolderName = To`.
4. After processing all accounts, if any `FolderName` changed → persist settings
   via `settingsMgr.Save`.

`From`/`To` are already **sanitized folder names**, so the move works directly on
folder paths — not on account names. The current `renameAccountFolder(oldName,
newName)` takes account names and sanitizes internally; refactor its move/merge
body into a path-based helper `moveStatementFolder(fromDir, toDir string)` and
have `renameAccountFolder` call it (so the legacy migration keeps working).
`moveStatementFolder` keeps the existing merge/collision handling
(`mergeStatementMetadata`, skip file collisions, remove emptied source).

### What does NOT change

`statementFolder(name)` and its ~15 callers are untouched: after reconcile the
on-disk folder always equals `SanitizeFilename(Name)`, so name-derived lookups
stay correct. `renameAccountFolder` and `mergeStatementMetadata` are reused.

### Triggers

`ensureAccountFolders()` already runs **on startup** and **after every account
save** (`persistBankAccounts`). So:

- UI rename → folder moves immediately on save.
- `settings.json` name edit → folder moves on next startup.

Self-healing in both directions, no marker files, no separate immutable ID.

## Edge cases

- **Old folder already moved / missing** → `renameAccountFolder` no-ops; pointer
  still updated to `want`.
- **Destination folder already exists** (e.g. the empty folder a prior buggy
  rename created) → existing fold-in merge (move files, skip collisions, merge
  `metadata.json` with new-wins), then remove the emptied source.
- **Two accounts sanitised to the same folder** → merge risk. Names are expected
  unique; log a warning when a reconcile target collides with another account's
  folder and skip the move rather than mixing data.
- **`FolderName` backfill correctness**: existing accounts are already aligned
  (folder == name) after the manual repair, so backfilling `FolderName = want`
  is correct.

## Testing

- Unit-test `ReconcileAccountFolders` (pure): rename, backfill (empty pointer),
  no-op (aligned), and that a slice with one renamed account yields exactly one
  Move action with the right From/To.
- Integration-style test in `ui` (or a thin seam): a rename applied through the
  reconcile actually moves a temp folder and updates `FolderName`; a missing
  source is a no-op; a pre-existing destination folds in.

## Scope

- `internal/core/types.go` — add `FolderName`.
- `internal/core/bankfolder.go` (new) — `ReconcileAccountFolders` + `AccountFolderAction`.
- `internal/ui/kontenview.go` — rewrite `ensureAccountFolders` to drive the
  reconcile + persist; extract `moveStatementFolder(fromDir, toDir)` from
  `renameAccountFolder` (which now calls it) and reuse `mergeStatementMetadata`.
- Tests as above.

The legacy default-account migration (`migrateLegacyDefaultBankAccount`) stays.
Out of scope: enforcing globally-unique account names; opaque-ID storage.
