package autostart

import (
	"os/exec" //nolint:depguard // only sets process attributes on the service manager command
	"syscall"
)

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
