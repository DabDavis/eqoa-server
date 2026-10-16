package entity

import (
	"testing"

	"github.com/eqoa/server/internal/session"
)

// CLIENT reads an object channel only below 24 (VIClient::ProcessStateChannel:
// `type < 24` goes to ChannelEntity and its m_entities[24]). Channel 0 is the
// player's own, so the entities in view may use 1-23 and no more; an entity on
// 24-28 was invisible to a PS2.
func TestEntityChannelsStayBelow24(t *testing.T) {
	if last := MaxVisibleEntities + NPCChannelOffset; last > 23 {
		t.Fatalf("the last entity channel is %d; a PS2 reads 0-23", last)
	}
	if session.MaxServerObjects != 24 {
		t.Fatalf("MaxServerObjects = %d, want 24 (channels 0x00-0x17)", session.MaxServerObjects)
	}
	if MaxVisibleEntities+1+NPCChannelOffset > session.MaxServerObjects {
		t.Fatal("the visibility system addresses a channel the session never allocated")
	}
}
