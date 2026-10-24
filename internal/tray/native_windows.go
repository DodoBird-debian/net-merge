package tray

// The controls and hover panel use Win32 directly: no browser runtime, server,
// or second network process. Each window belongs to this locked UI thread.
import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"fyne.io/systray"
	"golang.org/x/sys/windows"
	"net-merge/internal/engine"
	"net-merge/internal/i18n"
	"net-merge/internal/state"
)

var user32 = windows.NewLazySystemDLL("user32.dll")
var kernel32 = windows.NewLazySystemDLL("kernel32.dll")
var gdi32 = windows.NewLazySystemDLL("gdi32.dll")
var shell32 = windows.NewLazySystemDLL("shell32.dll")

func win(name string) *windows.LazyProc { return user32.NewProc(name) }
func u16(s string) *uint16              { p, _ := windows.UTF16PtrFromString(s); return p }

const (
	wmShowLimits = 0x8001
	wmHover      = 0x8002
	wmStop       = 0x8003
	wmHideHover  = 0x8004
	wmInvoke     = 0x8005
	wmTimer      = 0x113
	wmCommand    = 0x111
	wmClose      = 0x10
	wmDestroy    = 2
	wsChild      = 0x40000000
	wsVisible    = 0x10000000
	wsTabStop    = 0x10000
	wsBorder     = 0x800000
	wsRTL        = 0x400000
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }
type winMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          point
	Private        uint32
}
type winClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}
type iconIdentifier struct {
	Size   uint32
	Window uintptr
	ID     uint32
	GUID   windows.GUID
}
type nativeWindows struct {
	mu                                                             sync.Mutex
	text                                                           string
	rtl                                                            bool
	hwnd                                                           atomic.Uintptr
	window, hover, hoverText, combo, speed, quota, used, remaining uintptr
	speedUnlimited, quotaUnlimited                                 uintptr
	lang                                                           string
	ready, done                                                    chan struct{}
	tasks                                                          chan func()
	adapterSignature                                               string
	ids                                                            []string
	selected                                                       string
	sched                                                          *engine.Scheduler
	dir                                                            string
	trayWindow                                                     uintptr
	oldProc                                                        atomic.Uintptr
	hooked                                                         atomic.Bool
	hoverPoint                                                     point
	hoverVisible                                                   bool
}

var native nativeWindows
var nativeProc = syscall.NewCallback(nativeWndProc)
var trayHookProc = syscall.NewCallback(trayHook)

func startNativeUI(s *engine.Scheduler, dir string) {
	native.ready = make(chan struct{})
	native.done = make(chan struct{})
	native.tasks = make(chan func(), 8)
	native.sched = s
	native.dir = dir
	go native.run()
}
func closeNativeUI() {
	if h := native.hwnd.Load(); h != 0 {
		win("PostMessageW").Call(h, wmStop, 0, 0)
	}
}
func showLimitsWindow() {
	if h := native.hwnd.Load(); h != 0 {
		win("PostMessageW").Call(h, wmShowLimits, 0, 0)
	}
}
func setNativeHover(text string, rtl bool) {
	native.mu.Lock()
	native.text, native.rtl = text, rtl
	native.mu.Unlock()
	if native.hooked.Load() {
		systray.SetTooltip("")
	} else {
		systray.SetTooltip(shorten(strings.ReplaceAll(text, "\n", " "), tooltipMax))
	}
}
func (n *nativeWindows) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(n.done)
	ready := false
	defer func() {
		if !ready {
			close(n.ready)
		}
	}()
	inst, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	cursor, _, _ := win("LoadCursorW").Call(0, 32512)
	class := winClass{Proc: nativeProc, Instance: inst, Cursor: cursor, Background: 16, Name: u16("NetMergeControls")}
	class.Size = uint32(unsafe.Sizeof(class))
	if r, _, _ := win("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class))); r == 0 {
		return
	}
	controller := createWindow(0, "NetMergeControls", "", 0, 0, 0, 0, 0, 0, 0)
	if controller == 0 {
		return
	}
	n.hwnd.Store(controller)
	close(n.ready)
	ready = true
	defer n.hwnd.Store(0)
	win("SetTimer").Call(controller, 1, 250, 0)
	var m winMessage
	for {
		r, _, _ := win("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if n.window != 0 {
			handled, _, _ := win("IsDialogMessageW").Call(n.window, uintptr(unsafe.Pointer(&m)))
			if handled != 0 {
				continue
			}
		}
		win("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
		win("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
	}
	if n.trayWindow != 0 && n.oldProc.Load() != 0 {
		win("SetWindowLongPtrW").Call(n.trayWindow, ^uintptr(3), n.oldProc.Load())
	}
	if n.window != 0 {
		win("DestroyWindow").Call(n.window)
	}
	if n.hover != 0 {
		win("DestroyWindow").Call(n.hover)
	}
	win("DestroyWindow").Call(controller)
}
func createWindow(ex uintptr, class, title string, style uintptr, x, y, w, h int32, parent, id uintptr) uintptr {
	inst, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	r, _, _ := win("CreateWindowExW").Call(ex, uintptr(unsafe.Pointer(u16(class))), uintptr(unsafe.Pointer(u16(title))), style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, id, inst, 0)
	return r
}
func text(h uintptr, s string) { win("SetWindowTextW").Call(h, uintptr(unsafe.Pointer(u16(s)))) }
func getText(h uintptr) string {
	length, _, _ := win("GetWindowTextLengthW").Call(h)
	b := make([]uint16, length+1)
	win("GetWindowTextW").Call(h, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	return windows.UTF16ToString(b)
}
func send(h uintptr, m uint32, w, l uintptr) uintptr {
	r, _, _ := win("SendMessageW").Call(h, uintptr(m), w, l)
	return r
}
func (n *nativeWindows) control(class, label string, style uintptr, x, y, w, h int32, id uintptr) uintptr {
	ex := uintptr(0)
	if i18n.RTL() {
		ex = wsRTL | 0x2000
	}
	if class == "EDIT" || class == "COMBOBOX" {
		ex = 0x200
	} // numeric/name content remains LTR
	hwnd := createWindow(ex, class, i18n.T(label), wsChild|wsVisible|style, x, y, w, h, n.window, id)
	font, _, _ := gdi32.NewProc("GetStockObject").Call(17)
	send(hwnd, 0x30, font, 1)

	return hwnd
}
func (n *nativeWindows) open() {
	if n.window != 0 {
		win("ShowWindow").Call(n.window, 5)
		win("SetForegroundWindow").Call(n.window)
		return
	}

	n.lang = i18n.Language()
	ex := uintptr(0x10000)
	if i18n.RTL() {
		ex |= wsRTL | 0x100000
	}
	n.window = createWindow(ex, "NetMergeControls", i18n.T("Connection limits"), 0x00CA0000, 120, 120, 600, 485, 0, 0)
	if n.window == 0 {
		return
	}
	n.control("STATIC", "Connection", 0, 22, 18, 535, 24, 0)
	n.combo = n.control("COMBOBOX", "", wsTabStop|0x200000|3, 22, 44, 535, 300, 101)
	n.control("STATIC", "Speed limit (Mbps, each direction)", 0, 22, 86, 350, 24, 0)
	n.speed = n.control("EDIT", "", wsTabStop|wsBorder|0x80, 22, 113, 240, 27, 102)
	n.speedUnlimited = n.control("BUTTON", "Unlimited", wsTabStop|3, 285, 113, 260, 27, 103)
	n.control("STATIC", "Data allowance (GB, upload + download)", 0, 22, 155, 535, 24, 0)
	n.quota = n.control("EDIT", "", wsTabStop|wsBorder|0x80, 22, 182, 240, 27, 104)
	n.quotaUnlimited = n.control("BUTTON", "Unlimited", wsTabStop|3, 285, 182, 260, 27, 105)
	n.used = n.control("STATIC", "", 0, 22, 225, 535, 24, 0)
	n.remaining = n.control("STATIC", "", 0, 22, 254, 535, 24, 0)
	n.control("STATIC", "Only traffic relayed through net-merge is counted and limited. While paused, traffic is not counted or limited. This is not your ISP billing total.", 0, 22, 289, 535, 65, 0)
	n.control("BUTTON", "Apply", wsTabStop|1, 22, 365, 150, 33, 106)
	n.control("BUTTON", "Reset used data…", wsTabStop, 187, 365, 190, 33, 107)
	n.control("BUTTON", "Close", wsTabStop, 392, 365, 165, 33, 108)
	n.refreshAdapters()
	n.fill()
	win("ShowWindow").Call(n.window, 5)
	win("SetForegroundWindow").Call(n.window)
	win("SetFocus").Call(n.combo)
}
func (n *nativeWindows) refreshAdapters() {
	stats := n.sched.Snapshot()
	ids := make([]string, 0, len(stats))
	var signature strings.Builder
	for _, st := range stats {
		ids = append(ids, st.ID)
		fmt.Fprintf(&signature, "%s:%s:%t;", st.ID, st.Name, st.Present)
	}
	if signature.String() == n.adapterSignature && len(ids) > 0 && len(n.ids) == len(ids) {
		return
	}
	n.adapterSignature = signature.String()
	n.ids = ids
	send(n.combo, 0x14b, 0, 0)
	selected := 0
	for j, st := range stats {
		name := st.Name
		if !st.Present {
			name += " (" + i18n.T("Disconnected") + ")"
		}
		send(n.combo, 0x143, 0, uintptr(unsafe.Pointer(u16(name))))
		if st.ID == n.selected {
			selected = j
		}
	}
	if len(ids) > 0 {
		send(n.combo, 0x14e, uintptr(selected), 0)
		n.selected = ids[selected]
	} else {
		n.selected = ""
	}
	for _, h := range []uintptr{n.speed, n.quota, n.speedUnlimited, n.quotaUnlimited} {
		win("EnableWindow").Call(h, boolInt(n.selected != ""))
	}
}
func boolInt(v bool) uintptr {
	if v {
		return 1
	}
	return 0
}
func (n *nativeWindows) current() (engine.SlotStats, bool) {
	for _, st := range n.sched.Snapshot() {
		if st.ID == n.selected {
			return st, true
		}
	}
	return engine.SlotStats{}, false
}
func (n *nativeWindows) fill() {
	st, ok := n.current()
	if !ok {
		text(n.used, i18n.T("No network adapters detected."))
		return
	}
	text(n.speed, strconv.FormatFloat(st.LimitMbps, 'f', -1, 64))
	send(n.speedUnlimited, 0xf1, boolInt(st.LimitMbps == 0), 0)
	text(n.quota, strconv.FormatFloat(float64(st.QuotaBytes)/1e9, 'f', -1, 64))
	send(n.quotaUnlimited, 0xf1, boolInt(st.QuotaBytes == 0), 0)
	n.enableInputs()
	n.refreshUsage()
}
func (n *nativeWindows) enableInputs() {
	win("EnableWindow").Call(n.speed, boolInt(send(n.speedUnlimited, 0xf0, 0, 0) == 0))
	win("EnableWindow").Call(n.quota, boolInt(send(n.quotaUnlimited, 0xf0, 0, 0) == 0))
}
func (n *nativeWindows) refreshUsage() {
	st, ok := n.current()
	if !ok {
		return
	}
	text(n.used, fmt.Sprintf(i18n.T("Allowance used: %s · Used since start: %s"), gb(st.QuotaUsedBytes), gb(st.SessionBytes)))
	remaining := i18n.T("Unlimited")
	if st.QuotaBytes > 0 {
		remaining = gb(st.QuotaRemainingBytes)
	}
	label := i18n.T("Remaining") + ": " + remaining
	if st.QuotaExhausted {
		label += " · " + i18n.T("Data allowance exhausted")
	}
	text(n.remaining, label)
}
func (n *nativeWindows) message(message string, confirm bool) bool {
	flags := uintptr(0x40)
	if confirm {
		flags = 0x124
	} // warning + Yes/No + default No
	if i18n.RTL() {
		flags |= 0x180000
	}
	result, _, _ := win("MessageBoxW").Call(n.window, uintptr(unsafe.Pointer(u16(message))), uintptr(unsafe.Pointer(u16(i18n.T("Connection limits")))), flags)
	return result == 6
}
func (n *nativeWindows) saveInputs() error {
	if n.selected == "" {
		return fmt.Errorf("%s", i18n.T("No network adapters detected."))
	}
	speed := 0.0
	quota := int64(0)
	var err error
	if send(n.speedUnlimited, 0xf0, 0, 0) == 0 {
		speed, err = engine.ParseLimit(getText(n.speed))
		if err == nil && speed <= 0 {
			err = fmt.Errorf("%s", i18n.T("enter a positive speed or select Unlimited"))
		}
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("Invalid speed limit"), err)
		}
	}
	if send(n.quotaUnlimited, 0xf0, 0, 0) == 0 {
		quota, err = engine.ParseQuota(getText(n.quota))
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("Invalid data allowance"), err)
		}
	}
	n.sched.SetLimit(n.selected, speed)
	if err = state.CheckpointLimits(n.dir, n.sched); err != nil {
		return err
	}
	return state.ApplyQuota(n.dir, n.sched, state.QuotaRequest{Adapter: n.selected, Action: "set", Bytes: quota})
}
func (n *nativeWindows) apply() {
	if err := n.saveInputs(); err != nil {
		n.message(i18n.T("Could not save connection limits")+": "+err.Error(), false)
		return
	}
	n.fill()
	n.message(i18n.T("Connection limits saved."), false)
}

// invoke marshals work onto the window thread; callers never read window state concurrently.
func (n *nativeWindows) invoke(fn func()) {
	done := make(chan struct{})
	n.tasks <- func() { fn(); close(done) }
	win("PostMessageW").Call(n.hwnd.Load(), wmInvoke, 0, 0)
	<-done
}

func nativeWndProc(hwnd uintptr, message uint32, w, l uintptr) uintptr {
	n := &native
	switch message {
	case wmInvoke:
		select {
		case fn := <-n.tasks:
			fn()
		default:
		}
		return 0
	case wmShowLimits:
		n.open()
		return 0
	case wmHover:
		n.showHover()
		return 0
	case wmHideHover:
		if n.hover != 0 {
			win("ShowWindow").Call(n.hover, 0)
			n.hoverVisible = false
		}
		return 0
	case wmStop:
		win("PostQuitMessage").Call(0)
		return 0
	case wmTimer:
		n.tick()
		return 0
	case 0x21:
		if hwnd == n.hover {
			return 3
		} // MA_NOACTIVATE
	case wmClose:
		if hwnd == n.window {
			win("DestroyWindow").Call(hwnd)
			n.window = 0
			n.ids = nil
			return 0
		}
		if hwnd == n.hover {
			win("ShowWindow").Call(hwnd, 0)
			return 0
		}
	case wmDestroy:
		if hwnd == n.window {
			n.window = 0
		}
		return 0
	case wmCommand:
		id := w & 0xffff
		notification := (w >> 16) & 0xffff
		switch id {
		case 101:
			if notification == 1 {
				index := int(send(n.combo, 0x147, 0, 0))
				if index >= 0 && index < len(n.ids) {
					n.selected = n.ids[index]
					n.fill()
				}
			}
		case 103, 105:
			n.enableInputs()
		case 106:
			n.apply()
		case 107:
			if n.selected != "" && n.message(i18n.T("Reset this connection's allowance usage to zero? Session usage will not change."), true) {
				if err := state.ApplyQuota(n.dir, n.sched, state.QuotaRequest{Adapter: n.selected, Action: "reset"}); err != nil {
					n.message(i18n.T("Could not save connection limits")+": "+err.Error(), false)
				}
				n.fill()
			}
		case 108, 2:
			if n.window != 0 {
				win("DestroyWindow").Call(n.window)
				n.window = 0
				n.ids = nil
			}
		}
		return 0
	}
	r, _, _ := win("DefWindowProcW").Call(hwnd, uintptr(message), w, l)
	return r
}
func (n *nativeWindows) tick() {
	if n.trayWindow == 0 {
		h, _, _ := win("FindWindowW").Call(uintptr(unsafe.Pointer(u16("SystrayClass"))), 0)
		if h != 0 {
			var pid uint32
			win("GetWindowThreadProcessId").Call(h, uintptr(unsafe.Pointer(&pid)))
			if pid == uint32(windows.GetCurrentProcessId()) {
				n.trayWindow = h
				old, _, _ := win("GetWindowLongPtrW").Call(h, ^uintptr(3))
				n.oldProc.Store(old)
				replaced, _, _ := win("SetWindowLongPtrW").Call(h, ^uintptr(3), trayHookProc)
				if replaced != 0 {
					n.hooked.Store(true)
				} else {
					n.trayWindow = 0
				}
			}
		}
	}
	if n.window != 0 {
		if n.lang != i18n.Language() {
			win("DestroyWindow").Call(n.window)
			n.window = 0
			n.ids = nil
			n.open()
		}
		n.refreshAdapters()
		n.refreshUsage()
	}
	if n.hoverVisible {
		var p point
		win("GetCursorPos").Call(uintptr(unsafe.Pointer(&p)))
		var r rect
		id := iconIdentifier{Window: n.trayWindow, ID: 100}
		id.Size = uint32(unsafe.Sizeof(id))
		result, _, _ := shell32.NewProc("Shell_NotifyIconGetRect").Call(uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(&r)))
		inside := false
		if int32(result) >= 0 {
			inside = p.X >= r.Left && p.X <= r.Right && p.Y >= r.Top && p.Y <= r.Bottom
		} else {
			inside = abs32(p.X-n.hoverPoint.X) < 22 && abs32(p.Y-n.hoverPoint.Y) < 22
		}
		var panel rect
		win("GetWindowRect").Call(n.hover, uintptr(unsafe.Pointer(&panel)))
		inside = inside || (p.X >= panel.Left && p.X <= panel.Right && p.Y >= panel.Top && p.Y <= panel.Bottom)
		if !inside {
			win("ShowWindow").Call(n.hover, 0)
			n.hoverVisible = false
		} else {
			n.updateHoverText()
		}
	}
}
func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
func (n *nativeWindows) updateHoverText() {
	n.mu.Lock()
	t, rtl := n.text, n.rtl
	n.mu.Unlock()
	text(n.hoverText, strings.ReplaceAll(t, "\n", "\r\n"))
	ex := uintptr(0)
	if rtl {
		ex = wsRTL | 0x2000
	}
	win("SetWindowLongPtrW").Call(n.hoverText, ^uintptr(19), ex)
}
func (n *nativeWindows) showHover() {
	if n.hover == 0 {
		n.hover = createWindow(0x08000088, "NetMergeControls", "net-merge", 0x80800000, 0, 0, 540, 250, 0, 0)
		n.hoverText = createWindow(0, "EDIT", "", wsChild|wsVisible|0x4|0x40|0x800|0x200000, 12, 12, 516, 226, n.hover, 0)
		font, _, _ := gdi32.NewProc("GetStockObject").Call(17)
		send(n.hoverText, 0x30, font, 1)
	}
	if n.hoverVisible {
		return
	}
	win("GetCursorPos").Call(uintptr(unsafe.Pointer(&n.hoverPoint)))
	var work rect
	win("SystemParametersInfoW").Call(0x30, 0, uintptr(unsafe.Pointer(&work)), 0)
	n.mu.Lock()
	lineCount := strings.Count(n.text, "\n") + 1
	n.mu.Unlock()
	width := int32(540)
	height := min(int32(lineCount*20+32), max(int32(150), (work.Bottom-work.Top)*3/4))
	x := min(max(work.Left, n.hoverPoint.X-width/2), work.Right-width)
	y := n.hoverPoint.Y - height - 16
	if y < work.Top {
		y = n.hoverPoint.Y + 24
	}
	win("MoveWindow").Call(n.hover, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 1)
	win("MoveWindow").Call(n.hoverText, 12, 12, uintptr(width-24), uintptr(height-24), 1)
	n.updateHoverText()
	win("SetWindowPos").Call(n.hover, ^uintptr(0), uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0x50)
	n.hoverVisible = true
}

// fyne/systray v1.12 uses WM_USER+1 for its icon callback and a Unicode
// window procedure. Subclass only our own process's window; forward every event.
func trayHook(h uintptr, message uint32, w, l uintptr) uintptr {
	if message == 0x117 {
		setMenuRTL(w, i18n.RTL())
	}
	if message == 0x401 {
		if l == 0x200 {
			win("PostMessageW").Call(native.hwnd.Load(), wmHover, 0, 0)
		} else if l == 0x201 || l == 0x204 || l == 0x202 || l == 0x205 {
			win("PostMessageW").Call(native.hwnd.Load(), wmHideHover, 0, 0)
		}
	}
	r, _, _ := win("CallWindowProcW").Call(native.oldProc.Load(), h, uintptr(message), w, l)
	return r
}

// MFT_RIGHTORDER makes submenus cascade in the Arabic reading direction. Existing
// checkbox, separator and disabled flags remain intact.
type menuItemInfo struct {
	Size, Mask, Type, State, ID       uint32
	Submenu, Checked, Unchecked, Data uintptr
	Text                              *uint16
	TextLength                        uint32
	Bitmap                            uintptr
}

func setMenuRTL(menu uintptr, rtl bool) {
	count, _, _ := win("GetMenuItemCount").Call(menu)
	if int32(count) < 0 {
		return
	}
	for j := uintptr(0); j < count; j++ {
		info := menuItemInfo{Mask: 0x100}
		info.Size = uint32(unsafe.Sizeof(info))
		if ok, _, _ := win("GetMenuItemInfoW").Call(menu, j, 1, uintptr(unsafe.Pointer(&info))); ok != 0 {
			info.Type &^= 0x2000
			if rtl {
				info.Type |= 0x2000
			}
			win("SetMenuItemInfoW").Call(menu, j, 1, uintptr(unsafe.Pointer(&info)))
		}
	}
}
