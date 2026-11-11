package ui

import (
	"context"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tailscale/win"

	"SnipGlyph/internal/cliphist"
	"SnipGlyph/internal/winclip"
)

// clipFixture fills a store with one item of each kind, a pinned one and a
// second tab.
func clipFixture(t *testing.T) (*cliphist.Store, map[string]int64) {
	t.Helper()
	st, err := cliphist.Open(filepath.Join(t.TempDir(), "c.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	pic := image.NewNRGBA(image.Rect(0, 0, 300, 200))
	for y := range 200 {
		for x := range 300 {
			pic.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), 180, 255})
		}
	}
	ids := map[string]int64{}
	for name, snap := range map[string]cliphist.Snapshot{
		"files": {Files: []string{`C:\Users\me\报告.docx`, `C:\Users\me\图.png`}},
		"image": {DIB: winclip.EncodeDIB(pic), Source: "SnippingTool.exe"},
		"rich":  {Text: "Hello world\r\nsecond line\r\nthird line", HTML: []byte("<b>Hello</b>"), Source: "chrome.exe"},
		"plain": {Text: "plain text 纯文本"},
	} {
		it, err := snap.Item()
		if err != nil {
			t.Fatal(err)
		}
		if ids[name], err = st.Record(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SetPinned(ctx, ids["plain"], true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddTab(ctx, "常用"); err != nil {
		t.Fatal(err)
	}
	return st, ids
}

func key(p *ClipPanel, vk uintptr) bool {
	return clipKeys{p}.OnPreTranslate(&win.MSG{HWnd: p.search.Handle(), Message: win.WM_KEYDOWN, WParam: vk})
}

func TestClipPanel(t *testing.T) {
	requireUITests(t)
	st, ids := clipFixture(t)
	type paste struct {
		id    int64
		plain bool
	}
	var pasted []paste
	var p *ClipPanel
	Do(func() {
		var err error
		p, err = NewClipPanel(ClipPanelOptions{
			Store:     st,
			Paste:     func(id int64, plain bool) { pasted = append(pasted, paste{id, plain}) },
			Copy:      func(int64) {},
			Recording: true,
		})
		if err != nil {
			t.Error(err) // not Fatal: this is the UI thread
			return
		}
		p.holdOpen++ // the test process may not keep the focus
	})
	if p == nil {
		t.FailNow()
	}
	t.Cleanup(func() { Do(p.Dispose) })
	show := func() {
		t.Helper()
		Do(func() {
			if err := p.Show(); err != nil {
				t.Error(err)
			}
		})
		waitFor(t, "the panel to appear", func() bool { return p.Visible() && !p.cloaked })
	}
	waitHidden := func() {
		t.Helper()
		waitFor(t, "the panel to hide", func() bool {
			return !p.Visible() && !p.mw.IsDisposed() && !win.IsWindowVisible(p.mw.Handle())
		})
	}
	indexOf := func(id int64) int {
		for i, it := range p.list.items {
			if it.ID == id {
				return i
			}
		}
		return -1
	}

	show()
	Do(func() {
		if n := len(p.list.items); n != 4 {
			t.Errorf("%d items listed", n)
			return
		}
		if first := p.list.items[0]; first.ID != ids["plain"] || !first.Pinned {
			t.Errorf("pinned item not first: %+v", first)
		}
		if p.list.sel != 0 || p.count.Text() != "共 4 条" {
			t.Errorf("selection %d, count %q", p.list.sel, p.count.Text())
		}
		// Rows fit their content: one line, three lines cut to two, a
		// thumbnail, and two lines for files.
		lineH := p.list.lineH
		for i, it := range p.list.items {
			h := p.list.rowTop[i+1] - p.list.rowTop[i]
			t.Logf("row %d %s: %d px (line %d px)", i, it.Kind, h, lineH)
			if h < lineH || h > 5*lineH {
				t.Errorf("row %d (%s) is %d px", i, it.Kind, h)
			}
		}
		if !key(p, win.VK_DOWN) || p.list.sel != 1 {
			t.Errorf("Down: selection %d", p.list.sel)
		}
		p.itemMenuFor(1) // builds without trouble
		if n := p.itemMenu.Actions().Len(); n < 8 {
			t.Errorf("item menu has %d entries", n)
		}
		p.search.SetText("hello")
	})
	waitFor(t, "the search", func() bool { return len(p.list.items) == 1 })
	Do(func() {
		if p.list.items[0].ID != ids["rich"] {
			t.Errorf("search found %+v", p.list.items[0])
		}
		key(p, win.VK_RETURN)
	})
	waitHidden()
	if len(pasted) != 1 || pasted[0] != (paste{ids["rich"], false}) {
		t.Errorf("pasted %+v", pasted)
	}

	show()
	Do(func() {
		if p.search.Text() != "" || len(p.list.items) != 4 {
			t.Errorf("shown again with search %q and %d items", p.search.Text(), len(p.list.items))
		}
		p.list.setSel(indexOf(ids["rich"]))
		p.startEdit()
		if p.editing != ids["rich"] || !win.IsWindowVisible(p.editor.Handle()) {
			t.Errorf("editor not open (editing %d)", p.editing)
			p.Hide()
			return
		}
		if !strings.Contains(p.editor.Text(), "second line") {
			t.Errorf("editor holds %q", p.editor.Text())
		}
		p.editor.SetText("edited")
		p.endEdit(true)
		if it, _ := st.Get(context.Background(), ids["rich"]); it.Text != "edited" || it.HTML != nil {
			t.Errorf("after editing %+v", it)
		}
		p.togglePin()
		if it, _ := st.Get(context.Background(), ids["rich"]); !it.Pinned {
			t.Error("Ctrl+P did not pin")
		}
		p.switchTab(1)
		if len(p.list.items) != 0 || !strings.Contains(p.list.empty, "移动到") {
			t.Errorf("second tab: %d items, empty text %q", len(p.list.items), p.list.empty)
		}
		p.switchTab(0)
		p.list.setSel(len(p.list.items) - 1)
		p.moveTo(p.tabList[1].ID)
		if n, _ := st.Count(context.Background(), p.tabList[1].ID); n != 1 {
			t.Errorf("moved: second tab holds %d", n)
		}
		key(p, win.VK_ESCAPE)
	})
	waitHidden()

	// Hidden mid-edit, as the hotkey does: shown again with the list.
	show()
	Do(func() {
		p.list.setSel(indexOf(ids["plain"]))
		p.startEdit()
		if p.editing == 0 {
			t.Error("editor not open")
		}
		p.Hide()
	})
	waitHidden()
	show()
	Do(func() {
		if p.editing != 0 || !win.IsWindowVisible(p.listCard.Handle()) || win.IsWindowVisible(p.editCard.Handle()) || !p.search.Enabled() {
			t.Errorf("shown again still editing (editing %d)", p.editing)
		}
		if it, _ := st.Get(context.Background(), ids["plain"]); it.Text != "plain text 纯文本" {
			t.Errorf("unsaved edit kept: %q", it.Text)
		}
		p.mw.Close() // Alt+F4
	})
	waitHidden()
}
