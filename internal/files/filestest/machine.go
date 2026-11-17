// Package filestest answers a connected machine's file questions from this
// computer's files, as the helper does on Linux, with /proc supplied by the
// test. The machine names files the Linux way, so a test hands it Remote paths.
package filestest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// ErrDown is what a machine that cannot be reached answers.
var ErrDown = errors.New("lost the line to the machine")

// Proc is a process as /proc shows it.
type Proc struct {
	Argv []string
	Open []string
}

// Machine is a connected machine whose files are this computer's.
type Machine struct {
	mu    sync.Mutex
	procs map[int]Proc
	calls int
	down  bool
	stall bool
}

func New() *Machine {
	return &Machine{procs: make(map[int]Proc)}
}

// SetProc puts a process in /proc.
func (m *Machine) SetProc(pid int, proc Proc) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.procs[pid] = proc
}

// SetDown makes every batch fail until it is set back.
func (m *Machine) SetDown(down bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.down = down
}

// SetStalled makes every batch wait for its deadline.
func (m *Machine) SetStalled(stall bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stall = stall
}

// Calls counts the batches the machine was sent.
func (m *Machine) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.calls
}

// Exec answers one batch; argv is the helper, which is not run.
func (m *Machine) Exec(ctx context.Context, _ []string, input []byte) ([]byte, error) {
	m.mu.Lock()
	m.calls++
	down, stall := m.down, m.stall
	m.mu.Unlock()

	if stall {
		<-ctx.Done()

		return nil, ctx.Err()
	}

	if down {
		return nil, ErrDown
	}

	var questions []struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		From  int64  `json:"from"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &questions); err != nil {
		return nil, err
	}

	answers := make([]map[string]any, 0, len(questions))
	for _, q := range questions {
		answers = append(answers, m.answer(q.Op, q.Path, q.From, q.Limit))
	}

	return json.Marshal(answers)
}

func (m *Machine) answer(op, path string, from int64, limit int) map[string]any {
	if proc, rest, isProc := m.proc(path); isProc {
		switch {
		case op == "read" && rest == "cmdline":
			return map[string]any{"data": []byte(strings.Join(proc.Argv, "\x00") + "\x00")}
		case op == "links" && rest == "fd":
			return map[string]any{"names": proc.Open}
		}

		return map[string]any{"missing": true}
	}

	result, err := answerLocal(op, localOf(path), from, limit)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return map[string]any{"missing": true}
	case err != nil:
		return map[string]any{"failed": true}
	}

	return result
}

// Remote is a local path as the machine names it: unchanged on Unix, and
// C:\x as /C/x on Windows, whose paths a Linux machine would not take as absolute.
func Remote(local string) string {
	volume := filepath.VolumeName(local)
	rest := filepath.ToSlash(local[len(volume):])

	if volume == "" {
		return rest
	}

	return "/" + strings.TrimSuffix(volume, ":") + rest
}

func localOf(remote string) string {
	if runtime.GOOS != "windows" {
		return remote
	}

	volume, rest, _ := strings.Cut(strings.TrimPrefix(remote, "/"), "/")

	return volume + `:\` + filepath.FromSlash(rest)
}

func (m *Machine) proc(path string) (Proc, string, bool) {
	rest, isProc := strings.CutPrefix(path, "/proc/")
	if !isProc {
		return Proc{}, "", false
	}

	pid, rest, _ := strings.Cut(rest, "/")
	number, _ := strconv.Atoi(pid)

	m.mu.Lock()
	defer m.mu.Unlock()

	proc, known := m.procs[number]
	if !known {
		return Proc{}, "", true
	}

	return proc, rest, true
}

func answerLocal(op, path string, from int64, limit int) (map[string]any, error) {
	switch op {
	case "stat":
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"size": info.Size(), "mtime_ns": info.ModTime().UnixNano(),
			"dir": info.IsDir(), "regular": info.Mode().IsRegular(),
		}, nil
	case "read":
		file, err := os.Open(path) //nolint:gosec // a test's own temporary files
		if err != nil {
			return nil, err
		}

		defer func() { _ = file.Close() }()

		if _, err := file.Seek(from, io.SeekStart); err != nil {
			return nil, err
		}

		data, err := io.ReadAll(io.LimitReader(file, int64(limit)))

		return map[string]any{"data": data}, err
	case "glob":
		names, err := filepath.Glob(path)
		for i, name := range names {
			names[i] = Remote(name)
		}

		return map[string]any{"names": names}, err
	}

	return nil, errors.New("unknown question")
}
