//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32Result                   = syscall.NewLazyDLL("user32.dll")
	enumWindowsResult              = user32Result.NewProc("EnumWindows")
	isWindowVisibleResult          = user32Result.NewProc("IsWindowVisible")
	getWindowTextWResult           = user32Result.NewProc("GetWindowTextW")
	getClassNameWResult            = user32Result.NewProc("GetClassNameW")
	getWindowThreadProcessIDResult = user32Result.NewProc("GetWindowThreadProcessId")
	attachThreadInputResult        = user32Result.NewProc("AttachThreadInput")
	setForegroundWindowResult      = user32Result.NewProc("SetForegroundWindow")
	bringWindowToTopResult         = user32Result.NewProc("BringWindowToTop")
	showWindowResult               = user32Result.NewProc("ShowWindow")
)

type resultWindow struct {
	hwnd  uintptr
	class string
	title string
}

// enumWindows returns top-level windows in Z order. The callback is only
// used synchronously (EnumWindows does not retain it).
func resultWindows() []resultWindow {
	var windows []resultWindow
	cb := syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
		visible, _, _ := isWindowVisibleResult.Call(hwnd)
		if visible == 0 {
			return 1
		}
		classBuf := make([]uint16, 256)
		titleBuf := make([]uint16, 512)
		nclass, _, _ := getClassNameWResult.Call(hwnd, uintptr(unsafe.Pointer(&classBuf[0])), uintptr(len(classBuf)))
		ntitle, _, _ := getWindowTextWResult.Call(hwnd, uintptr(unsafe.Pointer(&titleBuf[0])), uintptr(len(titleBuf)))
		if nclass == 0 || ntitle == 0 {
			return 1
		}
		windows = append(windows, resultWindow{hwnd: hwnd, class: syscall.UTF16ToString(classBuf), title: syscall.UTF16ToString(titleBuf)})
		return 1
	})
	enumWindowsResult.Call(cb, 0)
	return windows
}

func explorerWindow(w resultWindow) bool {
	return w.class == "CabinetWClass" || w.class == "ExploreWClass"
}

func pickResultWindow(before map[uintptr]bool, windows []resultWindow, path string, show bool) uintptr {
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	// For Show, only activate a *new* Explorer window. An existing Explorer
	// window cannot be identified by its Z-order or title alone; choosing one
	// arbitrarily could display an unrelated folder.
	for _, w := range windows {
		if before[w.hwnd] {
			continue
		}
		if show {
			if explorerWindow(w) {
				return w.hwnd
			}
		} else if !explorerWindow(w) && strings.Contains(strings.ToLower(w.title), base) {
			return w.hwnd
		}
	}
	return 0
}

func activateResultWindow(hwnd, originalForeground uintptr) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if hwnd == 0 {
		return
	}
	// Do not steal focus if the user switched away while Explorer was opening.
	current, _, _ := getForegroundWindow.Call()
	if current != originalForeground {
		return
	}

	fgThread, _, _ := getWindowThreadProcessIDResult.Call(current, 0)
	targetThread, _, _ := getWindowThreadProcessIDResult.Call(hwnd, 0)
	callerThread, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId").Call()
	if callerThread != 0 && fgThread != 0 && callerThread != fgThread {
		attached, _, _ := attachThreadInputResult.Call(callerThread, fgThread, 1)
		if attached != 0 {
			defer attachThreadInputResult.Call(callerThread, fgThread, 0)
		}
	}
	if callerThread != 0 && targetThread != 0 && callerThread != targetThread && targetThread != fgThread {
		attached, _, _ := attachThreadInputResult.Call(callerThread, targetThread, 1)
		if attached != 0 {
			defer attachThreadInputResult.Call(callerThread, targetThread, 0)
		}
	}
	showWindowResult.Call(hwnd, 9) // SW_RESTORE
	bringWindowToTopResult.Call(hwnd)
	setForegroundWindowResult.Call(hwnd)
}

// The browser's process is foreground, not the Go HTTP server. ShellExecute's
// owner hwnd is used for dialogs only; it does not activate Explorer or the ZIP
// handler. Find the resulting window and explicitly request activation.
func openWindowsResult(path string, show bool) error {
	foreground, _, _ := getForegroundWindow.Call()
	before := make(map[uintptr]bool)
	for _, w := range resultWindows() {
		before[w.hwnd] = true
	}

	verb, _ := syscall.UTF16PtrFromString("open")
	var target, parameters string
	if show {
		target = "explorer.exe"
		parameters = `/n,/select,"` + path + `"`
	} else {
		target = path
	}
	file, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	var argsPtr uintptr
	if parameters != "" {
		args, err := syscall.UTF16PtrFromString(parameters)
		if err != nil {
			return err
		}
		argsPtr = uintptr(unsafe.Pointer(args))
		// The call uses this pointer synchronously.
		result, _, _ := shellExecuteW.Call(foreground, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), argsPtr, 0, 1)
		if result <= 32 {
			return fmt.Errorf("ShellExecuteW failed: %d", result)
		}
	} else {
		result, _, _ := shellExecuteW.Call(foreground, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, 1)
		if result <= 32 {
			return fmt.Errorf("ShellExecuteW failed: %d", result)
		}
	}
	// Window creation is asynchronous. Never activate an unrelated Explorer
	// window when Windows reuses a previously open one.
	go func() {
		start := time.Now()
		deadline := start.Add(2 * time.Second)
		for time.Now().Before(deadline) {
			windows := resultWindows()
			hwnd := pickResultWindow(before, windows, path, show)
			if hwnd != 0 {
				activateResultWindow(hwnd, foreground)
				return
			}
			time.Sleep(80 * time.Millisecond)
		}
	}()
	return nil
}
