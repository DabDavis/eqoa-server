package tcp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/accountmail"
	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/mailer"
)

// STATION's account forms with e-mail on (mail.accounts; stationpages.go has
// the pages served for them). Each is a request on the auth connection:
//
//   - 0x38 SubmitAcctCreate (476 bytes): the account made here, with its
//     password (DES, as the login's) and its e-mail, which is required, and
//     its verify link mailed at once. Answered 0x29: success, or a refusal
//     STATION shows as it stands (a name taken says "exists", which STATION
//     answers with its own word 1371 and the page before).
//   - 0x40 the account update (208 bytes: the id, a country, a postal code and
//     an e-mail): the address of the account this connection logged in as is
//     changed and mailed its link. Answered 0x41 (word 1345 "Successfully
//     updated your account info", else 1254, an error).
//   - 0x42 the forgotten password (268 bytes, its fields AES-256 under the
//     session key the hello carries): a reset link mailed when the user name's
//     verified address is the one typed. Answered 0x43 with 1 whatever
//     happened (word 1396 "Your new password has been e-mailed to you."), so
//     it says nothing of an account.
//
// With e-mail off nothing here runs: 0x38 is answered success and makes
// nothing (the follow-up login registers the account, auth.allow_auto_register),
// and 0x40 and 0x42 end the connection, as before.

const (
	MsgAccountUpdate         = 0x40
	MsgAccountUpdateResponse = 0x41
	MsgForgotPassword        = 0x42
	MsgForgotPasswordReply   = 0x43
)

// stationCreatePerDay is the most accounts STATION makes from one address a
// day (an exempt address aside).
const stationCreatePerDay = 3

// SetAccountMail turns STATION's e-mail forms on (nil: off).
func (s *AuthServer) SetAccountMail(m *accountmail.Service, db *database.SQLiteDB) {
	s.mail, s.mailDB = m, db
}

// stationMailState is the auth server's e-mail state.
type stationMailState struct {
	mail   *accountmail.Service
	mailDB *database.SQLiteDB

	createdMu sync.Mutex
	created   map[string][]time.Time
}

// sessionKey is the AES key the hello carries: serverHelloBlob under the
// login's DES (des_soe_decrypt, which STATION's OnSessionKey runs).
func sessionKey() ([]byte, error) {
	key, err := hex.DecodeString(desKeyHex)
	if err != nil {
		return nil, err
	}
	block, err := des.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(serverHelloBlob))
	cipher.NewCBCDecrypter(block, key).CryptBlocks(out, serverHelloBlob)
	return out, nil
}

// aesField is a field STATION wrote with aes_ecb_encrypt_buffer under the
// session key, read to its NUL.
func aesField(field []byte) (string, error) {
	if len(field) == 0 || len(field)%16 != 0 {
		return "", errors.New("not whole AES blocks")
	}
	key, err := sessionKey()
	if err != nil {
		return "", err
	}
	c, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	plain := make([]byte, len(field))
	for i := 0; i < len(field); i += 16 {
		c.Decrypt(plain[i:i+16], field[i:i+16])
	}
	return extractString(plain), nil
}

// buildCreateReply is 0x29: 276 bytes, the success word at 12 and a text at
// 20 (STATION's VIStationReply41: success, code, text[256]).
func buildCreateReply(ok bool, text string) []byte {
	response := make([]byte, 276)
	binary.BigEndian.PutUint32(response[0:4], 276)
	binary.BigEndian.PutUint32(response[4:8], MsgAcctCreateResponse)
	if ok {
		binary.BigEndian.PutUint32(response[12:16], 1)
	}
	if len(text) > 255 {
		text = text[:255]
	}
	copy(response[20:276], text)
	return response
}

// buildOneReply is 0x41 or 0x43: 16 bytes, the word at 12.
func buildOneReply(op uint32, a uint32) []byte {
	b := make([]byte, 16)
	binary.BigEndian.PutUint32(b[0:4], 16)
	binary.BigEndian.PutUint32(b[4:8], op)
	binary.BigEndian.PutUint32(b[12:16], a)
	return b
}

// allowCreate counts an account made from ip and says whether it is within
// stationCreatePerDay; nil and exempt addresses are not counted.
func (s *AuthServer) allowCreate(ip net.IP) bool {
	if ip == nil || s.exemptIP(ip) {
		return true
	}
	key := accountmailAddrKey(ip)
	s.createdMu.Lock()
	defer s.createdMu.Unlock()
	if s.created == nil {
		s.created = map[string][]time.Time{}
	}
	now := time.Now()
	cut := now.Add(-24 * time.Hour)
	ts := s.created[key]
	i := 0
	for i < len(ts) && ts[i].Before(cut) {
		i++
	}
	ts = ts[i:]
	if len(ts) >= stationCreatePerDay {
		s.created[key] = ts
		return false
	}
	s.created[key] = append(ts, now)
	return true
}

func accountmailAddrKey(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// validNewAccount is the account pages' rule for a new account's name and
// password, for STATION's form too.
func validNewAccount(username, password string) error {
	if len(username) < 3 || len(username) > 16 {
		return fmt.Errorf("A user name is 3 to 16 characters.")
	}
	for _, c := range username {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return fmt.Errorf("A user name is letters, numbers and _ alone.")
		}
	}
	if len(password) < 4 {
		return fmt.Errorf("A password is at least 4 characters.")
	}
	if len(password) > auth.MaxPasswordLen {
		return fmt.Errorf("A password is at most %d characters.", auth.MaxPasswordLen)
	}
	return nil
}

// handleCreateAccountMail is 0x38 with e-mail on: the account made, with its
// address, and the link mailed.
func (s *AuthServer) handleCreateAccountMail(payload []byte, ip net.IP) []byte {
	if len(payload) < 340+128 {
		log.Printf("Auth: create account payload too short (%d bytes)", len(payload))
		return buildCreateReply(false, "The form did not arrive whole. Try again.")
	}
	username := strings.TrimSpace(extractString(payload[4:36]))
	password, err := decryptPassword(payload[36:68])
	if err != nil {
		log.Printf("Auth: create account for '%s': the password field: %v", username, err)
		return buildCreateReply(false, "The password could not be read. Use letters and numbers.")
	}
	raw := extractString(payload[340:468])
	if err := validNewAccount(username, password); err != nil {
		return buildCreateReply(false, err.Error())
	}
	// No usable address (the owner, 2026-11-16: "players are getting stuck on zip code and extra fields"): a
	// client that has not patched yet shows the disc's own Create Account (zip code and the rest, no e-mail at
	// this offset), and refusing it left them nowhere to go. The account is made without an address; the world's
	// reminders and mail.require_after ask for one, as for every account made before e-mail.
	email, err := mailer.Normalize(raw)
	if err != nil {
		log.Printf("Auth: create account for '%s': no usable e-mail (%q: %v): made without one, to be added on the website", username, strings.TrimSpace(raw), err)
		email = ""
	}
	if _, err := s.store.GetByUsername(username); err == nil {
		log.Printf("Auth: create account: '%s' exists already", username)
		return buildCreateReply(false, "User Name already exists.")
	} else if !errors.Is(err, database.ErrAccountNotFound) {
		log.Printf("Auth: create account for '%s': %v", username, err)
		return buildCreateReply(false, "The server could not make the account now. Try again later.")
	}
	if !s.allowCreate(ip) {
		log.Printf("Auth: create account for '%s' from %v refused: %d made from there today", username, ip, stationCreatePerDay)
		return buildCreateReply(false, "That is a lot of new accounts from here today. Try again tomorrow.")
	}
	hash, err := auth.HashPassword(password, s.cfg.Auth.BCryptCost)
	if err != nil {
		log.Printf("Auth: create account for '%s': hashing: %v", username, err)
		return buildCreateReply(false, "The server could not make the account now. Try again later.")
	}
	acc, err := s.store.Create(username, hash)
	if err != nil {
		log.Printf("Auth: create account for '%s': %v", username, err)
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return buildCreateReply(false, "User Name already exists.")
		}
		return buildCreateReply(false, "The server could not make the account now. Try again later.")
	}
	ipText := ""
	if ip != nil {
		ipText = ip.String()
	}
	if email == "" {
		log.Printf("Auth: account '%s' (ID: %d) made by STATION from %v without an e-mail", username, acc.ID, ip)
		return buildCreateReply(true, "")
	}
	masked, err := s.mail.SetEmail(acc.ID, email, ipText)
	if err != nil {
		// the account stands (its login follows at once); the address is
		// set on the website
		log.Printf("Auth: account '%s' (ID: %d) made by STATION; its e-mail not kept: %v", username, acc.ID, err)
	} else {
		log.Printf("Auth: account '%s' (ID: %d) made by STATION from %v, its verify link sent to %s", username, acc.ID, ip, masked)
	}
	return buildCreateReply(true, "")
}

// handleAccountUpdate is 0x40: the e-mail of the account the connection
// logged in as (loggedIn, 0 for none).
func (s *AuthServer) handleAccountUpdate(payload []byte, loggedIn int, ip net.IP) []byte {
	if len(payload) < 200 {
		return buildOneReply(MsgAccountUpdateResponse, 0)
	}
	id := int(int32(binary.BigEndian.Uint32(payload[4:8])))
	if loggedIn <= 0 || id != loggedIn {
		log.Printf("Auth: account update for %d refused: the connection logged in as %d", id, loggedIn)
		return buildOneReply(MsgAccountUpdateResponse, 0)
	}
	if on, err := s.mailDB.HasAuthenticator(id); err != nil || on {
		log.Printf("Auth: account update for %d refused: it has an authenticator (the website asks its code)", id)
		return buildOneReply(MsgAccountUpdateResponse, 0)
	}
	ipText := ""
	if ip != nil {
		ipText = ip.String()
	}
	masked, err := s.mail.SetEmail(id, extractString(payload[72:200]), ipText)
	if err != nil {
		log.Printf("Auth: account update for %d: %v", id, err)
		return buildOneReply(MsgAccountUpdateResponse, 0)
	}
	log.Printf("Auth: account %d set its e-mail in STATION; link to %s", id, masked)
	return buildOneReply(MsgAccountUpdateResponse, 1)
}

// handleForgotPassword is 0x42: a reset link for the user name, when its
// verified address is the one typed; answered 1 whatever happened.
func (s *AuthServer) handleForgotPassword(payload []byte, ip net.IP) []byte {
	answer := buildOneReply(MsgForgotPasswordReply, 1)
	if len(payload) < 260 {
		return answer
	}
	user, err1 := aesField(payload[4:36])
	email, err2 := aesField(payload[132:260])
	if err1 != nil || err2 != nil {
		log.Printf("Auth: a forgotten password form that would not decrypt")
		return answer
	}
	acc, err := s.store.GetByUsername(strings.TrimSpace(user))
	if err != nil {
		log.Printf("Auth: a forgotten password for no such account")
		return answer
	}
	st, err := s.mailDB.AccountEmail(acc.ID)
	if err != nil {
		log.Printf("Auth: a forgotten password for '%s': %v", acc.Username, err)
		return answer
	}
	ipText := ""
	if ip != nil {
		ipText = ip.String()
	}
	if !s.mail.SendReset(st, email, ipText) {
		log.Printf("Auth: a forgotten password for '%s' from STATION: no link sent", acc.Username)
	}
	return answer
}
