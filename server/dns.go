package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"emoserver/backend"
)

// A tiny DNS helper, so pointing the robot at this server needs one setting on the router (or none, if the robot's DNS is this machine).
// It answers the names listed in dns_names with dns_answer_ip and passes every other question to dns_upstream (usually the router's own
// resolver). With no upstream it refuses other names, so it can also be a dedicated resolver for the robot alone.
// Only UDP, only plain questions (one per packet) - which is what ordinary clients, including small embedded ones, send.

type dnsHelper struct {
	conn     net.PacketConn
	names    map[string]bool
	ip       net.IP
	upstream string
	wg       sync.WaitGroup
}

const dnsMaxPacket = 512 + 3*256 // ordinary UDP answers are small; forwarded answers up to a bit more

func normName(n string) string { return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(n), ".")) }

func validDNSConfig(c Config) error {
	if c.DNSListen == "" {
		return nil
	}
	if len(c.DNSNames) == 0 {
		return errors.New("config: dns_listen needs dns_names (the host names to point at this server)")
	}
	ip := net.ParseIP(c.DNSAnswerIP)
	if ip == nil || ip.To4() == nil {
		return errors.New("config: dns_answer_ip must be this machine's IPv4 address on the robot's network")
	}
	for _, n := range c.DNSNames {
		n = normName(n)
		if n == "" || len(n) > 253 || strings.ContainsAny(n, " /\\:@") {
			return fmt.Errorf("config: dns_names entry %q is not a host name", n)
		}
	}
	if c.DNSUpstream != "" {
		if _, _, err := net.SplitHostPort(c.DNSUpstream); err != nil {
			return errors.New("config: dns_upstream must be host:port, e.g. 192.168.1.1:53")
		}
	}
	return nil
}

func startDNS(c Config) (*dnsHelper, error) {
	if c.DNSListen == "" {
		return nil, nil
	}
	if err := validDNSConfig(c); err != nil {
		return nil, err
	}
	conn, err := net.ListenPacket("udp", c.DNSListen)
	if err != nil {
		return nil, fmt.Errorf("dns: %w (port 53 needs root or: setcap cap_net_bind_service=+ep emoserver)", err)
	}
	d := &dnsHelper{conn: conn, names: map[string]bool{}, ip: net.ParseIP(c.DNSAnswerIP).To4(), upstream: c.DNSUpstream}
	for _, n := range c.DNSNames {
		d.names[normName(n)] = true
	}
	go d.serve()
	return d, nil
}

func (d *dnsHelper) addr() string { return d.conn.LocalAddr().String() }

func (d *dnsHelper) close() {
	d.conn.Close()
	d.wg.Wait()
}

func (d *dnsHelper) serve() {
	buf := make([]byte, 1500)
	for {
		n, from, err := d.conn.ReadFrom(buf)
		if err != nil {
			return
		}
		pkt := append([]byte(nil), buf[:n]...)
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			if resp := d.answer(pkt); resp != nil {
				d.conn.WriteTo(resp, from)
			}
		}()
	}
}

// question reads the single question of a standard query. ok=false for anything else.
func question(p []byte) (name string, qtype uint16, end int, ok bool) {
	if len(p) < 12 || p[2]&0x80 != 0 || (p[2]>>3)&0xF != 0 || binary.BigEndian.Uint16(p[4:]) != 1 { // a query, opcode 0, one question
		return "", 0, 0, false
	}
	i := 12
	var labels []string
	for {
		if i >= len(p) {
			return "", 0, 0, false
		}
		l := int(p[i])
		if l == 0 {
			i++
			break
		}
		if l&0xC0 != 0 || i+1+l > len(p) { // no compression in a question
			return "", 0, 0, false
		}
		labels = append(labels, string(p[i+1:i+1+l]))
		i += 1 + l
	}
	if i+4 > len(p) {
		return "", 0, 0, false
	}
	return normName(strings.Join(labels, ".")), binary.BigEndian.Uint16(p[i:]), i + 4, true
}

// reply builds a response to query p (only its header and question are echoed).
func reply(p []byte, end int, rcode byte, answer []byte) []byte {
	out := append([]byte(nil), p[:end]...)
	out[2] = 0x80 | (p[2] & 0x01) // a response; keep the recursion-desired bit
	out[3] = 0x80 | rcode         // recursion available
	binary.BigEndian.PutUint16(out[6:], 0)
	if answer != nil {
		binary.BigEndian.PutUint16(out[6:], 1)
	}
	for i := 8; i < 12; i++ {
		out[i] = 0
	}
	return append(out, answer...)
}

func (d *dnsHelper) answer(p []byte) []byte {
	name, qtype, end, ok := question(p)
	if !ok {
		return nil
	}
	if d.names[name] {
		if qtype == 1 { // A
			rr := []byte{0xC0, 0x0C, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, d.ip[0], d.ip[1], d.ip[2], d.ip[3]}
			return reply(p, end, 0, rr)
		}
		return reply(p, end, 0, nil) // AAAA and the rest: the name exists, but has no such record
	}
	if backend.BlockedHost(name) { // the robot vendor's names are never looked up for real, whatever the upstream would say
		return reply(p, end, 3, nil) // NXDOMAIN
	}
	if d.upstream == "" {
		return reply(p, end, 5, nil) // refused
	}
	conn, err := net.DialTimeout("udp", d.upstream, 2*time.Second)
	if err != nil {
		return reply(p, end, 2, nil)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(p); err != nil {
		return reply(p, end, 2, nil)
	}
	buf := make([]byte, dnsMaxPacket)
	n, err := conn.Read(buf)
	if err != nil || n < 12 || buf[0] != p[0] || buf[1] != p[1] { // must be the answer to this question
		return reply(p, end, 2, nil)
	}
	return buf[:n]
}

func logDNS(d *dnsHelper) {
	if d == nil {
		return
	}
	other := "refused"
	if d.upstream != "" {
		other = "sent to " + d.upstream
	}
	log.Printf("DNS helper on %s: %d name(s) answered with %s; other names %s", d.addr(), len(d.names), d.ip, other)
}
