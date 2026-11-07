package gmweb

// A panel on one shard of several (the user, 2026-11-07: "lets add another
// shard"). Each shard's panel is its own: its own database, audit log, sessions,
// authenticators and tokens. What differs on a shard whose accounts are another's
// (Deps.AccountsShared, auth.shared_db) is that its accountinfo is a copy
// (database.AccountMirror, overwritten within seconds), so nothing here changes an
// account: a level or a password set here would be gone at the next pull. Reading
// them, joining on them and signing in by them go on as ever. Testers go with the
// accounts: a tester is made on the owner's panel, for the owner's database.

// sharedAccountOps are the operations a copy of the accounts refuses.
var sharedAccountOps = []string{"create_account", "set_account_password", "ban_account", "set_account_level", "set_account_tester"}

const sharedAccountsWord = "this shard's accounts are a copy of the main shard's: change them on that server's GM panel"

// holdSharedAccounts makes what would change an account say where to go instead,
// and keeps accountinfo out of the generic editor.
func (s *Server) holdSharedAccounts() {
	if !s.d.AccountsShared {
		return
	}
	for _, name := range sharedAccountOps {
		if op := s.ops[name]; op != nil {
			op.Handler = func(*opCtx) (any, error) { return nil, forbidden("%s", sharedAccountsWord) }
		}
	}
}

// shardName is this server's line of the server list: the entry on its game
// port; empty when config names no shards.
func (s *Server) shardName() string {
	if s.d.Config == nil {
		return ""
	}
	for _, sh := range s.d.Config.Shards {
		if sh.Port == s.d.Config.Server.GamePort {
			return sh.Name
		}
	}
	return ""
}
