package ui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	"golang.org/x/sys/windows"

	"SnipGlyph/internal/history"
)

const (
	historyShown    = 500 // rows listed at most
	historyDebounce = 250 * time.Millisecond
)

// HistoryOptions configures the history window.
type HistoryOptions struct {
	// Recording says whether new results are being saved.
	Recording bool
	// Search returns up to limit entries containing every term of query,
	// newest first, and the number of entries in total. Delete removes one
	// entry and Clear all of them. These run on a background goroutine.
	Search func(ctx context.Context, query string, limit int) (entries []history.Entry, total int, err error)
	Delete func(ctx context.Context, id int64) error
	Clear  func(ctx context.Context) error
	// Open shows an entry in a result popup. UI thread.
	Open func(e history.Entry)
	// OnClose runs on the UI thread when the window closes.
	OnClose func()
}

// HistoryWindow lists saved results with search and a detail view. Its
// methods run on the UI thread.
type HistoryWindow struct {
	opts HistoryOptions
	mw   *walk.MainWindow

	search  *walk.LineEdit
	clearBt *walk.PushButton
	table   *walk.TableView
	model   *entryModel
	info    *walk.Label
	source  *walk.TextEdit
	result  *walk.TextEdit
	copySrc *walk.LinkLabel
	copyRes *walk.LinkLabel
	count   *walk.Label
	delBt   *walk.PushButton
	openBt  *walk.PushButton

	seq     int // of the latest search; older results are dropped
	timer   *time.Timer
	closing bool
}

// NewHistoryWindow creates and shows the window. UI thread only.
func NewHistoryWindow(opts HistoryOptions) (*HistoryWindow, error) {
	mw, err := newWindow("SnipGlyph 历史记录", &walk.MainWindowCfg{})
	if err != nil {
		return nil, err
	}
	h := &HistoryWindow{opts: opts, mw: mw, model: &entryModel{}}
	ok := false
	defer func() {
		if !ok {
			mw.Dispose()
		}
	}()
	if err := h.build(); err != nil {
		return nil, err
	}
	hwnd := mw.Handle()
	walk.App().AddPreTranslateHandlerForHWND(hwnd, historyKeys{h})
	mw.Closing().Attach(func(*bool, walk.CloseReason) {
		h.closing = true
		if h.timer != nil {
			h.timer.Stop()
		}
		walk.App().DeletePreTranslateHandlerForHWND(hwnd)
		if opts.OnClose != nil {
			opts.OnClose()
		}
	})
	work, dpi := cursorMonitor()
	if err := mw.SetBoundsPixels(centerIn(work, 820*dpi/96, 600*dpi/96)); err != nil {
		return nil, err
	}
	mw.Show()
	win.SetForegroundWindow(hwnd)
	h.search.SetFocus()
	h.Refresh()
	ok = true
	return h, nil
}

// Activate brings the window to the front.
func (h *HistoryWindow) Activate() {
	if !h.gone() {
		bringToFront(h.mw)
	}
}

// SetRecording updates the note on whether new results are saved.
func (h *HistoryWindow) SetRecording(on bool) {
	if h.gone() {
		return
	}
	h.opts.Recording = on
	h.Refresh()
}

func (h *HistoryWindow) gone() bool { return h.closing || h.mw.IsDisposed() }

func (h *HistoryWindow) build() error {
	mw := h.mw
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 12, VNear: 10, HFar: 12, VFar: 10})
	layout.SetSpacing(8)
	if err := mw.SetLayout(layout); err != nil {
		return err
	}

	top, err := row(mw)
	if err != nil {
		return err
	}
	if h.search, err = walk.NewLineEdit(top); err != nil {
		return err
	}
	// Shown even while the box has focus, which it has on opening.
	cue, _ := windows.UTF16PtrFromString("搜索原文或译文，空格分隔多个关键字（Ctrl+F）")
	h.search.SendMessage(emSetCueBanner, 1, uintptr(unsafe.Pointer(cue)))
	h.search.TextChanged().Attach(h.searchSoon)
	if h.clearBt, err = walk.NewPushButton(top); err != nil {
		return err
	}
	h.clearBt.SetText("清空全部…")
	h.clearBt.Clicked().Attach(h.clearAll)

	if h.table, err = walk.NewTableView(mw); err != nil {
		return err
	}
	for _, c := range []struct {
		title string
		width int
	}{{"时间", 96}, {"类型", 72}, {"原文", 300}, {"译文", 240}} {
		col := walk.NewTableViewColumn()
		col.SetTitle(c.title)
		col.SetWidth(c.width)
		if err := h.table.Columns().Add(col); err != nil {
			return err
		}
	}
	h.table.SetLastColumnStretched(true)
	if err := h.table.SetModel(h.model); err != nil {
		return err
	}
	h.table.CurrentIndexChanged().Attach(h.showCurrent)
	h.table.ItemActivated().Attach(h.openCurrent)

	// Detail: source and translation side by side.
	detail, err := row(mw)
	if err != nil {
		return err
	}
	detail.Layout().SetSpacing(10)
	if h.copySrc, h.source, err = detailCard(detail, "原文"); err != nil {
		return err
	}
	if h.copyRes, h.result, err = detailCard(detail, "译文"); err != nil {
		return err
	}
	layout.SetStretchFactor(h.table, 3)
	layout.SetStretchFactor(detail, 2)

	foot, err := row(mw)
	if err != nil {
		return err
	}
	if h.info, err = label(foot, "", colorMuted, nil); err != nil {
		return err
	}
	if _, err := walk.NewHSpacer(foot); err != nil {
		return err
	}
	if h.count, err = label(foot, "", colorMuted, nil); err != nil {
		return err
	}
	if h.delBt, err = walk.NewPushButton(foot); err != nil {
		return err
	}
	h.delBt.SetText("删除")
	h.delBt.Clicked().Attach(h.deleteCurrent)
	if h.openBt, err = walk.NewPushButton(foot); err != nil {
		return err
	}
	h.openBt.SetText("在弹窗中打开")
	h.openBt.SetToolTipText("可以修改原文、重新翻译（双击或 Enter）")
	h.openBt.Clicked().Attach(h.openCurrent)

	h.copySrc.LinkActivated().Attach(func(*walk.LinkLabelLink) { h.copy(h.copySrc, func(e history.Entry) string { return e.Source }) })
	h.copyRes.LinkActivated().Attach(func(*walk.LinkLabelLink) { h.copy(h.copyRes, func(e history.Entry) string { return e.Translation }) })
	h.showCurrent()
	return nil
}

// detailCard adds a card with a heading, a copy link and a read-only box.
func detailCard(parent walk.Container, title string) (*walk.LinkLabel, *walk.TextEdit, error) {
	_, inner, err := card(parent)
	if err != nil {
		return nil, nil, err
	}
	head, err := row(inner)
	if err != nil {
		return nil, nil, err
	}
	if _, err := label(head, title, colorMuted, titleFont()); err != nil {
		return nil, nil, err
	}
	if _, err := walk.NewHSpacer(head); err != nil {
		return nil, nil, err
	}
	copyLink, err := walk.NewLinkLabel(head)
	if err != nil {
		return nil, nil, err
	}
	box, err := textBox(inner)
	if err != nil {
		return nil, nil, err
	}
	box.SetReadOnly(true)
	box.SizeChanged().Attach(func() { syncScroll(box) })
	return copyLink, box, nil
}

// Refresh runs the current search again, for example after a new result
// was saved.
func (h *HistoryWindow) Refresh() {
	if h.gone() {
		return
	}
	h.seq++
	seq, query := h.seq, h.search.Text()
	go func() {
		ctx, cancel := context.WithTimeout(walk.App().Context(), 10*time.Second)
		defer cancel()
		entries, total, err := h.opts.Search(ctx, query, historyShown)
		walk.App().Synchronize(func() {
			if !h.gone() && seq == h.seq {
				h.showEntries(entries, total, query, err)
			}
		})
	}()
}

func (h *HistoryWindow) searchSoon() {
	if h.timer != nil {
		h.timer.Stop()
	}
	h.timer = time.AfterFunc(historyDebounce, func() { walk.App().Synchronize(h.Refresh) })
}

func (h *HistoryWindow) showEntries(entries []history.Entry, total int, query string, err error) {
	if err != nil {
		slog.Warn("search history", "err", err)
		h.count.SetTextColor(colorError)
		h.count.SetText("读取历史记录失败：" + err.Error())
		return
	}
	// Keep the selected entry selected, or select the newest.
	keep := int64(0)
	if e, ok := h.current(); ok {
		keep = e.ID
	}
	h.model.items = entries
	h.model.PublishRowsReset()
	idx := 0
	for i, e := range entries {
		if e.ID == keep {
			idx = i
			break
		}
	}
	if len(entries) > 0 {
		h.table.SetCurrentIndex(idx)
	} else {
		h.table.SetCurrentIndex(-1)
	}
	h.showCurrent()

	var msg string
	switch {
	case strings.TrimSpace(query) != "" && len(entries) >= historyShown:
		msg = fmt.Sprintf("找到很多条，只列出最近 %d 条", historyShown)
	case strings.TrimSpace(query) != "":
		msg = fmt.Sprintf("找到 %d 条", len(entries))
	case len(entries) < total:
		msg = fmt.Sprintf("共 %d 条，列出最近 %d 条", total, len(entries))
	default:
		msg = fmt.Sprintf("共 %d 条", total)
	}
	if !h.opts.Recording {
		msg += " · 已暂停记录，可在设置中开启"
	}
	h.count.SetTextColor(colorMuted)
	h.count.SetText(msg)
	h.clearBt.SetEnabled(total > 0)
}

func (h *HistoryWindow) current() (history.Entry, bool) {
	i := h.table.CurrentIndex()
	if i < 0 || i >= len(h.model.items) {
		return history.Entry{}, false
	}
	return h.model.items[i], true
}

func (h *HistoryWindow) showCurrent() {
	e, ok := h.current()
	h.delBt.SetEnabled(ok)
	h.openBt.SetEnabled(ok)
	h.copySrc.SetEnabled(ok)
	h.copyRes.SetEnabled(ok && e.Translation != "")
	setLinkText(h.copySrc, "复制", ok)
	setLinkText(h.copyRes, "复制", ok && e.Translation != "")
	if !ok {
		h.info.SetText("")
		h.source.SetText("")
		h.result.SetText("")
		return
	}
	info := e.Kind.String() + " · " + e.Time.Format("2006-01-02 15:04:05")
	if e.Engine != "" {
		info += " · " + e.Engine
	}
	h.info.SetText(info)
	h.source.SetText(crlf(e.Source))
	if e.Translation == "" {
		h.result.SetTextColor(colorMuted)
		h.result.SetText("（没有翻译）")
	} else {
		h.result.SetTextColor(colorText)
		h.result.SetText(crlf(e.Translation))
	}
	syncScroll(h.source)
	syncScroll(h.result)
}

const (
	emGetRect      = 0x00B2
	emSetCueBanner = 0x1501
)

// syncScroll shows box's scroll bar only when its text overflows.
func syncScroll(box *walk.TextEdit) {
	var fr win.RECT // the formatting rectangle: where text goes
	box.SendMessage(emGetRect, 0, uintptr(unsafe.Pointer(&fr)))
	m := newMeasurer(win.HFONT(box.SendMessage(win.WM_GETFONT, 0, 0)))
	defer m.close()
	showVScroll(box, m.height(box.Text(), int(fr.Right-fr.Left)) > int(fr.Bottom-fr.Top))
}

func (h *HistoryWindow) copy(ll *walk.LinkLabel, text func(history.Entry) string) {
	e, ok := h.current()
	if !ok {
		return
	}
	if err := walk.Clipboard().SetText(text(e)); err != nil {
		slog.Warn("copy to clipboard", "err", err)
		return
	}
	setLinkText(ll, "已复制", false)
}

func (h *HistoryWindow) openCurrent() {
	if e, ok := h.current(); ok && h.opts.Open != nil {
		h.opts.Open(e)
	}
}

func (h *HistoryWindow) deleteCurrent() {
	e, ok := h.current()
	if !ok {
		return
	}
	// Select the next row once the list reloads.
	if i := h.table.CurrentIndex(); i+1 < len(h.model.items) {
		h.table.SetCurrentIndex(i + 1)
	} else if i > 0 {
		h.table.SetCurrentIndex(i - 1)
	}
	h.background(func(ctx context.Context) error { return h.opts.Delete(ctx, e.ID) })
}

func (h *HistoryWindow) clearAll() {
	if walk.MsgBox(h.mw, "清空历史记录", "删除全部历史记录？删除后无法恢复。",
		walk.MsgBoxYesNo|walk.MsgBoxIconWarning|walk.MsgBoxDefButton2) != walk.DlgCmdYes {
		return
	}
	h.background(h.opts.Clear)
}

// background runs a change off the UI thread, then reloads the list.
func (h *HistoryWindow) background(change func(ctx context.Context) error) {
	go func() {
		ctx, cancel := context.WithTimeout(walk.App().Context(), 30*time.Second)
		defer cancel()
		err := change(ctx)
		walk.App().Synchronize(func() {
			if h.gone() {
				return
			}
			if err != nil {
				slog.Warn("change history", "err", err)
				walk.MsgBox(h.mw, "历史记录", "操作失败："+err.Error(), walk.MsgBoxIconError)
			}
			h.Refresh()
		})
	}()
}

// historyKeys handles the window's shortcuts.
type historyKeys struct{ h *HistoryWindow }

func (k historyKeys) OnPreTranslate(msg *win.MSG) bool {
	h := k.h
	if h.gone() || msg.Message != win.WM_KEYDOWN {
		return false
	}
	hwnd := h.mw.Handle()
	if msg.HWnd != hwnd && !win.IsChild(hwnd, msg.HWnd) {
		return false
	}
	mods := walk.ModifiersDown()
	switch {
	case msg.WParam == 'F' && mods == walk.ModControl:
		h.search.SetFocus()
		h.search.SetTextSelection(0, -1)
		return true
	case msg.WParam == win.VK_ESCAPE && mods == 0:
		if msg.HWnd == h.search.Handle() && h.search.Text() != "" {
			h.search.SetText("")
			return true
		}
		walk.App().Synchronize(func() {
			if !h.gone() {
				h.mw.Close()
			}
		})
		return true
	case msg.WParam == win.VK_DELETE && mods == 0 && msg.HWnd == h.table.Handle():
		h.deleteCurrent()
		return true
	}
	return false
}

// entryModel lists entries in the table.
type entryModel struct {
	walk.TableModelBase
	items []history.Entry
}

func (m *entryModel) RowCount() int { return len(m.items) }

func (m *entryModel) Value(row, col int) any {
	if row < 0 || row >= len(m.items) {
		return ""
	}
	e := m.items[row]
	switch col {
	case 0:
		return shortTime(e.Time, time.Now())
	case 1:
		return e.Kind.String()
	case 2:
		return preview(e.Source)
	case 3:
		return preview(e.Translation)
	}
	return ""
}

// shortTime shows the time of day for today, and the date otherwise.
func shortTime(t, now time.Time) string {
	switch {
	case t.YearDay() == now.YearDay() && t.Year() == now.Year():
		return t.Format("15:04")
	case t.Year() == now.Year():
		return t.Format("01-02 15:04")
	}
	return t.Format("2006-01-02")
}

// preview squeezes text onto one line for the table.
func preview(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const maxRunes = 120
	if utf8.RuneCountInString(s) > maxRunes {
		s = string([]rune(s)[:maxRunes]) + "…"
	}
	return s
}
