//go:build !windows

package main

import "github.com/ncruces/zenity"

func selectAudioFile() (string, error) {
	return zenity.SelectFile(zenity.Title("Select an audio or video file"))
}
