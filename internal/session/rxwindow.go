package session

// rxwindow.go: which of the client's reliable messages the server has taken,
// and what it acknowledges of them.
//
// CLIENT retires a message the moment an acknowledgement covers it
// (connection_record_guaranteed_ack: everything up to the ack, and the
// mask's), and never sends it again. The server acknowledged the NEWEST
// message it had taken, so a message lost on its way, with a later one
// through, was retired by the client unseen: a chat line, a target request
// (the con), an attack press gone for good. Now the ack is the newest taken
// with every one before it, as CLIENT's own (m_usRxSeqnum), and those taken
// past it go in the ack's mask (body flag 0x08, connection_get_ack_mask's:
// bit i the message Ack+2+i), so the client resends only the one missing.
// The mask only grows until the ack takes its messages in, which CLIENT
// checks ("some of the previously acked packets have become un-acked").

// rxWindowAhead is how far past the ack a message is taken: one further is
// not (the mask could not say so), and the client sends it again once the
// ack has moved on.
const rxWindowAhead = 64

// takeLocked takes the client's reliable message seq and reports whether it
// is new: false for one at or before the ack, one taken already, or one
// beyond the window. Caller holds cd.mu.
func (cd *ConnectionData) takeLocked(seq uint16) bool {
	off := int(int16(seq-cd.lastReceivedMessageSeq)) - 1
	if off < 0 || off >= rxWindowAhead {
		return false
	}
	bit := uint64(1) << uint(off)
	if cd.rxWin&bit != 0 {
		return false
	}
	cd.rxWin |= bit
	for cd.rxWin&1 != 0 {
		cd.rxWin >>= 1
		cd.lastReceivedMessageSeq++
	}
	return true
}

// AckReport is what one of our packets says of the connection's numbers.
type AckReport struct {
	Packet   uint16 // the packet's own number
	FlushAck uint16 // the newest of the client's packets taken
	Ack      uint16 // the client's reliable messages taken, every one to here
	Mask     uint64 // and those past it: bit i the message Ack+2+i (CLIENT's)
}

// NextPacketReport numbers our next packet and reads, at once, what it
// acknowledges: a packet numbered later never says less than one numbered
// before it, which CLIENT would refuse whole, however the goroutines that
// build them interleave.
func (cd *ConnectionData) NextPacketReport() AckReport {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.lastSentPacketSeq++
	return AckReport{
		Packet:   cd.lastSentPacketSeq,
		FlushAck: cd.lastReceivedPacketSeq,
		Ack:      cd.lastReceivedMessageSeq,
		Mask:     cd.rxWin >> 1,
	}
}
