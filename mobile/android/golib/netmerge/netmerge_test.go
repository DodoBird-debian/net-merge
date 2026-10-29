package netmerge

import (
	"context"
	"encoding/json"
	"net"
	"netmerge-android/internal/engine"
	"testing"
	"time"
)

func resetScheduler() {
	sched = engine.NewScheduler(nil)
	bal = engine.NewBalancer(sched)
	indexes = map[string]int{}
	last = nil
	lastJS = ""
	auto = true
	sched.SetAuto(true)
}
func TestSetNetworksAndStats(t *testing.T) {
	resetScheduler()
	js := `[{"name":"Wi-Fi","handle":"432902426637","v4":true,"v6":true},{"name":"Mobile data","handle":"437197393933","v4":true}]`
	if err := SetNetworks(js); err != nil {
		t.Fatal(err)
	}
	var st []stat
	if err := json.Unmarshal([]byte(Stats()), &st); err != nil {
		t.Fatal(err)
	}
	if len(st) != 2 || st[0].Name != "Wi-Fi" || st[1].State6 != "-" {
		t.Fatalf("unexpected stats: %+v", st)
	}
	// The same name keeps its index when its handle changes (Wi-Fi reconnect).
	if err := SetNetworks(`[{"name":"Wi-Fi","handle":"1","v4":true}]`); err != nil {
		t.Fatal(err)
	}
	if indexes["Wi-Fi"] != 1 || indexes["Mobile data"] != 2 {
		t.Fatalf("indexes changed: %v", indexes)
	}
	if err := SetNetworks(`[{"name":"x","handle":"nope"}]`); err == nil {
		t.Fatal("want error for a bad handle")
	}
}

func TestSetAutoWeightsRestoresEvenSplit(t *testing.T) {
	resetScheduler()
	if err := SetNetworks(`[{"name":"A","handle":"1","v4":true},{"name":"B","handle":"2","v4":true}]`); err != nil {
		t.Fatal(err)
	}
	SetAutoWeights(false)
	defer SetAutoWeights(true)
	var st []stat
	if err := json.Unmarshal([]byte(Stats()), &st); err != nil {
		t.Fatal(err)
	}
	for _, s := range st {
		if s.Present && s.Weight != 1 {
			t.Fatalf("%s weight %v, want 1", s.Name, s.Weight)
		}
	}
}

func TestSystemEstimateSeedsWeights(t *testing.T) {
	resetScheduler()
	SetAutoWeights(true)
	if err := SetNetworks(`[{"name":"Wi-Fi","handle":"1","v4":true,"downKbps":400000},{"name":"Mobile data","handle":"2","v4":true,"downKbps":50000}]`); err != nil {
		t.Fatal(err)
	}
	// Before measured traffic, Android seeds the first selection from the system estimate.
	w := map[string]float64{}
	var st []stat
	json.Unmarshal([]byte(Stats()), &st)
	for _, s := range st {
		w[s.Name] = s.Weight
	}
	if w["Wi-Fi"] != 1 || w["Mobile data"] != 0.125 {
		t.Fatalf("weights %v, want Wi-Fi 1 and mobile 0.125", w)
	}
	SetAutoWeights(false) // even split ignores the estimates
	defer SetAutoWeights(true)
	json.Unmarshal([]byte(Stats()), &st)
	for _, s := range st {
		if s.Present && s.Weight != 1 {
			t.Fatalf("even split: %s weight %v", s.Name, s.Weight)
		}
	}
}

func TestNetworkHandleReplacementClosesFlowsPreservesAllowance(t *testing.T) {
	resetScheduler()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		c, e := listener.Accept()
		if e == nil {
			defer c.Close()
			buf := make([]byte, 1)
			for {
				n, e := c.Read(buf)
				if n > 0 {
					c.Write(buf[:n])
				}
				if e != nil {
					return
				}
			}
		}
	}()
	js := `[{"id":"wifi-interface","name":"Wi-Fi","handle":"101","v4":true}]`
	if err := SetNetworks(js); err != nil {
		t.Fatal(err)
	}
	if err := sched.SetQuota("wifi-interface", 100000, false); err != nil {
		t.Fatal(err)
	}
	c, err := bal.DialPinned(context.Background(), "127.0.0.1", listener.Addr().(*net.TCPAddr).Port)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(time.Second))
	if _, err = c.Write([]byte{42}); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 1)
	if _, err = c.Read(b); err != nil {
		t.Fatal(err)
	}
	before := sched.Snapshot()[0]
	if err := SetNetworks(`[{"id":"wifi-interface","name":"Renamed Wi-Fi","handle":"102","v4":true}]`); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Read(b); err == nil {
		t.Fatal("old network handle flow was not closed")
	}
	after := sched.Snapshot()[0]
	if after.ID != before.ID || after.SessionBytes != before.SessionBytes || after.QuotaUsedBytes != before.QuotaUsedBytes || after.QuotaBytes != before.QuotaBytes {
		t.Fatalf("replacement reset usage: before=%+v after=%+v", before, after)
	}
}
