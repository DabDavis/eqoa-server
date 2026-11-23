// Package mailer sends the server's mail (the accounts' verification and
// password reset links, internal/accountmail): through an SMTP relay, the
// box's own send-only Postfix on 127.0.0.1:25, or into the log where no mail
// is to go out. A message is queued and sent in the background, retried a few
// times; nothing that asks for one ever waits on the relay, and a relay that
// is down (the provider's port 25 still shut) costs a log line, never a page.
package mailer

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

// Message is one mail: to one address, a subject and plain text.
type Message struct {
	To      string
	Subject string
	Text    string
}

// Sender delivers one message now, or says why not.
type Sender interface {
	Send(Message) error
}

// SMTP is a Sender through a relay that takes mail from this host with no
// login (Postfix's mynetworks): Addr is host:port, From the envelope and
// header sender.
type SMTP struct {
	Addr    string
	From    string
	Timeout time.Duration // per message; unset: 20 s
}

// Send requires verified STARTTLS for connections outside loopback. Account
// verification and password reset links are bearer credentials.
func (s SMTP) Send(m Message) error {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	conn, err := net.DialTimeout("tcp", s.Addr, timeout)
	if err != nil {
		return err
	}
	conn.SetDeadline(time.Now().Add(timeout))
	host, _, _ := net.SplitHostPort(s.Addr)
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if err := c.Hello(helloName(s.From)); err != nil {
		return err
	}
	if !conn.RemoteAddr().(*net.TCPAddr).IP.IsLoopback() {
		if err := requireTLS(c, host); err != nil {
			return err
		}
	}
	if err := c.Mail(s.From); err != nil {
		return err
	}
	if err := c.Rcpt(m.To); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(Compose(s.From, m, time.Now())); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func requireTLS(c *smtp.Client, host string) error {
	if ok, _ := c.Extension("STARTTLS"); !ok {
		return errors.New("remote SMTP relay requires STARTTLS")
	}
	return c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
}

// helloName is the name EHLO gives: the sender's domain.
func helloName(from string) string {
	if i := strings.LastIndexByte(from, '@'); i >= 0 && i+1 < len(from) {
		return from[i+1:]
	}
	return "localhost"
}

// Compose is a message as it goes over the wire: headers, a blank line and
// the text, every line ending CRLF and none starting with a lone dot left for
// the relay to eat (textproto's writer dot-stuffs; this keeps the text whole).
func Compose(from string, m Message, now time.Time) []byte {
	var b bytes.Buffer
	id := make([]byte, 12)
	rand.Read(id)
	hdr := func(k, v string) {
		b.WriteString(k + ": " + v + "\r\n")
	}
	hdr("From", "EQOA Classic <"+from+">")
	hdr("To", m.To)
	hdr("Subject", mime.QEncoding.Encode("utf-8", oneLine(m.Subject)))
	hdr("Date", now.UTC().Format(time.RFC1123Z))
	hdr("Message-ID", "<"+hex.EncodeToString(id)+"@"+helloName(from)+">")
	hdr("MIME-Version", "1.0")
	hdr("Content-Type", "text/plain; charset=utf-8")
	hdr("Content-Transfer-Encoding", "8bit")
	hdr("Auto-Submitted", "auto-generated")
	b.WriteString("\r\n")
	text := strings.ReplaceAll(strings.ReplaceAll(m.Text, "\r\n", "\n"), "\r", "\n")
	for _, line := range strings.Split(text, "\n") {
		b.WriteString(line + "\r\n")
	}
	return b.Bytes()
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Log is a Sender that writes each message to the log: to whom (masked) and
// its subject, and with Bodies its whole text (a developer's box: the links
// in it are live credentials).
type Log struct {
	Bodies bool
}

func (l Log) Send(m Message) error {
	if l.Bodies {
		log.Printf("Mail (not sent, log only) to %s: %q\n%s", Mask(m.To), m.Subject, m.Text)
	} else {
		log.Printf("Mail (not sent, log only) to %s: %q", Mask(m.To), m.Subject)
	}
	return nil
}

// Memory is a Sender for tests: it keeps what it was handed, and Fail makes
// the next sends fail.
type Memory struct {
	mu   sync.Mutex
	Sent []Message
	Fail int
}

func (m *Memory) Send(msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail > 0 {
		m.Fail--
		return errors.New("refused (test)")
	}
	m.Sent = append(m.Sent, msg)
	return nil
}

// All is a copy of what was sent.
func (m *Memory) All() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Message(nil), m.Sent...)
}

// Queue sends in the background: Post never blocks; a message that fails is
// tried again after each of Backoff, then dropped with a log line.
type Queue struct {
	sender  Sender
	ch      chan Message
	pending chan struct{} // counts queued, sending and retrying messages
	backoff []time.Duration
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
	wg      sync.WaitGroup
}

// DefaultBackoff is the waits before each retry: a relay restarting, or the
// provider's port opening, is caught; a dead one is given up on in a day's
// first hour.
var DefaultBackoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute}

// NewQueue starts a queue in front of sender holding at most size messages
// (unset: 256) and retrying after each of backoff (nil: DefaultBackoff).
func NewQueue(sender Sender, size int, backoff []time.Duration) *Queue {
	if size <= 0 {
		size = 256
	}
	if backoff == nil {
		backoff = DefaultBackoff
	}
	q := &Queue{sender: sender, ch: make(chan Message, size), pending: make(chan struct{}, size+1), backoff: backoff,
		stop: make(chan struct{}), done: make(chan struct{})}
	go q.run()
	return q
}

// Post queues m and says whether it was taken (false: the queue is full or
// stopped).
func (q *Queue) Post(m Message) bool {
	select {
	case <-q.stop:
		return false
	default:
	}
	// Keep a place for this message through its last retry. A failing relay
	// otherwise moves the whole queue into unbounded sleeping goroutines.
	select {
	case q.pending <- struct{}{}:
	default:
		log.Printf("Mail: the queue is full; a message to %s (%q) is dropped", Mask(m.To), m.Subject)
		return false
	}
	select {
	case q.ch <- m:
		return true
	default:
		<-q.pending
		log.Printf("Mail: the queue is full; a message to %s (%q) is dropped", Mask(m.To), m.Subject)
		return false
	}
}

func (q *Queue) run() {
	defer close(q.done)
	for {
		select {
		case <-q.stop:
			return
		case m := <-q.ch:
			if err := q.sender.Send(m); err != nil {
				log.Printf("Mail: to %s (%q) not sent: %v; retrying", Mask(m.To), m.Subject, err)
				q.wg.Add(1)
				go q.retry(m)
			} else {
				<-q.pending
			}
		}
	}
}

func (q *Queue) retry(m Message) {
	defer q.wg.Done()
	defer func() { <-q.pending }()
	for i, wait := range q.backoff {
		select {
		case <-q.stop:
			return
		case <-time.After(wait):
		}
		err := q.sender.Send(m)
		if err == nil {
			log.Printf("Mail: to %s (%q) sent at try %d", Mask(m.To), m.Subject, i+2)
			return
		}
		if i == len(q.backoff)-1 {
			log.Printf("Mail: to %s (%q) given up after %d tries: %v", Mask(m.To), m.Subject, i+2, err)
		}
	}
}

// Stop ends the queue: what waits in it is not sent.
func (q *Queue) Stop() {
	q.once.Do(func() { close(q.stop) })
	<-q.done
	q.wg.Wait()
}

// Mask is an address as anyone but its owner may see it: the first letter of
// the name and of the domain, the rest starred, the top-level domain kept
// ("d***@g***.com"). Something that is no address is "***".
func Mask(addr string) string {
	at := strings.LastIndexByte(addr, '@')
	if at <= 0 || at == len(addr)-1 {
		return "***"
	}
	local, domain := addr[:at], addr[at+1:]
	first := func(s string) string {
		for _, r := range s {
			return string(r) + "***"
		}
		return "***"
	}
	tld := ""
	if dot := strings.LastIndexByte(domain, '.'); dot > 0 {
		tld, domain = domain[dot:], domain[:dot]
	}
	return first(local) + "@" + first(domain) + tld
}

// MaxLen is the longest address kept: STATION's field is 128 bytes with its
// NUL, and RFC 5321's path is 254 at most.
const MaxLen = 127

// Normalize is an address as it is kept: trimmed, lower case, checked to be
// one plain address (a name and a domain with a dot, letters, digits and the
// usual punctuation, no display name, no spaces, at most MaxLen). The error
// says what is wrong, for the player.
func Normalize(addr string) (string, error) {
	a := strings.ToLower(strings.TrimSpace(addr))
	switch {
	case a == "":
		return "", errors.New("type your e-mail address")
	case len(a) > MaxLen:
		return "", fmt.Errorf("an e-mail address is at most %d characters here", MaxLen)
	}
	at := strings.LastIndexByte(a, '@')
	if at <= 0 || at != strings.IndexByte(a, '@') {
		return "", errors.New("an e-mail address has one @, with your name before it and the domain after it")
	}
	local, domain := a[:at], a[at+1:]
	if len(local) > 64 {
		return "", errors.New("the part before the @ is at most 64 characters")
	}
	for _, r := range local {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune(".!#$%&'*+/=?^_`{|}~-", r)) {
			return "", errors.New("that e-mail address has a character it cannot have")
		}
	}
	if local[0] == '.' || local[len(local)-1] == '.' || strings.Contains(local, "..") {
		return "", errors.New("that e-mail address has a dot out of place")
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", errors.New("the domain after the @ needs a dot (gmail.com, say)")
	}
	for _, l := range labels {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return "", errors.New("the domain after the @ is not one")
		}
		for _, r := range l {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return "", errors.New("the domain after the @ has a character it cannot have")
			}
		}
	}
	if tld := labels[len(labels)-1]; len(tld) < 2 || strings.Trim(tld, "0123456789") == "" {
		return "", errors.New("the domain after the @ is not one")
	}
	return a, nil
}
