// Package shardchat carries player chat across the two game servers.
package shardchat

import (
	"strings"

	"github.com/eqoa/server/internal/database"
)

// Realm is the prefix players see on a name from the other server.
type Realm string

const (
	Vanilla Realm = "v"
	Sandbox Realm = "s"
)

// FromConfig maps gm.shard_id to the player's spelling. The panel's default is Heirloom.
func FromConfig(id string) Realm {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "", "heirloom", "sandbox":
		return Sandbox
	case "live", "vanilla":
		return Vanilla
	}
	return ""
}

func (r Realm) Other() Realm {
	switch r {
	case Vanilla:
		return Sandbox
	case Sandbox:
		return Vanilla
	}
	return ""
}

func (r Realm) Name(character string) string { return string(r) + "." + character }

// Address separates an explicit player address. An unqualified name stays local.
func Address(target string) (Realm, string) {
	prefix, name, dotted := strings.Cut(target, ".")
	if dotted {
		switch strings.ToLower(prefix) {
		case "v", "vanilla":
			return Vanilla, name
		case "s", "sandbox":
			return Sandbox, name
		}
	}
	return "", target
}

// Link carries the item as the sender's server saw it. Link IDs are local to a server.
type Link struct {
	Start int
	Name  string
	Item  database.CharInventoryEntry
}

type Message struct {
	Channel string // "shout" or "tell"
	From    string
	To      string `json:",omitempty"`
	Text    string
	Links   []Link `json:",omitempty"`
}

// A tell receipt names the character whose session was sent the tell. An empty
// name means nobody was online by that name. Away is their optional AFK line.
type Receipt struct {
	Name string
	Away string `json:",omitempty"`
}
