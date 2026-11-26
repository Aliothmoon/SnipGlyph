package ocr

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func touchOneOCR(t *testing.T, dir string, files ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		files = []string{dllName, ortName, modelName}
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("stub"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLocatePriority(t *testing.T) {
	t.Setenv(EnvDir, "")
	root := t.TempDir()
	exeDir := filepath.Join(root, "app")
	subDir := filepath.Join(exeDir, "oneocr")
	customDir := filepath.Join(root, "custom")

	// Empty exe dir without MSIX fallback -> ErrNotInstalled.
	if _, err := LocateWith(LocateOptions{ExeDir: exeDir, SkipSnippingTool: true}); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("empty dir: err=%v, want ErrNotInstalled", err)
	}

	// Files directly beside the executable are found.
	touchOneOCR(t, exeDir)
	inst, err := LocateWith(LocateOptions{ExeDir: exeDir, SkipSnippingTool: true})
	if err != nil {
		t.Fatal(err)
	}
	if inst.Source != SourceLocal || inst.Dir != exeDir || inst.NeedsStage {
		t.Fatalf("exeDir hit: got %+v", inst)
	}

	// <exeDir>\oneocr\ takes priority over <exeDir>\.
	touchOneOCR(t, subDir)
	inst, err = LocateWith(LocateOptions{ExeDir: exeDir, SkipSnippingTool: true})
	if err != nil {
		t.Fatal(err)
	}
	if inst.Source != SourceLocal || inst.Dir != subDir || inst.NeedsStage {
		t.Fatalf("subDir hit: got %+v", inst)
	}

	// CustomDir takes priority over local dirs.
	touchOneOCR(t, customDir)
	inst, err = LocateWith(LocateOptions{ExeDir: exeDir, CustomDir: customDir, SkipSnippingTool: true})
	if err != nil {
		t.Fatal(err)
	}
	if inst.Source != SourceCustom || inst.Dir != customDir || inst.NeedsStage {
		t.Fatalf("customDir hit: got %+v", inst)
	}

	// Relative CustomDir resolves against ExeDir.
	relDir := filepath.Join(exeDir, "vendor", "ocr")
	touchOneOCR(t, relDir)
	inst, err = LocateWith(LocateOptions{ExeDir: exeDir, CustomDir: filepath.Join("vendor", "ocr"), SkipSnippingTool: true})
	if err != nil {
		t.Fatal(err)
	}
	if inst.Source != SourceCustom || inst.Dir != relDir {
		t.Fatalf("relative customDir hit: got %+v", inst)
	}

	// EnvDir works when CustomDir is empty.
	t.Setenv(EnvDir, customDir)
	inst, err = LocateWith(LocateOptions{ExeDir: exeDir, SkipSnippingTool: true})
	if err != nil {
		t.Fatal(err)
	}
	if inst.Source != SourceCustom || inst.Dir != customDir {
		t.Fatalf("envDir hit: got %+v", inst)
	}
}

func TestLocateCustomDirMissingFilesReturnsError(t *testing.T) {
	t.Setenv(EnvDir, "")
	root := t.TempDir()
	customDir := filepath.Join(root, "partial")
	touchOneOCR(t, customDir, dllName) // missing onnxruntime.dll and oneocr.onemodel

	_, err := LocateWith(LocateOptions{ExeDir: root, CustomDir: customDir, SkipSnippingTool: true})
	var ocrErr *Error
	if !errors.As(err, &ocrErr) || ocrErr.Code != ErrCodeCustomDirMissing {
		t.Fatalf("err=%v, want ErrCodeCustomDirMissing", err)
	}
	if !strings.Contains(ocrErr.Detail, ortName) || !strings.Contains(ocrErr.Detail, modelName) {
		t.Errorf("missing files detail=%q", ocrErr.Detail)
	}
}

func TestLocatePartialLocalDirIgnored(t *testing.T) {
	t.Setenv(EnvDir, "")
	exeDir := t.TempDir()
	// Incomplete oneocr\ subfolder falls through to complete exeDir.
	touchOneOCR(t, filepath.Join(exeDir, "oneocr"), dllName)
	touchOneOCR(t, exeDir)

	inst, err := LocateWith(LocateOptions{ExeDir: exeDir, SkipSnippingTool: true})
	if err != nil {
		t.Fatal(err)
	}
	if inst.Dir != exeDir {
		t.Fatalf("got %q, want fallback to %q", inst.Dir, exeDir)
	}
}

func TestANSIPathASCII(t *testing.T) {
	p := filepath.Join(t.TempDir(), modelName)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ansiPath(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[len(got)-1] != 0 {
		t.Fatalf("not NUL-terminated: %v", got)
	}
}
