package state

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// replaceFile renames tmp over dst. Windows refuses that while another process has dst open.
// A CLI reader may briefly hold limits.json or quotas.json, so wait for it to finish.
func replaceFile(tmp, dst string) error {
	deadline := time.Now().Add(time.Second)
	for {
		err := os.Rename(tmp, dst)
		if err == nil || time.Now().After(deadline) ||
			!errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}
