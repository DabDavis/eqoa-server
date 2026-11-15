package gmweb

import (
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eqoa/server/internal/accountmail"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/mailer"
)

// The accounts' e-mail on the panel (the owner, 2026-11-15: every account
// needs a verified address; internal/accountmail). An address is its owner's
// alone to see, so the panel shows it masked (d***@g***.com) with its state:
// email_counts, email_list, email_get. email_resend mails the account's address
// its verify link again (the mail process's; no per-account wait for a GM, the
// address's day limit still holds). email_mark_verified is a GM's override for
// a player who cannot get the mail: the account counts as verified, audited
// (who, when, the note; never the address). Both refused on a shard whose
// accounts are a copy (shard.go).

const emailDataNote = "An account's e-mail address is its owner's: the panel shows it masked and never whole. Mark one " +
	"verified by hand only for a player who has shown the account is theirs and cannot receive the mail."

// emailJSON is an account's e-mail state as the panel shows it.
func emailJSON(e database.EmailState) map[string]any {
	masked := ""
	if e.Email != "" {
		masked = mailer.Mask(e.Email)
	}
	state := "none"
	switch {
	case e.Verified():
		state = "verified"
	case e.Email != "":
		state = "pending"
	}
	return map[string]any{"accountid": e.AccountID, "username": e.Username, "level": e.Level, "banned": e.Banned,
		"lastLogin": e.LastLogin, "email": masked, "hasEmail": e.Email != "", "state": state,
		"verified": e.Verified(), "verifiedAt": e.VerifiedAt, "verifiedBy": e.VerifiedBy}
}

func (s *Server) registerEmailOps() {
	s.addOp(&Op{
		Name: "email_counts", Title: "E-mail: counts", Scope: scopeRead,
		Description: "How many accounts have a verified e-mail address (by its link, or by a GM's override), one waiting " +
			"for its link to be opened (pending), or none; and the requirement's settings (mail.accounts, " +
			"mail.require_verified, mail.require_after).",
		Input: obj(map[string]any{}),
		Handler: func(c *opCtx) (any, error) {
			n, err := c.s.d.DB.AccountEmailCounts()
			if err != nil {
				return nil, err
			}
			out := map[string]any{"counts": n, "note": emailDataNote}
			if c.s.d.Config != nil {
				m := c.s.d.Config.Mail
				out["settings"] = map[string]any{"accounts": m.Accounts, "sending": m.Enabled,
					"requireVerified": m.RequireVerified, "requireAfter": m.RequireAfter, "page": accountmail.Short(m)}
			}
			return out, nil
		},
	})
	s.addOp(&Op{
		Name: "email_list", Title: "E-mail: accounts by state", Scope: scopeRead,
		Description: "Accounts by e-mail state: unverified (the default: pending or none), pending (an address whose link " +
			"is not opened yet), none, verified or all; by name. Each with its address masked, its state, when and how it " +
			"was verified (link, or GM <name>), its level and last login. " + emailDataNote,
		Input: obj(map[string]any{
			"filter": strEnum("which accounts", database.EmailStateFilters...),
			"limit":  integer("at most this many (default 50, at most 200)"),
			"offset": integer("accounts to skip"),
		}),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Filter        string
				Limit, Offset int
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			limit := req.Limit
			if limit <= 0 {
				limit = 50
			}
			l, n, err := c.s.d.DB.AccountEmails(req.Filter, min(limit, 200), req.Offset)
			if err != nil {
				return nil, badRequest("%v", err)
			}
			out := []map[string]any{}
			for _, e := range l {
				out = append(out, emailJSON(e))
			}
			counts, _ := c.s.d.DB.AccountEmailCounts()
			return map[string]any{"accounts": out, "matching": n, "counts": counts, "note": emailDataNote}, nil
		},
	})
	s.addOp(&Op{
		Name: "email_get", Title: "E-mail: one account", Scope: scopeRead,
		Description: "One account's e-mail state: its address masked, verified or not, when and how. " + emailDataNote,
		Input:       obj(map[string]any{"account": idOrName("the account, by name or accountid")}, "account"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Account any }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			acc, err := c.s.accountRef(req.Account)
			if err != nil {
				return nil, err
			}
			e, err := c.s.d.DB.AccountEmail(acc.ID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"email": emailJSON(e), "note": emailDataNote}, nil
		},
	})
	s.addOp(&Op{
		Name: "email_resend", Title: "E-mail: send the verify link again", Scope: scopeWrite,
		Description: "Mail the account's address its verify link again (good for 24 hours), for a player who lost it. " +
			"The address's limit of five a day holds; the account's own wait does not. Nothing is sent while the account " +
			"has no address or it is verified by its link already.",
		Input: obj(map[string]any{"account": idOrName("the account, by name or accountid")}, "account"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Account any }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			if c.s.d.AccountMail == nil {
				return nil, badRequest("e-mail is off on this server (mail.accounts)")
			}
			acc, err := c.s.accountRef(req.Account)
			if err != nil {
				return nil, err
			}
			masked, err := c.s.d.AccountMail.SendVerify(acc.ID, "", true)
			if err != nil {
				return nil, badRequest("%v", err)
			}
			log.Printf("GM %s (%s): sent %s's verify link again (to %s)", c.a.Username, c.a.Via, acc.Username, masked)
			if _, err := c.s.auditTx(c.s.d.DB, c.a, "account", "accountinfo", map[string]any{"accountid": acc.ID}, nil, nil,
				fmt.Sprintf("sent %s's e-mail verify link again", acc.Username)); err != nil {
				log.Printf("GM: the audit line of a resend: %v", err)
			}
			return map[string]any{"sent": true, "to": masked,
				"message": fmt.Sprintf("A verify link went to %s (%s): it works for 24 hours", acc.Username, masked)}, nil
		},
	})
	s.addOp(&Op{
		Name: "email_mark_verified", Title: "E-mail: mark verified by hand", Scope: scopeWrite, Destructive: true,
		Description: "A GM's override for a player who cannot receive the mail: the account counts as verified (it may " +
			"enter the world when a verified e-mail is required) without its link. Audited with your name and note. A " +
			"reset by e-mail still needs an address verified by its link; without one, their forgotten password is a GM's " +
			"reset (pwreset_start). Changing the address later clears it. " + emailDataNote,
		Input: obj(map[string]any{
			"account": idOrName("the account, by name or accountid"),
			"note":    str("how you confirmed the account is theirs, and why the mail cannot reach them (kept in the audit log)"),
		}, "account", "note"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Account any
				Note    string
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			note := strings.TrimSpace(req.Note)
			if note == "" {
				return nil, badRequest("say how you confirmed it is theirs: it is kept in the audit log")
			}
			if utf8.RuneCountInString(note) > 500 {
				return nil, badRequest("a note is at most 500 characters")
			}
			acc, err := c.s.accountRef(req.Account)
			if err != nil {
				return nil, err
			}
			if err := pwresetMayReset(c.a, acc); err != nil {
				return nil, err
			}
			e, audit, err := c.s.d.DB.MarkEmailVerifiedByGM(acc.ID, workWho(c.a), note, time.Now())
			if err != nil {
				return nil, badRequest("%v", err)
			}
			log.Printf("GM %s (%s): marked %s's e-mail verified by hand", c.a.Username, c.a.Via, acc.Username)
			return map[string]any{"email": emailJSON(e), "audit": audit,
				"message": fmt.Sprintf("%s counts as verified now (by %s)", acc.Username, e.VerifiedBy), "note": emailDataNote}, nil
		},
	})
}
