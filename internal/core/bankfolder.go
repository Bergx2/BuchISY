package core

import (
	"os"
	"path/filepath"
)

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
//   - FolderName == ""  → backfill: assume data already lives in To (no move)
//   - FolderName == To  → aligned: ensure-exists only (no move)
//   - FolderName != To  → drifted: move From → To
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
	metaName := filepath.Base(StatementMetaPath(fromDir)) // "metadata.json"
	for _, e := range entries {
		src := filepath.Join(fromDir, e.Name())
		dst := filepath.Join(toDir, e.Name())
		if e.Name() == metaName {
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
