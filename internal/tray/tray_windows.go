package tray

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"enylvia-downloader/internal/winreg"
)

var (
	shell32          = syscall.NewLazyDLL("shell32.dll")
	user32           = syscall.NewLazyDLL("user32.dll")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	shellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	extractIconExW   = shell32.NewProc("ExtractIconExW")

	createWindowExW   = user32.NewProc("CreateWindowExW")
	defWindowProcW    = user32.NewProc("DefWindowProcW")
	registerClassExW  = user32.NewProc("RegisterClassExW")
	destroyWindow     = user32.NewProc("DestroyWindow")
	postQuitMessage   = user32.NewProc("PostQuitMessage")
	getMessageW       = user32.NewProc("GetMessageW")
	translateMessage  = user32.NewProc("TranslateMessage")
	dispatchMessageW  = user32.NewProc("DispatchMessageW")
	createPopupMenu   = user32.NewProc("CreatePopupMenu")
	appendMenuW       = user32.NewProc("AppendMenuW")
	trackPopupMenu    = user32.NewProc("TrackPopupMenu")
	setForegroundWnd  = user32.NewProc("SetForegroundWindow")
	getCursorPos      = user32.NewProc("GetCursorPos")

	getModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	NIM_ADD        = 0x00000000
	NIM_MODIFY     = 0x00000001
	NIM_DELETE     = 0x00000002
	NIF_MESSAGE    = 0x00000001
	NIF_ICON       = 0x00000002
	NIF_TIP        = 0x00000004
	NIF_INFO       = 0x00000010
	WM_USER        = 0x0400
	WM_TRAY_MSG    = WM_USER + 101
	WM_LBUTTONUP   = 0x0202
	WM_LBUTTONDBL  = 0x0203
	WM_RBUTTONUP   = 0x0205
	MF_STRING      = 0x00000000
	MF_SEPARATOR   = 0x00000800
	MF_CHECKED     = 0x00000008
	MF_UNCHECKED   = 0x00000000
	TPM_BOTTOMALIGN = 0x0020
	TPM_RIGHTALIGN  = 0x0008

	ID_OPEN_WINDOW = 1001
	ID_RESUME_ALL  = 1002
	ID_PAUSE_ALL   = 1003
	ID_OPEN_FOLDER = 1004
	ID_AUTO_START  = 1005
	ID_EXIT        = 1006
)

type NOTIFYICONDATAW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeoutOrVersion uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type POINT struct {
	X int32
	Y int32
}

type MSG struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type TrayCallbacks struct {
	OnOpenWindow func()
	OnResumeAll  func()
	OnPauseAll   func()
	OnOpenFolder func()
	OnExit       func()
}

var globalCallbacks TrayCallbacks
var globalHWnd uintptr
var globalNID NOTIFYICONDATAW

func wndProc(hWnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_TRAY_MSG:
		switch lParam {
		case WM_LBUTTONUP, WM_LBUTTONDBL:
			if globalCallbacks.OnOpenWindow != nil {
				globalCallbacks.OnOpenWindow()
			}
		case WM_RBUTTONUP:
			showContextMenu(hWnd)
		}
		return 0
	case 0x0111: // WM_COMMAND
		cmdId := int(wParam & 0xFFFF)
		switch cmdId {
		case ID_OPEN_WINDOW:
			if globalCallbacks.OnOpenWindow != nil {
				globalCallbacks.OnOpenWindow()
			}
		case ID_RESUME_ALL:
			if globalCallbacks.OnResumeAll != nil {
				globalCallbacks.OnResumeAll()
			}
		case ID_PAUSE_ALL:
			if globalCallbacks.OnPauseAll != nil {
				globalCallbacks.OnPauseAll()
			}
		case ID_OPEN_FOLDER:
			if globalCallbacks.OnOpenFolder != nil {
				globalCallbacks.OnOpenFolder()
			}
		case ID_AUTO_START:
			ToggleAutoStart()
		case ID_EXIT:
			if globalCallbacks.OnExit != nil {
				globalCallbacks.OnExit()
			}
			postQuitMessage.Call(0)
		}
		return 0
	case 0x0002: // WM_DESTROY
		postQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := defWindowProcW.Call(hWnd, uintptr(msg), wParam, lParam)
	return ret
}

func showContextMenu(hWnd uintptr) {
	hMenu, _, _ := createPopupMenu.Call()
	if hMenu == 0 {
		return
	}

	appendMenu(hMenu, MF_STRING, ID_OPEN_WINDOW, "Buka EnylviaDownloader")
	appendMenu(hMenu, MF_SEPARATOR, 0, "")
	appendMenu(hMenu, MF_STRING, ID_RESUME_ALL, "▶ Lanjutkan Semua Unduhan")
	appendMenu(hMenu, MF_STRING, ID_PAUSE_ALL, "⏸ Jeda Semua Unduhan")
	appendMenu(hMenu, MF_STRING, ID_OPEN_FOLDER, "📁 Buka Folder Unduhan")
	appendMenu(hMenu, MF_SEPARATOR, 0, "")

	autoStartFlags := uint32(MF_STRING)
	if IsAutoStartEnabled() {
		autoStartFlags |= MF_CHECKED
	} else {
		autoStartFlags |= MF_UNCHECKED
	}
	appendMenu(hMenu, autoStartFlags, ID_AUTO_START, "Jalankan Otomatis saat Startup")

	appendMenu(hMenu, MF_SEPARATOR, 0, "")
	appendMenu(hMenu, MF_STRING, ID_EXIT, "✕ Keluar Sepenuhnya")

	var pt POINT
	getCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	setForegroundWnd.Call(hWnd)
	trackPopupMenu.Call(hMenu, uintptr(TPM_BOTTOMALIGN|TPM_RIGHTALIGN), uintptr(pt.X), uintptr(pt.Y), 0, hWnd, 0)
}

func appendMenu(hMenu uintptr, flags uint32, id uintptr, text string) {
	uText, _ := syscall.UTF16PtrFromString(text)
	appendMenuW.Call(hMenu, uintptr(flags), id, uintptr(unsafe.Pointer(uText)))
}

func StartTray(callbacks TrayCallbacks) error {
	globalCallbacks = callbacks

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		hInst, _, _ := getModuleHandleW.Call(0)
		className, _ := syscall.UTF16PtrFromString("EnylviaDownloaderTrayClass")

		wcex := WNDCLASSEXW{
			CbSize:      uint32(unsafe.Sizeof(WNDCLASSEXW{})),
			LpfnWndProc: syscall.NewCallback(wndProc),
			HInstance:   hInst,
			LpszClassName: className,
		}

		registerClassExW.Call(uintptr(unsafe.Pointer(&wcex)))

		windowName, _ := syscall.UTF16PtrFromString("EnylviaDownloaderTrayWindow")
		hWnd, _, _ := createWindowExW.Call(
			0,
			uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(windowName)),
			0,
			0, 0, 0, 0,
			0, 0, hInst, 0,
		)

		if hWnd == 0 {
			return
		}
		globalHWnd = hWnd

		// Extract default application icon or system shell icon
		exePath, _ := os.Executable()
		exePtr, _ := syscall.UTF16PtrFromString(exePath)
		var hIcon uintptr
		extractIconExW.Call(uintptr(unsafe.Pointer(exePtr)), 0, 0, uintptr(unsafe.Pointer(&hIcon)), 1)
		if hIcon == 0 {
			// Fallback icon from shell32
			shellPtr, _ := syscall.UTF16PtrFromString("shell32.dll")
			extractIconExW.Call(uintptr(unsafe.Pointer(shellPtr)), 18, 0, uintptr(unsafe.Pointer(&hIcon)), 1)
		}

		globalNID = NOTIFYICONDATAW{
			CbSize:           uint32(unsafe.Sizeof(NOTIFYICONDATAW{})),
			HWnd:             hWnd,
			UID:              1,
			UFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
			UCallbackMessage: WM_TRAY_MSG,
			HIcon:            hIcon,
		}

		copy(globalNID.SzTip[:], syscall.StringToUTF16("EnylviaDownloader - Mesin Aktif"))
		shellNotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&globalNID)))

		var msg MSG
		for {
			ret, _, _ := getMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if ret == 0 || int32(ret) == -1 {
				break
			}
			translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			dispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}

		shellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&globalNID)))
	}()

	return nil
}

func UpdateTrayTooltip(tipText string) {
	if globalHWnd == 0 {
		return
	}
	copy(globalNID.SzTip[:], syscall.StringToUTF16(tipText))
	shellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&globalNID)))
}

func ShowBalloonNotification(title, message string) {
	if globalHWnd == 0 {
		return
	}
	globalNID.UFlags |= NIF_INFO
	copy(globalNID.SzInfoTitle[:], syscall.StringToUTF16(title))
	copy(globalNID.SzInfo[:], syscall.StringToUTF16(message))
	globalNID.DwInfoFlags = 1 // NIIF_INFO
	shellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&globalNID)))
}

func StopTray() {
	if globalHWnd != 0 {
		shellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&globalNID)))
		destroyWindow.Call(globalHWnd)
		globalHWnd = 0
	}
}

// Windows Auto-Start Registry Utilities (Native Win32, Zero Window Glitch)
const runSubKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const appName = "EnylviaDownloader"

func IsAutoStartEnabled() bool {
	return winreg.ValueExists(runSubKey, appName)
}

func EnableAutoStart() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return err
	}

	val := fmt.Sprintf(`"%s" --minimized`, exePath)
	return winreg.SetStringValue(runSubKey, appName, val)
}

func DisableAutoStart() error {
	return winreg.DeleteValue(runSubKey, appName)
}

func ToggleAutoStart() bool {
	if IsAutoStartEnabled() {
		_ = DisableAutoStart()
		return false
	} else {
		_ = EnableAutoStart()
		return true
	}
}
