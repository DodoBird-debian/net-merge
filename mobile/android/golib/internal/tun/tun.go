// Package tun terminates the VpnService's TUN in a userland TCP/IP stack (gVisor via tun2socks)
// and re-dials every flow through the Balancer. Unlike upstream, Android creates the adapter and
// its routes (VpnService.Builder); this side only receives the file descriptor.
package tun

import (
	"context"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"net"
	"strconv"
	"sync"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip/stack"

	"github.com/xjasonlyu/tun2socks/v2/core"
	"github.com/xjasonlyu/tun2socks/v2/core/device"
	"github.com/xjasonlyu/tun2socks/v2/core/device/fdbased"
	"github.com/xjasonlyu/tun2socks/v2/core/option"
	tlog "github.com/xjasonlyu/tun2socks/v2/log"
	M "github.com/xjasonlyu/tun2socks/v2/metadata"
	"github.com/xjasonlyu/tun2socks/v2/proxy/reject"
	"github.com/xjasonlyu/tun2socks/v2/tunnel"

	"netmerge-android/internal/engine"
)

// udpSessionTimeout ends idle UDP sessions sooner than tun2socks' 60s (see upstream).
const udpSessionTimeout = 30 * time.Second

type tunProxy struct{ b *engine.Balancer }

func (p tunProxy) DialContext(ctx context.Context, m *M.Metadata) (net.Conn, error) {
	return p.b.DialPinned(ctx, m.DstIP.Unmap().String(), int(m.DstPort))
}

func (p tunProxy) DialUDP(m *M.Metadata) (net.PacketConn, error) {
	f := engine.Fam4
	if d := m.DstIP.Unmap(); d.Is6() {
		f = engine.Fam6
	}
	return p.b.ListenUDP(context.Background(), f)
}

// Tun owns the stack built on the VPN's file descriptor.
type Tun struct {
	mu  sync.Mutex
	dev device.Device
	stk *stack.Stack
}

// Start takes ownership of fd (it is closed by Stop).
func (t *Tun) Start(b *engine.Balancer, fd, mtu int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.teardown()
	if lvl, e := tlog.ParseLevel("error"); e == nil {
		tlog.SetLogger(tlog.Must(tlog.NewLeveled(lvl)))
	}
	tunnel.T().SetUDPTimeout(udpSessionTimeout)
	dev, err := fdbased.Open(strconv.Itoa(fd), uint32(mtu), 0)
	if err != nil {
		return err
	}
	stk, err := core.CreateStack(&core.Config{
		LinkEndpoint:     dev,
		TransportHandler: tunnel.T(),
		Options:          []option.Option{option.WithTCPModerateReceiveBuffer(true)},
	})
	if err != nil {
		dev.Close()
		return err
	}
	installForwarders(stk, b.V6Allowed)
	tunnel.T().SetProxy(tunProxy{b})
	t.dev, t.stk = dev, stk
	return nil
}

func (t *Tun) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.teardown()
}

func (t *Tun) teardown() {
	tunnel.T().SetProxy(&reject.Reject{})
	if t.dev != nil {
		t.dev.Close()
	}
	if t.stk != nil {
		done := make(chan struct{})
		go func() { t.stk.Close(); t.stk.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}
	t.dev, t.stk = nil, nil
}

// PacketDevice bridges NEPacketTunnelFlow to the same userland stack, without private TUN APIs.
type PacketDevice struct {
	ep  *channel.Endpoint
	stk *stack.Stack
}

func NewPacketDevice(b *engine.Balancer, mtu int) (*PacketDevice, error) {
	ep := channel.New(512, uint32(mtu), "")
	tunnel.T().SetUDPTimeout(udpSessionTimeout)
	s, err := core.CreateStack(&core.Config{LinkEndpoint: ep, TransportHandler: tunnel.T(), Options: []option.Option{option.WithTCPModerateReceiveBuffer(true)}})
	if err != nil {
		ep.Close()
		return nil, err
	}
	installForwarders(s, b.V6Allowed)
	tunnel.T().SetProxy(tunProxy{b})
	return &PacketDevice{ep, s}, nil
}
func (p *PacketDevice) Feed(data []byte) {
	if len(data) == 0 {
		return
	}
	proto := header.IPv4ProtocolNumber
	if data[0]>>4 == 6 {
		proto = header.IPv6ProtocolNumber
	} else if data[0]>>4 != 4 {
		return
	}
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(data)})
	p.ep.InjectInbound(proto, pkt)
	pkt.DecRef()
}
func (p *PacketDevice) Next() []byte {
	pkt := p.ep.ReadContext(context.Background())
	if pkt == nil {
		return nil
	}
	defer pkt.DecRef()
	v := pkt.ToView()
	defer v.Release()
	return append([]byte(nil), v.AsSlice()...)
}
func (p *PacketDevice) Close() { p.ep.Close(); p.stk.Close(); p.stk.Wait() }
