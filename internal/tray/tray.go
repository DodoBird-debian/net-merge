// Package tray provides live relayed-traffic usage and connection controls.
package tray

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf16"

	"fyne.io/systray"
	"net-merge/internal/engine"
	"net-merge/internal/i18n"
	"net-merge/internal/icon"
	"net-merge/internal/logx"
	"net-merge/internal/platform"
	"net-merge/internal/state"
	"net-merge/internal/tun"
	"net-merge/internal/update"
)

type menuAction struct {
	kind, id, language string
	speed              float64
}
type adapterRow struct {
	settings  *systray.MenuItem
	item      *systray.MenuItem
	unlimited *systray.MenuItem
	presets   []*systray.MenuItem
	name      string
}
type trayUI struct {
	connectionsItem                                                                                                                        *systray.MenuItem
	lastOn                                                                                                                                 *bool
	tun                                                                                                                                    *tun.Tun
	bal                                                                                                                                    *engine.Balancer
	sched                                                                                                                                  *engine.Scheduler
	stateDir, version                                                                                                                      string
	startOn                                                                                                                                bool
	ready                                                                                                                                  chan struct{}
	ap                                                                                                                                     autoPauser
	actions                                                                                                                                chan menuAction
	updates                                                                                                                                chan string
	rows                                                                                                                                   map[string]*adapterRow
	header, totalUsage, toggleItem, v6Item, autoItem, signInItem, versionItem, limitsItem, languageItem, englishItem, arabicItem, quitItem *systray.MenuItem
	lastErr, latest, language                                                                                                              string
	iconOn, iconOff                                                                                                                        []byte
}

func Run(t *tun.Tun, sched *engine.Scheduler, bal *engine.Balancer, stateDir, version string, startOn, autoPause bool, ready chan struct{}) {
	u := &trayUI{tun: t, sched: sched, bal: bal, stateDir: stateDir, version: version, startOn: startOn, ready: ready, actions: make(chan menuAction, 32), updates: make(chan string, 1), rows: map[string]*adapterRow{}, iconOn: icon.Tray(true), iconOff: icon.Tray(false)}
	u.ap.enabled = autoPause
	sched.Pause(true)
	exit := func() {
		t.Close()
		if err := state.CheckpointQuotas(stateDir, sched); err != nil {
			logx.Printf(i18n.T("saving data allowances: %v\n"), err)
		}
		state.Clear(stateDir)
		closeNativeUI()
	}
	systray.Run(u.onReady, exit)
	exit()
}
func (u *trayUI) watch(item *systray.MenuItem, a menuAction) {
	go func() {
		for range item.ClickedCh {
			u.actions <- a
		}
	}()
}
func (u *trayUI) onReady() {
	systray.SetIcon(u.iconOff)
	u.header = systray.AddMenuItem("net-merge", "")
	u.header.Disable()
	u.totalUsage = systray.AddMenuItem("", "")
	u.totalUsage.Disable()
	systray.AddSeparator()
	u.connectionsItem = systray.AddMenuItem(i18n.T("Connections"), "")
	for _, st := range u.sched.Snapshot() {
		u.addRow(st)
	}
	systray.AddSeparator()
	u.limitsItem = systray.AddMenuItem("", "")
	u.watch(u.limitsItem, menuAction{kind: "limits"})
	u.languageItem = systray.AddMenuItem("العربية / English", "")
	u.englishItem = u.languageItem.AddSubMenuItemCheckbox("English", "", false)
	u.watch(u.englishItem, menuAction{kind: "language", language: "en"})
	u.arabicItem = u.languageItem.AddSubMenuItemCheckbox("العربية", "", false)
	u.watch(u.arabicItem, menuAction{kind: "language", language: "ar"})
	u.v6Item = systray.AddMenuItemCheckbox("", "", u.bal.V6Mode() == engine.V6Auto)
	u.watch(u.v6Item, menuAction{kind: "v6"})
	u.autoItem = systray.AddMenuItemCheckbox("", "", u.ap.enabled)
	u.watch(u.autoItem, menuAction{kind: "auto"})
	u.signInItem = systray.AddMenuItemCheckbox("", "", platform.AutostartEnabled())
	u.watch(u.signInItem, menuAction{kind: "signin"})
	u.toggleItem = systray.AddMenuItem("", "")
	u.watch(u.toggleItem, menuAction{kind: "toggle"})
	u.versionItem = systray.AddMenuItem(versionLabel(u.version), "")
	u.watch(u.versionItem, menuAction{kind: "version"})
	u.quitItem = systray.AddMenuItem("", "")
	u.watch(u.quitItem, menuAction{kind: "quit"})
	startNativeUI(u.sched, u.stateDir)
	if u.ready != nil {
		close(u.ready)
	}
	go u.watchUpdates()
	go func() {
		u.ap.start(u.startOn)
		if u.startOn {
			if !u.ap.enabled || u.sched.HasQuotas() {
				u.turn(true)
			} else if err := u.tun.Warm(); err != nil {
				logx.Printf(i18n.T("warning: %v\n"), err)
			}
		}
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			u.refresh()
			select {
			case a := <-u.actions:
				switch a.kind {
				case "quit":
					systray.Quit()
					return
				case "limits":
					showLimitsWindow()
				case "language":
					if err := i18n.Save(state.PreferencesDir(u.stateDir), a.language); err != nil {
						u.lastErr = err.Error()
					}
				case "speed":
					u.applyLimit(a.id, a.speed)
				case "toggle":
					u.manual(!u.tun.IsOn())
				case "auto":
					u.ap.setEnabled(!u.ap.enabled, u.tun.IsOn())
				case "signin":
					if err := platform.SetAutostart(!platform.AutostartEnabled()); err != nil {
						u.lastErr = err.Error()
					}
				case "v6":
					if u.bal.V6Mode() == engine.V6Auto {
						u.bal.SetV6Mode(engine.V6On)
					} else {
						u.bal.SetV6Mode(engine.V6Auto)
					}
				case "version":
					if err := platform.OpenURL(update.ReleasesURL); err != nil {
						u.lastErr = err.Error()
					}
				}
			case latest := <-u.updates:
				u.latest = latest
			case <-tick.C:
				if r, ok := state.TakeLimit(u.stateDir); ok {
					u.applyLimit(r.Adapter, r.Mbps)
				}
				if r, ok := state.TakeQuota(u.stateDir); ok {
					if err := state.ApplyQuota(u.stateDir, u.sched, r); err != nil {
						u.lastErr = err.Error()
					}
				}
				if state.TakeMarker(u.stateDir, "quit") {
					systray.Quit()
					return
				}
				if state.TakeMarker(u.stateDir, "off") {
					u.manual(false)
				}
				if state.TakeMarker(u.stateDir, "on") {
					u.manual(true)
				}
				if u.sched.HasQuotas() {
					// Never release capture routes automatically while protecting an allowance.
					if !u.tun.IsOn() && u.ap.waiting && !u.ap.manualOff {
						u.turn(true)
						u.ap.waiting = false
					}
					continue
				}
				switch u.ap.tick(time.Now(), u.tun.IsOn(), u.usable(), u.movingFlows()) {
				case autoPause:
					u.turn(false)
				case autoResume:
					u.turn(true)
					if !u.tun.IsOn() {
						u.ap.resumeFailed()
					}
				}
			}
		}
	}()
}
func (u *trayUI) addRow(st engine.SlotStats) {
	if _, ok := u.rows[st.ID]; ok {
		return
	}
	r := &adapterRow{item: u.connectionsItem.AddSubMenuItem(st.Name, "")}
	r.unlimited = r.item.AddSubMenuItemCheckbox(i18n.T("Unlimited"), "", false)
	u.watch(r.unlimited, menuAction{kind: "speed", id: st.ID})
	for _, speed := range limitPresets {
		it := r.item.AddSubMenuItemCheckbox(limitText(speed), limitTip(speed), false)
		r.presets = append(r.presets, it)
		u.watch(it, menuAction{kind: "speed", id: st.ID, speed: speed})
	}
	r.settings = r.item.AddSubMenuItem(i18n.T("Connection limits…"), "")
	u.watch(r.settings, menuAction{kind: "limits"})
	u.rows[st.ID] = r
}
func setChecked(it *systray.MenuItem, v bool) {
	if it.Checked() != v {
		if v {
			it.Check()
		} else {
			it.Uncheck()
		}
	}
}
func (u *trayUI) manual(on bool) { u.ap.userSet(on, u.usable()); u.turn(on) }
func (u *trayUI) usable() (n int) {
	for _, s := range u.sched.Snapshot() {
		if s.Present && !s.QuotaExhausted && (s.State == "up" || s.State6 == "up") {
			n++
		}
	}
	return
}
func (u *trayUI) movingFlows() (n int64) {
	var rate float64
	for _, s := range u.sched.Snapshot() {
		if s.Present {
			n += s.Active
			rate += s.RateIn + s.RateOut
		}
	}
	if rate < quietRate {
		return 0
	}
	return
}
func (u *trayUI) turn(on bool) {
	u.lastErr = ""
	if !on {
		u.tun.Off()
		return
	}
	if err := u.tun.On(); err != nil {
		u.lastErr = err.Error()
		logx.Printf(i18n.T("error: %v\n"), err)
	}
}
func (u *trayUI) refresh() {
	if err := i18n.Init(state.PreferencesDir(u.stateDir)); err != nil {
		u.lastErr = err.Error()
	}
	on := u.tun.IsOn()
	if u.lastOn == nil || *u.lastOn != on {
		if on {
			systray.SetIcon(u.iconOn)
		} else {
			systray.SetIcon(u.iconOff)
		}
		v := on
		u.lastOn = &v
	}
	stats := u.sched.Snapshot()
	var down, up float64
	var used int64
	for _, s := range stats {
		used += s.SessionBytes
		if on && s.Present {
			down += s.RateIn
			up += s.RateOut
		}
	}
	word := stateWord(on)
	if on && u.sched.AllExhausted() {
		word = i18n.T("Data allowances exhausted")
	}
	if !on && u.ap.enabled && u.ap.waiting && !u.ap.manualOff {
		word = i18n.T("Paused: one connection")
	}
	if err := u.sched.StorageError(); err != "" {
		word = i18n.T("Cannot save data usage; traffic blocked") + ": " + err
	}
	u.header.SetTitle(fmt.Sprintf(i18n.T("%s · %s ↓%s ↑%s Mbps"), word, i18n.T("Total"), mbps(down), mbps(up)))
	u.totalUsage.SetTitle(i18n.T("Used since start") + ": " + gb(used))
	if u.lastErr != "" {
		u.header.SetTitle(i18n.T("Error") + ": " + u.lastErr)
	}
	for _, s := range stats {
		u.addRow(s)
		r := u.rows[s.ID]
		rateIn, rateOut := s.RateIn, s.RateOut
		if !on || !s.Present {
			rateIn, rateOut = 0, 0
		}
		txt := fmt.Sprintf(i18n.T("%s: ↓%s ↑%s Mbps · %s: %s"), i18n.LTR(s.Name), mbps(rateIn), mbps(rateOut), i18n.T("Used since start"), gb(s.SessionBytes))
		switch {
		case !s.Present:
			txt += " · " + i18n.T("Disconnected")
		case !on:
			txt += " · " + i18n.T("Paused")
		case s.QuotaExhausted:
			txt += " · " + i18n.T("Data allowance exhausted")
		}
		if s.QuotaBytes > 0 {
			txt += fmt.Sprintf(i18n.T(" · %s: %s / %s"), i18n.T("Allowance used"), gb(s.QuotaUsedBytes), gb(s.QuotaBytes))
		}
		r.item.SetTitle(txt)
		r.settings.SetTitle(i18n.T("Connection limits…"))
		r.unlimited.SetTitle(i18n.T("Unlimited"))
		r.unlimited.SetTooltip(limitTip(0))
		setChecked(r.unlimited, s.LimitMbps <= 0)
		for j, it := range r.presets {
			it.SetTooltip(limitTip(limitPresets[j]))
			setChecked(it, math.Abs(limitPresets[j]-s.LimitMbps) < 0.01)
		}
	}
	u.connectionsItem.SetTitle(i18n.T("Connections"))
	u.limitsItem.SetTitle(i18n.T("Connection limits…"))
	u.v6Item.SetTitle(i18n.T("Use all adapters (IPv6 only if every adapter has it)"))
	u.v6Item.SetTooltip(i18n.T("Enable IPv6 only when all eligible adapters support it."))
	u.autoItem.SetTitle(i18n.T("Pause when there is only one connection"))
	u.autoItem.SetTooltip(i18n.T("Automatic pause is suspended while a data allowance is enabled."))
	setChecked(u.autoItem, u.ap.enabled)
	u.signInItem.SetTitle(i18n.T("Start at sign-in"))
	if on {
		u.toggleItem.SetTitle(i18n.T("Pause merging"))
	} else {
		u.toggleItem.SetTitle(i18n.T("Start merging"))
	}
	u.toggleItem.SetTooltip(i18n.T("While paused, traffic bypasses net-merge and is not counted or limited."))
	u.quitItem.SetTitle(i18n.T("Quit"))
	setChecked(u.englishItem, i18n.Language() == "en")
	setChecked(u.arabicItem, i18n.Language() == "ar")
	if u.latest != "" {
		u.versionItem.SetTitle(updateLabel(u.latest))
	} else {
		u.versionItem.SetTitle(versionLabel(u.version))
	}
	u.versionItem.SetTooltip(i18n.T("Open the release page on GitHub."))
	tip := usageTooltip(word, down, up, stats, on)
	setNativeHover(tip, i18n.RTL())
}
func (u *trayUI) applyLimit(name string, speed float64) {
	u.sched.SetLimit(name, speed)
	if err := state.CheckpointLimits(u.stateDir, u.sched); err != nil {
		u.lastErr = err.Error()
	}
}
func usageTooltip(word string, down, up float64, stats []engine.SlotStats, on bool) string {
	lines := []string{"net-merge · " + word}
	var used int64
	for _, s := range stats {
		used += s.SessionBytes
		d, u := s.RateIn, s.RateOut
		if !on || !s.Present {
			d, u = 0, 0
		}
		name := i18n.LTR(s.Name)
		if !s.Present {
			name += " (" + i18n.T("Disconnected") + ")"
		}
		lines = append(lines, fmt.Sprintf(i18n.T("%s: ↓%s ↑%s Mbps\n%s: %s"), name, mbps(d), mbps(u), i18n.T("Used since start"), gb(s.SessionBytes)))
	}
	lines = append(lines, fmt.Sprintf(i18n.T("%s: ↓%s ↑%s Mbps\n%s: %s"), i18n.T("Total"), mbps(down), mbps(up), i18n.T("Used since start"), gb(used)))
	return strings.Join(lines, "\n\n")
}
func gb(bytes int64) string {
	return i18n.LTR(fmt.Sprintf(i18n.T("%.2f GB"), float64(bytes)/float64(engine.BytesPerGB)))
}

const tooltipMax = 63

func stateWord(on bool) string {
	if on {
		return i18n.T("Merging")
	}
	return i18n.T("Paused")
}
func versionLabel(v string) string {
	if v = strings.TrimSpace(v); v == "" {
		v = "dev"
	}
	return withV(v)
}
func updateLabel(v string) string { return i18n.T("Update to ") + withV(v) }
func withV(v string) string {
	if v == "" || v == "dev" || strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}
func mbps(v float64) string {
	m := v * 8 / 1e6
	if m < 9.95 {
		return fmt.Sprintf(i18n.T("%.1f"), m)
	}
	return fmt.Sprintf(i18n.T("%.0f"), m)
}
func shorten(s string, n int) string {
	if utf16Len(s) <= n {
		return s
	}
	units := 0
	for i, r := range s {
		w := utf16.RuneLen(r)
		if units+w > n-1 {
			return s[:i] + "…"
		}
		units += w
	}
	return s
}
func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

var shortNamer = strings.NewReplacer("Ethernet", "Eth", "Wi-Fi", "WiFi", "Local Area Connection", "LAN", "Cellular", "Cell")

func shortName(s string) string { return shortNamer.Replace(s) }

// Legacy compact tooltip retained for non-Windows integrations and compatibility.
func trayTooltip(word string, down, up float64, live []engine.SlotStats) string {
	build := func(head string, withUp bool, unit string) string {
		lines := []string{head}
		for _, s := range live {
			l := shortName(s.Name) + " ↓" + mbps(s.LinkIn)
			if withUp {
				l += " ↑" + mbps(s.LinkOut)
			}
			if s.State == "no internet" && s.State6 != "up" {
				l += " (no net)"
			}
			lines = append(lines, l)
		}
		lines = append(lines, "Total ↓"+mbps(down)+" ↑"+mbps(up)+unit)
		return strings.Join(lines, "\n")
	}
	leanest := build(word, false, "")
	for _, v := range []string{build("net-merge · "+word, true, " Mbps"), build(word, true, " Mbps"), build(word, false, " Mbps"), leanest} {
		if utf16Len(v) <= tooltipMax {
			return v
		}
	}
	return shorten(leanest, tooltipMax)
}

var limitPresets = []float64{1, 2, 5, 10, 20, 50, 100}

func limitLabel(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf(i18n.T("%.0f"), v)
	}
	return fmt.Sprintf(i18n.T("%.1f"), v)
}
func limitText(v float64) string { return limitLabel(v) + " Mbit/s" }
func limitTip(v float64) string {
	if v <= 0 {
		return i18n.T("No cap: the adapter may use its full speed.")
	}
	return fmt.Sprintf(i18n.T("Cap this adapter at %s for downloads and uploads, across all of its flows. Takes effect immediately."), limitText(v))
}
