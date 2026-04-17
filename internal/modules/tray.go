package modules

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"statusbar/internal/services"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/godbus/dbus/v5"
)

// Start embedded watcher if none is present

type trayItem struct {
	ID            string
	Bus           string
	Path          dbus.ObjectPath
	MenuPath      dbus.ObjectPath
	IconName      string
	IconSpec      string
	IconPixmap    []byte
	IconPixmapW   int32
	IconPixmapH   int32
	IconThemePath string
	Title         string
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

type trayReadState int

const (
	trayReadHidden trayReadState = iota
	trayReadReady
	trayReadRetry
)

func NewTray() gtk.Widgetter {
	container := gtk.NewBox(gtk.OrientationHorizontal, 2)
	container.SetName("tray")
	container.AddCSSClass("module")
	container.SetVisible(false)
	go runTrayHost(container)
	return container
}

func runTrayHost(container *gtk.Box) {
	// Derive a stable host name from our PID so it matches the name
	// pre-registered inside MaybeStartEmbeddedWatcher.
	hostName := fmt.Sprintf("org.kde.StatusNotifierHost-%d", os.Getpid())
	go services.MaybeStartEmbeddedWatcher(hostName)
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
	log.Printf("tray: pinging StatusNotifierWatcher on bus %s, path %s", watcherBus, watcherPath)
	if err := watcher.Call("org.freedesktop.DBus.Peer.Ping", 0).Err; err != nil {
		log.Printf("tray: StatusNotifierWatcher not found or not responding: %v", err)
		return
	}

	hostName := fmt.Sprintf("org.kde.StatusNotifierHost-%d", os.Getpid())
	log.Printf("tray: requesting name %s", hostName)
	if _, err := conn.RequestName(hostName, dbus.NameFlagDoNotQueue); err != nil {
		log.Printf("tray: host request name failed: %v", err)
	}
	log.Printf("tray: registering as StatusNotifierHost")
	if err := watcher.Call("org.kde.StatusNotifierWatcher.RegisterStatusNotifierHost", 0, hostName).Err; err != nil {
		log.Printf("tray: host register failed: %v", err)
	}

	if err := conn.AddMatchSignal(dbus.WithMatchInterface("org.kde.StatusNotifierWatcher")); err != nil {
		log.Printf("tray: watcher match failed: %v", err)
	}
	if err := conn.AddMatchSignal(dbus.WithMatchInterface("org.kde.StatusNotifierItem")); err != nil {
		log.Printf("tray: item match failed: %v", err)
	}
	if err := conn.AddMatchSignal(dbus.WithMatchInterface("org.ayatana.NotificationItem")); err != nil {
		log.Printf("tray: ayatana item match failed: %v", err)
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
	); err != nil {
		log.Printf("tray: owner match failed: %v", err)
	}

	signals := make(chan *dbus.Signal, 256)
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

	scheduleRefresh := func(delay time.Duration) {
		if !debounceTimer.Stop() {
			select {
			case <-debounceTimer.C:
			default:
			}
		}
		debounceTimer.Reset(delay)
	}

	var lastItemsKey string
	var trayPopups []*Popup
	retryCount := 0
	refresh := func() {
		items, needsRetry := readTrayItems(conn, watcher)
		key := trayItemsKey(items)
		if key != lastItemsKey {
			lastItemsKey = key
			ui(func() { trayPopups = renderTrayItems(container, conn, items, trayPopups) })
		}

		if needsRetry {
			retryCount++
			if retryCount <= 5 {
				delay := time.Duration(retryCount) * 250 * time.Millisecond

				scheduleRefresh(delay)
				return
			}
		}
		retryCount = 0
	}

	refresh()

	// refreshPending tracks whether we need to refresh again after the
	// current refresh finishes. This lets us drain signals continuously
	// while a (potentially slow) refresh is running, without spawning
	// unbounded goroutines.
	refreshPending := false
	refreshRunning := false
	refreshDone := make(chan struct{}, 1)

	for {
		select {
		case signal := <-signals:
			if signal == nil {
				return
			}
			if !traySignalNeedsRefresh(signal, watcherBus) {
				continue
			}
			retryCount = 0
			if refreshRunning {
				refreshPending = true
			} else {
				scheduleRefresh(120 * time.Millisecond)
			}
		case <-debounceTimer.C:
			refreshRunning = true
			go func() {
				refresh()
				refreshDone <- struct{}{}
			}()
		case <-refreshDone:
			refreshRunning = false
			if refreshPending {
				refreshPending = false
				scheduleRefresh(120 * time.Millisecond)
			}
		}
	}
}

func traySignalNeedsRefresh(signal *dbus.Signal, watcherBus string) bool {
	switch signal.Name {
	case "org.kde.StatusNotifierWatcher.StatusNotifierItemRegistered",
		"org.kde.StatusNotifierWatcher.StatusNotifierItemUnregistered":
		return true
	// Per-item property signals emitted by the item itself
	case "org.kde.StatusNotifierItem.NewIcon",
		"org.kde.StatusNotifierItem.NewAttentionIcon",
		"org.kde.StatusNotifierItem.NewOverlayIcon",
		"org.kde.StatusNotifierItem.NewIconThemePath",
		"org.kde.StatusNotifierItem.NewStatus",
		"org.kde.StatusNotifierItem.NewTitle",
		"org.kde.StatusNotifierItem.NewToolTip",
		"org.ayatana.NotificationItem.NewIcon",
		"org.ayatana.NotificationItem.NewAttentionIcon",
		"org.ayatana.NotificationItem.NewOverlayIcon",
		"org.ayatana.NotificationItem.NewIconThemePath",
		"org.ayatana.NotificationItem.NewStatus",
		"org.ayatana.NotificationItem.NewTitle",
		"org.ayatana.NotificationItem.NewToolTip":
		return true
	case "org.freedesktop.DBus.NameOwnerChanged":
		if len(signal.Body) == 0 {
			return false
		}
		name, _ := signal.Body[0].(string)
		return name == watcherBus ||
			strings.HasPrefix(name, "org.kde.StatusNotifierItem") ||
			strings.HasPrefix(name, "org.ayatana.NotificationItem") ||
			strings.HasPrefix(name, ":")
	case "org.freedesktop.DBus.Properties.PropertiesChanged":
		if len(signal.Body) == 0 {
			return false
		}
		iface, _ := signal.Body[0].(string)
		return iface == "org.kde.StatusNotifierWatcher" ||
			iface == "org.kde.StatusNotifierItem" ||
			iface == "org.ayatana.NotificationItem"
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
		fmt.Fprintf(&builder, "%d", len(item.IconPixmap))
		builder.WriteByte('|')
		builder.WriteString(item.Title)
		builder.WriteByte('|')
		builder.WriteString(string(item.MenuPath))
		builder.WriteByte(';')
	}
	return builder.String()
}

func readTrayItems(conn *dbus.Conn, watcher dbus.BusObject) ([]trayItem, bool) {
	call := watcher.Call("org.freedesktop.DBus.Properties.Get", 0, "org.kde.StatusNotifierWatcher", "RegisteredStatusNotifierItems")
	if call.Err != nil {
		log.Printf("[tray] SNI watcher call failed: %v", call.Err)
		return nil, false
	}

	variant := dbus.Variant{}
	if err := call.Store(&variant); err != nil {
		log.Printf("[tray] SNI watcher variant store failed: %v", err)
		return nil, false
	}

	registered, ok := variant.Value().([]string)
	if !ok {
		log.Printf("[tray] SNI watcher returned non-string slice: %v", variant.Value())
		return nil, false
	}

	items := make([]trayItem, 0, len(registered))
	needsRetry := false
	for _, id := range registered {
		bus, path, ok := parseTrayItemID(id)
		if !ok {
			continue
		}

		item, state := readTrayItem(conn, id, bus, path)
		switch state {
		case trayReadReady:
			items = append(items, item)
		case trayReadRetry:
			needsRetry = true
		}
	}

	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	log.Printf("[tray] Using SNI tray items: %v", items)
	return items, needsRetry
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

func readTrayItem(conn *dbus.Conn, id string, bus string, path dbus.ObjectPath) (trayItem, trayReadState) {
	obj := conn.Object(bus, path)
	props, ok := readTrayItemProperties(obj)
	if !ok {
		return trayItem{}, trayReadRetry
	}

	status := variantString(props, "Status")
	if strings.EqualFold(status, "Passive") {
		return trayItem{}, trayReadHidden
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
	iconThemePath := variantString(props, "IconThemePath")

	// Electron apps report id as "chrome_status_icon_1"; use tooltip text
	// as the icon lookup key instead (matches Waybar behaviour).
	if appID == "chrome_status_icon_1" {
		if tt := readTooltipTitle(props); tt != "" {
			appID = strings.ToLower(tt)
		}
	}

	iconName, iconSpec := resolveTrayIcon(iconName, appID, bus, title, iconThemePath)

	// If no named icon was resolved, fall back to the raw pixmap provided by
	// the application (used by most Electron-based apps).
	var pixmapData []byte
	var pixmapW, pixmapH int32
	if iconName == "image-missing" && iconSpec == "" {
		pixmapData, pixmapW, pixmapH = extractIconPixmap(props)
		if pixmapData != nil {
			iconName = ""
		}
	}

	return trayItem{
		ID:            id,
		Bus:           bus,
		Path:          path,
		MenuPath:      menuPath,
		IconName:      iconName,
		IconSpec:      iconSpec,
		IconPixmap:    pixmapData,
		IconPixmapW:   pixmapW,
		IconPixmapH:   pixmapH,
		IconThemePath: iconThemePath,
		Title:         title,
	}, trayReadReady
}

func readTrayItemProperties(obj dbus.BusObject) (map[string]dbus.Variant, bool) {
	for _, iface := range []string{"org.kde.StatusNotifierItem", "org.ayatana.NotificationItem"} {
		call := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, iface)
		if call.Err != nil {
			continue
		}

		props := map[string]dbus.Variant{}
		if err := call.Store(&props); err != nil {
			continue
		}
		return props, true
	}
	return nil, false
}

func renderTrayItems(container *gtk.Box, conn *dbus.Conn, items []trayItem, oldPopups []*Popup) []*Popup {
	for _, p := range oldPopups {
		p.Destroy()
	}
	removeChildren(container)
	container.SetVisible(len(items) > 0)
	popups := make([]*Popup, 0, len(items))
	for _, item := range items {
		btn, p := newTrayItemWidget(conn, item)
		container.Append(btn)
		if p != nil {
			popups = append(popups, p)
		}
	}
	return popups
}

func newTrayItemWidget(conn *dbus.Conn, item trayItem) (gtk.Widgetter, *Popup) {
	button := gtk.NewButton()
	button.SetHasFrame(false)
	button.AddCSSClass("tray-item")

	icon := gtk.NewImage()
	switch {
	case item.IconPixmap != nil:
		if texture := trayPixmapTexture(item.IconPixmap, item.IconPixmapW, item.IconPixmapH); texture != nil {
			icon.SetFromPaintable(texture)
		} else {
			icon.SetFromIconName("image-missing")
		}
	case item.IconSpec != "":
		if gicon, err := gio.NewIconForString(item.IconSpec); err == nil {
			icon.SetFromGIcon(gicon)
		} else {
			icon.SetFromIconName(item.IconName)
		}
	default:
		icon.SetFromIconName(item.IconName)
	}
	icon.SetPixelSize(16)
	button.SetChild(icon)
	button.SetTooltipText(item.Title)

	menu := gtk.NewPopover()
	menu.AddCSSClass("status-popup")
	menu.SetHasArrow(false)
	menu.SetAutohide(true)
	menu.SetParent(button)

	menuBox := gtk.NewBox(gtk.OrientationVertical, 2)
	menuBox.SetName("tray-item-menu")
	menu.SetChild(menuBox)

	openMenu := func() {
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

	p := attachHoverPopover(button, menu, nil, openMenu)
	p.SetAfterClose(func() {
		removeChildren(menuBox)
	})

	return button, p
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
	for _, iface := range []string{"org.kde.StatusNotifierItem", "org.ayatana.NotificationItem"} {
		call := obj.Call("org.freedesktop.DBus.Properties.Get", 0, iface, "Menu")
		if call.Err != nil {
			continue
		}

		var variant dbus.Variant
		if err := call.Store(&variant); err != nil {
			continue
		}

		path, ok := variant.Value().(dbus.ObjectPath)
		if ok && path.IsValid() {
			return path
		}
	}
	return ""
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

func resolveTrayIcon(iconName string, appID string, bus string, title string, iconThemePath string) (string, string) {
	iconName = strings.TrimSpace(iconName)

	// If a custom icon theme path is provided, check there first before the
	// system theme. Electron-based apps commonly ship their own icon assets.
	if iconThemePath != "" && iconName != "" {
		for _, ext := range []string{".png", ".svg", ".xpm"} {
			candidate := filepath.Join(iconThemePath, iconName+ext)
			if _, err := os.Stat(candidate); err == nil {
				return "", candidate
			}
		}
		// Also try nested paths (e.g. hicolor/NxN/apps/)
		for _, size := range []string{"22x22", "24x24", "32x32", "48x48", "16x16"} {
			for _, ext := range []string{".png", ".svg"} {
				candidate := filepath.Join(iconThemePath, "hicolor", size, "apps", iconName+ext)
				if _, err := os.Stat(candidate); err == nil {
					return "", candidate
				}
			}
		}
	}

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

// extractIconPixmap decodes the "IconPixmap" D-Bus property (type a(iiay),
// ARGB32) and returns the largest frame converted to RGBA, or nil on failure.
func extractIconPixmap(props map[string]dbus.Variant) ([]byte, int32, int32) {
	v, ok := props["IconPixmap"]
	if !ok {
		return nil, 0, 0
	}
	return decodeIconPixmapVariant(v.Value())
}

func decodeIconPixmapVariant(raw interface{}) ([]byte, int32, int32) {
	frames, ok := raw.([]interface{})
	if !ok {
		return nil, 0, 0
	}

	var bestData []byte
	var bestW, bestH int32

	for _, frameRaw := range frames {
		frame, ok := frameRaw.([]interface{})
		if !ok || len(frame) < 3 {
			continue
		}
		w, ok1 := variantInt32(frame[0])
		h, ok2 := variantInt32(frame[1])
		data, ok3 := frame[2].([]byte)
		if !ok1 || !ok2 || !ok3 {
			continue
		}
		if w <= 0 || h <= 0 || int(w)*int(h)*4 != len(data) {
			continue
		}
		// Pick the largest image.
		if w*h > bestW*bestH {
			bestW, bestH = w, h
			bestData = argbToRGBA(data)
		}
	}
	return bestData, bestW, bestH
}

// argbToRGBA converts a byte slice from ARGB order (as sent by SNI) to RGBA.
func argbToRGBA(src []byte) []byte {
	dst := make([]byte, len(src))
	for i := 0; i+3 < len(src); i += 4 {
		a := src[i]
		dst[i] = src[i+1]   // R
		dst[i+1] = src[i+2] // G
		dst[i+2] = src[i+3] // B
		dst[i+3] = a        // A
	}
	return dst
}

// trayPixmapTexture creates a GDK MemoryTexture from RGBA pixel data.
func trayPixmapTexture(data []byte, width, height int32) *gdk.MemoryTexture {
	if len(data) != int(width)*int(height)*4 {
		return nil
	}
	bytes := glib.NewBytes(data)
	return gdk.NewMemoryTexture(int(width), int(height), gdk.MemoryR8G8B8A8, bytes, uint(width*4))
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
