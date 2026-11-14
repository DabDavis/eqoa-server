package gmweb

import (
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/handlers"
)

// The login announcement (petition 2120, the GM Jibwiz, 2026-11-14: "GM's need to be able to set an announce that
// displays at login for all players. This needs to load as the newest message in chat upon login, after all the
// other tickets closure entries and so on"): one a shard, table loginannounce, the line every player entering the
// world is sent last (handlers/loginannounce.go). The game's #announce sets the same row; each change is an audited
// row change, which revert undoes.

func (s *Server) registerAnnounceOps() {
	s.addOp(&Op{
		Name: "get_login_announce", Title: "The login announcement", Scope: scopeRead,
		Description: "The login announcement: the line every player entering the world is sent as the last line of " +
			"its login (after its petitions' answers, the line on reporting a bug and the boon's), who set it and " +
			"when. text is empty when none is set.",
		Input: obj(map[string]any{}),
		Handler: func(c *opCtx) (any, error) {
			a, err := c.s.d.DB.LoginAnnounce()
			if err != nil {
				return nil, err
			}
			out := map[string]any{"text": a.Text, "set_by": a.SetBy, "changed": a.Changed}
			if a.Text != "" {
				out["line"] = handlers.LoginAnnounceLine(a.Text)
			}
			return out, nil
		},
	})
	s.addOp(&Op{
		Name: "set_login_announce", Title: "Set the login announcement", Scope: scopeWrite,
		Description: "Set the login announcement, the line every player entering the world is sent last, " +
			"\"Announcement: \" and the text, in light violet; an empty text clears it. It counts from the next " +
			"login (no restart) and outlives restarts. The text keeps what a PS2 can show (letters, digits and its " +
			"keyboard's marks), one line, at most 400 characters: the answer says what was kept. The game's " +
			"#announce sets the same. Audited; revert undoes it.",
		Input: obj(map[string]any{"text": str("the announcement; empty clears it")}),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Text string }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			text := handlers.CleanLoginAnnounce(req.Text)
			if text == "" && strings.TrimSpace(req.Text) != "" {
				return nil, badRequest("nothing a PS2 can show is left of that text: use letters, digits and its keyboard's marks")
			}
			who := database.GMEdit{AccountID: c.a.AccountID, Username: c.a.Username, Via: c.a.Via, IP: c.a.IP}
			detail := "login announcement cleared"
			if text != "" {
				detail = "login announcement: " + text
			}
			old, err := c.s.d.DB.SetLoginAnnounceAudited(text, who, detail)
			if err != nil {
				return nil, err
			}
			log.Printf("GM %s (%s): %s", c.a.Username, c.a.Via, detail)
			out := map[string]any{"text": text, "previous": old.Text}
			if text != "" {
				out["line"] = handlers.LoginAnnounceLine(text)
			}
			return out, nil
		},
	})
}
