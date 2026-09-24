package instance

import "syscall"

// processQueryLimitedInformation is the least a handle needs to be waited on,
// which package syscall does not name.
const processQueryLimitedInformation = 0x1000

// alive reports whether a process with this pid is still running. A process
// that has exited can still be opened while something holds a handle on it,
// so it is the wait that tells, with no time to wait.
func alive(pid int) bool {
	handle, err := syscall.OpenProcess(
		processQueryLimitedInformation|syscall.SYNCHRONIZE,
		false,
		uint32(pid), //nolint:gosec // a pid is positive and fits
	)
	if err != nil {
		return false
	}
	defer func() { _ = syscall.CloseHandle(handle) }()

	event, err := syscall.WaitForSingleObject(handle, 0)

	return err == nil && event == uint32(syscall.WAIT_TIMEOUT)
}
