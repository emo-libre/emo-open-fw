package main

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

func dnsQuery(id uint16, name string, qtype uint16) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint16(b, id)
	b[2] = 0x01 // recursion desired
	binary.BigEndian.PutUint16(b[4:], 1)
	for _, l := range strings.Split(name, ".") {
		b = append(b, byte(len(l)))
		b = append(b, l...)
	}
	b = append(b, 0)
	b = append(b, byte(qtype>>8), byte(qtype), 0, 1)
	return b
}

func ask(t *testing.T, addr string, q []byte) []byte {
	t.Helper()
	c, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write(q); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1500)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatalf("no answer: %v", err)
	}
	return buf[:n]
}

func TestDNSHelperAnswersForwardsAndRefuses(t *testing.T) {
	// a fake upstream resolver that answers everything with 203.0.113.9
	up, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no UDP")
	}
	defer up.Close()
	go func() {
		buf := make([]byte, 600)
		for {
			n, from, err := up.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _, end, ok := question(buf[:n])
			if !ok {
				continue
			}
			rr := []byte{0xC0, 0x0C, 0, 1, 0, 1, 0, 0, 0, 30, 0, 4, 203, 0, 113, 9}
			up.WriteTo(reply(buf[:n], end, 0, rr), from)
		}
	}()

	cfg := defaultConfig()
	cfg.DNSListen, cfg.DNSNames, cfg.DNSAnswerIP, cfg.DNSUpstream = "127.0.0.1:0", []string{"Api.Example.Test.", "other.example.test"}, "192.168.1.50", up.LocalAddr().String()
	d, err := startDNS(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()

	// a listed name (any case, trailing dot) -> our address, TTL 60, ID and question echoed
	resp := ask(t, d.addr(), dnsQuery(0xBEEF, "api.example.test", 1))
	if binary.BigEndian.Uint16(resp) != 0xBEEF || resp[2]&0x80 == 0 || resp[3]&0x0F != 0 || binary.BigEndian.Uint16(resp[6:]) != 1 {
		t.Fatalf("header % x", resp[:12])
	}
	ip := resp[len(resp)-4:]
	if net.IP(ip).String() != "192.168.1.50" || resp[len(resp)-5] != 4 || resp[len(resp)-7] != 60 {
		t.Fatalf("answer % x", resp)
	}
	// AAAA for a listed name: the name exists, no such record
	resp = ask(t, d.addr(), dnsQuery(1, "api.example.test", 28))
	if binary.BigEndian.Uint16(resp[6:]) != 0 || resp[3]&0x0F != 0 {
		t.Fatalf("AAAA % x", resp)
	}
	// any other name goes to the upstream, unchanged
	resp = ask(t, d.addr(), dnsQuery(2, "www.unrelated.test", 1))
	if net.IP(resp[len(resp)-4:]).String() != "203.0.113.9" {
		t.Fatalf("forwarded % x", resp)
	}
	// junk is ignored (no answer), a response packet is not answered
	c, _ := net.Dial("udp", d.addr())
	c.Write([]byte{1, 2, 3})
	bad := dnsQuery(3, "api.example.test", 1)
	bad[2] |= 0x80
	c.Write(bad)
	c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if n, err := c.Read(make([]byte, 100)); err == nil {
		t.Fatalf("answered junk (%d bytes)", n)
	}
	c.Close()
}

func TestDNSHelperWithoutUpstreamRefusesOtherNames(t *testing.T) {
	cfg := defaultConfig()
	cfg.DNSListen, cfg.DNSNames, cfg.DNSAnswerIP = "127.0.0.1:0", []string{"api.example.test"}, "192.168.1.50"
	d, err := startDNS(cfg)
	if err != nil {
		t.Skip(err)
	}
	defer d.close()
	resp := ask(t, d.addr(), dnsQuery(7, "www.unrelated.test", 1))
	if resp[3]&0x0F != 5 {
		t.Fatalf("rcode %d, want REFUSED", resp[3]&0x0F)
	}
}

func TestDNSConfigValidation(t *testing.T) {
	ok := defaultConfig()
	ok.DNSListen, ok.DNSNames, ok.DNSAnswerIP = ":53", []string{"api.example.test"}, "192.168.1.50"
	if err := validDNSConfig(ok); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*Config){
		"no names":  func(c *Config) { c.DNSNames = nil },
		"bad ip":    func(c *Config) { c.DNSAnswerIP = "not-an-ip" },
		"ipv6 only": func(c *Config) { c.DNSAnswerIP = "::1" },
		"bad name":  func(c *Config) { c.DNSNames = []string{"has space.test"} },
		"bad up":    func(c *Config) { c.DNSUpstream = "192.168.1.1" },
	} {
		c := ok
		mut(&c)
		if validDNSConfig(c) == nil {
			t.Errorf("%s accepted", name)
		}
	}
	off := defaultConfig()
	if validDNSConfig(off) != nil {
		t.Error("an off helper needs nothing")
	}
}

func TestDNSHelperNeverLooksUpVendorNames(t *testing.T) {
	up, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no UDP")
	}
	defer up.Close()
	asked := make(chan string, 4)
	go func() {
		buf := make([]byte, 600)
		for {
			n, _, err := up.ReadFrom(buf)
			if err != nil {
				return
			}
			if name, _, _, ok := question(buf[:n]); ok {
				asked <- name
			}
		}
	}()
	cfg := defaultConfig()
	cfg.DNSListen, cfg.DNSNames, cfg.DNSAnswerIP, cfg.DNSUpstream = "127.0.0.1:0", []string{"api.example.test"}, "192.168.1.50", up.LocalAddr().String()
	d, err := startDNS(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	resp := ask(t, d.addr(), dnsQuery(9, "something.living.ai", 1))
	if resp[3]&0x0F != 3 {
		t.Fatalf("rcode %d, want NXDOMAIN", resp[3]&0x0F)
	}
	select {
	case n := <-asked:
		t.Fatalf("the vendor name %q was sent upstream", n)
	case <-time.After(300 * time.Millisecond):
	}
}
