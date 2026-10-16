// Package logging opens the size-capped log file.
package logging

import (
	"os"
	"path/filepath"
)

// MaxSize is the size above which Open rotates the log before appending.
const MaxSize = 5 << 20

// Open opens dir/name for appending. If the file already exceeds MaxSize it
// is first renamed to name+".1", replacing any older rotation.
func Open(dir, name string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name)
	if fi, err := os.Stat(path); err == nil && fi.Size() > MaxSize {
		_ = os.Rename(path, path+".1")
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}
