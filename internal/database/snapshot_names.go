package database

import (
	"regexp"
	"strings"
	"unicode"
)

// What a table or a column is called, read for what it might hold: the
// sanitiser (snapshot.go) refuses a snapshot when a table it does not know,
// or a column it has no rule for, is named like a secret, so that a table
// added tomorrow cannot leak by default. A name it flags wrongly costs a rule
// (allow, with the reason); a name it misses is a leak, so it flags widely.

// secretStems are flagged anywhere in a name, whatever the case: "pwhash",
// "addr_hash", "rowkey", "api_key", "lastlogin" and "birthyear" all carry one.
var secretStems = []string{
	"passw", "passwd", "passphrase", "passcode", "pwhash", "pw_hash", "pwd",
	"secret", "token", "hash", "salt", "key", "ticket", "cookie", "session",
	"credential", "nonce", "oauth", "totp", "hotp", "bearer", "jwt", "private", "signing",
	"mail", "addr", "login", "phone", "birth", "firstname", "lastname", "surname", "realname",
	"fullname", "device", "install",
}

// secretWords are flagged as a whole word of a name (split at '_' and other
// punctuation and at a lower-to-upper case change), so "ip" is "last_ip" and
// "ipAddress" but not "equipslot", "recipe" or "description".
var secretWords = map[string]bool{
	"ip": true, "ips": true, "ipv4": true, "ipv6": true, "ipaddr": true, "ipaddress": true,
	"pw": true, "pass": true, "otp": true, "auth": true, "mac": true, "zip": true, "dob": true,
}

// ipCompound catches an address in a name written in one lower-case word:
// "ipaddress", "lastip", "remoteip", "clientip".
var ipCompound = regexp.MustCompile(`^ip(v[46]|addr|address|s)?$|ipv[46]|ip_?addr|` +
	`(last|remote|client|sender|peer|src|source|real|from|user|login|origin|host|my|their)_?ips?$`)

// nameWords splits a name into its lower-case words.
func nameWords(name string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	runes := []rune(name)
	for i, r := range runes {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if unicode.IsUpper(r) && i > 0 && unicode.IsLower(runes[i-1]) {
				flush()
			}
			cur = append(cur, r)
		default:
			flush()
		}
	}
	flush()
	return words
}

// SecretLikeName is whether a table or column name reads like it holds a
// credential or a person's data: what the sanitiser must have a rule for.
func SecretLikeName(name string) bool {
	low := strings.ToLower(name)
	for _, s := range secretStems {
		if strings.Contains(low, s) {
			return true
		}
	}
	for _, w := range nameWords(name) {
		if secretWords[w] || ipCompound.MatchString(w) {
			return true
		}
	}
	return false
}
