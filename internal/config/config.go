// Package config loads and saves the user's settings file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FileName is the settings file inside the roaming app directory.
const FileName = "config.json"

type Config struct {
	Hotkeys Hotkeys `json:"hotkeys"`
}

// Hotkeys holds one hotkey string per action, in the format accepted by
// hotkey.Parse. An empty string disables the action's hotkey.
type Hotkeys struct {
	ScreenshotOCR       string `json:"screenshot_ocr"`
	SilentOCR           string `json:"silent_ocr"`
	ScreenshotTranslate string `json:"screenshot_translate"`
	SelectionTranslate  string `json:"selection_translate"`
	InputTranslate      string `json:"input_translate"`
}

func Default() Config {
	return Config{
		Hotkeys: Hotkeys{
			ScreenshotOCR:       "Win+Alt+C",
			SilentOCR:           "Win+Alt+Shift+C",
			ScreenshotTranslate: "Win+Alt+S",
			SelectionTranslate:  "Win+Alt+X",
			InputTranslate:      "Win+Alt+I",
		},
	}
}

// Load reads path on top of the defaults, so settings missing from the file
// keep their default values. A missing file yields Default() and no error.
func Load(path string) (Config, error) {
	c := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return Default(), fmt.Errorf("parse %s: %w", path, err)
	}
	return c, nil
}

// Save writes c to path atomically, creating the directory if needed.
func Save(path string, c Config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), FileName+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
