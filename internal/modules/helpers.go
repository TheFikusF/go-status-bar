package modules

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type TextModule struct {
	Box   *gtk.Box
	Label *gtk.Label
}

func newTextModule(name string) TextModule {
	box := gtk.NewBox(gtk.OrientationHorizontal, 0)
	box.SetName(name)
	box.AddCSSClass("module")
	box.SetVisible(false)

	label := gtk.NewLabel("")
	box.Append(label)

	return TextModule{Box: box, Label: label}
}

func setTextModule(module TextModule, text string) {
	text = strings.TrimSpace(text)
	module.Label.SetLabel(text)
	module.Box.SetVisible(text != "")
}

func startPolling(interval time.Duration, fn func()) {
	go func() {
		fn()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			fn()
		}
	}()
}

func ui(fn func()) {
	glib.IdleAdd(func() {
		fn()
	})
}

func runCommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

func runCommandStdin(stdin []byte, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	return cmd.Run()
}

func runDetached(name string, args ...string) {
	go func() {
		cmd := exec.Command(name, args...)
		if err := cmd.Start(); err != nil {
			log.Printf("start %s: %v", name, err)
		}
	}()
}

func attachClick(widget gtk.Widgetter, left, right func()) {
	click := gtk.NewGestureClick()
	click.ConnectPressed(func(nPress int, x, y float64) {
		switch click.CurrentButton() {
		case 1:
			if left != nil {
				left()
			}
		case 3:
			if right != nil {
				right()
			}
		}
	})

	widget.(interface{ AddController(gtk.EventControllerer) }).AddController(click)
}

func attachHoverPopover(anchor gtk.Widgetter, popover *gtk.Popover, rightClick func(), beforeOpen func()) (func(), func(bool)) {
	popover.SetAutohide(false)
	popover.SetCascadePopdown(false)
	anchorWidget := gtk.BaseWidget(anchor)
	popupArea := popover.Child()
	if popupArea == nil {
		popupArea = popover
	}

	var overAnchor bool
	var overPopup bool
	var interacting bool
	var keyHeld bool
	var closeToken int
	var visible bool

	open := func() {
		closeToken++
		if visible {
			return
		}

		if root := anchorWidget.Root(); root != nil {
			if popover.Parent() != root {
				if popover.Parent() != nil {
					popover.Unparent()
				}
				popover.SetParent(root)
			}

			x, y, ok := anchorWidget.TranslateCoordinates(root, 0, 0)
			if ok {
				rect := gdk.NewRectangle(int(x), int(y), anchorWidget.AllocatedWidth(), anchorWidget.AllocatedHeight())
				popover.SetPointingTo(&rect)
			}
		}

		if beforeOpen != nil {
			beforeOpen()
		}
		popover.Popup()
		visible = true
	}

	closeIfNeeded := func() {
		if overAnchor || overPopup || interacting || keyHeld {
			return
		}
		if visible {
			popover.Popdown()
			visible = false
		}
	}

	scheduleClose := func(delay time.Duration) {
		closeToken++
		token := closeToken
		run := func() {
			ui(func() {
				if token != closeToken {
					return
				}
				closeIfNeeded()
			})
		}
		if delay <= 0 {
			run()
			return
		}
		time.AfterFunc(delay, run)
	}

	anchorMotion := gtk.NewEventControllerMotion()
	anchorMotion.ConnectEnter(func(_, _ float64) {
		overAnchor = true
		open()
	})
	anchorMotion.ConnectLeave(func() {
		overAnchor = false
		scheduleClose(90 * time.Millisecond)
	})
	anchor.(interface{ AddController(gtk.EventControllerer) }).AddController(anchorMotion)

	popupMotion := gtk.NewEventControllerMotion()
	popupMotionBase := gtk.BaseEventController(popupMotion)
	popupMotionBase.SetPropagationPhase(gtk.PhaseCapture)
	popupMotionBase.SetPropagationLimit(gtk.LimitNone)
	popupMotion.ConnectEnter(func(_, _ float64) {
		overPopup = true
		open()
	})
	popupMotion.ConnectLeave(func() {
		overPopup = false
		scheduleClose(90 * time.Millisecond)
	})
	popupArea.(interface{ AddController(gtk.EventControllerer) }).AddController(popupMotion)

	popupClick := gtk.NewGestureClick()
	popupClick.SetButton(0)
	popupClickBase := gtk.BaseEventController(popupClick)
	popupClickBase.SetPropagationPhase(gtk.PhaseCapture)
	popupClickBase.SetPropagationLimit(gtk.LimitNone)
	popupClick.ConnectPressed(func(_ int, _, _ float64) {
		interacting = true
		open()
	})
	popupClick.ConnectReleased(func(_ int, _, _ float64) {
		interacting = false
		scheduleClose(0)
	})
	popupClick.ConnectUnpairedRelease(func(_, _ float64, _ uint, _ *gdk.EventSequence) {
		interacting = false
		scheduleClose(0)
	})
	popupArea.(interface{ AddController(gtk.EventControllerer) }).AddController(popupClick)

	popover.ConnectClosed(func() {
		visible = false
		overPopup = false
		interacting = false
	})

	click := gtk.NewGestureClick()
	click.SetButton(0)
	click.ConnectPressed(func(_ int, _, _ float64) {
		if click.CurrentButton() == 3 && rightClick != nil {
			rightClick()
		}
	})
	anchor.(interface{ AddController(gtk.EventControllerer) }).AddController(click)

	setHeld := func(held bool) {
		keyHeld = held
		if held {
			open()
		} else {
			scheduleClose(0)
		}
	}
	return open, setHeld
}

func attachScroll(widget gtk.Widgetter, onUp, onDown func()) {
	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollVertical | gtk.EventControllerScrollDiscrete)
	scroll.ConnectScroll(func(dx, dy float64) bool {
		switch {
		case dy < 0:
			if onUp != nil {
				onUp()
			}
		case dy > 0:
			if onDown != nil {
				onDown()
			}
		}
		return true
	})

	widget.(interface{ AddController(gtk.EventControllerer) }).AddController(scroll)
}

func removeChildren(box *gtk.Box) {
	for child := box.FirstChild(); child != nil; child = box.FirstChild() {
		box.Remove(child)
	}
}

func readFirstExisting(paths ...string) string {
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}

func fetchJSON(url string, target any) error {
	client := http.Client{Timeout: 4 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "statusbar/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return err
	}
	return json.Unmarshal(buf.Bytes(), target)
}

func netDialTimeout(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, timeout)
}
