package modules

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type hyprWorkspace struct {
	ID int `json:"id"`
}

type hyprWorkspaceInfo struct {
	ID      int    `json:"id"`
	Monitor string `json:"monitor"`
}

type hyprMonitor struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	ActiveWorkspace struct {
		ID int `json:"id"`
	} `json:"activeWorkspace"`
}

type hyprClient struct {
	Class     string `json:"class"`
	Title     string `json:"title"`
	Workspace struct {
		ID int `json:"id"`
	} `json:"workspace"`
}

var (
	appInfoOnce  sync.Once
	appIconIndex []appIconEntry
)

type appIconEntry struct {
	id          string
	name        string
	displayName string
	executable  string
	icon        *gio.Icon
}

func NewWorkspaces(monitorName string) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationHorizontal, 0)
	box.SetName("workspaces")

	var wsPopups []*Popup

	refresh := func() {
		clientsOut, err := runCommand("hyprctl", "-j", "clients")
		if err != nil {
			ui(func() { box.SetVisible(false) })
			return
		}

		var clients []hyprClient
		if err := json.Unmarshal(clientsOut, &clients); err != nil {
			return
		}

		wsOut, _ := runCommand("hyprctl", "-j", "workspaces")
		var allWorkspaces []hyprWorkspaceInfo
		json.Unmarshal(wsOut, &allWorkspaces)

		monsOut, _ := runCommand("hyprctl", "-j", "monitors")
		var monitors []hyprMonitor
		json.Unmarshal(monsOut, &monitors)

		// Find the active workspace for this specific monitor.
		activeWsID := 0
		for _, m := range monitors {
			if monitorName == "" || m.Name == monitorName {
				activeWsID = m.ActiveWorkspace.ID
				break
			}
		}

		// Build the set of workspace IDs that belong to this monitor.
		monitorWSIDs := map[int]bool{}
		for _, ws := range allWorkspaces {
			if monitorName == "" || ws.Monitor == monitorName {
				monitorWSIDs[ws.ID] = true
			}
		}
		// Always include the monitor's active workspace (may be empty).
		if activeWsID > 0 {
			monitorWSIDs[activeWsID] = true
		}

		workspaces := map[int][]hyprClient{}
		for _, client := range clients {
			if client.Workspace.ID <= 0 || !monitorWSIDs[client.Workspace.ID] {
				continue
			}
			workspaces[client.Workspace.ID] = append(workspaces[client.Workspace.ID], client)
		}
		// Ensure every known workspace ID appears (even if empty).
		for id := range monitorWSIDs {
			if _, ok := workspaces[id]; !ok {
				workspaces[id] = nil
			}
		}

		ids := make([]int, 0, len(workspaces))
		for id := range workspaces {
			ids = append(ids, id)
		}
		sort.Ints(ids)

		ui(func() {
			// Destroy old popups before removing their anchor widgets.
			for _, p := range wsPopups {
				p.Destroy()
			}
			wsPopups = wsPopups[:0]

			removeChildren(box)
			box.SetVisible(len(ids) > 0)
			for _, id := range ids {
				button := gtk.NewButton()
				button.SetHasFrame(false)
				button.SetChild(workspaceButtonContent(id, workspaces[id]))
				if id == activeWsID {
					button.AddCSSClass("active")
				}

				// Create popover for window titles
				popover := gtk.NewPopover()
				popover.AddCSSClass("status-popup")
				popover.SetHasArrow(false)
				popover.SetAutohide(true)
				popover.SetParent(button)

				// Function to update popover content
				updatePopover := func() {
					vbox := gtk.NewBox(gtk.OrientationVertical, 4)
					vbox.SetName("workspace-popup")

					clients := workspaces[id]
					if len(clients) == 0 {
						label := gtk.NewLabel("(empty)")
						vbox.Append(label)
					} else {
						for _, client := range clients {
							title := strings.TrimSpace(client.Title)
							if title == "" {
								title = "(untitled)"
							}
							label := gtk.NewLabel(title)
							vbox.Append(label)
						}
					}
					popover.SetChild(vbox)
				}

				// Update popover before open
				p := attachHoverPopover(button, popover, nil, updatePopover)
				wsPopups = append(wsPopups, p)

				workspaceID := id
				button.ConnectClicked(func() {
					runDetached("hyprctl", "dispatch", "workspace", fmt.Sprintf("%d", workspaceID))
				})

				box.Append(button)
			}
		})
	}

	refresh()

	go func() {
		for event := range subscribeHyprEvents() {
			switch event.Name {
			case "workspace", "workspacev2", "focusedmon", "focusedmonv2", "createworkspace", "createworkspacev2", "destroyworkspace", "destroyworkspacev2", "movewindow", "movewindowv2", "openwindow", "closewindow", "changefloatingmode", "activewindow", "activewindowv2", "urgent":
				refresh()
			}
		}
	}()

	return box
}

func workspaceButtonContent(id int, clients []hyprClient) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 4)

	label := gtk.NewLabel(fmt.Sprintf("%d", id))
	row.Append(label)

	for _, client := range clients {
		row.Append(windowIconWidget(client))
	}

	return row
}

func windowIconWidget(client hyprClient) gtk.Widgetter {
	if icon := resolveWindowIcon(client); icon != nil {
		image := gtk.NewImageFromGIcon(icon)
		image.SetPixelSize(14)
		return image
	}

	image := gtk.NewImageFromIconName("application-x-executable")
	image.SetPixelSize(14)
	return image
}

func resolveWindowIcon(client hyprClient) *gio.Icon {
	indexAppIcons()

	class := normalizeIconKey(client.Class)
	title := normalizeIconKey(client.Title)
	for _, entry := range appIconIndex {
		if entry.icon == nil {
			continue
		}

		if iconEntryMatches(entry, class, title) {
			return entry.icon
		}
	}

	return nil
}

func indexAppIcons() {
	appInfoOnce.Do(func() {
		for _, app := range gio.AppInfoGetAll() {
			icon := app.Icon()
			if icon == nil {
				continue
			}

			appIconIndex = append(appIconIndex, appIconEntry{
				id:          normalizeIconKey(app.ID()),
				name:        normalizeIconKey(app.Name()),
				displayName: normalizeIconKey(app.DisplayName()),
				executable:  normalizeIconKey(app.Executable()),
				icon:        icon,
			})
		}
	})
}

func iconEntryMatches(entry appIconEntry, class string, title string) bool {
	values := []string{entry.id, entry.name, entry.displayName, entry.executable}
	for _, value := range values {
		if value == "" {
			continue
		}
		if value == class || strings.Contains(value, class) || strings.Contains(class, value) {
			return true
		}
		if title != "" && (strings.Contains(value, title) || strings.Contains(title, value)) {
			return true
		}
	}
	return false
}

func normalizeIconKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(".", "", "-", "", "_", "", " ", "")
	return replacer.Replace(value)
}

func firstN(value string, n int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "APP"
	}
	runes := []rune(value)
	if len(runes) < n {
		n = len(runes)
	}
	return string(runes[:n])
}
