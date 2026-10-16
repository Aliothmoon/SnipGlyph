package hotkey

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey    = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey  = user32.NewProc("UnregisterHotKey")
	procGetMessageW       = user32.NewProc("GetMessageW")
	procPeekMessageW      = user32.NewProc("PeekMessageW")
	procPostThreadMessage = user32.NewProc("PostThreadMessageW")
)

const (
	modNoRepeat = 0x4000
	wmHotkey    = 0x0312
	wmWake      = 0x8000 + 1 // WM_APP+1: process pending requests
	wmStop      = 0x8000 + 2 // WM_APP+2: unregister everything and exit
	pmNoRemove  = 0
)

// ErrClosed is returned by Manager methods after Close.
var ErrClosed = errors.New("hotkey: manager closed")

// IsTaken reports whether err means another program already owns the hotkey.
func IsTaken(err error) bool {
	return errors.Is(err, windows.ERROR_HOTKEY_ALREADY_REGISTERED)
}

type msg struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       struct{ X, Y int32 }
	LPrivate uint32
}

type request struct {
	id       int
	hk       Hotkey
	register bool
	reply    chan error
}

// Manager registers system-wide hotkeys and reports presses on Events.
//
// Hotkeys registered without a window belong to the calling thread, so the
// Manager runs its own message loop on a dedicated, locked OS thread and
// funnels every RegisterHotKey/UnregisterHotKey call onto it.
type Manager struct {
	threadID uint32
	reqs     chan request
	events   chan int
	done     chan struct{}

	mu     sync.Mutex
	closed bool
}

// NewManager starts the hotkey thread.
func NewManager() (*Manager, error) {
	m := &Manager{
		reqs:   make(chan request, 16),
		events: make(chan int, 8),
		done:   make(chan struct{}),
	}
	ready := make(chan struct{})
	go m.loop(ready)
	<-ready
	return m, nil
}

// Events delivers the id of each pressed hotkey. It is closed by Close.
// Presses are dropped if the receiver falls behind.
func (m *Manager) Events() <-chan int { return m.events }

// Register binds hk to id, replacing any hotkey previously bound to id.
// Use IsTaken to detect conflicts with other programs.
func (m *Manager) Register(id int, hk Hotkey) error {
	return m.do(request{id: id, hk: hk, register: true})
}

// Unregister removes the hotkey bound to id, if any.
func (m *Manager) Unregister(id int) error {
	return m.do(request{id: id})
}

// Close unregisters all hotkeys and stops the thread.
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.mu.Unlock()
	procPostThreadMessage.Call(uintptr(m.threadID), wmStop, 0, 0)
	<-m.done
}

func (m *Manager) do(r request) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrClosed
	}
	r.reply = make(chan error, 1)
	m.reqs <- r
	// Enqueue before waking so the loop is guaranteed to see the request.
	ok, _, err := procPostThreadMessage.Call(uintptr(m.threadID), wmWake, 0, 0)
	m.mu.Unlock()
	if ok == 0 {
		return fmt.Errorf("hotkey: wake thread: %w", err)
	}
	select {
	case err := <-r.reply:
		return err
	case <-m.done: // loop died (GetMessage error) without serving us
		select {
		case err := <-r.reply:
			return err
		default:
			return ErrClosed
		}
	}
}

func (m *Manager) loop(ready chan<- struct{}) {
	runtime.LockOSThread()
	defer close(m.done)

	m.threadID = windows.GetCurrentThreadId()
	var mg msg
	// Force the thread's message queue into existence before anyone posts.
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&mg)), 0, 0, 0, pmNoRemove)
	close(ready)

	registered := map[int]bool{}
	defer func() {
		for id := range registered {
			procUnregisterHotKey.Call(0, uintptr(id))
		}
		close(m.events)
	}()

	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&mg)), 0, 0, 0)
		if int32(r) <= 0 { // WM_QUIT or error
			return
		}
		switch mg.Message {
		case wmHotkey:
			select {
			case m.events <- int(mg.WParam):
			default:
			}
		case wmWake:
			m.drain(registered)
		case wmStop:
			return
		}
	}
}

func (m *Manager) drain(registered map[int]bool) {
	for {
		select {
		case r := <-m.reqs:
			r.reply <- apply(r, registered)
		default:
			return
		}
	}
}

func apply(r request, registered map[int]bool) error {
	if registered[r.id] {
		procUnregisterHotKey.Call(0, uintptr(r.id))
		delete(registered, r.id)
	}
	if !r.register {
		return nil
	}
	ok, _, err := procRegisterHotKey.Call(0, uintptr(r.id), uintptr(r.hk.Mods)|modNoRepeat, uintptr(r.hk.Key))
	if ok == 0 {
		return fmt.Errorf("register %s: %w", r.hk, err)
	}
	registered[r.id] = true
	return nil
}
