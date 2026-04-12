package main

import (
	"flag"
	"os"
	"sync"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"statusbar/internal/app"
)

const appID = "dev.fikus.statusbar"

func main() {
	cssPath := flag.String("css", "", "path to a CSS file to load instead of the default")
	flag.Parse()
	// Rebuild os.Args with only the program name and non-flag remainder so GTK
	// does not choke on our custom --css flag.
	os.Args = append(os.Args[:1], flag.Args()...)

	application := gtk.NewApplication(appID, gio.ApplicationFlagsNone)
	application.ConnectActivate(func() {
		display := gdk.DisplayGetDefault()
		if display == nil {
			window := app.New(application, defaultCSS, *cssPath, nil, "")
			window.Present()
			return
		}

		var mu sync.Mutex
		windows := map[string]*gtk.ApplicationWindow{}

		spawnBar := func(mon *gdk.Monitor) {
			name := mon.Connector()
			mu.Lock()
			defer mu.Unlock()
			if _, exists := windows[name]; exists {
				return
			}
			window := app.New(application, defaultCSS, *cssPath, mon, name)
			windows[name] = window
			window.Present()
		}

		destroyBar := func(name string) {
			mu.Lock()
			window, ok := windows[name]
			if ok {
				delete(windows, name)
			}
			mu.Unlock()
			if ok {
				window.Destroy()
			}
		}

		monitors := display.Monitors()

		// Spawn a bar for every currently connected monitor.
		for i := uint(0); i < monitors.NItems(); i++ {
			mon := monitors.Item(i).Cast().(*gdk.Monitor)
			spawnBar(mon)
		}

		// Watch for future connect/disconnect events.
		monitors.ConnectItemsChanged(func(position, removed, added uint) {
			// First destroy bars for removed monitors.
			// We don't know the connector at removal time, so reconcile
			// by diffing the current connector set against tracked windows.
			current := map[string]*gdk.Monitor{}
			for i := uint(0); i < monitors.NItems(); i++ {
				mon := monitors.Item(i).Cast().(*gdk.Monitor)
				current[mon.Connector()] = mon
			}

			mu.Lock()
			var gone []string
			for name := range windows {
				if _, ok := current[name]; !ok {
					gone = append(gone, name)
				}
			}
			mu.Unlock()

			for _, name := range gone {
				destroyBar(name)
			}

			// Spawn bars for newly added monitors.
			for _, mon := range current {
				spawnBar(mon)
			}
		})
	})

	os.Exit(application.Run(os.Args))
}
