//go:build windows

package main

import "syscall"

func windowsAdmin() bool {
	dll := syscall.NewLazyDLL("shell32.dll")
	proc := dll.NewProc("IsUserAnAdmin")
	result, _, _ := proc.Call()
	return result != 0
}
