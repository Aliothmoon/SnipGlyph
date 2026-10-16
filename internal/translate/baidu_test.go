package translate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"SnipGlyph/internal/netx"
)

func TestBaiduSign(t *testing.T) {
	// Example from the Baidu API documentation.
	got := baiduSign("2015063000000001", "apple", "1435660288", "12345678")
	if got != "f89f9594663708c1605f3d736d01d2d4" {
		t.Fatalf("sign = %s", got)
	}
}

func TestBaiduRequestAndMultilineResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		f := r.PostForm
		if f.Get("q") != "Hello\nWorld" || f.Get("from") != "auto" || f.Get("to") != "zh" || f.Get("appid") != "id" {
			t.Errorf("form = %v", f)
		}
		if f.Get("sign") != baiduSign("id", "Hello\nWorld", f.Get("salt"), "secret") {
			t.Error("bad sign")
		}
		w.Write([]byte(`{"from":"en","to":"zh","trans_result":[{"src":"Hello","dst":"你好"},{"src":"World","dst":"世界"}]}`))
	}))
	defer srv.Close()

	b := &Baidu{AppID: "id", Key: "secret", Endpoint: srv.URL}
	var streamed string
	res, err := b.Translate(context.Background(), Request{Text: "Hello\nWorld", From: Auto, To: ChineseSimplified}, func(s string) { streamed = s })
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "你好\n世界" || res.From != English {
		t.Fatalf("got %+v", res)
	}
	if streamed != res.Text {
		t.Errorf("partial got %q", streamed)
	}
}

func TestBaiduErrors(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"54001","error_msg":"Invalid Sign"}`,
		`{"error_code":54001,"error_msg":"Invalid Sign"}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(body))
		}))
		b := &Baidu{AppID: "id", Key: "k", Endpoint: srv.URL}
		_, err := b.Translate(context.Background(), Request{Text: "x", From: Auto, To: English}, nil)
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), "签名错误") {
			t.Errorf("body %s: err = %v", body, err)
		}
	}

	b := &Baidu{AppID: "id", Key: "k"}
	if _, err := b.Translate(context.Background(), Request{Text: strings.Repeat("a", baiduMaxBytes+1), From: Auto, To: English}, nil); err == nil {
		t.Error("want error for oversized text")
	}
	if _, err := (&Baidu{}).Translate(context.Background(), Request{Text: "x", From: Auto, To: English}, nil); err == nil {
		t.Error("want error without credentials")
	}
}

// TestBaiduLive calls the real API when credentials are in the environment.
func TestBaiduLive(t *testing.T) {
	id, key := os.Getenv("SNIPGLYPH_BAIDU_APPID"), os.Getenv("SNIPGLYPH_BAIDU_KEY")
	if id == "" || key == "" {
		t.Skip("set SNIPGLYPH_BAIDU_APPID and SNIPGLYPH_BAIDU_KEY to call the real API")
	}
	hc, err := netx.Client(netx.System) // same path as the app
	if err != nil {
		t.Fatal(err)
	}
	b := &Baidu{AppID: id, Key: key, HTTP: hc}
	ctx := context.Background()

	text := "The quick brown fox jumps over the lazy dog.\nSecond line."
	from, to := Direction(text, ChineseSimplified)
	res, err := b.Translate(ctx, Request{Text: text, From: from, To: to}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("en->zh: %q (detected %s)", res.Text, res.From)
	if Detect(res.Text) != ChineseSimplified || strings.Count(res.Text, "\n") != 1 {
		t.Errorf("unexpected translation %q", res.Text)
	}

	text = "这是一段用于测试划词翻译的中文文本。"
	from, to = Direction(text, ChineseSimplified)
	res, err = b.Translate(ctx, Request{Text: text, From: from, To: to}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("zh->en: %q", res.Text)
	if Detect(res.Text) != Auto {
		t.Errorf("expected English, got %q", res.Text)
	}
}
