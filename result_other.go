//go:build !windows

package main

import "fmt"

func openWindowsResult(path string, show bool) error {
	return fmt.Errorf("Windows shell is unavailable")
}
