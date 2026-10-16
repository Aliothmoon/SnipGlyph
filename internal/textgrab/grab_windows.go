// Package textgrab reads the text the user has selected in another program.
package textgrab

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"SnipGlyph/internal/input"
	"SnipGlyph/internal/winclip"
)

// Method says how Selected obtained the text.
type Method string

const (
	ByAutomation Method = "ui-automation"
	ByCopy       Method = "copy"
)

var (
	// ErrNoSelection means neither method found selected text.
	ErrNoSelection = errors.New("no text selected")
	// ErrKeysHeld means the hotkey's modifiers stayed down, so Ctrl+C could
	// not be sent cleanly (Win+Ctrl+C would toggle the colour filter).
	ErrKeysHeld = input.ErrKeysHeld
)

const (
	automationTimeout = 600 * time.Millisecond
	releaseTimeout    = 2 * time.Second
	copyTimeout       = 800 * time.Millisecond
)

// Selected returns the text selected in the foreground program. It asks UI
// Automation first, which has no side effects; when the focused control
// does not expose its selection that way, it sends Ctrl+C and puts the
// clipboard back as it was. Call input.MaskHotkey as soon as the hotkey
// fires.
func Selected(ctx context.Context) (string, Method, error) {
	actx, cancel := context.WithTimeout(ctx, automationTimeout)
	text, err := automationSelected(actx)
	cancel()
	if err == nil && strings.TrimSpace(text) != "" {
		return text, ByAutomation, nil
	}
	slog.Debug("ui automation gave no selection, copying", "err", err)

	text, err = copySelected(ctx)
	if err != nil {
		return "", ByCopy, err
	}
	if strings.TrimSpace(text) == "" {
		return "", ByCopy, ErrNoSelection
	}
	return text, ByCopy, nil
}

func automationSelected(ctx context.Context) (string, error) {
	type result struct {
		s   string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		s, err := withAutomation(focusedSelection)
		ch <- result{s, err}
	}()
	select {
	case r := <-ch:
		return r.s, r.err
	case <-ctx.Done():
		// A hung target program can block UI Automation; leave that call
		// behind and fall back to copying.
		return "", ctx.Err()
	}
}

func copySelected(ctx context.Context) (string, error) {
	if err := input.WaitReleased(ctx, releaseTimeout); err != nil {
		return "", err
	}
	saved, err := winclip.Save(ctx)
	if err != nil {
		return "", fmt.Errorf("save clipboard: %w", err)
	}
	before := winclip.Sequence()
	if err := input.Ctrl('C'); err != nil {
		return "", err
	}

	// Wait for the program to put something on the clipboard. Programs
	// often empty it first and fill it a moment later, so also wait for the
	// sequence number to settle.
	deadline := time.Now().Add(copyTimeout)
	last, stableSince := before, time.Time{}
	for time.Now().Before(deadline) {
		if seq := winclip.Sequence(); seq != last {
			last, stableSince = seq, time.Now()
		} else if last != before && time.Since(stableSince) >= 40*time.Millisecond {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if last == before {
		return "", ErrNoSelection // nothing copied: clipboard untouched
	}

	text, err := winclip.Text(ctx)
	if rerr := winclip.Restore(ctx, saved); rerr != nil {
		slog.Warn("restore clipboard after copying the selection", "err", rerr)
	}
	return text, err
}
