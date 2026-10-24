package state

import (
	"encoding/json"
	"net"
	"netmerge-android/internal/engine"
	"netmerge-android/internal/nics"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQuotaFileRoundTripAndCorruption(t *testing.T) {
	dir := t.TempDir()
	r := map[string]engine.QuotaRecord{"stable-id": {Name: "اتصال", LimitBytes: 1e9, UsedBytes: 1234}}
	if err := SaveQuotas(dir, r); err != nil {
		t.Fatal(err)
	}
	got, err := LoadQuotas(dir)
	if err != nil || got["stable-id"] != r["stable-id"] {
		t.Fatal(got, err)
	}
	for _, bad := range []string{`{`, `{"version":2,"adapters":{}}`, `{"version":1}`, `{"version":1,"adapters":{"a":{"used_bytes":-1}}}`} {
		os.WriteFile(QuotasPath(dir), []byte(bad), 0600)
		if _, err := LoadQuotas(dir); err == nil {
			t.Fatal("silently accepted bad allowances", bad)
		}
	}
}
func TestQuotaRequestsValidateUntrustedInput(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ctl"), 0755)
	for _, bad := range []string{`{"adapter":"A","action":"execute"}`, `{"adapter":"","action":"set"}`, `{"adapter":"A","action":"set","bytes":-1}`, `{"adapter":"A","action":"set","bytes":9223372036854775807}`} {
		os.WriteFile(filepath.Join(dir, "ctl", "quota"), []byte(bad), 0600)
		if _, ok := TakeQuota(dir); ok {
			t.Fatal("accepted", bad)
		}
	}
	req := QuotaRequest{Adapter: "A", Action: "set", Bytes: 2e9}
	if err := SendQuota(dir, req); err != nil {
		t.Fatal(err)
	}
	got, ok := TakeQuota(dir)
	if !ok || got != req {
		t.Fatal(got, ok)
	}
}
func TestQuotaCannotReadSymlink(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ctl"), 0755)
	if err := os.Symlink(filepath.Join(dir, "target"), filepath.Join(dir, "ctl", "quota")); err != nil {
		t.Skip(err)
	}
	if _, ok := TakeQuota(dir); ok {
		t.Fatal("read symlink")
	}
}

func TestQuotaStorageFailureBlocksUntilRecovery(t *testing.T) {
	dir := t.TempDir()
	s := engine.NewScheduler([]nics.Nic{{Name: "A", IP: net.ParseIP("127.0.0.1"), Weight: 1}})
	if err := s.SetQuota("A", 1e9, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(QuotasPath(dir), 0755); err != nil {
		t.Fatal(err)
	}
	err := CheckpointQuotas(dir, s)
	if err == nil {
		t.Fatal("expected a failed atomic replace")
	}
	s.SetStorageError(err)
	if s.Acquire(nil) != nil || s.StorageError() == "" {
		t.Fatal("storage failure silently allowed traffic")
	}
	os.Remove(QuotasPath(dir))
	err = CheckpointQuotas(dir, s)
	s.SetStorageError(err)
	sl := s.Acquire(nil)
	if sl == nil || s.StorageError() != "" {
		t.Fatal("storage recovery did not unblock traffic", err)
	}
	s.Release(sl)
}
func TestQuotaResetAndRequestReceipt(t *testing.T) {
	dir := t.TempDir()
	s := engine.NewScheduler([]nics.Nic{{Name: "A", IP: net.ParseIP("127.0.0.1"), Weight: 1}})
	s.LoadQuotas(map[string]engine.QuotaRecord{"A": {Name: "A", LimitBytes: 100, UsedBytes: 70}})
	id, err := NewQuotaRequestID()
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyQuota(dir, s, QuotaRequest{Adapter: "A", Action: "reset", RequestID: id}); err != nil {
		t.Fatal(err)
	}
	records, err := LoadQuotas(dir)
	if err != nil || records["A"].UsedBytes != 0 || records["A"].LimitBytes != 100 {
		t.Fatal(records, err)
	}
	b, err := os.ReadFile(QuotaReceiptPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var result QuotaReceipt
	if err = json.Unmarshal(b, &result); err != nil || result.RequestID != id || result.Error != "" {
		t.Fatal(result, err)
	}
}
func TestPausedStatusRetainsSessionAndMachineState(t *testing.T) {
	dir := t.TempDir()
	Write(dir, Status{PID: 42, Mode: "paused", Paused: true, SessionBytes: 1234, Updated: time.Now()})
	if _, active := Read(dir); active {
		t.Fatal("paused process was reported active")
	}
	st, fresh := ReadAny(dir)
	if !fresh || !st.Paused || st.SessionBytes != 1234 {
		t.Fatal("paused session lost", st)
	}
}
