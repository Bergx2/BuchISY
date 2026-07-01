//go:build windows

package core

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW: run the child without allocating a console, so spawning the
// highlight worker never flashes a console window (matters because a GUI-built
// app spawns one child per PDF preview).
const createNoWindow = 0x08000000

func hideChildConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
