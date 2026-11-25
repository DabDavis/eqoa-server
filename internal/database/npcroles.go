package database

// SpellMerchants is every merchant that sells spell scrolls, by its name, with
// the class whose scrolls it sells most (0 WAR ... 14 ALC; a scroll's class
// mask has bit i for class i), for vanilla's minimap (handlers/npcroles.go;
// Heirloom petition 3092: the friendly NPCs' squares coloured by what they
// do, spell merchants by class). A scroll is read as MerchantScrolls reads
// one: the scroll art (icon, stored signed or not), worn nowhere, in the
// npcitem list of an NPC with the merchant flag; each scroll counts once for
// a merchant, ties going to the lower class.
func (s *SpellbookStore) SpellMerchants(icon int) (map[string]int, error) {
	signed := int64(int32(icon))
	rows, err := s.db.Query(`SELECT DISTINCT m.npc_name, p.patternid, p.classuse
		FROM npcitem n
		JOIN itempattern p ON p.patternid = n.patternid
		JOIN npcs m ON m.npc_id = n.npcid AND m.npc_id != 0 AND (m.npc_type & 1) = 1
		WHERE p.itemicon IN (?, ?) AND p.equipslot = -1`, signed, int64(uint32(signed)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]*[15]int{}
	for rows.Next() {
		var name string
		var pattern, mask int
		if err := rows.Scan(&name, &pattern, &mask); err != nil {
			return nil, err
		}
		c := counts[name]
		if c == nil {
			c = new([15]int)
			counts[name] = c
		}
		for i := range c {
			if mask&(1<<i) != 0 {
				c[i]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(map[string]int, len(counts))
	for name, c := range counts {
		best := 0
		for i := range c {
			if c[i] > c[best] {
				best = i
			}
		}
		out[name] = best
	}
	return out, nil
}
