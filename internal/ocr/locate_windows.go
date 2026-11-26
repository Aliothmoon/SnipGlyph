package ocr

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	packageFamily = "Microsoft.ScreenSketch_8wekyb3d8bbwe"
	// EnvDir overrides the OneOCR directory when set.
	EnvDir = "SNIPGLYPH_ONEOCR_DIR"
)

var (
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procGetPackagesByFamily      = kernel32.NewProc("GetPackagesByPackageFamily")
	procGetPackagePathByFullName = kernel32.NewProc("GetPackagePathByFullName")
	procWideCharToMultiByte      = kernel32.NewProc("WideCharToMultiByte")
	procGetShortPathNameW        = kernel32.NewProc("GetShortPathNameW")
)

// Files OneOCR needs.
const (
	dllName   = "oneocr.dll"
	ortName   = "onnxruntime.dll"
	modelName = "oneocr.onemodel"
)

// Source describes where OneOCR files were located.
type Source string

const (
	SourceCustom       Source = "custom"
	SourceLocal        Source = "local"
	SourceSnippingTool Source = "snipping_tool"
)

// LocateOptions controls where LocateWith looks for OneOCR files.
type LocateOptions struct {
	// ExeDir is the directory holding the SnipGlyph executable. When empty,
	// os.Executable is queried.
	ExeDir string
	// CustomDir is an explicit user-configured directory (e.g. config.OCR.Dir).
	// Relative paths resolve against ExeDir. When empty, SNIPGLYPH_ONEOCR_DIR
	// is checked.
	CustomDir string
	// SkipSnippingTool disables the MSIX Snipping Tool fallback (for tests).
	SkipSnippingTool bool
}

// Install is a usable OneOCR file set.
type Install struct {
	Source     Source
	Version    string // MSIX version, e.g. "11.2608.41.0", or "local"/"custom"
	Dir        string // folder holding the DLLs and the model
	NeedsStage bool   // true for MSIX packages whose ACL blocks direct LoadLibrary
}

// Locate finds OneOCR files using default options: custom env dir, local
// folders beside the executable, and finally the installed Snipping Tool.
func Locate() (Install, error) {
	return LocateWith(LocateOptions{})
}

// LocateWith finds OneOCR files in priority order:
//  1. CustomDir (or SNIPGLYPH_ONEOCR_DIR) — returns an explicit error if files are missing
//  2. <ExeDir>\oneocr\
//  3. <ExeDir>\
//  4. Installed Windows Snipping Tool (MSIX)
func LocateWith(opts LocateOptions) (Install, error) {
	exeDir := opts.ExeDir
	if exeDir == "" {
		if exe, err := os.Executable(); err == nil {
			exeDir = filepath.Dir(exe)
		}
	}

	customDir := strings.TrimSpace(opts.CustomDir)
	if customDir == "" {
		customDir = strings.TrimSpace(os.Getenv(EnvDir))
	}
	if customDir != "" {
		if !filepath.IsAbs(customDir) && exeDir != "" {
			customDir = filepath.Join(exeDir, customDir)
		}
		if missing := missingFiles(customDir, dllName, ortName, modelName); len(missing) > 0 {
			return Install{}, &Error{
				Code:   ErrCodeCustomDirMissing,
				Arg:    customDir,
				Detail: strings.Join(missing, ", "),
			}
		}
		return Install{
			Source:     SourceCustom,
			Version:    "custom",
			Dir:        customDir,
			NeedsStage: false,
		}, nil
	}

	if exeDir != "" {
		for _, dir := range []string{filepath.Join(exeDir, "oneocr"), exeDir} {
			if len(missingFiles(dir, dllName, ortName, modelName)) == 0 {
				return Install{
					Source:     SourceLocal,
					Version:    "local",
					Dir:        dir,
					NeedsStage: false,
				}, nil
			}
		}
	}

	if opts.SkipSnippingTool {
		return Install{}, ErrNotInstalled
	}
	return locateSnippingTool()
}

func locateSnippingTool() (Install, error) {
	names, err := packageFullNames(packageFamily)
	if err != nil {
		return Install{}, err
	}
	var best Install
	for _, full := range names {
		// Name_Version_Arch_ResourceId_PublisherId
		parts := strings.Split(full, "_")
		if len(parts) < 3 || parts[2] != "x64" {
			continue
		}
		root, err := packagePath(full)
		if err != nil {
			continue
		}
		dir := filepath.Join(root, "SnippingTool")
		if !allExist(dir, dllName, ortName, modelName) {
			continue
		}
		if best.Dir == "" || versionLess(best.Version, parts[1]) {
			best = Install{
				Source:     SourceSnippingTool,
				Version:    parts[1],
				Dir:        dir,
				NeedsStage: true,
			}
		}
	}
	if best.Dir == "" {
		return Install{}, ErrNotInstalled
	}
	return best, nil
}

func packageFullNames(family string) ([]string, error) {
	if err := procGetPackagesByFamily.Find(); err != nil {
		return nil, ErrNotInstalled
	}
	fam, err := windows.UTF16PtrFromString(family)
	if err != nil {
		return nil, err
	}
	var count, bufLen uint32
	r, _, _ := procGetPackagesByFamily.Call(uintptr(unsafe.Pointer(fam)),
		uintptr(unsafe.Pointer(&count)), 0, uintptr(unsafe.Pointer(&bufLen)), 0)
	const appModelNoPackage = 15700
	if count == 0 || r == appModelNoPackage {
		return nil, ErrNotInstalled
	}
	if r != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) && r != 0 {
		return nil, fmt.Errorf("GetPackagesByPackageFamily: %w", windows.Errno(r))
	}
	ptrs := make([]*uint16, count)
	buf := make([]uint16, bufLen)
	r, _, _ = procGetPackagesByFamily.Call(uintptr(unsafe.Pointer(fam)),
		uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&ptrs[0])),
		uintptr(unsafe.Pointer(&bufLen)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 {
		return nil, fmt.Errorf("GetPackagesByPackageFamily: %w", windows.Errno(r))
	}
	names := make([]string, 0, count)
	for _, p := range ptrs[:count] {
		names = append(names, windows.UTF16PtrToString(p))
	}
	return names, nil
}

func packagePath(fullName string) (string, error) {
	n, err := windows.UTF16PtrFromString(fullName)
	if err != nil {
		return "", err
	}
	var size uint32
	procGetPackagePathByFullName.Call(uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(&size)), 0)
	if size == 0 {
		return "", fmt.Errorf("GetPackagePathByFullName(%s): no path", fullName)
	}
	buf := make([]uint16, size)
	r, _, _ := procGetPackagePathByFullName.Call(uintptr(unsafe.Pointer(n)),
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 {
		return "", fmt.Errorf("GetPackagePathByFullName(%s): %w", fullName, windows.Errno(r))
	}
	return windows.UTF16ToString(buf), nil
}

func allExist(dir string, names ...string) bool {
	return len(missingFiles(dir, names...)) == 0
}

func missingFiles(dir string, names ...string) []string {
	var missing []string
	for _, n := range names {
		fi, err := os.Stat(filepath.Join(dir, n))
		if err != nil || fi.IsDir() {
			missing = append(missing, n)
		}
	}
	return missing
}

// versionLess compares dotted numeric versions.
func versionLess(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x < y
		}
	}
	return false
}

// stageKey names the staging subfolder under cacheDir. MSIX installs use
// their package version; fallback staging of local files keys on file sizes
// and modification times so replacing a DLL invalidates the copy.
func stageKey(inst Install) string {
	if inst.Source == SourceSnippingTool && inst.Version != "" {
		return inst.Version
	}
	base := inst.Version
	if base == "" {
		base = "local"
	}
	var parts []string
	parts = append(parts, base)
	for _, n := range []string{dllName, ortName} {
		if fi, err := os.Stat(filepath.Join(inst.Dir, n)); err == nil {
			parts = append(parts, fmt.Sprintf("%d_%d", fi.Size(), fi.ModTime().Unix()))
		}
	}
	return strings.Join(parts, "-")
}

// stage copies the two DLLs into cacheDir\<key>\ and returns that folder.
// The MSIX package folder grants execute rights only to the Snipping Tool
// itself, so the DLLs cannot be loaded in place. The model is only read, so
// it stays. A copy is reused when the file sizes still match.
func stage(inst Install, cacheDir string) (string, error) {
	key := stageKey(inst)
	dst := filepath.Join(cacheDir, key)
	if sameSizes(inst.Dir, dst, dllName, ortName) {
		return dst, nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(cacheDir, key+".tmp-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp) // no-op after a successful rename
	for _, n := range []string{dllName, ortName} {
		if err := copyFile(filepath.Join(inst.Dir, n), filepath.Join(tmp, n)); err != nil {
			return "", err
		}
	}
	if err := os.RemoveAll(dst); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// pruneStaged deletes staged copies other than keep. Best effort: a copy
// still loaded by another process simply survives until next time.
func pruneStaged(cacheDir, keep string) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if p := filepath.Join(cacheDir, e.Name()); e.IsDir() && p != keep {
			os.RemoveAll(p)
		}
	}
}

func sameSizes(srcDir, dstDir string, names ...string) bool {
	for _, n := range names {
		a, err1 := os.Stat(filepath.Join(srcDir, n))
		b, err2 := os.Stat(filepath.Join(dstDir, n))
		if err1 != nil || err2 != nil || a.Size() != b.Size() {
			return false
		}
	}
	return true
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ansiPath converts path to the system ANSI code page, which is how
// CreateOcrPipeline interprets the model path. If path contains characters
// outside the active code page, it retries with the Windows 8.3 short path.
func ansiPath(path string) ([]byte, error) {
	buf, unrepresentable, err := wideToANSI(path)
	if err == nil {
		return buf, nil
	}
	if unrepresentable {
		if short, serr := shortPath(path); serr == nil && short != "" && short != path {
			if buf2, _, err2 := wideToANSI(short); err2 == nil {
				return buf2, nil
			}
		}
		return nil, &Error{Code: ErrCodeModelPathEncoding, Arg: path}
	}
	return nil, err
}

func wideToANSI(path string) (buf []byte, unrepresentable bool, err error) {
	w, err := windows.UTF16FromString(path)
	if err != nil {
		return nil, false, err
	}
	const cpACP, wcNoBestFitChars = 0, 0x400
	n, _, callErr := procWideCharToMultiByte.Call(cpACP, wcNoBestFitChars,
		uintptr(unsafe.Pointer(&w[0])), uintptr(len(w)), 0, 0, 0, 0)
	if n == 0 {
		return nil, false, fmt.Errorf("WideCharToMultiByte: %w", callErr)
	}
	buf = make([]byte, n)
	var usedDefault int32
	n, _, callErr = procWideCharToMultiByte.Call(cpACP, wcNoBestFitChars,
		uintptr(unsafe.Pointer(&w[0])), uintptr(len(w)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, uintptr(unsafe.Pointer(&usedDefault)))
	if n == 0 {
		return nil, false, fmt.Errorf("WideCharToMultiByte: %w", callErr)
	}
	if usedDefault != 0 {
		return nil, true, errors.New("not representable in ANSI code page")
	}
	return buf, false, nil // NUL-terminated: w included the terminator
}

func shortPath(path string) (string, error) {
	w, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	n, _, callErr := procGetShortPathNameW.Call(uintptr(unsafe.Pointer(w)), 0, 0)
	if n == 0 {
		return "", callErr
	}
	buf := make([]uint16, n)
	r, _, callErr := procGetShortPathNameW.Call(uintptr(unsafe.Pointer(w)), uintptr(unsafe.Pointer(&buf[0])), n)
	if r == 0 || r >= n {
		return "", callErr
	}
	return windows.UTF16ToString(buf[:r]), nil
}
