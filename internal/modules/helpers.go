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
	"github.com/diamondburned/gotk4/pkg/graphene"
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

// ---------------------------------------------------------------------------
// Popup – robust hover-popover with proper lifecycle management.
// ---------------------------------------------------------------------------

// popupRegistry tracks all live Popup instances for mutual exclusion.
// Using a map allows O(1) removal when a Popup is destroyed, preventing the
// unbounded growth that caused the original memory leak.
var (
	popupNextID   int
	popupRegistry = map[int]*Popup{}
)

// Popup represents a hover-triggered popover attached to an anchor widget.
// All methods must be called on the GTK main thread.
type Popup struct {
	id         int
	popover    *gtk.Popover
	anchor     gtk.Widgetter
	anchorBase *gtk.Widget

	beforeOpen    func()
	rightClick    func()
	bindPopupArea func(gtk.Widgetter)

	overAnchor  bool
	overPopup   bool
	interacting bool
	keyHeld     bool
	visible     bool
	closeToken  int
	destroyed   bool
}

func closeAllPopups() {
	targets := make([]*Popup, 0, len(popupRegistry))
	for _, p := range popupRegistry {
		targets = append(targets, p)
	}
	for _, p := range targets {
		p.Close()
	}
}

func attachHoverPopover(anchor gtk.Widgetter, popover *gtk.Popover, rightClick func(), beforeOpen func()) *Popup {
	popover.SetAutohide(false)
	popover.SetCascadePopdown(false)

	p := &Popup{
		popover:    popover,
		anchor:     anchor,
		anchorBase: gtk.BaseWidget(anchor),
		beforeOpen: beforeOpen,
		rightClick: rightClick,
	}

	// Register in global map.
	popupNextID++
	p.id = popupNextID
	popupRegistry[p.id] = p

	// --- Popup-area motion tracking ---
	// Track motion on the popover's child widget (the actual content box) rather
	// than the popover surface itself.  GTK4 on Wayland renders popovers as
	// native xdg_popup surfaces and enter/leave events on the surface boundary
	// are unreliable during content refreshes.  Tracking the child avoids this.
	// bindPopupArea is called once at setup and again on each Open() so it
	// follows child replacements (SetChild).
	var popupArea gtk.Widgetter
	bindPopupArea := func(widget gtk.Widgetter) {
		if widget == nil {
			widget = popover
		}
		if popupArea == widget {
			return
		}
		popupArea = widget

		motion := gtk.NewEventControllerMotion()
		mBase := gtk.BaseEventController(motion)
		mBase.SetPropagationPhase(gtk.PhaseCapture)
		mBase.SetPropagationLimit(gtk.LimitNone)
		motion.ConnectEnter(func(_, _ float64) {
			if p.destroyed {
				return
			}
			p.overPopup = true
			p.Open()
		})
		motion.ConnectLeave(func() {
			if p.destroyed {
				return
			}
			p.overPopup = false
			p.scheduleClose(90 * time.Millisecond)
		})
		popupArea.(interface{ AddController(gtk.EventControllerer) }).AddController(motion)
	}
	p.bindPopupArea = bindPopupArea

	bindPopupArea(popover.Child())

	// --- Anchor motion ---
	anchorMotion := gtk.NewEventControllerMotion()
	anchorMotion.ConnectEnter(func(_, _ float64) {
		if p.destroyed {
			return
		}
		p.overAnchor = true
		p.Open()
	})
	anchorMotion.ConnectLeave(func() {
		if p.destroyed {
			return
		}
		p.overAnchor = false
		p.scheduleClose(90 * time.Millisecond)
	})
	anchor.(interface{ AddController(gtk.EventControllerer) }).AddController(anchorMotion)

	// --- Click tracking inside popover ---
	popupClick := gtk.NewGestureClick()
	popupClick.SetButton(0)
	pcBase := gtk.BaseEventController(popupClick)
	pcBase.SetPropagationPhase(gtk.PhaseCapture)
	pcBase.SetPropagationLimit(gtk.LimitNone)
	popupClick.ConnectPressed(func(_ int, _, _ float64) {
		if p.destroyed {
			return
		}
		p.overPopup = true
		p.interacting = true
		p.Open()
	})
	popupClick.ConnectReleased(func(_ int, _, _ float64) {
		if p.destroyed {
			return
		}
		p.interacting = false
		p.scheduleClose(90 * time.Millisecond)
	})
	popupClick.ConnectUnpairedRelease(func(_, _ float64, _ uint, _ *gdk.EventSequence) {
		if p.destroyed {
			return
		}
		p.interacting = false
		p.scheduleClose(90 * time.Millisecond)
	})
	popover.AddController(popupClick)

	// --- Popover closed signal ---
	popover.ConnectClosed(func() {
		p.visible = false
		p.overPopup = false
		p.interacting = false
	})

	// --- Right-click on anchor ---
	click := gtk.NewGestureClick()
	click.SetButton(0)
	click.ConnectPressed(func(_ int, _, _ float64) {
		if p.destroyed {
			return
		}
		if click.CurrentButton() == 3 && p.rightClick != nil {
			p.rightClick()
		}
	})
	anchor.(interface{ AddController(gtk.EventControllerer) }).AddController(click)

	return p
}

// Open shows the popover, closing all other popups first.
func (p *Popup) Open() {
	if p.destroyed {
		return
	}
	p.closeToken++
	if p.visible {
		return
	}

	closeAllPopups()

	if root := p.anchorBase.Root(); root != nil {
		if p.popover.Parent() != root {
			if p.popover.Parent() != nil {
				p.popover.Unparent()
			}
			p.popover.SetParent(root)
		}

		x, y, ok := p.anchorBase.TranslateCoordinates(root, 0, 0)
		if ok {
			rect := gdk.NewRectangle(int(x), int(y), p.anchorBase.AllocatedWidth(), p.anchorBase.AllocatedHeight())
			p.popover.SetPointingTo(&rect)
		}
	}

	if p.beforeOpen != nil {
		p.beforeOpen()
	}
	p.bindPopupArea(p.popover.Child())
	p.popover.Popup()
	p.visible = true
}

// Close hides the popover immediately and resets interaction state.
func (p *Popup) Close() {
	if p.destroyed {
		return
	}
	if p.visible {
		p.overAnchor = false
		p.overPopup = false
		p.interacting = false
		p.keyHeld = false
		p.popover.Popdown()
		p.visible = false
	}
}

// SetHeld marks the popup as held open (e.g. by a modifier key).
func (p *Popup) SetHeld(held bool) {
	if p.destroyed {
		return
	}
	p.keyHeld = held
	if held {
		p.Open()
	} else {
		p.scheduleClose(0)
	}
}

// Destroy permanently tears down the popup: closes it, removes it from
// the global registry, and unparents the popover widget. After Destroy
// the Popup must not be used.
func (p *Popup) Destroy() {
	if p.destroyed {
		return
	}
	p.destroyed = true
	if p.visible {
		p.popover.Popdown()
		p.visible = false
	}
	delete(popupRegistry, p.id)
	if p.popover.Parent() != nil {
		p.popover.Unparent()
	}
}

func (p *Popup) closeIfNeeded() {
	if p.destroyed {
		return
	}
	p.overAnchor = widgetContainsPointer(p.anchor)
	p.overPopup = widgetContainsPointer(p.popover)
	// Safety: if cursor is outside both widgets, clear interacting. This
	// handles the case where a child widget is removed mid-click and the
	// released signal never fires, leaving interacting stuck true.
	if !p.overAnchor && !p.overPopup {
		p.interacting = false
	}
	if p.overAnchor || p.overPopup || p.interacting || p.keyHeld {
		return
	}
	if p.visible {
		p.popover.Popdown()
		p.visible = false
	}
}

func (p *Popup) scheduleClose(delay time.Duration) {
	if p.destroyed {
		return
	}
	p.closeToken++
	token := p.closeToken
	run := func() {
		ui(func() {
			if p.destroyed || token != p.closeToken {
				return
			}
			p.closeIfNeeded()
		})
	}
	if delay <= 0 {
		run()
		return
	}
	time.AfterFunc(delay, run)
}

func widgetContainsPointer(widget gtk.Widgetter) bool {
	if widget == nil {
		return false
	}

	base := gtk.BaseWidget(widget)
	native := base.Native()
	if native == nil {
		return false
	}

	surface := native.Surface()
	if surface == nil {
		return false
	}

	display := gdk.BaseSurface(surface).Display()
	if display == nil {
		return false
	}

	seat := display.DefaultSeat()
	if seat == nil {
		return false
	}

	pointer := gdk.BaseSeat(seat).Pointer()
	if pointer == nil {
		return false
	}

	x, y, _, ok := gdk.BaseSurface(surface).DevicePosition(pointer)
	if !ok {
		return false
	}

	origin, ok := base.ComputePoint(native, graphene.NewPointAlloc().Init(0, 0))
	if !ok {
		return false
	}

	relX := x - float64(origin.X())
	relY := y - float64(origin.Y())
	return base.Contains(relX, relY)
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
