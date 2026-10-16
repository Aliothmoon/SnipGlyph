// Package ui holds SnipGlyph's walk-based windows and the tray icon.
package ui

import (
	"log/slog"
	"strings"
	"sync"

	"github.com/tailscale/walk"
)

// appIcon is the icon embedded by go-winres, loaded once and shared.
var appIcon = sync.OnceValue(func() *walk.Icon {
	icon, err := walk.NewIconFromResource("APP")
	if err != nil {
		// walk errors carry a stack trace; the first line is the message.
		msg, _, _ := strings.Cut(err.Error(), "\n")
		slog.Warn("app icon resource missing, using stock icon", "err", msg)
		return walk.IconApplication()
	}
	return icon
})

// MenuItem is one tray context-menu entry. A zero Text makes a separator.
type MenuItem struct {
	Text     string
	Shortcut string // shown right-aligned; display only
	Disabled bool
	OnClick  func() // runs on the UI thread
}

// Tray is the notification-area icon with its context menu.
type Tray struct {
	ni *walk.NotifyIcon
}

// NewTray shows the tray icon. It must be called on the UI thread after
// walk.InitApp.
func NewTray(tooltip string, items []MenuItem) (*Tray, error) {
	ni, err := walk.NewNotifyIcon()
	if err != nil {
		return nil, err
	}
	t := &Tray{ni: ni}

	if err := ni.SetIcon(appIcon()); err != nil {
		ni.Dispose()
		return nil, err
	}
	if err := ni.SetToolTip(tooltip); err != nil {
		ni.Dispose()
		return nil, err
	}
	if err := t.SetMenu(items); err != nil {
		ni.Dispose()
		return nil, err
	}
	ni.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		if button == walk.LeftButton {
			ni.ShowContextMenu(x, y)
		}
	})
	if err := ni.SetVisible(true); err != nil {
		ni.Dispose()
		return nil, err
	}
	return t, nil
}

// SetMenu replaces the context menu. UI thread only.
func (t *Tray) SetMenu(items []MenuItem) error {
	actions := t.ni.ContextMenu().Actions()
	if err := actions.Clear(); err != nil {
		return err
	}
	for _, it := range items {
		var a *walk.Action
		if it.Text == "" {
			a = walk.NewSeparatorAction()
		} else {
			a = walk.NewAction()
			text := it.Text
			if it.Shortcut != "" {
				text += "\t" + it.Shortcut
			}
			if err := a.SetText(text); err != nil {
				return err
			}
			if err := a.SetEnabled(!it.Disabled); err != nil {
				return err
			}
			if it.OnClick != nil {
				a.Triggered().Attach(it.OnClick)
			}
		}
		if err := actions.Add(a); err != nil {
			return err
		}
	}
	return nil
}

// Info shows a notification. Safe to call from any goroutine.
func (t *Tray) Info(title, msg string) {
	walk.App().Synchronize(func() {
		if err := t.ni.ShowInfo(title, msg); err != nil {
			slog.Warn("tray notification failed", "err", err)
		}
	})
}

// Warn shows a warning notification. Safe to call from any goroutine.
func (t *Tray) Warn(title, msg string) {
	walk.App().Synchronize(func() {
		if err := t.ni.ShowWarning(title, msg); err != nil {
			slog.Warn("tray notification failed", "err", err)
		}
	})
}

// Dispose removes the icon. UI thread only.
func (t *Tray) Dispose() {
	t.ni.Dispose()
}
