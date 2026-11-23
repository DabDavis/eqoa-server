package mailer

import (
	"bufio"
	"errors"
	"net"
	"net/smtp"
	"strings"
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	good := map[string]string{
		"  Player.One@Example.COM ": "player.one@example.com",
		"a+tag@mail.example.org":    "a+tag@mail.example.org",
		"x_y-z@sub-domain.example":  "x_y-z@sub-domain.example",
	}
	for in, want := range good {
		got, err := Normalize(in)
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "nobody", "a@b", "a@@example.com", "a@b@example.com", "Name <a@example.com>",
		"a b@example.com", ".a@example.com", "a..b@example.com", "a@-example.com", "a@example.c", "a@example.123",
		"a@exa_mple.com", strings.Repeat("a", 65) + "@example.com", "a@" + strings.Repeat("b", 130) + ".com", "ünï@example.com"} {
		if got, err := Normalize(bad); err == nil {
			t.Errorf("Normalize(%q) = %q, want an error", bad, got)
		}
	}
}

func TestMask(t *testing.T) {
	for in, want := range map[string]string{
		"dave@game-mail.example":   "d***@g***.example",
		"a@example.co.uk":          "a***@e***.uk",
		"player@localhost":         "p***@l***",
		"nothing":                  "***",
		"@example.com":             "***",
		"someone@mail.example.org": "s***@m***.org",
	} {
		if got := Mask(in); got != want {
			t.Errorf("Mask(%q) = %q, want %q", in, got, want)
		}
	}
	if m := Mask("secret.name@private-domain.example"); strings.Contains(m, "secret") || strings.Contains(m, "private") {
		t.Errorf("the mask shows the address: %q", m)
	}
}

func TestComposeIsCRLFAndHeaders(t *testing.T) {
	b := string(Compose("no-reply@example.com", Message{To: "p@example.com", Subject: "Hi\r\nBcc: x@example.com", Text: "one\ntwo"}, time.Unix(0, 0)))
	if strings.Contains(b, "\nBcc:") {
		t.Fatalf("a subject's line break made a header:\n%s", b)
	}
	if !strings.Contains(b, "To: p@example.com\r\n") || !strings.HasSuffix(b, "\r\n\r\none\r\ntwo\r\n") {
		t.Fatalf("composed:\n%q", b)
	}
}

func TestQueueRetriesAndNeverBlocks(t *testing.T) {
	m := &Memory{Fail: 2}
	q := NewQueue(m, 1, []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond})
	if !q.Post(Message{To: "a@example.com", Subject: "one"}) {
		t.Fatal("not taken")
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(m.All()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := m.All(); len(got) != 1 || got[0].Subject != "one" {
		t.Fatalf("sent %v", got)
	}
	// a full queue drops and says so at once
	blocked := make(chan struct{})
	slow := NewQueue(senderFunc(func(Message) error { <-blocked; return nil }), 1, nil)
	slow.Post(Message{To: "a@example.com"}) // taken by the worker
	time.Sleep(10 * time.Millisecond)
	slow.Post(Message{To: "a@example.com"}) // waits in the queue
	start := time.Now()
	if slow.Post(Message{To: "a@example.com"}) {
		t.Error("a full queue took a message")
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Error("Post blocked")
	}
	close(blocked)
	slow.Stop()
	q.Stop()
}

type senderFunc func(Message) error

func (f senderFunc) Send(m Message) error { return f(m) }

// The SMTP sender against a fake relay on loopback: no auth, no STARTTLS.
func TestSMTPSend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		w := func(s string) { c.Write([]byte(s + "\r\n")) }
		w("220 relay ESMTP")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					w("250 queued")
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"):
				w("250-relay")
				w("250 8BITMIME")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				w("250 ok")
			case cmd == "DATA":
				inData = true
				w("354 go")
			case cmd == "QUIT":
				w("221 bye")
				got <- data.String()
				return
			default:
				w("500 what")
			}
		}
	}()
	s := SMTP{Addr: ln.Addr().String(), From: "no-reply@example.com", Timeout: 5 * time.Second}
	if err := s.Send(Message{To: "p@example.com", Subject: "Verify", Text: "the link"}); err != nil {
		t.Fatal(err)
	}
	select {
	case d := <-got:
		if !strings.Contains(d, "Subject: Verify") || !strings.Contains(d, "the link") {
			t.Fatalf("the relay got %q", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no mail at the relay")
	}
	// a relay that is not there is an error, not a hang
	if err := (SMTP{Addr: "127.0.0.1:1", From: "no-reply@example.com", Timeout: time.Second}).Send(Message{To: "p@example.com"}); err == nil {
		t.Fatal("no error for a closed port")
	}
}

func TestRemoteSMTPRequiresSTARTTLS(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		defer server.Close()
		server.Write([]byte("220 relay ESMTP\r\n"))
		bufio.NewReader(server).ReadString('\n') // EHLO
		server.Write([]byte("250 relay\r\n"))
	}()
	c, err := smtp.NewClient(client, "relay.example")
	if err != nil {
		t.Fatal(err)
	}
	if err := requireTLS(c, "relay.example"); err == nil {
		t.Fatal("accepted remote SMTP without STARTTLS")
	}
}

func TestQueueBoundsOutstandingRetries(t *testing.T) {
	attempted := make(chan struct{}, 1)
	q := NewQueue(senderFunc(func(Message) error {
		attempted <- struct{}{}
		return errors.New("unavailable")
	}), 1, []time.Duration{time.Hour})
	defer q.Stop()
	for i := 0; i < 2; i++ {
		if !q.Post(Message{}) {
			t.Fatal("queue refused a message before reaching capacity")
		}
		<-attempted
	}
	if q.Post(Message{}) {
		t.Fatal("accepted another message while retries occupied every slot")
	}
}
