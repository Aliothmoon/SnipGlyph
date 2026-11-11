package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"strings"
	"time"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"

	"SnipGlyph/internal/cliphist"
)

const (
	clipPanelW     = 560 // DIP
	clipPanelH     = 640
	clipListLimit  = cliphist.DefaultLimit + 500 // a whole tab, pinned items included
	clipSearchWait = 120 * time.Millisecond
	clipRevealWait = 100 * time.Millisecond // longest wait for walk's layout
	clipHint       = "Enter 粘贴 · Shift+Enter 纯文本 · Ctrl+P 置顶 · F2 编辑 · Del 删除 · 右键更多"
	clipEditHint   = "Ctrl+S 保存 · Esc 取消（保存后去掉原有格式）"
	fillStretch    = 1000 // against the default 1 of every other item
)

// ClipPanelOptions configures the clipboard history panel.
type ClipPanelOptions struct {
	Store *cliphist.Store
	// Paste puts an item on the clipboard and pastes it into the window
	// that was in front before the panel; plain leaves out formatting.
	// Copy only puts it on the clipboard. Both are called once the panel
	// has hidden, on the UI thread.
	Paste func(id int64, plain bool)
	Copy  func(id int64)
	// Recording says whether new copies are being saved; SetRecording
	// changes it.
	Recording bool
}

// ClipPanel is the clipboard history panel: a search box, tabs and the
// list. It is made once and then shown and hidden, because making it takes
// longer than a hotkey should. It hides when it loses focus. Its methods
// run on the UI thread.
type ClipPanel struct {
	opts ClipPanelOptions
	mw   *walk.MainWindow

	search   *walk.LineEdit
	tabs     *tabStrip
	prompt   *walk.Composite
	promptL  *walk.Label
	promptE  *walk.LineEdit
	listCard *walk.Composite
	list     *clipList
	editCard *walk.Composite
	editor   *walk.TextEdit
	hint     *walk.Label
	count    *walk.Label

	itemMenu, moveMenu, copyMenu, tabMenu *walk.Menu

	tabList  []cliphist.Tab
	tab      int                     // index into tabList
	editing  int64                   // item in the editor, 0 when not editing
	onPrompt func(name string) error // what Enter in the prompt does
	menuTab  int                     // tab the tab menu was opened on
	holdOpen int                     // > 0 while a message box is up
	shown    bool                    // between Show and Hide
	cloaked  bool                    // shown but not revealed yet
	timer    *time.Timer
}

func storeCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// NewClipPanel makes the panel, hidden; Show opens it.
func NewClipPanel(opts ClipPanelOptions) (*ClipPanel, error) {
	mw, err := newWindow("SnipGlyph · 剪贴板", &walk.MainWindowCfg{DisableMaximize: true, DisableMinimize: true, DisableResizing: true})
	if err != nil {
		return nil, err
	}
	p := &ClipPanel{opts: opts, mw: mw}
	ok := false
	defer func() {
		if !ok {
			mw.Dispose()
		}
	}()
	if err := p.build(); err != nil {
		return nil, err
	}
	h := mw.Handle()
	walk.App().AddPreTranslateHandlerForHWND(h, clipKeys{p})
	mw.Deactivating().Attach(func() {
		walk.App().Synchronize(func() {
			// It may have been activated again in the meantime.
			if !p.gone() && p.holdOpen == 0 && p.editing == 0 && win.GetForegroundWindow() != h {
				p.Hide()
			}
		})
	})
	mw.Closing().Attach(func(canceled *bool, _ walk.CloseReason) {
		*canceled = true // Alt+F4 hides it, as Esc does
		p.Hide()
	})
	mw.Disposing().Attach(func() {
		p.shown = false
		if p.timer != nil {
			p.timer.Stop()
		}
		walk.App().DeletePreTranslateHandlerForHWND(h)
		p.list.dropThumbs()
	})
	ex := win.GetWindowLong(h, win.GWL_EXSTYLE)
	win.SetWindowLong(h, win.GWL_EXSTYLE, ex|win.WS_EX_TOOLWINDOW)
	frameless(mw)
	if err := p.place(); err != nil {
		return nil, err
	}
	win.SetWindowPos(h, win.HWND_TOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE)
	// walk sets a form up on its first Show. From here on the panel is
	// cloaked whenever it is off the screen.
	cloak(h, true)
	mw.Show()
	mw.SetVisible(false)
	ok = true
	return p, nil
}

// place centres the panel on the monitor under the mouse. Moving it there
// while hidden gives it that monitor's DPI before it is laid out.
func (p *ClipPanel) place() error {
	work, dpi := cursorMonitor()
	return p.mw.SetBoundsPixels(centerIn(work, clipPanelW*dpi/96, clipPanelH*dpi/96))
}

func (p *ClipPanel) gone() bool { return !p.shown || p.mw.IsDisposed() }

// Visible reports whether the panel is open.
func (p *ClipPanel) Visible() bool { return !p.gone() }

// Show opens the panel on the monitor under the mouse, on the first tab
// with an empty search. It appears once laid out and drawn.
func (p *ClipPanel) Show() error {
	if p.mw.IsDisposed() {
		return errors.New("the clipboard panel is gone")
	}
	if p.shown {
		return nil
	}
	if err := p.place(); err != nil {
		return err
	}
	if err := p.loadTabs(cliphist.DefaultTab); err != nil {
		return err
	}
	p.mw.SetVisible(true) // still cloaked
	p.shown, p.cloaked = true, true
	p.reload(0)
	// walk lays the window out on another goroutine and applies the result
	// later. A newer layout request drops the callback, so a timer stands
	// in for it.
	p.mw.RequestImmediateLayoutWithCompletionFunc(p.reveal)
	time.AfterFunc(clipRevealWait, func() { walk.App().Synchronize(p.reveal) })
	return nil
}

// reveal draws the panel, uncloaks it and gives it the focus.
func (p *ClipPanel) reveal() {
	if p.gone() || !p.cloaked {
		return
	}
	p.cloaked = false
	h := p.mw.Handle()
	win.RedrawWindow(h, nil, 0, win.RDW_UPDATENOW|win.RDW_ALLCHILDREN)
	cloak(h, false)
	win.SetForegroundWindow(h)
	p.search.SetFocus()
}

// Hide puts the panel away until the next Show. An unsaved edit is lost.
func (p *ClipPanel) Hide() {
	if p.gone() {
		return
	}
	// Back to the plain list while still shown: walk does not lay out a
	// hidden window, and Show expects the layout to be current.
	if p.editing != 0 {
		p.editing = 0
		setShown(p.editCard, false)
		setShown(p.listCard, true)
		p.search.SetEnabled(true)
	}
	if p.onPrompt != nil {
		p.onPrompt = nil
		setShown(p.prompt, false)
	}
	p.resetHint()
	p.search.SetText("")
	p.shown, p.cloaked = false, false
	if p.timer != nil { // started by the SetText
		p.timer.Stop()
	}
	cloak(p.mw.Handle(), true)
	p.mw.SetVisible(false)
	p.list.dropThumbs()
}

// SetRecording says whether new copies are being saved.
func (p *ClipPanel) SetRecording(on bool) { p.opts.Recording = on }

// Dispose destroys the panel.
func (p *ClipPanel) Dispose() { p.mw.Dispose() }

// Refresh reloads the list, after a new copy was recorded.
func (p *ClipPanel) Refresh() {
	if p.gone() || p.editing != 0 {
		return
	}
	keep := int64(0)
	if it, ok := p.list.selected(); ok {
		keep = it.ID
	}
	p.reload(keep)
}

func (p *ClipPanel) build() error {
	mw := p.mw
	outer := walk.NewVBoxLayout()
	outer.SetMargins(walk.Margins{})
	outer.SetSpacing(0)
	if err := mw.SetLayout(outer); err != nil {
		return err
	}

	bar, err := row(mw)
	if err != nil {
		return err
	}
	bar.Layout().SetMargins(walk.Margins{HNear: 12, VNear: 6, HFar: 6, VFar: 2})
	title, err := label(bar, "剪贴板历史", colorMuted, nil)
	if err != nil {
		return err
	}
	if _, err := walk.NewHSpacer(bar); err != nil {
		return err
	}
	if _, err := newIconButton(bar, glyphClose, "关闭（Esc）", true, p.Hide); err != nil {
		return err
	}
	dragBy(mw, bar)
	dragBy(mw, title)

	body, err := walk.NewComposite(mw)
	if err != nil {
		return err
	}
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 12, VNear: 2, HFar: 12, VFar: 10})
	layout.SetSpacing(6)
	if err := body.SetLayout(layout); err != nil {
		return err
	}
	// walk counts custom-drawn widgets (the header buttons, the tab strip)
	// as wanting to grow; a capped height keeps them small but their slot
	// would still take a share of the spare height. The list takes it all.
	outer.SetStretchFactor(body, fillStretch)

	if p.search, err = walk.NewLineEdit(body); err != nil {
		return err
	}
	setCueBanner(p.search, "搜索当前标签，空格分隔多个关键字")
	p.search.TextChanged().Attach(p.searchSoon)

	if p.tabs, err = newTabStrip(body); err != nil {
		return err
	}
	p.tabs.onSelect = func(i int) { p.switchTab(i) }
	p.tabs.onAdd = p.addTab
	p.tabs.onRightButton = p.tabMenuFor

	if p.prompt, err = row(body); err != nil {
		return err
	}
	if p.promptL, err = label(p.prompt, "", colorText, nil); err != nil {
		return err
	}
	if p.promptE, err = walk.NewLineEdit(p.prompt); err != nil {
		return err
	}
	setShown(p.prompt, false)

	var inner *walk.Composite
	if p.listCard, inner, err = card(body); err != nil {
		return err
	}
	inner.Layout().SetMargins(walk.Margins{})
	thumb := func(id int64) image.Image {
		ctx, cancel := storeCtx()
		defer cancel()
		data, err := p.opts.Store.Thumb(ctx, id)
		if err != nil || len(data) == 0 {
			return nil
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil
		}
		return img
	}
	if p.list, err = newClipList(inner, thumb); err != nil {
		return err
	}
	p.list.onActivate = func(int) { p.paste(false) }
	p.list.onRightButton = p.itemMenuFor
	p.list.cw.MouseDown().Attach(func(int, int, walk.MouseButton) {
		if !p.gone() { // a double click pastes and closes the panel
			p.search.SetFocus()
		}
	})
	layout.SetStretchFactor(p.listCard, fillStretch)

	var editInner *walk.Composite
	if p.editCard, editInner, err = card(body); err != nil {
		return err
	}
	if p.editor, err = textBox(editInner); err != nil {
		return err
	}
	layout.SetStretchFactor(p.editCard, fillStretch)
	setShown(p.editCard, false)

	foot, err := row(body)
	if err != nil {
		return err
	}
	if p.hint, err = label(foot, clipHint, colorMuted, nil); err != nil {
		return err
	}
	if _, err := walk.NewHSpacer(foot); err != nil {
		return err
	}
	if p.count, err = label(foot, "", colorMuted, nil); err != nil {
		return err
	}

	for _, m := range []**walk.Menu{&p.itemMenu, &p.moveMenu, &p.copyMenu, &p.tabMenu} {
		if *m, err = walk.NewMenu(); err != nil {
			return err
		}
	}
	p.list.cw.SetContextMenu(p.itemMenu)
	p.tabs.cw.SetContextMenu(p.tabMenu)
	return nil
}

// loadTabs reads the tabs and selects the one with id sel.
func (p *ClipPanel) loadTabs(sel int64) error {
	ctx, cancel := storeCtx()
	defer cancel()
	tabs, err := p.opts.Store.Tabs(ctx)
	if err != nil {
		return err
	}
	p.tabList, p.tab = tabs, 0
	for i, t := range tabs {
		if t.ID == sel {
			p.tab = i
		}
	}
	p.tabs.setTabs(tabs, p.tab)
	return nil
}

func (p *ClipPanel) tabID() int64 { return p.tabList[p.tab].ID }

func (p *ClipPanel) switchTab(i int) {
	if i < 0 || i >= len(p.tabList) || p.editing != 0 {
		return
	}
	p.tab = i
	p.tabs.setTabs(p.tabList, i)
	p.reload(0)
	p.search.SetFocus()
}

func (p *ClipPanel) searchSoon() {
	if p.timer != nil {
		p.timer.Stop()
	}
	p.timer = time.AfterFunc(clipSearchWait, func() {
		walk.App().Synchronize(func() {
			if !p.gone() {
				p.reload(0)
			}
		})
	})
}

// reload lists the current tab, keeping item keep selected if it is there.
func (p *ClipPanel) reload(keep int64) {
	ctx, cancel := storeCtx()
	defer cancel()
	query := strings.TrimSpace(p.search.Text())
	items, err := p.opts.Store.List(ctx, p.tabID(), query, clipListLimit)
	if err != nil {
		slog.Warn("list clipboard history", "err", err)
		p.list.empty = "读取剪贴板记录失败：" + err.Error()
		p.list.setItems(nil, 0)
		return
	}
	switch {
	case query != "":
		p.list.empty = "没有找到"
	case p.tabID() != cliphist.DefaultTab:
		p.list.empty = "这个标签还是空的。\n在条目上右键，选「移动到」或「复制到」。"
	case !p.opts.Recording:
		p.list.empty = "剪贴板记录已关闭，可在设置中开启。"
	default:
		p.list.empty = "还没有记录。\n复制的文字、图片和文件会出现在这里。"
	}
	p.list.setItems(items, keep)
	if query != "" {
		p.count.SetText(fmt.Sprintf("找到 %d 条", len(items)))
	} else {
		p.count.SetText(fmt.Sprintf("共 %d 条", len(items)))
	}
}

func (p *ClipPanel) selected() (cliphist.Summary, bool) { return p.list.selected() }

func (p *ClipPanel) paste(plain bool) {
	it, ok := p.selected()
	if !ok {
		return
	}
	p.Hide()
	p.opts.Paste(it.ID, plain)
}

func (p *ClipPanel) copyItem() {
	it, ok := p.selected()
	if !ok {
		return
	}
	p.Hide()
	p.opts.Copy(it.ID)
}

// change runs a store update on the selected item and reloads, keeping
// the selection on keep (0: the same item).
func (p *ClipPanel) change(f func(ctx context.Context, it cliphist.Summary) error, keep int64) {
	it, ok := p.selected()
	if !ok {
		return
	}
	ctx, cancel := storeCtx()
	defer cancel()
	if err := f(ctx, it); err != nil {
		p.showError(err)
		return
	}
	if keep == 0 {
		keep = it.ID
	}
	p.reload(keep)
}

func (p *ClipPanel) togglePin() {
	p.change(func(ctx context.Context, it cliphist.Summary) error {
		return p.opts.Store.SetPinned(ctx, it.ID, !it.Pinned)
	}, 0)
}

// neighbour is the item to select once the selected one leaves the list.
func (p *ClipPanel) neighbour() int64 {
	i := p.list.sel
	switch {
	case i+1 < len(p.list.items):
		return p.list.items[i+1].ID
	case i > 0:
		return p.list.items[i-1].ID
	}
	return -1
}

func (p *ClipPanel) deleteItem() {
	p.change(func(ctx context.Context, it cliphist.Summary) error {
		return p.opts.Store.Delete(ctx, it.ID)
	}, p.neighbour())
}

func (p *ClipPanel) moveTo(tab int64) {
	p.change(func(ctx context.Context, it cliphist.Summary) error {
		return p.opts.Store.MoveTo(ctx, it.ID, tab)
	}, p.neighbour())
}

func (p *ClipPanel) copyTo(tab int64) {
	p.change(func(ctx context.Context, it cliphist.Summary) error {
		_, err := p.opts.Store.CopyTo(ctx, it.ID, tab)
		return err
	}, 0)
}

func (p *ClipPanel) showError(err error) {
	slog.Warn("clipboard panel", "err", err)
	p.hint.SetTextColor(colorError)
	p.hint.SetText("操作失败：" + err.Error())
}

func (p *ClipPanel) resetHint() {
	p.hint.SetTextColor(colorMuted)
	if p.editing != 0 {
		p.hint.SetText(clipEditHint)
	} else {
		p.hint.SetText(clipHint)
	}
}

// startEdit opens the selected text item in the editor.
func (p *ClipPanel) startEdit() {
	sel, ok := p.selected()
	if !ok || sel.Kind != cliphist.Text {
		return
	}
	ctx, cancel := storeCtx()
	defer cancel()
	it, err := p.opts.Store.Get(ctx, sel.ID)
	if err != nil {
		p.showError(err)
		return
	}
	p.editing = it.ID
	p.editor.SetText(crlf(lf(it.Text)))
	setShown(p.listCard, false)
	setShown(p.editCard, true)
	p.search.SetEnabled(false)
	p.resetHint()
	p.editor.SetFocus()
}

func (p *ClipPanel) endEdit(save bool) {
	id := p.editing
	if save {
		ctx, cancel := storeCtx()
		err := p.opts.Store.SetText(ctx, id, p.editor.Text())
		cancel()
		if err != nil {
			p.showError(err)
			return
		}
	}
	p.editing = 0
	setShown(p.editCard, false)
	setShown(p.listCard, true)
	p.search.SetEnabled(true)
	p.resetHint()
	p.reload(id)
	p.search.SetFocus()
}

// askName shows the tab name row; ok gets the name on Enter.
func (p *ClipPanel) askName(label, initial string, ok func(name string) error) {
	p.promptL.SetText(label)
	p.promptE.SetText(initial)
	p.onPrompt = ok
	p.hint.SetTextColor(colorMuted)
	p.hint.SetText("Enter 确定 · Esc 取消")
	setShown(p.prompt, true)
	p.promptE.SetFocus()
	p.promptE.SetTextSelection(0, -1)
}

func (p *ClipPanel) endPrompt(confirm bool) {
	if confirm {
		if err := p.onPrompt(p.promptE.Text()); err != nil {
			if errors.Is(err, cliphist.ErrTabExists) {
				err = errors.New("已经有同名的标签")
			}
			p.showError(err)
			return
		}
	}
	p.onPrompt = nil
	setShown(p.prompt, false)
	p.resetHint()
	p.search.SetFocus()
}

func (p *ClipPanel) addTab() {
	p.askName("新标签名称", "", func(name string) error {
		ctx, cancel := storeCtx()
		defer cancel()
		t, err := p.opts.Store.AddTab(ctx, name)
		if err != nil {
			return err
		}
		if err := p.loadTabs(t.ID); err != nil {
			return err
		}
		p.reload(0)
		return nil
	})
}

func (p *ClipPanel) renameTab(i int) {
	t := p.tabList[i]
	p.askName("标签名称", t.Name, func(name string) error {
		ctx, cancel := storeCtx()
		defer cancel()
		if err := p.opts.Store.RenameTab(ctx, t.ID, name); err != nil {
			return err
		}
		return p.loadTabs(p.tabID())
	})
}

func (p *ClipPanel) deleteTab(i int) {
	t := p.tabList[i]
	p.holdOpen++
	answer := walk.MsgBox(p.mw, "删除标签", fmt.Sprintf("删除标签「%s」和其中的全部条目？", t.Name),
		walk.MsgBoxYesNo|walk.MsgBoxIconWarning|walk.MsgBoxDefButton2)
	p.holdOpen--
	if answer != walk.DlgCmdYes {
		return
	}
	ctx, cancel := storeCtx()
	defer cancel()
	if err := p.opts.Store.DeleteTab(ctx, t.ID); err != nil {
		p.showError(err)
		return
	}
	if err := p.loadTabs(cliphist.DefaultTab); err != nil {
		p.showError(err)
		return
	}
	p.reload(0)
	p.search.SetFocus()
}

// itemMenuFor fills the item context menu before it opens.
func (p *ClipPanel) itemMenuFor(int) {
	acts := p.itemMenu.Actions()
	acts.Clear()
	it, ok := p.selected()
	if !ok {
		return
	}
	add := func(list *walk.ActionList, text string, enabled bool, f func()) {
		a := walk.NewAction()
		a.SetText(text)
		a.SetEnabled(enabled)
		a.Triggered().Attach(func() { p.later(f) })
		list.Add(a)
	}
	add(acts, "粘贴\tEnter", true, func() { p.paste(false) })
	add(acts, "以纯文本粘贴\tShift+Enter", it.Kind == cliphist.Text && it.Rich, func() { p.paste(true) })
	add(acts, "复制\tCtrl+C", true, p.copyItem)
	acts.Add(walk.NewSeparatorAction())
	pin := "置顶\tCtrl+P"
	if it.Pinned {
		pin = "取消置顶\tCtrl+P"
	}
	add(acts, pin, true, p.togglePin)
	add(acts, "编辑\tF2", it.Kind == cliphist.Text, p.startEdit)
	for _, sub := range []struct {
		menu *walk.Menu
		text string
		f    func(int64)
	}{{p.moveMenu, "移动到", p.moveTo}, {p.copyMenu, "复制到", p.copyTo}} {
		subActs := sub.menu.Actions()
		subActs.Clear()
		for _, t := range p.tabList {
			if t.ID != p.tabID() {
				id, f := t.ID, sub.f
				add(subActs, t.Name, true, func() { f(id) })
			}
		}
		if subActs.Len() == 0 {
			add(subActs, "（没有其他标签，点标签栏的 + 新建）", false, func() {})
		}
		a := walk.NewMenuAction(sub.menu)
		a.SetText(sub.text)
		acts.Add(a)
	}
	acts.Add(walk.NewSeparatorAction())
	add(acts, "删除\tDel", true, p.deleteItem)
}

// tabMenuFor fills the tab context menu for tab i before it opens.
func (p *ClipPanel) tabMenuFor(i int) {
	acts := p.tabMenu.Actions()
	acts.Clear()
	p.menuTab = i
	add := func(text string, f func()) {
		a := walk.NewAction()
		a.SetText(text)
		a.Triggered().Attach(func() { p.later(f) })
		acts.Add(a)
	}
	add("新建标签…", p.addTab)
	if i > 0 && i < len(p.tabList) {
		add("重命名…", func() { p.renameTab(i) })
		add("删除标签…", func() { p.deleteTab(i) })
	}
}

// later runs f after the menu that chose it has closed, unless the panel
// closed meanwhile.
func (p *ClipPanel) later(f func()) {
	walk.App().Synchronize(func() {
		if !p.gone() {
			f()
		}
	})
}

// clipKeys drives the panel from the keyboard while the search box (or
// the editor, or the tab name box) has the focus.
type clipKeys struct{ p *ClipPanel }

func (k clipKeys) OnPreTranslate(msg *win.MSG) bool {
	p := k.p
	if p.gone() || msg.Message != win.WM_KEYDOWN {
		return false
	}
	h := p.mw.Handle()
	if msg.HWnd != h && !win.IsChild(h, msg.HWnd) {
		return false
	}
	key := msg.WParam
	mods := walk.ModifiersDown()
	ctrl, shift := mods == walk.ModControl, mods == walk.ModShift

	if p.editing != 0 {
		switch {
		case key == win.VK_ESCAPE && mods == 0:
			p.endEdit(false)
			return true
		case (key == 'S' || key == win.VK_RETURN) && ctrl:
			p.endEdit(true)
			return true
		}
		return false
	}
	if p.onPrompt != nil && msg.HWnd == p.promptE.Handle() {
		switch {
		case key == win.VK_RETURN && mods == 0:
			p.endPrompt(true)
			return true
		case key == win.VK_ESCAPE && mods == 0:
			p.endPrompt(false)
			return true
		}
		return false
	}

	switch {
	case key == win.VK_UP && mods == 0:
		p.list.move(-1)
	case key == win.VK_DOWN && mods == 0:
		p.list.move(1)
	case key == win.VK_PRIOR && mods == 0:
		p.list.movePage(-1)
	case key == win.VK_NEXT && mods == 0:
		p.list.movePage(1)
	case key == win.VK_HOME && ctrl:
		p.list.setSel(0)
	case key == win.VK_END && ctrl:
		p.list.setSel(len(p.list.items) - 1)
	case key == win.VK_RETURN && (mods == 0 || shift):
		p.paste(shift)
	case key == win.VK_ESCAPE && mods == 0:
		if p.search.Text() != "" {
			p.search.SetText("")
		} else {
			walk.App().Synchronize(p.Hide)
		}
	case key == win.VK_DELETE && (shift || (mods == 0 && p.search.Text() == "")):
		p.deleteItem()
	case key == 'C' && ctrl && !hasSelection(p.search):
		p.copyItem()
	case key == 'P' && ctrl:
		p.togglePin()
	case key == win.VK_F2 && mods == 0:
		p.startEdit()
	case key == win.VK_TAB && (ctrl || mods == walk.ModControl|walk.ModShift):
		n := len(p.tabList)
		if mods&walk.ModShift != 0 {
			p.switchTab((p.tab + n - 1) % n)
		} else {
			p.switchTab((p.tab + 1) % n)
		}
	default:
		return false
	}
	return true
}

// hasSelection reports whether some of le's text is selected.
func hasSelection(le *walk.LineEdit) bool {
	r := le.SendMessage(win.EM_GETSEL, 0, 0)
	return win.LOWORD(uint32(r)) != win.HIWORD(uint32(r))
}
