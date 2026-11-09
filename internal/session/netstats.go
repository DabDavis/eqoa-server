package session

import (
	"fmt"
	"sync"
	"time"
)

// netstats.go: what the reliable stream to one client is doing, for the log's
// "Net:" line (MessageQueue.TakeRetransmits) and a GM's #net
// (handlers/netcmd.go).
//
// The round trip is measured as CLIENT measures its own (connection.cpp's
// connection_record_flush_ack): every packet's send time is kept by its
// number (& 1023), and the client's flush ack, the highest packet number it
// has had, names one of them; now less that time is a sample. The client's
// flush ack mask (body flag 0x04, the 64 packets before the ack, a bit each)
// says which of ours it had, so the share of our last packets it never got is
// read off it.

// netRTTSamples is how many round trips are kept: CLIENT keeps sixteen
// (m_usRtt) and times its resends by the largest.
const netRTTSamples = 16

// netBucket is ten seconds of resends, for the rate over the last minute.
type netBucket struct {
	start      time.Time
	msgs, pkts int
}

// netBuckets is a minute of buckets.
const netBuckets = 6

// NetStats is one session's reliable stream, as measured. Its own lock: the
// packet goroutines note acks while the tick sends.
type NetStats struct {
	mu sync.Mutex

	// sentAt is when each of our packets went out, by its number & 1023
	sentAt [1024]time.Time
	// packets and bytes sent, all told
	packets, bytes int64

	// the round trips, newest at rttNext-1; rttN of them are real
	rtt     [netRTTSamples]time.Duration
	rttN    int
	rttNext int
	// srtt is the smoothed round trip (RFC 6298's 1/8), 0 before a sample
	srtt time.Duration

	// the client's flush ack and its mask, the last seen
	flushAck  uint16
	flushMask uint64
	haveMask  bool

	// ack is the client's acknowledgement of our reliable messages, the
	// highest in sequence; ackAt when it last moved, ackSeen whether one came
	ack     uint16
	ackAt   time.Time
	ackSeen bool
	// movedAt is when the ack last went up (a first word of 0 is no move)
	movedAt time.Time

	// resends, all told, and by ten seconds over the last minute
	resent, resentPkts int64
	buckets            [netBuckets]netBucket

	// the log's line: when the last went out, and the resends since
	logAt            time.Time
	logMsgs, logPkts int
}

// Restart forgets the packet numbers and the acknowledgement, kept totals
// aside: a promotion starts both sequence spaces again
// (ConnectionData.ResetForMasterSession).
func (st *NetStats) Restart() {
	st.mu.Lock()
	st.sentAt = [1024]time.Time{}
	st.flushAck, st.flushMask, st.haveMask = 0, 0, false
	st.ack, st.ackAt, st.ackSeen, st.movedAt = 0, time.Time{}, false, time.Time{}
	st.mu.Unlock()
}

// NoteSent notes our packet number seq going out now with n bytes.
func (st *NetStats) NoteSent(seq uint16, n int, now time.Time) {
	st.mu.Lock()
	st.sentAt[seq&1023] = now
	st.packets++
	st.bytes += int64(n)
	st.mu.Unlock()
}

// NoteFlushAck takes the client's flush ack (the highest of our packet
// numbers it has had) and, when it sent one, its mask: a round trip sample
// for each packet it names that is still on the ring, as CLIENT's
// connection_record_acks takes them.
func (st *NetStats) NoteFlushAck(ack uint16, mask uint64, haveMask bool, now time.Time) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sampleLocked(ack, now)
	if haveMask {
		seq := ack
		for m := mask; m != 0; m >>= 1 {
			seq--
			if m&1 != 0 {
				st.sampleLocked(seq, now)
			}
		}
	}
	if int16(ack-st.flushAck) >= 0 {
		st.flushAck, st.flushMask, st.haveMask = ack, mask, haveMask
	}
}

// sampleLocked takes the round trip to our packet seq, once.
func (st *NetStats) sampleLocked(seq uint16, now time.Time) {
	at := st.sentAt[seq&1023]
	if at.IsZero() {
		return
	}
	st.sentAt[seq&1023] = time.Time{}
	d := now.Sub(at)
	if d < 0 || d > time.Minute {
		return
	}
	st.rtt[st.rttNext] = d
	st.rttNext = (st.rttNext + 1) % netRTTSamples
	if st.rttN < netRTTSamples {
		st.rttN++
	}
	if st.srtt == 0 {
		st.srtt = d
	} else {
		st.srtt += (d - st.srtt) / 8
	}
}

// NoteAck takes the client's acknowledgement of our reliable messages.
func (st *NetStats) NoteAck(ack uint16, now time.Time) {
	st.mu.Lock()
	if !st.ackSeen || int16(ack-st.ack) > 0 {
		if st.ackSeen || ack != 0 {
			st.movedAt = now
		}
		st.ack, st.ackAt, st.ackSeen = ack, now, true
	}
	st.mu.Unlock()
}

// AckMovedWithin reports whether the client's acknowledgement of our
// reliable messages moved within d of now: the link is delivering, and what
// waits unacknowledged is on its way, not lost (TakeResends holds its
// resends: a queued link, a relay's TCP stream behind a slow line, sent
// every message in flight again each timeout, and its acks crept further
// behind).
func (st *NetStats) AckMovedWithin(now time.Time, d time.Duration) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return !st.movedAt.IsZero() && now.Sub(st.movedAt) < d
}

// NoteResent notes a packet carrying msgs messages sent again.
func (st *NetStats) NoteResent(msgs int, now time.Time) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.resent += int64(msgs)
	st.resentPkts++
	st.logMsgs += msgs
	st.logPkts++
	b := &st.buckets[int(now.Unix()/10)%netBuckets]
	if now.Sub(b.start) >= 10*time.Second {
		*b = netBucket{start: now.Truncate(10 * time.Second)}
	}
	b.msgs += msgs
	b.pkts++
}

// The resend timeout (RTO): CLIENT's own, the largest of its last sixteen
// round trips and 100 ms more (connection_message_get_desired_flush_time:
// time + m_usTimeout + 100), within minRTO and maxRTO; defaultRTO before a
// round trip is measured (the server's fixed delay until 2026-11-09).
const (
	defaultRTO = 600 * time.Millisecond
	minRTO     = 250 * time.Millisecond
	maxRTO     = 3 * time.Second
)

// RTO is how long a reliable message waits for its acknowledgement before it
// goes again (its first resend: TakeResends doubles it for each after).
func (st *NetStats) RTO() time.Duration {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.rttN == 0 {
		return defaultRTO
	}
	_, _, most := st.rttLocked()
	rto := most + 100*time.Millisecond
	if rto < minRTO {
		rto = minRTO
	}
	if rto > maxRTO {
		rto = maxRTO
	}
	return rto
}

// RTT is the round trip: the smoothed, the least and the most of the last
// sixteen (0s before a sample).
func (st *NetStats) RTT() (smooth, least, most time.Duration) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.rttLocked()
}

func (st *NetStats) rttLocked() (smooth, least, most time.Duration) {
	for i := 0; i < st.rttN; i++ {
		d := st.rtt[i]
		if i == 0 || d < least {
			least = d
		}
		if d > most {
			most = d
		}
	}
	return st.srtt, least, most
}

// resentLastMinuteLocked is the resends of the last minute.
func (st *NetStats) resentLastMinuteLocked(now time.Time) (msgs, pkts int) {
	for _, b := range st.buckets {
		if !b.start.IsZero() && now.Sub(b.start) < time.Minute {
			msgs += b.msgs
			pkts += b.pkts
		}
	}
	return msgs, pkts
}

// lostOfLast is how many of the 64 packets before the client's flush ack it
// said it never had, when it sent a mask (-1 when it did not). Only packets
// we sent count: the first 64 of a connection have fewer before them.
func (st *NetStats) lostOfLastLocked() (lost, of int) {
	if !st.haveMask {
		return -1, 0
	}
	of = 64
	if st.flushAck <= 64 {
		of = int(st.flushAck) - 1 // a connection's first packets have fewer before them
	}
	for i := 0; i < of; i++ {
		if st.flushMask&(1<<uint(i)) == 0 {
			lost++
		}
	}
	return lost, of
}

// NetReport is a session's reliable stream at one moment, for #net and the
// log's line.
type NetReport struct {
	Unacked       int           // reliable messages sent and not acknowledged
	Pending       int           // messages queued and not yet sent once
	FirstSeq      uint16        // the oldest unacknowledged
	LastSeq       uint16        // the newest unacknowledged
	Oldest        time.Duration // how long the oldest has waited since it was queued
	Ack           uint16        // the client's acknowledgement
	AckSeen       bool          // whether one ever came
	AckAge        time.Duration // since it last moved
	SRTT          time.Duration
	RTTMin        time.Duration
	RTTMax        time.Duration
	ResentMinute  int // messages sent again in the last minute
	ResentPktsMin int // packets that carried them
	Resent        int64
	Packets       int64
	Bytes         int64
	Lost, LostOf  int // of our last LostOf packets the client said it missed Lost (-1: no word)
}

// report is the queue's half of a NetReport filled in by the stats.
func (st *NetStats) fill(r *NetReport, now time.Time) {
	st.mu.Lock()
	defer st.mu.Unlock()
	r.Ack, r.AckSeen = st.ack, st.ackSeen
	if st.ackSeen {
		r.AckAge = now.Sub(st.ackAt)
	}
	r.SRTT, r.RTTMin, r.RTTMax = st.rttLocked()
	r.ResentMinute, r.ResentPktsMin = st.resentLastMinuteLocked(now)
	r.Resent, r.Packets, r.Bytes = st.resent, st.packets, st.bytes
	r.Lost, r.LostOf = st.lostOfLastLocked()
}

// String is the report as the log's line and #net say it.
func (r NetReport) String() string {
	s := fmt.Sprintf("%d unacked", r.Unacked)
	if r.Unacked > 0 {
		s += fmt.Sprintf(" (seq %d-%d, oldest %.1fs)", r.FirstSeq, r.LastSeq, r.Oldest.Seconds())
	}
	if r.Pending > 0 {
		s += fmt.Sprintf(", %d queued", r.Pending)
	}
	if r.AckSeen {
		s += fmt.Sprintf("; client acked %d (%.1fs ago)", r.Ack, r.AckAge.Seconds())
	} else {
		s += "; no ack yet"
	}
	if r.SRTT > 0 {
		s += fmt.Sprintf("; rtt %d ms (%d-%d)", r.SRTT.Milliseconds(), r.RTTMin.Milliseconds(), r.RTTMax.Milliseconds())
	}
	s += fmt.Sprintf("; resent %d in %d packets the last minute", r.ResentMinute, r.ResentPktsMin)
	if r.Lost >= 0 && r.LostOf > 0 {
		s += fmt.Sprintf("; it missed %d of our last %d packets", r.Lost, r.LostOf)
	}
	return s
}

// takeLogWindow says whether a "Net:" line is due (every netLogEvery while
// resending) and hands over the resends since the last.
func (st *NetStats) takeLogWindow(now time.Time) (due bool, msgs, pkts int, over time.Duration) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.logPkts == 0 {
		return false, 0, 0, 0
	}
	if !st.logAt.IsZero() && now.Sub(st.logAt) < netLogEvery {
		return false, 0, 0, 0
	}
	over = now.Sub(st.logAt)
	if st.logAt.IsZero() {
		over = 0
	}
	msgs, pkts = st.logMsgs, st.logPkts
	st.logMsgs, st.logPkts, st.logAt = 0, 0, now
	return true, msgs, pkts, over
}

// netLogEvery is how often a session resending gets a "Net:" line: the line
// it replaced was one a packet (7,583 a minute on 2026-11-09, unnamed).
const netLogEvery = 10 * time.Second
