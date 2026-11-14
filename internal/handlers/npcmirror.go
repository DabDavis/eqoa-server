package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// npcmirror.go — the owner, 2026-11-14: "./copy should work for jib also on
// live server, also his placements and copies should show on heirloom also".
// On a shard whose config names the accounts (world.mirror_npc_edits_accounts)
// and the other shard's database (world.mirror_npc_db), an NPC row edit by a
// GM of one of those accounts is written there too (database/npcmirror.go):
//
//   - /place, /clone and /copy: the new row's copy made there;
//   - /move (and /move undo), /roamer (and /clone's or /place's wander), /size:
//     the copy of a row mirrored before changed to match;
//   - /delete: the copy of a row mirrored before deleted.
//
// A row that was never mirrored (one that was there before, on both shards) is
// left alone there, whoever edits it here. /delete undo is not mirrored: the
// copy stays deleted there. A failure there never undoes or fails the command
// here: the GM is told ("Not mirrored to Heirloom: ...").
//
// The other shard's server reads each mirrored row into its running world
// within npcMirrorInboxEvery (watchNPCMirrorInbox, run by every server on its
// own database: a server whose database no mirror writes finds no inbox).

var npcMirrorCfg struct {
	mu       sync.RWMutex
	accounts map[string]bool
	path     string
}

// SetNPCMirror sets world.mirror_npc_edits_accounts (usernames, any case) and
// world.mirror_npc_db.
func SetNPCMirror(accounts []string, path string) {
	m := make(map[string]bool, len(accounts))
	for _, n := range accounts {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			m[n] = true
		}
	}
	npcMirrorCfg.mu.Lock()
	npcMirrorCfg.accounts, npcMirrorCfg.path = m, strings.TrimSpace(path)
	npcMirrorCfg.mu.Unlock()
	if len(m) > 0 && path != "" {
		log.Printf("NPC mirror: %d account(s)' NPC row edits written to %s too (world.mirror_npc_edits_accounts)", len(m), path)
	}
}

// npcMirrorPath is world.mirror_npc_db when username's edits mirror, else "".
func npcMirrorPath(username string) string {
	npcMirrorCfg.mu.RLock()
	defer npcMirrorCfg.mu.RUnlock()
	if npcMirrorCfg.path == "" || !npcMirrorCfg.accounts[strings.ToLower(strings.TrimSpace(username))] {
		return ""
	}
	return npcMirrorCfg.path
}

// npcMirrorShard is how a GM is told of the other shard.
const npcMirrorShard = "Heirloom"

// mirrorNPC writes an edit of row id here to the other shard when sess's
// account mirrors (the file's doc); src is the row a new one was copied from,
// cols the npcs columns an update copies. It returns what to add to the GM's
// reply: "" when nothing is mirrored (not such an account, a row never
// mirrored), else what was done there, or why not.
func (h *ChatHandler) mirrorNPC(sess *session.Session, kind string, id, src int64, cols []string, detail string) string {
	if h.camps.db == nil || sess == nil || sess.AccountID() == 0 {
		return ""
	}
	var username string
	if err := h.camps.db.QueryRow(`SELECT username FROM accountinfo WHERE accountid = ?`, sess.AccountID()).Scan(&username); err != nil {
		return ""
	}
	path := npcMirrorPath(username)
	if path == "" {
		return ""
	}
	fail := func(err error) string {
		log.Printf("NPC mirror: %s's %s of row %d not mirrored to %s: %v", username, kind, id, path, err)
		return fmt.Sprintf(" Not mirrored to %s: %v.", npcMirrorShard, err)
	}
	if h.camps.db.SameFile(path) {
		return fail(fmt.Errorf("world.mirror_npc_db is this server's own database"))
	}
	peer, err := database.OpenNPCMirrorPeer(path)
	if err != nil {
		return fail(err)
	}
	defer peer.Close()
	m, err := h.camps.db.MirrorNPCEdit(peer, database.NPCMirrorEdit{Kind: kind, FromID: id, Source: src, Cols: cols,
		Detail: detail, Who: gmEditOf(sess)})
	if err != nil {
		return fail(err)
	}
	if !m.Mapped {
		return ""
	}
	log.Printf("NPC mirror: %s's %s of row %d mirrored to %s as row %d: %v", username, kind, id, path, m.ToID, m.Tables)
	switch {
	case kind == database.MirrorDelete:
		return fmt.Sprintf(" Its %s copy (row %d) deleted too.", npcMirrorShard, m.ToID)
	case kind == database.MirrorCreate:
		return fmt.Sprintf(" Mirrored to %s as row %d.", npcMirrorShard, m.ToID)
	case len(m.Tables) > 0:
		return fmt.Sprintf(" Its %s copy (row %d) changed too.", npcMirrorShard, m.ToID)
	}
	return ""
}

// npcMirrorInboxEvery is how often a server reads its mirror inbox.
const npcMirrorInboxEvery = 3 * time.Second

// watchNPCMirrorInbox reads the database's mirror inbox every
// npcMirrorInboxEvery, for ever, putting each row another shard's mirror wrote
// into the running world (applyNPCMirrorInbox).
func (h *ChatHandler) watchNPCMirrorInbox() {
	go func() {
		t := time.NewTicker(npcMirrorInboxEvery)
		defer t.Stop()
		for range t.C {
			h.applyNPCMirrorInbox()
		}
	}()
}

// applyNPCMirrorInbox puts the rows of the mirror inbox's lines into the
// running world as their rows are now: one gone out of it, one changed or new
// made again, the tables of their extras read again. It returns how many lines.
func (h *ChatHandler) applyNPCMirrorInbox() int {
	if h.camps.db == nil || h.gameWorld == nil {
		return 0
	}
	notes, err := h.camps.db.TakeNPCMirrorInbox()
	if err != nil {
		log.Printf("NPC mirror inbox: %v", err)
		return 0
	}
	tables := map[string]bool{}
	for _, n := range notes {
		for _, t := range n.Tables {
			if t != "npcs" && t != "npcpaths" {
				tables[t] = true
			}
		}
	}
	var list []string
	for t := range tables {
		list = append(list, t)
	}
	h.reloadNPCExtras(list)
	for _, n := range notes {
		id := entity.EntityID(n.Row)
		if n.Gone {
			h.gameWorld.RemoveNPC(id)
			h.gameWorld.ClearNPCPath(id)
			log.Printf("NPC mirror inbox: row %d deleted by another shard's mirror, out of the world", n.Row)
			continue
		}
		if _, err := h.upsertRowNPC(n.Row); err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				log.Printf("NPC mirror inbox: row %d not put into the world: %v", n.Row, err)
			}
			continue
		}
		log.Printf("NPC mirror inbox: row %d from another shard's mirror put into the world (%v)", n.Row, n.Tables)
	}
	return len(notes)
}

// npcPlaceColumns are the npcs columns a /move changes, which its mirror copies.
var npcPlaceColumns = []string{"x_coord", "y_coord", "z_coord", "facing", "world", "dt_kill"}
