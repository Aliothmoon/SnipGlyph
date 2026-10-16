package ui

import (
	"fmt"
	"image"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	"golang.org/x/image/draw"
	"golang.org/x/sys/windows"

	"SnipGlyph/internal/cliphist"
)

// List metrics, in DIPs.
const (
	rowPadX   = 12
	rowPadY   = 7
	rowGap    = 2
	rowInset  = 4  // selection highlight inset from the edges
	metaW     = 72 // time and pin column on the right
	thumbMaxW = 160
	thumbMaxH = 64
)

const glyphDocument = "\uE8A5"

var (
	colorSelected = walk.RGB(229, 240, 251)
	colorRowHot   = walk.RGB(245, 245, 245)
	listFont      = newFont(10, 0)
	metaFont      = newFont(8, 0)
)

var procGetDoubleClickTime = windows.NewLazySystemDLL("user32.dll").NewProc("GetDoubleClickTime")

// clipList draws clipboard items as rows of two-line previews, thumbnails
// and file names, with a native scroll bar. It never takes the focus: the
// panel's search box keeps it and forwards navigation keys.
type clipList struct {
	cw    *walk.CustomWidget
	items []cliphist.Summary
	sel   int // selected row, -1 for none
	hot   int // row under the mouse, -1 for none
	top   int // scroll offset, pixels

	rowTop []int // y of each row; one extra entry holds the total height
	dpi    int
	lineH  int
	metaH  int

	empty string // shown when there are no items

	thumb  func(id int64) image.Image // loads a thumbnail
	thumbs map[int64]*walk.Bitmap     // scaled for display

	lastClick     time.Time
	lastClickRow  int
	onActivate    func(i int) // double click
	onSelect      func(i int)
	onRightButton func(i int) // before the context menu opens
}

func newClipList(parent walk.Container, thumb func(int64) image.Image) (*clipList, error) {
	l := &clipList{sel: -1, hot: -1, thumb: thumb, thumbs: map[int64]*walk.Bitmap{}, lastClickRow: -1}
	cw, err := walk.NewCustomWidgetPixels(parent, win.WS_VSCROLL, l.paint)
	if err != nil {
		return nil, err
	}
	l.cw = cw
	cw.SetClearsBackground(false)
	cw.SetInvalidatesOnResize(true)
	cw.SetPaintMode(walk.PaintBuffered)
	cw.SizeChanged().Attach(func() {
		l.layout()
		l.ensureVisible(l.sel)
	})
	cw.MouseMove().Attach(func(_, y int, _ walk.MouseButton) {
		if i := l.rowAt(y); i != l.hot {
			if l.hot == -1 {
				l.trackLeave()
			}
			l.hot = i
			cw.Invalidate()
		}
	})
	cw.MouseDown().Attach(l.mouseDown)
	hookWindow(cw.Handle(), l.message)
	return l, nil
}

func (l *clipList) trackLeave() {
	t := win.TRACKMOUSEEVENT{DwFlags: win.TME_LEAVE, HwndTrack: l.cw.Handle()}
	t.CbSize = uint32(unsafe.Sizeof(t))
	win.TrackMouseEvent(&t)
}

func (l *clipList) mouseDown(_, y int, button walk.MouseButton) {
	i := l.rowAt(y)
	switch button {
	case walk.LeftButton:
		if i < 0 {
			return
		}
		l.setSel(i)
		dbl, _, _ := procGetDoubleClickTime.Call()
		if i == l.lastClickRow && time.Since(l.lastClick) <= time.Duration(dbl)*time.Millisecond {
			l.lastClickRow = -1
			if l.onActivate != nil {
				l.onActivate(i)
			}
			return
		}
		l.lastClick, l.lastClickRow = time.Now(), i
	case walk.RightButton:
		if i >= 0 {
			l.setSel(i)
		}
		if l.onRightButton != nil {
			l.onRightButton(i)
		}
	}
}

// message handles scrolling and the end of hovering.
func (l *clipList) message(hwnd win.HWND, msg uint32, wp, lp uintptr) (uintptr, bool) {
	switch msg {
	case win.WM_MOUSELEAVE:
		l.hot = -1
		l.cw.Invalidate()
	case win.WM_MOUSEWHEEL:
		delta := int(int16(wp >> 16))
		l.scrollTo(l.top - delta*3*l.lineH/120)
		return 0, true
	case win.WM_VSCROLL:
		page := l.cw.ClientBoundsPixels().Height
		switch win.LOWORD(uint32(wp)) {
		case win.SB_LINEUP:
			l.scrollTo(l.top - l.lineH)
		case win.SB_LINEDOWN:
			l.scrollTo(l.top + l.lineH)
		case win.SB_PAGEUP:
			l.scrollTo(l.top - page)
		case win.SB_PAGEDOWN:
			l.scrollTo(l.top + page)
		case win.SB_TOP:
			l.scrollTo(0)
		case win.SB_BOTTOM:
			l.scrollTo(l.total())
		case win.SB_THUMBTRACK, win.SB_THUMBPOSITION:
			si := win.SCROLLINFO{FMask: win.SIF_TRACKPOS}
			si.CbSize = uint32(unsafe.Sizeof(si))
			win.GetScrollInfo(hwnd, win.SB_VERT, &si)
			l.scrollTo(int(si.NTrackPos))
		}
		return 0, true
	}
	return 0, false
}

// setItems replaces the rows, keeping the selection on the same item when
// it is still there, else on the first row.
func (l *clipList) setItems(items []cliphist.Summary, keepID int64) {
	l.items = items
	l.sel, l.hot = -1, -1
	for i, it := range items {
		if it.ID == keepID {
			l.sel = i
			break
		}
	}
	if l.sel < 0 && len(items) > 0 {
		l.sel = 0
	}
	if keepID == 0 {
		l.top = 0
	}
	l.layout()
	l.ensureVisible(l.sel)
	l.cw.Invalidate()
}

func (l *clipList) selected() (cliphist.Summary, bool) {
	if l.sel < 0 || l.sel >= len(l.items) {
		return cliphist.Summary{}, false
	}
	return l.items[l.sel], true
}

func (l *clipList) setSel(i int) {
	if len(l.items) == 0 {
		return
	}
	i = max(0, min(i, len(l.items)-1))
	if i != l.sel {
		l.sel = i
		l.cw.Invalidate()
		if l.onSelect != nil {
			l.onSelect(i)
		}
	}
	l.ensureVisible(i)
}

// move moves the selection by n rows; page moves are a screenful.
func (l *clipList) move(n int) { l.setSel(l.sel + n) }

func (l *clipList) movePage(dir int) {
	page := l.cw.ClientBoundsPixels().Height
	if l.sel < 0 {
		l.setSel(0)
		return
	}
	y := l.rowTop[l.sel] + dir*page
	l.setSel(l.rowAtContent(y))
}

func (l *clipList) px(dip int) int { return dip * l.dpi / 96 }

// layout works out every row's height for the current DPI.
func (l *clipList) layout() {
	dpi := l.cw.DPI()
	if dpi != l.dpi || l.lineH == 0 {
		l.dpi = dpi
		l.lineH = fontLineHeight(listFont(), dpi)
		l.metaH = fontLineHeight(metaFont(), dpi)
		for id, b := range l.thumbs {
			b.Dispose()
			delete(l.thumbs, id)
		}
	}
	l.rowTop = l.rowTop[:0]
	y := l.px(rowGap)
	for _, it := range l.items {
		l.rowTop = append(l.rowTop, y)
		y += l.rowHeight(it) + l.px(rowGap)
	}
	l.rowTop = append(l.rowTop, y)
	l.updateScroll()
}

func (l *clipList) total() int {
	if len(l.rowTop) == 0 {
		return 0
	}
	return l.rowTop[len(l.rowTop)-1]
}

func (l *clipList) rowHeight(it cliphist.Summary) int {
	pad := 2 * l.px(rowPadY)
	switch it.Kind {
	case cliphist.Image:
		_, h := l.thumbSize(it)
		return pad + max(h, l.lineH)
	case cliphist.Files:
		return pad + 2*l.lineH
	}
	n := len(cliphist.Preview(it.Preview, 2))
	return pad + max(1, n)*l.lineH
}

// thumbSize is an image's size in the list, in pixels.
func (l *clipList) thumbSize(it cliphist.Summary) (int, int) {
	w, h := max(it.Width, 1), max(it.Height, 1)
	scale := min(float64(l.px(thumbMaxW))/float64(w), float64(l.px(thumbMaxH))/float64(h), 1)
	return max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
}

// rowAt returns the row at client y, or -1.
func (l *clipList) rowAt(y int) int {
	i := l.rowAtContent(y + l.top)
	if i < 0 || i >= len(l.items) || y+l.top >= l.rowTop[i+1]-l.px(rowGap) {
		return -1
	}
	return i
}

// rowAtContent returns the row covering content y, clamped to the rows.
func (l *clipList) rowAtContent(y int) int {
	if len(l.items) == 0 {
		return -1
	}
	i := sort.Search(len(l.items), func(i int) bool { return l.rowTop[i+1] > y })
	return min(i, len(l.items)-1)
}

func (l *clipList) ensureVisible(i int) {
	if i < 0 || i >= len(l.items) {
		return
	}
	page := l.cw.ClientBoundsPixels().Height
	top, bottom := l.rowTop[i]-l.px(rowGap), l.rowTop[i+1]
	switch {
	case top < l.top:
		l.scrollTo(top)
	case bottom > l.top+page:
		l.scrollTo(bottom - page)
	}
}

func (l *clipList) scrollTo(y int) {
	page := l.cw.ClientBoundsPixels().Height
	y = max(0, min(y, l.total()-page))
	if y != l.top {
		l.top = y
		l.cw.Invalidate()
	}
	l.updateScroll()
}

func (l *clipList) updateScroll() {
	si := win.SCROLLINFO{FMask: win.SIF_RANGE | win.SIF_PAGE | win.SIF_POS} // hidden when all fits
	si.CbSize = uint32(unsafe.Sizeof(si))
	si.NMax = int32(max(0, l.total()-1))
	si.NPage = uint32(l.cw.ClientBoundsPixels().Height)
	si.NPos = int32(l.top)
	win.SetScrollInfo(l.cw.Handle(), win.SB_VERT, &si, true)
}

func (l *clipList) paint(c *walk.Canvas, _ walk.Rectangle) error {
	r := l.cw.ClientBoundsPixels()
	if err := fill(c, colorCard, r, 0); err != nil {
		return err
	}
	if len(l.items) == 0 {
		// Centred by hand: DT_VCENTER only works on a single line.
		hf := win.CreateFontIndirect(uiFont().LOGFONTForDPI(l.dpi))
		m := newMeasurer(hf)
		w := r.Width - 2*l.px(rowPadX)
		th := m.height(l.empty, w)
		m.close()
		win.DeleteObject(win.HGDIOBJ(hf))
		box := walk.Rectangle{X: l.px(rowPadX), Y: max(0, (r.Height-th)/2), Width: w, Height: th}
		return c.DrawTextPixels(l.empty, uiFont(), colorMuted, box, walk.TextCenter|walk.TextWordbreak|walk.TextNoPrefix)
	}
	for i := max(0, l.rowAtContent(l.top)); i < len(l.items); i++ {
		y := l.rowTop[i] - l.top
		if y >= r.Height {
			break
		}
		h := l.rowTop[i+1] - l.rowTop[i] - l.px(rowGap)
		if err := l.paintRow(c, i, walk.Rectangle{X: 0, Y: y, Width: r.Width, Height: h}); err != nil {
			return err
		}
	}
	return nil
}

func (l *clipList) paintRow(c *walk.Canvas, i int, r walk.Rectangle) error {
	it := l.items[i]
	bg := walk.Rectangle{X: r.X + l.px(rowInset), Y: r.Y, Width: r.Width - 2*l.px(rowInset), Height: r.Height}
	radius := l.px(4)
	switch {
	case i == l.sel:
		fill(c, colorSelected, bg, radius)
		pill := walk.Rectangle{X: bg.X, Y: bg.Y + bg.Height/4, Width: l.px(3), Height: bg.Height / 2}
		fill(c, colorAccent, pill, l.px(2))
	case i == l.hot:
		fill(c, colorRowHot, bg, radius)
	}

	padX, padY := l.px(rowPadX), l.px(rowPadY)
	meta := walk.Rectangle{X: r.X + r.Width - padX - l.px(metaW), Y: r.Y + padY, Width: l.px(metaW), Height: l.metaH}
	c.DrawTextPixels(ago(it.Used, time.Now()), metaFont(), colorMuted, meta, walk.TextRight|walk.TextSingleLine|walk.TextNoPrefix)
	if it.Pinned {
		pin := meta
		pin.Y += l.metaH + l.px(2)
		pin.Height = l.lineH
		if f := iconFont(); f != nil {
			c.DrawTextPixels(glyphPinned, f, colorAccent, pin, walk.TextRight|walk.TextSingleLine)
		}
	}

	content := walk.Rectangle{X: r.X + padX, Y: r.Y + padY, Width: meta.X - l.px(8) - (r.X + padX), Height: l.lineH}
	line := func(text string, color walk.Color, row int) {
		lr := content
		lr.Y += row * l.lineH
		c.DrawTextPixels(text, listFont(), color, lr, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis|walk.TextNoPrefix)
	}
	switch it.Kind {
	case cliphist.Image:
		w, h := l.thumbSize(it)
		if bmp := l.thumbBitmap(it, w, h); bmp != nil {
			c.DrawImagePixels(bmp, walk.Point{X: content.X, Y: content.Y})
		}
		label := content
		label.X += w + l.px(12)
		label.Width -= w + l.px(12)
		label.Height = h
		c.DrawTextPixels(fmt.Sprintf("图片 %d×%d", it.Width, it.Height), uiFont(), colorMuted, label,
			walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
	case cliphist.Files:
		paths := strings.Split(it.Preview, "\n")
		first := filepath.Base(paths[0])
		if it.Count > 1 {
			first = fmt.Sprintf("%s 等 %d 项", first, it.Count)
		}
		if f := iconFont(); f != nil {
			ir := content
			ir.Width = l.px(20)
			c.DrawTextPixels(glyphDocument, f, colorMuted, ir, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		}
		content.X += l.px(24)
		content.Width -= l.px(24)
		line(first, colorText, 0)
		line(filepath.Dir(paths[0]), colorMuted, 1)
	default:
		for n, text := range cliphist.Preview(it.Preview, 2) {
			line(text, colorText, n)
		}
	}
	return nil
}

// thumbBitmap returns the item's thumbnail scaled to w x h pixels.
func (l *clipList) thumbBitmap(it cliphist.Summary, w, h int) *walk.Bitmap {
	if b, ok := l.thumbs[it.ID]; ok {
		return b
	}
	var bmp *walk.Bitmap
	if src := l.thumb(it.ID); src != nil {
		dst := image.NewNRGBA(image.Rect(0, 0, w, h))
		draw.BiLinear.Scale(dst, dst.Rect, src, src.Bounds(), draw.Src, nil)
		bmp, _ = walk.NewBitmapFromImageForDPI(dst, l.dpi)
	}
	l.thumbs[it.ID] = bmp // nil too: do not retry a broken one
	return bmp
}

func (l *clipList) dispose() {
	for _, b := range l.thumbs {
		if b != nil {
			b.Dispose()
		}
	}
	l.thumbs = nil
}

// fontLineHeight is the height of a line of text in f.
func fontLineHeight(f *walk.Font, dpi int) int {
	if f == nil {
		return 20 * dpi / 96
	}
	hf := win.CreateFontIndirect(f.LOGFONTForDPI(dpi))
	defer win.DeleteObject(win.HGDIOBJ(hf))
	m := newMeasurer(hf)
	defer m.close()
	return m.lineH
}

// ago says when something was copied, briefly.
func ago(t, now time.Time) string {
	d := now.Sub(t)
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d/time.Minute))
	case y1 == y2 && m1 == m2 && d1 == d2:
		return t.Format("15:04")
	case now.AddDate(0, 0, -1).Format("20060102") == t.Format("20060102"):
		return "昨天 " + t.Format("15:04")
	case y1 == y2:
		return t.Format("01-02")
	}
	return t.Format("2006-01-02")
}
