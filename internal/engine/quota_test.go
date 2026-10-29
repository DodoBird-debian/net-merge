package engine

import (
	"net"
	"net-merge/internal/nics"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testCloser struct{ closed atomic.Int32 }

func (c *testCloser) Close() error { c.closed.Add(1); return nil }
func quotaScheduler() *Scheduler {
	return NewScheduler([]nics.Nic{{ID: "stable-a", Name: "A", Index: 1, IP: net.ParseIP("127.0.0.1"), IP6: net.ParseIP("::1"), Weight: 1}, {ID: "stable-b", Name: "B", Index: 2, IP: net.ParseIP("127.0.0.2"), Weight: 1}})
}
func TestQuotaCountsBothFamiliesAndDirections(t *testing.T) {
	s := quotaScheduler()
	sl := s.slots[0]
	for _, f := range []Family{Fam4, Fam6} {
		sl.account(f, true, 40)
		sl.account(f, false, 10)
	}
	st := s.Snapshot()[0]
	if st.SessionBytes != 100 || st.QuotaUsedBytes != 100 || st.BytesIn != 80 || st.BytesOut != 20 {
		t.Fatalf("bad accounting: %+v", st)
	}
}
func TestQuotaExhaustionClosesAndNeverFallsBack(t *testing.T) {
	s := quotaScheduler()
	a, b := s.slots[0], s.slots[1]
	_ = s.SetQuota(a.key, 100, false)
	_ = s.SetQuota(b.key, 100, false)
	tcp, udp := &testCloser{}, &testCloser{}
	a.register(tcp)
	a.register(udp)
	a.account(Fam4, false, 100)
	if tcp.closed.Load() == 0 || udp.closed.Load() == 0 {
		t.Fatal("open flows not closed")
	}
	for i := 0; i < 10; i++ {
		got := s.Acquire(nil)
		if got != b {
			t.Fatal("capped adapter scheduled")
		}
		s.Release(got)
	}
	b.account(Fam4, true, 100)
	s.SetState(a, Fam4, false)
	s.SetState(b, Fam4, false)
	if s.Acquire(nil) != nil || s.AcquireFor(Fam6, nil) != nil || !s.AllExhausted() || s.V6Usable() {
		t.Fatal("exhausted link reentered health fallback")
	}
	late := &testCloser{}
	if a.register(late) || late.closed.Load() == 0 {
		t.Fatal("late connection was admitted")
	}
	select {
	case <-s.QuotaChanged:
	default:
		t.Fatal("exhaustion not signaled for immediate persistence")
	}
}
func TestQuotaResetDoesNotResetSession(t *testing.T) {
	s := quotaScheduler()
	sl := s.slots[0]
	_ = s.SetQuota(sl.key, 10, false)
	sl.account(Fam4, true, 10)
	_ = s.SetQuota(sl.key, 10, true)
	st := s.Snapshot()[0]
	if st.SessionBytes != 10 || st.QuotaUsedBytes != 0 || st.QuotaExhausted {
		t.Fatalf("bad reset: %+v", st)
	}
	_ = s.SetQuota(sl.key, 5, false)
	sl.account(Fam4, false, 5)
	_ = s.SetQuota(sl.key, 0, false)
	st = s.Snapshot()[0]
	if st.QuotaUsedBytes != 5 || !sl.allowed() || st.QuotaRemainingBytes != -1 {
		t.Fatal("unlimited must preserve usage")
	}
}
func TestQuotaPersistenceAndAdapterIdentity(t *testing.T) {
	s := quotaScheduler()
	a := s.slots[0]
	_ = s.SetQuota(a.key, 500, false)
	a.account(Fam6, true, 200)
	s.Update([]nics.Nic{{ID: a.key, Name: "Renamed adapter", Index: 9, IP: net.ParseIP("127.0.0.3"), Weight: 1}})
	st := s.Snapshot()[0]
	if st.Name != "Renamed adapter" || st.SessionBytes != 200 || st.QuotaUsedBytes != 200 || a.ifIndex() != 9 {
		t.Fatal(st)
	}
	s.Update(nil)
	s.Update([]nics.Nic{{ID: a.key, Name: "Back", Index: 10, IP: net.ParseIP("127.0.0.4"), Weight: 1}})
	if s.Snapshot()[0].SessionBytes != 200 {
		t.Fatal("reconnect reset session")
	}
	restart := quotaScheduler()
	restart.LoadQuotas(s.Quotas())
	st = restart.Snapshot()[0]
	if st.SessionBytes != 0 || st.QuotaUsedBytes != 200 || st.QuotaBytes != 500 {
		t.Fatal("restart mixed session and durable usage", st)
	}
}
func TestQuotaPauseFreezesCountersAndClosesUDP(t *testing.T) {
	s := quotaScheduler()
	a := s.slots[0]
	c := &testCloser{}
	a.register(c)
	a.account(Fam4, true, 20)
	s.Pause(true)
	a.account(Fam4, true, 100)
	if c.closed.Load() == 0 || s.Snapshot()[0].SessionBytes != 20 || s.Acquire(nil) != nil {
		t.Fatal("paused state still active")
	}
	s.Pause(false)
	a.account(Fam4, false, 5)
	if s.Snapshot()[0].SessionBytes != 25 {
		t.Fatal("resume reset or lost session")
	}
}
func TestQuotaConcurrentAccounting(t *testing.T) {
	s := quotaScheduler()
	a := s.slots[0]
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 2000; j++ {
				a.account(Family(i%2), i%3 == 0, 17)
			}
		}(i)
	}
	wg.Wait()
	st := s.Snapshot()[0]
	if st.SessionBytes != 24*2000*17 || st.QuotaUsedBytes != st.SessionBytes {
		t.Fatal(st)
	}
}
func TestQuotaSmallUDPRealIO(t *testing.T) {
	raw, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	peer, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	sl := &Slot{}
	sl.quota.limit = 20
	sl.register(raw)
	pc := &slotPacketConn{PacketConn: raw, sl: sl, fam: Fam4}
	if n, err := pc.WriteTo(make([]byte, 10), peer.LocalAddr()); n != 10 || err != nil {
		t.Fatal(n, err)
	}
	peer.WriteTo(make([]byte, 10), raw.LocalAddr())
	raw.SetReadDeadline(time.Now().Add(time.Second))
	if n, _, err := pc.ReadFrom(make([]byte, 64)); n != 10 || err != nil {
		t.Fatal(n, err)
	}
	if sl.quotaSnapshot().UsedBytes != 20 {
		t.Fatal("small UDP packets skipped data accounting")
	}
	if _, err := pc.WriteTo([]byte{1}, peer.LocalAddr()); err != ErrQuota {
		t.Fatal("exhausted UDP not blocked", err)
	}
}
func TestQuotaNumberValidation(t *testing.T) {
	for _, input := range []string{"NaN", "Inf", "-1", "0", "", "1000001", "0.00000000001"} {
		if _, err := ParseQuota(input); err == nil {
			t.Error("accepted", input)
		}
	}
	for _, input := range []string{"1.5", "١٫٥", "۱.۵"} {
		got, err := ParseQuota(input)
		if err != nil || got != 1_500_000_000 {
			t.Error(input, got, err)
		}
	}
	if n, err := ParseQuota("unlimited"); n != 0 || err != nil {
		t.Fatal(n, err)
	}
}
