package modules

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type wifiNetwork struct {
	SSID     string
	Signal   int
	Security string
	Active   bool
}

type networkSnapshot struct {
	Text              string
	Disconnected      bool
	NetworkingEnabled bool
	Networks          []wifiNetwork
}

func NewNetwork() gtk.Widgetter {
	module := newTextModule("network")
	popover := gtk.NewPopover()
	popover.AddCSSClass("status-popup")
	popover.SetHasArrow(false)
	popover.SetAutohide(true)
	popover.SetParent(module.Box)

	menu := gtk.NewBox(gtk.OrientationVertical, 4)
	menu.SetName("wifi-menu")
	popover.SetChild(menu)

	titleRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	titleRow.AddCSSClass("wifi-title-row")
	title := gtk.NewLabel("Wi-Fi")
	title.SetName("wifi-menu-title")
	title.SetXAlign(0)
	title.SetHExpand(true)
	networkingSwitch := gtk.NewSwitch()
	titleRow.Append(title)
	titleRow.Append(networkingSwitch)
	menu.Append(titleRow)

	listBox := gtk.NewBox(gtk.OrientationVertical, 2)
	menu.Append(listBox)

	attachHoverPopover(module.Box, popover, nil, nil)
	refreshRequests := make(chan struct{}, 1)
	requestRefresh := func() {
		select {
		case refreshRequests <- struct{}{}:
		default:
		}
	}

	updatingSwitch := false
	networkingSwitch.ConnectStateSet(func(state bool) bool {
		if updatingSwitch {
			return false
		}
		command := "off"
		if state {
			command = "on"
		}
		runDetached("nmcli", "n", command)
		time.AfterFunc(250*time.Millisecond, requestRefresh)
		return false
	})

	go func() {
		for range refreshRequests {
			snapshot := readNetworkSnapshot()
			ui(func() {
				setTextModule(module, snapshot.Text)
				if snapshot.Disconnected {
					module.Box.AddCSSClass("disconnected")
				} else {
					module.Box.RemoveCSSClass("disconnected")
				}

				updatingSwitch = true
				networkingSwitch.SetState(snapshot.NetworkingEnabled)
				networkingSwitch.SetActive(snapshot.NetworkingEnabled)
				updatingSwitch = false

				removeChildren(listBox)
				switch {
				case !snapshot.NetworkingEnabled:
					row := gtk.NewLabel("Networking is turned off")
					row.AddCSSClass("wifi-network-row")
					row.SetXAlign(0)
					listBox.Append(row)
				case len(snapshot.Networks) == 0:
					row := gtk.NewLabel("No Wi-Fi networks found")
					row.AddCSSClass("wifi-network-row")
					row.SetXAlign(0)
					listBox.Append(row)
				default:
					for _, network := range snapshot.Networks {
						row := gtk.NewLabel(formatWifiNetwork(network))
						row.AddCSSClass("wifi-network-row")
						row.SetXAlign(0)
						if network.Active {
							row.AddCSSClass("active")
						}
						listBox.Append(row)
					}
				}
			})
		}
	}()

	requestRefresh()
	startPolling(5*time.Second, requestRefresh)
	return module.Box
}

func readNetworkSnapshot() networkSnapshot {
	enabled := readNetworkingEnabled()
	text, disconnected := readNetwork()
	if !enabled {
		text = "Wi-Fi Off"
		disconnected = true
	}
	return networkSnapshot{
		Text:              text,
		Disconnected:      disconnected,
		NetworkingEnabled: enabled,
		Networks:          readWifiNetworks(),
	}
}

func readNetworkingEnabled() bool {
	output, err := runCommand("nmcli", "n")
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(string(output)), "enabled")
}

func readNetwork() (string, bool) {
	output, err := runCommand("nmcli", "-t", "-f", "DEVICE,TYPE,STATE,CONNECTION", "device")
	if err != nil {
		return "", true
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 4 {
			continue
		}

		device, kind, state := parts[0], parts[1], parts[2]
		if state != "connected" {
			continue
		}

		if kind == "wifi" {
			wifi, _ := runCommand("nmcli", "-t", "-f", "ACTIVE,SIGNAL,SSID", "device", "wifi")
			for _, wifiLine := range strings.Split(strings.TrimSpace(string(wifi)), "\n") {
				fields := strings.Split(wifiLine, ":")
				if len(fields) >= 3 && fields[0] == "yes" {
					signal := strings.TrimSpace(fields[1])
					if signal == "" {
						signal = "0"
					}
					return signal + "% ", false
				}
			}
			return "0% ", false
		}

		if kind == "ethernet" {
			return device + " ", false
		}
	}

	return "Disconnected ⚠", true
}

func readWifiNetworks() []wifiNetwork {
	output, err := runCommand("nmcli", "-t", "-f", "IN-USE,SIGNAL,SECURITY,SSID", "device", "wifi", "list")
	if err != nil {
		return nil
	}

	networks := make([]wifiNetwork, 0, 12)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := splitNmcliLine(line)
		if len(fields) < 4 {
			continue
		}

		signal := 0
		fmt.Sscanf(fields[1], "%d", &signal)
		security := strings.TrimSpace(unescapeNmcliField(fields[2]))
		if security == "" || security == "--" {
			security = "Open"
		}

		ssid := strings.TrimSpace(unescapeNmcliField(fields[3]))
		if ssid == "" {
			ssid = "<hidden>"
		}

		networks = append(networks, wifiNetwork{
			SSID:     ssid,
			Signal:   signal,
			Security: security,
			Active:   strings.TrimSpace(fields[0]) == "*",
		})
	}

	sort.SliceStable(networks, func(i, j int) bool {
		if networks[i].Active != networks[j].Active {
			return networks[i].Active
		}
		return networks[i].Signal > networks[j].Signal
	})

	if len(networks) > 10 {
		networks = networks[:10]
	}
	return networks
}

func splitNmcliLine(line string) []string {
	result := make([]string, 0, 4)
	var builder strings.Builder
	escaped := false

	for _, r := range line {
		switch {
		case escaped:
			builder.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == ':':
			result = append(result, builder.String())
			builder.Reset()
		default:
			builder.WriteRune(r)
		}
	}

	result = append(result, builder.String())
	return result
}

func unescapeNmcliField(value string) string {
	value = strings.ReplaceAll(value, `\:`, `:`)
	value = strings.ReplaceAll(value, `\\`, `\`)
	return value
}

func formatWifiNetwork(network wifiNetwork) string {
	prefix := " "
	if network.Active {
		prefix = "•"
	}
	return fmt.Sprintf("%s %3d%%  %s (%s)", prefix, network.Signal, network.SSID, network.Security)
}
