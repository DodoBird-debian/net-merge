package netmerge

import (
	"errors"
	"net"
	"netmerge-android/internal/nics"
	"netmerge-android/internal/state"
	"netmerge-android/internal/tun"
	"strings"
	"sync"
	"time"
)

// PacketCore is owned by an iOS packet tunnel provider. Outbound sockets bind to physical
// interfaces; utun interfaces are deliberately excluded to prevent a tunnel routing loop.
type PacketCore struct {
	dev  *tun.PacketDevice
	done chan struct{}
	once sync.Once
}

func NewPacketCore(dir, language string) (*PacketCore, error) {
	if err := Configure(dir, language); err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	if stop != nil {
		return nil, errors.New("packet engine is already running")
	}
	p := &PacketCore{done: make(chan struct{})}
	if err := p.RefreshNetworks(); err != nil {
		return nil, err
	}
	d, err := tun.NewPacketDevice(bal, 1500)
	if err != nil {
		return nil, err
	}
	p.dev = d
	sched.Pause(false)
	stop = p.done
	go sched.RunProbes(p.done)
	go tick(p.done)
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-p.done:
				return
			case <-t.C:
				_ = p.RefreshNetworks()
			}
		}
	}()
	return p, nil
}
func (p *PacketCore) RefreshNetworks() error {
	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	var list []nics.Nic
	cellularAdded := false
	for _, it := range interfaces {
		if it.Flags&net.FlagUp == 0 || !(it.Name == "en0" || strings.HasPrefix(it.Name, "pdp_ip")) {
			continue
		}
		name := it.Name
		if strings.HasPrefix(it.Name, "pdp_ip") {
			if cellularAdded {
				continue
			}
			cellularAdded = true
			name = "Mobile data"
		}
		n := nics.Nic{ID: it.Name, Name: name, Index: it.Index, Weight: 1}
		addrs, e := it.Addrs()
		if e != nil {
			continue
		}
		for _, a := range addrs {
			ip, _, e := net.ParseCIDR(a.String())
			if e != nil || !ip.IsGlobalUnicast() {
				continue
			}
			if ip.To4() != nil {
				n.IP = ip
			} else {
				n.IP6 = ip
			}
		}
		if n.IP != nil || n.IP6 != nil {
			list = append(list, n)
		}
	}
	sched.Update(list)
	sched.CloseGone()
	return nil
}
func (p *PacketCore) FeedPacket(data []byte) { p.dev.Feed(data) }

// NextPacket blocks until a packet is available or Close is called; call on a background queue.
func (p *PacketCore) NextPacket() []byte { return p.dev.Next() }
func (p *PacketCore) Close() {
	p.once.Do(func() {
		mu.Lock()
		defer mu.Unlock()
		close(p.done)
		stop = nil
		sched.Pause(true)
		p.dev.Close()
		sched.SetStorageError(state.CheckpointQuotas(stateDir, sched))
	})
}

// Pause retains packet capture while closing all relayed flows and freezing usage.
func Pause(paused bool) { sched.Pause(paused) }
