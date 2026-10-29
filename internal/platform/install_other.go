//go:build !darwin

package platform

import "net-merge/internal/i18n"

import "errors"

// Install is the macOS-only setup of the passwordless tray (see install_darwin.go).
func Install(uninstall bool) error { return errors.New(i18n.T("install is only needed on macOS")) }
