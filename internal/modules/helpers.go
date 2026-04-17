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
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/graphene"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/godbus/dbus/v5"
)

func init() {
	// Periodically trigger Go GC to finalize gotk4 GObject wrappers.
	go func() {
		for {
			time.Sleep(30 * time.Second)
			runtime.GC()
		}
	}()
}

// sharedSessionBus returns a cached D-Bus session bus connection.
// The connection is created on first use and reused for all subsequent calls.
// If the connection becomes invalid it is re-established transparently.
var (
	sessionBusMu   sync.Mutex
	sessionBusConn *dbus.Conn
)

func sharedSessionBus() (*dbus.Conn, error) {
	sessionBusMu.Lock()
	defer sessionBusMu.Unlock()

	if sessionBusConn != nil {
		return sessionBusConn, nil
	}

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	// Re-establish if the daemon closes the connection.
	go func() {
		<-conn.Context().Done()
		sessionBusMu.Lock()
		if sessionBusConn == conn {
			sessionBusConn = nil
		}
		sessionBusMu.Unlock()
	}()
	sessionBusConn = conn
	return conn, nil
}

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

func startPolling(interval time.Duration, fn func()) func() {
	stop := make(chan struct{})
	go func() {
		fn()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fn()
			case <-stop:
				return
			}
		}
	}()
	return sync.OnceFunc(func() { close(stop) })
}

func ui(fn func()) {
	glib.IdleAdd(fn)
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
// Popup – hover-popover with lifecycle management.
//
// Key design decisions to avoid memory leaks:
//   - All event controllers are attached once at creation and removed on
//     Destroy(). No controllers are created or replaced during open/close.
//   - Motion is tracked on the popover widget itself (not its child), so
//     child replacements via SetChild() don't create new controllers.
//   - A generation counter (closeGen) debounces close checks. When motion
//     events are unreliable (Wayland xdg_popup surfaces), a polling fallback
//     uses widgetContainsPointer to determine whether the cursor is still
//     inside the anchor or popover.
//   - The popover is re-parented to the window root once (not every open).
// ---------------------------------------------------------------------------

var (
	popupNextID   int
	popupRegistry = map[int]*Popup{}
)

type Popup struct {
	id      int
	popover *gtk.Popover
	anchor  *gtk.Widget

	beforeOpen func()
	afterClose func()
	rightClick func()

	controllers []controllerBinding

	visible    bool
	destroyed  bool
	closeGen   int
	keyHeld    bool
	reparented bool
}

type controllerBinding struct {
	widget     gtk.Widgetter
	controller gtk.EventControllerer
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
		anchor:     gtk.BaseWidget(anchor),
		beforeOpen: beforeOpen,
		rightClick: rightClick,
	}

	popupNextID++
	p.id = popupNextID
	popupRegistry[p.id] = p

	// --- Anchor motion: open on enter, schedule close on leave ---
	anchorMotion := gtk.NewEventControllerMotion()
	anchorMotion.ConnectEnter(func(_, _ float64) {
		if !p.destroyed {
			p.Open()
		}
	})
	anchorMotion.ConnectLeave(func() {
		if !p.destroyed {
			p.scheduleClose(90 * time.Millisecond)
		}
	})
	anchor.(interface{ AddController(gtk.EventControllerer) }).AddController(anchorMotion)
	p.controllers = append(p.controllers, controllerBinding{anchor, anchorMotion})

	// --- Popover motion: cancel close on enter, schedule on leave ---
	// Attached to the popover widget directly (not its child) so the
	// controller is stable across child replacements.
	popoverMotion := gtk.NewEventControllerMotion()
	pmBase := gtk.BaseEventController(popoverMotion)
	pmBase.SetPropagationPhase(gtk.PhaseCapture)
	pmBase.SetPropagationLimit(gtk.LimitNone)
	popoverMotion.ConnectEnter(func(_, _ float64) {
		if !p.destroyed {
			p.closeGen++
		}
	})
	popoverMotion.ConnectLeave(func() {
		if !p.destroyed {
			p.scheduleClose(90 * time.Millisecond)
		}
	})
	popover.AddController(popoverMotion)
	p.controllers = append(p.controllers, controllerBinding{popover, popoverMotion})

	// --- Click inside popover: cancel close during press ---
	popupClick := gtk.NewGestureClick()
	popupClick.SetButton(0)
	pcBase := gtk.BaseEventController(popupClick)
	pcBase.SetPropagationPhase(gtk.PhaseCapture)
	pcBase.SetPropagationLimit(gtk.LimitNone)
	popupClick.ConnectPressed(func(_ int, _, _ float64) {
		if !p.destroyed {
			p.closeGen++
		}
	})
	popupClick.ConnectReleased(func(_ int, _, _ float64) {
		if !p.destroyed {
			p.scheduleClose(90 * time.Millisecond)
		}
	})
	popupClick.ConnectUnpairedRelease(func(_, _ float64, _ uint, _ *gdk.EventSequence) {
		if !p.destroyed {
			p.scheduleClose(90 * time.Millisecond)
		}
	})
	popover.AddController(popupClick)
	p.controllers = append(p.controllers, controllerBinding{popover, popupClick})

	// --- Popover closed signal ---
	popover.ConnectClosed(func() {
		p.visible = false
		if p.afterClose != nil {
			p.afterClose()
		}
	})

	// --- Right-click on anchor ---
	click := gtk.NewGestureClick()
	click.SetButton(0)
	click.ConnectPressed(func(_ int, _, _ float64) {
		if !p.destroyed && click.CurrentButton() == 3 && p.rightClick != nil {
			p.rightClick()
		}
	})
	anchor.(interface{ AddController(gtk.EventControllerer) }).AddController(click)
	p.controllers = append(p.controllers, controllerBinding{anchor, click})

	return p
}

func (p *Popup) Open() {
	if p.destroyed {
		return
	}
	p.closeGen++
	if p.visible {
		return
	}

	closeAllPopups()

	if root := p.anchor.Root(); root != nil {
		if !p.reparented {
			if p.popover.Parent() != nil {
				p.popover.Unparent()
			}
			p.popover.SetParent(root)
			p.reparented = true
		}

		x, y, ok := p.anchor.TranslateCoordinates(root, 0, 0)
		if ok {
			rect := gdk.NewRectangle(int(x), int(y), p.anchor.AllocatedWidth(), p.anchor.AllocatedHeight())
			p.popover.SetPointingTo(&rect)
		}
	}

	if p.beforeOpen != nil {
		p.beforeOpen()
	}

	p.popover.Popup()
	p.visible = true
}

func (p *Popup) Close() {
	if p.destroyed {
		return
	}
	p.closeGen++
	if p.visible {
		p.keyHeld = false
		p.popover.Popdown()
		p.visible = false
	}
}

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

func (p *Popup) SetBeforeOpen(fn func()) { p.beforeOpen = fn }
func (p *Popup) SetAfterClose(fn func()) { p.afterClose = fn }

func (p *Popup) Destroy() {
	if p.destroyed {
		return
	}
	p.destroyed = true
	p.closeGen++

	if p.visible {
		p.popover.Popdown()
		p.visible = false
	}

	delete(popupRegistry, p.id)

	for _, cb := range p.controllers {
		cb.widget.(interface{ RemoveController(gtk.EventControllerer) }).RemoveController(cb.controller)
	}
	p.controllers = nil

	if p.popover.Parent() != nil {
		p.popover.Unparent()
	}

	p.popover = nil
	p.anchor = nil
	p.beforeOpen = nil
	p.afterClose = nil
	p.rightClick = nil
}

func (p *Popup) scheduleClose(delay time.Duration) {
	if p.destroyed {
		return
	}
	p.closeGen++
	gen := p.closeGen
	if delay <= 0 {
		p.tryClose(gen)
	} else {
		time.AfterFunc(delay, func() { ui(func() { p.tryClose(gen) }) })
	}
}

// tryClose performs a single close check. No polling or rescheduling —
// future leave events will schedule new checks if needed.
func (p *Popup) tryClose(gen int) {
	if p.destroyed || gen != p.closeGen || !p.visible || p.keyHeld {
		return
	}
	if widgetContainsPointer(p.anchor) || widgetContainsPointer(p.popover) {
		return
	}
	p.popover.Popdown()
	p.visible = false
}

// originPoint is reused across widgetContainsPointer calls to avoid
// repeated graphene_point_alloc C heap allocations.
var originPoint = graphene.NewPointAlloc().Init(0, 0)

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

	origin, ok := base.ComputePoint(native, originPoint)
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
		gtk.BaseWidget(child).Unparent()
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
