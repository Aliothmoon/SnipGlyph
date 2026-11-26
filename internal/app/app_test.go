package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"SnipGlyph/internal/config"
	"SnipGlyph/internal/i18n"
	"SnipGlyph/internal/ocr"
)

func TestFormatOCRErrorBothLanguages(t *testing.T) {
	orig := i18n.Active()
	t.Cleanup(func() { i18n.Set(orig) })

	cases := []*ocr.Error{
		{Code: ocr.ErrCodeNotInstalled},
		{Code: ocr.ErrCodeCustomDirMissing, Arg: `C:\ocr`, Detail: "oneocr.dll"},
		{Code: ocr.ErrCodeStageFailed, Detail: "access denied"},
		{Code: ocr.ErrCodeLoadDLL, Arg: "oneocr.dll", Detail: "err"},
		{Code: ocr.ErrCodeBadArch, Arg: "oneocr.dll", Detail: "193"},
		{Code: ocr.ErrCodeMissingProc, Arg: "CreateOcrPipeline", Detail: "missing"},
		{Code: ocr.ErrCodeModelPathEncoding, Arg: `C:\非ASCII\oneocr.onemodel`},
		{Code: ocr.ErrCodeModelOpen, Arg: `C:\oneocr.onemodel`},
		{Code: ocr.ErrCodeModelKey},
		{Code: ocr.ErrCodePipelineInit, Arg: "RunOcrPipeline", Detail: "3"},
		{Code: ocr.ErrCodeSelfTest, Detail: "mismatch"},
		{Code: ocr.ErrCodeWorkerStart, Detail: "eof"},
		{Code: ocr.ErrCodeWorkerTimeout},
		{Code: ocr.ErrCodeWorkerIO, Detail: "broken pipe"},
	}

	for _, lang := range []i18n.Lang{i18n.ZhCN, i18n.En} {
		i18n.Set(lang)
		for _, tc := range cases {
			msg := formatOCRError(tc)
			if msg == "" {
				t.Errorf("lang=%s code=%s returned empty message", lang, tc.Code)
			}
			if strings.Contains(msg, "%!") {
				t.Errorf("lang=%s code=%s has format error: %q", lang, tc.Code, msg)
			}
		}
	}
}

func TestLoadConfigReturnsSecretUnreadableWithoutOverwriting(t *testing.T) {
	orig := i18n.Active()
	t.Cleanup(func() { i18n.Set(orig) })

	dir := t.TempDir()
	path := filepath.Join(dir, config.FileName)
	raw := `{"language":"en","translate":{"baidu":{"app_id":"a","key":"dpapi:AAAA"}}}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &App{opts: Options{ConfigPath: path}}
	err := a.loadConfig()
	if !errors.Is(err, config.ErrSecretUnreadable) {
		t.Fatalf("err = %v, want ErrSecretUnreadable", err)
	}
	if i18n.Active() != i18n.En {
		t.Errorf("active lang = %q, want %q", i18n.Active(), i18n.En)
	}
	// Disk file must remain untouched so the encrypted blob is not erased.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != raw {
		t.Fatalf("config file was modified on ErrSecretUnreadable: %s", after)
	}
}

func TestCheckHotkeysLocalized(t *testing.T) {
	orig := i18n.Active()
	t.Cleanup(func() { i18n.Set(orig) })

	h := config.Hotkeys{
		ScreenshotOCR: "Ctrl+Alt+A",
		SilentOCR:     "Ctrl+Alt+A",
	}
	i18n.Set(i18n.En)
	err := checkHotkeys(h)
	if err == nil || !strings.Contains(err.Error(), "same hotkey") {
		t.Fatalf("en dup err = %v", err)
	}
	i18n.Set(i18n.ZhCN)
	err = checkHotkeys(h)
	if err == nil || !strings.Contains(err.Error(), "同一个快捷键") {
		t.Fatalf("zh dup err = %v", err)
	}
}
