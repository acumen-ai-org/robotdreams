//go:build windows

package main

import "os/exec"

func startInOwnProcessGroup(*exec.Cmd) {}

func terminateProcessGroup(cmd *exec.Cmd) {
	_ = cmd.Process.Kill()
}

func killProcessGroup(cmd *exec.Cmd) {
	_ = cmd.Process.Kill()
}
