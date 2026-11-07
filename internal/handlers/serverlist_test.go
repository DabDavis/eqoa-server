package handlers

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/eqoa/server/internal/config"
)

// The shard list's count is ZIGZAG: CLIENT's VIShardInfo::Read (0x00BFC208)
// takes it with ReadSigned. The default list is one shard, which is the byte
// 0x02, the same byte the old two-entry list sent raw (and a PS2 read as one).
func TestServerListCountIsZigzag(t *testing.T) {
	h := NewServerHandler("192.0.2.7")
	sess := testSession()
	h.SendServerList(sess)

	msgs := sess.Queue().PeekPending()
	if len(msgs) != 1 {
		t.Fatalf("%d messages queued, want 1", len(msgs))
	}
	d := msgs[0].Data()
	if !bytes.HasPrefix(d, []byte{0xB3, 0x07, 0x02}) {
		t.Fatalf("server list starts % x, want b3 07 02 (opcode, zigzag 1)", d[:3])
	}
	// One entry: u32 length + 11 UTF-16 chars, recommended, endpoint, port,
	// IP and language, and nothing after it.
	if want := 3 + 4 + 2*len("Marr's Fist") + 1 + 2 + 2 + 4 + 1; len(d) != want {
		t.Fatalf("server list is %d bytes, want %d (exactly one entry)", len(d), want)
	}
}

// The list config names, as a PS2 reads it: a zigzag count, then each entry's
// name (u32 count, wide), recommended byte, the 64-bit id (endpoint, port, IP
// reversed) and region. Config says the names, the ports and which is
// recommended; CLIENT connects to the id whole, so each entry is its own address.
func TestServerListFromConfig(t *testing.T) {
	shards, bad := ShardList([]config.ShardConfig{
		{Name: "Heirloom Test Server", Port: 10070},
		{Name: "Live Server", Port: 10071, Recommended: true},
	}, "192.0.2.7", 10070)
	if len(bad) != 0 || len(shards) != 2 {
		t.Fatalf("shards %v, bad %v", shards, bad)
	}
	h := NewServerHandler("192.0.2.7")
	h.SetServers(shards)
	sess := testSession()
	h.SendServerList(sess)
	d := sess.Queue().PeekPending()[0].Data()
	if !bytes.HasPrefix(d, []byte{0xB3, 0x07, 0x04}) {
		t.Fatalf("server list starts % x, want b3 07 04 (opcode, zigzag 2)", d[:3])
	}
	at := 3
	read := func() (name string, rec byte, ep uint16, port uint16, ip [4]byte, region byte) {
		n := int(binary.LittleEndian.Uint32(d[at:]))
		at += 4
		for i := 0; i < n; i++ {
			name += string(rune(binary.LittleEndian.Uint16(d[at:])))
			at += 2
		}
		rec = d[at]
		at++
		ep = binary.LittleEndian.Uint16(d[at:])
		port = binary.LittleEndian.Uint16(d[at+2:])
		copy(ip[:], d[at+4:at+8])
		region = d[at+8]
		at += 9
		return
	}
	name, rec, ep, port, ip, region := read()
	if name != "Heirloom Test Server" || rec != 0 || ep != GameServerEndpoint || port != 10070 || ip != [4]byte{7, 2, 0, 192} || region != 0 {
		t.Errorf("first entry: %q %d %#x %d %v %d", name, rec, ep, port, ip, region)
	}
	name, rec, ep, port, ip, region = read()
	if name != "Live Server" || rec != 1 || ep != GameServerEndpoint || port != 10071 || ip != [4]byte{7, 2, 0, 192} || region != 0 {
		t.Errorf("second entry: %q %d %#x %d %v %d", name, rec, ep, port, ip, region)
	}
	if at != len(d) {
		t.Errorf("%d bytes left after two entries", len(d)-at)
	}
}

// No shards in config is this server alone under the old name, on its own game
// port; a line that cannot be sent is left out and said.
func TestShardListChecks(t *testing.T) {
	shards, bad := ShardList(nil, "192.0.2.7", 10099)
	if len(bad) != 0 || len(shards) != 1 || shards[0].Name != DefaultShardName || shards[0].Port != 10099 || shards[0].Recommended != 1 {
		t.Fatalf("the default: %v, bad %v", shards, bad)
	}
	long := "This Name Runs Past What CLIENT Keeps"
	shards, bad = ShardList([]config.ShardConfig{
		{Name: "", Port: 1},
		{Name: long, Port: 1},
		{Name: "Port", Port: 0},
		{Name: "Address", Port: 1, IP: "2001:db8::1"},
		{Name: "Fine", Port: 2, IP: "198.51.100.1"},
		{Name: "Fine", Port: 3},
	}, "192.0.2.7", 2)
	if len(bad) != 5 || len(shards) != 1 || shards[0].Name != "Fine" || !shards[0].IP.Equal(net.ParseIP("198.51.100.1")) {
		t.Fatalf("shards %v, bad %v", shards, bad)
	}
}
