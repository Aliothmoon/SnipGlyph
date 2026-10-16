//go:build debug

package main

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
)

// Debug builds (go build -tags debug) serve pprof on SNIPGLYPH_PPROF, e.g.
// 127.0.0.1:6060, to inspect hangs: /debug/pprof/goroutine?debug=2.
func init() {
	if addr := os.Getenv("SNIPGLYPH_PPROF"); addr != "" && !(len(os.Args) > 1 && os.Args[1] == workerFlag) {
		go func() { slog.Warn("pprof server stopped", "err", http.ListenAndServe(addr, nil)) }()
	}
}
