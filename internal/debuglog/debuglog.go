// Package debuglog is the switch for the packet path's debug lines: every
// packet's and every state update's own lines in the log (the UDP receive
// loop, DRDP, the state channels, the visibility pass). With 20 players they
// were about 100 lines a second a player and 15% of the server's CPU
// (claude-notes/server-tick-profile-20261103.md), so they are off unless
// `server.packet_log` or a GM's `#packetlog on` turns them on. cmd/movereplay
// reads the "UDP received" and "UDP raw hex" lines: a log for it is written
// with them on.
package debuglog

import "sync/atomic"

var packets atomic.Bool

// Packets reports whether the packet path's debug lines go to the log.
func Packets() bool { return packets.Load() }

// SetPackets turns them on or off, at once, for every session.
func SetPackets(on bool) { packets.Store(on) }
