package state

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"netmerge-android/internal/i18n"
	"os"
	"path/filepath"
	"sync"
	"time"

	"netmerge-android/internal/engine"
)

// Kept separate from legacy limits.json so existing speed-limit installations
// remain compatible. Missing is first use; malformed/unsupported is an error.
type quotasFile struct {
	Version  int                           `json:"version"`
	Adapters map[string]engine.QuotaRecord `json:"adapters"`
}

func PreferencesDir(dir string) string { return limitsDir(dir) }

func QuotasPath(dir string) string { return filepath.Join(limitsDir(dir), "quotas.json") }

func LoadQuotas(dir string) (map[string]engine.QuotaRecord, error) {
	b, err := os.ReadFile(QuotasPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]engine.QuotaRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	var f quotasFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf(i18n.T("%s: %w"), QuotasPath(dir), err)
	}
	if f.Version != 1 || f.Adapters == nil {
		return nil, fmt.Errorf(i18n.T("%s: unsupported or invalid allowance file"), QuotasPath(dir))
	}
	for id, r := range f.Adapters {
		if id == "" || r.LimitBytes < 0 || r.LimitBytes > 1e6*engine.BytesPerGB || r.UsedBytes < 0 {
			return nil, fmt.Errorf(i18n.T("%s: invalid allowance record"), QuotasPath(dir))
		}
	}
	return f.Adapters, nil
}

// One lock serializes snapshot+replace, including shutdown and control writes;
// an older periodic snapshot must never overwrite an immediately saved reset.
var quotaSaveMu sync.Mutex

func SaveQuotas(dir string, records map[string]engine.QuotaRecord) error {
	quotaSaveMu.Lock()
	defer quotaSaveMu.Unlock()
	return saveQuotas(dir, records)
}

func CheckpointQuotas(dir string, s *engine.Scheduler) error {
	quotaSaveMu.Lock()
	defer quotaSaveMu.Unlock()
	return saveQuotas(dir, s.Quotas())
}

func saveQuotas(dir string, records map[string]engine.QuotaRecord) error {
	b, err := json.MarshalIndent(quotasFile{1, records}, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(QuotasPath(dir), append(b, '\n'), 0644)
}

func AtomicWrite(path string, b []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".net-merge-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

// Checkpoint every second and immediately when a link exhausts its allowance.
// A storage failure blocks relaying until a successful retry, and is visible in status.
func RunQuotaStore(dir string, s *engine.Scheduler, stop <-chan struct{}) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			_ = CheckpointQuotas(dir, s)
			return
		case <-tick.C:
		case <-s.QuotaChanged:
		}
		s.SetStorageError(CheckpointQuotas(dir, s))
	}
}

type QuotaRequest struct {
	RequestID string `json:"request_id,omitempty"`
	Adapter   string `json:"adapter"`
	Action    string `json:"action"` // set or reset
	Bytes     int64  `json:"bytes"`  // zero = unlimited
}

func NewQuotaRequestID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}

func validQuotaRequest(r QuotaRequest) bool {
	if r.RequestID != "" {
		if len(r.RequestID) != 32 {
			return false
		}
		if _, err := hex.DecodeString(r.RequestID); err != nil {
			return false
		}
	}
	return r.Adapter != "" && len(r.Adapter) <= 512 && (r.Action == "set" || r.Action == "reset") && r.Bytes >= 0 && r.Bytes <= 1e6*engine.BytesPerGB
}

func SendQuota(dir string, r QuotaRequest) error {
	if !validQuotaRequest(r) {
		return errors.New(i18n.T("invalid data allowance request"))
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return AtomicWrite(filepath.Join(dir, "ctl", "quota"), b, 0600)
}

func TakeQuota(dir string) (QuotaRequest, bool) {
	path := filepath.Join(dir, "ctl", "quota")
	taken := path + ".taken"
	if os.Rename(path, taken) != nil {
		return QuotaRequest{}, false
	}
	defer os.Remove(taken)
	b, ok := readLimitRequest(taken)
	var r QuotaRequest
	if !ok || json.Unmarshal(b, &r) != nil || !validQuotaRequest(r) {
		return QuotaRequest{}, false
	}
	return r, true
}

type QuotaReceipt struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
}

func QuotaReceiptPath(dir string) string { return filepath.Join(dir, "quota-result") }

func ApplyQuota(dir string, s *engine.Scheduler, r QuotaRequest) (result error) {
	defer func() {
		if r.RequestID == "" {
			return
		}
		receipt := QuotaReceipt{RequestID: r.RequestID}
		if result != nil {
			receipt.Error = result.Error()
		}
		b, _ := json.Marshal(receipt)
		_ = AtomicWrite(QuotaReceiptPath(dir), b, 0644)
	}()
	if !validQuotaRequest(r) {
		return errors.New(i18n.T("invalid data allowance request"))
	}
	id, ok := s.AdapterID(r.Adapter)
	if !ok {
		return errors.New(i18n.T("unknown network adapter"))
	}
	limit := r.Bytes
	if r.Action == "reset" {
		limit = s.Quotas()[id].LimitBytes
	}
	if err := s.SetQuota(id, limit, r.Action == "reset"); err != nil {
		return err
	}
	err := CheckpointQuotas(dir, s)
	s.SetStorageError(err)
	return err
}
