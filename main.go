package main

import (
	"flag"
	"os"

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
		window := app.New(application, defaultCSS, *cssPath)
		window.Present()
	})

	os.Exit(application.Run(os.Args))
}
