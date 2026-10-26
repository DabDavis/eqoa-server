package voice

import (
	"sort"
	"time"
)

// A listener's slots: which talker each of its four receiving tracks
// carries, and the rewriting that makes one track of several talkers' RTP.

const (
	// Slots is how many talkers a listener hears at once (the client's
	// recvonly audio transceivers).
	Slots = 4

	// slotHold is how long a talker keeps its slot after it was last among
	// the loudest (or since it got it), so a slot does not flap between two
	// of about the same loudness.
	slotHold = 1500 * time.Millisecond

	// a talker heard of no packet for this long is not a candidate (DTX
	// sends one every 400 ms in a pause)
	talkerQuiet = 2 * time.Second
)

// seat is one of a listener's slots.
type seat struct {
	talker  uint32 // its voice id; 0 free
	since   time.Time
	lastTop time.Time // when it was last among the loudest
	rw      rewriter
}

// candidate is a talker a listener may hear now, with its loudness.
type candidate struct {
	id   uint32
	loud float64
}

// byLoudness orders candidates loudest first, then by voice id.
func byLoudness(c []candidate) {
	sort.Slice(c, func(i, j int) bool {
		if c[i].loud != c[j].loud {
			return c[i].loud > c[j].loud
		}
		return c[i].id < c[j].id
	})
}

// pick seats a listener's talkers out of cands (byLoudness'ed): a seat whose
// talker is no candidate any more is freed; each of the len(seats) loudest
// not seated takes a free seat, or else the seat of a talker not among the
// loudest whose hold has run out (the quietest such first). It says whether
// any seat changed.
func pick(seats []seat, cands []candidate, now time.Time) bool {
	changed := false
	rank := make(map[uint32]int, len(cands))
	for i, c := range cands {
		rank[c.id] = i
	}
	seated := map[uint32]bool{}
	for i := range seats {
		s := &seats[i]
		if s.talker == 0 {
			continue
		}
		r, ok := rank[s.talker]
		if !ok {
			*s = seat{rw: s.rw}
			s.rw.fresh = true
			changed = true
			continue
		}
		if r < len(seats) {
			s.lastTop = now
		}
		seated[s.talker] = true
	}
	for i := 0; i < len(cands) && i < len(seats); i++ {
		c := cands[i]
		if seated[c.id] {
			continue
		}
		k := -1
		for j := range seats {
			if seats[j].talker == 0 {
				k = j
				break
			}
		}
		if k < 0 {
			// the quietest seated talker past its hold and not among the loudest
			worst := -1
			for j := range seats {
				s := &seats[j]
				r := rank[s.talker]
				held := s.since
				if s.lastTop.After(held) {
					held = s.lastTop
				}
				if r < len(seats) || now.Sub(held) < slotHold {
					continue
				}
				if worst < 0 || r > rank[seats[worst].talker] {
					worst = j
				}
			}
			if worst < 0 {
				continue
			}
			delete(seated, seats[worst].talker)
			k = worst
		}
		seatTalker(&seats[k], c.id, now)
		seated[c.id] = true
		changed = true
	}
	return changed
}

// seatTalker puts talker on s from now; its RTP is rebased at its first
// packet there.
func seatTalker(s *seat, talker uint32, now time.Time) {
	s.talker, s.since, s.lastTop = talker, now, now
	s.rw.fresh = true
}

// rewriter makes one outbound stream of the packets of whichever talker a
// slot carries: the sequence numbers run on by one and the timestamps by the
// time between, across a change of talker, so the listener's jitter buffer
// sees one stream (its SSRC is the slot track's own).
type rewriter struct {
	started bool
	fresh   bool   // rebase at the next packet (a new talker, or the same one seated again)
	talker  uint32 // whose packets the offsets are for
	seqOff  uint16
	tsOff   uint32
	lastSeq uint16 // the last sent
	lastTS  uint32
	lastAt  time.Time
}

// opusFrame is the shortest step a rebased timestamp takes: one 20 ms Opus
// frame at 48 kHz.
const opusFrame = 960

// next is talker's packet (seq, ts) as the slot sends it, and whether it
// starts a talkspurt (the first after a rebase: the RTP marker).
func (r *rewriter) next(talker uint32, seq uint16, ts uint32, now time.Time) (uint16, uint32, bool) {
	rebase := r.fresh || !r.started || r.talker != talker
	if rebase {
		if r.started {
			step := uint32(now.Sub(r.lastAt).Seconds() * 48000)
			if step < opusFrame {
				step = opusFrame
			}
			r.seqOff = r.lastSeq + 1 - seq
			r.tsOff = r.lastTS + step - ts
		} else {
			r.seqOff, r.tsOff = 0, 0
		}
		r.started, r.fresh, r.talker = true, false, talker
	}
	oseq, ots := seq+r.seqOff, ts+r.tsOff
	if rebase || int16(oseq-r.lastSeq) > 0 {
		r.lastSeq, r.lastTS, r.lastAt = oseq, ots, now
	}
	return oseq, ots, rebase
}
