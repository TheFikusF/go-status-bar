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

func NewWorkspaces() gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationHorizontal, 0)
	box.SetName("workspaces")

	refresh := func() {
		output, err := runCommand("hyprctl", "-j", "clients")
		if err != nil {
			ui(func() { box.SetVisible(false) })
			return
		}

		var clients []hyprClient
		if err := json.Unmarshal(output, &clients); err != nil {
			return
		}

		activeOut, err := runCommand("hyprctl", "-j", "activeworkspace")
		if err != nil {
			return
		}

		var active hyprWorkspace
		if err := json.Unmarshal(activeOut, &active); err != nil {
			return
		}

		workspaces := map[int][]hyprClient{}
		for _, client := range clients {
			if client.Workspace.ID <= 0 {
				continue
			}
			workspaces[client.Workspace.ID] = append(workspaces[client.Workspace.ID], client)
		}
		if active.ID > 0 {
			if _, ok := workspaces[active.ID]; !ok {
				workspaces[active.ID] = nil
			}
		}

		ids := make([]int, 0, len(workspaces))
		for id := range workspaces {
			ids = append(ids, id)
		}
		sort.Ints(ids)

		ui(func() {
			removeChildren(box)
			box.SetVisible(len(ids) > 0)
			for _, id := range ids {
				button := gtk.NewButton()
				button.SetHasFrame(false)
				button.SetChild(workspaceButtonContent(id, workspaces[id]))
				if id == active.ID {
					button.AddCSSClass("active")
				}

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
