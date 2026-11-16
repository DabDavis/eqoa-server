package tcp

import (
	"regexp"
	"strings"
	"testing"
)

// Every hidden field passes the checks STATION's forms run on it
// (viwndgenericrenderer.cpp): CheckName (a letter or a digit), and for the
// forms that check a postal code (IDCreat2's SubmitAcctCreate, UpAcct's
// SubmitAcctUpdate) IsZip, five digits, which applies whenever the Country
// begins "US -": PushPage sets a US console's Country so whatever the page
// holds. A hidden "x" there stopped every US player at Create Account
// (the owner, 2026-11-16).
func TestStationHiddenFieldsPassTheFormsChecks(t *testing.T) {
	reHidden := regexp.MustCompile(`<Text name="([A-Za-z0-9]+)" val="([^"]*)">\s*<Position x="0" y="0"/>`)
	isZip := regexp.MustCompile(`^[0-9]{5}$`)
	checksZip := map[string]bool{"data/ui/IDCreat2.xml": true, "data/ui/UpAcct.xml": true}
	for name, p := range stationEmailPages("eqoaclassic.com/eqoa/password", true) {
		hidden := map[string]string{}
		for _, m := range reHidden.FindAllStringSubmatch(string(p), -1) {
			hidden[m[1]] = m[2]
			if strings.Trim(strings.ToLower(m[2]), "abcdefghijklmnopqrstuvwxyz0123456789") == m[2] {
				t.Errorf("%s: %s=%q fails CheckName", name, m[1], m[2])
			}
		}
		if checksZip[name] {
			if z, ok := hidden["PostalCode"]; !ok || !isZip.MatchString(z) {
				t.Errorf("%s: PostalCode %q is no zip (IsZip, under a US console's Country)", name, z)
			}
		}
	}
}
