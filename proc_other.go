//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func killProcessTree(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = syscall.Kill(pid, syscall.SIGTERM)
}

func openDefaultBrowser(url string) error { return exec.Command("xdg-open", url).Start() }
