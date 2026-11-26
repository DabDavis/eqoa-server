package accountmail

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/mailer"
)

type rig struct {
	t     *testing.T
	db    *database.SQLiteDB
	sent  *mailer.Memory
	svc   *Service
	clock time.Time
	ids   map[string]int
}

func newRig(t *testing.T) *rig {
	t.Helper()
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := &rig{t: t, db: db, sent: &mailer.Memory{}, clock: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC), ids: map[string]int{}}
	q := mailer.NewQueue(r.sent, 0, []time.Duration{time.Millisecond})
	r.svc = NewWithQueue(db, config.MailConfig{Accounts: true, BaseURL: "https://example.com"}, q)
	r.svc.Now = func() time.Time { return r.clock }
	t.Cleanup(r.svc.Close)
	store := database.NewSQLiteAccountStore(db)
	for _, n := range []string{"alice", "bob", "carol"} {
		hash, _ := auth.HashPassword("pw-"+n, 4)
		a, err := store.Create(n, hash)
		if err != nil {
			t.Fatal(err)
		}
		r.ids[n] = a.ID
	}
	return r
}

// mails waits for n messages and answers them.
func (r *rig) mails(n int) []mailer.Message {
	r.t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for len(r.sent.All()) < n && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got := r.sent.All()
	if len(got) != n {
		r.t.Fatalf("%d mails, want %d: %v", len(got), n, got)
	}
	return got
}

var reToken = regexp.MustCompile(`#(verify|reset)=([A-Za-z0-9_-]+)`)

func tokenOf(t *testing.T, m mailer.Message, kind string) string {
	t.Helper()
	g := reToken.FindStringSubmatch(m.Text)
	if g == nil || g[1] != kind {
		t.Fatalf("no %s link in %q", kind, m.Text)
	}
	return g[2]
}

func TestVerifyTokenLifecycle(t *testing.T) {
	r := newRig(t)
	id := r.ids["alice"]
	masked, err := r.svc.SetEmail(id, " Alice@Example.COM ", "192.0.2.1")
	if err != nil || masked != "a***@e***.com" {
		t.Fatalf("SetEmail: %q %v", masked, err)
	}
	m := r.mails(1)[0]
	if m.To != "alice@example.com" || !strings.Contains(m.Text, "https://example.com/eqoa/password#verify=") {
		t.Fatalf("the mail: %+v", m)
	}
	tok := tokenOf(t, m, "verify")
	// stored hashed: the token itself is nowhere in the database
	var n int
	r.db.QueryRow(`SELECT COUNT(*) FROM email_tokens WHERE token_hash = ? OR email = ?`, tok, tok).Scan(&n)
	if n != 0 {
		t.Fatal("the token is kept in the clear")
	}
	if st, _ := r.db.AccountEmail(id); st.Verified() || st.Email != "alice@example.com" {
		t.Fatalf("before the link: %+v", st)
	}
	// a wrong token, then the right one, then the right one again
	if _, err := r.svc.Verify(tok + "x"); !errors.Is(err, database.ErrEmailToken) {
		t.Fatalf("a wrong token: %v", err)
	}
	st, err := r.svc.Verify(tok)
	if err != nil || !st.Verified() || st.VerifiedBy != "link" {
		t.Fatalf("verify: %+v %v", st, err)
	}
	if _, err := r.svc.Verify(tok); !errors.Is(err, database.ErrEmailToken) {
		t.Fatalf("a used token: %v", err)
	}
	// a new address clears it until its own link is opened
	r.clock = r.clock.Add(time.Hour)
	if _, err := r.svc.SetEmail(id, "alice2@example.com", ""); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.db.AccountEmail(id); st.Verified() || st.Email != "alice2@example.com" {
		t.Fatalf("after a change: %+v", st)
	}
	tok2 := tokenOf(t, r.mails(2)[1], "verify")
	// a link to the old address dies with the change
	r.clock = r.clock.Add(3 * time.Minute)
	if _, err := r.svc.SetEmail(id, "alice3@example.com", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Verify(tok2); !errors.Is(err, database.ErrEmailToken) {
		t.Fatalf("a link to an address no longer the account's: %v", err)
	}
	// a link ends after 24 hours
	tok3 := tokenOf(t, r.mails(3)[2], "verify")
	r.clock = r.clock.Add(VerifyTTL + time.Second)
	if _, err := r.svc.Verify(tok3); !errors.Is(err, database.ErrEmailToken) {
		t.Fatalf("an ended link: %v", err)
	}
}

func TestVerifyLimits(t *testing.T) {
	r := newRig(t)
	id := r.ids["bob"]
	if _, err := r.svc.SetEmail(id, "bob@example.com", "192.0.2.2"); err != nil {
		t.Fatal(err)
	}
	// at once again: the gap
	if _, err := r.svc.SendVerify(id, "192.0.2.2", false); !IsLimit(err) {
		t.Fatalf("a resend at once: %v", err)
	}
	// a GM's resend has no gap
	if _, err := r.svc.SendVerify(id, "", true); err != nil {
		t.Fatalf("a GM's resend: %v", err)
	}
	for i := 0; i < VerifyPerDay-2; i++ {
		r.clock = r.clock.Add(SendGap)
		if _, err := r.svc.SendVerify(id, "192.0.2.2", false); err != nil {
			t.Fatalf("resend %d: %v", i, err)
		}
	}
	r.clock = r.clock.Add(SendGap)
	if _, err := r.svc.SendVerify(id, "192.0.2.2", false); !IsLimit(err) {
		t.Fatalf("past the day's links: %v", err)
	}
	// the address's own limit holds over another account, and a GM
	if _, err := r.svc.SetEmail(r.ids["carol"], "BOB@example.com", "192.0.2.3"); !IsLimit(err) {
		t.Fatalf("another account naming a mailed-out address: %v", err)
	}
	if _, err := r.svc.SendVerify(id, "", true); !IsLimit(err) {
		t.Fatalf("a GM past the address's limit: %v", err)
	}
	// the next day it is free again
	r.clock = r.clock.Add(25 * time.Hour)
	if _, err := r.svc.SendVerify(id, "192.0.2.2", false); err != nil {
		t.Fatalf("the next day: %v", err)
	}
	// a sender's address: PerHourFromAddr an hour, whatever the accounts
	r2 := newRig(t)
	for i := 0; i < PerHourFromAddr; i++ {
		if !r2.svc.allowFrom("198.51.100.7") {
			t.Fatalf("refused at %d", i)
		}
	}
	if r2.svc.allowFrom("198.51.100.7") {
		t.Fatal("past the hour's mails from one address")
	}
	// a bad address is the player's to fix
	var bad *BadAddress
	if _, err := r2.svc.SetEmail(r2.ids["alice"], "not an address", ""); !errors.As(err, &bad) {
		t.Fatalf("a bad address: %v", err)
	}
	if _, err := r2.svc.SendVerify(r2.ids["alice"], "", false); !errors.Is(err, ErrNoEmail) {
		t.Fatalf("no address: %v", err)
	}
}

func TestResetFlow(t *testing.T) {
	r := newRig(t)
	id := r.ids["alice"]
	st := func() database.EmailState { s, _ := r.db.AccountEmail(id); return s }
	// no address, then one not verified: no link
	if r.svc.SendReset(st(), "", "") {
		t.Fatal("a reset link to an account with no address")
	}
	r.svc.SetEmail(id, "alice@example.com", "")
	verifyTok := tokenOf(t, r.mails(1)[0], "verify")
	if r.svc.SendReset(st(), "", "") {
		t.Fatal("a reset link to an unverified address")
	}
	if _, err := r.svc.Verify(verifyTok); err != nil {
		t.Fatal(err)
	}
	// a typed address that is not the account's: nothing
	if r.svc.SendReset(st(), "someone@example.com", "") {
		t.Fatal("a reset link for another address")
	}
	if !r.svc.SendReset(st(), "ALICE@example.com", "192.0.2.1") {
		t.Fatal("no reset link")
	}
	tok := tokenOf(t, r.mails(2)[1], "reset")
	// the gap holds a second at once
	if r.svc.SendReset(st(), "", "") {
		t.Fatal("a second reset at once")
	}
	r.db.SetPasswordTemporary(id, true)
	hash, _ := auth.HashPassword("new-password", 4)
	got, err := r.svc.Reset(tok, hash)
	if err != nil || got.AccountID != id {
		t.Fatalf("reset: %+v %v", got, err)
	}
	var pw string
	var temp int
	r.db.QueryRow(`SELECT CAST(pwhash AS TEXT), pwtemp FROM accountinfo WHERE accountid = ?`, id).Scan(&pw, &temp)
	if auth.VerifyPassword(pw, "new-password") != auth.Accepted || temp != 0 {
		t.Fatalf("after the reset: temp %d", temp)
	}
	if _, err := r.svc.Reset(tok, hash); !errors.Is(err, database.ErrEmailToken) {
		t.Fatalf("a reset link used twice: %v", err)
	}
	// a reset link lives an hour
	r.clock = r.clock.Add(SendGap)
	if !r.svc.SendReset(st(), "", "") {
		t.Fatal("no second reset link")
	}
	tok2 := tokenOf(t, r.mails(3)[2], "reset")
	r.clock = r.clock.Add(ResetTTL + time.Second)
	if _, err := r.svc.Reset(tok2, hash); !errors.Is(err, database.ErrEmailToken) {
		t.Fatalf("an ended reset link: %v", err)
	}
	// ResetPerDay a day
	for i := 2; i < ResetPerDay; i++ {
		r.clock = r.clock.Add(SendGap)
		if !r.svc.SendReset(st(), "", "") {
			t.Fatalf("reset %d refused", i)
		}
	}
	r.clock = r.clock.Add(SendGap)
	if r.svc.SendReset(st(), "", "") {
		t.Fatal("past the day's resets")
	}
	// a verify link does not reset, a reset link does not verify
	if _, err := r.svc.Verify(tok2); !errors.Is(err, database.ErrEmailToken) {
		t.Fatalf("a reset link as a verify link: %v", err)
	}
}

func TestGMOverrideAndCounts(t *testing.T) {
	r := newRig(t)
	r.svc.SetEmail(r.ids["alice"], "alice@example.com", "")
	tok := tokenOf(t, r.mails(1)[0], "verify")
	r.svc.Verify(tok)
	r.svc.SetEmail(r.ids["bob"], "bob@example.com", "")
	e, audit, err := r.db.MarkEmailVerifiedByGM(r.ids["carol"], database.GMEdit{AccountID: r.ids["alice"], Via: "test"}, "named her characters", r.clock)
	if err != nil || audit == 0 || !e.Verified() || e.VerifiedBy != "GM alice" {
		t.Fatalf("the override: %+v %d %v", e, audit, err)
	}
	var detail string
	r.db.QueryRow(`SELECT detail FROM gm_audit WHERE id = ?`, audit).Scan(&detail)
	if !strings.Contains(detail, "carol") || !strings.Contains(detail, "named her characters") || strings.Contains(detail, "@") {
		t.Fatalf("the audit line: %q", detail)
	}
	if _, _, err := r.db.MarkEmailVerifiedByGM(r.ids["carol"], database.GMEdit{AccountID: r.ids["alice"]}, "again", r.clock); err == nil {
		t.Fatal("marked twice")
	}
	c, err := r.db.AccountEmailCounts()
	if err != nil || c.Accounts != 3 || c.Verified != 2 || c.ByGM != 1 || c.Pending != 1 || c.NoAddress != 0 {
		t.Fatalf("counts: %+v %v", c, err)
	}
	l, n, err := r.db.AccountEmails("unverified", 10, 0)
	if err != nil || n != 1 || l[0].Username != "bob" {
		t.Fatalf("unverified: %v %d %v", l, n, err)
	}
	// a GM override is no address to mail a reset to
	st, _ := r.db.AccountEmail(r.ids["carol"])
	if HasVerifiedEmail(st) {
		t.Fatal("an override with no address counts as a mailable address")
	}
}

func TestGateBeforeAndAfterTheDate(t *testing.T) {
	r := newRig(t)
	r.svc.SetEmail(r.ids["alice"], "alice@example.com", "")
	r.svc.Verify(tokenOf(t, r.mails(1)[0], "verify"))
	if g, err := NewGate(r.db, config.MailConfig{Accounts: true}); g != nil || err != nil {
		t.Fatalf("a gate with the requirement off: %v %v", g, err)
	}
	if _, err := NewGate(r.db, config.MailConfig{Accounts: true, BaseURL: "https://example.com", RequireVerified: true, RequireAfter: "next week"}); err == nil {
		t.Fatal("a bad date taken")
	}
	g, err := NewGate(r.db, config.MailConfig{Accounts: true, BaseURL: "https://example.com", RequireVerified: true, RequireAfter: "2026-12-01"})
	if err != nil || g == nil || g.Every != DefaultRemind || g.Page != "example.com/eqoa/password" {
		t.Fatalf("gate: %+v %v", g, err)
	}
	before := time.Date(2026, 11, 30, 23, 59, 0, 0, time.UTC)
	after := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	if g.Refusal(r.ids["bob"], before) != "" {
		t.Fatal("refused before the date")
	}
	if line := g.Reminder(r.ids["bob"], before); !strings.Contains(line, "https://example.com/eqoa/password#email") || !strings.Contains(line, "December 1, 2026") {
		t.Fatalf("the reminder: %q", line)
	}
	if g.Reminder(r.ids["alice"], before) != "" {
		t.Fatal("a verified account reminded")
	}
	why := g.Refusal(r.ids["bob"], after)
	if why == "" || len(why) > MaxRefusal || !strings.Contains(why, "example.com/eqoa/password") {
		t.Fatalf("the refusal: %q", why)
	}
	if g.Reminder(r.ids["bob"], after) != "" {
		t.Fatal("reminded past the date (refused instead)")
	}
	if g.Refusal(r.ids["alice"], after) != "" {
		t.Fatal("a verified account refused")
	}
	// a GM's override lets bob in
	r.db.MarkEmailVerifiedByGM(r.ids["bob"], database.GMEdit{AccountID: r.ids["alice"]}, "test", after)
	if g.Refusal(r.ids["bob"], after) != "" {
		t.Fatal("refused after a GM's override")
	}
	// with no date: reminders, never a refusal
	g2, _ := NewGate(r.db, config.MailConfig{Accounts: true, BaseURL: "https://example.com", RequireVerified: true})
	if g2.Refusal(r.ids["carol"], after.AddDate(5, 0, 0)) != "" || g2.Reminder(r.ids["carol"], after) == "" {
		t.Fatal("a gate with no date")
	}
	// a long page still fits STATION's reply
	if t2 := RefusalText(strings.Repeat("x", 80)); len(t2) > MaxRefusal {
		t.Fatalf("a refusal of %d", len(t2))
	}
}

// A shard's copy of the accounts carries the e-mail state (AccountMirror), so
// its gate holds its players as the owner's does.
func TestMirrorCarriesEmailState(t *testing.T) {
	owner := newRig(t)
	owner.svc.SetEmail(owner.ids["alice"], "alice@example.com", "")
	owner.svc.Verify(tokenOf(t, owner.mails(1)[0], "verify"))
	ownerPath := filepath.Join(t.TempDir(), "owner.db")
	// the owner's file as the shard reads it
	if _, err := owner.db.Exec(`VACUUM INTO ?`, ownerPath); err != nil {
		t.Fatal(err)
	}
	shard, err := database.NewSQLite(filepath.Join(t.TempDir(), "shard.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer shard.Close()
	m := database.NewAccountMirror(shard, ownerPath)
	if _, _, _, err := m.Pull(); err != nil {
		t.Fatal(err)
	}
	st, err := shard.AccountEmail(owner.ids["alice"])
	if err != nil || !st.Verified() || st.VerifiedBy != "link" || st.Email != "alice@example.com" {
		t.Fatalf("the shard's copy: %+v %v", st, err)
	}
	g, _ := NewGate(shard, config.MailConfig{Accounts: true, BaseURL: "https://example.com", RequireVerified: true, RequireAfter: "2026-11-01"})
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	if g.Refusal(owner.ids["alice"], now) != "" || g.Refusal(owner.ids["bob"], now) == "" {
		t.Fatal("the shard's gate")
	}
	// bob verified on the owner reaches the shard at its next pull
	owner.svc.SetEmail(owner.ids["bob"], "bob@example.com", "")
	owner.svc.Verify(tokenOf(t, owner.mails(2)[1], "verify"))
	ownerPath2 := filepath.Join(t.TempDir(), "owner2.db")
	owner.db.Exec(`VACUUM INTO ?`, ownerPath2)
	if _, _, _, err := database.NewAccountMirror(shard, ownerPath2).Pull(); err != nil {
		t.Fatal(err)
	}
	if g.Refusal(owner.ids["bob"], now) != "" {
		t.Fatal("bob's verification did not reach the shard")
	}
}

// No site and no sender are guessed: a server with e-mail on that names
// neither mails no link (to someone else's site) and has no gate to name a
// page, and Check says which setting is missing.
func TestNoDefaultSiteOrSender(t *testing.T) {
	if Filled(config.MailConfig{}).BaseURL != "" || Filled(config.MailConfig{}).From != "" {
		t.Fatal("Filled made up a site or a sender")
	}
	if Short(config.MailConfig{}) != "" {
		t.Fatal("Short named a page with no site")
	}
	if err := Check(config.MailConfig{}, true); err != nil {
		t.Fatalf("e-mail off refused: %v", err)
	}
	if err := Check(config.MailConfig{Accounts: true}, false); !errors.Is(err, ErrNoBaseURL) {
		t.Fatalf("no base_url: %v", err)
	}
	on := config.MailConfig{Accounts: true, Enabled: true, BaseURL: "https://example.com"}
	if err := Check(on, true); !errors.Is(err, ErrNoFrom) {
		t.Fatalf("SMTP with no from: %v", err)
	}
	if err := Check(on, false); err != nil {
		t.Fatalf("a process that does not send needs no from: %v", err)
	}
	on.From = "no-reply@example.com"
	if err := Check(on, true); err != nil {
		t.Fatal(err)
	}
	if _, err := NewGate(nil, config.MailConfig{Accounts: true, RequireVerified: true}); !errors.Is(err, ErrNoBaseURL) {
		t.Fatalf("a gate with no page: %v", err)
	}

	r := newRig(t)
	q := mailer.NewQueue(r.sent, 0, []time.Duration{time.Millisecond})
	bare := NewWithQueue(r.db, config.MailConfig{Accounts: true}, q)
	defer bare.Close()
	if bare.PageURL() != "" {
		t.Fatalf("PageURL %q with no site", bare.PageURL())
	}
	if _, err := bare.SetEmail(r.ids["alice"], "alice@example.com", ""); !errors.Is(err, ErrNoBaseURL) {
		t.Fatalf("a verify link with no site: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if n := len(r.sent.All()); n != 0 {
		t.Fatalf("%d mails sent with no site", n)
	}
}
