package app

import (
	"log"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"statusbar/internal/config"
	"statusbar/internal/modules"
)

const userCSSRelativePath = ".config/status-bar/style.css"

func New(application *gtk.Application, defaultCSS string, cssPath string) *gtk.ApplicationWindow {
	loadCSS(defaultCSS, cssPath)
	cfg := config.Load()

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

	appendIf := func(box *gtk.Box, enabled bool, w gtk.Widgetter) {
		if enabled {
			box.Append(w)
		}
	}

	appendIf(left, cfg.Modules.Workspaces, modules.NewWorkspaces())
	appendIf(left, cfg.Modules.FocusedApp, modules.NewFocusedApp(window))
	appendIf(left, cfg.Modules.Music, modules.NewMusic())
	appendIf(left, cfg.Modules.Mode, modules.NewMode())
	appendIf(left, cfg.Modules.Scratchpad, modules.NewScratchpad())

	appendIf(center, cfg.Modules.DateClock, modules.NewDateClock())
	appendIf(center, cfg.Modules.TimeClock, modules.NewTimeClock(cfg))
	appendIf(center, cfg.Modules.Notification, modules.NewNotification())

	appendIf(right, cfg.Modules.MPD, modules.NewMPD())
	appendIf(right, cfg.Modules.Wallpaper, modules.NewWallpaper(cfg))
	appendIf(right, cfg.Modules.Clipboard, modules.NewClipboard())
	appendIf(right, cfg.Modules.Weather, modules.NewWeather(cfg))
	appendIf(right, cfg.Modules.Pipewire, modules.NewPipewire())
	appendIf(right, cfg.Modules.Network, modules.NewNetwork())
	appendIf(right, cfg.Modules.PowerProfile, modules.NewPowerProfile())
	appendIf(right, cfg.Modules.CPU, modules.NewCPU())
	appendIf(right, cfg.Modules.Memory, modules.NewMemory())
	appendIf(right, cfg.Modules.Temperature, modules.NewTemperature())
	appendIf(right, cfg.Modules.KeyboardState, modules.NewKeyboardState())
	appendIf(right, cfg.Modules.Language, modules.NewLanguage(cfg))
	if cfg.Modules.Battery {
		right.Append(modules.NewBattery("BAT0", "battery"))
	}
	appendIf(right, cfg.Modules.Tray, modules.NewTray())
	appendIf(right, cfg.Modules.Power, modules.NewPower())

	return window
}

func loadCSS(defaultCSS string, cssPath string) {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}

	css := defaultCSS
	if cssPath != "" {
		if data, err := os.ReadFile(cssPath); err == nil {
			css = string(data)
			log.Printf("loaded css from flag: %s", cssPath)
		} else {
			log.Printf("--css flag path unreadable, falling back: %v", err)
		}
	} else if path, ok := userCSSPath(); ok {
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
