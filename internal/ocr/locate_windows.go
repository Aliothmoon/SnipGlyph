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

const packageFamily = "Microsoft.ScreenSketch_8wekyb3d8bbwe"

var (
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procGetPackagesByFamily      = kernel32.NewProc("GetPackagesByPackageFamily")
	procGetPackagePathByFullName = kernel32.NewProc("GetPackagePathByFullName")
	procWideCharToMultiByte      = kernel32.NewProc("WideCharToMultiByte")
)

// Files OneOCR needs, all in the Snipping Tool's SnippingTool folder.
const (
	dllName   = "oneocr.dll"
	ortName   = "onnxruntime.dll"
	modelName = "oneocr.onemodel"
)

// ErrNotInstalled means no usable Snipping Tool was found.
var ErrNotInstalled = errors.New("找不到 Windows 截图工具，请从 Microsoft Store 安装或更新「截图工具」")

// Install is a Snipping Tool installation that contains OneOCR.
type Install struct {
	Version string // package version, e.g. "11.2608.41.0"
	Dir     string // folder holding the DLLs and the model
}

// Locate finds the newest installed Snipping Tool that ships OneOCR. It only
// queries the package manager and stats files, so it is cheap.
func Locate() (Install, error) {
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
			best = Install{Version: parts[1], Dir: dir}
		}
	}
	if best.Dir == "" {
		return Install{}, ErrNotInstalled
	}
	return best, nil
}

func packageFullNames(family string) ([]string, error) {
	fam, err := windows.UTF16PtrFromString(family)
	if err != nil {
		return nil, err
	}
	var count, bufLen uint32
	r, _, _ := procGetPackagesByFamily.Call(uintptr(unsafe.Pointer(fam)),
		uintptr(unsafe.Pointer(&count)), 0, uintptr(unsafe.Pointer(&bufLen)), 0)
	if count == 0 {
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
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			return false
		}
	}
	return true
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

// stage copies the two DLLs into cacheDir\<version>\ and returns that folder.
// The package folder grants execute rights only to the Snipping Tool itself,
// so the DLLs cannot be loaded in place. The model is only read, so it stays.
// A copy is reused when the file sizes still match.
func stage(inst Install, cacheDir string) (string, error) {
	dst := filepath.Join(cacheDir, inst.Version)
	if sameSizes(inst.Dir, dst, dllName, ortName) {
		return dst, nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(cacheDir, inst.Version+".tmp-")
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
// CreateOcrPipeline interprets the model path.
func ansiPath(path string) ([]byte, error) {
	w, err := windows.UTF16FromString(path)
	if err != nil {
		return nil, err
	}
	const cpACP, wcNoBestFitChars = 0, 0x400
	n, _, err := procWideCharToMultiByte.Call(cpACP, wcNoBestFitChars,
		uintptr(unsafe.Pointer(&w[0])), uintptr(len(w)), 0, 0, 0, 0)
	if n == 0 {
		return nil, fmt.Errorf("WideCharToMultiByte: %w", err)
	}
	buf := make([]byte, n)
	var usedDefault int32
	n, _, err = procWideCharToMultiByte.Call(cpACP, wcNoBestFitChars,
		uintptr(unsafe.Pointer(&w[0])), uintptr(len(w)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, uintptr(unsafe.Pointer(&usedDefault)))
	if n == 0 {
		return nil, fmt.Errorf("WideCharToMultiByte: %w", err)
	}
	if usedDefault != 0 {
		return nil, fmt.Errorf("path %q is not representable in the ANSI code page", path)
	}
	return buf, nil // NUL-terminated: w included the terminator
}
