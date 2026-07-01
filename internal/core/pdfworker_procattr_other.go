//go:build !windows

package core

import "os/exec"

// hideChildConsole is a no-op off Windows (no console-window concept).
func hideChildConsole(cmd *exec.Cmd) {}
