//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var kernel32Mutex = syscall.NewLazyDLL("kernel32.dll")
var createMutexW = kernel32Mutex.NewProc("CreateMutexW")
var closeHandle = kernel32Mutex.NewProc("CloseHandle")

func acquireSingleInstance() (func(), error) {
	name, _ := syscall.UTF16PtrFromString("Local\\WhisperGUI-SingleInstance")
	handle, _, err := createMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return nil, fmt.Errorf("CreateMutexW: %w", err)
	}
	if err == syscall.Errno(183) {
		closeHandle.Call(handle)
		return nil, fmt.Errorf("another instance is running")
	}
	return func() { closeHandle.Call(handle) }, nil
}
