package tray

import (
	"golang.org/x/sys/windows"
	"image"
	"image/png"
	"net"
	"net-merge/internal/engine"
	"net-merge/internal/i18n"
	"net-merge/internal/nics"
	"net-merge/internal/state"
	"os"
	"path/filepath"
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
		captureNativeWindow(t, native.window, "limits-english.png")
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
		captureNativeWindow(t, native.window, "limits-arabic.png")
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
		captureNativeWindow(t, native.hover, "hover-arabic.png")
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

// Capture only the test's own windows, never the runner's desktop or other apps.
func captureNativeWindow(t *testing.T, hwnd uintptr, name string) {
	t.Helper()
	dir := os.Getenv("NETMERGE_UI_SCREENSHOTS")
	if dir == "" {
		return
	}
	win("SetWindowPos").Call(hwnd, ^uintptr(0), 0, 0, 0, 0, 0x53)
	win("RedrawWindow").Call(hwnd, 0, 0, 0x185)
	win("UpdateWindow").Call(hwnd)
	dwm := windows.NewLazySystemDLL("dwmapi.dll")
	dwm.NewProc("DwmFlush").Call()
	var r rect
	win("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&r)))
	var frame rect
	if result, _, _ := dwm.NewProc("DwmGetWindowAttribute").Call(hwnd, 9, uintptr(unsafe.Pointer(&frame)), unsafe.Sizeof(frame)); result == 0 {
		r = frame
	}
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		t.Error("invalid capture size")
		return
	}
	dc, _, _ := win("GetDC").Call(0)
	defer win("ReleaseDC").Call(0, dc)
	mem, _, _ := gdi32.NewProc("CreateCompatibleDC").Call(dc)
	defer gdi32.NewProc("DeleteDC").Call(mem)
	bitmap, _, _ := gdi32.NewProc("CreateCompatibleBitmap").Call(dc, uintptr(w), uintptr(h))
	defer gdi32.NewProc("DeleteObject").Call(bitmap)
	old, _, _ := gdi32.NewProc("SelectObject").Call(mem, bitmap)
	gdi32.NewProc("SetLayout").Call(mem, 0)
	copied, _, _ := gdi32.NewProc("BitBlt").Call(mem, 0, 0, uintptr(w), uintptr(h), dc, uintptr(r.Left), uintptr(r.Top), 0xc0cc0020)
	gdi32.NewProc("SelectObject").Call(mem, old)
	if copied == 0 {
		t.Error("could not capture native window")
		return
	}
	type bitmapHeader struct {
		Size                   uint32
		Width, Height          int32
		Planes, Bits           uint16
		Compression, ImageSize uint32
		XPixels, YPixels       int32
		Colors, Important      uint32
	}
	header := bitmapHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, Bits: 32}
	pixels := make([]byte, w*h*4)
	if rows, _, _ := gdi32.NewProc("GetDIBits").Call(dc, bitmap, 0, uintptr(h), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&header)), 0); rows == 0 {
		t.Error("GetDIBits failed")
		return
	}
	result := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(pixels); i += 4 {
		result.Pix[i], result.Pix[i+1], result.Pix[i+2], result.Pix[i+3] = pixels[i+2], pixels[i+1], pixels[i], 255
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Error(err)
		return
	}
	file, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Error(err)
		return
	}
	defer file.Close()
	if err := png.Encode(file, result); err != nil {
		t.Error(err)
	}
}
