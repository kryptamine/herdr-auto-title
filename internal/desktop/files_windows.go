package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

var (
	queryProcess = syscall.NewLazyDLL("ntdll.dll").NewProc("NtQueryInformationProcess")
	finalPath    = syscall.NewLazyDLL("kernel32.dll").NewProc("GetFinalPathNameByHandleW")
)

const (
	processDuplicateHandle   = 0x0040
	errInvalidHandle         = syscall.Errno(6)
	errNotSupported          = syscall.Errno(50)
	errInvalidParameter      = syscall.Errno(87)
	processHandleInformation = 51
	statusLengthMismatch     = 0xc0000004
	handleBufferLimit        = 16 << 20
	fileNameOpened           = 8
	pathBufferLimit          = 32768
)

// handleEntry keeps only the handle; the padding is the native ABI's counts,
// access mask, type index and attributes: docs/architecture/desktop-api.md.
type handleEntry struct {
	handle syscall.Handle
	_      [2]uintptr
	_      [4]uint32
}

func openFiles(ctx context.Context, pids []int) ([]string, error) {
	var paths []string

	for _, pid := range pids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		found, err := processFiles(ctx, pid)
		if err != nil {
			return nil, fmt.Errorf("open files for pid %d: %w", pid, err)
		}

		paths = append(paths, found...)
	}

	return paths, nil
}

func processFiles(ctx context.Context, pid int) ([]string, error) {
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("invalid pid %d", pid)
	}

	process, err := syscall.OpenProcess(
		syscall.PROCESS_QUERY_INFORMATION|processDuplicateHandle,
		false,
		uint32(pid),
	)
	if errors.Is(err, errInvalidParameter) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	defer func() { _ = syscall.CloseHandle(process) }()

	handles, err := processHandles(ctx, process)
	if err != nil {
		return nil, err
	}

	var paths []string

	for _, handle := range handles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		path, err := duplicatedPath(process, handle)
		if err != nil {
			return nil, err
		}

		if path != "" {
			paths = append(paths, path)
		}
	}

	return paths, nil
}

//nolint:gosec // native API pointers refer to live, bounded Go buffers
func processHandles(ctx context.Context, process syscall.Handle) ([]syscall.Handle, error) {
	if err := queryProcess.Find(); err != nil {
		return nil, err
	}

	for size := 4096; size <= handleBufferLimit; size *= 2 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		buffer := make([]byte, size)

		var written uint32

		status, _, _ := queryProcess.Call(
			uintptr(process),
			processHandleInformation,
			uintptr(
				unsafe.Pointer(&buffer[0]),
			),
			uintptr(len(buffer)),
			uintptr(unsafe.Pointer(&written)),
		)

		status &= 0xffffffff
		if status == statusLengthMismatch {
			continue
		}

		if status != 0 {
			return nil, fmt.Errorf("query process handles: NTSTATUS %#x", status)
		}

		if uint64(written) > uint64(len(buffer)) {
			return nil, errors.New("handle snapshot exceeds buffer")
		}

		return handlesIn(buffer[:written])
	}

	return nil, errors.New("process handle snapshot exceeds size limit")
}

func handlesIn(buffer []byte) ([]syscall.Handle, error) {
	const (
		header = 2 * unsafe.Sizeof(uintptr(0))
		stride = unsafe.Sizeof(handleEntry{})
	)

	if uintptr(len(buffer)) < header {
		return nil, errors.New("short handle snapshot")
	}

	//nolint:gosec // the header length was checked before reading its native count
	count := *(*uintptr)(unsafe.Pointer(&buffer[0]))
	if count > (uintptr(len(buffer))-header)/stride {
		return nil, errors.New("truncated handle snapshot")
	}

	handles := make([]syscall.Handle, 0, count)
	for i := range count {
		//nolint:gosec // count is bounded by the snapshot length above
		entry := (*handleEntry)(unsafe.Pointer(&buffer[header+i*stride]))
		handles = append(handles, entry.handle)
	}

	return handles, nil
}

func duplicatedPath(process, handle syscall.Handle) (string, error) {
	current, err := syscall.GetCurrentProcess()
	if err != nil {
		return "", fmt.Errorf("current process: %w", err)
	}

	var copied syscall.Handle

	err = syscall.DuplicateHandle(
		process,
		handle,
		current,
		&copied,
		0,
		false,
		syscall.DUPLICATE_SAME_ACCESS,
	)
	// Closed handles and object types that cannot be duplicated have no file path.
	if errors.Is(err, errInvalidHandle) || errors.Is(err, errNotSupported) {
		return "", nil
	}

	if err != nil {
		return "", fmt.Errorf("duplicate handle %#x: %w", handle, err)
	}

	defer func() { _ = syscall.CloseHandle(copied) }()

	// A pipe's name query can wait on its peer; only disk files and directories count.
	if kind, err := syscall.GetFileType(copied); err != nil || kind != syscall.FILE_TYPE_DISK {
		return "", nil
	}

	for size := uintptr(syscall.MAX_PATH); size <= pathBufferLimit; {
		buffer := make([]uint16, size)

		// shortcut: cancellation waits for disk calls; isolate queries if stalls are observed.
		//nolint:gosec // the UTF-16 buffer stays alive throughout the native call
		n, _, err := finalPath.Call(
			uintptr(copied),
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(len(buffer)),
			fileNameOpened,
		)
		if n == 0 {
			return "", fmt.Errorf("file name: %w", err)
		}

		if n < uintptr(len(buffer)) {
			return dosPath(syscall.UTF16ToString(buffer[:n])), nil
		}

		size = n
	}

	return "", errors.New("open file path exceeds size limit")
}

func dosPath(path string) string {
	if rest, unc := strings.CutPrefix(path, `\\?\UNC\`); unc {
		return `\\` + rest
	}

	return strings.TrimPrefix(path, `\\?\`)
}
