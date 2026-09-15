package instance

import "syscall"

// detachedProcess is the creation flag for a process without a console, which
// package syscall does not name.
const detachedProcess = 0x00000008

// detached starts the new instance without the action's console or process
// group, so that neither ending takes the instance with it.
func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}
}
