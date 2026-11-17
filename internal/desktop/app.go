// Package desktop implements the huan desktop app shell (MyGo-based).
// Phase 1: a webview hosting the embedded admin SPA plus native tray/menu.
package desktop

import (
	"fmt"

	"github.com/egoist/mygo"
)

// Run starts the huan desktop app and blocks until the app quits.
func Run() error {
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:  "huan",
			Width:  1280,
			Height: 800,
		})
		// Spike: prove the webview can load a running HTTP origin.
		if err := win.Page().LoadURL("https://example.com"); err != nil {
			mygo.Dialog.Error("huan", fmt.Sprintf("load url: %v", err))
		}
	})
	return mygo.App.Run()
}
