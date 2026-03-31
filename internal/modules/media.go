package modules

import (
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
	"github.com/godbus/dbus/v5"
)

type mprisInfo struct {
	Bus       string
	Player    string
	Artist    string
	Album     string
	Title     string
	Text      string
	ArtURL    string
	Playing   bool
	CanSeek   bool
	HasVolume bool
	TrackID   dbus.ObjectPath
	Length    int64
	Position  int64
	Volume    float64
}

type cavaColor struct {
	r float64
	g float64
	b float64
	a float64
}

func NewMusic() gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationHorizontal, 0)
	box.SetName("custom-music")
	box.SetVisible(false)

	coverShell := gtk.NewBox(gtk.OrientationHorizontal, 0)
	coverShell.SetName("custom-music-cover")

	cover := gtk.NewPicture()
	cover.SetCanShrink(true)
	cover.SetContentFit(gtk.ContentFitCover)
	cover.SetSizeRequest(22, 22)
	coverShell.Append(cover)

	textShell := gtk.NewBox(gtk.OrientationHorizontal, 0)
	textShell.SetName("custom-music-text")
	textShell.SetHExpand(false)
	label := gtk.NewLabel("")
	label.SetSingleLineMode(true)
	label.SetWrap(false)
	label.SetEllipsize(pango.EllipsizeEnd)
	label.SetHExpand(false)
	label.SetXAlign(0)
	textShell.Append(label)

	box.Append(coverShell)
	box.Append(textShell)

	popover := gtk.NewPopover()
	popover.AddCSSClass("status-popup")
	popover.SetHasArrow(false)
	popover.SetAutohide(true)
	popover.SetParent(box)

	menu := gtk.NewOverlay()
	menu.SetName("music-menu")
	menu.SetSizeRequest(240, 120) // 320, 128
	menu.SetHExpand(false)
	menu.SetVExpand(false)

	background := gtk.NewPicture()
	background.SetCanShrink(true)
	background.SetContentFit(gtk.ContentFitCover)
	background.SetName("music-menu-bg")
	background.SetSizeRequest(240, 120)
	background.SetHExpand(true)
	background.SetVExpand(true)
	menu.SetChild(background)

	foreground := gtk.NewBox(gtk.OrientationVertical, 0)
	foreground.SetName("music-menu-foreground")
	foreground.SetHExpand(true)
	foreground.SetVExpand(true)
	menu.AddOverlay(foreground)

	content := gtk.NewBox(gtk.OrientationVertical, 0)
	content.SetName("music-menu-content")
	content.SetHExpand(true)
	content.SetVExpand(true)
	foreground.Append(content)

	header := gtk.NewBox(gtk.OrientationHorizontal, 10)
	header.SetName("music-menu-header")
	header.SetHExpand(true)
	content.Append(header)

	texts := gtk.NewBox(gtk.OrientationVertical, 4)
	texts.SetName("music-menu-texts")
	texts.SetHExpand(true)
	header.Append(texts)

	titleLabel := gtk.NewLabel("")
	titleLabel.SetName("music-menu-title")
	titleLabel.SetXAlign(0)
	titleLabel.SetHExpand(true)
	titleLabel.SetSingleLineMode(true)
	titleLabel.SetWrap(false)
	titleLabel.SetEllipsize(pango.EllipsizeEnd)
	artistLabel := gtk.NewLabel("")
	artistLabel.SetName("music-menu-artist")
	artistLabel.SetXAlign(0)
	artistLabel.SetHExpand(true)
	artistLabel.SetSingleLineMode(true)
	artistLabel.SetWrap(false)
	artistLabel.SetEllipsize(pango.EllipsizeEnd)
	texts.Append(titleLabel)
	texts.Append(artistLabel)

	slider := gtk.NewScaleWithRange(gtk.OrientationHorizontal, 0, 100, 1)
	slider.SetName("music-menu-slider")
	slider.SetDrawValue(false)
	slider.SetHExpand(true)
	positionLabel := gtk.NewLabel("0:00")
	positionLabel.SetName("music-menu-position")
	positionLabel.SetXAlign(0)
	durationLabel := gtk.NewLabel("0:00")
	durationLabel.SetName("music-menu-duration")
	durationLabel.SetXAlign(1)
	progressRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	progressRow.SetName("music-menu-progress-row")
	progressRow.SetHExpand(true)
	progressRow.Append(positionLabel)
	progressRow.Append(slider)
	progressRow.Append(durationLabel)

	controls := gtk.NewBox(gtk.OrientationHorizontal, 8)
	controls.SetName("music-menu-controls")
	controls.SetHAlign(gtk.AlignEnd)
	controls.SetVAlign(gtk.AlignStart)
	prevButton := gtk.NewButtonWithLabel("󰒮")
	playPauseButton := gtk.NewButtonWithLabel("󰏤")
	nextButton := gtk.NewButtonWithLabel("󰒭")
	prevButton.SetHasFrame(false)
	playPauseButton.SetHasFrame(false)
	nextButton.SetHasFrame(false)
	controls.Append(prevButton)
	controls.Append(playPauseButton)
	controls.Append(nextButton)
	header.Append(controls)

	content.Append(progressRow)

	volumeRow := gtk.NewBox(gtk.OrientationHorizontal, 0)
	volumeRow.SetName("music-menu-volume-row")
	volumeRow.SetHExpand(true)
	volumeRow.SetHAlign(gtk.AlignEnd)
	volumeRow.SetVisible(false)
	volumeBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	volumeBox.SetName("music-menu-volume")
	volumeBox.SetHAlign(gtk.AlignEnd)
	volumeButton := gtk.NewButtonWithLabel("󰕾")
	volumeButton.SetName("music-menu-volume-button")
	volumeButton.SetHasFrame(false)
	volumeSlider := gtk.NewScaleWithRange(gtk.OrientationHorizontal, 0, 100, 1)
	volumeSlider.SetName("music-menu-volume-slider")
	volumeSlider.SetDrawValue(false)
	volumeSlider.SetHExpand(false)
	volumeRevealer := gtk.NewRevealer()
	volumeRevealer.SetTransitionType(gtk.RevealerTransitionTypeSlideLeft)
	volumeRevealer.SetTransitionDuration(140)
	volumeRevealer.SetRevealChild(false)
	volumeRevealer.SetChild(volumeSlider)
	volumeBox.Append(volumeRevealer)
	volumeBox.Append(volumeButton)
	volumeRow.Append(volumeBox)
	content.Append(volumeRow)

	cavaWrap := gtk.NewBox(gtk.OrientationVertical, 0)
	cavaWrap.SetName("music-menu-cava-wrap")
	cavaWrap.SetHExpand(true)
	cavaWrap.SetVExpand(true)
	cavaWrap.SetVAlign(gtk.AlignEnd)
	cavaArea := gtk.NewDrawingArea()
	cavaArea.SetName("music-menu-cava")
	cavaArea.SetContentHeight(96)
	cavaArea.SetHExpand(true)
	cavaArea.SetVExpand(true)
	var cavaTargets []float64
	var cavaLevels []float64
	var lastCavaAnimation time.Time
	cavaArea.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, width, height int) {
		drawCavaBars(cr, width, height, cavaLevels)
	})
	cavaWrap.Append(cavaArea)
	foreground.Append(cavaWrap)

	popover.SetChild(menu)

	var current mprisInfo
	var updatingSlider bool
	var draggingSlider bool
	var lastCoverURL string
	var lastBackgroundURL string
	var widgetText string
	var marqueeStep int
	var lastPositionSnapshot time.Time
	var seekToken int
	var updatingVolume bool

	updateWidgetLabel := func() {
		label.SetLabel(marqueeText(widgetText, 28, marqueeStep))
	}

	updateProgressLabels := func(positionMicros int64, lengthMicros int64) {
		positionLabel.SetLabel(formatMediaTime(positionMicros))
		durationLabel.SetLabel(formatMediaTime(lengthMicros))
	}

	var requestRefresh func()
	applySeek := func(target int64) {
		if !canSeek(current) || current.Length <= 0 {
			log.Printf(
				"music seek skipped: player=%s bus=%s canSeek=%t length=%d position=%d target=%d track=%q",
				current.Player,
				current.Bus,
				current.CanSeek,
				current.Length,
				current.Position,
				target,
				string(current.TrackID),
			)
			return
		}

		target = int64(clamp(float64(target), 0, float64(current.Length)))
		log.Printf(
			"music seek request: player=%s bus=%s canSeek=%t length=%d position=%d target=%d track=%q",
			current.Player,
			current.Bus,
			current.CanSeek,
			current.Length,
			current.Position,
			target,
			string(current.TrackID),
		)
		current.Position = target
		lastPositionSnapshot = time.Now()
		seekPlayer(current.Bus, current.TrackID, target)
		requestRefresh()
	}

	scheduleSeek := func(target int64, delay time.Duration) {
		seekToken++
		token := seekToken
		target = int64(clamp(float64(target), 0, float64(current.Length)))

		run := func() {
			ui(func() {
				if token != seekToken {
					return
				}
				applySeek(target)
			})
		}

		if delay <= 0 {
			run()
			return
		}

		time.AfterFunc(delay, run)
	}

	refreshRequests := make(chan struct{}, 1)
	requestRefresh = func() {
		select {
		case refreshRequests <- struct{}{}:
		default:
		}
	}

	go func() {
		for range refreshRequests {
			info := readMPRIS()
			ui(func() {
				current = info
				lastPositionSnapshot = time.Now()
				if widgetText != info.Text {
					widgetText = info.Text
					marqueeStep = 0
				}
				updateWidgetLabel()
				box.SetVisible(strings.TrimSpace(widgetText) != "")

				if info.ArtURL != lastCoverURL {
					lastCoverURL = info.ArtURL
					if info.ArtURL != "" {
						cover.SetFile(gio.NewFileForURI(info.ArtURL))
						coverShell.SetVisible(true)
					} else {
						cover.SetPaintable(nil)
						coverShell.SetVisible(false)
					}
				}

				if info.Playing {
					box.AddCSSClass("playing")
					coverShell.AddCSSClass("playing")
					textShell.AddCSSClass("playing")
					playPauseButton.SetLabel("󰏥")
				} else {
					box.RemoveCSSClass("playing")
					coverShell.RemoveCSSClass("playing")
					textShell.RemoveCSSClass("playing")
					playPauseButton.SetLabel("󰐊")
				}

				titleLabel.SetLabel(strings.TrimSpace(info.Title))
				artistLabel.SetLabel(formatTrackSubtitle(info.Artist, info.Album))
				if info.ArtURL != lastBackgroundURL {
					lastBackgroundURL = info.ArtURL
					if info.ArtURL != "" {
						background.SetFile(gio.NewFileForURI(info.ArtURL))
					} else {
						background.SetPaintable(nil)
					}
				}

				updatingSlider = true
				if info.Length > 0 {
					slider.SetRange(0, float64(info.Length))
					if !draggingSlider {
						position := currentPlaybackPosition(info, lastPositionSnapshot)
						slider.SetValue(float64(position))
						updateProgressLabels(position, info.Length)
					}
				} else {
					slider.SetRange(0, 100)
					if !draggingSlider {
						slider.SetValue(0)
					}
					updateProgressLabels(0, 0)
				}
				slider.SetSensitive(canSeek(info))
				updatingSlider = false

				volumeRow.SetVisible(info.HasVolume)
				updatingVolume = true
				if info.HasVolume {
					volumeSlider.SetValue(clamp(info.Volume*100, 0, 100))
				} else {
					volumeSlider.SetValue(0)
				}
				updatingVolume = false
				volumeButton.SetLabel(volumeIcon(info.Volume))

				box.SetTooltipText(info.Player)
			})
		}
	}()

	startPolling(280*time.Millisecond, func() {
		ui(func() {
			if len([]rune(strings.TrimSpace(widgetText))) <= 28 {
				return
			}
			marqueeStep++
			updateWidgetLabel()
		})
	})

	startPolling(160*time.Millisecond, func() {
		ui(func() {
			if updatingSlider || draggingSlider || current.Length <= 0 || !current.Playing {
				return
			}

			position := currentPlaybackPosition(current, lastPositionSnapshot)
			updatingSlider = true
			slider.SetValue(float64(position))
			updatingSlider = false
			updateProgressLabels(position, current.Length)
		})
	})

	startCavaVisualizer(func(levels []int) {
		if len(levels) == 0 {
			return
		}

		if len(cavaTargets) != len(levels) {
			cavaTargets = make([]float64, len(levels))
			cavaLevels = make([]float64, len(levels))
		}

		for i, raw := range levels {
			cavaTargets[i] = clamp(float64(raw), 0, 11)
		}

		if lastCavaAnimation.IsZero() {
			lastCavaAnimation = time.Now()
		}

		cavaWrap.SetVisible(true)
	}, func(available bool) {
		if !available {
			cavaTargets = nil
			cavaLevels = nil
			lastCavaAnimation = time.Time{}
			cavaArea.QueueDraw()
		}
		cavaWrap.SetVisible(available)
	})

	startPolling(16*time.Millisecond, func() {
		ui(func() {
			if len(cavaLevels) == 0 || len(cavaLevels) != len(cavaTargets) {
				lastCavaAnimation = time.Time{}
				return
			}

			now := time.Now()
			dt := 1.0 / 60.0
			if !lastCavaAnimation.IsZero() {
				dt = clamp(now.Sub(lastCavaAnimation).Seconds(), 1.0/240.0, 0.05)
			}
			lastCavaAnimation = now

			const decay = 20.0
			changed := false
			for i := range cavaTargets {
				current := cavaLevels[i]
				target := cavaTargets[i]
				next := (current-target)*math.Exp(-decay*dt) + target
				if math.Abs(next-target) <= 0.01 {
					next = target
				}
				next = clamp(next, 0, 11)
				if math.Abs(next-current) > 0.001 {
					changed = true
				}
				cavaLevels[i] = next
			}

			if changed {
				cavaArea.QueueDraw()
			}
		})
	})

	attachClick(coverShell, func() { controlPlayer(current.Bus, "PlayPause") }, nil)
	attachHoverPopover(textShell, popover, nil, nil)
	attachScroll(box, func() { controlActivePlayer("Next") }, func() { controlActivePlayer("Previous") })

	prevButton.ConnectClicked(func() { controlPlayer(current.Bus, "Previous") })
	playPauseButton.ConnectClicked(func() { controlPlayer(current.Bus, "PlayPause") })
	nextButton.ConnectClicked(func() { controlPlayer(current.Bus, "Next") })
	slider.ConnectValueChanged(func() {
		if updatingSlider || !canSeek(current) || current.Length <= 0 {
			return
		}

		target := int64(clamp(slider.Value(), 0, float64(current.Length)))
		updateProgressLabels(target, current.Length)
		if draggingSlider {
			scheduleSeek(target, 120*time.Millisecond)
			return
		}

		scheduleSeek(target, 0)
	})
	sliderClick := gtk.NewGestureClick()
	sliderClickBase := gtk.BaseEventController(sliderClick)
	sliderClickBase.SetPropagationPhase(gtk.PhaseCapture)
	sliderClickBase.SetPropagationLimit(gtk.LimitNone)
	sliderClick.ConnectPressed(func(_ int, _, _ float64) {
		draggingSlider = true
	})
	sliderClick.ConnectReleased(func(_ int, _, _ float64) {
		draggingSlider = false
		if updatingSlider || !canSeek(current) {
			return
		}

		target := int64(clamp(slider.Value(), 0, float64(current.Length)))
		scheduleSeek(target, 0)
	})
	sliderClick.ConnectUnpairedRelease(func(_, _ float64, _ uint, _ *gdk.EventSequence) {
		draggingSlider = false
	})
	slider.AddController(sliderClick)

	volumeMotion := gtk.NewEventControllerMotion()
	volumeMotion.ConnectEnter(func(_, _ float64) {
		volumeRevealer.SetRevealChild(true)
	})
	volumeMotion.ConnectLeave(func() {
		volumeRevealer.SetRevealChild(false)
	})
	volumeBox.AddController(volumeMotion)

	volumeSlider.ConnectValueChanged(func() {
		if updatingVolume || !current.HasVolume || strings.TrimSpace(current.Bus) == "" {
			return
		}

		target := clamp(volumeSlider.Value(), 0, 100) / 100.0
		current.Volume = target
		volumeButton.SetLabel(volumeIcon(target))
		setPlayerVolume(current.Bus, target)
	})

	requestRefresh()
	startPolling(time.Second, requestRefresh)
	go func() {
		for range subscribeMPRISEvents() {
			requestRefresh()
		}
	}()

	return box
}

func NewMedia() gtk.Widgetter {
	module := newTextModule("custom-media")
	refreshRequests := make(chan struct{}, 1)
	requestRefresh := func() {
		select {
		case refreshRequests <- struct{}{}:
		default:
		}
	}

	go func() {
		for range refreshRequests {
			info := readMPRIS()
			text := ""
			if info.Player != "" {
				text = fmt.Sprintf("%s %s", strings.ToUpper(info.Player), truncate(info.Text, 20))
			}
			ui(func() { setTextModule(module, text) })
		}
	}()

	requestRefresh()
	go func() {
		for range subscribeMPRISEvents() {
			requestRefresh()
		}
	}()

	return module.Box
}

func NewMPD() gtk.Widgetter {
	module := newTextModule("mpd")
	startPolling(5*time.Second, func() {
		ui(func() { setTextModule(module, readMPD()) })
	})
	return module.Box
}

func readMPRIS() mprisInfo {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return mprisInfo{}
	}
	defer conn.Close()

	var names []string
	if err := conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return mprisInfo{}
	}

	best := mprisInfo{}
	for _, name := range names {
		if !strings.HasPrefix(name, "org.mpris.MediaPlayer2.") {
			continue
		}

		obj := conn.Object(name, "/org/mpris/MediaPlayer2")
		statusVar, err := obj.GetProperty("org.mpris.MediaPlayer2.Player.PlaybackStatus")
		if err != nil {
			continue
		}
		metaVar, err := obj.GetProperty("org.mpris.MediaPlayer2.Player.Metadata")
		if err != nil {
			continue
		}

		status, _ := statusVar.Value().(string)
		metadata, _ := metaVar.Value().(map[string]dbus.Variant)
		artist := firstString(metadata["xesam:artist"])
		album := mediaVariantString(metadata["xesam:album"])
		title := mediaVariantString(metadata["xesam:title"])
		artURL := mediaVariantString(metadata["mpris:artUrl"])
		trackID := mediaVariantObjectPath(metadata["mpris:trackid"])
		length := mediaVariantInt64(metadata["mpris:length"])
		position := propertyInt64(obj, "org.mpris.MediaPlayer2.Player.Position")
		canSeek := propertyBool(obj, "org.mpris.MediaPlayer2.Player.CanSeek")
		volume, hasVolume := propertyFloat64(obj, "org.mpris.MediaPlayer2.Player.Volume")
		player := strings.TrimPrefix(name, "org.mpris.MediaPlayer2.")

		text := strings.TrimSpace(strings.TrimSpace(artist) + " - " + strings.TrimSpace(title))
		if text == "-" {
			text = title
		}

		candidate := mprisInfo{
			Bus:       name,
			Player:    player,
			Artist:    artist,
			Album:     album,
			Title:     title,
			Text:      text,
			ArtURL:    artURL,
			Playing:   strings.EqualFold(status, "Playing"),
			CanSeek:   canSeek,
			HasVolume: hasVolume,
			TrackID:   trackID,
			Length:    length,
			Position:  position,
			Volume:    volume,
		}

		if best.Bus == "" {
			best = candidate
		}
		if candidate.Playing && !best.Playing {
			best = candidate
			continue
		}
		if candidate.Playing == best.Playing && strings.EqualFold(candidate.Player, "spotify") && !strings.EqualFold(best.Player, "spotify") {
			best = candidate
		}
	}

	return best
}

func controlActivePlayer(action string) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return
	}
	defer conn.Close()

	var names []string
	if err := conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return
	}

	for _, name := range names {
		if !strings.HasPrefix(name, "org.mpris.MediaPlayer2.") {
			continue
		}
		obj := conn.Object(name, "/org/mpris/MediaPlayer2")
		_ = obj.Call("org.mpris.MediaPlayer2.Player."+action, 0).Err
		return
	}
}

func controlPlayer(bus string, action string) {
	if strings.TrimSpace(bus) == "" {
		controlActivePlayer(action)
		return
	}

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return
	}
	defer conn.Close()

	obj := conn.Object(bus, "/org/mpris/MediaPlayer2")
	_ = obj.Call("org.mpris.MediaPlayer2.Player."+action, 0).Err
}

func seekPlayer(bus string, trackID dbus.ObjectPath, targetPosition int64) {
	if strings.TrimSpace(bus) == "" {
		log.Printf("music seek failed: empty bus target=%d track=%q", targetPosition, string(trackID))
		return
	}

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Printf("music seek connect failed: bus=%s target=%d track=%q err=%v", bus, targetPosition, string(trackID), err)
		return
	}
	defer conn.Close()

	obj := conn.Object(bus, "/org/mpris/MediaPlayer2")
	if trackID != "" && trackID.IsValid() {
		if err := obj.Call("org.mpris.MediaPlayer2.Player.SetPosition", 0, trackID, targetPosition).Err; err == nil {
			log.Printf("music seek SetPosition ok: bus=%s target=%d track=%q", bus, targetPosition, string(trackID))
			return
		} else {
			log.Printf("music seek SetPosition failed: bus=%s target=%d track=%q err=%v", bus, targetPosition, string(trackID), err)
		}
	} else {
		log.Printf("music seek SetPosition skipped: bus=%s target=%d invalid track=%q", bus, targetPosition, string(trackID))
	}

	currentPosition := propertyInt64(obj, "org.mpris.MediaPlayer2.Player.Position")
	offset := targetPosition - currentPosition
	if err := obj.Call("org.mpris.MediaPlayer2.Player.Seek", 0, offset).Err; err != nil {
		log.Printf("music seek Seek failed: bus=%s target=%d current=%d offset=%d err=%v", bus, targetPosition, currentPosition, offset, err)
		return
	}

	log.Printf("music seek Seek ok: bus=%s target=%d current=%d offset=%d", bus, targetPosition, currentPosition, offset)
}

func setPlayerVolume(bus string, volume float64) {
	if strings.TrimSpace(bus) == "" {
		return
	}

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return
	}
	defer conn.Close()

	obj := conn.Object(bus, "/org/mpris/MediaPlayer2")
	_ = obj.Call(
		"org.freedesktop.DBus.Properties.Set",
		0,
		"org.mpris.MediaPlayer2.Player",
		"Volume",
		dbus.MakeVariant(clamp(volume, 0, 1)),
	).Err
}

func currentPlaybackPosition(info mprisInfo, snapshot time.Time) int64 {
	if info.Length <= 0 {
		return 0
	}

	position := info.Position
	if info.Playing && !snapshot.IsZero() {
		position += time.Since(snapshot).Microseconds()
	}

	if position < 0 {
		position = 0
	}
	if position > info.Length {
		position = info.Length
	}
	return position
}

func propertyBool(obj dbus.BusObject, name string) bool {
	value, err := obj.GetProperty(name)
	if err != nil {
		return false
	}
	boolean, _ := value.Value().(bool)
	return boolean
}

func propertyInt64(obj dbus.BusObject, name string) int64 {
	value, err := obj.GetProperty(name)
	if err != nil {
		return 0
	}
	switch v := value.Value().(type) {
	case int64:
		return v
	case int32:
		return int64(v)
	case uint64:
		return int64(v)
	case uint32:
		return int64(v)
	default:
		return 0
	}
}

func propertyFloat64(obj dbus.BusObject, name string) (float64, bool) {
	value, err := obj.GetProperty(name)
	if err != nil {
		return 0, false
	}

	switch v := value.Value().(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case uint32:
		return float64(v), true
	default:
		return 0, false
	}
}

func mediaVariantString(variant dbus.Variant) string {
	switch value := variant.Value().(type) {
	case string:
		return value
	default:
		return ""
	}
}

func mediaVariantObjectPath(variant dbus.Variant) dbus.ObjectPath {
	switch value := variant.Value().(type) {
	case dbus.ObjectPath:
		return value
	case string:
		return dbus.ObjectPath(value)
	default:
		return ""
	}
}

func mediaVariantInt64(variant dbus.Variant) int64 {
	switch value := variant.Value().(type) {
	case int64:
		return value
	case int32:
		return int64(value)
	case uint64:
		return int64(value)
	case uint32:
		return int64(value)
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func sliderTargetPosition(sliderValue float64, length int64) int64 {
	if length <= 0 {
		return 0
	}
	return int64(clamp(sliderValue, 0, 100) / 100.0 * float64(length))
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func canSeek(info mprisInfo) bool {
	if strings.TrimSpace(info.Bus) == "" {
		return false
	}
	return info.CanSeek || info.Length > 0
}

func firstString(variant dbus.Variant) string {
	if variant.Value() == nil {
		return ""
	}

	switch value := variant.Value().(type) {
	case []string:
		if len(value) > 0 {
			return value[0]
		}
	case []any:
		if len(value) > 0 {
			if s, ok := value[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

func truncate(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max-3]) + "..."
}

func formatMediaTime(positionMicros int64) string {
	if positionMicros <= 0 {
		return "0:00"
	}

	totalSeconds := positionMicros / int64(time.Second/time.Microsecond)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func volumeIcon(volume float64) string {
	switch {
	case volume <= 0.001:
		return "󰖁"
	case volume < 0.35:
		return "󰕿"
	case volume < 0.7:
		return "󰖀"
	default:
		return "󰕾"
	}
}

func formatTrackSubtitle(artist string, album string) string {
	artist = strings.TrimSpace(artist)
	album = strings.TrimSpace(album)
	switch {
	case artist != "" && album != "":
		return artist + " · " + album
	case artist != "":
		return artist
	default:
		return album
	}
}

func marqueeText(value string, width int, step int) string {
	value = strings.TrimSpace(value)
	if width <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}

	window := width - 1
	padding := []rune("   ")
	cycle := append(append([]rune{}, runes...), padding...)
	cycle = append(cycle, runes...)

	limit := len(runes) + len(padding)
	if limit <= 0 {
		return value
	}
	offset := step % limit
	segment := cycle[offset : offset+window]
	return string(segment) + "…"
}

func startCavaVisualizer(onFrame func([]int), onAvailability func(bool)) {
	go func() {
		if _, err := exec.LookPath("cava"); err != nil {
			ui(func() { onAvailability(false) })
			return
		}

		for {
			configPath, err := writeCavaConfig()
			if err != nil {
				ui(func() { onAvailability(false) })
				return
			}

			cmd := exec.Command("cava", "-p", configPath)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				_ = os.Remove(configPath)
				ui(func() { onAvailability(false) })
				return
			}

			if err := cmd.Start(); err != nil {
				_ = os.Remove(configPath)
				ui(func() { onAvailability(false) })
				return
			}

			ui(func() { onAvailability(true) })

			scanner := bufio.NewScanner(stdout)
			scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
			for scanner.Scan() {
				frame := parseCavaFrame(scanner.Text())
				ui(func() {
					onFrame(frame)
				})
			}

			_ = cmd.Wait()
			_ = os.Remove(configPath)
			ui(func() {
				onAvailability(false)
			})
			time.Sleep(2 * time.Second)
		}
	}()
}

func writeCavaConfig() (string, error) {
	file, err := os.CreateTemp("", "statusbar-cava-*.conf")
	if err != nil {
		return "", err
	}
	defer file.Close()

	config := `[general]
bars = 40
framerate = 60
autosens = 1

[input]
source = auto

[output]
method = raw
raw_target = /dev/stdout
data_format = ascii
ascii_max_range = 11
bar_delimiter = 59
frame_delimiter = 10
channels = mono
`
	if _, err := file.WriteString(config); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func parseCavaFrame(line string) []int {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	parts := strings.Split(line, ";")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		if value < 0 {
			value = 0
		}
		if value > 11 {
			value = 11
		}
		values = append(values, value)
	}
	return values
}

func drawCavaBars(cr *cairo.Context, width int, height int, levels []float64) {
	if width <= 0 || height <= 0 {
		return
	}

	cr.SetSourceRGBA(0, 0, 0, 0)
	cr.Paint()

	if len(levels) == 0 {
		return
	}

	const maxLevel = 11.0
	gap := 1.0
	barWidth := (float64(width) - gap*float64(len(levels)-1)) / float64(len(levels))
	if barWidth < 1 {
		gap = 0
		barWidth = 1
	}

	for i, raw := range levels {
		level := clamp(raw, 0, maxLevel) / maxLevel
		x := float64(i) * (barWidth + gap)

		if level <= 0.001 {
			cr.SetSourceRGBA(1, 1, 1, 0.03)
			cr.Rectangle(x, float64(height)-2, barWidth, 2)
			cr.Fill()
			continue
		}

		barHeight := math.Max(2, level*float64(height))
		y := float64(height) - barHeight

		bottomColor, midColor, topColor := cavaBarColors(level)
		pattern, err := cairo.NewPatternLinear(x, y, x, float64(height))
		if err == nil {
			_ = pattern.AddColorStopRGBA(0, topColor.r, topColor.g, topColor.b, topColor.a)
			_ = pattern.AddColorStopRGBA(0.58, midColor.r, midColor.g, midColor.b, midColor.a)
			_ = pattern.AddColorStopRGBA(1, bottomColor.r, bottomColor.g, bottomColor.b, bottomColor.a)
			cr.SetSource(pattern)
		} else {
			cr.SetSourceRGBA(topColor.r, topColor.g, topColor.b, topColor.a)
		}
		cr.Rectangle(x, y, barWidth, barHeight)
		cr.Fill()

		headHeight := clamp(2+level*4, 2, 6)
		if headHeight > barHeight {
			headHeight = barHeight
		}
		cr.SetSourceRGBA(topColor.r, topColor.g, topColor.b, 0.35+level*0.45)
		cr.Rectangle(x, y, barWidth, headHeight)
		cr.Fill()
	}
}

func mixCavaColor(a cavaColor, b cavaColor, t float64) cavaColor {
	t = clamp(t, 0, 1)
	return cavaColor{
		r: a.r + (b.r-a.r)*t,
		g: a.g + (b.g-a.g)*t,
		b: a.b + (b.b-a.b)*t,
		a: a.a + (b.a-a.a)*t,
	}
}

func paletteCavaColor(level float64, low cavaColor, mid cavaColor, high cavaColor) cavaColor {
	level = clamp(level, 0, 1)
	if level <= 0.5 {
		return mixCavaColor(low, mid, level*2)
	}
	return mixCavaColor(mid, high, (level-0.5)*2)
}

func cavaBarColors(level float64) (cavaColor, cavaColor, cavaColor) {
	level = math.Pow(clamp(level, 0, 1), 0.85)

	bottom := paletteCavaColor(level,
		cavaColor{r: 1.00, g: 1.00, b: 1.00, a: 0.30},
		cavaColor{r: 0.75, g: 0.55, b: 0.55, a: 0.48},
		cavaColor{r: 0.00, g: 0.00, b: 1.00, a: 0.88},
	)
	middle := paletteCavaColor(level,
		cavaColor{r: 1.00, g: 1.00, b: 1.00, a: 0.55},
		cavaColor{r: 0.98, g: 0.78, b: 0.60, a: 0.78},
		cavaColor{r: 1.00, g: 0.80, b: 0.00, a: 0.96},
	)
	top := paletteCavaColor(level,
		cavaColor{r: 1.00, g: 1.00, b: 1.00, a: 0.88},
		cavaColor{r: 1.00, g: 0.70, b: 0.60, a: 0.96},
		cavaColor{r: 1.00, g: 0.34, b: 0.20, a: 1.00},
	)

	return bottom, middle, top
}

func readMPD() string {
	conn, err := netDialTimeout("127.0.0.1:6600", 1200*time.Millisecond)
	if err != nil {
		return ""
	}
	defer conn.Close()

	_, _ = conn.Write([]byte("status\ncurrentsong\nclose\n"))
	buffer := make([]byte, 4096)
	n, err := conn.Read(buffer)
	if err != nil {
		return ""
	}

	response := string(buffer[:n])
	if !strings.Contains(response, "OK MPD") {
		return ""
	}

	var artist, album, title, state string
	for _, line := range strings.Split(response, "\n") {
		switch {
		case strings.HasPrefix(line, "Artist: "):
			artist = strings.TrimPrefix(line, "Artist: ")
		case strings.HasPrefix(line, "Album: "):
			album = strings.TrimPrefix(line, "Album: ")
		case strings.HasPrefix(line, "Title: "):
			title = strings.TrimPrefix(line, "Title: ")
		case strings.HasPrefix(line, "state: "):
			state = strings.TrimPrefix(line, "state: ")
		}
	}

	if state == "" {
		return ""
	}
	if title == "" {
		title = "Stopped"
	}

	icon := "MPD"
	if state == "play" {
		icon = ""
	} else if state == "pause" {
		icon = ""
	}

	return truncate(strings.TrimSpace(fmt.Sprintf("%s %s - %s - %s ", icon, artist, album, title)), 48)
}
