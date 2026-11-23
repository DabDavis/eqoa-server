package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unicode/utf16"

	"github.com/eqoa/server/internal/protocol"
)

// dumpReader retains the first read error. A count is bounded by the bytes
// left, and a failed read stops every enclosing record loop.
type dumpReader struct {
	data []byte
	err  error
}

func (r *dumpReader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.data) {
		r.err = io.ErrUnexpectedEOF
		return nil
	}
	b := r.data[:n]
	r.data = r.data[n:]
	return b
}

func (r *dumpReader) u32() uint32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func (r *dumpReader) signed() int64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Varint(r.data)
	if n <= 0 {
		r.err = errors.New("invalid or incomplete signed integer")
		return 0
	}
	r.data = r.data[n:]
	return v
}

func (r *dumpReader) count(n int64) int {
	if r.err != nil {
		return 0
	}
	if n < 0 || n > int64(len(r.data)) {
		r.err = errors.New("invalid record count")
		return 0
	}
	return int(n)
}

func (r *dumpReader) wide() string {
	n := r.u32()
	if uint64(n)*2 > uint64(len(r.data)) {
		r.err = io.ErrUnexpectedEOF
		return ""
	}
	b := r.take(int(n) * 2)
	chars := make([]uint16, len(b)/2)
	for i := range chars {
		chars[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	return string(utf16.Decode(chars))
}

func parseMemoryDump(data []byte) ([]parsedItem, []parsedItem, []parsedSpell, string, error) {
	if len(data) < 2 || binary.LittleEndian.Uint16(data) != uint16(protocol.OpcodeMemoryDump) {
		return nil, nil, nil, "", errors.New("expected a MemoryDump opcode")
	}
	r := dumpReader{data: data[2:]}
	r.take(1)
	r.take(int(r.u32())) // zone path
	r.signed()           // character ID
	name := string(r.take(int(r.u32())))
	for range 5 {
		r.signed()
	} // class, race, level, XP, debt
	r.take(1) // breath
	for range 5 {
		r.signed()
	} // purse, bank, TP, base stat, world
	r.take(6 * 4) // position and two reserved floats
	for n := r.count(r.signed()); n > 0 && r.err == nil; n-- {
		r.signed() // direction
		for range 4 {
			r.take(1)
			r.wide() // label
			r.wide() // message
		}
	}
	r.take(8) // ability and quest tokens
	for n := r.count(int64(r.u32())); n > 0 && r.err == nil; n-- {
		r.wide()
	}
	items := r.items()
	bank := r.items()
	lots, bids := r.signed(), r.signed()
	if lots != 0 || bids != 0 {
		return nil, nil, nil, "", errors.New("captures with auction listings are unsupported")
	}
	r.take(1) // bank flag
	if flag := r.take(1); len(flag) == 1 && flag[0] != 0 {
		return nil, nil, nil, "", errors.New("captures with corpse inventory are unsupported")
	}
	var spells []parsedSpell
	for n := r.count(r.signed()); n > 0 && r.err == nil; n-- {
		spells = append(spells, r.spell())
	}
	if r.err != nil {
		return nil, nil, nil, "", fmt.Errorf("at byte %d: %w", len(data)-len(r.data), r.err)
	}
	return items, bank, spells, name, nil
}

func (r *dumpReader) items() []parsedItem {
	n := r.count(r.signed())
	r.u32() // container token
	var items []parsedItem
	for ; n > 0 && r.err == nil; n-- {
		it := parsedItem{}
		for _, field := range []*int64{&it.StackLeft, &it.RemainingHP, &it.Charges, &it.EquipLocation, &it.Location} {
			*field = r.signed()
		}
		it.Key = r.u32()
		for _, field := range []*int64{
			&it.ItemID, &it.ItemCost, &it.Unk1, &it.ItemIcon, &it.Unk2, &it.Slot, &it.Unk3,
			&it.NoTrade, &it.NoRent, &it.Unk4, &it.AttackType, &it.WeaponDamage, &it.Unk5,
			&it.LevelReq, &it.MaxStack, &it.MaxHP, &it.Duration, &it.ClassUse, &it.RaceUse,
			&it.ProcAnim, &it.IsLore, &it.Unk6, &it.IsCraft,
		} {
			*field = r.signed()
		}
		it.Name = r.wide()
		it.Desc = r.wide()
		it.Stats = make(map[int]int64)
		for pairs := r.count(r.signed()); pairs > 0 && r.err == nil; pairs-- {
			idx := r.signed()
			it.Stats[int(idx)] = r.signed()
		}
		items = append(items, it)
	}
	return items
}

func (r *dumpReader) spell() parsedSpell {
	var sp parsedSpell
	for _, field := range []*int64{&sp.SpellID, &sp.AddedOrder, &sp.OnHotBar, &sp.WhereOnHotBar, &sp.Unk1, &sp.ShowHide, &sp.AbilityLevel} {
		*field = r.signed()
	}
	bits := r.u32()
	// RangeBits stores the low two bytes as single, non-negative zigzags.
	if bits&0x8181 != 0 && r.err == nil {
		r.err = errors.New("spell range cannot be represented by this schema")
	}
	sp.Range = int(bits >> 16)
	sp.Unk2 = int64(byte(bits) >> 1)
	sp.Unk3 = int64(byte(bits>>8) >> 1)
	for _, field := range []*int64{&sp.CastTime, &sp.Power, &sp.IconColor, &sp.Icon, &sp.Scope, &sp.Recast, &sp.EqpRequire} {
		*field = r.signed()
	}
	sp.Name = r.wide()
	sp.Desc = r.wide()
	if r.signed() != 0 {
		r.err = errors.New("captures with spell sub-records are unsupported")
	}
	return sp
}
