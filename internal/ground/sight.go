package ground

// What a body can be seen past (entity/sight.go): a creature notices a player,
// and an area spell reaches one, only with nothing between them. The walk's
// test (Wall, Crosses) takes a face only from its front, as VICollide stops a
// body, so a house's wall lets a way out of the room pass and the terrain's
// ceilings and floors count only one way; sight goes both ways, so it meets a
// face from either side, as Roof does. Everything the game has collision for
// counts: the terrain's floors, ceilings and steep faces, and anything placed
// on it (a house, a city's wall, a floor above, a rock, a tree's trunk).
// Plants have no collision on the console (the bake leaves the FloraSprites
// out), so a tree's leaves and the grass hide no one, and water is no surface.

// Sees reports the straight line from (x0, y0, z0) to (x1, y1, z1) in world
// meeting nothing of the game's collision; true where the world has neither
// file.
func (g *Ground) Sees(worldN uint8, x0, y0, z0, x1, y1, z1 float32) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	a, b := Vertex{x0, y0, z0}, Vertex{x1, y1, z1}
	return !g.terrainMeetsLocked(worldN, a, b) && !g.placedMeetsLocked(worldN, a, b)
}

// terrainMeetsLocked is whether the line from a to b meets a triangle of the
// terrain, its floors and ceilings or its steep faces, from either side. Must
// be called with g.mu held.
func (g *Ground) terrainMeetsLocked(worldN uint8, a, b Vertex) bool {
	w := g.worlds[worldN]
	if w == nil {
		return false
	}
	lo, hi := lowest(a, b), highest(a, b)
	for _, zi := range w.terrainIndexLocked().near(lo.X, lo.Z, hi.X, hi.Z) {
		i, e := int(zi), w.entries[zi]
		if e.verts == 0 || hi.X < e.minX || lo.X > e.maxX || hi.Z < e.minZ || lo.Z > e.maxZ {
			continue
		}
		zg, err := g.zoneLocked(w, i)
		if err != nil {
			continue
		}
		if zg.gridMeets(&zg.floors, zg.tri, lo, hi, a, b) || zg.gridMeets(&zg.wallGrid, zg.wall, lo, hi, a, b) {
			return true
		}
	}
	return false
}

// gridMeets is whether the line from a to b, within the box lo..hi, meets a
// triangle of one of the zone's grids.
func (z *zoneGrid) gridMeets(g *triGrid, tri func(int32) tri, lo, hi, a, b Vertex) bool {
	for cx := z.cellX(g, lo.X); cx <= z.cellX(g, hi.X); cx++ {
		for cz := z.cellZ(g, lo.Z); cz <= z.cellZ(g, hi.Z); cz++ {
			c := cx*g.nz + cz
			for _, ti := range g.cells[g.offsets[c]:g.offsets[c+1]] {
				t := tri(ti)
				if meets(a, b, t[0], t[1], t[2]) {
					return true
				}
			}
		}
	}
	return false
}

// placedMeetsLocked is whether the line from a to b meets a face of anything
// placed on the terrain, from either side. Must be called with g.mu held.
func (g *Ground) placedMeetsLocked(worldN uint8, a, b Vertex) bool {
	w := g.walls[worldN]
	if w == nil {
		return false
	}
	lo, hi := lowest(a, b), highest(a, b)
	for _, zi := range w.wallsIndexLocked().near(lo.X, lo.Z, hi.X, hi.Z) {
		i, e := int(zi), w.zones[zi]
		if e.count == 0 || !overlaps(lo, hi, e.lo, e.hi) {
			continue
		}
		z, err := g.wallZoneLocked(w, i)
		if err != nil {
			continue
		}
		x0, x1, z0, z1 := z.span(lo, hi)
		for cx := x0; cx <= x1; cx++ {
			for cz := z0; cz <= z1; cz++ {
				c := cx*z.nz + cz
				for _, k := range z.cells[z.offsets[c]:z.offsets[c+1]] {
					p := &z.places[k]
					if !overlaps(lo, hi, p.lo, p.hi) {
						continue
					}
					buf, err := w.buffer(p.buf)
					if err != nil {
						continue
					}
					if p.meets(buf, a, b) {
						return true
					}
				}
			}
		}
	}
	return false
}
