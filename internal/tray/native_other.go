//go:build !windows

package tray

import (
	"fyne.io/systray"
	"net-merge/internal/engine"
	"net-merge/internal/i18n"
	"net-merge/internal/platform"
)

func startNativeUI(_ *engine.Scheduler, _ string) {}
func closeNativeUI()                              {}
func setNativeHover(text string, _ bool)          { systray.SetTooltip(text) }
func showLimitsWindow() {
	platform.ShowError(i18n.T("The connection limits window is available on Windows. Use net-merge limit and net-merge quota on this platform."))
}
