package modules

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

func NewFocusedApp(win *gtk.ApplicationWindow) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationHorizontal, 6)
	box.SetName("focused-app")
	box.SetVisible(false)

	wsNumLabel := gtk.NewLabel("")
	wsNumLabel.AddCSSClass("focused-app-ws-badge")

	appIcon := gtk.NewImage()
	appIcon.SetPixelSize(14)

	titleLabel := gtk.NewLabel("")
	titleLabel.SetSingleLineMode(true)
	titleLabel.SetEllipsize(pango.EllipsizeEnd)
	titleLabel.SetMaxWidthChars(32)
	titleLabel.SetXAlign(0)

	box.Append(wsNumLabel)
	box.Append(appIcon)
	box.Append(titleLabel)

	popover := gtk.NewPopover()
	popover.AddCSSClass("status-popup")
	popover.SetHasArrow(false)
	popover.SetAutohide(true)
	popover.SetParent(box)

	popoverBox := gtk.NewBox(gtk.OrientationVertical, 4)
	popoverBox.SetName("focused-app-menu")
	popover.SetChild(popoverBox)

	activeWsID := 0
	popupOpen := false

	popover.ConnectClosed(func() { popupOpen = false })

	// buildPopup runs on the GTK main thread with pre-fetched data.
	buildPopup := func(wsClients map[int][]hyprClient, ids []int) {
		removeChildren(popoverBox)
		for _, id := range ids {
			prefix := "  "
			if id == activeWsID {
				prefix = "• "
			}
			wsLabel := gtk.NewLabel(fmt.Sprintf("%sWorkspace %d", prefix, id))
			wsLabel.AddCSSClass("focused-app-ws-header")
			if id == activeWsID {
				wsLabel.AddCSSClass("active")
			}
			wsLabel.SetXAlign(0)
			popoverBox.Append(wsLabel)

			for _, client := range wsClients[id] {
				client := client
				row := gtk.NewButton()
				row.SetHasFrame(false)
				row.AddCSSClass("focused-app-window-row")

				inner := gtk.NewBox(gtk.OrientationHorizontal, 6)

				iconImg := gtk.NewImage()
				iconImg.SetPixelSize(14)
				if gicon := resolveWindowIcon(client); gicon != nil {
					iconImg.SetFromGIcon(gicon)
				} else {
					iconImg.SetFromIconName("application-x-executable")
				}
				inner.Append(iconImg)

				title := strings.TrimSpace(client.Title)
				if title == "" {
					title = client.Class
				}
				lbl := gtk.NewLabel(title)
				lbl.SetXAlign(0)
				lbl.SetEllipsize(pango.EllipsizeEnd)
				lbl.SetMaxWidthChars(36)
				inner.Append(lbl)

				row.SetChild(inner)
				row.ConnectClicked(func() {
					popover.Popdown()
					runDetached("hyprctl", "dispatch", "workspace", fmt.Sprintf("%d", client.Workspace.ID))
				})
				popoverBox.Append(row)
			}
		}
		if len(ids) == 0 {
			empty := gtk.NewLabel("No windows open")
			empty.SetXAlign(0)
			popoverBox.Append(empty)
		}
	}

	// refresh fetches all data in a background goroutine and updates
	// the bar widget and (if open) the popup on the GTK main thread.
	refresh := func() {
		go func() {
			activeOut, _ := runCommand("hyprctl", "-j", "activewindow")
			clientsOut, _ := runCommand("hyprctl", "-j", "clients")

			var active hyprClient
			json.Unmarshal(activeOut, &active)

			var clients []hyprClient
			json.Unmarshal(clientsOut, &clients)

			wsClients := map[int][]hyprClient{}
			for _, c := range clients {
				if c.Workspace.ID <= 0 {
					continue
				}
				wsClients[c.Workspace.ID] = append(wsClients[c.Workspace.ID], c)
			}
			ids := make([]int, 0, len(wsClients))
			for id := range wsClients {
				ids = append(ids, id)
			}
			sort.Ints(ids)

			ui(func() {
				if active.Class == "" {
					box.SetVisible(false)
					return
				}
				box.SetVisible(true)
				activeWsID = active.Workspace.ID
				wsNumLabel.SetLabel(fmt.Sprintf("%d", active.Workspace.ID))
				if gicon := resolveWindowIcon(active); gicon != nil {
					appIcon.SetFromGIcon(gicon)
				} else {
					appIcon.SetFromIconName("application-x-executable")
				}
				title := strings.TrimSpace(active.Title)
				if title == "" {
					title = active.Class
				}
				titleLabel.SetLabel(title)

				if popupOpen {
					buildPopup(wsClients, ids)
				}
			})
		}()
	}

	_, setHeld := attachHoverPopover(box, popover, nil, func() {
		popupOpen = true
		refresh()
	})

	refresh()
	watchSuperKey(setHeld)

	go func() {
		for event := range subscribeHyprEvents() {
			switch event.Name {
			case "activewindow", "activewindowv2", "openwindow", "closewindow", "closewindowv2",
				"workspace", "workspacev2", "focusedmon", "focusedmonv2":
				refresh()
			}
		}
	}()

	return box
}
func watchSuperKey(setHeld func(bool)) {
	// bind (not binde) fires on press AND key-repeat for Super_L.
	// bindr does NOT fire for modifier keys in Hyprland.
	// So: each SIGUSR1 keeps a 150ms timer alive; when repeats stop
	// (key released), the timer fires and closes the popup.
	pid := strconv.Itoa(os.Getpid())
	runCommand("hyprctl", "keyword", "binde", ",Super_L,exec,kill -USR1 "+pid)

	sigCh := make(chan os.Signal, 16)
	signal.Notify(sigCh, syscall.SIGUSR1)
	go func() {
		var timer *time.Timer
		var count int
		for range sigCh {
			if timer != nil {
				timer.Stop()
			}
			count++
			if count == 2 {
				// First repeat confirms key is held — open popup.
				ui(func() { setHeld(true) })
			}
			timer = time.AfterFunc(150*time.Millisecond, func() {
				count = 0
				ui(func() { setHeld(false) })
			})
		}
	}()
}
