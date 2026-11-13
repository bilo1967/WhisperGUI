//go:build windows

package main

import (
	"syscall"

	"github.com/ncruces/zenity"
)

var getForegroundWindow = syscall.NewLazyDLL("user32.dll").NewProc("GetForegroundWindow")

// selectAudioFile attaches the native dialog to the window that had focus
// when the user pressed Select file (normally Chrome or Firefox).
func selectAudioFile() (string, error) {
	options := []zenity.Option{zenity.Title("Select an audio or video file")}
	hwnd, _, _ := getForegroundWindow.Call()
	if hwnd != 0 {
		options = append(options, zenity.Attach(uintptr(hwnd)), zenity.Modal())
	}
	return zenity.SelectFile(options...)
}
