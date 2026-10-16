// Package netx builds HTTP clients that honour SnipGlyph's per-engine proxy
// setting, including the Windows system proxy that Go ignores by default.
package netx

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
)

// Proxy settings understood by Client.
const (
	System = ""       // the Windows (WinINET) proxy: Settings > Network > Proxy, or set by Clash etc.
	Direct = "direct" // no proxy
)

// Client returns an HTTP client for a proxy setting: System, Direct, or a
// proxy URL such as "http://127.0.0.1:7890" or "socks5://127.0.0.1:1080".
// Callers bound requests with contexts; the client sets no timeout itself,
// so streamed responses are not cut off.
func Client(setting string) (*http.Client, error) {
	t := http.DefaultTransport.(*http.Transport).Clone()
	switch s := strings.TrimSpace(setting); strings.ToLower(s) {
	case System, "system":
		t.Proxy = func(r *http.Request) (*url.URL, error) { return systemProxy(r.URL) }
	case Direct:
		t.Proxy = nil
	default:
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			return nil, fmt.Errorf("代理地址 %q 无效，应形如 http://127.0.0.1:7890 或 socks5://127.0.0.1:1080", s)
		}
		switch u.Scheme {
		case "http", "https", "socks5", "socks5h":
		default:
			return nil, fmt.Errorf("不支持的代理协议 %q", u.Scheme)
		}
		t.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Transport: t}, nil
}

// winProxy is the WinINET proxy configuration of the current user.
type winProxy struct {
	enabled  bool
	server   string // "host:port" or "http=h:p;https=h:p;socks=h:p"
	override string // "localhost;*.corp;<local>"
	pac      string // AutoConfigURL, which we cannot evaluate
}

var warnPACOnce sync.Once

func systemProxy(target *url.URL) (*url.URL, error) {
	cfg := readWinProxy()
	if !cfg.enabled || cfg.server == "" {
		if cfg.pac != "" {
			warnPACOnce.Do(func() {
				slog.Warn("system proxy uses a PAC script, which is not supported; connecting directly", "pac", cfg.pac)
			})
		}
		return nil, nil
	}
	if bypassed(target.Hostname(), cfg.override) {
		return nil, nil
	}
	return proxyFor(cfg.server, target.Scheme), nil
}

// proxyFor picks the proxy for scheme from a WinINET ProxyServer value.
func proxyFor(server, scheme string) *url.URL {
	if !strings.Contains(server, "=") {
		return hostURL("http", server)
	}
	byProto := map[string]string{}
	for _, part := range strings.Split(server, ";") {
		if k, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok && v != "" {
			byProto[strings.ToLower(k)] = v
		}
	}
	if addr := byProto[scheme]; addr != "" {
		return hostURL("http", addr)
	}
	if addr := byProto["socks"]; addr != "" {
		return hostURL("socks5", addr)
	}
	return nil
}

func hostURL(scheme, addr string) *url.URL {
	addr = strings.TrimSpace(addr)
	if strings.Contains(addr, "://") {
		if u, err := url.Parse(addr); err == nil {
			return u
		}
	}
	return &url.URL{Scheme: scheme, Host: addr}
}

// bypassed applies a WinINET ProxyOverride list to host. Loopback is always
// bypassed so a local engine (Ollama) keeps working behind a system proxy.
func bypassed(host, override string) bool {
	host = strings.ToLower(host)
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	for _, p := range strings.Split(override, ";") {
		p = strings.ToLower(strings.TrimSpace(p))
		switch {
		case p == "":
		case p == "<local>":
			if !strings.Contains(host, ".") {
				return true
			}
		default:
			if ok, _ := path.Match(p, host); ok {
				return true
			}
		}
	}
	return false
}
