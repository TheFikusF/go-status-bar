package modules

import (
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/godbus/dbus/v5"
)

type trayItem struct {
	ID       string
	Bus      string
	Path     dbus.ObjectPath
	MenuPath dbus.ObjectPath
	IconName string
	IconSpec string
	Title    string
}

type dbusMenuNode struct {
	ID              int32
	Label           string
	Type            string
	ChildrenDisplay string
	Enabled         bool
	Visible         bool
	Children        []dbusMenuNode
}

func NewTray() gtk.Widgetter {
	container := gtk.NewBox(gtk.OrientationHorizontal, 2)
	container.SetName("tray")
	container.AddCSSClass("module")
	container.SetVisible(false)

	go runTrayHost(container)

	return container
}

func runTrayHost(container *gtk.Box) {
	for {
		conn, err := dbus.ConnectSessionBus()
		if err != nil {
			ui(func() { container.SetVisible(false) })
			time.Sleep(3 * time.Second)
			continue
		}

		runTraySession(conn, container)
		_ = conn.Close()
		ui(func() { container.SetVisible(false) })
		time.Sleep(time.Second)
	}
}

func runTraySession(conn *dbus.Conn, container *gtk.Box) {
	const (
		watcherBus  = "org.kde.StatusNotifierWatcher"
		watcherPath = dbus.ObjectPath("/StatusNotifierWatcher")
	)

	watcher := conn.Object(watcherBus, watcherPath)
	if err := watcher.Call("org.freedesktop.DBus.Peer.Ping", 0).Err; err != nil {
		return
	}

	hostName := fmt.Sprintf("org.kde.StatusNotifierHost-%d", time.Now().UnixNano())
	if _, err := conn.RequestName(hostName, dbus.NameFlagDoNotQueue); err != nil {
		log.Printf("tray: host request name failed: %v", err)
	}
	if err := watcher.Call("org.kde.StatusNotifierWatcher.RegisterStatusNotifierHost", 0, hostName).Err; err != nil {
		log.Printf("tray: host register failed: %v", err)
	}

	var lastItemsKey string
	refresh := func() {
		items := readTrayItems(conn, watcher)
		key := trayItemsKey(items)
		if key == lastItemsKey {
			return
		}
		lastItemsKey = key
		ui(func() { renderTrayItems(container, conn, items) })
	}
	refresh()

	if err := conn.AddMatchSignal(dbus.WithMatchInterface("org.kde.StatusNotifierWatcher")); err != nil {
		log.Printf("tray: watcher match failed: %v", err)
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"),
	); err != nil {
		log.Printf("tray: properties match failed: %v", err)
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
	); err != nil {
		log.Printf("tray: owner match failed: %v", err)
	}

	signals := make(chan *dbus.Signal, 64)
	conn.Signal(signals)
	defer conn.RemoveSignal(signals)
	debounceTimer := time.NewTimer(time.Hour)
	if !debounceTimer.Stop() {
		select {
		case <-debounceTimer.C:
		default:
		}
	}
	defer debounceTimer.Stop()

	for {
		select {
		case signal := <-signals:
			if signal == nil {
				return
			}
			if !traySignalNeedsRefresh(signal, watcherBus) {
				continue
			}
			if !debounceTimer.Stop() {
				select {
				case <-debounceTimer.C:
				default:
				}
			}
			debounceTimer.Reset(120 * time.Millisecond)
		case <-debounceTimer.C:
			refresh()
		}
	}
}

func traySignalNeedsRefresh(signal *dbus.Signal, watcherBus string) bool {
	switch signal.Name {
	case "org.kde.StatusNotifierWatcher.StatusNotifierItemRegistered",
		"org.kde.StatusNotifierWatcher.StatusNotifierItemUnregistered":
		return true
	case "org.freedesktop.DBus.NameOwnerChanged":
		if len(signal.Body) == 0 {
			return false
		}
		name, _ := signal.Body[0].(string)
		return name == watcherBus || strings.HasPrefix(name, "org.kde.StatusNotifierItem")
	case "org.freedesktop.DBus.Properties.PropertiesChanged":
		if len(signal.Body) == 0 {
			return false
		}
		iface, _ := signal.Body[0].(string)
		return iface == "org.kde.StatusNotifierWatcher" || iface == "org.kde.StatusNotifierItem"
	default:
		return false
	}
}

func trayItemsKey(items []trayItem) string {
	if len(items) == 0 {
		return ""
	}

	var builder strings.Builder
	for _, item := range items {
		builder.WriteString(item.ID)
		builder.WriteByte('|')
		builder.WriteString(item.IconName)
		builder.WriteByte('|')
		builder.WriteString(item.IconSpec)
		builder.WriteByte('|')
		builder.WriteString(item.Title)
		builder.WriteByte('|')
		builder.WriteString(string(item.MenuPath))
		builder.WriteByte(';')
	}
	return builder.String()
}

func readTrayItems(conn *dbus.Conn, watcher dbus.BusObject) []trayItem {
	call := watcher.Call("org.freedesktop.DBus.Properties.Get", 0, "org.kde.StatusNotifierWatcher", "RegisteredStatusNotifierItems")
	if call.Err != nil {
		return nil
	}

	variant := dbus.Variant{}
	if err := call.Store(&variant); err != nil {
		return nil
	}

	registered, ok := variant.Value().([]string)
	if !ok {
		return nil
	}

	items := make([]trayItem, 0, len(registered))
	for _, id := range registered {
		bus, path, ok := parseTrayItemID(id)
		if !ok {
			continue
		}

		item, ok := readTrayItem(conn, id, bus, path)
		if !ok {
			continue
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func parseTrayItemID(id string) (bus string, path dbus.ObjectPath, ok bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", "", false
	}

	if strings.HasPrefix(id, "/") {
		return "", dbus.ObjectPath(id), false
	}

	index := strings.IndexRune(id, '/')
	if index == -1 {
		return id, dbus.ObjectPath("/StatusNotifierItem"), true
	}

	bus = id[:index]
	path = dbus.ObjectPath(id[index:])
	if bus == "" || !path.IsValid() {
		return "", "", false
	}
	return bus, path, true
}

func readTrayItem(conn *dbus.Conn, id string, bus string, path dbus.ObjectPath) (trayItem, bool) {
	obj := conn.Object(bus, path)
	call := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, "org.kde.StatusNotifierItem")
	if call.Err != nil {
		return trayItem{}, false
	}

	props := map[string]dbus.Variant{}
	if err := call.Store(&props); err != nil {
		return trayItem{}, false
	}

	status := variantString(props, "Status")
	if strings.EqualFold(status, "Passive") {
		return trayItem{}, false
	}

	iconName := variantString(props, "IconName")
	if strings.EqualFold(status, "NeedsAttention") {
		if attention := variantString(props, "AttentionIconName"); attention != "" {
			iconName = attention
		}
	}

	title := variantString(props, "Title")
	if title == "" {
		title = readTooltipTitle(props)
	}
	if title == "" {
		title = bus
		if index := strings.LastIndex(title, "."); index >= 0 && index+1 < len(title) {
			title = title[index+1:]
		}
	}

	menuPath := variantObjectPath(props, "Menu")
	if !menuPath.IsValid() || menuPath == "/" {
		menuPath = readMenuPath(conn, bus, path)
	}

	appID := variantString(props, "Id")
	iconName, iconSpec := resolveTrayIcon(iconName, appID, bus, title)

	return trayItem{
		ID:       id,
		Bus:      bus,
		Path:     path,
		MenuPath: menuPath,
		IconName: iconName,
		IconSpec: iconSpec,
		Title:    title,
	}, true
}

func renderTrayItems(container *gtk.Box, conn *dbus.Conn, items []trayItem) {
	removeChildren(container)
	container.SetVisible(len(items) > 0)
	for _, item := range items {
		container.Append(newTrayItemWidget(conn, item))
	}
}

func newTrayItemWidget(conn *dbus.Conn, item trayItem) gtk.Widgetter {
	button := gtk.NewButton()
	button.SetHasFrame(false)
	button.AddCSSClass("tray-item")

	icon := gtk.NewImage()
	if item.IconSpec != "" {
		if gicon, err := gio.NewIconForString(item.IconSpec); err == nil {
			icon.SetFromGIcon(gicon)
		} else {
			icon.SetFromIconName(item.IconName)
		}
	} else {
		icon.SetFromIconName(item.IconName)
	}
	icon.SetPixelSize(16)
	button.SetChild(icon)
	button.SetTooltipText(item.Title)

	menu := gtk.NewPopover()
	menu.SetHasArrow(false)
	menu.SetAutohide(true)
	menu.SetParent(button)

	menuBox := gtk.NewBox(gtk.OrientationVertical, 2)
	menuBox.SetName("tray-item-menu")
	menu.SetChild(menuBox)

	openMenu := func() {
		menu.Popup()
		removeChildren(menuBox)
		menuBox.Append(gtk.NewLabel("Loading..."))

		go func() {
			nodes, ok := fetchDBusMenuNodes(conn, item)
			ui(func() {
				removeChildren(menuBox)
				if !ok || len(nodes) == 0 {
					menuBox.Append(gtk.NewLabel("No menu items exposed"))
					return
				}
				appendDBusMenuNodes(menuBox, menu, conn, item, nodes, 0)
			})
		}()
	}

	attachHoverPopover(button, menu, nil, openMenu)

	return button
}

func fetchDBusMenuNodes(conn *dbus.Conn, item trayItem) ([]dbusMenuNode, bool) {
	if !item.MenuPath.IsValid() || item.MenuPath == "/" {
		return nil, false
	}

	menuObj := conn.Object(item.Bus, item.MenuPath)
	_ = menuObj.Call("com.canonical.dbusmenu.AboutToShow", 0, int32(0)).Err

	var revision uint32
	var raw any
	call := menuObj.Call(
		"com.canonical.dbusmenu.GetLayout",
		0,
		int32(0),
		int32(-1),
		[]string{"label", "enabled", "visible", "children-display", "type"},
	)
	if call.Err != nil {
		return nil, false
	}
	if err := call.Store(&revision, &raw); err != nil {
		return nil, false
	}

	root, ok := parseDBusMenuNode(raw)
	if !ok {
		return nil, false
	}
	if len(root.Children) == 0 && root.ID != 0 {
		return []dbusMenuNode{root}, true
	}
	return root.Children, true
}

func parseDBusMenuNode(raw any) (dbusMenuNode, bool) {
	switch value := raw.(type) {
	case dbus.Variant:
		return parseDBusMenuNode(value.Value())
	case []interface{}:
		return parseDBusMenuNodeSlice(value)
	case []dbus.Variant:
		children := make([]dbusMenuNode, 0, len(value))
		for _, childRaw := range value {
			child, ok := parseDBusMenuNode(childRaw)
			if ok {
				children = append(children, child)
			}
		}
		return dbusMenuNode{Children: children, Visible: true, Enabled: true}, true
	default:
		return dbusMenuNode{}, false
	}
}

func parseDBusMenuNodeSlice(value []interface{}) (dbusMenuNode, bool) {
	if len(value) < 3 {
		return dbusMenuNode{}, false
	}

	id, ok := variantInt32(value[0])
	if !ok {
		return dbusMenuNode{}, false
	}

	props, ok := coerceVariantMap(value[1])
	if !ok {
		return dbusMenuNode{}, false
	}

	childrenRaw := any(nil)
	if len(value) > 2 {
		childrenRaw = value[2]
	}

	node := dbusMenuNode{
		ID:              id,
		Label:           cleanMenuLabel(variantString(props, "label")),
		Type:            variantString(props, "type"),
		ChildrenDisplay: variantString(props, "children-display"),
		Enabled:         variantBoolDefault(props, "enabled", true),
		Visible:         variantBoolDefault(props, "visible", true),
	}

	switch children := childrenRaw.(type) {
	case []interface{}:
		for _, childRaw := range children {
			child, ok := parseDBusMenuNode(childRaw)
			if ok {
				node.Children = append(node.Children, child)
			}
		}
	case []dbus.Variant:
		for _, childRaw := range children {
			child, ok := parseDBusMenuNode(childRaw)
			if ok {
				node.Children = append(node.Children, child)
			}
		}
	}

	return node, true
}

func appendDBusMenuNodes(parent *gtk.Box, popover *gtk.Popover, conn *dbus.Conn, item trayItem, nodes []dbusMenuNode, depth int) {
	for _, node := range nodes {
		if !node.Visible {
			continue
		}

		if strings.EqualFold(node.Type, "separator") {
			parent.Append(gtk.NewSeparator(gtk.OrientationHorizontal))
			continue
		}

		label := node.Label
		if label == "" {
			label = "(item)"
		}
		if depth > 0 {
			label = strings.Repeat("  ", depth) + label
		}

		button := gtk.NewButtonWithLabel(label)
		button.SetHasFrame(false)
		button.AddCSSClass("tray-item-action")
		button.SetSensitive(node.Enabled)

		current := node
		if len(current.Children) > 0 || strings.EqualFold(current.ChildrenDisplay, "submenu") {
			button.AddCSSClass("tray-item-submenu")
			button.ConnectClicked(func() {})
			parent.Append(button)
			appendDBusMenuNodes(parent, popover, conn, item, current.Children, depth+1)
			continue
		}

		button.ConnectClicked(func() {
			popover.Popdown()
			triggerDBusMenuClick(conn, item.Bus, item.MenuPath, current.ID)
		})
		parent.Append(button)
	}
}

func triggerDBusMenuClick(conn *dbus.Conn, bus string, menuPath dbus.ObjectPath, id int32) {
	go func() {
		menuObj := conn.Object(bus, menuPath)
		timestamp := uint32(time.Now().UnixMilli())
		_ = menuObj.Call(
			"com.canonical.dbusmenu.Event",
			0,
			id,
			"clicked",
			dbus.MakeVariant(int32(0)),
			timestamp,
		).Err
	}()
}

func variantString(props map[string]dbus.Variant, key string) string {
	value, ok := props[key]
	if !ok {
		return ""
	}
	text, ok := value.Value().(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func variantObjectPath(props map[string]dbus.Variant, key string) dbus.ObjectPath {
	value, ok := props[key]
	if !ok {
		return ""
	}
	path, ok := value.Value().(dbus.ObjectPath)
	if !ok || !path.IsValid() {
		return ""
	}
	return path
}

func coerceVariantMap(value any) (map[string]dbus.Variant, bool) {
	switch props := value.(type) {
	case map[string]dbus.Variant:
		return props, true
	case dbus.Variant:
		return coerceVariantMap(props.Value())
	case map[string]any:
		coerced := make(map[string]dbus.Variant, len(props))
		for key, raw := range props {
			if variant, ok := raw.(dbus.Variant); ok {
				coerced[key] = variant
			} else {
				coerced[key] = dbus.MakeVariant(raw)
			}
		}
		return coerced, true
	default:
		return nil, false
	}
}

func readMenuPath(conn *dbus.Conn, bus string, itemPath dbus.ObjectPath) dbus.ObjectPath {
	obj := conn.Object(bus, itemPath)
	call := obj.Call("org.freedesktop.DBus.Properties.Get", 0, "org.kde.StatusNotifierItem", "Menu")
	if call.Err != nil {
		return ""
	}

	var variant dbus.Variant
	if err := call.Store(&variant); err != nil {
		return ""
	}

	path, ok := variant.Value().(dbus.ObjectPath)
	if !ok || !path.IsValid() {
		return ""
	}
	return path
}

func variantBoolDefault(props map[string]dbus.Variant, key string, fallback bool) bool {
	value, ok := props[key]
	if !ok {
		return fallback
	}
	boolean, ok := value.Value().(bool)
	if !ok {
		return fallback
	}
	return boolean
}

func variantInt32(value any) (int32, bool) {
	switch v := value.(type) {
	case int32:
		return v, true
	case int64:
		return int32(v), true
	case uint32:
		return int32(v), true
	case uint64:
		return int32(v), true
	case dbus.Variant:
		return variantInt32(v.Value())
	default:
		return 0, false
	}
}

func readTooltipTitle(props map[string]dbus.Variant) string {
	value, ok := props["ToolTip"]
	if !ok {
		return ""
	}
	fields, ok := value.Value().([]interface{})
	if !ok || len(fields) < 4 {
		return ""
	}
	title, _ := fields[2].(string)
	description, _ := fields[3].(string)
	title = strings.TrimSpace(title)
	if title != "" {
		return title
	}
	return strings.TrimSpace(description)
}

func cleanMenuLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	label = strings.ReplaceAll(label, "__", "\x00")
	label = strings.ReplaceAll(label, "_", "")
	label = strings.ReplaceAll(label, "\x00", "_")
	return label
}

func resolveTrayIcon(iconName string, appID string, bus string, title string) (string, string) {
	iconName = strings.TrimSpace(iconName)
	if trayIconExists(iconName) {
		return iconName, ""
	}

	if gicon := fallbackAppIcon(appID, bus, title); gicon != nil {
		return "", gicon.String()
	}

	if iconName != "" {
		return iconName, ""
	}
	return "image-missing", ""
}

func trayIconExists(iconName string) bool {
	iconName = strings.TrimSpace(iconName)
	if iconName == "" {
		return false
	}
	if strings.HasPrefix(iconName, "/") {
		return true
	}

	display := gdk.DisplayGetDefault()
	if display == nil {
		return false
	}
	theme := gtk.IconThemeGetForDisplay(display)
	if theme == nil {
		return false
	}
	return theme.HasIcon(iconName)
}

func fallbackAppIcon(appID string, bus string, title string) gio.Iconner {
	candidates := trayAppCandidates(appID, bus, title)
	if len(candidates) == 0 {
		return nil
	}

	bestScore := 0
	var bestIcon gio.Iconner
	for _, app := range gio.AppInfoGetAll() {
		if app == nil {
			continue
		}
		icon := app.Icon()
		if icon == nil {
			continue
		}

		score := trayAppMatchScore(app, candidates)
		if score > bestScore {
			bestScore = score
			bestIcon = icon
		}
	}

	return bestIcon
}

func trayAppCandidates(appID string, bus string, title string) []string {
	values := []string{
		appID,
		bus,
		title,
		filepath.Base(bus),
		filepath.Base(strings.ReplaceAll(bus, ".", "/")),
	}

	if lastDot := strings.LastIndex(bus, "."); lastDot >= 0 && lastDot+1 < len(bus) {
		values = append(values, bus[lastDot+1:])
	}

	seen := make(map[string]struct{}, len(values)*2)
	result := make([]string, 0, len(values)*2)
	for _, value := range values {
		value = normalizeTrayToken(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			result = append(result, value)
		}

		trimmed := strings.TrimSuffix(value, "desktop")
		if trimmed != value && trimmed != "" {
			if _, ok := seen[trimmed]; !ok {
				seen[trimmed] = struct{}{}
				result = append(result, trimmed)
			}
		}
	}
	return result
}

func trayAppMatchScore(app *gio.AppInfo, candidates []string) int {
	fields := []string{
		normalizeTrayToken(app.ID()),
		normalizeTrayToken(app.Name()),
		normalizeTrayToken(app.DisplayName()),
		normalizeTrayToken(filepath.Base(app.Executable())),
	}

	best := 0
	for _, field := range fields {
		if field == "" {
			continue
		}
		for _, candidate := range candidates {
			switch {
			case field == candidate:
				best = maxInt(best, 100)
			case strings.TrimSuffix(field, "desktop") == strings.TrimSuffix(candidate, "desktop"):
				best = maxInt(best, 85)
			case strings.Contains(field, candidate) || strings.Contains(candidate, field):
				best = maxInt(best, 45)
			}
		}
	}
	return best
}

func normalizeTrayToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		" ", "",
		"-", "",
		"_", "",
		"/", "",
		".", "",
		":", "",
	)
	return replacer.Replace(value)
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
