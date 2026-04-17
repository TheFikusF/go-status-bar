package modules

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"statusbar/internal/config"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type wifiNetwork struct {
	SSID     string
	Signal   int
	Security string
	Active   bool
}

type networkInfoField struct {
	Label     string
	Value     string
	CopyValue string
}

type networkInfoGroup struct {
	Key      string
	Title    string
	Subtitle string
	Fields   []networkInfoField
}

type networkSnapshot struct {
	Text              string
	Disconnected      bool
	NetworkingEnabled bool
	Networks          []wifiNetwork
	Details           []networkInfoGroup
}

type wifiNetworkRowState struct {
	button  *gtk.Button
	label   *gtk.Label
	network wifiNetwork
}

func NewNetwork(cfg *config.Config) gtk.Widgetter {
	module := newTextModule("network")
	removeChildren(module.Box)
	module.Box.SetVisible(true)

	iconBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	signalIcon := gtk.NewImageFromIconName("network-wireless-signal-excellent-symbolic")
	valueLabel := gtk.NewLabel("")
	valueLabel.SetXAlign(0)
	iconBox.Append(signalIcon)
	if cfg.Network.ShowText {
		iconBox.Append(valueLabel)
	}
	module.Box.Append(iconBox)

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
	emptyRow := gtk.NewLabel("")
	emptyRow.AddCSSClass("wifi-network-row")
	emptyRow.SetXAlign(0)
	emptyRow.SetVisible(false)
	listBox.Append(emptyRow)

	infoBox := gtk.NewBox(gtk.OrientationVertical, 6)
	infoBox.AddCSSClass("wifi-info-section")
	// menu.Append(infoBox) // TODO: FIX

	expandedGroups := make(map[string]bool)
	latestSnapshot := networkSnapshot{}
	refreshRequests := make(chan struct{}, 1)
	var popupVisible bool
	var networkRows []*wifiNetworkRowState

	requestRefresh := func() {
		select {
		case refreshRequests <- struct{}{}:
		default:
		}
	}

	onClickCmd := "nm-connection-editor"
	if cfg != nil && cfg.Network.OnClick != "" {
		onClickCmd = cfg.Network.OnClick
	}
	parts := strings.Fields(onClickCmd)

	popup := attachHoverPopover(module.Box, popover, func() {
		if len(parts) > 0 {
			runDetached(parts[0], parts[1:]...)
		}
	}, func() {
		popupVisible = true
		for key := range expandedGroups {
			delete(expandedGroups, key)
		}
		if infoBox.Parent() != nil {
			renderNetworkInfo(infoBox, latestSnapshot, expandedGroups)
		}
		requestRefresh()
	})
	popup.SetAfterClose(func() {
		popupVisible = false
		for key := range expandedGroups {
			delete(expandedGroups, key)
		}
		if infoBox.Parent() != nil {
			removeChildren(infoBox)
		}
	})

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
				latestSnapshot = snapshot

				bestSignal := 0
				bestSSID := ""
				for _, network := range snapshot.Networks {
					if network.Signal > bestSignal {
						bestSignal = network.Signal
						bestSSID = network.SSID
					}
				}

				iconName := "network-wireless-signal-none-symbolic"
				switch {
				case !snapshot.NetworkingEnabled:
					iconName = "network-wireless-offline-symbolic"
				case bestSignal >= 80:
					iconName = "network-wireless-signal-excellent-symbolic"
				case bestSignal >= 60:
					iconName = "network-wireless-signal-good-symbolic"
				case bestSignal >= 40:
					iconName = "network-wireless-signal-ok-symbolic"
				case bestSignal > 0:
					iconName = "network-wireless-signal-weak-symbolic"
				}
				signalIcon.SetFromIconName(iconName)

				if cfg.Network.ShowText {
					switch {
					case bestSSID != "":
						valueLabel.SetLabel(bestSSID)
					case snapshot.Disconnected:
						valueLabel.SetLabel("Disconnected")
					default:
						valueLabel.SetLabel("")
					}
				} else {
					valueLabel.SetLabel("")
				}

				if snapshot.Disconnected {
					module.Box.AddCSSClass("disconnected")
				} else {
					module.Box.RemoveCSSClass("disconnected")
				}

				updatingSwitch = true
				networkingSwitch.SetState(snapshot.NetworkingEnabled)
				networkingSwitch.SetActive(snapshot.NetworkingEnabled)
				updatingSwitch = false

				if popupVisible {
					renderWifiNetworkList(listBox, emptyRow, &networkRows, popover, snapshot)

					if infoBox.Parent() != nil {
						renderNetworkInfo(infoBox, snapshot, expandedGroups)
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
		Details:           readNetworkInfoGroups(),
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

func readNetworkInfoGroups() []networkInfoGroup {
	output, err := runCommand("nmcli", "-t", "-f", "DEVICE,TYPE,STATE,CONNECTION", "device")
	if err != nil {
		return nil
	}

	groups := make([]networkInfoGroup, 0, 2)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := splitNmcliLine(line)
		if len(fields) < 4 {
			continue
		}

		state := strings.TrimSpace(unescapeNmcliField(fields[2]))
		if !strings.HasPrefix(state, "connected") {
			continue
		}

		device := strings.TrimSpace(unescapeNmcliField(fields[0]))
		kind := strings.TrimSpace(unescapeNmcliField(fields[1]))
		connection := strings.TrimSpace(unescapeNmcliField(fields[3]))
		group, ok := readNetworkInfoGroup(device, kind, connection)
		if ok {
			groups = append(groups, group)
		}
	}

	return groups
}

func readNetworkInfoGroup(device, kind, connection string) (networkInfoGroup, bool) {
	output, err := runCommand(
		"nmcli",
		"-t",
		"-m",
		"multiline",
		"-f",
		"GENERAL.DEVICE,GENERAL.TYPE,GENERAL.CONNECTION,GENERAL.HWADDR,IP4.ADDRESS,IP4.GATEWAY,IP4.DNS,IP6.ADDRESS,IP6.GATEWAY,IP6.DNS",
		"device",
		"show",
		device,
	)
	if err != nil {
		return networkInfoGroup{}, false
	}

	var ipv4 []string
	var ipv6 []string
	var gateways []string
	var dns []string
	hardwareAddr := ""

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		value = strings.TrimSpace(value)
		switch {
		case key == "GENERAL.DEVICE" && value != "":
			device = value
		case key == "GENERAL.TYPE" && value != "":
			kind = value
		case key == "GENERAL.CONNECTION" && value != "" && value != "--":
			connection = value
		case key == "GENERAL.HWADDR":
			hardwareAddr = value
		case strings.HasPrefix(key, "IP4.ADDRESS"):
			ipv4 = append(ipv4, value)
		case strings.HasPrefix(key, "IP6.ADDRESS"):
			ipv6 = append(ipv6, value)
		case strings.HasPrefix(key, "IP4.GATEWAY") || strings.HasPrefix(key, "IP6.GATEWAY"):
			gateways = append(gateways, value)
		case strings.HasPrefix(key, "IP4.DNS") || strings.HasPrefix(key, "IP6.DNS"):
			dns = append(dns, value)
		}
	}

	if connection == "" || connection == "--" {
		connection = device
	}

	fields := make([]networkInfoField, 0, 5)
	fields = appendNetworkInfoField(fields, "Device", []string{device})
	fields = appendNetworkInfoField(fields, "IPv4", ipv4)
	fields = appendNetworkInfoField(fields, "IPv6", ipv6)
	fields = appendNetworkInfoField(fields, "Gateway", gateways)
	fields = appendNetworkInfoField(fields, "DNS", dns)
	fields = appendNetworkInfoField(fields, "MAC", []string{hardwareAddr})

	return networkInfoGroup{
		Key:      formatNetworkInfoKey(connection, device, kind),
		Title:    connection,
		Subtitle: formatNetworkInfoSubtitle(kind, device),
		Fields:   fields,
	}, connection != "" || len(fields) > 0
}

func appendNetworkInfoField(fields []networkInfoField, label string, values []string) []networkInfoField {
	values = compactNetworkValues(values)
	if len(values) == 0 {
		return fields
	}

	fields = append(fields, networkInfoField{
		Label:     label,
		Value:     strings.Join(values, ", "),
		CopyValue: strings.Join(values, "\n"),
	})
	return fields
}

func compactNetworkValues(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(unescapeNmcliField(value))
		if value == "" || value == "--" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func formatNetworkInfoSubtitle(kind, device string) string {
	parts := make([]string, 0, 2)
	if label := formatNetworkType(kind); label != "" {
		parts = append(parts, label)
	}
	if device != "" {
		parts = append(parts, device)
	}
	return strings.Join(parts, " • ")
}

func formatNetworkInfoKey(connection, device, kind string) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(connection)),
		strings.ToLower(strings.TrimSpace(device)),
		strings.ToLower(strings.TrimSpace(kind)),
	}
	return strings.Join(parts, "|")
}

func formatNetworkType(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "wifi", "wireless", "802-11-wireless":
		return "Wi-Fi"
	case "ethernet", "802-3-ethernet":
		return "Ethernet"
	case "tun", "tunnel":
		return "Tunnel"
	case "wireguard":
		return "WireGuard"
	case "vpn":
		return "VPN"
	default:
		return strings.TrimSpace(kind)
	}
}

func renderNetworkInfo(parent *gtk.Box, snapshot networkSnapshot, expandedGroups map[string]bool) {
	removeChildren(parent)

	title := gtk.NewLabel("Network info")
	title.AddCSSClass("wifi-info-title")
	title.SetXAlign(0)
	parent.Append(title)

	if len(snapshot.Details) == 0 {
		message := "Connect to a network to view IP details"
		switch {
		case !snapshot.NetworkingEnabled:
			message = "Turn networking on to view IP details"
		case snapshot.Disconnected:
			message = "Connect to a network to view IP details"
		}

		empty := gtk.NewLabel(message)
		empty.AddCSSClass("wifi-info-empty")
		empty.SetXAlign(0)
		parent.Append(empty)
		return
	}

	for _, detail := range snapshot.Details {
		parent.Append(newNetworkInfoGroup(detail, expandedGroups))
	}
}

func renderWifiNetworkList(parent *gtk.Box, empty *gtk.Label, rows *[]*wifiNetworkRowState, popover *gtk.Popover, snapshot networkSnapshot) {
	message := ""
	switch {
	case !snapshot.NetworkingEnabled:
		message = "Networking is turned off"
	case len(snapshot.Networks) == 0:
		message = "No Wi-Fi networks found"
	}

	if message != "" {
		empty.SetLabel(message)
		empty.SetVisible(true)
		for _, row := range *rows {
			row.button.SetVisible(false)
		}
		return
	}

	empty.SetVisible(false)
	for len(*rows) < len(snapshot.Networks) {
		row := newWifiNetworkRowState(popover)
		row.button.SetVisible(false)
		parent.Append(row.button)
		*rows = append(*rows, row)
	}

	for i, network := range snapshot.Networks {
		row := (*rows)[i]
		row.update(network)
		row.button.SetVisible(true)
	}
	for i := len(snapshot.Networks); i < len(*rows); i++ {
		(*rows)[i].button.SetVisible(false)
	}
}

func newWifiNetworkRowState(popover *gtk.Popover) *wifiNetworkRowState {
	row := &wifiNetworkRowState{}
	row.button = gtk.NewButton()
	row.button.SetHasFrame(false)
	row.button.AddCSSClass("wifi-network-row")
	row.label = gtk.NewLabel("")
	row.label.SetXAlign(0)
	row.button.SetChild(row.label)
	row.button.ConnectClicked(func() {
		popover.Popdown()
		runDetached("nmcli", "device", "wifi", "connect", row.network.SSID)
	})
	return row
}

func (row *wifiNetworkRowState) update(network wifiNetwork) {
	row.network = network
	row.label.SetLabel(formatWifiNetwork(network))
	if network.Active {
		row.button.AddCSSClass("active")
	} else {
		row.button.RemoveCSSClass("active")
	}
}

func newNetworkInfoGroup(detail networkInfoGroup, expandedGroups map[string]bool) gtk.Widgetter {
	group := gtk.NewBox(gtk.OrientationVertical, 4)
	group.AddCSSClass("wifi-info-group")

	content := gtk.NewBox(gtk.OrientationVertical, 4)
	if detail.Subtitle != "" {
		subtitle := gtk.NewLabel(detail.Subtitle)
		subtitle.AddCSSClass("wifi-info-subtitle")
		subtitle.SetXAlign(0)
		content.Append(subtitle)
	}

	for _, field := range detail.Fields {
		content.Append(newNetworkInfoRow(field))
	}

	revealer := gtk.NewRevealer()
	revealer.SetTransitionType(gtk.RevealerTransitionTypeSlideDown)
	revealer.SetTransitionDuration(140)
	revealer.SetChild(content)

	expanded := expandedGroups[detail.Key]
	revealer.SetRevealChild(expanded)

	toggle := gtk.NewButton()
	toggle.SetHasFrame(false)
	toggle.AddCSSClass("wifi-info-toggle")

	header := gtk.NewBox(gtk.OrientationHorizontal, 6)
	indicator := gtk.NewImageFromIconName("pan-end-symbolic")
	indicator.AddCSSClass("wifi-info-expander")
	header.Append(indicator)

	heading := gtk.NewLabel(detail.Title)
	heading.AddCSSClass("wifi-info-heading")
	heading.SetXAlign(0)
	heading.SetHExpand(true)
	header.Append(heading)
	toggle.SetChild(header)

	updateExpandedState := func(isExpanded bool) {
		expandedGroups[detail.Key] = isExpanded
		revealer.SetRevealChild(isExpanded)
		if isExpanded {
			indicator.SetFromIconName("pan-down-symbolic")
			toggle.AddCSSClass("expanded")
			group.AddCSSClass("expanded")
		} else {
			indicator.SetFromIconName("pan-end-symbolic")
			toggle.RemoveCSSClass("expanded")
			group.RemoveCSSClass("expanded")
		}
	}

	updateExpandedState(expanded)
	toggle.ConnectClicked(func() {
		updateExpandedState(!expandedGroups[detail.Key])
	})

	group.Append(toggle)
	group.Append(revealer)
	return group
}

func newNetworkInfoRow(field networkInfoField) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 6)
	row.AddCSSClass("wifi-info-row")

	label := gtk.NewLabel(field.Label)
	label.AddCSSClass("wifi-info-label")
	label.SetSizeRequest(72, -1)
	label.SetXAlign(0)
	row.Append(label)

	value := gtk.NewLabel(field.Value)
	value.AddCSSClass("wifi-info-value")
	value.SetWrap(true)
	value.SetXAlign(0)
	value.SetHExpand(true)
	row.Append(value)

	copyButton := gtk.NewButton()
	copyButton.SetHasFrame(false)
	copyButton.AddCSSClass("wifi-info-copy")
	copyButton.SetTooltipText("Copy " + field.Label)
	copyButton.SetChild(gtk.NewImageFromIconName("edit-copy-symbolic"))

	copyValue := field.CopyValue
	copyButton.ConnectClicked(func() {
		go func() {
			_ = runCommandStdin([]byte(copyValue), "wl-copy")
		}()
	})
	row.Append(copyButton)

	return row
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
