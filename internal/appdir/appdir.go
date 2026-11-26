// Package appdir locates SnipGlyph's configuration and data directories,
// either under %APPDATA% / %LOCALAPPDATA% or beside the executable in
// portable mode.
package appdir

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	name = "SnipGlyph"
	// PortableEnv enables portable mode when set to a truthy value.
	PortableEnv = "SNIPGLYPH_PORTABLE"
)

// markerFiles next to the executable enable portable mode.
var markerFiles = []string{"portable", "portable.txt"}

// Paths holds the directories SnipGlyph reads and writes.
type Paths struct {
	Portable    bool
	ConfigDir   string // config.json
	DataDir     string // history.db, clipboard.db
	LogDir      string // logs
	OCRCacheDir string // staged OneOCR DLLs when loaded from MSIX
}

// IsPortable reports whether portable mode should be active: true if flagSet
// is true, SNIPGLYPH_PORTABLE is truthy, or a marker file ("portable" or
// "portable.txt") exists beside exe.
func IsPortable(exe string, flagSet bool) bool {
	if flagSet {
		return true
	}
	if v := strings.TrimSpace(os.Getenv(PortableEnv)); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	if exe == "" {
		return false
	}
	dir := filepath.Dir(exe)
	for _, f := range markerFiles {
		if fi, err := os.Stat(filepath.Join(dir, f)); err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

// Resolve returns the directories for the chosen mode and ensures the config
// and data directories exist.
//
// In portable mode everything lives under <exeDir>\data so the root folder
// stays tidy and separate from a user-supplied <exeDir>\oneocr\ folder.
func Resolve(exe string, portable bool) (Paths, error) {
	if portable {
		data := filepath.Join(filepath.Dir(exe), "data")
		if _, err := ensure(data); err != nil {
			return Paths{}, err
		}
		return Paths{
			Portable:    true,
			ConfigDir:   data,
			DataDir:     data,
			LogDir:      filepath.Join(data, "logs"),
			OCRCacheDir: filepath.Join(data, "cache", "oneocr"),
		}, nil
	}
	roaming, err := Roaming()
	if err != nil {
		return Paths{}, err
	}
	local, err := Local()
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		Portable:    false,
		ConfigDir:   roaming,
		DataDir:     local,
		LogDir:      filepath.Join(local, "logs"),
		OCRCacheDir: filepath.Join(local, "oneocr"),
	}, nil
}

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
