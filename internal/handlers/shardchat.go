package handlers

import (
	"fmt"
	"log"
	"time"
	"unicode/utf8"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/shardchat"
)

func (h *ChatHandler) addressedTell(sess *session.Session, from *entity.Player, realm shardchat.Realm, name string, body chatText) {
	if h.shards != nil && realm == h.shards.Home {
		h.tell(sess, from, name, body)
		return
	}
	body = body.clean(chatLineMost)
	if name == "" || body.text == "" {
		SendErrorMessage(sess, "Tell Failed")
		return
	}
	target := realm.Name(name)
	if h.shards == nil || realm != h.shards.Home.Other() {
		SendErrorMessage(sess, "Shard chat is unavailable.")
		return
	}
	answer, err := h.shards.Send(shardMessage("tell", from.Name, name, body))
	// The peer can take a moment to answer. An echo belongs only to the
	// character who sent the tell, even if the session went back to selection.
	if !sess.CharacterInWorld() || h.gameWorld.SessionPlayer(sess) != from {
		return
	}
	if err != nil {
		SendErrorMessage(sess, "Could not confirm delivery to "+target+".")
		return
	}
	if answer.Name == "" {
		SendErrorMessage(sess, fmt.Sprintf("%s is not connected.", target))
		return
	}
	target = realm.Name(answer.Name)
	h.tellLine(sess, from, body.made().wrap("You told "+target+", '", "'"))
	if answer.Away != "" {
		h.tellLine(sess, from, plainText(realm.Name(answer.Away)))
	}
	log.Printf("Tell: %s -> %s", from.Name, target)
}

// receiveShardChat is the authenticated peer's local delivery path. Source
// identity comes from the link; player text never runs through typedLine.
func (h *ChatHandler) receiveShardChat(m shardchat.Message) shardchat.Receipt {
	if h.gameWorld == nil || m.From == "" || h.shards.Home.Other() == "" {
		return shardchat.Receipt{}
	}
	sender := h.shards.Home.Other().Name(m.From)
	body := h.shardBody(sender, m)
	if body.text == "" {
		return shardchat.Receipt{}
	}
	switch m.Channel {
	case "shout":
		h.broadcastShout(sender, body, nil, nil)
	case "tell":
		to, sess := h.tellTarget(m.To)
		if sess != nil {
			h.receiveTell(sess, to, sender, body.made())
			away, _ := h.gameWorld.AFKReplyFromShard(to.ServerID, sender, time.Now())
			return shardchat.Receipt{Name: to.Name, Away: away}
		}
	}
	return shardchat.Receipt{}
}

func shardMessage(channel, from, to string, body chatText) shardchat.Message {
	m := shardchat.Message{Channel: channel, From: from, To: to, Text: body.text}
	for _, l := range body.links {
		if l.src != nil {
			m.Links = append(m.Links, shardchat.Link{Start: l.at, Name: l.src.name, Item: l.src.rec})
		}
	}
	return m
}

// Recreate clickable item links from the sending shard's item snapshots.
// Owner zero reserves these copies outside local characters' inventory rows.
func (h *ChatHandler) shardBody(sender string, m shardchat.Message) chatText {
	body := plainText(m.Text)
	if len(m.Links) == 0 {
		return body.clean(chatLineMost)
	}
	runes := []rune(m.Text)
	end := 0
	for _, l := range m.Links {
		n := utf8.RuneCountInString(l.Name)
		if len(body.links) >= itemLinksPerLine || n == 0 || l.Name != consoleLine(l.Name, itemNameMost) ||
			l.Start < end || l.Start > len(runes)-n || string(runes[l.Start:l.Start+n]) != l.Name {
			continue
		}
		src := &linkSource{links: h.links, who: sender, itemID: l.Item.ItemID, name: l.Name, rec: l.Item}
		body.links = append(body.links, textLink{at: l.Start, n: n, src: src})
		end = l.Start + n
	}
	return body.clean(chatLineMost)
}
