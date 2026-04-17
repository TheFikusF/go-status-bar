package services

import (
	"context"
	"encoding/xml"
	"log"
	"path"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

type SNIEntry struct {
	BusName    string
	ObjectPath string
}

type StatusNotifierWatcher struct {
	Conn  *dbus.Conn
	Items map[string]SNIEntry // key: busName|objectPath
	Hosts map[string]SNIEntry
}

type WatcherExport struct {
	Watcher *StatusNotifierWatcher
}

const (
	defaultStatusNotifierItemPath = "/StatusNotifierItem"
	watcherObjectPath             = dbus.ObjectPath("/StatusNotifierWatcher")
	maxIntrospectionNodes         = 128
)

func sniKey(busName, objectPath string) string {
	return busName + "|" + objectPath
}

func isStatusNotifierHostRegistered() bool {
	// Mature SNI watchers report that a host is present as soon as the watcher
	// appears, which is what Chromium/Electron expects before it exposes tray
	// icons. Relying on a later RegisterStatusNotifierHost call creates a race.
	return true
}

// parseSNIItemService resolves the unique bus name and object path from a
// RegisterStatusNotifierItem payload. Electron/Chromium can send either an
// object path (using the sender's unique name) or a well-known service name.
func parseSNIItemService(conn *dbus.Conn, sender dbus.Sender, service string) (string, string, error) {
	if strings.HasPrefix(service, "/") {
		return string(sender), service, nil
	}

	if strings.HasPrefix(service, ":") {
		return service, defaultStatusNotifierItemPath, nil
	}

	busName, err := resolveUniqueBusName(conn, service)
	if err != nil {
		return "", "", err
	}
	return busName, defaultStatusNotifierItemPath, nil
}

func resolveUniqueBusName(conn *dbus.Conn, service string) (string, error) {
	if conn == nil {
		return "", dbus.ErrClosed
	}

	var owner string
	err := conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, service).Store(&owner)
	if err != nil {
		return "", err
	}
	return owner, nil
}

// --- org.kde.StatusNotifierWatcher methods ---

func (w *WatcherExport) RegisterStatusNotifierItem(sender dbus.Sender, service string) *dbus.Error {
	busName, objectPath, err := parseSNIItemService(w.Watcher.Conn, sender, service)
	if err != nil {
		log.Printf("tray/watcher: failed to resolve item %q from %s: %v", service, sender, err)
		return nil
	}
	if !w.Watcher.registerItem(busName, objectPath) {
		return nil
	}
	w.Watcher.emitPropertiesChanged()
	return nil
}

func (w *WatcherExport) RegisterStatusNotifierHost(sender dbus.Sender, service string) *dbus.Error {
	busName := string(sender)
	objectPath := service
	if !strings.HasPrefix(objectPath, "/") {
		objectPath = "/StatusNotifierHost"
	}
	key := sniKey(busName, objectPath)
	if _, exists := w.Watcher.Hosts[key]; exists {
		return nil
	}
	w.Watcher.Hosts[key] = SNIEntry{busName, objectPath}
	w.Watcher.emitPropertiesChanged()
	w.Watcher.emitHostSignal("StatusNotifierHostRegistered")
	log.Printf("tray/watcher: registered host %s%s", busName, objectPath)
	return nil
}

// --- org.freedesktop.DBus.Properties ---

func (w *WatcherExport) Get(interfaceName, property string) (dbus.Variant, *dbus.Error) {
	if interfaceName != "org.kde.StatusNotifierWatcher" {
		return dbus.Variant{}, nil
	}
	switch property {
	case "RegisteredStatusNotifierItems":
		items := make([]string, 0, len(w.Watcher.Items))
		for _, e := range w.Watcher.Items {
			items = append(items, e.BusName+e.ObjectPath)
		}
		return dbus.MakeVariant(items), nil
	case "IsStatusNotifierHostRegistered":
		return dbus.MakeVariant(isStatusNotifierHostRegistered()), nil
	case "ProtocolVersion":
		return dbus.MakeVariant(int32(0)), nil
	}
	return dbus.Variant{}, nil
}

func (w *WatcherExport) GetAll(interfaceName string) (map[string]dbus.Variant, *dbus.Error) {
	if interfaceName != "org.kde.StatusNotifierWatcher" {
		return map[string]dbus.Variant{}, nil
	}
	items := make([]string, 0, len(w.Watcher.Items))
	for _, e := range w.Watcher.Items {
		items = append(items, e.BusName+e.ObjectPath)
	}
	return map[string]dbus.Variant{
		"RegisteredStatusNotifierItems":  dbus.MakeVariant(items),
		"IsStatusNotifierHostRegistered": dbus.MakeVariant(isStatusNotifierHostRegistered()),
		"ProtocolVersion":                dbus.MakeVariant(int32(0)),
	}, nil
}

func (w *WatcherExport) Set(_ string, _ string, _ dbus.Variant) *dbus.Error {
	return nil
}

// --- internal helpers ---

// emitSignal emits a watcher signal that carries a single string argument
// (StatusNotifierItemRegistered / StatusNotifierItemUnregistered).
func (w *StatusNotifierWatcher) emitSignal(signal, value string) {
	if w.Conn != nil {
		_ = w.Conn.Emit("/StatusNotifierWatcher", "org.kde.StatusNotifierWatcher."+signal, value)
	}
}

// emitHostSignal emits a host signal that carries NO arguments
// (StatusNotifierHostRegistered / StatusNotifierHostUnregistered per the SNI spec).
func (w *StatusNotifierWatcher) emitHostSignal(signal string) {
	if w.Conn != nil {
		_ = w.Conn.Emit("/StatusNotifierWatcher", "org.kde.StatusNotifierWatcher."+signal)
	}
}

func (w *StatusNotifierWatcher) emitPropertiesChanged() {
	if w.Conn == nil {
		return
	}
	items := make([]string, 0, len(w.Items))
	for _, e := range w.Items {
		items = append(items, e.BusName+e.ObjectPath)
	}
	_ = w.Conn.Emit(
		"/StatusNotifierWatcher",
		"org.freedesktop.DBus.Properties.PropertiesChanged",
		"org.kde.StatusNotifierWatcher",
		map[string]dbus.Variant{
			"RegisteredStatusNotifierItems":  dbus.MakeVariant(items),
			"IsStatusNotifierHostRegistered": dbus.MakeVariant(isStatusNotifierHostRegistered()),
		},
		[]string{},
	)
}

func (w *StatusNotifierWatcher) registerItem(busName, objectPath string) bool {
	if w == nil || w.Conn == nil {
		return false
	}

	pathValue := dbus.ObjectPath(objectPath)
	if busName == "" || !pathValue.IsValid() {
		return false
	}
	if !hasStatusNotifierItemInterface(w.Conn.Object(busName, pathValue)) {
		log.Printf("tray/watcher: ignoring item %s%s without SNI interface", busName, objectPath)
		return false
	}

	key := sniKey(busName, objectPath)
	if _, exists := w.Items[key]; exists {
		return false
	}

	w.Items[key] = SNIEntry{busName, objectPath}
	w.emitSignal("StatusNotifierItemRegistered", busName+objectPath)
	log.Printf("tray/watcher: registered item %s%s", busName, objectPath)
	return true
}

func (w *StatusNotifierWatcher) discoverExistingItems() {
	if w == nil || w.Conn == nil {
		return
	}

	var names []string
	if err := w.Conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		log.Printf("tray/watcher: could not list bus names for discovery: %v", err)
		return
	}

	changed := false
	for _, name := range names {
		if !strings.HasPrefix(name, ":") {
			continue
		}
		if w.discoverBusName(name) {
			changed = true
		}
	}
	if changed {
		w.emitPropertiesChanged()
	}
}

func (w *StatusNotifierWatcher) discoverBusName(busName string) bool {
	if w == nil || w.Conn == nil || !strings.HasPrefix(busName, ":") {
		return false
	}

	paths := discoverStatusNotifierItemPaths(w.Conn, busName)
	changed := false
	for _, objectPath := range paths {
		if w.registerItem(busName, string(objectPath)) {
			changed = true
		}
	}
	if changed {
		w.emitPropertiesChanged()
	}
	return changed
}

func discoverStatusNotifierItemPaths(conn *dbus.Conn, busName string) []dbus.ObjectPath {
	if conn == nil || busName == "" {
		return nil
	}

	seen := map[dbus.ObjectPath]struct{}{}
	paths := make([]dbus.ObjectPath, 0, 2)
	addPath := func(candidate dbus.ObjectPath) bool {
		if !candidate.IsValid() {
			return false
		}
		if _, exists := seen[candidate]; exists {
			return true
		}
		seen[candidate] = struct{}{}
		if hasStatusNotifierItemInterface(conn.Object(busName, candidate)) {
			paths = append(paths, candidate)
			return true
		}
		return false
	}

	for _, candidate := range []dbus.ObjectPath{
		defaultStatusNotifierItemPath,
		"/org/ayatana/NotificationItem",
	} {
		addPath(candidate)
	}
	if len(paths) > 0 {
		return paths
	}

	queue := []dbus.ObjectPath{"/"}
	visited := map[dbus.ObjectPath]struct{}{"/": {}}
	visitedCount := 0

	for len(queue) > 0 && visitedCount < maxIntrospectionNodes {
		current := queue[0]
		queue = queue[1:]
		visitedCount++

		node, err := introspectNode(conn, busName, current)
		if err != nil {
			continue
		}
		if nodeHasStatusNotifierItemInterface(node) && addPath(current) {
			continue
		}

		for _, child := range node.Children {
			childPath := childObjectPath(current, child.Name)
			if !childPath.IsValid() {
				continue
			}
			if _, exists := visited[childPath]; exists {
				continue
			}
			visited[childPath] = struct{}{}
			queue = append(queue, childPath)
		}
	}

	return paths
}

func introspectNode(conn *dbus.Conn, busName string, objectPath dbus.ObjectPath) (introspect.Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var xmlData string
	err := conn.Object(busName, objectPath).CallWithContext(ctx, "org.freedesktop.DBus.Introspectable.Introspect", 0).Store(&xmlData)
	if err != nil {
		return introspect.Node{}, err
	}

	var node introspect.Node
	if err := xml.Unmarshal([]byte(xmlData), &node); err != nil {
		return introspect.Node{}, err
	}
	return node, nil
}

func nodeHasStatusNotifierItemInterface(node introspect.Node) bool {
	for _, iface := range node.Interfaces {
		switch iface.Name {
		case "org.kde.StatusNotifierItem", "org.ayatana.NotificationItem":
			return true
		}
	}
	return false
}

func childObjectPath(parent dbus.ObjectPath, childName string) dbus.ObjectPath {
	childName = strings.TrimSpace(childName)
	if childName == "" {
		return ""
	}
	if strings.HasPrefix(childName, "/") {
		pathValue := dbus.ObjectPath(childName)
		if pathValue.IsValid() {
			return pathValue
		}
		return ""
	}

	base := string(parent)
	if base == "" {
		base = "/"
	}
	joined := path.Join(base, childName)
	if !strings.HasPrefix(joined, "/") {
		joined = "/" + joined
	}

	pathValue := dbus.ObjectPath(joined)
	if !pathValue.IsValid() {
		return ""
	}
	return pathValue
}

// removeBusName removes all items and hosts owned by uniqueName and emits the
// corresponding unregistered signals.  Called when a name disappears from the bus.
func (w *StatusNotifierWatcher) removeBusName(uniqueName string) {
	changed := false
	for key, e := range w.Items {
		if e.BusName == uniqueName {
			delete(w.Items, key)
			w.emitSignal("StatusNotifierItemUnregistered", e.BusName+e.ObjectPath)
			log.Printf("tray/watcher: removed item %s%s (owner left bus)", e.BusName, e.ObjectPath)
			changed = true
		}
	}
	for key, e := range w.Hosts {
		if e.BusName == uniqueName {
			delete(w.Hosts, key)
			w.emitHostSignal("StatusNotifierHostUnregistered")
			log.Printf("tray/watcher: removed host %s%s (owner left bus)", e.BusName, e.ObjectPath)
			changed = true
		}
	}
	if changed {
		w.emitPropertiesChanged()
	}
}

var sniWatcherNode = &introspect.Node{
	Name: "/StatusNotifierWatcher",
	Interfaces: []introspect.Interface{
		introspect.IntrospectData,
		{
			Name: "org.kde.StatusNotifierWatcher",
			Methods: []introspect.Method{
				{Name: "RegisterStatusNotifierItem", Args: []introspect.Arg{{Name: "service", Type: "s", Direction: "in"}}},
				{Name: "RegisterStatusNotifierHost", Args: []introspect.Arg{{Name: "service", Type: "s", Direction: "in"}}},
			},
			Properties: []introspect.Property{
				{Name: "RegisteredStatusNotifierItems", Type: "as", Access: "read"},
				{Name: "IsStatusNotifierHostRegistered", Type: "b", Access: "read"},
				{Name: "ProtocolVersion", Type: "i", Access: "read"},
			},
			Signals: []introspect.Signal{
				{Name: "StatusNotifierItemRegistered", Args: []introspect.Arg{{Name: "service", Type: "s"}}},
				{Name: "StatusNotifierItemUnregistered", Args: []introspect.Arg{{Name: "service", Type: "s"}}},
				{Name: "StatusNotifierHostRegistered"},
				{Name: "StatusNotifierHostUnregistered"},
			},
		},
		{
			Name: "org.freedesktop.DBus.Properties",
			Methods: []introspect.Method{
				{Name: "Get", Args: []introspect.Arg{{Type: "s", Direction: "in"}, {Type: "s", Direction: "in"}, {Type: "v", Direction: "out"}}},
				{Name: "GetAll", Args: []introspect.Arg{{Type: "s", Direction: "in"}, {Type: "a{sv}", Direction: "out"}}},
				{Name: "Set", Args: []introspect.Arg{{Type: "s", Direction: "in"}, {Type: "s", Direction: "in"}, {Type: "v", Direction: "in"}}},
			},
		},
	},
}

// MaybeStartEmbeddedWatcher acquires org.kde.StatusNotifierWatcher on the session
// bus and runs the watcher loop.  If another watcher is already present it exits
// silently.  This function blocks until the connection is lost, so call it in a
// goroutine.
//
// preRegisteredHostName is kept for API compatibility with the tray host, but
// the watcher now advertises the host as available immediately in the same way
// mature SNI watchers do. That avoids Electron/Chromium deciding too early
// that no tray host exists.
func MaybeStartEmbeddedWatcher(preRegisteredHostName string) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Printf("tray/watcher: session bus connect failed: %v", err)
		return
	}

	watcher := &StatusNotifierWatcher{
		Conn:  conn,
		Items: make(map[string]SNIEntry),
		Hosts: make(map[string]SNIEntry),
	}

	// Pre-populate the host map before the watcher name is claimed so that
	// IsStatusNotifierHostRegistered is immediately true.
	if preRegisteredHostName != "" {
		key := sniKey(preRegisteredHostName, "/StatusNotifierHost")
		watcher.Hosts[key] = SNIEntry{preRegisteredHostName, "/StatusNotifierHost"}
	}

	export := &WatcherExport{Watcher: watcher}

	// Export objects BEFORE claiming the bus name so there is never a window
	// where the watcher is addressable but not yet ready to serve calls.
	conn.Export(export, "/StatusNotifierWatcher", "org.kde.StatusNotifierWatcher")
	conn.Export(export, "/StatusNotifierWatcher", "org.freedesktop.DBus.Properties")
	conn.Export(introspect.NewIntrospectable(sniWatcherNode), "/StatusNotifierWatcher", "org.freedesktop.DBus.Introspectable")

	flags := dbus.NameFlagDoNotQueue | dbus.NameFlagAllowReplacement | dbus.NameFlagReplaceExisting
	reply, err := conn.RequestName("org.kde.StatusNotifierWatcher", flags)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		// Another watcher is already running — nothing to do.
		conn.Close()
		return
	}

	// Watch for connections disappearing so stale entries are cleaned up.
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
	); err != nil {
		log.Printf("tray/watcher: could not subscribe to NameOwnerChanged: %v", err)
	}

	signals := make(chan *dbus.Signal, 256)
	conn.Signal(signals)

	log.Printf("tray/watcher: started (org.kde.StatusNotifierWatcher)")
	watcher.emitHostSignal("StatusNotifierHostRegistered")
	watcher.discoverExistingItems()

	for sig := range signals {
		if sig == nil || sig.Name != "org.freedesktop.DBus.NameOwnerChanged" || len(sig.Body) < 3 {
			continue
		}
		name, _ := sig.Body[0].(string)
		newOwner, _ := sig.Body[2].(string)
		switch {
		case newOwner == "" && strings.HasPrefix(name, ":"):
			watcher.removeBusName(name)
		case newOwner != "" && strings.HasPrefix(name, ":"):
			watcher.discoverBusName(name)
		case newOwner != "" && (strings.HasPrefix(name, "org.kde.StatusNotifierItem") || strings.HasPrefix(name, "org.ayatana.NotificationItem")):
			watcher.discoverBusName(newOwner)
		}
	}
}
