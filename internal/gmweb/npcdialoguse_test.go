package gmweb

import "testing"

// npcdialog.use holds its three values in the panel: empty, ambient and shop pass; anything else (a misspelling,
// another case, a number) is refused; a row that does not name the column passes (the control).
func TestCheckRowHoldsNPCDialogUse(t *testing.T) {
	for _, u := range []any{"", "ambient", "shop", nil, []byte("shop")} {
		if err := checkRow("npcdialog", map[string]any{"line": "Hello.", "use": u}); err != nil {
			t.Errorf("use %v: %v", u, err)
		}
	}
	for _, u := range []any{"Shop", "ambiant", "talk", 1} {
		if err := checkRow("npcdialog", map[string]any{"line": "Hello.", "use": u}); err == nil {
			t.Errorf("use %v: taken", u)
		}
	}
	if err := checkRow("npcdialog", map[string]any{"line": "Hello."}); err != nil {
		t.Errorf("no use: %v", err)
	}
}
