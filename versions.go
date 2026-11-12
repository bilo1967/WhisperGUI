package main

import (
	"context"
	"os/exec"
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
	cmd := exec.CommandContext(ctx, path, flag)
	prepareProcess(cmd)
	out, err := cmd.CombinedOutput()
	// Some builds print --version to stderr; CombinedOutput captures both.
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(prefix)) {
			if prefix == "whisper.cpp version:" {
				return strings.TrimSpace(strings.TrimPrefix(line, prefix))
			}
			return line
		}
	}
	if ctx.Err() != nil {
		return "unavailable (timeout)"
	}
	if err != nil {
		return "unavailable"
	}
	return "version not recognized"
}
