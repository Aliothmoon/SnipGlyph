package history

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func open(t *testing.T, limit int) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "h.db"), limit)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func add(t *testing.T, s *Store, src, tr string) int64 {
	t.Helper()
	id, err := s.Add(context.Background(), Entry{Kind: Selection, Source: src, Translation: tr, Engine: "百度翻译"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func ids(es []Entry) []int64 {
	var out []int64
	for _, e := range es {
		out = append(out, e.ID)
	}
	return out
}

func TestSearch(t *testing.T) {
	s := open(t, 0)
	ctx := context.Background()
	hello := add(t, s, "Hello, World", "你好，世界")
	weather := add(t, s, "The weather is lovely today.", "今天天气很好。")
	ocr := add(t, s, "识别出的中文文本：光学字符识别", "")
	pct := add(t, s, "100% sure_thing", "")

	for _, tc := range []struct {
		query string
		want  []int64
	}{
		{"", []int64{pct, ocr, weather, hello}},
		{"world", []int64{hello}},           // trigram, case-insensitive
		{"WEATHER today", []int64{weather}}, // every term must match
		{"天气", []int64{weather}},            // two characters: LIKE fallback
		{"光学字符", []int64{ocr}},              // Chinese substring through the index
		{"世", []int64{hello}},               // one character
		{"lovely 世界", nil},                  // terms in different entries
		{"%", []int64{pct}},                 // LIKE wildcards are literal
		{"_t", []int64{pct}},
		{`"quoted"`, nil}, // quotes do not break the FTS query
		{"   ", []int64{pct, ocr, weather, hello}},
	} {
		got, err := s.Search(ctx, tc.query, 100)
		if err != nil {
			t.Fatalf("Search(%q): %v", tc.query, err)
		}
		if !slices.Equal(ids(got), tc.want) {
			t.Errorf("Search(%q) = %v, want %v", tc.query, ids(got), tc.want)
		}
	}

	got, _ := s.Search(ctx, "", 2)
	if !slices.Equal(ids(got), []int64{pct, ocr}) {
		t.Errorf("max 2 = %v", ids(got))
	}
	e := got[1]
	if e.Kind != Selection || e.Engine != "百度翻译" || e.Source != "识别出的中文文本：光学字符识别" || time.Since(e.Time) > time.Minute {
		t.Errorf("entry = %+v", e)
	}
}

func TestUpdateReindexes(t *testing.T) {
	s := open(t, 0)
	ctx := context.Background()
	id := add(t, s, "draft text", "")
	if err := s.Update(ctx, Entry{ID: id, Source: "final text", Translation: "最终文本", Engine: "百度翻译"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Search(ctx, "draft", 10); len(got) != 0 {
		t.Errorf("old text still found: %v", ids(got))
	}
	got, _ := s.Search(ctx, "最终文本", 10)
	if !slices.Equal(ids(got), []int64{id}) || got[0].Kind != Selection {
		t.Errorf("updated entry: %+v", got)
	}
	if err := s.Update(ctx, Entry{ID: id + 100, Source: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update of a missing entry: %v", err)
	}
}

func TestLimitDropsOldest(t *testing.T) {
	s := open(t, 3)
	ctx := context.Background()
	var all []int64
	for _, src := range []string{"one entry", "two entry", "three entry", "four entry", "five entry"} {
		all = append(all, add(t, s, src, ""))
	}
	if n, _ := s.Count(ctx); n != 3 {
		t.Fatalf("count = %d, want 3", n)
	}
	got, _ := s.Search(ctx, "entry", 10)
	if want := []int64{all[4], all[3], all[2]}; !slices.Equal(ids(got), want) {
		t.Errorf("kept %v, want %v", ids(got), want)
	}
}

func TestDeleteAndClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.db")
	s, err := Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	a := add(t, s, "keep this one", "")
	b := add(t, s, "delete this one", "")
	if err := s.Delete(ctx, b); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Search(ctx, "this one", 10); !slices.Equal(ids(got), []int64{a}) {
		t.Errorf("after delete: %v", ids(got))
	}
	if err := s.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Count(ctx); n != 0 {
		t.Errorf("count after clear = %d", n)
	}
	// Ids keep growing after a clear, and the database reopens.
	c := add(t, s, "after clear", "")
	if c <= b {
		t.Errorf("id %d reused (last was %d)", c, b)
	}
	s.Close()
	s, err = Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got, _ := s.Search(ctx, "after", 10); !slices.Equal(ids(got), []int64{c}) {
		t.Errorf("after reopen: %v", ids(got))
	}
}
