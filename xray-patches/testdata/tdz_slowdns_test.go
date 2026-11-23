package internet

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"www.bamsoftware.com/git/dnstt.git/dns"
	"www.bamsoftware.com/git/dnstt.git/noise"
)

func TestTDZSlowDNSSharedSocketAndDNSMinimization(t *testing.T) {
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := udp.LocalAddr().String()
	udp.Close()
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	key, err := noise.GeneratePrivkey()
	if err != nil {
		t.Fatal(err)
	}
	cfg := tdzDNSConfig{Enabled: true, Listen: address, HostDomain: "ns.example.test", NSDomain: "t.example.test", PrivateKey: noise.EncodeKey(key), MTU: 1232, Routes: []tdzDNSRoute{{Domain: "i1.t.example.test", Port: first.Addr().(*net.TCPAddr).Port, Network: "tcp"}, {Domain: "i2.t.example.test", Port: second.Addr().(*net.TCPAddr).Port, Network: "tcp"}}}
	path := filepath.Join(t.TempDir(), "dns.json")
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TDZ_XRAY_SLOWDNS", path)
	l1, err := TDZSlowDNSListener(first, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Close()
	l2, err := TDZSlowDNSListener(second, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	if l1.(*tdzDNSListener).hub != l2.(*tdzDNSListener).hub {
		t.Fatal("inbounds must share one UDP listener")
	}
	conn, err := net.Dial("udp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	for _, domain := range []string{"t.example.test", "i1.t.example.test"} {
		name, _ := dns.ParseName(domain)
		query := dns.Message{ID: 23, Question: []dns.Question{{Name: name, Type: 2, Class: 1}}}
		wire, _ := query.WireFormat()
		if _, err := conn.Write(wire); err != nil {
			t.Fatal(err)
		}
		buffer := make([]byte, 4096)
		n, err := conn.Read(buffer)
		if err != nil {
			t.Fatal(err)
		}
		response, err := dns.MessageFromWireFormat(buffer[:n])
		if err != nil || response.Rcode() != dns.RcodeNoError {
			t.Fatalf("NS minimization probe failed: %v (%v)", response, err)
		}
	}
	l1.Close()
	if probe, err := net.ListenPacket("udp", address); err == nil {
		probe.Close()
		t.Fatal("closing one inbound must retain the other's DNS service")
	}
	l2.Close()
	probe, err := net.ListenPacket("udp", address)
	if err != nil {
		t.Fatal("last inbound must release its UDP socket", err)
	}
	probe.Close()
}

func TestTDZSlowDNSIdentitiesRequireAnActiveNativeSession(t *testing.T) {
	source := "fd53:1:2:3::4"
	if TDZIsSlowDNSSource(source) {
		t.Fatal("an unregistered private address is not a DNS session")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := &tdzDNSSession{ctx: ctx, cancel: cancel, activity: &tdzDNSActivity{resolver: "192.0.2.53"}}
	tdzDNSSources.Store(source, state)
	defer tdzDNSSources.Delete(source)
	if !TDZIsSlowDNSSource(source) {
		t.Fatal("active native session missing")
	}
	life, online, ok := TDZSlowDNSSourceInfo(source)
	if !ok || life != ctx || online != "slowdns|"+source {
		t.Fatal("online metadata lost the tunnel's separate identity")
	}
	cancel()
	if TDZIsSlowDNSSource(source) {
		t.Fatal("closed session remained active")
	}
}
