package modules

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

type audioDevice struct {
	ID        int
	Name      string
	IsDefault bool
	Volume    int
	Muted     bool
	IsInput   bool
}

type audioSnapshot struct {
	Text    string
	Muted   bool
	Outputs []audioDevice
	Inputs  []audioDevice
}

func NewPipewire() gtk.Widgetter {
	module := newTextModule("pipewire")
	module.Label.SetLabel("  0%  ")
	module.Label.SetWidthChars(9)
	module.Label.SetMaxWidthChars(9)
	module.Label.SetXAlign(0.5)
	module.Label.SetSingleLineMode(true)
	module.Label.SetWrap(false)
	module.Label.SetEllipsize(pango.EllipsizeEnd)
	module.Box.SetVisible(true)

	popover := gtk.NewPopover()
	popover.AddCSSClass("status-popup")
	popover.SetHasArrow(false)
	popover.SetAutohide(false)
	popover.SetParent(module.Box)

	menu := gtk.NewBox(gtk.OrientationVertical, 6)
	menu.SetName("audio-menu")
	popover.SetChild(menu)

	title := gtk.NewLabel("Audio Devices")
	title.SetName("audio-menu-title")
	title.SetXAlign(0)
	menu.Append(title)

	outputTitle := gtk.NewLabel("Outputs")
	outputTitle.AddCSSClass("audio-menu-section")
	outputTitle.SetXAlign(0)
	menu.Append(outputTitle)
	outputList := gtk.NewBox(gtk.OrientationVertical, 2)
	menu.Append(outputList)

	inputTitle := gtk.NewLabel("Inputs")
	inputTitle.AddCSSClass("audio-menu-section")
	inputTitle.SetXAlign(0)
	menu.Append(inputTitle)
	inputList := gtk.NewBox(gtk.OrientationVertical, 2)
	menu.Append(inputList)

	refreshRequests := make(chan struct{}, 1)
	var adjustingSlider bool
	requestRefresh := func() {
		select {
		case refreshRequests <- struct{}{}:
		default:
		}
	}

	openMixer := func() {
		popover.Popdown()
		runDetached("flatpak", "run", "com.saivert.pwvucontrol")
	}
	attachHoverPopover(module.Box, popover, openMixer, nil)
	attachScroll(module.Box, func() {
		adjustDefaultSinkVolume(5)
		time.AfterFunc(150*time.Millisecond, requestRefresh)
	}, func() {
		adjustDefaultSinkVolume(-5)
		time.AfterFunc(150*time.Millisecond, requestRefresh)
	})

	go func() {
		for range refreshRequests {
			snapshot := readAudioSnapshot()
			ui(func() {
				setTextModule(module, snapshot.Text)
				if snapshot.Muted {
					module.Box.AddCSSClass("muted")
				} else {
					module.Box.RemoveCSSClass("muted")
				}

				if !adjustingSlider {
					renderAudioDeviceList(outputList, snapshot.Outputs, requestRefresh, func() {
						adjustingSlider = true
					}, func() {
						adjustingSlider = false
					})
					renderAudioDeviceList(inputList, snapshot.Inputs, requestRefresh, func() {
						adjustingSlider = true
					}, func() {
						adjustingSlider = false
					})
				}
			})
		}
	}()

	requestRefresh()
	startPolling(3*time.Second, requestRefresh)

	return module.Box
}

func readAudioSnapshot() audioSnapshot {
	text, muted := readVolume()
	outputs, inputs := readAudioDevices()
	if strings.TrimSpace(text) == "" {
		text = "  0%  "
	}
	return audioSnapshot{
		Text:    text,
		Muted:   muted,
		Outputs: outputs,
		Inputs:  inputs,
	}
}

func readVolume() (string, bool) {
	percent, muted, ok := readPulseDeviceVolume(false, "@DEFAULT_SINK@")
	if !ok {
		return "", false
	}
	icon := ""
	if muted {
		icon = ""
	}
	switch {
	case percent == 0:
		icon = ""
	case percent < 35:
		icon = ""
	case percent < 70:
		icon = ""
	default:
		if !muted {
			icon = ""
		}
	}

	if percent < 0 {
		percent = 0
	}
	if percent > 999 {
		percent = 999
	}

	return fmt.Sprintf("%3d%% %s ", percent, icon), muted
}

func readAudioDevices() ([]audioDevice, []audioDevice) {
	defaultSink, defaultSource := readDefaultPulseDevices()
	outputs := readPulseDeviceList(false, defaultSink)
	inputs := readPulseDeviceList(true, defaultSource)
	return outputs, inputs
}

func formatAudioDevice(device audioDevice) string {
	if device.IsDefault {
		return fmt.Sprintf("• %s", device.Name)
	}
	return "  " + device.Name
}

func renderAudioDeviceList(parent *gtk.Box, devices []audioDevice, requestRefresh func(), beginAdjust func(), endAdjust func()) {
	removeChildren(parent)

	if len(devices) == 0 {
		row := gtk.NewLabel("No devices found")
		row.AddCSSClass("audio-device-row")
		row.SetXAlign(0)
		parent.Append(row)
		return
	}

	for _, device := range devices {
		parent.Append(newAudioDeviceRow(device, requestRefresh, beginAdjust, endAdjust))
	}
}

func newAudioDeviceRow(device audioDevice, requestRefresh func(), beginAdjust func(), endAdjust func()) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationVertical, 2)
	row.AddCSSClass("audio-device-row")
	if device.IsDefault {
		row.AddCSSClass("active")
	}

	button := gtk.NewButtonWithLabel(formatAudioDevice(device))
	button.SetHasFrame(false)
	button.AddCSSClass("audio-device-button")
	if child := button.Child(); child != nil {
		if lbl, ok := child.(*gtk.Label); ok {
			lbl.SetXAlign(0)
		}
	}
	button.ConnectClicked(func() {
		if device.IsInput {
			runDetached("pactl", "set-default-source", strconv.Itoa(device.ID))
		} else {
			runDetached("pactl", "set-default-sink", strconv.Itoa(device.ID))
		}
		time.AfterFunc(150*time.Millisecond, requestRefresh)
	})
	row.Append(button)

	sliderRow := gtk.NewBox(gtk.OrientationHorizontal, 6)
	sliderRow.AddCSSClass("audio-slider-row")

	slider := gtk.NewScaleWithRange(gtk.OrientationHorizontal, 0, 100, 1)
	slider.SetDrawValue(false)
	slider.SetHExpand(true)
	slider.AddCSSClass("audio-device-slider")

	value := gtk.NewLabel(fmt.Sprintf("%d%%", device.Volume))
	value.AddCSSClass("audio-device-volume")
	value.SetXAlign(1)

	initializing := true
	slider.SetValue(float64(device.Volume))
	initializing = false

	slider.ConnectValueChanged(func() {
		if initializing {
			return
		}
		volume := int(clamp(slider.Value(), 0, 100))
		value.SetLabel(fmt.Sprintf("%d%%", volume))
		if device.IsInput {
			runDetached("pactl", "set-source-volume", strconv.Itoa(device.ID), fmt.Sprintf("%d%%", volume))
		} else {
			runDetached("pactl", "set-sink-volume", strconv.Itoa(device.ID), fmt.Sprintf("%d%%", volume))
		}
	})

	sliderGesture := gtk.NewGestureClick()
	sliderGesture.SetButton(0)
	sliderGesture.ConnectPressed(func(_ int, _, _ float64) {
		if beginAdjust != nil {
			beginAdjust()
		}
	})
	sliderGesture.ConnectReleased(func(_ int, _, _ float64) {
		if endAdjust != nil {
			endAdjust()
		}
		time.AfterFunc(80*time.Millisecond, requestRefresh)
	})
	slider.AddController(sliderGesture)

	sliderRow.Append(slider)
	sliderRow.Append(value)
	row.Append(sliderRow)

	if device.Muted {
		muteLabel := gtk.NewLabel("Muted")
		muteLabel.AddCSSClass("audio-device-muted")
		muteLabel.SetXAlign(0)
		row.Append(muteLabel)
	}

	return row
}

func readDefaultPulseDevices() (string, string) {
	output, err := runCommand("pactl", "info")
	if err != nil {
		return "", ""
	}

	defaultSink := ""
	defaultSource := ""
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(line, "Default Sink: "); ok {
			defaultSink = strings.TrimSpace(value)
		}
		if value, ok := strings.CutPrefix(line, "Default Source: "); ok {
			defaultSource = strings.TrimSpace(value)
		}
	}

	return defaultSink, defaultSource
}

func readPulseDeviceList(input bool, defaultName string) []audioDevice {
	kind := "sinks"
	if input {
		kind = "sources"
	}
	output, err := runCommand("pactl", "-f", "json", "list", kind)
	if err != nil {
		return nil
	}

	var entries []struct {
		Index       int    `json:"index"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Mute        bool   `json:"mute"`
		Volume      map[string]struct {
			Percent string `json:"value_percent"`
		} `json:"volume"`
	}
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil
	}

	devices := make([]audioDevice, 0, len(entries))
	for _, e := range entries {
		name := e.Description
		if name == "" {
			name = simplifyPulseName(e.Name)
		}

		vol := 0
		for _, ch := range e.Volume {
			p := strings.TrimSuffix(ch.Percent, "%")
			if v, err := strconv.Atoi(p); err == nil {
				vol = v
				break
			}
		}

		devices = append(devices, audioDevice{
			ID:        e.Index,
			Name:      name,
			IsDefault: defaultName != "" && e.Name == defaultName,
			Volume:    vol,
			Muted:     e.Mute,
			IsInput:   input,
		})
	}

	return devices
}

func readPulseDeviceVolume(input bool, target string) (int, bool, bool) {
	if strings.TrimSpace(target) == "" {
		return 0, false, false
	}

	volumeCmd := "get-sink-volume"
	muteCmd := "get-sink-mute"
	if input {
		volumeCmd = "get-source-volume"
		muteCmd = "get-source-mute"
	}

	volumeOutput, err := runCommand("pactl", volumeCmd, target)
	if err != nil {
		return 0, false, false
	}
	muteOutput, err := runCommand("pactl", muteCmd, target)
	if err != nil {
		return 0, false, false
	}

	percentages := extractPercents(string(volumeOutput))
	if len(percentages) == 0 {
		return 0, false, false
	}

	total := 0
	for _, percent := range percentages {
		total += percent
	}
	average := int(clamp(float64(total)/float64(len(percentages)), 0, 100))

	muted := strings.Contains(strings.ToLower(string(muteOutput)), "yes")
	return average, muted, true
}

func adjustDefaultSinkVolume(delta int) {
	go func() {
		if delta == 0 {
			return
		}

		arg := fmt.Sprintf("%+d%%", delta)
		if _, err := runCommand("pactl", "set-sink-volume", "@DEFAULT_SINK@", arg); err == nil {
			return
		}

		step := fmt.Sprintf("%d%%", absInt(delta))
		if delta > 0 {
			step += "+"
		} else {
			step += "-"
		}
		runDetached("wpctl", "set-volume", "@DEFAULT_AUDIO_SINK@", step)
	}()
}

func simplifyPulseName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}

	name = strings.TrimPrefix(name, "alsa_output.")
	name = strings.TrimPrefix(name, "alsa_input.")
	name = strings.TrimPrefix(name, "bluez_output.")
	name = strings.TrimPrefix(name, "bluez_input.")
	name = strings.TrimSuffix(name, ".monitor")

	// Strip common technical prefixes like "pci-0000_00_1f.3-platform-skl_hda_dsp_generic."
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		candidate := name[idx+1:]
		if candidate != "" {
			name = candidate
		}
	}

	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")

	// Capitalize first letter of each word
	words := strings.Fields(name)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func extractPercents(text string) []int {
	values := make([]int, 0, 4)
	for _, field := range strings.Fields(text) {
		if !strings.HasSuffix(field, "%") {
			continue
		}
		raw := strings.TrimSuffix(field, "%")
		value, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		values = append(values, value)
	}
	return values
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
