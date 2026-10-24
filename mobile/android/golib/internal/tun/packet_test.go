package tun

import (
	"bytes"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/checksum"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"net"
	"netmerge-android/internal/engine"
	"netmerge-android/internal/nics"
	"os"
	"testing"
	"time"
)

// Exercise the actual NEPacketTunnelFlow bridge, small UDP relay, both IP families,
// reply injection, allowance accounting and pause closure, with a real loopback server.
func TestPacketBridgeUDP(t *testing.T) {
	for _, family := range []string{"udp4", "udp6"} {
		t.Run(family, func(t *testing.T) {
			var ip net.IP
			interfaces, _ := net.Interfaces()
			for _, it := range interfaces {
				addrs, _ := it.Addrs()
				for _, a := range addrs {
					candidate, _, _ := net.ParseCIDR(a.String())
					if candidate.To4() != nil && !candidate.IsLoopback() {
						ip = candidate
					}
				}
			}
			if ip == nil {
				t.Skip("no non-loopback IPv4 interface")
			}
			source := net.IPv4(198, 18, 0, 2)
			hlen := header.IPv4MinimumSize
			n := nics.Nic{ID: "test", Name: "test", IP: ip, Weight: 1}
			if family == "udp6" {
				ip = net.ParseIP(os.Getenv("NETMERGE_TEST_IP6"))
				if ip == nil {
					t.Skip("set NETMERGE_TEST_IP6 to a non-loopback IPv6 address; CI adds a local test address")
				}
				source = net.ParseIP("fd00::2")
				hlen = header.IPv6MinimumSize
				n.IP = nil
				n.IP6 = ip
			}
			server, err := net.ListenUDP(family, &net.UDPAddr{IP: ip})
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			go func() {
				buf := make([]byte, 1024)
				n, addr, e := server.ReadFromUDP(buf)
				if e == nil {
					server.WriteToUDP(buf[:n], addr)
				}
			}()
			scheduler := engine.NewScheduler([]nics.Nic{n})
			balancer := engine.NewBalancer(scheduler)
			p, err := NewPacketDevice(balancer, 1500)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			defer scheduler.Pause(true)
			payload := []byte("small-packet-accounted")
			data := make([]byte, hlen+header.UDPMinimumSize+len(payload))
			copy(data[hlen+8:], payload)
			src := tcpip.AddrFromSlice(source.To4())
			dst := tcpip.AddrFromSlice(ip.To4())
			if family == "udp6" {
				src = tcpip.AddrFromSlice(source.To16())
				dst = tcpip.AddrFromSlice(ip.To16())
			}
			u := header.UDP(data[hlen:])
			u.Encode(&header.UDPFields{SrcPort: 12345, DstPort: uint16(server.LocalAddr().(*net.UDPAddr).Port), Length: uint16(len(data) - hlen)})
			partial := header.PseudoHeaderChecksum(header.UDPProtocolNumber, src, dst, uint16(len(data)-hlen))
			partial = checksum.Checksum(payload, partial)
			u.SetChecksum(^u.CalculateChecksum(partial))
			if family == "udp4" {
				h := header.IPv4(data)
				h.Encode(&header.IPv4Fields{TotalLength: uint16(len(data)), TTL: 64, Protocol: uint8(header.UDPProtocolNumber), SrcAddr: src, DstAddr: dst})
				h.SetChecksum(^h.CalculateChecksum())
			} else {
				header.IPv6(data).Encode(&header.IPv6Fields{PayloadLength: uint16(len(data) - hlen), TransportProtocol: header.UDPProtocolNumber, HopLimit: 64, SrcAddr: src, DstAddr: dst})
			}
			got := make(chan []byte, 1)
			go func() { got <- p.Next() }()
			p.Feed(data)
			select {
			case packet := <-got:
				if len(packet) < hlen+8 || !bytes.Equal(packet[hlen+8:], payload) {
					t.Fatalf("unexpected reply %x", packet)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("no packet reply")
			}
			snap := scheduler.Snapshot()
			if len(snap) != 1 || snap[0].SessionBytes != int64(2*len(payload)) {
				t.Fatalf("accounting: %+v", snap)
			}
			scheduler.Pause(true)
			before := scheduler.Snapshot()[0].SessionBytes
			p.Feed(data)
			time.Sleep(20 * time.Millisecond)
			if scheduler.Snapshot()[0].SessionBytes != before {
				t.Fatal("paused packet changed accounting")
			}
		})
	}
}
