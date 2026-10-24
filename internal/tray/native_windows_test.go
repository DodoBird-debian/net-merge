package tray

import (
	"net"
	"net-merge/internal/engine"
	"net-merge/internal/i18n"
	"net-merge/internal/nics"
	"net-merge/internal/state"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func TestNativeWindowsLimitsAndArabicHover(t *testing.T) {
	defer i18n.Set("en")
	s := engine.NewScheduler([]nics.Nic{{ID: "stable-a", Name: "Wi-Fi اتصال", Index: 1, IP: net.ParseIP("127.0.0.1"), Weight: 1}})
	dir := t.TempDir()
	startNativeUI(s, dir)
	select {
	case <-native.ready:
	case <-time.After(10 * time.Second):
		t.Fatal("native UI thread did not initialize")
	}
	if native.hwnd.Load() == 0 {
		t.Fatal("Windows controls could not be created")
	}
	defer func() {
		closeNativeUI()
		select {
		case <-native.done:
		case <-time.After(5 * time.Second):
			t.Error("UI thread did not stop")
		}
	}()
	native.invoke(func() {
		native.open()
		if native.window == 0 || native.combo == 0 || native.quota == 0 || native.speed == 0 {
			t.Error("missing connection controls")
		}
		text(native.speed, "٧٫٥")
		text(native.quota, "٢٫٥")
		send(native.speedUnlimited, 0xf1, 0, 0)
		send(native.quotaUnlimited, 0xf1, 0, 0)
		if err := native.saveInputs(); err != nil {
			t.Error(err)
		}
	})
	st := s.Snapshot()[0]
	if st.LimitMbps != 7.5 || st.QuotaBytes != 2_500_000_000 {
		t.Fatalf("native Arabic input was not applied: %+v", st)
	}
	records, err := state.LoadQuotas(dir)
	if err != nil || records["stable-a"].LimitBytes != st.QuotaBytes {
		t.Fatal("native controls did not persist allowance", records, err)
	}
	i18n.Set("ar")
	native.invoke(func() {
		native.tick()
		title := getText(native.window)
		if title != i18n.T("Connection limits") {
			t.Error("language switch did not rebuild window", title)
		}
		style, _, _ := win("GetWindowLongPtrW").Call(native.window, ^uintptr(19))
		if style&wsRTL == 0 {
			t.Error("Arabic window is not RTL")
		}
		native.fill()
		send(native.quotaUnlimited, 0xf1, 1, 0)
		native.enableInputs()
		if enabled, _, _ := win("IsWindowEnabled").Call(native.quota); enabled != 0 {
			t.Error("Unlimited did not disable numeric allowance")
		}
		if err := native.saveInputs(); err != nil {
			t.Error(err)
		}
	})
	if s.Snapshot()[0].QuotaBytes != 0 {
		t.Fatal("Unlimited control did not clear allowance")
	}
	tip := usageTooltip(stateWord(false), 0, 0, s.Snapshot(), false)
	setNativeHover(tip, true)
	native.invoke(func() {
		native.showHover()
		if native.hover == 0 || native.hoverText == 0 {
			t.Error("missing hover panel")
		}
		got := getText(native.hoverText)
		for _, want := range []string{i18n.T("Paused"), i18n.T("Used since start"), i18n.T("Total"), "0.00 GB", "Wi-Fi"} {
			if !strings.Contains(got, want) {
				t.Errorf("hover panel missing %q in %q", want, got)
			}
		}
		var r rect
		win("GetWindowRect").Call(native.hover, uintptr(unsafe.Pointer(&r)))
		if r.Right <= r.Left || r.Bottom <= r.Top {
			t.Error("invalid hover panel size")
		}
	})
}
