package app

import (
	"log"
	"os"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"statusbar/internal/modules"
)

const cssPath = "styles/bar.css"

func New(application *gtk.Application) *gtk.ApplicationWindow {
	loadCSS()

	window := gtk.NewApplicationWindow(application)
	window.SetTitle("Waybar Clone")
	window.SetDecorated(false)
	window.SetDeletable(false)
	window.SetResizable(false)
	window.SetDefaultSize(1920, 34)
	window.SetName("waybar")

	initLayerShell(window)

	root := gtk.NewCenterBox()

	left := gtk.NewBox(gtk.OrientationHorizontal, 0)
	left.AddCSSClass("modules-left")

	center := gtk.NewBox(gtk.OrientationHorizontal, 0)
	center.AddCSSClass("modules-center")

	right := gtk.NewBox(gtk.OrientationHorizontal, 0)
	right.AddCSSClass("modules-right")

	root.SetStartWidget(left)
	root.SetCenterWidget(center)
	root.SetEndWidget(right)
	window.SetChild(root)

	for _, widget := range []gtk.Widgetter{
		modules.NewWorkspaces(),
		modules.NewMusic(),
		modules.NewMode(),
		modules.NewScratchpad(),
		// modules.NewMedia(),
	} {
		left.Append(widget)
	}

	for _, widget := range []gtk.Widgetter{
		modules.NewDateClock(),
		modules.NewTimeClock(),
		modules.NewNotification(),
	} {
		center.Append(widget)
	}

	for _, widget := range []gtk.Widgetter{
		modules.NewMPD(),
		modules.NewWallpaper(),
		modules.NewWeather(),
		modules.NewPipewire(),
		modules.NewNetwork(),
		// modules.NewPowerProfile(),
		modules.NewCPU(),
		modules.NewMemory(),
		modules.NewTemperature(),
		// modules.NewKeyboardState(),
		modules.NewLanguage(),
		modules.NewBattery("BAT0", "battery"),
		// modules.NewBattery("BAT1", "battery-bat2"),
		modules.NewTray(),
		modules.NewPower(),
	} {
		right.Append(widget)
	}

	return window
}

func loadCSS() {
	css, err := os.ReadFile(cssPath)
	if err != nil {
		log.Printf("css load skipped: %v", err)
		return
	}

	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}

	provider := gtk.NewCSSProvider()
	provider.LoadFromString(string(css))
	gtk.StyleContextAddProviderForDisplay(display, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}
