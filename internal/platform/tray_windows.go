package platform

import "net-merge/internal/i18n"

import "fmt"

// RelaunchTray is RelaunchElevated for the tray.
func RelaunchTray(args []string, hidden bool) error {
	if err := RelaunchElevated(args, hidden); err != nil {
		return fmt.Errorf(i18n.T("could not start elevated (UAC declined?): %w"), err)
	}
	return nil
}
