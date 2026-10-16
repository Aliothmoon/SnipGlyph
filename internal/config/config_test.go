package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), FileName))
	if err != nil {
		t.Fatal(err)
	}
	if c != Default() {
		t.Fatalf("got %+v, want defaults", c)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", FileName)
	want := Default()
	want.Hotkeys.InputTranslate = "Ctrl+Alt+Q"
	want.Hotkeys.SilentOCR = "" // disabled
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestLoadPartialFileKeepsOtherDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(`{"hotkeys":{"screenshot_ocr":"F1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Default()
	want.Hotkeys.ScreenshotOCR = "F1"
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestLoadBrokenFileReturnsDefaultsAndError(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(`{"hotkeys":`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err == nil {
		t.Fatal("want parse error")
	}
	if got != Default() {
		t.Fatalf("got %+v, want defaults", got)
	}
}
