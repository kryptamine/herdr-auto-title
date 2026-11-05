package desktop

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

func processTable(ctx context.Context) (map[int][]int, error) {
	handle, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("process snapshot: %w", err)
	}
	defer func() { _ = syscall.CloseHandle(handle) }()

	entry := syscall.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	children := make(map[int][]int)

	for err = syscall.Process32First(handle, &entry); err == nil; err = syscall.Process32Next(handle, &entry) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if entry.ProcessID != entry.ParentProcessID {
			parent := int(entry.ParentProcessID)
			children[parent] = append(children[parent], int(entry.ProcessID))
		}
	}

	if !errors.Is(err, syscall.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("enumerate processes: %w", err)
	}

	return children, nil
}

func openFiles(context.Context, []int) ([]string, error) {
	// shortcut: Windows has no open-file reader; add one when session ownership is verified.
	return nil, errors.New("open-file session discovery is unavailable on Windows")
}
