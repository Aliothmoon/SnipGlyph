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
	"SnipGlyph/internal/config"
	"SnipGlyph/internal/history"
	"SnipGlyph/internal/instance"
	"SnipGlyph/internal/logging"
	"SnipGlyph/internal/ocr"
)

var version = "dev" // set with -ldflags "-X main.version=..."

// workerFlag makes the binary act as the OCR worker process. The worker
// is the same executable so that OneOCR, its ~150 MB and any crash in it stay
// out of the tray process.
const workerFlag = "--ocr-worker"

func main() {
	if len(os.Args) == 3 && os.Args[1] == workerFlag {
		os.Exit(runWorker(os.Args[2]))
	}
	os.Exit(run())
}

// runWorker serves OCR over stdin/stdout; logs go to stderr, which the
// parent forwards into its own log.
func runWorker(cacheDir string) int {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := ocr.ServeWorker(os.Stdin, os.Stdout, cacheDir); err != nil {
		slog.Error("ocr worker", "err", err)
		return 1
	}
	return 0
}

func run() int {
	release, err := instance.Acquire("SnipGlyph")
	if errors.Is(err, instance.ErrRunning) {
		messageBox("SnipGlyph 已经在运行了，可以在任务栏托盘里找到它。", windows.MB_ICONINFORMATION)
		return 0
	}
	if err != nil {
		return fatal("单实例检查失败", err)
	}
	defer release()

	local, err := appdir.Local()
	if err != nil {
		return fatal("无法创建数据目录", err)
	}
	roaming, err := appdir.Roaming()
	if err != nil {
		return fatal("无法创建配置目录", err)
	}
	logDir := filepath.Join(local, "logs")
	logFile, err := logging.Open(logDir, "snipglyph.log")
	if err != nil {
		return fatal("无法打开日志文件", err)
	}
	defer logFile.Close()
	// A GUI-subsystem binary has no visible stderr; send runtime crashes to the log.
	_ = debug.SetCrashOutput(logFile, debug.CrashOptions{})
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, nil)))
	slog.Info("starting", "version", version, "pid", os.Getpid())

	exe, err := os.Executable()
	if err != nil {
		return fatal("无法确定程序路径", err)
	}
	ocrCache := filepath.Join(local, "oneocr")
	err = app.Run(app.Options{
		ConfigPath:  filepath.Join(roaming, config.FileName),
		ConfigDir:   roaming,
		LogDir:      logDir,
		HistoryPath: filepath.Join(local, history.FileName),
		Exe:         exe,
		OCRWorker:   func() *exec.Cmd { return exec.Command(exe, workerFlag, ocrCache) },
	})
	if err != nil {
		return fatal("启动失败", err)
	}
	return 0
}

func fatal(what string, err error) int {
	slog.Error(what, "err", err)
	messageBox(fmt.Sprintf("%s：%v", what, err), windows.MB_ICONERROR)
	return 1
}

func messageBox(text string, icon uint32) {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString("SnipGlyph")
	windows.MessageBox(0, t, c, windows.MB_OK|icon)
}
