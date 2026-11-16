package handlers

import (
	"log"
	"sync/atomic"

	"github.com/eqoa/server/internal/database"
)

// itemowner.go — records one character alone may wear (database/itemowner.go;
// the owner, 2026-11-16, Drizzt's scale mail: "that he alone can wear drizzt
// only"). cannotWear asks itemOwnerOf before the class, race and level gates.

// itemNotYours is the refusal, in the gates' red (mayWear).
const itemNotYours = "This item can only be worn by the one it was made for."

// itemOwners is the database the bindings are read from, set as the registry
// is made; nil (a test's handler without one): nothing is bound.
var itemOwners atomic.Pointer[database.SQLiteDB]

// itemOwnerOf is the one character that may wear patternID, ok false when
// anyone may. A read that fails binds nothing (logged): an equip is never
// refused for a database error.
func itemOwnerOf(patternID int) (uint32, bool) {
	db := itemOwners.Load()
	if db == nil {
		return 0, false
	}
	owner, ok, err := db.ItemOwner(patternID)
	if err != nil {
		log.Printf("itemowner: patternid %d: %v", patternID, err)
		return 0, false
	}
	return owner, ok
}
