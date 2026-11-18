package handlers

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sort"
	"syscall"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// questmarks_perf_real_test.go — the marks pass against a real server's
// database (EQOA_REAL_DB; a copy), for the tick's cost (a profile of the live
// Heirloom server, 2026-11-18: the marks were 30% of its CPU with 34 players).
//
// realMarksRig puts every character of the database (or the EQOA_MARKS_CHARS
// first, by serverid) in the World, each where it logged out and again at the
// giver of a quest near its level, and TestRealQuestMarksGolden holds each
// one's body (what sendMarksLocked sends) and head marks (syncHeadMarks's
// line), all in one sync as syncMarks runs them, to the file EQOA_MARKS_GOLDEN names:
// written there when EQOA_MARKS_GOLDEN_WRITE is set (from the code before a
// change), compared byte for byte otherwise (after it).
// BenchmarkRealMarksSync is one syncMarks over the 34 characters with the most
// inventory rows.

type realMarksRig struct {
	w     *questWalker
	sess  *session.Session
	chars []realMarksChar
}

type realMarksChar struct {
	label string
	p     *entity.Player
}

func newRealMarksRig(t testing.TB, src string, limit int) *realMarksRig {
	w := newQuestWalker(t, src)
	gw := w.gw
	gw.LoadNPCs(database.NewNPCStore(w.db))
	if _, _, err := gw.LoadSpawnOnly(w.db); err != nil {
		t.Fatal(err)
	}
	rows, err := w.db.Query(`SELECT c.serverid, c.charName, c.level, c.tclass, c.race, c.tunar, c.world, c.x, c.y, c.z,
		(SELECT count(*) FROM charinventory i WHERE i.serverid = c.serverid) n
		FROM characters c ORDER BY c.serverid`)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		id                uint32
		name              string
		level, class, rc  int
		tunar, world, inv int
		x, y, z           float32
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.name, &r.level, &r.class, &r.rc, &r.tunar, &r.world, &r.x, &r.y, &r.z, &r.inv); err != nil {
			t.Fatal(err)
		}
		all = append(all, r)
	}
	rows.Close()
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	// a giver near each level: the first enabled quest's giver standing in the world
	q := w.qe
	q.mu.Lock()
	quests := append([]*database.Quest(nil), q.cat.all...)
	q.mu.Unlock()
	sort.Slice(quests, func(i, j int) bool { return quests[i].ID < quests[j].ID })
	giverAt := map[int]*entity.NPC{}
	for _, qu := range quests {
		if !qu.Enabled {
			continue
		}
		lv := max(qu.MinLevel, 1)
		if giverAt[lv] != nil {
			continue
		}
		for _, n := range gw.NPCs {
			if foldName(n.Name) == foldName(qu.Giver) && !n.Temporary && n.Owner == 0 {
				if g := giverAt[lv]; g == nil || n.ServerID < g.ServerID {
					giverAt[lv] = n
				}
			}
		}
	}
	rig := &realMarksRig{w: w, sess: session.NewSession(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}, 1, 1, 1, 1, false)}
	next := entity.EntityID(0x7000000)
	for _, r := range all {
		mk := func(label string, world uint8, x, y, z float32) {
			next++
			p := &entity.Player{ServerID: next, CharServerID: r.id, Name: r.name, Level: r.level, Class: r.class, Race: r.rc,
				Tunar: r.tunar, World: world, X: x, Y: y, Z: z, HP: 100}
			rig.chars = append(rig.chars, realMarksChar{label: fmt.Sprintf("%d %s %s", r.id, r.name, label), p: p})
		}
		mk("logout", uint8(r.world), r.x, r.y, r.z)
		for lv := r.level; lv >= 1; lv-- {
			if g := giverAt[lv]; g != nil {
				mk(fmt.Sprintf("at %s", g.Name), g.World, g.SpawnX+1, g.SpawnY, g.SpawnZ+1)
				break
			}
		}
	}
	return rig
}

// bodyLocked is the character's marks body as sendMarksLocked sends it (""
// when none is sent: not in the world), and its head marks' line as
// syncHeadMarks makes it. The player stands in the World alone (another
// character's copy at another place is taken out). Must be called with q.mu
// held.
func (r *realMarksRig) bodyLocked(ch realMarksChar) (string, string) {
	gw := r.w.gw
	gw.Players = map[entity.EntityID]*entity.Player{ch.p.ServerID: ch.p}
	q := r.w.qe
	c := q.namedLocked(ch.p.CharServerID, ch.p.Name)
	c.marksSent = ""
	q.sendMarksLocked(r.sess, ch.p, c)
	end := q.beginPassLocked(ch.p)
	head := headMarksText(q.headMarksLocked(q.lookAt(r.sess), ch.p, c))
	end()
	return c.marksSent, head
}

// syncLocked runs f as a sync of the marks runs its passes (one read of the
// Ascended records for them all, where the code has it). Must be called with
// q.mu held.
func (r *realMarksRig) syncLocked(f func()) {
	if begin, ok := any(r.w.qe).(interface{ beginSyncLocked() func() }); ok {
		defer begin.beginSyncLocked()()
	}
	f()
}

func TestRealQuestMarksGolden(t *testing.T) {
	src, golden := os.Getenv("EQOA_REAL_DB"), os.Getenv("EQOA_MARKS_GOLDEN")
	if src == "" || golden == "" {
		t.Skip("EQOA_REAL_DB names a server database, EQOA_MARKS_GOLDEN the bodies' file")
	}
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	limit := 0
	fmt.Sscan(os.Getenv("EQOA_MARKS_CHARS"), &limit)
	rig := newRealMarksRig(t, src, limit)
	defer rig.w.db.Close()
	var out bytes.Buffer
	marks, sent, heads := 0, 0, 0
	q := rig.w.qe
	q.mu.Lock()
	rig.syncLocked(func() {
		for _, ch := range rig.chars {
			b, head := rig.bodyLocked(ch)
			if b != "" {
				sent++
				marks += int(b[0])
			}
			if head != "" {
				heads++
			}
			fmt.Fprintf(&out, "%s\x00", ch.label)
			out.Write(binary.LittleEndian.AppendUint32(nil, uint32(len(b))))
			out.WriteString(b)
			out.Write(binary.LittleEndian.AppendUint32(nil, uint32(len(head))))
			out.WriteString(head)
		}
	})
	q.mu.Unlock()
	t.Logf("%d characters' places, %d bodies, %d marks, %d head mark lines, %d bytes", len(rig.chars), sent, marks, heads, out.Len())
	if os.Getenv("EQOA_MARKS_GOLDEN_WRITE") != "" {
		if err := os.WriteFile(golden, out.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, out.Bytes()) {
		// the first character whose body differs
		a, b := bytes.Split(want, []byte{0}), bytes.Split(out.Bytes(), []byte{0})
		for i := 0; i < len(a) && i < len(b); i++ {
			if !bytes.Equal(a[i], b[i]) {
				t.Fatalf("bodies differ at record %d: %q... vs %q...", i, clip(string(a[i]), 80), clip(string(b[i]), 80))
			}
		}
		t.Fatalf("bodies differ: %d bytes vs %d", len(want), out.Len())
	}
}

func BenchmarkRealMarksSync(b *testing.B) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		b.Skip("EQOA_REAL_DB names a server database")
	}
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	rig := newRealMarksRig(b, src, 0)
	defer rig.w.db.Close()
	// the 34 characters with the most inventory rows, where they logged out
	type ranked struct {
		ch realMarksChar
		n  int
	}
	var rs []ranked
	seen := map[uint32]bool{}
	for _, ch := range rig.chars {
		if seen[ch.p.CharServerID] {
			continue
		}
		seen[ch.p.CharServerID] = true
		var n int
		rig.w.db.QueryRow(`SELECT count(*) FROM charinventory WHERE serverid = ?`, ch.p.CharServerID).Scan(&n)
		rs = append(rs, ranked{ch, n})
	}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].n > rs[j].n })
	if len(rs) > 34 {
		rs = rs[:34]
	}
	gw, q := rig.w.gw, rig.w.qe
	gw.Players = map[entity.EntityID]*entity.Player{}
	var cs []*questChar
	for _, r := range rs {
		gw.Players[r.ch.p.ServerID] = r.ch.p
		q.mu.Lock()
		cs = append(cs, q.namedLocked(r.ch.p.CharServerID, r.ch.p.Name))
		q.mu.Unlock()
	}
	// a sync as syncMarks makes it: every character's pass under one lock
	sync := func() {
		q.mu.Lock()
		defer q.mu.Unlock()
		rig.syncLocked(func() {
			for k, r := range rs {
				q.sendMarksLocked(rig.sess, r.ch.p, cs[k])
			}
		})
	}
	// the process's CPU time a sync (the dev box is shared: its wall time is not the work's)
	cpu := func() time.Duration {
		var ru syscall.Rusage
		syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
		return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
	}
	b.ReportAllocs()
	b.ResetTimer()
	c0 := cpu()
	for i := 0; i < b.N; i++ {
		sync()
		rig.sess.Queue().GetPending()
	}
	b.ReportMetric(float64(cpu()-c0)/float64(b.N), "cpu-ns/op")
}
