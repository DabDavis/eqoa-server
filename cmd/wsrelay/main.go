// Command wsrelay holds a browser page's UDP and TCP sockets behind
// WebSockets, so a web build of an EQOA client can reach this server: DRDP
// on the game port, the login on 9735 and the patch server's HTTP. The
// protocol is eqoa-port's tools/wsrelay.py; see internal/wsrelay and
// README.md here.
//
//	wsrelay -allow-udp 203.0.113.10:10070 -allow-tcp 203.0.113.10:9735,203.0.113.10:7000
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/eqoa/server/internal/wsrelay"
)

// list is a flag that may repeat and may hold commas.
type list []string

func (l *list) String() string { return strings.Join(*l, ",") }

func (l *list) Set(v string) error {
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			*l = append(*l, p)
		}
	}
	return nil
}

func main() {
	var allowUDP, allowTCP, origins list
	listen := flag.String("listen", "127.0.0.1:8790", "address to serve on (behind a TLS proxy: keep it on loopback)")
	flag.Var(&allowUDP, "allow-udp", "HOST:PORT or HOST:LO-HI the relay may send datagrams to and take them from (repeatable, comma-separated)")
	flag.Var(&allowTCP, "allow-tcp", "HOST:PORT or HOST:LO-HI the relay may connect to (repeatable, comma-separated)")
	flag.Var(&origins, "origin", "a page origin allowed to connect, e.g. https://example.org (repeatable; none: any)")
	maxPerIP := flag.Int("max-per-ip", 8, "sockets one client address may hold at once")
	udpIdle := flag.Duration("udp-idle", 120*time.Second, "close a UDP socket after this long with no datagram either way")
	tcpIdle := flag.Duration("tcp-idle", 10*time.Minute, "close a TCP connection after this long idle")
	ping := flag.Duration("ping", 30*time.Second, "WebSocket ping interval that notices a silent client (0: off)")
	verbose := flag.Bool("v", false, "log dropped datagrams and failed handshakes too")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: wsrelay -allow-udp HOST:PORT -allow-tcp HOST:PORT[,...] [flags]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	udp, err := wsrelay.ParseAllow(allowUDP)
	if err != nil {
		log.Fatal(err)
	}
	tcp, err := wsrelay.ParseAllow(allowTCP)
	if err != nil {
		log.Fatal(err)
	}
	if len(udp) == 0 && len(tcp) == 0 {
		flag.Usage()
		os.Exit(2)
	}
	p := *ping
	if p == 0 {
		p = -1
	}
	relay := wsrelay.New(wsrelay.Config{
		AllowUDP: udp,
		AllowTCP: tcp,
		Origins:  origins,
		MaxPerIP: *maxPerIP,
		UDPIdle:  *udpIdle,
		TCPIdle:  *tcpIdle,
		Ping:     p,
		Verbose:  *verbose,
	})
	o := "any origin"
	if len(origins) > 0 {
		o = "origins " + origins.String()
	}
	log.Printf("wsrelay: ws://%s, udp to %s, tcp to %s, %s, %d sockets per client", *listen, orNone(udp), orNone(tcp), o, *maxPerIP)
	srv := &http.Server{Addr: *listen, Handler: relay, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func orNone(a wsrelay.AllowList) string {
	if len(a) == 0 {
		return "nothing"
	}
	return a.String()
}
