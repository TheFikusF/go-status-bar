package app

import (
	"log"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"statusbar/internal/modules"
)

const userCSSRelativePath = ".config/status-bar/style.css"

func New(application *gtk.Application, defaultCSS string) *gtk.ApplicationWindow {
	loadCSS(defaultCSS)

	window := gtk.NewApplicationWindow(application)
	window.SetTitle("Status Bar")
	window.SetDecorated(false)
	window.SetDeletable(false)
	window.SetResizable(false)
	window.SetDefaultSize(1920, 34)
	window.SetName("status-bar")

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
		modules.NewClipboard(),
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

func loadCSS(defaultCSS string) {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}

	css := defaultCSS
	if path, ok := userCSSPath(); ok {
		if userCSS, err := os.ReadFile(path); err == nil {
			css = string(userCSS)
			log.Printf("loaded css override from %s", path)
		} else if !os.IsNotExist(err) {
			log.Printf("css override skipped: %v", err)
		}
	}
	if css == "" {
		log.Printf("css load skipped: embedded css is empty")
		return
	}

	provider := gtk.NewCSSProvider()
	provider.LoadFromString(css)
	gtk.StyleContextAddProviderForDisplay(display, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}

func userCSSPath() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	return filepath.Join(home, userCSSRelativePath), true
}
