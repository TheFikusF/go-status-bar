package modules

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"statusbar/internal/config"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type hyprDevices struct {
	Keyboards []struct {
		Name         string `json:"name"`
		ActiveKeymap string `json:"active_keymap"`
	} `json:"keyboards"`
}

func NewKeyboardState() gtk.Widgetter {
	module := newTextModule("keyboard-state")
	startPolling(2*time.Second, func() {
		num := lockState("numlock")
		caps := lockState("capslock")
		ui(func() { setTextModule(module, "NUM "+lockIcon(num)+" CAPS "+lockIcon(caps)) })
	})
	return module.Box
}

func lockState(name string) string {
	matches, _ := filepath.Glob("/sys/class/leds/*::" + name + "/brightness")
	for _, match := range matches {
		if strings.TrimSpace(readFirstExisting(match)) == "1" {
			return "LOCK"
		}
	}
	return "OPEN"
}

func lockIcon(state string) string {
	if state == "LOCK" {
		return ""
	}
	return ""
}

func NewLanguage(cfg *config.Config) gtk.Widgetter {
	module := newTextModule("language")
	module.Box.SetHExpand(false)
	module.Label.SetHExpand(true)
	module.Label.SetHAlign(gtk.AlignFill)
	module.Label.SetXAlign(0.5)
	keyboardName := "at-translated-set-2-keyboard"

	popover := gtk.NewPopover()
	popover.AddCSSClass("status-popup")
	popover.SetHasArrow(false)
	popover.SetAutohide(true)
	popover.SetParent(module.Box)

	menu := gtk.NewBox(gtk.OrientationVertical, 4)
	menu.SetName("language-menu")
	popover.SetChild(menu)

	var currentLayout string

	fmtLayout := func(layout string) string {
		if len(cfg.Languages) > 0 {
			lower := strings.ToLower(strings.TrimSpace(layout))
			for _, entry := range cfg.Languages {
				if strings.Contains(lower, strings.ToLower(entry.Match)) {
					return entry.Label
				}
			}
			return strings.ToUpper(firstN(lower, 3))
		}
		return formatLanguage(layout)
	}

	var updatePopover func()
	updatePopover = func() {
		removeChildren(menu)
		for i, entry := range cfg.Languages {
			idx := strconv.Itoa(i)
			match := strings.ToLower(entry.Match)
			active := currentLayout != "" && strings.Contains(strings.ToLower(currentLayout), match)
			label := entry.Label
			if active {
				label = "· " + entry.Label
			}
			row := gtk.NewButtonWithLabel(label)
			row.SetHasFrame(false)
			row.AddCSSClass("language-row")
			if active {
				row.AddCSSClass("active")
			}
			row.ConnectClicked(func() {
				runDetached("hyprctl", "switchxkblayout", keyboardName, idx)
				time.AfterFunc(100*time.Millisecond, func() {
					ui(func() { updatePopover() })
				})
			})
			menu.Append(row)
		}
	}

	attachHoverPopover(module.Box, popover, nil, updatePopover)
	attachClick(module.Box, func() {
		runDetached("hyprctl", "switchxkblayout", keyboardName, "next")
	}, nil)

	refresh := func() {
		output, err := runCommand("hyprctl", "devices", "-j")
		if err != nil {
			ui(func() { setTextModule(module, "") })
			return
		}

		var devices hyprDevices
		if err := json.Unmarshal(output, &devices); err != nil {
			return
		}

		layout := ""
		for _, keyboard := range devices.Keyboards {
			if keyboard.Name != "" && keyboardName == "at-translated-set-2-keyboard" {
				keyboardName = keyboard.Name
			}
			if keyboard.ActiveKeymap != "" {
				layout = keyboard.ActiveKeymap
				break
			}
		}

		ui(func() {
			currentLayout = layout
			setTextModule(module, fmtLayout(layout))
		})
	}

	refresh()
	go func() {
		for event := range subscribeHyprEvents() {
			if event.Name == "activelayout" {
				if _, layout, ok := strings.Cut(event.Data, ","); ok {
					ui(func() {
						currentLayout = layout
						setTextModule(module, fmtLayout(layout))
					})
					continue
				}
				refresh()
			}
		}
	}()

	return module.Box
}

func formatLanguage(layout string) string {
	layout = strings.ToLower(strings.TrimSpace(layout))
	switch {
	case strings.Contains(layout, "english"), strings.Contains(layout, "en"):
		return "🇺🇸🦅🗽"
	case strings.Contains(layout, "ukrain"), strings.Contains(layout, "ua"), strings.Contains(layout, "uk"):
		return "🇺🇦 УКР"
	case strings.Contains(layout, "russian"), strings.Contains(layout, "ru"):
		return "🛡 РДК"
	default:
		return strings.ToUpper(firstN(layout, 3))
	}
}

func NewMode() gtk.Widgetter {
	module := newTextModule("mode")
	startPolling(2*time.Second, func() {
		output, err := runCommand("hyprctl", "submap")
		if err != nil {
			ui(func() { setTextModule(module, "") })
			return
		}

		value := strings.TrimSpace(string(output))
		if value == "" || value == "default" {
			value = ""
		}
		ui(func() { setTextModule(module, value) })
	})
	return module.Box
}

func NewScratchpad() gtk.Widgetter {
	module := newTextModule("scratchpad")
	startPolling(2*time.Second, func() {
		output, err := runCommand("hyprctl", "-j", "clients")
		if err != nil {
			ui(func() { setTextModule(module, "") })
			return
		}

		var clients []struct {
			Workspace struct {
				Name string `json:"name"`
			} `json:"workspace"`
		}
		if err := json.Unmarshal(output, &clients); err != nil {
			return
		}

		count := 0
		for _, client := range clients {
			if strings.Contains(strings.ToLower(client.Workspace.Name), "special") {
				count++
			}
		}

		text := ""
		if count > 0 {
			text = "SCR " + strconv.Itoa(count)
		}
		ui(func() { setTextModule(module, text) })
	})
	return module.Box
}
