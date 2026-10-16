package tcp

import (
	"encoding/binary"
	"log"
	"net"
)

// WHAT THE SD SAYS ABOUT WHERE WORLDS COME FROM — read, never rewritten.
//
// This file replaces repointSD, which used to rewrite the SD payload's
// world-download address as the file went out. That existed because the
// address was DATA inside the payload, baked at build time, so a built SD was
// correct for exactly one server. Rewriting it on the way out fixed the
// operator's side and created a worse problem on the console's: the console
// then held the server's address TWICE — here and in eqahosts.txt, which the
// game itself reads to find the server — written by two different mechanisms.
// When they disagreed the game played perfectly and no world ever arrived,
// silently, at both ends.
//
// And the second copy could not be corrected. The rewrite was IN PLACE, so a
// stale SD is byte-for-byte the same LENGTH as a current one, and the patcher
// re-downloads on SIZE. A card holding the wrong address kept it forever.
// EQOADIAG's first real card was exactly that: the SD pointing at a dev LAN
// while eqahosts.txt pointed at the live server.
//
// So the payload reads eqahosts.txt itself now (hdd_sd.S, _dl_addr), and this
// server writes nothing into the SD at all. What is left is worth keeping as a
// READ: three things can still be wrong, and every one of them is silent at the
// console.
//
//   - The payload is an OLD one, built before the change, still carrying a
//     baked address. Nothing repoints it any more, so it would send world
//     requests to whichever LAN it was built on.
//   - Its port and worlds_udp_port disagree. The port is still baked (it is the
//     protocol's, not the deployment's) and nothing on the console can discover
//     it.
//   - server_ip is a hostname. The game resolves one perfectly well; the
//     payload cannot — sceInetSendTo takes a raw u32 — so worlds would simply
//     never download.
//
// FORMAT. re-tools/sd_patch.py is the reference:
//
//	u32   magic 'SDP1' | u32 version | u32 arenaSize | u32 reserved
//	record[]            u32 addr + u32 size + data[size], padded to 4
//	u32,u32             0,0 terminator
//	section[]           relocations; not walked here
//
// The world-server block is one 24-byte record, `{0, ip, 0, 0, port, 'EQWA'}`,
// found by its MAGIC. It used to be found by SHAPE — a nonzero IP with zeros
// around it — which is exactly the heuristic that stops working the moment the
// IP is legitimately zero, which is now always.
//
// THE MAGIC IS AT +0x14, PAST THE ADDRESS BLOCK, and word 0 is zero. The first
// 16 bytes are the destination address sceInetSendTo ships to the IOP, so a
// magic in word 0 is a magic inside somebody else's struct: between 2026-08-13
// and 08-14 it made every send return short with nothing on the wire, and both
// world downloads and the technician's reply went silent. A payload with the
// magic at word 0 is that broken build — recognised here so it is named rather
// than served.
const (
	sdMagic       = "SDP1"
	sdHeaderLen   = 16
	sdSrvLen      = 24                 // {0, ip, 0, 0, port, magic}
	sdSrvLegacy   = 20                 // {0, ip, 0, 0, port} — pre-eqahosts
	sdSrvMagicOff = 20                 // and 0 in the broken 08-13 build
	sdSrvMagic    = uint32(0x41575145) // "EQWA", as the EE loads it
)

// checkSDAddressing reports, in the log, how the payload being served will
// reach this server — or why it will not. It returns nothing and changes
// nothing: every outcome is invisible at the console, so "it will not work"
// must not also be invisible here.
func (s *PatchServer) checkSDAddressing(data []byte) {
	srv, legacyIP, ok := findSDWorldServer(data)
	if !ok {
		return
	}
	if srv == nil {
		// Every SD built before the downloader existed, and any built without
		// --world-port, lands here. Not an error: that payload only ever reads
		// worlds already on the disk.
		log.Printf("SD: no world-download record — this payload cannot fetch " +
			"worlds, so a console reaches only what its hard disk already holds")
		return
	}

	// Word 0 is the first word of the address struct sceInetSendTo hands the
	// IOP, so a nonzero one is not a variant of this record — it is a send that
	// returns short with nothing on the wire, on every console, for both world
	// downloads and the technician's reply.
	if w0 := binary.LittleEndian.Uint32(srv); w0 != 0 {
		log.Printf("SD: THIS PAYLOAD CANNOT SEND. Its world-download record has "+
			"%08x in word 0, which sceInetSendTo ships to the IOP as the first "+
			"word of the destination address — every request fails silently "+
			"(ST_DLTX stays 0). Rebuild it: the magic belongs at +0x14, past "+
			"the block", w0)
		return
	}

	if legacyIP != 0 {
		log.Printf("SD: THIS PAYLOAD IS OLD. It carries a baked world-download "+
			"address, %s, and nothing rewrites it any more — consoles patched "+
			"from here would send their world requests there. Rebuild it with "+
			"`make_hdd_sd.py --world-port`, which reads the address out of "+
			"eqahosts.txt on the console instead", sdDottedQuad(legacyIP))
		return
	}

	port := binary.LittleEndian.Uint32(srv[16:])
	cfgPort := s.cfg.Patch.WorldsUDPPort
	switch {
	case cfgPort <= 0 || cfgPort > 65535:
		log.Printf("SD: world downloads are disabled here (worlds_udp_port=%d) "+
			"but the payload will still ask on port %d, and nothing will answer",
			cfgPort, port)
	case uint32(cfgPort) != port:
		log.Printf("SD: THE PORT DOES NOT MATCH. The payload asks on %d and this "+
			"server listens on %d (worlds_udp_port). The port is baked at build "+
			"time — rebuild with `make_hdd_sd.py --world-port %d`, or set "+
			"worlds_udp_port to %d", port, cfgPort, cfgPort, port)
	default:
		log.Printf("SD: worlds are fetched on port %d, at whatever address "+
			"eqahosts.txt names — which this server writes, so there is one "+
			"address on the card and it is this one", port)
	}

	// The payload has no resolver. The game does, so a named host is a working
	// configuration for everything EXCEPT world downloads, and that is the kind
	// of half-working that has to be said out loud.
	if ip := net.ParseIP(s.cfg.Server.ServerIP); ip == nil || ip.To4() == nil {
		log.Printf("SD: server_ip %q is not an IPv4 address, so eqahosts.txt "+
			"will not hold one either. The game resolves a name; the payload "+
			"cannot (sceInetSendTo takes a raw address), so no world will "+
			"download until server_ip is a dotted quad", s.cfg.Server.ServerIP)
	}
}

// findSDWorldServer returns the world-server record's DATA, the IP of a
// pre-eqahosts payload that still bakes one, and whether the file walked
// cleanly. A nil record with ok == true means the payload has no downloader.
func findSDWorldServer(data []byte) (rec []byte, legacyIP uint32, ok bool) {
	if len(data) < sdHeaderLen || string(data[:4]) != sdMagic {
		log.Printf("SD: not an SDP1 payload, serving as-is")
		return nil, 0, false
	}

	var found [][]byte
	for off := sdHeaderLen; off+8 <= len(data); {
		addr := binary.LittleEndian.Uint32(data[off:])
		size := int(binary.LittleEndian.Uint32(data[off+4:]))
		if addr == 0 && size == 0 {
			break // terminator; the relocation sections follow
		}
		body := off + 8
		if size < 0 || body+size > len(data) {
			log.Printf("SD: record at offset %d claims %d bytes, serving as-is", off, size)
			return nil, 0, false
		}
		if size == sdSrvLen || size == sdSrvLegacy {
			w := data[body : body+size]
			switch {
			case size == sdSrvLen && binary.LittleEndian.Uint32(w[sdSrvMagicOff:]) == sdSrvMagic:
				found = append(found, w)
			case size == sdSrvLegacy && binary.LittleEndian.Uint32(w) == sdSrvMagic:
				// The BROKEN 08-13 shape: the magic in word 0, which is the
				// IOP's. Matched so it is named, not skipped as "no downloader".
				found = append(found, w)
			case size == sdSrvLegacy && isLegacyWorldServer(w):
				// The OLD shape, which had no magic. Recognised on purpose:
				// serving one of these is the one regression this check exists
				// to catch, and it must not read as "no downloader".
				legacyIP = binary.LittleEndian.Uint32(w[4:])
				found = append(found, w)
			}
		}
		// Records are padded to 4. Written the long way because Go's % takes
		// the sign of the DIVIDEND, so the (-size)%4 idiom that is correct in
		// the Python reference walks BACKWARDS here.
		off = body + size + (4-size%4)%4
	}

	switch len(found) {
	case 0:
		return nil, 0, true
	case 1:
		return found[0], legacyIP, true
	default:
		log.Printf("SD: %d records look like the world-download block; refusing "+
			"to guess which one is live", len(found))
		return nil, 0, false
	}
}

// isLegacyWorldServer reports whether a 20-byte record has the shape a payload
// built before the eqahosts change used: `{0, ip != 0, 0, 0, port}`.
func isLegacyWorldServer(rec []byte) bool {
	w := func(i int) uint32 { return binary.LittleEndian.Uint32(rec[i*4:]) }
	return w(0) == 0 && w(2) == 0 && w(3) == 0 && w(1) != 0 && w(4) >= 1 && w(4) <= 65535
}

// sdDottedQuad renders a host-order address the way an operator wrote it.
func sdDottedQuad(v uint32) string {
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).String()
}
