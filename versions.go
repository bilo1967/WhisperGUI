package main

import (
	"context"
	"strings"
	"time"
)

type toolVersionInfo struct {
	Whisper string
	FFmpeg  string
}

func detectToolVersions(p Paths) toolVersionInfo {
	return toolVersionInfo{
		Whisper: readToolVersion(p.Whisper, "--version", "whisper.cpp version:"),
		FFmpeg:  readToolVersion(p.FFmpeg, "-version", "ffmpeg version"),
	}
}

func readToolVersion(path, flag, prefix string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use the same process-launching path as transcription, which also
	// sets the runtime directory and hides subprocess consoles on Windows.
	version := ""
	err := runProcess(ctx, path, []string{flag}, func(line string) {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(line), strings.ToLower(prefix)) {
			return
		}
		if prefix == "whisper.cpp version:" {
			version = strings.TrimSpace(line[len(prefix):])
		} else {
			version = line
		}
	})
	if version != "" {
		return version
	}
	if ctx.Err() != nil {
		return "unavailable (timeout)"
	}
	if err != nil {
		return "unavailable"
	}
	return "version not recognized"
}
