package main

import (
	"os"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"statusbar/internal/app"
)

const appID = "dev.fikus.statusbar"

func main() {
	application := gtk.NewApplication(appID, gio.ApplicationFlagsNone)
	application.ConnectActivate(func() {
		window := app.New(application)
		window.Present()
	})

	os.Exit(application.Run(os.Args))
}
