// Package files is where a reader finds the files it parses: this computer's
// filesystem, or a connected machine's as its last fetch described it, so one
// parser serves both.
package files

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ErrUnfetched is a file a connected machine has not been asked about yet. It
// says nothing of whether the file exists, so a reader must not act on it as
// on a file that is gone.
var ErrUnfetched = errors.New("not fetched from the machine yet")

// Info is what a stat says of a file.
type Info struct {
	Size    int64
	ModTime time.Time
	Dir     bool
	Regular bool
}

// FS is a filesystem in its own path syntax.
type FS interface {
	Stat(name string) (Info, error)
	// ReadAt returns at most limit bytes of name from offset from.
	ReadAt(name string, from int64, limit int) ([]byte, error)
	Glob(pattern string) ([]string, error)
	Join(elem ...string) string
	Dir(name string) string
	Base(name string) string
	IsAbs(name string) bool
}

// Local is this computer's filesystem.
var Local FS = local{}

type local struct{}

func (local) Stat(name string) (Info, error) {
	info, err := os.Stat(name)
	if err != nil {
		return Info{}, err
	}

	return Info{
		Size:    info.Size(),
		ModTime: info.ModTime(),
		Dir:     info.IsDir(),
		Regular: info.Mode().IsRegular(),
	}, nil
}

func (local) ReadAt(name string, from int64, limit int) ([]byte, error) {
	//nolint:gosec // each caller refuses a path it did not build or check itself
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	// A read-only file has nothing to report on close.
	defer func() { _ = file.Close() }()

	if _, err := file.Seek(from, io.SeekStart); err != nil {
		return nil, err
	}

	return io.ReadAll(io.LimitReader(file, int64(limit)))
}

func (local) Glob(pattern string) ([]string, error) { return filepath.Glob(pattern) }
func (local) Join(elem ...string) string            { return filepath.Join(elem...) }
func (local) Dir(name string) string                { return filepath.Dir(name) }
func (local) Base(name string) string               { return filepath.Base(name) }
func (local) IsAbs(name string) bool                { return filepath.IsAbs(name) }
