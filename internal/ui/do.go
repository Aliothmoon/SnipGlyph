package ui

import "github.com/tailscale/walk"

// Do runs f on the UI thread and waits for it to return. Called on the UI
// thread it runs f directly.
func Do(f func()) {
	app := walk.App()
	if app.IsUIThread() {
		f()
		return
	}
	done := make(chan struct{})
	app.Synchronize(func() {
		defer close(done)
		f()
	})
	<-done
}
