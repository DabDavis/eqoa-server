package handlers

import (
	"bytes"
	"encoding/binary"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/ground"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

// Exercise the actual menu packet, recipient's consent box and both world loads.
func TestNeriakInstancedDuelProtocol(t *testing.T) {
	dir := os.Getenv("EQOA_NERIAK_GROUND")
	if dir == "" {
		t.Skip("set EQOA_NERIAK_GROUND to the baked arena")
	}
	env, err := ground.OpenEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := newChatWorld(t)
	c.gw.SetEnv(env)
	store := ServerWorldList()
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 16, Name: "Neriak Duel Arena", Zones: []worlds.RecipeZone{{From: "TUNARIA.ESF", Zone: 45}}}}})
	SetupDuelArena(c.gw)
	t.Cleanup(func() { c.gw.SetDuelArena(nil); SetServerWorlds(store) })
	sa, a := c.join(1, "Alice", 100, 100, 0)
	sb, b := c.join(2, "Bob", 104, 100, 0)
	a.DuelWindow, b.DuelWindow = true, true
	sa.SetServerWorlds([]uint8{entity.DuelArenaWorld})
	h := NewDuelHandler(c.gw)
	request := binary.LittleEndian.AppendUint32([]byte{duelUpInstanced, 0, 0, 0, 0}, c.gw.IDSeenBy(sa, b))
	challenge := func() {
		t.Helper()
		if err := h.HandleDuelRequest(sa, incoming(protocol.OpcodeDuelRequest, request)); err != nil {
			t.Fatal(err)
		}
	}
	challenge()
	if id, _ := c.gw.DuelOf(a); id != 0 {
		t.Fatal("client without the room received an arena invitation")
	}
	sb.SetServerWorlds([]uint8{entity.DuelArenaWorld})
	challenge()
	id, _ := c.gw.DuelOf(a)
	want := entity.BuildDuelBox(entity.DuelBoxID(id), "Alice challenges you to an instanced duel in Neriak. Enter the arena?", []string{"Accept", "Decline"})
	if got := queued(sb, protocol.OpcodeQuestPopupMulti); id == 0 || len(got) != 1 || !bytes.Equal(got[0], want) || a.World != 0 || b.World != 0 {
		t.Fatal("the consent box did not precede both teleports")
	}
	answer := append(binary.LittleEndian.AppendUint32(nil, entity.DuelBoxID(id)), 0)
	ih := &InteractHandler{gameWorld: c.gw}
	if err := ih.HandleDialogueOption(sb, incoming(protocol.OpcodeDialogueBoxOption, answer)); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*session.Session{sa, sb} {
		packets := queued(s, protocol.OpcodeTeleport)
		if len(packets) != 1 || packets[0][2] != entity.DuelArenaWorld || LoadableWorlds(s)(entity.DuelArenaWorld) {
			t.Fatal("private entry or teleport", packets)
		}
		loader := NewWorldLoadHandler(c.gw)
		if err := loader.HandleEnableChannel(s, incoming(protocol.OpcodeEnableChannel, []byte{0})); err != nil {
			t.Fatal(err)
		}
	}
	if a.InstanceKey() != id || !entity.SamePlace(a, b) {
		t.Fatal("accepted pair did not enter their instance")
	}
	h.HandleDuelRequest(sa, incoming(protocol.OpcodeDuelRequest, []byte{duelUpSurrender, 0, 0, 0, 0}))
	for _, s := range []*session.Session{sa, sb} {
		packets := queued(s, protocol.OpcodeTeleport)
		if len(packets) != 2 || packets[1][2] != 0 {
			t.Fatal("missing return teleport", packets)
		}
	}
	if a.X != 100 || b.X != 104 || a.InstanceKey() != 0 || b.InstanceKey() != 0 {
		t.Fatal("return after cancellation")
	}
}

func TestMoogieRecentArenaDuelsBoard(t *testing.T) {
	r := newPortalRig(t)
	npc := &entity.NPC{ServerID: 7777, Name: entity.MoogieName, X: 5001, Y: 50, Z: 5000, HP: 100, MaxHP: 100, NPCType: 0x80}
	r.gw.NPCs[npc.ServerID] = npc
	r.ih.recentDuels = func() ([][2]string, error) { return [][2]string{{"Newest", "Opponent"}, {"Older", "Rival"}}, nil }
	r.drain()
	r.talk(r.anna, npc)
	ask := r.msgs(r.anna)
	if len(ask) != 1 {
		t.Fatal(ask)
	}
	choices := strings.Split(ask[0], "|")[1:]
	index := slices.Index(choices, "Recent duels")
	if index < 0 || len(choices) > maxDialogOptions {
		t.Fatal(ask)
	}
	r.pick(r.anna, boxID(ask), byte(index))
	if got := r.msgs(r.anna); len(got) != 1 || got[0] != "box:Winner / Loser\nNewest/Opponent\nOlder/Rival" {
		t.Fatal(got)
	}
}
