//go:build !windows

package main

import "os/exec"

func prepareProcess(cmd *exec.Cmd) {}
