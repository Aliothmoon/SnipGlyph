package ocr

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// startTimeout bounds loading the engine in a fresh worker; it normally
// takes well under a second.
const startTimeout = 30 * time.Second

// Client runs OCR in a worker process started on demand and stopped after
// it has been idle for a while, so the ~150 MB engine is only resident while
// in use. A crashed or misbehaving worker is replaced on the next request.
// Requests are served one at a time. Safe for concurrent use.
type Client struct {
	newCmd func() *exec.Cmd
	idle   time.Duration

	mu      sync.Mutex
	w       *worker // nil when not running
	gen     int     // bumps whenever w changes, to void stale idle timers
	timer   *time.Timer
	version string
}

type worker struct {
	cmd    *exec.Cmd
	stdin  *os.File
	stdout *bufio.Reader
	rawOut *os.File
	exited chan struct{}
}

// NewClient returns a client that starts workers with newCmd, a command
// whose process calls ServeWorker on its stdin/stdout.
func NewClient(newCmd func() *exec.Cmd, idle time.Duration) *Client {
	return &Client{newCmd: newCmd, idle: idle}
}

// Warm starts the worker in the background if it is not running, so that a
// following Recognize does not wait for the engine to load.
func (c *Client) Warm() {
	go func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if err := c.ensure(); err != nil {
			slog.Warn("ocr warm-up failed", "err", err)
			return
		}
		c.armTimer()
	}()
}

// Version is the Snipping Tool version the last worker loaded, or "".
func (c *Client) Version() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.version
}

// Recognize runs OCR on img. Cancelling ctx kills the worker.
func (c *Client) Recognize(ctx context.Context, img Image) (Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensure(); err != nil {
		return Result{}, err
	}
	defer c.armTimer()

	type reply struct {
		resp response
		err  error
	}
	w := c.w
	ch := make(chan reply, 1)
	go func() {
		var r reply
		if r.err = writeRequest(w.stdin, img); r.err == nil {
			r.err = readJSON(w.stdout, &r.resp)
		}
		ch <- r
	}()
	var r reply
	select {
	case r = <-ch:
	case <-ctx.Done():
		c.stop(false)
		<-ch
		return Result{}, ctx.Err()
	}
	if r.err != nil {
		c.stop(false)
		return Result{}, fmt.Errorf("OCR 进程通信失败：%w", r.err)
	}
	if r.resp.Error != "" {
		return Result{}, errors.New(r.resp.Error)
	}
	return r.resp.Result, nil
}

// Close stops the worker. The client may still be used afterwards.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stop(true)
}

// ensure starts a worker unless a live one exists. c.mu must be held.
func (c *Client) ensure() error {
	if c.w != nil {
		select {
		case <-c.w.exited:
			slog.Warn("ocr worker exited unexpectedly")
			c.stop(false)
		default:
			return nil
		}
	}
	w, h, err := c.start()
	if err != nil {
		return err
	}
	if h.Error != "" {
		c.w = w
		c.stop(true)
		return errors.New(h.Error)
	}
	c.w, c.version = w, h.Version
	c.gen++
	return nil
}

func (c *Client) start() (*worker, hello, error) {
	began := time.Now()
	cmd := c.newCmd()
	inR, inW, err := os.Pipe()
	if err != nil {
		return nil, hello{}, err
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		inR.Close()
		inW.Close()
		return nil, hello{}, err
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		inR.Close()
		inW.Close()
		outR.Close()
		outW.Close()
		return nil, hello{}, err
	}
	// *os.File ends mean exec starts no copying goroutines and Wait never
	// closes our ends of the pipes underneath a pending read.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inR, outW, errW
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	err = cmd.Start()
	inR.Close()
	outW.Close()
	errW.Close()
	if err != nil {
		inW.Close()
		outR.Close()
		errR.Close()
		return nil, hello{}, fmt.Errorf("启动 OCR 进程失败：%w", err)
	}
	w := &worker{cmd: cmd, stdin: inW, stdout: bufio.NewReader(outR), rawOut: outR, exited: make(chan struct{})}
	go forwardLog(errR, cmd.Process.Pid)
	go func() {
		_ = cmd.Wait()
		close(w.exited)
	}()

	type result struct {
		h   hello
		err error
	}
	ch := make(chan result, 1)
	go func() {
		var r result
		r.err = readJSON(w.stdout, &r.h)
		ch <- r
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			w.kill()
			return nil, hello{}, fmt.Errorf("OCR 进程启动失败：%w", r.err)
		}
		slog.Info("ocr worker ready", "pid", cmd.Process.Pid, "version", r.h.Version, "took", time.Since(began).Round(time.Millisecond), "err", r.h.Error)
		return w, r.h, nil
	case <-time.After(startTimeout):
		w.kill()
		<-ch
		return nil, hello{}, errors.New("OCR 进程启动超时")
	}
}

// stop shuts the worker down: politely (close stdin, wait briefly) or not.
// c.mu must be held.
func (c *Client) stop(graceful bool) {
	if c.w == nil {
		return
	}
	w := c.w
	c.w = nil
	c.gen++
	if c.timer != nil {
		c.timer.Stop()
	}
	if graceful {
		w.stdin.Close()
		select {
		case <-w.exited:
		case <-time.After(3 * time.Second):
		}
	}
	w.kill()
}

func (w *worker) kill() {
	w.stdin.Close()
	_ = w.cmd.Process.Kill()
	<-w.exited
	w.rawOut.Close()
}

// armTimer schedules an idle shutdown. c.mu must be held.
func (c *Client) armTimer() {
	if c.w == nil || c.idle <= 0 {
		return
	}
	if c.timer != nil {
		c.timer.Stop()
	}
	gen := c.gen
	c.timer = time.AfterFunc(c.idle, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.gen == gen && c.w != nil {
			slog.Info("ocr worker idle, stopping")
			c.stop(true)
		}
	})
}

// forwardLog copies the worker's stderr (its slog output and any crash
// trace) into our log.
func forwardLog(r io.ReadCloser, pid int) {
	defer r.Close()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		slog.Info("ocr-worker", "pid", pid, "line", sc.Text())
	}
}
