package ocr

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as the worker: started with workerEnv set it
// serves OCR instead of running tests.
const workerEnv = "SNIPGLYPH_TEST_OCR_WORKER"

func TestMain(m *testing.M) {
	switch os.Getenv(workerEnv) {
	case "serve":
		if err := ServeWorker(os.Stdin, os.Stdout, testCacheDir); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "crash":
		os.Exit(3)
	}
	os.Exit(m.Run())
}

func workerCmd(mode string) func() *exec.Cmd {
	return func() *exec.Cmd {
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), workerEnv+"="+mode)
		return cmd
	}
}

func requireSnippingTool(t *testing.T) {
	t.Helper()
	if _, err := Locate(); errors.Is(err, ErrNotInstalled) {
		t.Skip("Snipping Tool not installed")
	}
}

func TestClientRecognizeAndIdleStop(t *testing.T) {
	requireSnippingTool(t)
	c := NewClient(workerCmd("serve"), 300*time.Millisecond)
	defer c.Close()

	img := loadPNG(t, "testdata/mixed.png")
	res, err := c.Recognize(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text(), "你好，世界！") {
		t.Fatalf("got %q", res.Text())
	}
	if c.Version() == "" {
		t.Error("Version empty after start")
	}

	// Idle timeout stops the worker; the next request starts a new one.
	time.Sleep(time.Second)
	c.mu.Lock()
	running := c.w != nil
	c.mu.Unlock()
	if running {
		t.Fatal("worker still running after idle timeout")
	}
	if _, err := c.Recognize(context.Background(), img); err != nil {
		t.Fatalf("after restart: %v", err)
	}
}

func TestClientReplacesKilledWorker(t *testing.T) {
	requireSnippingTool(t)
	c := NewClient(workerCmd("serve"), time.Minute)
	defer c.Close()
	img := loadPNG(t, "testdata/selftest.png")
	if _, err := c.Recognize(context.Background(), img); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	c.w.cmd.Process.Kill()
	<-c.w.exited
	c.mu.Unlock()
	if _, err := c.Recognize(context.Background(), img); err != nil {
		t.Fatalf("after worker death: %v", err)
	}
}

func TestClientWorkerThatDiesAtStartup(t *testing.T) {
	c := NewClient(workerCmd("crash"), time.Minute)
	defer c.Close()
	_, err := c.Recognize(context.Background(), Image{Width: 1, Height: 1, Pix: make([]byte, 4)})
	if err == nil {
		t.Fatal("want error from a worker that exits immediately")
	}
}

func TestClientRejectsBadImage(t *testing.T) {
	requireSnippingTool(t)
	c := NewClient(workerCmd("serve"), time.Minute)
	defer c.Close()
	// Header claims 10x10 but carries 4 bytes: the worker reports an error
	// and keeps serving.
	_, err := c.Recognize(context.Background(), Image{Width: 10, Height: 10, Pix: make([]byte, 4)})
	if err == nil {
		t.Fatal("want error for inconsistent image")
	}
	if _, err := c.Recognize(context.Background(), loadPNG(t, "testdata/selftest.png")); err != nil {
		t.Fatalf("worker unusable after bad request: %v", err)
	}
}
