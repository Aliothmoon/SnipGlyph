package netx

import (
	"net/http"
	"testing"
)

func TestProxyFor(t *testing.T) {
	tests := []struct {
		server, scheme, want string
	}{
		{"127.0.0.1:7890", "https", "http://127.0.0.1:7890"},
		{"http=127.0.0.1:8080;https=127.0.0.1:8443", "https", "http://127.0.0.1:8443"},
		{"http=127.0.0.1:8080;https=127.0.0.1:8443", "http", "http://127.0.0.1:8080"},
		{"socks=127.0.0.1:1080", "https", "socks5://127.0.0.1:1080"},
		{"http=127.0.0.1:8080", "https", ""},
		{"http://proxy.corp:3128", "https", "http://proxy.corp:3128"},
	}
	for _, tt := range tests {
		got := ""
		if u := proxyFor(tt.server, tt.scheme); u != nil {
			got = u.String()
		}
		if got != tt.want {
			t.Errorf("proxyFor(%q, %q) = %q, want %q", tt.server, tt.scheme, got, tt.want)
		}
	}
}

func TestBypassed(t *testing.T) {
	const override = "*.corp.example;10.*;<local>"
	tests := map[string]bool{
		"api.corp.example":     true,
		"10.1.2.3":             true,
		"intranet":             true, // <local>: no dot
		"localhost":            true,
		"127.0.0.1":            true,
		"::1":                  true,
		"fanyi-api.baidu.com":  false,
		"api.openai.com":       false,
		"corp.example.evil.io": false,
	}
	for host, want := range tests {
		if got := bypassed(host, override); got != want {
			t.Errorf("bypassed(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestClientSettings(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://fanyi-api.baidu.com/", nil)

	c, err := Client(Direct)
	if err != nil {
		t.Fatal(err)
	}
	if c.Transport.(*http.Transport).Proxy != nil {
		t.Error("direct client has a proxy")
	}

	c, err = Client("socks5://127.0.0.1:1080")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := c.Transport.(*http.Transport).Proxy(req)
	if u == nil || u.String() != "socks5://127.0.0.1:1080" {
		t.Errorf("explicit proxy = %v", u)
	}

	for _, bad := range []string{"127.0.0.1:7890", "ftp://x:21", "http://"} {
		if _, err := Client(bad); err == nil {
			t.Errorf("Client(%q) accepted", bad)
		}
	}

	// The system setting must at least evaluate without error on this machine.
	c, err = Client(System)
	if err != nil {
		t.Fatal(err)
	}
	u, err = c.Transport.(*http.Transport).Proxy(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("system proxy for %s: %v", req.URL.Host, u)
}
