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

	"statusbar/internal/config"
)

func NewFocusedApp(cfg *config.Config, win *gtk.ApplicationWindow, monitorName string) gtk.Widgetter {
	showEmpty := cfg.FocusedApp.ShowEmptyWorkspace
	emptyText := cfg.FocusedApp.EmptyText

	box := gtk.NewBox(gtk.OrientationHorizontal, 0)
	box.SetName("focused-app")
	box.SetVisible(false)

	// Workspace badge shell (independent box for styling).
	badgeShell := gtk.NewBox(gtk.OrientationHorizontal, 0)
	badgeShell.SetName("focused-app-badge")
	wsNumLabel := gtk.NewLabel("")
	wsNumLabel.AddCSSClass("focused-app-ws-badge")
	badgeShell.Append(wsNumLabel)

	// Text shell: icon + title (can be styled thinner).
	textShell := gtk.NewBox(gtk.OrientationHorizontal, 6)
	textShell.SetName("focused-app-text")

	appIcon := gtk.NewImage()
	appIcon.SetPixelSize(14)

	titleLabel := gtk.NewLabel("")
	titleLabel.SetSingleLineMode(true)
	titleLabel.SetEllipsize(pango.EllipsizeEnd)
	titleLabel.SetMaxWidthChars(32)
	titleLabel.SetXAlign(0)

	textShell.Append(appIcon)
	textShell.Append(titleLabel)

	box.Append(badgeShell)
	box.Append(textShell)

	popover := gtk.NewPopover()
	popover.AddCSSClass("status-popup")
	popover.SetHasArrow(false)
	popover.SetAutohide(true)
	popover.SetParent(box)

	// Horizontal box: one vertical column per monitor.
	popoverBox := gtk.NewBox(gtk.OrientationHorizontal, 0)
	popoverBox.SetName("focused-app-menu")
	popover.SetChild(popoverBox)

	popupOpen := false
	popover.ConnectClosed(func() { popupOpen = false })

	type monitorColumn struct {
		name          string
		isThisMonitor bool
		wsIDs         []int
		wsClients     map[int][]hyprClient
		activeWsID    int
	}

	// buildPopup runs on the GTK main thread with pre-fetched column data.
	buildPopup := func(columns []monitorColumn) {
		removeChildren(popoverBox)

		for i, col := range columns {
			col := col
			if i > 0 {
				sep := gtk.NewSeparator(gtk.OrientationVertical)
				sep.AddCSSClass("focused-app-monitor-sep")
				popoverBox.Append(sep)
			}

			column := gtk.NewBox(gtk.OrientationVertical, 4)
			column.AddCSSClass("focused-app-monitor-column")
			if col.isThisMonitor {
				column.AddCSSClass("this-monitor")
			}

			monLabel := gtk.NewLabel(col.name)
			monLabel.AddCSSClass("focused-app-monitor-header")
			monLabel.SetXAlign(0)
			column.Append(monLabel)

			for _, wsID := range col.wsIDs {
				wsID := wsID
				prefix := "  "
				if wsID == col.activeWsID {
					prefix = "• "
				}
				wsLabel := gtk.NewLabel(fmt.Sprintf("%sWorkspace %d", prefix, wsID))
				wsLabel.AddCSSClass("focused-app-ws-header")
				if wsID == col.activeWsID {
					wsLabel.AddCSSClass("active")
				}
				wsLabel.SetXAlign(0)
				column.Append(wsLabel)

				for _, client := range col.wsClients[wsID] {
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
					column.Append(row)
				}
			}

			if len(col.wsIDs) == 0 {
				empty := gtk.NewLabel("(empty)")
				empty.SetXAlign(0)
				column.Append(empty)
			}

			popoverBox.Append(column)
		}

		if len(columns) == 0 {
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
			wsOut, _ := runCommand("hyprctl", "-j", "workspaces")
			monsOut, _ := runCommand("hyprctl", "-j", "monitors")

			var active hyprClient
			json.Unmarshal(activeOut, &active)

			var clients []hyprClient
			json.Unmarshal(clientsOut, &clients)

			var allWorkspaces []hyprWorkspaceInfo
			json.Unmarshal(wsOut, &allWorkspaces)

			var allMonitors []hyprMonitor
			json.Unmarshal(monsOut, &allMonitors)

			// Map workspace ID → clients.
			wsClients := map[int][]hyprClient{}
			for _, c := range clients {
				if c.Workspace.ID <= 0 {
					continue
				}
				wsClients[c.Workspace.ID] = append(wsClients[c.Workspace.ID], c)
			}

			// Map monitor name → sorted workspace IDs.
			monitorWS := map[string][]int{}
			for _, ws := range allWorkspaces {
				monitorWS[ws.Monitor] = append(monitorWS[ws.Monitor], ws.ID)
			}

			// Map monitor name → active workspace ID.
			monActiveWS := map[string]int{}
			for _, m := range allMonitors {
				monActiveWS[m.Name] = m.ActiveWorkspace.ID
				// Include the active (possibly empty) workspace in the list.
				aid := m.ActiveWorkspace.ID
				if aid > 0 {
					found := false
					for _, id := range monitorWS[m.Name] {
						if id == aid {
							found = true
							break
						}
					}
					if !found {
						monitorWS[m.Name] = append(monitorWS[m.Name], aid)
					}
				}
			}

			for mn := range monitorWS {
				sort.Ints(monitorWS[mn])
			}

			// Collect monitor names sorted: this monitor first, rest alphabetically.
			monNames := make([]string, 0, len(allMonitors))
			for _, m := range allMonitors {
				monNames = append(monNames, m.Name)
			}
			sort.Slice(monNames, func(i, j int) bool {
				if monNames[i] == monitorName {
					return true
				}
				if monNames[j] == monitorName {
					return false
				}
				return monNames[i] < monNames[j]
			})

			columns := make([]monitorColumn, 0, len(monNames))
			for _, mn := range monNames {
				columns = append(columns, monitorColumn{
					name:          mn,
					isThisMonitor: mn == monitorName,
					wsIDs:         monitorWS[mn],
					wsClients:     wsClients,
					activeWsID:    monActiveWS[mn],
				})
			}

			ui(func() {
				if active.Class == "" {
					if showEmpty {
						box.SetVisible(true)
						activeWsID := monActiveWS[monitorName]
						if activeWsID > 0 {
							wsNumLabel.SetLabel(fmt.Sprintf("%d", activeWsID))
						} else {
							wsNumLabel.SetLabel("—")
						}
						if emptyText != "" {
							titleLabel.SetLabel(emptyText)
							textShell.SetVisible(true)
						} else {
							textShell.SetVisible(false)
						}
						appIcon.SetVisible(false)
					} else {
						box.SetVisible(false)
					}
				} else {
					box.SetVisible(true)
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
					appIcon.SetVisible(true)
					textShell.SetVisible(true)
				}

				if popupOpen {
					buildPopup(columns)
				}
			})
		}()
	}

	popup := attachHoverPopover(box, popover, nil, func() {
		popupOpen = true
		refresh()
	})

	refresh()
	watchSuperKey(popup.SetHeld)

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
