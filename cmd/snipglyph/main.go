// Command snipglyph is the SnipGlyph tray application.
package main

//go:generate go run github.com/tc-hib/go-winres@v0.3.3 make --in ../../winres/winres.json --out rsrc --arch amd64

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"

	"golang.org/x/sys/windows"

	"SnipGlyph/internal/app"
	"SnipGlyph/internal/appdir"
	"SnipGlyph/internal/cliphist"
	"SnipGlyph/internal/config"
	"SnipGlyph/internal/history"
	"SnipGlyph/internal/i18n"
	"SnipGlyph/internal/instance"
	"SnipGlyph/internal/logging"
	"SnipGlyph/internal/ocr"
)

var version = "dev" // set with -ldflags "-X main.version=..."

// workerFlag makes the binary act as the OCR worker process. The worker
// is the same executable so that OneOCR, its ~150 MB and any crash in it stay
// out of the tray process.
const (
	workerFlag   = "--ocr-worker"
	portableFlag = "--portable"
)

func main() {
	if len(os.Args) >= 3 && os.Args[1] == workerFlag {
		var customDir string
		if len(os.Args) >= 4 {
			customDir = os.Args[3]
		}
		os.Exit(runWorker(os.Args[2], customDir))
	}
	portable := false
	for _, arg := range os.Args[1:] {
		if arg == portableFlag || arg == "-portable" {
			portable = true
		}
	}
	os.Exit(run(portable))
}

// runWorker serves OCR over stdin/stdout; logs go to stderr, which the
// parent forwards into its own log.
func runWorker(cacheDir, customDir string) int {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := ocr.ServeWorkerWith(os.Stdin, os.Stdout, ocr.WorkerOptions{
		CacheDir:  cacheDir,
		CustomDir: customDir,
	}); err != nil {
		slog.Error("ocr worker", "err", err)
		return 1
	}
	return 0
}

func run(forcePortable bool) int {
	i18n.Set(i18n.Auto)
	tr := i18n.Current()

	release, err := instance.Acquire("SnipGlyph")
	if errors.Is(err, instance.ErrRunning) {
		messageBox(tr.AlreadyRunning, windows.MB_ICONINFORMATION)
		return 0
	}
	if err != nil {
		return fatal(tr.FatalSingleInstance, err)
	}
	defer release()

	exe, err := os.Executable()
	if err != nil {
		return fatal(tr.FatalExePath, err)
	}
	portable := appdir.IsPortable(exe, forcePortable)
	paths, err := appdir.Resolve(exe, portable)
	if err != nil {
		return fatal(tr.FatalDataDir, err)
	}
	logFile, err := logging.Open(paths.LogDir, "snipglyph.log")
	if err != nil {
		return fatal(tr.FatalLogFile, err)
	}
	defer logFile.Close()
	// A GUI-subsystem binary has no visible stderr; send runtime crashes to the log.
	_ = debug.SetCrashOutput(logFile, debug.CrashOptions{})
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, nil)))
	slog.Info("starting", "version", version, "pid", os.Getpid(),
		"portable", paths.Portable, "config_dir", paths.ConfigDir, "data_dir", paths.DataDir)

	var autostartArgs []string
	if forcePortable {
		autostartArgs = []string{portableFlag}
	}
	err = app.Run(app.Options{
		Portable:      paths.Portable,
		ConfigPath:    filepath.Join(paths.ConfigDir, config.FileName),
		ConfigDir:     paths.ConfigDir,
		LogDir:        paths.LogDir,
		HistoryPath:   filepath.Join(paths.DataDir, history.FileName),
		ClipboardPath: filepath.Join(paths.DataDir, cliphist.FileName),
		Exe:           exe,
		AutostartArgs: autostartArgs,
		OCRWorker: func(customDir string) *exec.Cmd {
			if customDir != "" {
				return exec.Command(exe, workerFlag, paths.OCRCacheDir, customDir)
			}
			return exec.Command(exe, workerFlag, paths.OCRCacheDir)
		},
	})
	if err != nil {
		return fatal(i18n.Current().FatalStart, err)
	}
	return 0
}

func fatal(what string, err error) int {
	slog.Error(what, "err", err)
	sep := ": "
	if i18n.Active() == i18n.ZhCN {
		sep = "："
	}
	messageBox(fmt.Sprintf("%s%s%v", what, sep, err), windows.MB_ICONERROR)
	return 1
}

func messageBox(text string, icon uint32) {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString("SnipGlyph")
	windows.MessageBox(0, t, c, windows.MB_OK|icon)
}
