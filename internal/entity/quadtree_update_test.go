package entity

import (
	"fmt"
	"math/rand"
	"testing"
)

// updateAsBefore is QuadTree.Update as it was before its same-leaf step:
// Remove and Insert from the top whenever the tree is divided.
func updateAsBefore(qt *QuadTree, id EntityID, oldX, oldY, newX, newY float32) bool {
	if qt.bounds.Contains(oldX, oldY) && qt.bounds.Contains(newX, newY) {
		if qt.divided {
			qt.Remove(id, oldX, oldY)
			return qt.Insert(id, newX, newY)
		}
		for i := range qt.entries {
			if qt.entries[i].ID == id {
				qt.entries[i].X = newX
				qt.entries[i].Y = newY
				return true
			}
		}
		return true
	}
	qt.Remove(id, oldX, oldY)
	return qt.Insert(id, newX, newY)
}

// dumpTree is every node's entries in order, the shape with them.
func dumpTree(qt *QuadTree) string {
	s := fmt.Sprintf("[%v %v", qt.bounds, qt.entries)
	if qt.divided {
		s += dumpTree(qt.northwest) + dumpTree(qt.northeast) + dumpTree(qt.southwest) + dumpTree(qt.southeast)
	}
	return s + "]"
}

// Update's same-leaf step leaves the tree as Remove and Insert did: the same
// nodes, each one's entries in the same order (the order QueryRadius answers
// in, which a view's ties are cut by), over walks, jumps, stacks of entities
// at one spot and steps off the tree's edge.
func TestQuadTreeUpdateAsRemoveInsert(t *testing.T) {
	rnd := rand.New(rand.NewSource(3))
	for round := 0; round < 200; round++ {
		b := Bounds{X: 0, Y: 0, Width: 3200, Height: 3200}
		a, c := NewQuadTree(b), NewQuadTree(b)
		n := 50 + rnd.Intn(400)
		xs, ys := make([]float32, n), make([]float32, n)
		for i := 0; i < n; i++ {
			if i > 0 && rnd.Intn(8) == 0 { // a stack at one spot
				xs[i], ys[i] = xs[i-1], ys[i-1]
			} else {
				xs[i], ys[i] = rnd.Float32()*3200, rnd.Float32()*3200
			}
			a.Insert(EntityID(i+1), xs[i], ys[i])
			c.Insert(EntityID(i+1), xs[i], ys[i])
		}
		for step := 0; step < 3000; step++ {
			i := rnd.Intn(n)
			var nx, ny float32
			switch rnd.Intn(10) {
			case 0: // a jump anywhere, now and then off the edge
				nx, ny = rnd.Float32()*3400-100, rnd.Float32()*3400-100
			case 1: // onto another's spot
				j := rnd.Intn(n)
				nx, ny = xs[j], ys[j]
			default: // a step
				nx, ny = xs[i]+rnd.Float32()*4-2, ys[i]+rnd.Float32()*4-2
			}
			ra := updateAsBefore(a, EntityID(i+1), xs[i], ys[i], nx, ny)
			rc := c.Update(EntityID(i+1), xs[i], ys[i], nx, ny)
			if ra != rc {
				t.Fatalf("round %d step %d: Update %v, before %v", round, step, rc, ra)
			}
			xs[i], ys[i] = nx, ny
		}
		if da, dc := dumpTree(a), dumpTree(c); da != dc {
			t.Fatalf("round %d: the trees differ", round)
		}
	}
}
