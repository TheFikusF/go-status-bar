package modules

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"statusbar/internal/config"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type bluetoothDevice struct {
	Address   string
	Name      string
	Connected bool
	Paired    bool
	Trusted   bool
}

type bluetoothSnapshot struct {
	Powered       bool
	Connected     bool
	ConnectedName string
	Devices       []bluetoothDevice
}

type bluetoothScanner struct {
	mu      sync.Mutex
	devices map[string]bluetoothDevice
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	running bool
}

func newBluetoothScanner() *bluetoothScanner {
	return &bluetoothScanner{devices: make(map[string]bluetoothDevice)}
}

func (s *bluetoothScanner) start(onUpdate func()) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	cmd := exec.Command("bluetoothctl")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.mu.Unlock()
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.mu.Unlock()
		return
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		s.mu.Unlock()
		return
	}
	if err := cmd.Start(); err != nil {
		s.mu.Unlock()
		return
	}
	s.cmd = cmd
	s.stdin = stdin
	s.running = true
	s.mu.Unlock()

	go s.readStream(stdout, onUpdate)
	go s.readStream(stderr, onUpdate)
	_, _ = io.WriteString(stdin, "scan on\n")
}

func (s *bluetoothScanner) stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	stdin := s.stdin
	cmd := s.cmd
	s.stdin = nil
	s.cmd = nil
	s.running = false
	s.mu.Unlock()

	if stdin != nil {
		_, _ = io.WriteString(stdin, "scan off\nexit\n")
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		go func() {
			time.Sleep(500 * time.Millisecond)
			_ = cmd.Process.Kill()
		}()
	}
}

func (s *bluetoothScanner) snapshot() []bluetoothDevice {
	s.mu.Lock()
	defer s.mu.Unlock()
	devices := make([]bluetoothDevice, 0, len(s.devices))
	for _, device := range s.devices {
		devices = append(devices, device)
	}
	return devices
}

func (s *bluetoothScanner) readStream(stream io.ReadCloser, onUpdate func()) {
	defer stream.Close()
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		line := strings.TrimSpace(stripANSIEscapeCodes(scanner.Text()))
		if line == "" {
			continue
		}
		if s.parseLine(line) && onUpdate != nil {
			onUpdate()
		}
	}
}

func (s *bluetoothScanner) parseLine(line string) bool {
	for _, marker := range []string{"[NEW] Device ", "[CHG] Device ", "[DEL] Device "} {
		if idx := strings.Index(line, marker); idx >= 0 {
			line = line[idx:]
			break
		}
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}

	parts := strings.Fields(line)
	if len(parts) < 3 {
		return false
	}
	if (parts[0] != "[NEW]" && parts[0] != "[CHG]" && parts[0] != "[DEL]") || parts[1] != "Device" {
		return false
	}
	address := parts[2]
	if !looksLikeBluetoothAddress(address) {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	device := s.devices[address]
	device.Address = address
	changed := false

	if parts[0] == "[DEL]" {
		delete(s.devices, address)
		return true
	}

	rest := strings.TrimSpace(strings.TrimPrefix(line, parts[0]+" Device "+address))
	switch {
	case strings.HasPrefix(rest, "Name: "):
		name := strings.TrimSpace(strings.TrimPrefix(rest, "Name: "))
		if name != "" && device.Name != name {
			device.Name = name
			changed = true
		}
	case strings.HasPrefix(rest, "Alias: "):
		name := strings.TrimSpace(strings.TrimPrefix(rest, "Alias: "))
		if name != "" && device.Name != name {
			device.Name = name
			changed = true
		}
	case strings.HasPrefix(rest, "Connected: "):
		connected := strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(rest, "Connected: ")), "yes")
		if device.Connected != connected {
			device.Connected = connected
			changed = true
		}
	case strings.HasPrefix(rest, "Paired: "):
		paired := strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(rest, "Paired: ")), "yes")
		if device.Paired != paired {
			device.Paired = paired
			changed = true
		}
	case strings.HasPrefix(rest, "Trusted: "):
		trusted := strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(rest, "Trusted: ")), "yes")
		if device.Trusted != trusted {
			device.Trusted = trusted
			changed = true
		}
	default:
		if rest != "" && !strings.Contains(rest, ":") && device.Name != rest {
			device.Name = rest
			changed = true
		}
	}

	if device.Name == "" {
		device.Name = address
	}
	s.devices[address] = device
	return changed || parts[0] == "[NEW]"
}

func looksLikeBluetoothAddress(value string) bool {
	if len(value) != 17 {
		return false
	}
	for i, part := range strings.Split(value, ":") {
		if len(part) != 2 {
			return false
		}
		if i > 5 {
			return false
		}
	}
	return true
}

func stripANSIEscapeCodes(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	inEscape := false
	for i := 0; i < len(value); i++ {
		char := value[i]
		switch {
		case inEscape:
			if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') {
				inEscape = false
			}
		case char == 0x1b:
			inEscape = true
		case char == '\r':
			continue
		default:
			builder.WriteByte(char)
		}
	}
	return builder.String()
}

func NewBluetooth(cfg *config.Config) gtk.Widgetter {
	module := newTextModule("bluetooth")
	removeChildren(module.Box)
	module.Box.SetVisible(true)

	showText := cfg != nil && cfg.Bluetooth.ShowText

	iconBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	statusIcon := gtk.NewImageFromIconName("bluetooth-symbolic")
	valueLabel := gtk.NewLabel("")
	valueLabel.SetXAlign(0)
	iconBox.Append(statusIcon)
	if showText {
		iconBox.Append(valueLabel)
	}
	module.Box.Append(iconBox)

	popover := gtk.NewPopover()
	popover.AddCSSClass("status-popup")
	popover.SetHasArrow(false)
	popover.SetAutohide(true)
	popover.SetParent(module.Box)

	menu := gtk.NewBox(gtk.OrientationVertical, 4)
	menu.SetName("bluetooth-menu")
	popover.SetChild(menu)

	titleRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	titleRow.AddCSSClass("bluetooth-title-row")
	title := gtk.NewLabel("Bluetooth")
	title.SetName("bluetooth-menu-title")
	title.SetXAlign(0)
	title.SetHExpand(true)
	powerSwitch := gtk.NewSwitch()
	titleRow.Append(title)
	titleRow.Append(powerSwitch)
	menu.Append(titleRow)

	statusLabel := gtk.NewLabel("")
	statusLabel.AddCSSClass("bluetooth-status-row")
	statusLabel.SetXAlign(0)
	statusLabel.SetVisible(false)
	menu.Append(statusLabel)

	listBox := gtk.NewBox(gtk.OrientationVertical, 2)
	menu.Append(listBox)

	scanner := newBluetoothScanner()
	refreshRequests := make(chan struct{}, 1)
	scanning := false
	updatingSwitch := false
	pendingDevice := ""
	requestRefresh := func() {
		select {
		case refreshRequests <- struct{}{}:
		default:
		}
	}

	attachHoverPopover(module.Box, popover, nil, func() {
		if readBluetoothPowered() {
			scanning = true
			scanner.start(requestRefresh)
			requestRefresh()
		}
	})
	popover.ConnectClosed(func() {
		if scanning {
			scanning = false
			scanner.stop()
		}
	})
	powerSwitch.ConnectStateSet(func(state bool) bool {
		if updatingSwitch {
			return false
		}
		if state {
			runDetached("rfkill", "unblock", "bluetooth")
		} else {
			runDetached("rfkill", "block", "bluetooth")
		}
		time.AfterFunc(400*time.Millisecond, requestRefresh)
		return false
	})

	go func() {
		for range refreshRequests {
			snapshot := readBluetoothSnapshot(scanner.snapshot())
			ui(func() {
				iconName := "bluetooth-disabled-symbolic"
				switch {
				case !snapshot.Powered:
					iconName = "bluetooth-disabled-symbolic"
				case snapshot.Connected:
					iconName = "bluetooth-active-symbolic"
				default:
					iconName = "bluetooth-symbolic"
				}
				statusIcon.SetFromIconName(iconName)

				if showText {
					switch {
					case snapshot.ConnectedName != "":
						valueLabel.SetLabel(snapshot.ConnectedName)
					case !snapshot.Powered:
						valueLabel.SetLabel("Off")
					case len(snapshot.Devices) > 0:
						valueLabel.SetLabel(fmt.Sprintf("%d", len(snapshot.Devices)))
					default:
						valueLabel.SetLabel("")
					}
				}

				if !snapshot.Powered {
					module.Box.AddCSSClass("off")
				} else {
					module.Box.RemoveCSSClass("off")
				}
				if snapshot.Connected {
					module.Box.AddCSSClass("connected")
				} else {
					module.Box.RemoveCSSClass("connected")
				}

				updatingSwitch = true
				powerSwitch.SetState(snapshot.Powered)
				powerSwitch.SetActive(snapshot.Powered)
				updatingSwitch = false

				removeChildren(listBox)
				switch {
				case !snapshot.Powered:
					pendingDevice = ""
					statusLabel.SetLabel("")
					statusLabel.SetVisible(false)
					row := gtk.NewLabel("Bluetooth is turned off")
					row.AddCSSClass("bluetooth-device-row")
					row.SetXAlign(0)
					listBox.Append(row)
				case len(snapshot.Devices) == 0:
					message := "No Bluetooth devices found"
					if scanning {
						message = "Scanning for Bluetooth devices..."
					}
					row := gtk.NewLabel(message)
					row.AddCSSClass("bluetooth-device-row")
					row.SetXAlign(0)
					listBox.Append(row)
				default:
					for _, device := range snapshot.Devices {
						device := device
						row := gtk.NewButtonWithLabel(formatBluetoothDevice(device))
						row.SetHasFrame(false)
						row.AddCSSClass("bluetooth-device-row")
						if pendingDevice != "" && pendingDevice == device.Address {
							row.SetSensitive(false)
						}
						if child := row.Child(); child != nil {
							if lbl, ok := child.(*gtk.Label); ok {
								lbl.SetXAlign(0)
							}
						}
						if device.Connected {
							row.AddCSSClass("active")
						}
						row.ConnectClicked(func() {
							pendingDevice = device.Address
							statusLabel.SetLabel(bluetoothActionStartText(device))
							statusLabel.SetVisible(true)
							requestRefresh()
							go func(device bluetoothDevice) {
								err := performBluetoothAction(device)
								ui(func() {
									pendingDevice = ""
									if err != nil {
										statusLabel.SetLabel(err.Error())
										statusLabel.SetVisible(true)
									} else {
										statusLabel.SetLabel(bluetoothActionDoneText(device))
										statusLabel.SetVisible(true)
										time.AfterFunc(2*time.Second, func() {
											ui(func() {
												if pendingDevice == "" {
													statusLabel.SetLabel("")
													statusLabel.SetVisible(false)
												}
											})
										})
									}
								})
								time.AfterFunc(250*time.Millisecond, requestRefresh)
								time.AfterFunc(1500*time.Millisecond, requestRefresh)
								time.AfterFunc(3500*time.Millisecond, requestRefresh)
							}(device)
						})
						listBox.Append(row)
					}
				}
			})
		}
	}()

	requestRefresh()
	startPolling(2*time.Second, requestRefresh)
	return module.Box
}

func readBluetoothSnapshot(discovered []bluetoothDevice) bluetoothSnapshot {
	devices := mergeBluetoothDevices(readBluetoothDevices(), discovered)
	snapshot := bluetoothSnapshot{
		Powered: readBluetoothPowered(),
		Devices: devices,
	}
	for _, device := range devices {
		if device.Connected {
			snapshot.Connected = true
			if snapshot.ConnectedName == "" {
				snapshot.ConnectedName = device.Name
			}
		}
	}
	return snapshot
}

func mergeBluetoothDevices(known, discovered []bluetoothDevice) []bluetoothDevice {
	merged := make(map[string]bluetoothDevice, len(known)+len(discovered))
	for _, device := range discovered {
		if strings.TrimSpace(device.Address) == "" {
			continue
		}
		merged[device.Address] = device
	}
	for _, device := range known {
		if strings.TrimSpace(device.Address) == "" {
			continue
		}
		current := merged[device.Address]
		if device.Name != "" {
			current.Name = device.Name
		}
		current.Address = device.Address
		current.Connected = current.Connected || device.Connected
		current.Paired = current.Paired || device.Paired
		current.Trusted = current.Trusted || device.Trusted
		merged[device.Address] = current
	}

	devices := make([]bluetoothDevice, 0, len(merged))
	for _, device := range merged {
		if strings.TrimSpace(device.Name) == "" {
			device.Name = device.Address
		}
		devices = append(devices, device)
	}

	sort.SliceStable(devices, func(i, j int) bool {
		if devices[i].Connected != devices[j].Connected {
			return devices[i].Connected
		}
		if devices[i].Paired != devices[j].Paired {
			return devices[i].Paired
		}
		return strings.ToLower(devices[i].Name) < strings.ToLower(devices[j].Name)
	})
	return devices
}

func readBluetoothPowered() bool {
	output, err := runCommand("rfkill", "list", "bluetooth")
	if err != nil {
		return false
	}
	softBlocked := false
	hardBlocked := false
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(line, "Soft blocked: "); ok {
			softBlocked = strings.EqualFold(strings.TrimSpace(value), "yes")
		}
		if value, ok := strings.CutPrefix(line, "Hard blocked: "); ok {
			hardBlocked = strings.EqualFold(strings.TrimSpace(value), "yes")
		}
	}
	return !softBlocked && !hardBlocked
}

func readBluetoothDevices() []bluetoothDevice {
	output, err := runCommand("bluetoothctl", "devices")
	if err != nil {
		return nil
	}

	devices := make([]bluetoothDevice, 0, 8)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "Device ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		address := fields[1]
		name := strings.TrimSpace(strings.Join(fields[2:], " "))
		device := readBluetoothDeviceInfo(address)
		device.Address = address
		if device.Name == "" {
			device.Name = name
		}
		devices = append(devices, device)
	}

	sort.SliceStable(devices, func(i, j int) bool {
		if devices[i].Connected != devices[j].Connected {
			return devices[i].Connected
		}
		if devices[i].Paired != devices[j].Paired {
			return devices[i].Paired
		}
		return strings.ToLower(devices[i].Name) < strings.ToLower(devices[j].Name)
	})
	return devices
}

func readBluetoothDeviceInfo(address string) bluetoothDevice {
	device := bluetoothDevice{Address: address}
	output, err := runCommand("bluetoothctl", "info", address)
	if err != nil {
		return device
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Name: "):
			device.Name = strings.TrimSpace(strings.TrimPrefix(line, "Name: "))
		case strings.HasPrefix(line, "Connected: "):
			device.Connected = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "Connected: ")), "yes")
		case strings.HasPrefix(line, "Paired: "):
			device.Paired = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "Paired: ")), "yes")
		case strings.HasPrefix(line, "Trusted: "):
			device.Trusted = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "Trusted: ")), "yes")
		}
	}
	return device
}

func formatBluetoothDevice(device bluetoothDevice) string {
	action := bluetoothActionLabel(device)
	name := strings.TrimSpace(device.Name)
	if name == "" {
		name = device.Address
	}
	return fmt.Sprintf("%-10s %s", action, name)
}

func bluetoothActionLabel(device bluetoothDevice) string {
	switch {
	case device.Connected:
		return "Disconnect"
	case !device.Paired:
		return "Pair"
	case !device.Trusted:
		return "Trust"
	default:
		return "Connect"
	}
}

func bluetoothActionStartText(device bluetoothDevice) string {
	switch {
	case device.Connected:
		return "Disconnecting " + device.Name + "..."
	case !device.Paired:
		return "Pairing " + device.Name + "..."
	case !device.Trusted:
		return "Trusting " + device.Name + "..."
	default:
		return "Connecting " + device.Name + "..."
	}
}

func bluetoothActionDoneText(device bluetoothDevice) string {
	if device.Connected {
		return "Disconnected " + device.Name
	}
	return "Updated " + device.Name
}

func performBluetoothAction(device bluetoothDevice) error {
	name := strings.TrimSpace(device.Name)
	if name == "" {
		name = device.Address
	}

	if device.Connected {
		if err := runBluetoothctlCommand(10*time.Second, "disconnect", device.Address); err != nil {
			return fmt.Errorf("disconnect %s failed", name)
		}
		return nil
	}

	if !device.Paired {
		if err := runBluetoothctlCommand(20*time.Second, "pair", device.Address); err != nil {
			return fmt.Errorf("pair %s failed", name)
		}
	}
	if !device.Trusted {
		if err := runBluetoothctlCommand(10*time.Second, "trust", device.Address); err != nil {
			return fmt.Errorf("trust %s failed", name)
		}
	}
	if err := runBluetoothctlCommand(20*time.Second, "connect", device.Address); err != nil {
		return fmt.Errorf("connect %s failed", name)
	}
	return nil
}

func runBluetoothctlCommand(timeout time.Duration, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bluetoothctl", args...)
	return cmd.Run()
}
