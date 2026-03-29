package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func NewBattery(device, name string) gtk.Widgetter {
	module := newTextModule(name)
	startPolling(10*time.Second, func() {
		text, state := readBattery(device)
		ui(func() {
			setTextModule(module, text)
			for _, class := range []string{"charging", "plugged", "critical"} {
				module.Box.RemoveCSSClass(class)
			}
			if state != "" {
				module.Box.AddCSSClass(state)
			}
		})
	})
	return module.Box
}

func readBattery(device string) (string, string) {
	base := filepath.Join("/sys/class/power_supply", device)
	if _, err := os.Stat(base); err != nil {
		return "", ""
	}

	capacity, err := strconv.Atoi(readFirstExisting(filepath.Join(base, "capacity")))
	if err != nil {
		return "", ""
	}
	status := strings.ToLower(readFirstExisting(filepath.Join(base, "status")))

	icon := ""
	switch {
	case capacity < 20:
		icon = ""
	case capacity < 40:
		icon = ""
	case capacity < 70:
		icon = ""
	case capacity < 90:
		icon = ""
	default:
		icon = ""
	}

	state := ""
	if strings.Contains(status, "charging") {
		state = "charging"
		icon = "󱐋"
	}
	if strings.Contains(status, "full") || strings.Contains(status, "not charging") {
		state = "plugged"
		icon = "󱐋"
	}
	if capacity <= 15 && state == "" {
		state = "critical"
	}

	return fmt.Sprintf("%d%% %s", capacity, icon), state
}
