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
