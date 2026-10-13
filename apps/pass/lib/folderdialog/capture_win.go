//go:build windows

package folderdialog

import (
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUser32  = windows.NewLazySystemDLL("user32.dll")
	modShell32 = windows.NewLazySystemDLL("shell32.dll")
	modKernel  = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW     = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW      = modUser32.NewProc("CreateWindowExW")
	procDestroyWindow        = modUser32.NewProc("DestroyWindow")
	procDefWindowProcW       = modUser32.NewProc("DefWindowProcW")
	procPeekMessageW         = modUser32.NewProc("PeekMessageW")
	procTranslateMessage     = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW     = modUser32.NewProc("DispatchMessageW")
	procPostQuitMessage      = modUser32.NewProc("PostQuitMessage")
	procSetLayeredWindowAttributes = modUser32.NewProc("SetLayeredWindowAttributes")
	procSetWindowPos         = modUser32.NewProc("SetWindowPos")
	procShowWindow           = modUser32.NewProc("ShowWindow")
	procUpdateWindow         = modUser32.NewProc("UpdateWindow")
	procGetCursorPos         = modUser32.NewProc("GetCursorPos")
	procGetModuleHandleW     = modKernel.NewProc("GetModuleHandleW")

	procDragAcceptFiles = modShell32.NewProc("DragAcceptFiles")
	procDragQueryFileW  = modShell32.NewProc("DragQueryFileW")
	procDragFinish      = modShell32.NewProc("DragFinish")

	dropClassAtom   uint16
	dropClassOnce   sync.Once
	dropWndProcPtr  = syscall.NewCallback(dropWndProc)
	dropMu          sync.Mutex
	dropCancelFlag  atomic.Bool
	dropResultPath  string
	dropGotResult   atomic.Bool
)

const (
	wmDropFiles   = 0x0233
	wmDestroy     = 0x0002
	wmQuit        = 0x0012
	pmRemove      = 0x0001
	wsExTopmost   = 0x00000008
	wsExToolwindow = 0x00000080
	wsExLayered   = 0x00080000
	wsPopup       = 0x80000000
	wsVisible     = 0x10000000
	swShow        = 5
	swpNoactivate = 0x0010
	swpShowwindow = 0x0040
	hwndTopmost   = ^uintptr(0) // HWND_TOPMOST = -1
	lwaAlpha      = 0x00000002
	csDblclks     = 0x0008
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type msg struct {
	Hwnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type point struct {
	X, Y int32
}

func ensureDropClass() {
	dropClassOnce.Do(func() {
		name, _ := windows.UTF16PtrFromString("NextPassDropTarget")
		hInst, _, _ := procGetModuleHandleW.Call(0)
		wc := wndClassEx{
			Size:      uint32(unsafe.Sizeof(wndClassEx{})),
			Style:     csDblclks,
			WndProc:   dropWndProcPtr,
			Instance:  windows.Handle(hInst),
			ClassName: name,
		}
		ret, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		dropClassAtom = uint16(ret)
	})
}

func dropWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmDropFiles:
		if p := readHDROP(wParam); p != "" {
			dropResultPath = p
			dropGotResult.Store(true)
		}
		procDragFinish.Call(wParam)
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func readHDROP(hDrop uintptr) string {
	n, _, _ := procDragQueryFileW.Call(hDrop, 0, 0, 0)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	procDragQueryFileW.Call(hDrop, 0, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return windows.UTF16ToString(buf)
}

// CaptureDrop 在鼠标/输入框位置创建置顶接收窗，瞬间承接资源管理器拖放并返回绝对路径（不上传文件）。
func CaptureDrop(timeout time.Duration, rect DropRect) (string, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	dropMu.Lock()
	defer dropMu.Unlock()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	dropCancelFlag.Store(false)
	dropGotResult.Store(false)
	dropResultPath = ""
	ClearCaptureDropFlags()
	ensureDropClass()

	x, y, w, h := rect.X, rect.Y, rect.W, rect.H
	if w < 120 {
		w = 280
	}
	if h < 36 {
		h = 52
	}
	if x == 0 && y == 0 {
		var pt point
		procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		x = int(pt.X) - w/2
		y = int(pt.Y) - h/2
	}

	className, _ := windows.UTF16PtrFromString("NextPassDropTarget")
	title, _ := windows.UTF16PtrFromString("")
	hInst, _, _ := procGetModuleHandleW.Call(0)

	hwnd, _, err := procCreateWindowExW.Call(
		uintptr(wsExTopmost|wsExToolwindow|wsExLayered),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(wsPopup|wsVisible),
		uintptr(int32(x)),
		uintptr(int32(y)),
		uintptr(int32(w)),
		uintptr(int32(h)),
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		if err != nil && err != windows.ERROR_SUCCESS {
			return "", err
		}
		return "", syscall.EINVAL
	}

	procSetLayeredWindowAttributes.Call(hwnd, 0, 70, lwaAlpha) // 轻微可见，便于对准
	procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(int32(x)), uintptr(int32(y)), uintptr(int32(w)), uintptr(int32(h)), swpNoactivate|swpShowwindow)
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)
	procDragAcceptFiles.Call(hwnd, 1)

	deadline := time.Now().Add(timeout)
	var m msg
	for {
		if dropCancelFlag.Load() || time.Now().After(deadline) {
			procDestroyWindow.Call(hwnd)
			// drain quit
			for {
				ret, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
				if ret == 0 || m.Message == wmQuit {
					break
				}
			}
			break
		}
		ret, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
		if ret != 0 {
			if m.Message == wmQuit {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
			continue
		}
		time.Sleep(8 * time.Millisecond)
	}

	if dropGotResult.Load() {
		return dropResultPath, nil
	}
	return "", nil
}

// CancelCaptureDrop 取消正在进行的拖放捕获。
func CancelCaptureDrop() {
	dropCancelFlag.Store(true)
	_ = osWriteCancelFlag()
}

// ClearCaptureDropFlags 开始捕获前清理标记。
func ClearCaptureDropFlags() {
	dropCancelFlag.Store(false)
	_ = osRemoveCancelFlag()
}

func osWriteCancelFlag() error {
	return windowsWriteCancel(true)
}

func osRemoveCancelFlag() error {
	return windowsWriteCancel(false)
}
