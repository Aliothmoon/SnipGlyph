// Package appdir locates SnipGlyph's per-user directories.
package appdir

import (
	"os"
	"path/filepath"
)

const name = "SnipGlyph"

// Roaming returns %APPDATA%\SnipGlyph (settings), creating it if needed.
func Roaming() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return ensure(filepath.Join(base, name))
}

// Local returns %LOCALAPPDATA%\SnipGlyph (logs, caches, history), creating it
// if needed.
func Local() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return ensure(filepath.Join(base, name))
}

func ensure(dir string) (string, error) {
	return dir, os.MkdirAll(dir, 0o755)
}
