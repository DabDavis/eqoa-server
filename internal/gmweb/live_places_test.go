package gmweb

import "testing"

// placetexts and lightspheres rows hold what the game can show and travel to;
// the controls pass.
func TestCheckPlaceRows(t *testing.T) {
	good := map[string]any{"world": 0.0, "x1": 1.0, "x2": 2.0, "z1": 1.0, "z2": 2.0, "text": "Commotion."}
	if err := checkRow("placetexts", good); err != nil {
		t.Errorf("a good place text: %v", err)
	}
	for _, bad := range []map[string]any{
		{"world": 0.0, "x1": 3.0, "x2": 2.0, "z1": 1.0, "z2": 2.0, "text": "a"},
		{"world": 0.0, "x1": 1.0, "x2": 2.0, "z1": 1.0, "z2": 2.0, "text": ""},
		{"world": 0.0, "x1": 1.0, "x2": 2.0, "z1": 1.0, "z2": 2.0, "text": "a |x"},
	} {
		if err := checkRow("placetexts", bad); err == nil {
			t.Errorf("place text %v passed", bad)
		}
	}
	sphere := map[string]any{"world": 0.0, "x": 4503.2, "z": 21550.0, "line": "This sphere of light is used to teleport.",
		"to_world": 0.0, "to_x": 4185.5, "to_y": 128.3, "to_z": 21547.5, "to_facing": 130.0}
	if err := checkRow("lightspheres", sphere); err != nil {
		t.Errorf("a good sphere: %v", err)
	}
	for col, v := range map[string]any{"line": "", "to_facing": 256.0, "answer": "100%"} {
		bad := map[string]any{}
		for k, x := range sphere {
			bad[k] = x
		}
		bad[col] = v
		if err := checkRow("lightspheres", bad); err == nil {
			t.Errorf("sphere with %s %v passed", col, v)
		}
	}
}
