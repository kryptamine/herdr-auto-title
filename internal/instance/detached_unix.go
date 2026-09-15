//go:build !windows

package instance

import "syscall"

// detached puts the new instance in a session of its own, so that whatever
// ends the action's process group does not end the instance with it.
func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
