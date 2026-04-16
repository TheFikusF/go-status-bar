package modules

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"statusbar/internal/config"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type batteryInfo struct {
	Capacity   int
	IconName   string
	State      string
	Status     string
	Device     string
	EnergyNow  float64
	EnergyFull float64
	PowerNow   float64
}

func NewBattery(cfg *config.Config, device, name string) gtk.Widgetter {
	showText := cfg.Battery.ShowText

	module := newTextModule(name)
	removeChildren(module.Box)
	module.Box.SetVisible(true)

	iconTextBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	battIcon := gtk.NewImageFromIconName("battery-full-symbolic")
	percentLabel := gtk.NewLabel("")
	percentLabel.SetXAlign(0)
	iconTextBox.Append(percentLabel)
	iconTextBox.Append(battIcon)
	module.Box.Append(iconTextBox)

	popover := gtk.NewPopover()
	popover.AddCSSClass("status-popup")
	popover.SetHasArrow(false)
	popover.SetAutohide(true)
	popover.SetParent(module.Box)

	popoverBox := gtk.NewBox(gtk.OrientationVertical, 4)
	popoverBox.SetName("battery-popup")
	popover.SetChild(popoverBox)

	deviceLabel := gtk.NewLabel("")
	deviceLabel.AddCSSClass("battery-popup-device")
	deviceLabel.SetXAlign(0)

	percentPopupLabel := gtk.NewLabel("")
	percentPopupLabel.AddCSSClass("battery-popup-percent")
	percentPopupLabel.SetXAlign(0)

	statusLabel := gtk.NewLabel("")
	statusLabel.AddCSSClass("battery-popup-status")
	statusLabel.SetXAlign(0)

	timeLabel := gtk.NewLabel("")
	timeLabel.AddCSSClass("battery-popup-time")
	timeLabel.SetXAlign(0)

	popoverBox.Append(deviceLabel)
	popoverBox.Append(percentPopupLabel)
	popoverBox.Append(statusLabel)
	popoverBox.Append(timeLabel)

	popupOpen := false
	popover.ConnectClosed(func() { popupOpen = false })

	var lastInfo batteryInfo

	updatePopup := func(info batteryInfo) {
		deviceLabel.SetLabel(fmt.Sprintf("Device: %s", info.Device))
		percentPopupLabel.SetLabel(fmt.Sprintf("Capacity: %d%%", info.Capacity))

		displayStatus := info.Status
		if displayStatus == "" {
			displayStatus = "Unknown"
		} else {
			displayStatus = strings.ToUpper(displayStatus[:1]) + displayStatus[1:]
		}
		statusLabel.SetLabel(fmt.Sprintf("Status: %s", displayStatus))

		timeStr := estimateTime(info)
		if timeStr != "" {
			timeLabel.SetLabel(timeStr)
			timeLabel.SetVisible(true)
		} else {
			timeLabel.SetVisible(false)
		}
	}

	attachHoverPopover(module.Box, popover, nil, func() {
		popupOpen = true
		updatePopup(lastInfo)
	})

	startPolling(10*time.Second, func() {
		info := readBatteryInfo(device)
		ui(func() {
			if info.Capacity < 0 {
				module.Box.SetVisible(false)
				return
			}
			module.Box.SetVisible(true)
			battIcon.SetFromIconName(info.IconName)
			if showText {
				percentLabel.SetLabel(fmt.Sprintf("%d%%", info.Capacity))
			} else {
				percentLabel.SetLabel("")
			}
			for _, class := range []string{"charging", "plugged", "critical"} {
				module.Box.RemoveCSSClass(class)
			}
			if info.State != "" {
				module.Box.AddCSSClass(info.State)
			}
			lastInfo = info
			if popupOpen {
				updatePopup(info)
			}
		})
	})
	return module.Box
}

func readBatteryInfo(device string) batteryInfo {
	base := filepath.Join("/sys/class/power_supply", device)
	if _, err := os.Stat(base); err != nil {
		return batteryInfo{Capacity: -1, Device: device}
	}

	capacity, err := strconv.Atoi(readFirstExisting(filepath.Join(base, "capacity")))
	if err != nil {
		return batteryInfo{Capacity: -1, Device: device}
	}
	status := strings.ToLower(readFirstExisting(filepath.Join(base, "status")))

	energyNow := parseFloat(readFirstExisting(filepath.Join(base, "energy_now")))
	energyFull := parseFloat(readFirstExisting(filepath.Join(base, "energy_full")))
	powerNow := parseFloat(readFirstExisting(filepath.Join(base, "power_now")))

	iconName := "battery-full-symbolic"
	switch {
	case capacity < 10:
		iconName = "battery-empty-symbolic"
	case capacity < 30:
		iconName = "battery-caution-symbolic"
	case capacity < 55:
		iconName = "battery-low-symbolic"
	case capacity < 80:
		iconName = "battery-good-symbolic"
	default:
		iconName = "battery-full-symbolic"
	}

	state := ""
	if status == "charging" {
		state = "charging"
		iconName = "battery-full-charging-symbolic"
		switch {
		case capacity < 10:
			iconName = "battery-empty-charging-symbolic"
		case capacity < 30:
			iconName = "battery-caution-charging-symbolic"
		case capacity < 55:
			iconName = "battery-low-charging-symbolic"
		case capacity < 80:
			iconName = "battery-good-charging-symbolic"
		}
	}
	if status == "full" || status == "not charging" {
		state = "plugged"
		iconName = "battery-full-charged-symbolic"
	}
	if capacity <= 15 && state == "" {
		state = "critical"
	}

	return batteryInfo{
		Capacity:   capacity,
		IconName:   iconName,
		State:      state,
		Status:     status,
		Device:     device,
		EnergyNow:  energyNow,
		EnergyFull: energyFull,
		PowerNow:   powerNow,
	}
}

func estimateTime(info batteryInfo) string {
	if info.PowerNow <= 0 {
		return ""
	}
	switch info.Status {
	case "discharging":
		hours := info.EnergyNow / info.PowerNow
		return formatDuration(hours, "till empty")
	case "charging":
		remaining := info.EnergyFull - info.EnergyNow
		if remaining <= 0 {
			return ""
		}
		hours := remaining / info.PowerNow
		return formatDuration(hours, "till full")
	}
	return ""
}

func formatDuration(hours float64, suffix string) string {
	if hours <= 0 || math.IsInf(hours, 0) || math.IsNaN(hours) {
		return ""
	}
	totalMin := int(math.Round(hours * 60))
	h := totalMin / 60
	m := totalMin % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %s", h, m, suffix)
	}
	return fmt.Sprintf("%dm %s", m, suffix)
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
