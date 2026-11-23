package handlers

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

const turkeyQuest = 109910
const envarArmor = 1000810
const stuffing = 1000811
const turkeyClub = 1000812

type westGateSample struct {
	x, y, z float32
	at      time.Time
}

// An outward crossing in the actual gateway. First sight, zoning, a roof,
// a long absence and appearing somewhere beyond the gate do not start a raid.
func (s *westGateSample) crosses(world uint8, x, y, z float32, now time.Time) bool {
	prev := *s
	if world != 0 || math.Abs(float64(y-54.1)) > 5 {
		*s = westGateSample{}
		return false
	}
	*s = westGateSample{x, y, z, now}
	const gate = 25220
	if prev.at.IsZero() || now.Sub(prev.at) > 4*time.Second || now.Before(prev.at) || prev.x < gate || x >= gate || math.Hypot(float64(x-prev.x), float64(z-prev.z)) > 48 {
		return false
	}
	crossZ := prev.z + (z-prev.z)*(gate-prev.x)/(x-prev.x)
	return crossZ >= 15738 && crossZ < 15782
}

func (r *SeasonRewards) armorSources(rows []database.SeasonReward) map[entity.EntityID]string {
	for _, row := range rows {
		if row.Item != envarArmor || !strings.EqualFold(row.Season, "Thanksgiving") {
			continue
		}
		cur, err := r.db.Query("SELECT spawn, name FROM thanksgiving_envar_trash")
		if err != nil {
			log.Printf("Thanksgiving armor sources: %v", err)
			return nil
		}
		defer cur.Close()
		ids := make(map[entity.EntityID]string)
		for cur.Next() {
			var id entity.EntityID
			var name string
			if err = cur.Scan(&id, &name); err != nil {
				log.Printf("Thanksgiving armor: %v", err)
				return nil
			}
			ids[id] = name
		}
		if err = cur.Err(); err != nil {
			log.Printf("Thanksgiving armor: %v", err)
			return nil
		}
		return ids
	}
	return nil
}

func (r *SeasonRewards) envarDrop(k entity.KillCredit) {
	if k.World != 1 || k.NPCSpawn == 0 || k.NPCID == 0 || len(k.Players) == 0 {
		return
	}
	r.mu.Lock()
	name, ok := r.envar[k.NPCSpawn]
	r.mu.Unlock()
	if !ok || !strings.EqualFold(name, k.NPCName) || r.roll() >= 0.20 {
		return
	}
	if pat := r.pattern(envarArmor); pat != nil {
		r.drop(k.NPCID, k.Died, database.CharInventoryEntry{StackLeft: 1, EquipLoc: -1, Pattern: *pat})
	}
}

// The database inserted the reward with payment and progress. Send that exact
// row to the client instead of granting a second copy; a relog reads it normally.
func (q *QuestEngine) showTurkeyClub(sess *session.Session, p *entity.Player) {
	if sess == nil || p == nil {
		return
	}
	l := sessionInvList(sess)
	if l == nil {
		return
	}
	rows, err := q.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		log.Printf("Turkey reward readback: %v", err)
		return
	}
	for _, item := range rows {
		if item.Pattern.PatternID != turkeyClub || item.Location != database.LocPack {
			continue
		}
		if l.indexOfItem(item.ItemID) < 0 {
			key, n := l.add(item.ItemID, turkeyClub, 1, 1)
			if n == 1 {
				queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(&item, key, readsCapes(sess)))
			}
		}
		return
	}
}

func expireTurkeyLists(sm *session.Manager) {
	if sm == nil {
		return
	}
	sm.ForEach(func(s *session.Session) bool {
		withItems(s, func() {
			for index, l := range []*invList{sessionInvList(s), sessionBankList(s)} {
				if l == nil {
					continue
				}
				for {
					l.mu.Lock()
					i, qty := -1, 0
					for j, rec := range l.recs {
						if rec.patternID == envarArmor {
							i, qty = j, rec.qty
							break
						}
					}
					l.mu.Unlock()
					if i < 0 || !l.took(i, qty) {
						break
					}
					if index == 0 {
						queueReliable(s, protocol.OpcodeRemoveInvItem, removeInvItemPayload(i, 0, qty))
					} else {
						queueReliable(s, protocol.OpcodeRemoveBankItem, bankTookPayload(i, qty))
					}
				}
			}
		})
		return true
	})
}

func (q *QuestEngine) thanksgivingOpen() bool {
	on, err := q.db.SeasonActive("Thanksgiving")
	return err == nil && on
}

func (q *QuestEngine) turkeyPatrolOpen(id uint32) bool {
	var available bool
	err := q.db.QueryRow("SELECT EXISTS(SELECT 1 FROM thanksgiving_open_patrol WHERE serverid=?)", id).Scan(&available)
	return err == nil && available
}

func (q *QuestEngine) turkeyReport(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep) {
	if qu.ID != turkeyQuest && st.Step == 2 && !q.turkeyPatrolOpen(p.CharServerID) {
		reported := *st
		reported.Says = "The royal purse is closed. I'll strike this patrol off the roster. You've done your part."
		st = &reported
	}
	if !q.thanksgivingOpen() {
		q.tellLocked(c, sess, st.Target, "The Thanksgiving muster has ended.")
		return
	}
	if qu.ID == turkeyQuest && st.Step == 1 {
		m, err := q.db.CharMarksOf(p.CharServerID)
		if err != nil || m.Marks < 35 {
			q.tellLocked(c, sess, st.Target, fmt.Sprintf("Thirty-five Heirloom Marks, counted together. You have %d. My field captains can put you to work.", m.Marks))
			return
		}
	}
	// Switching the club for Stuffing is committed by the same transaction as
	// progress. Keep RewardItems for the normal pack-space check preceding this.
	q.atomicHandInLocked(sess, p, c, qu, cq, st, nil, -1)
}
