//go:build !windows

package desktop

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec" //nolint:depguard // ps and lsof, given numbers and no shell
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// processTable maps each process to its children.
func processTable(ctx context.Context) (map[int][]int, error) {
	//nolint:forbidigo // no terminal-derived value and no shell
	out, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}

	children := make(map[int][]int)

	for line := range strings.Lines(string(out)) {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}

		pid, pidErr := strconv.Atoi(fields[0])
		parent, parentErr := strconv.Atoi(fields[1])

		if pidErr == nil && parentErr == nil {
			children[parent] = append(children[parent], pid)
		}
	}

	return children, nil
}

// openFiles lists every path the processes hold open. Linux has no need of an
// lsof that may not be installed.
func openFiles(ctx context.Context, pids []int) ([]string, error) {
	if runtime.GOOS == "linux" {
		return procFiles(pids), nil
	}

	list := make([]string, 0, len(pids))
	for _, pid := range pids {
		list = append(list, strconv.Itoa(pid))
	}

	//nolint:forbidigo,gosec // nothing but numbers reaches it, and no shell
	out, err := exec.CommandContext(ctx, "lsof", "-nP", "-Fn", "-p", strings.Join(list, ",")).
		Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("lsof: %w", ctx.Err())
	}

	// lsof exits 1 when any one pid has gone, and still lists the others.
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("lsof: %w", err)
	}

	var paths []string

	for line := range bytes.Lines(out) {
		if name, isName := bytes.CutPrefix(bytes.TrimRight(line, "\n"), []byte("n")); isName {
			paths = append(paths, string(name))
		}
	}

	return paths, nil
}

// procFiles reads /proc, where a process that has gone simply lists nothing.
func procFiles(pids []int) []string {
	var paths []string

	for _, pid := range pids {
		dir := filepath.Join("/proc", strconv.Itoa(pid), "fd")

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if path, err := os.Readlink(filepath.Join(dir, entry.Name())); err == nil {
				paths = append(paths, path)
			}
		}
	}

	return paths
}
