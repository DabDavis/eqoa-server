package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// siegeTestLayout is a castle in world 15: the defenders' camp in its courtyard by the King, the attackers' outside
// its gatehouse, two gates in a row on the way in, a round field.
func siegeTestLayout() CTFLayout {
	return CTFLayout{
		Game: ArenaSiege,
		Name: "Test Keep",
		Teams: [2]CTFTeamLayout{
			{Name: "the Keep's Guard", Short: "Guard", Camp: CTFSpot{X: 466, Y: 54, Z: 474}, Stand: CTFSpot{X: 466, Y: 54, Z: 474}},
			{Name: "the Besiegers", Short: "Besiegers", Camp: CTFSpot{X: 600, Y: 54, Z: 478, Facing: -1.5708},
				Stand: CTFSpot{X: 600, Y: 54, Z: 478}},
		},
		Siege: SiegeLayout{
			King: SiegeNPCLayout{Name: "King Zentar", Original: "King Zentar", At: CTFSpot{X: 467, Y: 54, Z: 460},
				Body: 0x70D898C6, Size: 1, Level: 60},
			Gates: []SiegeNPCLayout{
				{Name: "Outer Gate", Original: "Outer Gate", At: CTFSpot{X: 520, Y: 59.5, Z: 477, Facing: 1.5708}, Body: 0xAA243482, Size: 1, Level: 60},
				{Name: "Inner Gate", Original: "Inner Gate", At: CTFSpot{X: 495, Y: 59.5, Z: 477, Facing: 1.5708}, Body: 0xAA243482, Size: 1, Level: 60},
			},
		},
		Ring: CTFRing{X: 500, Z: 478, Radius: 170},
	}
}

func siegeTestSettings() CTFSettings {
	set := DefaultSiegeSettings()
	set.MinPerTeam = 1
	set.KingHP, set.GateHP = 5000, 2000
	return set
}

// newSiege is newCTF's rig (capture the flag on in world 11) with the siege on too, in world 15, and the siege's
// spawn-only originals standing out of the world (the King a human in plate, the gates the portcullis), every
// player's game holding both worlds.
func newSiege(t *testing.T, n int) *ctfRig {
	t.Helper()
	r := newCTF(t, n)
	r.w.SetArena(GameSiege, true, siegeTestSettings(), siegeTestLayout())
	r.w.mu.Lock()
	for i, o := range []*NPC{
		{ServerID: 910001, Name: "King Zentar", ModelID: 0x70D898C6, Level: 60, HP: 9, MaxHP: 9, World: 15, Size: 1, SpawnOnly: true},
		{ServerID: 910002, Name: "Outer Gate", ModelID: 0xAA243482, Level: 60, HP: 9, MaxHP: 9, World: 15, Size: 1, SpawnOnly: true},
		{ServerID: 910003, Name: "Inner Gate", ModelID: 0xAA243482, Level: 60, HP: 9, MaxHP: 9, World: 15, Size: 1, SpawnOnly: true},
	} {
		o.X, o.Z = 100+float32(i), 100
		r.w.putNPCLocked(o)
		r.w.SpatialIndex.Insert(o.ServerID, o.X, o.Z)
	}
	r.w.mu.Unlock()
	for _, s := range r.ss {
		s.SetServerWorlds([]uint8{11, 15})
	}
	return r
}

// startSiege signs every player up for the siege and starts it (the countdown, everyone landed, the prep's end).
// It returns the defenders and the attackers.
func (r *ctfRig) startSiege(t *testing.T) (def, att []*Player) {
	t.Helper()
	for _, s := range r.ss {
		if got := r.w.ArenaJoin(GameSiege, s, r.at(0)); len(got) == 0 || !strings.HasPrefix(got[len(got)-1], "You are signed up for the siege") {
			t.Fatalf("join said %q", got)
		}
	}
	set := r.w.ctfGameLocked(GameSiege).set
	r.tick(time.Second)
	r.tick(time.Second + set.Countdown)
	m := r.w.ctfGameLocked(GameSiege).m
	if m == nil || m.phase != ctfPrep || m.siege == nil {
		t.Fatal("no siege after the countdown")
	}
	r.arrive()
	r.tick(time.Second + set.Countdown + set.Prep + time.Second)
	if m.phase != ctfPlay {
		t.Fatalf("phase %d after the prep", m.phase)
	}
	for _, p := range r.ps {
		if p.ctf.team == SiegeDefenders {
			def = append(def, p)
		} else {
			att = append(att, p)
		}
	}
	return def, att
}

// siegeLive is the time startSiege made the siege live.
func (r *ctfRig) siegeLive() time.Duration {
	set := r.w.ctfGameLocked(GameSiege).set
	return time.Second + set.Countdown + set.Prep + time.Second
}

// siegeNPCs is the match's King and gates.
func (r *ctfRig) siegeNPCs(t *testing.T) (king *NPC, gates []*NPC) {
	t.Helper()
	sm := r.w.ctfGameLocked(GameSiege).m.siege
	king = r.w.NPCs[sm.king]
	for _, g := range sm.gates {
		gates = append(gates, r.w.NPCs[g.npc])
	}
	return king, gates
}

// told is the lines s was sent since the last call (chat), joined.
func told(s *session.Session) string {
	return strings.Join(duelLines(duelDrain(s)), " | ")
}

// The siege starts in its own world beside capture the flag: its teams sent to their camps, the King and the
// gates made as copies of their spawn-only originals with the settings' hit points, in the siege's world, its
// layout's places and names; capture the flag's arena untouched, its own match its own (ids apart). Joining one
// game takes a player off the other's list.
func TestSiegeStartsBesideCTF(t *testing.T) {
	r := newSiege(t, 4)
	// two sign up for capture the flag, then one of them for the siege: off the banner list, told
	r.w.CTFJoin(r.ss[0], r.at(0))
	got := r.w.ArenaJoin(GameSiege, r.ss[0], r.at(0))
	if len(got) != 2 || got[0] != "You are no longer signed up for capture the banner." {
		t.Fatalf("switching games said %q", got)
	}
	if r.w.CTFQueued(r.ss[0]) || !r.w.ArenaQueued(GameSiege, r.ss[0]) {
		t.Fatal("still on the banner list, or not on the siege's")
	}
	r.w.CTFJoin(r.ss[2], r.at(0))
	r.w.CTFJoin(r.ss[3], r.at(0))
	r.w.ArenaJoin(GameSiege, r.ss[1], r.at(0))
	r.tick(time.Second)
	r.tick(2*time.Second + DefaultCTFSettings().Countdown)
	ctf, siege := r.w.ctfGameLocked(GameCTF).m, r.w.ctfGameLocked(GameSiege).m
	if ctf == nil || siege == nil || ctf.id == siege.id {
		t.Fatalf("matches %v and %v", ctf, siege)
	}
	if r.w.ctfCur != GameCTF || r.w.ctf.lay.Name != "the Test Canyon" {
		t.Fatalf("the engine left on game %d (%q)", r.w.ctfCur, r.w.ctf.lay.Name)
	}
	if siege.flags[0] != nil || len(siege.powerUps) != 0 || ctf.siege != nil || ctf.flags[0] == nil {
		t.Fatal("each game's objective is in the other's match")
	}
	for _, tr := range r.arrive() {
		want := uint8(11)
		if tr.P == r.ps[0] || tr.P == r.ps[1] {
			want = 15
		}
		if tr.To.World != want {
			t.Fatalf("%s sent to world %d, want %d", tr.P.Name, tr.To.World, want)
		}
	}
	king, gates := r.siegeNPCs(t)
	if king == nil || len(gates) != 2 || gates[0] == nil || gates[1] == nil {
		t.Fatal("no King or gates")
	}
	if king.Name != "King Zentar" || king.MaxHP != 5000 || king.HP != 5000 || king.World != 15 || !king.Temporary || king.SpawnOnly ||
		king.CopyOf == 0 || king.X != 467 || king.siege == nil || !king.siege.king {
		t.Fatalf("the King: %+v", king)
	}
	if g := gates[0]; g.Name != "Outer Gate" || g.MaxHP != 2000 || uint32(g.ModelID) != 0xAA243482 || g.X != 520 || g.Facing != 1.5708 || g.siege == nil || g.siege.king {
		t.Fatalf("the outer gate: %+v", g)
	}
	if gates[1].Name != "Inner Gate" || gates[1].X != 495 {
		t.Fatalf("the inner gate: %+v", gates[1])
	}
	if r.ps[0].ctf.match != siege.id || !r.ps[0].ctf.siege || r.ps[2].ctf.match != ctf.id || r.ps[2].ctf.siege {
		t.Fatal("players in the wrong match")
	}
}

// Without its original in the world the King is made of the recipe's body, a creature to fight.
func TestSiegeFallsBackToTheRecipesBody(t *testing.T) {
	r := newSiege(t, 2)
	r.w.mu.Lock()
	r.w.dropNPCLocked(r.w.NPCs[910001])
	r.w.mu.Unlock()
	r.startSiege(t)
	king, _ := r.siegeNPCs(t)
	if king == nil || uint32(king.ModelID) != 0x70D898C6 || king.NPCType != 0 || king.Level != 60 || king.MaxHP != 5000 || king.siege == nil {
		t.Fatalf("the King made of the recipe: %+v", king)
	}
}

// Only the attackers may strike the King and the gates, and only once the siege is live: a defender, an outsider
// and an attacker in the prep are barred (a melee not engaged, a spell refused, an area passing them by: every rule
// of challengeBars), each told why; every client but a live attacker's sees them with the NPC bit.
func TestSiegeOnlyAttackersStrike(t *testing.T) {
	r := newSiege(t, 3) // the third stays out
	for _, s := range r.ss[:2] {
		r.w.ArenaJoin(GameSiege, s, r.at(0))
	}
	set := r.w.ctfGameLocked(GameSiege).set
	r.tick(time.Second)
	r.tick(time.Second + set.Countdown)
	r.arrive()
	var def, att *Player
	for _, p := range r.ps[:2] {
		if p.ctf.team == SiegeDefenders {
			def = p
		} else {
			att = p
		}
	}
	out := r.ps[2]
	king, gates := r.siegeNPCs(t)
	// in the prep, nobody
	if !challengeBars(king, att.ServerID) || challengeRefusal(gates[0], att.ServerID) != siegeLineNotLive {
		t.Fatalf("an attacker in the prep: barred %v, told %q", challengeBars(king, att.ServerID), challengeRefusal(gates[0], att.ServerID))
	}
	r.tick(r.siegeLive())
	if challengeBars(king, att.ServerID) || challengeBars(gates[1], att.ServerID) || challengeRefusal(king, att.ServerID) != "" {
		t.Fatal("a live attacker is barred")
	}
	if !challengeBars(king, def.ServerID) || challengeRefusal(king, def.ServerID) != "You defend King Zentar: only the attackers strike at it." {
		t.Fatalf("a defender: %q", challengeRefusal(king, def.ServerID))
	}
	if !challengeBars(gates[0], out.ServerID) || !strings.Contains(challengeRefusal(gates[0], out.ServerID), "only its attackers") {
		t.Fatalf("an outsider: %q", challengeRefusal(gates[0], out.ServerID))
	}
	view := func(p *Player, n *NPC) byte {
		buf := make([]byte, 162)
		r.w.mu.Lock()
		r.w.challengeViewLocked(r.sessOf(p), n, buf)
		r.w.mu.Unlock()
		return buf[146] & 0x80
	}
	if view(att, gates[0]) != 0 || view(def, gates[0]) == 0 || view(out, king) == 0 {
		t.Fatal("the NPC bit is not every client's but a live attacker's")
	}
	// melee: the defender's swing at the King is not engaged, the attacker's is
	vs := GetOrCreateVisibility(r.sessOf(def))
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = king.ServerID, 0x01400001
	va := GetOrCreateVisibility(r.sessOf(att))
	va.ChannelEntity[1], va.ChannelObjectID[1] = king.ServerID, 0x01400001
	if r.w.EngageAttack(r.sessOf(def), 0x01400001, Swing{Delay: time.Second, Reach: 4.5}, r.at(r.siegeLive()+time.Second)) {
		t.Fatal("a defender engaged the King")
	}
	if !r.w.EngageAttack(r.sessOf(att), 0x01400001, Swing{Delay: time.Second, Reach: 4.5}, r.at(r.siegeLive()+time.Second)) {
		t.Fatal("the attacker could not engage the King")
	}
}

// A gate never fights and holds no hate; the King hates his attackers alone (a defender's hate, an outsider's, none)
// and notices no one on sight; neither heals of itself.
func TestSiegeGatesNeverFight(t *testing.T) {
	r := newSiege(t, 2)
	def, att := r.startSiege(t)
	king, gates := r.siegeNPCs(t)
	r.w.mu.Lock()
	r.w.addThreatLocked(gates[0], att[0].ServerID, 500)
	r.w.addThreatLocked(king, def[0].ServerID, 500)
	threatens := r.w.threatensLocked(king, att[0])
	r.w.mu.Unlock()
	if len(gates[0].Threats) != 0 || gates[0].CurrentEnemy != 0 {
		t.Fatalf("a gate holds hate: %v", gates[0].Threats)
	}
	if len(king.Threats) != 0 {
		t.Fatalf("the King hates a defender: %v", king.Threats)
	}
	if threatens {
		t.Fatal("the King attacks on sight")
	}
	r.w.mu.Lock()
	r.w.addThreatLocked(king, att[0].ServerID, 500)
	r.w.mu.Unlock()
	if king.CurrentEnemy != att[0].ServerID {
		t.Fatalf("the King's enemy is %d, not his attacker", king.CurrentEnemy)
	}
	if !regenQuiet(king) || !regenQuiet(gates[1]) || !king.DuelSpawn() || !gates[0].SiegeSpawn() {
		t.Fatal("a siege's NPC heals of itself, or gives loot")
	}
	r.w.mu.Lock()
	wanders, _, _ := r.w.movementLocked(gates[0])
	r.w.mu.Unlock()
	if wanders {
		t.Fatal("a gate wanders")
	}
}

// A defender's beneficial spell reaches the King (as a pet's master's does a pet); an attacker's or an outsider's
// is refused, and a defender's harmful one is told why.
func TestSiegeDefendersHealTheKing(t *testing.T) {
	r := newSiege(t, 3)
	for _, s := range r.ss[:2] {
		r.w.ArenaJoin(GameSiege, s, r.at(0))
	}
	set := r.w.ctfGameLocked(GameSiege).set
	r.tick(time.Second)
	r.tick(time.Second + set.Countdown)
	r.arrive()
	r.tick(r.siegeLive())
	king, _ := r.siegeNPCs(t)
	heal := CastSpell{SpellID: 9001, Scope: 1, Name: "Minor Healing", Range: 100, Def: ParseSpellText("Target, Increase Hitpoints by 100")}
	nuke := CastSpell{SpellID: 9002, Scope: 1, Name: "Burst of Flame", Range: 100, Def: ParseSpellText("Target, Decrease Hitpoints by 100")}
	cast := func(p *Player, sp CastSpell) ([]EntityID, string) {
		s := r.sessOf(p)
		v := GetOrCreateVisibility(s)
		v.ChannelEntity[2], v.ChannelObjectID[2] = king.ServerID, 0x01400002
		p.X, p.Y, p.Z = king.X+3, king.Y, king.Z
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		return r.w.castTargetsLocked(s, p, &sp, 0x01400002)
	}
	var def, att *Player
	for _, p := range r.ps[:2] {
		if p.ctf.team == SiegeDefenders {
			def = p
		} else {
			att = p
		}
	}
	if ids, why := cast(def, heal); why != "" || len(ids) != 1 || ids[0] != king.ServerID {
		t.Fatalf("a defender's heal on the King: %v %q", ids, why)
	}
	if _, why := cast(att, heal); why != castNotFriendly {
		t.Fatalf("an attacker's heal on the King: %q", why)
	}
	if _, why := cast(r.ps[2], heal); why != castNotFriendly {
		t.Fatalf("an outsider's heal on the King: %q", why)
	}
	if _, why := cast(def, nuke); why != "You defend King Zentar: only the attackers strike at it." {
		t.Fatalf("a defender's nuke on the King: %q", why)
	}
	if ids, why := cast(att, nuke); why != "" || len(ids) != 1 {
		t.Fatalf("the attacker's nuke on the King: %v %q", ids, why)
	}
}

// A gate broken by an attacker leaves the world at the next tick (its body would still be solid), its fall told to
// the match with the last blow's name, credited; the other gate stands. The King slain wins the siege for the
// attackers: told to everyone, each player its count, and after the end's hold all go back beside the herald and
// the King's corpse is taken in.
func TestSiegeGatesFallThenTheKing(t *testing.T) {
	r := newSiege(t, 2)
	def, att := r.startSiege(t)
	king, gates := r.siegeNPCs(t)
	a := att[0]
	live := r.siegeLive()
	for _, s := range r.ss {
		duelDrain(s)
	}
	r.w.mu.Lock()
	r.w.addThreatLocked(gates[0], a.ServerID, 1999)   // no hate (a gate holds none), but the hit counted
	r.w.addThreatLocked(gates[0], def[0].ServerID, 5) // a defender's, barred: not counted
	gates[0].HP = 0
	r.w.killNPC(gates[0])
	r.w.mu.Unlock()
	if r.w.NPCs[gates[0].ServerID] == nil {
		t.Fatal("the gate left the world inside its killer's hands")
	}
	r.tick(live + time.Second)
	if r.w.NPCs[gates[0].ServerID] != nil {
		t.Fatal("a broken gate still stands in the world")
	}
	if r.w.NPCs[gates[1].ServerID] == nil {
		t.Fatal("the other gate went too")
	}
	if got := told(r.sessOf(def[0])); !strings.Contains(got, a.Name+" has broken the Outer Gate! The way is open.") {
		t.Fatalf("the defender was told %q", got)
	}
	if a.ctf.gates != 1 {
		t.Fatalf("%d gates credited", a.ctf.gates)
	}
	if got := r.w.ArenaStatus(GameSiege, r.sessOf(a), r.at(live+time.Second)); !strings.Contains(got[0], "1 of 2 gates stand, King Zentar at 100%") {
		t.Fatalf("status %q", got)
	}
	r.w.mu.Lock()
	r.w.addThreatLocked(king, a.ServerID, 5000)
	king.HP = 0
	r.w.killNPC(king)
	r.w.mu.Unlock()
	r.tick(live + 2*time.Second)
	m := r.w.ctfGameLocked(GameSiege).m
	if m.phase != ctfOver || m.winner != SiegeAttackers {
		t.Fatalf("phase %d, winner %d", m.phase, m.winner)
	}
	got := told(r.sessOf(def[0]))
	if !strings.Contains(got, a.Name+" has slain King Zentar: the Besiegers take Test Keep!") {
		t.Fatalf("the end was told %q", got)
	}
	if mine := told(r.sessOf(a)); !strings.Contains(mine, "Your siege: 1 gate broken, 0 kills, 0 deaths.") ||
		!strings.Contains(mine, "You struck King Zentar's last blow.") {
		t.Fatalf("the attacker's count: %q", mine)
	}
	if r.w.NPCs[gates[1].ServerID] != nil {
		t.Fatal("a standing gate outlived the siege")
	}
	if r.w.NPCs[king.ServerID] == nil {
		t.Fatal("the King's corpse went before everyone went back")
	}
	r.tick(live + 2*time.Second + r.w.ctfGameLocked(GameSiege).set.EndHold + time.Second)
	if r.w.ctfGameLocked(GameSiege).m != nil || r.w.NPCs[king.ServerID] != nil {
		t.Fatal("the siege or the King's corpse is still there after everyone went back")
	}
	sent := r.arrive()
	if len(sent) != 2 || sent[0].To.World != 0 {
		t.Fatalf("the way back: %v", sent)
	}
	if def[0].ctf.match != 0 || a.ctf.match != 0 {
		t.Fatal("still in the match")
	}
}

// At the time limit the defenders win, the King standing: told, his and the gates' NPCs taken in.
func TestSiegeTimeLimitDefendersWin(t *testing.T) {
	r := newSiege(t, 2)
	def, _ := r.startSiege(t)
	king, gates := r.siegeNPCs(t)
	for _, s := range r.ss {
		duelDrain(s)
	}
	set := r.w.ctfGameLocked(GameSiege).set
	r.tick(r.siegeLive() + set.TimeLimit + time.Second)
	m := r.w.ctfGameLocked(GameSiege).m
	if m.phase != ctfOver || m.winner != SiegeDefenders || m.how != "time" {
		t.Fatalf("phase %d, winner %d, how %q", m.phase, m.winner, m.how)
	}
	if got := told(r.sessOf(def[0])); !strings.Contains(got, "The Keep's Guard holds Test Keep: King Zentar lives!") {
		t.Fatalf("told %q", got)
	}
	if r.w.NPCs[king.ServerID] != nil || r.w.NPCs[gates[0].ServerID] != nil {
		t.Fatal("the King or a gate outlived the siege")
	}
}

// A GM runs it by hand: #siege start with one a side when the settings want two, #siege time shortens the match
// on (and the time runs out on it), #siege stop ends one with no victor, #siege teams caps the teams.
func TestSiegeGMControls(t *testing.T) {
	r := newSiege(t, 2)
	r.w.mu.Lock()
	r.w.ctfGameLocked(GameSiege).set.MinPerTeam = 2
	r.w.mu.Unlock()
	for _, s := range r.ss {
		r.w.ArenaJoin(GameSiege, s, r.at(0))
	}
	r.tick(time.Second)
	if !r.w.ctfGameLocked(GameSiege).startAt.IsZero() {
		t.Fatal("a countdown with one a side when two are wanted")
	}
	if got := r.w.ArenaStartNow(GameSiege, r.at(2*time.Second)); got != "Match 1 begins: 1 against 1." {
		t.Fatalf("start: %q", got)
	}
	if r.w.ctfGameLocked(GameSiege).set.MinPerTeam != 2 {
		t.Fatal("the start changed the settings")
	}
	if got := r.w.ArenaStartNow(GameSiege, r.at(2*time.Second)); !strings.HasPrefix(got, "A match is on already") {
		t.Fatalf("a second start: %q", got)
	}
	r.arrive()
	set := r.w.ctfGameLocked(GameSiege).set
	open := 2*time.Second + set.Prep + time.Second
	r.tick(open)
	if got := r.w.ArenaSetTime(GameSiege, time.Minute, r.at(open+time.Second)); got != "The time limit is 1 minute now (this match ends in 1 minute)." {
		t.Fatalf("time: %q", got)
	}
	r.tick(open + time.Minute + time.Second)
	if m := r.w.ctfGameLocked(GameSiege).m; m.phase != ctfOver || m.winner != SiegeDefenders {
		t.Fatal("the shortened time did not end it")
	}
	r.tick(open + time.Minute + time.Second + set.EndHold + time.Second)
	r.arrive()
	if got := r.w.ArenaStop(GameSiege, r.at(open+2*time.Minute)); got != arenaGMNoMatch {
		t.Fatalf("stop with none on: %q", got)
	}
	if got := r.w.ArenaSetTeamSize(GameSiege, 1); got != "Teams hold up to 1 now (a match begins at 1 a side)." {
		t.Fatalf("teams: %q", got)
	}
	// a second match, stopped
	for _, s := range r.ss {
		r.w.ArenaJoin(GameSiege, s, r.at(open+3*time.Minute))
	}
	r.w.ArenaStartNow(GameSiege, r.at(open+3*time.Minute))
	m := r.w.ctfGameLocked(GameSiege).m
	if got := r.w.ArenaStop(GameSiege, r.at(open+3*time.Minute+time.Second)); got != "Match 2 is stopped: no victor." || m.winner != -1 {
		t.Fatalf("stop: %q (winner %d)", got, m.winner)
	}
}

// A login saved in the siege's world starts beside the herald, as one saved in capture the flag's arena.
func TestSiegeReturnSpotForItsWorld(t *testing.T) {
	r := newSiege(t, 1)
	for _, w := range []uint8{11, 15} {
		if at, ok := r.w.ArenaReturnSpot(w); !ok || at.World != 0 {
			t.Fatalf("world %d: %v %v", w, at, ok)
		}
	}
	if _, ok := r.w.ArenaReturnSpot(3); ok {
		t.Fatal("world 3 is an arena")
	}
}
