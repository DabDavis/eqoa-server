package entity

const (
	// MaxEntitiesPerNode before splitting.
	MaxEntitiesPerNode = 16
	// MinNodeSize is the minimum node dimension.
	MinNodeSize = 100.0
)

// Bounds represents a rectangular region.
type Bounds struct {
	X, Y          float32 // Top-left corner
	Width, Height float32
}

// Contains returns whether a point is inside the bounds.
func (b *Bounds) Contains(x, y float32) bool {
	return x >= b.X && x < b.X+b.Width &&
		y >= b.Y && y < b.Y+b.Height
}

// Intersects returns whether two bounds overlap.
func (b *Bounds) Intersects(other *Bounds) bool {
	return !(b.X+b.Width < other.X ||
		other.X+other.Width < b.X ||
		b.Y+b.Height < other.Y ||
		other.Y+other.Height < b.Y)
}

// qtEntry stores an entity alongside its position in the tree.
type qtEntry struct {
	ID   EntityID
	X, Y float32
}

// QuadTree is a spatial index for entities.
// All methods are unsynchronized — call only while holding the world lock.
type QuadTree struct {
	bounds  Bounds
	entries []qtEntry
	divided bool

	northwest *QuadTree
	northeast *QuadTree
	southwest *QuadTree
	southeast *QuadTree
}

// NewQuadTree creates a new quadtree with the given bounds.
func NewQuadTree(bounds Bounds) *QuadTree {
	return &QuadTree{
		bounds:  bounds,
		entries: make([]qtEntry, 0, MaxEntitiesPerNode),
	}
}

// Insert adds an entity at the given position.
func (qt *QuadTree) Insert(id EntityID, x, y float32) bool {
	if !qt.bounds.Contains(x, y) {
		return false
	}

	if !qt.divided {
		if len(qt.entries) < MaxEntitiesPerNode {
			qt.entries = append(qt.entries, qtEntry{ID: id, X: x, Y: y})
			return true
		}

		if qt.bounds.Width/2 < MinNodeSize {
			qt.entries = append(qt.entries, qtEntry{ID: id, X: x, Y: y})
			return true
		}

		qt.subdivide()
	}

	if qt.northwest.Insert(id, x, y) {
		return true
	}
	if qt.northeast.Insert(id, x, y) {
		return true
	}
	if qt.southwest.Insert(id, x, y) {
		return true
	}
	if qt.southeast.Insert(id, x, y) {
		return true
	}

	return false
}

// subdivide splits this node into four children.
func (qt *QuadTree) subdivide() {
	halfWidth := qt.bounds.Width / 2
	halfHeight := qt.bounds.Height / 2
	x := qt.bounds.X
	y := qt.bounds.Y

	qt.northwest = NewQuadTree(Bounds{X: x, Y: y, Width: halfWidth, Height: halfHeight})
	qt.northeast = NewQuadTree(Bounds{X: x + halfWidth, Y: y, Width: halfWidth, Height: halfHeight})
	qt.southwest = NewQuadTree(Bounds{X: x, Y: y + halfHeight, Width: halfWidth, Height: halfHeight})
	qt.southeast = NewQuadTree(Bounds{X: x + halfWidth, Y: y + halfHeight, Width: halfWidth, Height: halfHeight})

	qt.divided = true

	for _, entry := range qt.entries {
		qt.northwest.Insert(entry.ID, entry.X, entry.Y)
		qt.northeast.Insert(entry.ID, entry.X, entry.Y)
		qt.southwest.Insert(entry.ID, entry.X, entry.Y)
		qt.southeast.Insert(entry.ID, entry.X, entry.Y)
	}
	qt.entries = nil
}

// Remove removes an entity from the quadtree using its last known position.
func (qt *QuadTree) Remove(id EntityID, x, y float32) bool {
	if !qt.bounds.Contains(x, y) {
		return false
	}

	for i, entry := range qt.entries {
		if entry.ID == id {
			qt.entries = append(qt.entries[:i], qt.entries[i+1:]...)
			return true
		}
	}

	if qt.divided {
		if qt.northwest.Remove(id, x, y) {
			return true
		}
		if qt.northeast.Remove(id, x, y) {
			return true
		}
		if qt.southwest.Remove(id, x, y) {
			return true
		}
		if qt.southeast.Remove(id, x, y) {
			return true
		}
	}

	return false
}

// Update moves an entity from its old position to a new position.
func (qt *QuadTree) Update(id EntityID, oldX, oldY, newX, newY float32) bool {
	if qt.bounds.Contains(oldX, oldY) && qt.bounds.Contains(newX, newY) {
		if qt.divided {
			qt.Remove(id, oldX, oldY)
			return qt.Insert(id, newX, newY)
		}
		// Same leaf node — just update the stored position.
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

// Query returns all entity IDs within a rectangular region.
func (qt *QuadTree) Query(bounds Bounds) []EntityID {
	var result []EntityID
	qt.query(bounds, &result)
	return result
}

func (qt *QuadTree) query(bounds Bounds, result *[]EntityID) {
	if !qt.bounds.Intersects(&bounds) {
		return
	}

	for _, entry := range qt.entries {
		if bounds.Contains(entry.X, entry.Y) {
			*result = append(*result, entry.ID)
		}
	}

	if qt.divided {
		qt.northwest.query(bounds, result)
		qt.northeast.query(bounds, result)
		qt.southwest.query(bounds, result)
		qt.southeast.query(bounds, result)
	}
}

// QueryRadius returns all entity IDs within a radius of a point.
func (qt *QuadTree) QueryRadius(x, y, radius float32) []EntityID {
	bounds := Bounds{
		X:      x - radius,
		Y:      y - radius,
		Width:  radius * 2,
		Height: radius * 2,
	}

	// Collect entries (with positions) from the bounding box.
	var candidates []qtEntry
	qt.queryEntries(bounds, &candidates)

	// Filter to actual circle.
	radiusSq := radius * radius
	var result []EntityID
	for _, entry := range candidates {
		dx := entry.X - x
		dy := entry.Y - y
		if dx*dx+dy*dy <= radiusSq {
			result = append(result, entry.ID)
		}
	}
	return result
}

func (qt *QuadTree) queryEntries(bounds Bounds, result *[]qtEntry) {
	if !qt.bounds.Intersects(&bounds) {
		return
	}

	for _, entry := range qt.entries {
		if bounds.Contains(entry.X, entry.Y) {
			*result = append(*result, entry)
		}
	}

	if qt.divided {
		qt.northwest.queryEntries(bounds, result)
		qt.northeast.queryEntries(bounds, result)
		qt.southwest.queryEntries(bounds, result)
		qt.southeast.queryEntries(bounds, result)
	}
}

// Count returns the total number of entities.
func (qt *QuadTree) Count() int {
	count := len(qt.entries)
	if qt.divided {
		count += qt.northwest.Count()
		count += qt.northeast.Count()
		count += qt.southwest.Count()
		count += qt.southeast.Count()
	}
	return count
}

// Clear removes all entities from the quadtree.
func (qt *QuadTree) Clear() {
	qt.entries = qt.entries[:0]
	qt.divided = false
	qt.northwest = nil
	qt.northeast = nil
	qt.southwest = nil
	qt.southeast = nil
}
