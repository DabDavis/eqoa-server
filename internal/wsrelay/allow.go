package wsrelay

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// Rule lets the relay reach every address in Net on the ports Lo..Hi.
type Rule struct {
	Net    netip.Prefix
	Lo, Hi uint16
}

// AllowList is what the relay may reach. It is checked per host AND port: a
// relay on a host that runs other services on loopback must not become a
// way into them.
type AllowList []Rule

// ParseAllow reads rules of the form HOST:PORT or HOST:LO-HI, where HOST is
// an IPv4 address, an IPv4 network (203.0.113.0/24) or a name, resolved once
// here to every IPv4 address it has. An entry may hold several rules
// separated by commas.
func ParseAllow(specs []string) (AllowList, error) {
	var out AllowList
	for _, spec := range specs {
		for _, s := range strings.Split(spec, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			rules, err := parseRule(s)
			if err != nil {
				return nil, err
			}
			out = append(out, rules...)
		}
	}
	return out, nil
}

func parseRule(s string) ([]Rule, error) {
	i := strings.LastIndexByte(s, ':')
	if i <= 0 || i == len(s)-1 {
		return nil, fmt.Errorf("allow %q: want HOST:PORT or HOST:LO-HI", s)
	}
	host, ports := s[:i], s[i+1:]
	lo, hi, err := parsePorts(ports)
	if err != nil {
		return nil, fmt.Errorf("allow %q: %v", s, err)
	}
	var nets []netip.Prefix
	switch {
	case strings.Contains(host, "/"):
		p, err := netip.ParsePrefix(host)
		if err != nil || !p.Addr().Is4() {
			return nil, fmt.Errorf("allow %q: %q is not an IPv4 network", s, host)
		}
		nets = append(nets, p.Masked())
	default:
		if a, err := netip.ParseAddr(host); err == nil {
			if !a.Is4() {
				return nil, fmt.Errorf("allow %q: only IPv4 is relayed", s)
			}
			nets = append(nets, netip.PrefixFrom(a, 32))
			break
		}
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, fmt.Errorf("allow %q: %v", s, err)
		}
		for _, ip := range ips {
			if a, ok := netip.AddrFromSlice(ip); ok && a.Unmap().Is4() {
				nets = append(nets, netip.PrefixFrom(a.Unmap(), 32))
			}
		}
		if len(nets) == 0 {
			return nil, fmt.Errorf("allow %q: %s has no IPv4 address", s, host)
		}
	}
	rules := make([]Rule, len(nets))
	for k, n := range nets {
		rules[k] = Rule{Net: n, Lo: lo, Hi: hi}
	}
	return rules, nil
}

func parsePorts(s string) (lo, hi uint16, err error) {
	a, b, ranged := strings.Cut(s, "-")
	l, err1 := strconv.ParseUint(a, 10, 16)
	h := l
	var err2 error
	if ranged {
		h, err2 = strconv.ParseUint(b, 10, 16)
	}
	if err1 != nil || err2 != nil || l == 0 || h == 0 || h < l {
		return 0, 0, fmt.Errorf("bad port %q", s)
	}
	return uint16(l), uint16(h), nil
}

// Allows reports whether ip:port is reachable.
func (a AllowList) Allows(ip netip.Addr, port uint16) bool {
	ip = ip.Unmap()
	for _, r := range a {
		if r.Net.Contains(ip) && r.Lo <= port && port <= r.Hi {
			return true
		}
	}
	return false
}

// First is the address of the first rule, which the relay measures its own
// address toward (what a page is told is its local address), or false when
// the list is empty.
func (a AllowList) First() (netip.Addr, bool) {
	if len(a) == 0 {
		return netip.Addr{}, false
	}
	return a[0].Net.Addr(), true
}

func (a AllowList) String() string {
	parts := make([]string, len(a))
	for k, r := range a {
		host := r.Net.String()
		if r.Net.Bits() == 32 {
			host = r.Net.Addr().String()
		}
		if r.Lo == r.Hi {
			parts[k] = fmt.Sprintf("%s:%d", host, r.Lo)
		} else {
			parts[k] = fmt.Sprintf("%s:%d-%d", host, r.Lo, r.Hi)
		}
	}
	return strings.Join(parts, ", ")
}
