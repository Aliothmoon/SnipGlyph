package cliphist

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"slices"
	"testing"

	"SnipGlyph/internal/winclip"
)

func picture(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), 99, 255})
		}
	}
	return img
}

func pngOf(img image.Image) []byte {
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func TestSnapshotItem(t *testing.T) {
	pic := picture(640, 400)
	html := []byte("Version:0.9 <b>x</b>")

	text, err := Snapshot{Text: "hello", HTML: html, DIB: winclip.EncodeDIB(pic), Source: "excel.exe"}.Item()
	if err != nil || text.Kind != Text || text.Text != "hello" || !bytes.Equal(text.HTML, html) || text.PNG != nil {
		t.Errorf("text wins over a picture: %+v, %v", text, err)
	}

	fromDIB, err := Snapshot{DIB: winclip.EncodeDIB(pic), HTML: html, Files: []string{`C:\tmp\a.png`}}.Item()
	if err != nil || fromDIB.Kind != Image || fromDIB.Width != 640 || fromDIB.Height != 400 || len(fromDIB.PNG) == 0 {
		t.Fatalf("picture wins over files: %+v, %v", fromDIB.Kind, err)
	}
	thumb, err := png.Decode(bytes.NewReader(fromDIB.Thumb))
	if err != nil || thumb.Bounds().Dx() != 320 || thumb.Bounds().Dy() != 200 {
		t.Errorf("thumbnail %v, %v", thumb.Bounds(), err)
	}
	fromPNG, err := Snapshot{PNG: pngOf(pic)}.Item()
	if err != nil || !bytes.Equal(fromPNG.Hash, fromDIB.Hash) {
		t.Errorf("the same picture as PNG and as DIB hash differently (%v)", err)
	}

	files, err := Snapshot{Files: []string{`C:\a.txt`, `D:\b c\d.png`}}.Item()
	if err != nil || files.Kind != Files || !slices.Equal(files.Paths(), []string{`C:\a.txt`, `D:\b c\d.png`}) {
		t.Errorf("files %+v, %v", files, err)
	}

	for _, s := range []Snapshot{{}, {Text: " \r\n\t"}, {Private: true, Text: "password"}} {
		if _, err := s.Item(); !errors.Is(err, ErrSkip) {
			t.Errorf("%+v kept (%v)", s, err)
		}
	}
}

func TestPreview(t *testing.T) {
	got := Preview("\r\n  first   line\r\n\r\n\tsecond\tline \r\nthird\r\n", 2)
	if !slices.Equal(got, []string{"first line", "second line"}) {
		t.Errorf("Preview = %q", got)
	}
}

func openStore(t *testing.T, limit int) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "c.db"), limit)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func textItem(s string) Item {
	it, _ := Snapshot{Text: s}.Item()
	return it
}

func listIDs(t *testing.T, s *Store, tab int64, query string) []int64 {
	t.Helper()
	items, err := s.List(context.Background(), tab, query, 100)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	return ids
}

func record(t *testing.T, s *Store, it Item) int64 {
	t.Helper()
	id, err := s.Record(context.Background(), it)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRecordDedupesAndTrims(t *testing.T) {
	s := openStore(t, 3)
	ctx := context.Background()
	a := record(t, s, textItem("alpha"))
	b := record(t, s, textItem("bravo"))
	again := textItem("alpha")
	again.HTML = []byte("<i>alpha</i>")
	if id := record(t, s, again); id != a {
		t.Fatalf("copying alpha again made item %d, want %d", id, a)
	}
	if got := listIDs(t, s, DefaultTab, ""); !slices.Equal(got, []int64{a, b}) {
		t.Fatalf("order %v, want alpha moved to the top", got)
	}
	if it, _ := s.Get(ctx, a); string(it.HTML) != "<i>alpha</i>" {
		t.Errorf("formats not refreshed: %q", it.HTML)
	}

	// Pinned items do not count towards the limit and are never dropped.
	if err := s.SetPinned(ctx, b, true); err != nil {
		t.Fatal(err)
	}
	c := record(t, s, textItem("charlie"))
	d := record(t, s, textItem("delta"))
	e := record(t, s, textItem("echo"))
	if got := listIDs(t, s, DefaultTab, ""); !slices.Equal(got, []int64{b, e, d, c}) {
		t.Errorf("after trimming %v, want pinned bravo then echo delta charlie", got)
	}
}

func TestSearchEditAndTabs(t *testing.T) {
	s := openStore(t, 0)
	ctx := context.Background()
	hello := record(t, s, textItem("Hello, 世界"))
	weather := record(t, s, textItem("天气预报：明天晴"))
	if got := listIDs(t, s, DefaultTab, "hello"); !slices.Equal(got, []int64{hello}) {
		t.Errorf("search hello: %v", got)
	}
	if got := listIDs(t, s, DefaultTab, "天气"); !slices.Equal(got, []int64{weather}) {
		t.Errorf("search 天气: %v", got)
	}

	rich := textItem("rich")
	rich.HTML, rich.RTF = []byte("<b>rich</b>"), []byte(`{\rtf1 rich}`)
	id := record(t, s, rich)
	if err := s.SetText(ctx, id, "edited text"); err != nil {
		t.Fatal(err)
	}
	if it, _ := s.Get(ctx, id); it.Text != "edited text" || it.HTML != nil || it.RTF != nil {
		t.Errorf("after edit %+v", it)
	}
	if got := listIDs(t, s, DefaultTab, "edited"); !slices.Equal(got, []int64{id}) {
		t.Errorf("edited text not searchable: %v", got)
	}

	work, err := s.AddTab(ctx, "工作")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddTab(ctx, " 工作 "); !errors.Is(err, ErrTabExists) {
		t.Errorf("duplicate tab name: %v", err)
	}
	if err := s.MoveTo(ctx, hello, work.ID); err != nil {
		t.Fatal(err)
	}
	copied, err := s.CopyTo(ctx, weather, work.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := listIDs(t, s, work.ID, ""); !slices.Equal(got, []int64{copied, hello}) {
		t.Errorf("work tab %v", got)
	}
	if got := listIDs(t, s, DefaultTab, ""); slices.Contains(got, hello) || !slices.Contains(got, weather) {
		t.Errorf("default tab after move and copy %v", got)
	}
	if err := s.RenameTab(ctx, work.ID, "常用"); err != nil {
		t.Fatal(err)
	}
	tabs, _ := s.Tabs(ctx)
	if len(tabs) != 2 || tabs[0].ID != DefaultTab || tabs[0].Name != "剪贴板" || tabs[1].Name != "常用" {
		t.Errorf("tabs %+v", tabs)
	}
	if err := s.DeleteTab(ctx, DefaultTab); err == nil {
		t.Error("deleted the default tab")
	}
	if err := s.DeleteTab(ctx, work.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, hello); !errors.Is(err, ErrNotFound) {
		t.Errorf("items of a deleted tab survive: %v", err)
	}
}

func TestImageRoundTrip(t *testing.T) {
	s := openStore(t, 0)
	ctx := context.Background()
	pic, _ := Snapshot{DIB: winclip.EncodeDIB(picture(50, 30))}.Item()
	id := record(t, s, pic)
	items, _ := s.List(ctx, DefaultTab, "", 10)
	if len(items) != 1 || items[0].Kind != Image || !items[0].HasThumb || items[0].Width != 50 {
		t.Fatalf("listed %+v", items)
	}
	it, err := s.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	formats, err := Formats(it, false)
	if err != nil {
		t.Fatal(err)
	}
	var dib []byte
	for _, f := range formats {
		if f.Format == winclip.CFDIB {
			dib = f.Data
		}
	}
	back, err := winclip.DecodeDIB(dib)
	if err != nil || !bytes.Equal(back.Pix, picture(50, 30).Pix) {
		t.Errorf("pasted picture differs (%v)", err)
	}
}
