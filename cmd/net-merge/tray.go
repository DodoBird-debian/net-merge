package main

import (
	"flag"
	"fmt"
	"net-merge/internal/i18n"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"net-merge/internal/logx"
	"net-merge/internal/platform"
	"net-merge/internal/state"
	"net-merge/internal/tray"
	"net-merge/internal/tun"
)

func cmdTray(args []string) {
	fs := newFlags("tray", flag.ExitOnError)
	var c common
	c.register(fs)
	hidden := fs.Bool("hidden", true, i18n.T("hide the console window of the elevated process"))
	startOff := fs.Bool("off", false, i18n.T("start with system-wide mode OFF (default: ON)"))
	autoPause := fs.Bool("auto-pause", true, i18n.T("pause by itself when fewer than two connections work, and resume when two do (pausing by hand always wins)"))
	fs.Parse(args)

	if !platform.IsAdmin() {
		child := append([]string{"tray"}, args...)
		if c.stateDir != state.DefaultDir() { // the macOS passwordless rule only matches a bare `tray`
			child = append(child, "-state-dir", c.stateDir)
		}
		if err := platform.RelaunchTray(child, *hidden); err != nil {
			die(err)
		}
		fmt.Println(i18n.T("Approve the admin prompt. Look for the net-merge icon in the system tray (^ overflow arrow) or menu bar."))
		return
	}
	if !platform.SingleInstance() {
		fmt.Println(i18n.T("net-merge is already running."))
		return
	}
	if c.logFile == "" {
		c.stats = 0 // nobody is watching a hidden console
		if !platform.HasConsole() {
			c.logFile = filepath.Join(c.stateDir, "net-merge.log") // windowless build: keep a log for diagnosis
		}
	}
	if err := c.setup(); err != nil {
		die(err)
	}
	if err := platform.CheckDriver(); err != nil {
		die(err)
	}

	t := tun.New(nil)
	t.MTU = c.mtu
	mode := func() string {
		if t.IsOn() {
			return "system-wide (tray)"
		}
		return "paused"
	}
	sched, bal, err := c.boot(mode)
	if err != nil {
		die(err)
	}
	t.B = bal

	// Started at sign-in this process can beat the desktop shell by a fraction of a second. With no
	// taskbar the tray library fails to create the icon and never becomes ready, so wait for it.
	if !platform.WaitForTray(3 * time.Minute) {
		logx.Printf("%s", i18n.T("warning: taskbar not found after 3 minutes, trying anyway\n"))
	}
	ready := make(chan struct{})
	go func() { // safety net: if the icon still can't be created, exit so autostart can retry
		select {
		case <-ready:
		case <-time.After(30 * time.Second):
			logx.Printf("%s", i18n.T("error: tray icon did not initialise within 30s; exiting so autostart can restart it\n"))
			os.Exit(4)
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { // logout, `launchctl bootout`, kill: turn off cleanly instead of leaving routes behind
		<-sig
		t.Close()
		_ = state.CheckpointQuotas(c.stateDir, sched)
		state.Clear(c.stateDir)
		os.Exit(0)
	}()
	tray.Run(t, sched, bal, c.stateDir, version, !*startOff, *autoPause, ready)
}
