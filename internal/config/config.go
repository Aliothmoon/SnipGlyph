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
	Hotkeys   Hotkeys   `json:"hotkeys"`
	Translate Translate `json:"translate"`
	History   History   `json:"history"`
}

type History struct {
	// Enabled records the text of OCR and translation results for the
	// history window.
	Enabled bool `json:"enabled"`
}

type Translate struct {
	// Engine is the default engine: "baidu". When empty or not configured,
	// the first configured engine is used.
	Engine string `json:"engine"`
	// TargetLang is what non-Chinese text is translated into; Chinese text
	// always goes to English.
	TargetLang string `json:"target_lang"`
	Baidu      Baidu  `json:"baidu"`
}

// Baidu holds credentials for the Baidu general translation API
// (https://fanyi-api.baidu.com).
type Baidu struct {
	AppID string `json:"app_id"`
	Key   Secret `json:"key"`
	// Proxy is "" to follow the Windows proxy, "direct", or a proxy URL
	// such as "http://127.0.0.1:7890" or "socks5://127.0.0.1:1080".
	Proxy string `json:"proxy"`
}

func (b Baidu) Configured() bool { return b.AppID != "" && b.Key != "" }

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
			ScreenshotOCR:       "Win+Alt+Shift+C",
			SilentOCR:           "Win+Alt+C",
			ScreenshotTranslate: "Win+Alt+S",
			SelectionTranslate:  "Win+Alt+X",
			InputTranslate:      "Win+Alt+I",
		},
		Translate: Translate{
			TargetLang: "zh-Hans",
		},
		History: History{Enabled: true},
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
