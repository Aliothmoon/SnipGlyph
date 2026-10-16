package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestSecretIsEncryptedOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	c := Default()
	c.Translate.Baidu = Baidu{AppID: "app", Key: "s3cret-value"}
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "s3cret-value") || !strings.Contains(string(data), `"key": "dpapi:`) {
		t.Fatalf("secret not encrypted on disk:\n%s", data)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Translate.Baidu.Key != "s3cret-value" {
		t.Fatalf("decrypted %q", got.Translate.Baidu.Key)
	}
}

func TestSecretPlainValueIsAccepted(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(`{"translate":{"baidu":{"app_id":"a","key":"typed-by-hand"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Translate.Baidu.Key != "typed-by-hand" || !got.Translate.Baidu.Configured() {
		t.Fatalf("got %+v", got.Translate.Baidu)
	}
	if got.Translate.TargetLang != "zh-Hans" {
		t.Errorf("default target lost: %q", got.Translate.TargetLang)
	}
}

func TestSecretUndecryptableBecomesEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(`{"hotkeys":{"screenshot_ocr":"F2"},"translate":{"baidu":{"app_id":"a","key":"dpapi:AAAA"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("an undecryptable secret must not fail the whole file: %v", err)
	}
	if got.Translate.Baidu.Key != "" || got.Hotkeys.ScreenshotOCR != "F2" {
		t.Fatalf("got %+v", got)
	}
}
