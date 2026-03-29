package modules

import (
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func NewPowerProfile() gtk.Widgetter {
	module := newTextModule("power-profiles-daemon")
	startPolling(10*time.Second, func() {
		output, err := runCommand("powerprofilesctl", "get")
		if err != nil {
			ui(func() { setTextModule(module, "") })
			return
		}

		profile := strings.TrimSpace(string(output))
		text := ""
		switch profile {
		case "performance":
			text = ""
		case "balanced":
			text = ""
		case "power-saver":
			text = ""
		}

		ui(func() {
			setTextModule(module, text)
			for _, class := range []string{"performance", "balanced", "power-saver"} {
				module.Box.RemoveCSSClass(class)
			}
			if profile != "" {
				module.Box.AddCSSClass(profile)
			}
			module.Box.SetTooltipText(profile)
		})
	})
	return module.Box
}
