package state

import (
	"net-merge/internal/engine"
	"os"
	"path/filepath"
	"testing"
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
