package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/tailscale/walk"

	"SnipGlyph/internal/history"
)

// record is the history entry of one popup or silent OCR. Once a write is
// queued for it, only the history writer goroutine reads or writes id.
type record struct {
	kind history.Kind
	id   int64
}

// historyQueue bounds pending writes; past it they are dropped rather than
// making the UI thread wait on the disk.
const historyQueue = 64

// openHistory opens the database and starts its writer. It returns a
// problem to report, or "".
func (a *App) openHistory() string {
	st, err := history.Open(a.opts.HistoryPath, history.DefaultLimit)
	if err != nil {
		slog.Error("open history", "path", a.opts.HistoryPath, "err", err)
		return "历史记录不可用：" + err.Error()
	}
	a.hist = st
	a.histq = make(chan func(), historyQueue)
	a.histDone = make(chan struct{})
	go func() {
		defer close(a.histDone)
		for write := range a.histq {
			write()
		}
	}()
	return ""
}

// closeHistory finishes pending writes and closes the database. UI thread,
// after the message loop has ended.
func (a *App) closeHistory() {
	if a.hist == nil {
		return
	}
	close(a.histq)
	a.histq = nil // later remember calls drop their entry
	<-a.histDone
	a.hist.Close()
}

// remember saves the current text of r: the first call adds an entry, later
// ones (a translation, edited source text) replace its text. UI thread.
func (a *App) remember(r *record, source, translation, engine string) {
	if a.histq == nil || !a.cfg.History.Enabled || strings.TrimSpace(source) == "" {
		return
	}
	e := history.Entry{Kind: r.kind, Source: source, Translation: translation, Engine: engine}
	select {
	case a.histq <- func() { a.write(r, e) }:
	default:
		slog.Warn("history writer busy, entry dropped")
	}
}

// write runs on the history writer goroutine.
func (a *App) write(r *record, e history.Entry) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if r.id != 0 {
		e.ID = r.id
		err := a.hist.Update(ctx, e)
		if err == nil {
			a.historyChanged()
			return
		}
		if !errors.Is(err, history.ErrNotFound) {
			slog.Warn("update history entry", "err", err)
			return
		}
		// Deleted in the history window while the popup stayed open: the
		// new text is worth keeping again.
	}
	id, err := a.hist.Add(ctx, e)
	if err != nil {
		slog.Warn("add history entry", "err", err)
		return
	}
	r.id = id
	a.historyChanged()
}

// historyChanged refreshes the history window, if it is open.
func (a *App) historyChanged() {
	walk.App().Synchronize(func() {
		if a.histWin != nil {
			a.histWin.Refresh()
		}
	})
}
