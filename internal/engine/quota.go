package engine

import (
	"errors"
	"io"
	"math"
	"net-merge/internal/i18n"
	"strconv"
	"strings"
	"sync"

	"net-merge/internal/nics"
)

const BytesPerGB int64 = 1_000_000_000

var ErrQuota = errors.New(i18n.T("data allowance exhausted or merging paused"))

// QuotaRecord is durable usage since the last manual reset, not since startup.
type QuotaRecord struct {
	Name       string `json:"name"`
	LimitBytes int64  `json:"limit_bytes"`
	UsedBytes  int64  `json:"used_bytes"`
}

type quotaState struct {
	mu          sync.Mutex
	limit, used int64
	closers     map[io.Closer]struct{}
}

func nicKey(n nics.Nic) string {
	if n.ID != "" {
		return n.ID
	}
	return n.Name
}
func (sl *Slot) ifIndex() int { return sl.addr.Load().index }
func (s *Scheduler) limitFor(id, name string) float64 {
	if v, ok := s.limits[id]; ok {
		return v
	}
	return s.limits[name]
}

func NormalizeNumber(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= '٠' && r <= '٩':
			return '0' + r - '٠'
		case r >= '۰' && r <= '۹':
			return '0' + r - '۰'
		case r == '٫':
			return '.'
		}
		return r
	}, strings.TrimSpace(s))
}

func ParseQuota(s string) (int64, error) {
	s = NormalizeNumber(s)
	if strings.EqualFold(s, "unlimited") {
		return 0, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 1e6 || v*float64(BytesPerGB) < 1 {
		return 0, errors.New(i18n.T("enter a positive allowance in GB, or unlimited"))
	}
	return int64(math.Round(v * float64(BytesPerGB))), nil
}

func (sl *Slot) allowed() bool {
	if sl.paused.Load() || sl.storageBlocked.Load() {
		return false
	}
	sl.quota.mu.Lock()
	defer sl.quota.mu.Unlock()
	return sl.quota.limit == 0 || sl.quota.used < sl.quota.limit
}

func (sl *Slot) quotaSnapshot() QuotaRecord {
	sl.quota.mu.Lock()
	defer sl.quota.mu.Unlock()
	return QuotaRecord{LimitBytes: sl.quota.limit, UsedBytes: sl.quota.used}
}

func (sl *Slot) register(c io.Closer) bool {
	sl.quota.mu.Lock()
	if sl.paused.Load() || sl.storageBlocked.Load() || (sl.quota.limit > 0 && sl.quota.used >= sl.quota.limit) {
		sl.quota.mu.Unlock()
		c.Close()
		return false
	}
	if sl.quota.closers == nil {
		sl.quota.closers = map[io.Closer]struct{}{}
	}
	sl.quota.closers[c] = struct{}{}
	sl.quota.mu.Unlock()
	return true
}

func (sl *Slot) unregister(c io.Closer) {
	sl.quota.mu.Lock()
	delete(sl.quota.closers, c)
	sl.quota.mu.Unlock()
}

func (sl *Slot) closeFlows() {
	sl.quota.mu.Lock()
	list := make([]io.Closer, 0, len(sl.quota.closers))
	for c := range sl.quota.closers {
		list = append(list, c)
	}
	sl.quota.mu.Unlock()
	// Never invoke network or user callbacks while holding either accounting lock.
	for _, c := range list {
		c.Close()
	}
}

// account records bytes at the actual I/O boundary, once per payload. Small UDP
// packets bypass pacing, but never bypass data accounting. Kernel buffers can
// already hold bytes when exhaustion closes sockets; this is not ISP metering.
func (sl *Slot) account(f Family, down bool, n int) {
	if n <= 0 {
		return
	}
	sl.quota.mu.Lock()
	if sl.paused.Load() || sl.storageBlocked.Load() {
		sl.quota.mu.Unlock()
		return
	}
	before := sl.quota.used
	sl.quota.used += int64(n)
	if down {
		sl.bytesIn[f].Add(int64(n))
	} else {
		sl.bytesOut[f].Add(int64(n))
	}
	exhausted := sl.quota.limit > 0 && before < sl.quota.limit && sl.quota.used >= sl.quota.limit
	sl.quota.mu.Unlock()
	if exhausted {
		select {
		case sl.notify <- struct{}{}:
		default:
		}
		sl.closeFlows()
	}
}

func (s *Scheduler) AdapterID(name string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sl := range s.slots {
		if sl.key == name || sl.Name == name {
			return sl.key, true
		}
	}
	if _, ok := s.quotaRecords[name]; ok {
		return name, true
	}
	for id, r := range s.quotaRecords {
		if r.Name == name {
			return id, true
		}
	}
	return name, false
}

func (s *Scheduler) LoadQuotas(records map[string]QuotaRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if records == nil {
		records = map[string]QuotaRecord{}
	}
	s.quotaRecords = records
	for _, sl := range s.slots {
		r := records[sl.key]
		sl.quota.mu.Lock()
		sl.quota.limit, sl.quota.used = r.LimitBytes, r.UsedBytes
		sl.quota.mu.Unlock()
	}
}

func (s *Scheduler) Quotas() map[string]QuotaRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]QuotaRecord, len(s.quotaRecords)+len(s.slots))
	for k, v := range s.quotaRecords {
		out[k] = v
	}
	for _, sl := range s.slots {
		r := sl.quotaSnapshot()
		r.Name = sl.Name
		out[sl.key] = r
	}
	return out
}

// SetQuota preserves usage; only reset=true clears it. Session counters never reset here.
func (s *Scheduler) SetQuota(id string, limit int64, reset bool) error {
	if id == "" || limit < 0 || limit > 1e6*BytesPerGB {
		return errors.New(i18n.T("invalid data allowance"))
	}
	s.mu.Lock()
	var closeSlot *Slot
	r := s.quotaRecords[id]
	for _, sl := range s.slots {
		if sl.key != id && sl.Name != id {
			continue
		}
		id = sl.key
		sl.quota.mu.Lock()
		sl.quota.limit = limit
		if reset {
			sl.quota.used = 0
		}
		r = QuotaRecord{Name: sl.Name, LimitBytes: limit, UsedBytes: sl.quota.used}
		sl.quota.mu.Unlock()
		closeSlot = sl
		break
	}
	r.LimitBytes = limit
	if reset {
		r.UsedBytes = 0
	}
	s.quotaRecords[id] = r
	s.mu.Unlock()
	if closeSlot != nil && !closeSlot.allowed() {
		closeSlot.closeFlows()
	}
	select {
	case s.QuotaChanged <- struct{}{}:
	default:
	}
	return nil
}

func (s *Scheduler) HasQuotas() bool {
	for _, r := range s.Quotas() {
		if r.LimitBytes > 0 {
			return true
		}
	}
	return false
}

func (s *Scheduler) AllExhausted() bool {
	stats := s.Snapshot()
	present := false
	for _, st := range stats {
		if st.Present {
			present = true
			if !st.QuotaExhausted {
				return false
			}
		}
	}
	return present
}

// Pause closes all relayed sockets, including retained UDP sessions. Accounted
// session bytes stay unchanged until Resume; direct OS traffic is outside scope.
func (s *Scheduler) Pause(paused bool) {
	s.mu.Lock()
	s.paused = paused
	slots := append([]*Slot(nil), s.slots...)
	for _, sl := range slots {
		sl.paused.Store(paused)
		if paused {
			sl.rateIn, sl.rateOut = [2]float64{}, [2]float64{}
		}
	}
	s.mu.Unlock()
	if paused {
		for _, sl := range slots {
			sl.closeFlows()
		}
	}
}

// SetStorageError prevents a failed disk from silently losing allowance usage.
func (s *Scheduler) SetStorageError(err error) {
	s.mu.Lock()
	s.storageError = ""
	if err != nil {
		s.storageError = err.Error()
	}
	slots := append([]*Slot(nil), s.slots...)
	for _, sl := range slots {
		sl.storageBlocked.Store(err != nil)
	}
	s.mu.Unlock()
	if err != nil {
		for _, sl := range slots {
			sl.closeFlows()
		}
	}
}
func (s *Scheduler) StorageError() string { s.mu.Lock(); defer s.mu.Unlock(); return s.storageError }
