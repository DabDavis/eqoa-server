package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

// tempWords are the words a temporary password is made of: short, common,
// lower case, unlike each other when read aloud or typed on a pad's keyboard.
var tempWords = strings.Fields(`
	acorn amber anchor apple arrow aspen autumn badger banner barley basket beacon
	beetle berry birch bison blanket bloom boulder bramble breeze brook bucket butter
	cabin camel candle canyon carrot castle cedar cherry chime cider citrus clover
	cobalt comet copper coral cotton cradle crane cricket crown dagger daisy dawn
	desert dolphin dragon drum eagle ember falcon feather fern fiddle finch flint
	forest fossil fox garden garnet ginger glacier goblin granite grape gravel harbor
	hazel heron hollow honey island ivory jasper jungle kettle kitten lagoon lantern
	lemon lily lizard locket lotus maple marble meadow melon mint mirror moose
	moss nectar nutmeg oak ocean olive onyx orchid otter owl panda pebble pepper
	pickle pine planet plum pond poppy pumpkin quartz quill rabbit raven reed ribbon
	river robin rocket rose ruby saddle salmon sapphire scarf shell silver sparrow
	spruce squid star stone storm sugar summer swan tiger timber toad topaz torch
	tulip turtle valley velvet violet walnut walrus willow winter wolf wren zebra`)

// TempPassword makes a temporary password a GM can read out to a player:
// three words and two digits, as "otter-maple-lantern-42", from a
// cryptographic source. It is at most MaxPasswordLen characters.
func TempPassword() (string, error) {
	pick := func(n int) (int, error) {
		v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
		if err != nil {
			return 0, err
		}
		return int(v.Int64()), nil
	}
	parts := make([]string, 0, 4)
	for i := 0; i < 3; i++ {
		k, err := pick(len(tempWords))
		if err != nil {
			return "", err
		}
		parts = append(parts, tempWords[k])
	}
	d, err := pick(100)
	if err != nil {
		return "", err
	}
	parts = append(parts, fmt.Sprintf("%02d", d))
	p := strings.Join(parts, "-")
	if len(p) > MaxPasswordLen {
		return "", fmt.Errorf("a temporary password came out %d characters long", len(p))
	}
	return p, nil
}
