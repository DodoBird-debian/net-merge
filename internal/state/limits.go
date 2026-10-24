package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"net-merge/internal/engine"
	"net-merge/internal/i18n"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

const limitsVersion = 1

type limitsFile struct {
	Version int                `json:"version"`
	Limits  map[string]float64 `json:"limits"`
}

// LimitsPath is where the per-adapter speed limits live: next to the status file, except on macOS,
// where the status file is in /var/run and is gone after a reboot, so the caps go to Application
// Support instead. A custom -state-dir keeps them in that directory, which is what someone passing
// that flag expects.
func LimitsPath(dir string) string {
	return filepath.Join(limitsDir(dir), "limits.json")
}

func limitsDir(stateDir string) string { return limitsDirFor(runtime.GOOS, stateDir) }

// limitsDirFor is limitsDir with the OS passed in, so the macOS rule can be tested anywhere.
func limitsDirFor(goos, stateDir string) string {
	if goos == "darwin" && stateDir == macStateDir {
		return macLimitsDir
	}
	return stateDir
}

// LoadLimits reads the saved caps in Mbit/s. A missing file means no caps, not an error.
func LoadLimits(dir string) (map[string]float64, error) {
	b, err := os.ReadFile(LimitsPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]float64{}, nil
	}
	if err != nil {
		return nil, err
	}
	var f limitsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf(i18n.T("%s: %w"), LimitsPath(dir), err)
	}
	limits := make(map[string]float64, len(f.Limits))
	for name, mbitPerSec := range f.Limits {
		if name != "" && mbitPerSec > 0 && mbitPerSec <= 1e6 {
			limits[name] = mbitPerSec
		}
	}
	return limits, nil
}

// SaveLimits writes the caps atomically, so a crash mid-write cannot leave a broken file.
var limitsSaveMu sync.Mutex

func SaveLimits(dir string, limits map[string]float64) error {
	limitsSaveMu.Lock()
	defer limitsSaveMu.Unlock()
	return saveLimits(dir, limits)
}
func CheckpointLimits(dir string, s *engine.Scheduler) error {
	limitsSaveMu.Lock()
	defer limitsSaveMu.Unlock()
	return saveLimits(dir, s.Limits())
}
func saveLimits(dir string, limits map[string]float64) error {
	b, err := json.MarshalIndent(limitsFile{Version: limitsVersion, Limits: limits}, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(LimitsPath(dir), append(b, '\n'), 0644)
}
