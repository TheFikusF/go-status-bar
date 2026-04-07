package services

import (
	"log"
	"strings"

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

func sniKey(busName, objectPath string) string {
	return busName + "|" + objectPath
}

// parseSNIService resolves the (busName, objectPath) from a RegisterStatusNotifier*
// service string.  Electron/Chromium apps send just an object path (starting with
// '/'); in that case the D-Bus sender's unique name is used as the bus name.  All
// other apps send a well-known bus name; the default path is "/StatusNotifierItem".
func parseSNIService(sender dbus.Sender, service string) (string, string) {
	if len(service) > 0 && service[0] == '/' {
		return string(sender), service
	}
	return service, "/StatusNotifierItem"
}

// --- org.kde.StatusNotifierWatcher methods ---

func (w *WatcherExport) RegisterStatusNotifierItem(sender dbus.Sender, service string) *dbus.Error {
	busName, objectPath := parseSNIService(sender, service)
	key := sniKey(busName, objectPath)
	if _, exists := w.Watcher.Items[key]; exists {
		return nil
	}
	w.Watcher.Items[key] = SNIEntry{busName, objectPath}
	w.Watcher.emitPropertiesChanged()
	w.Watcher.emitSignal("StatusNotifierItemRegistered", busName+objectPath)
	log.Printf("tray/watcher: registered item %s%s", busName, objectPath)
	return nil
}

func (w *WatcherExport) RegisterStatusNotifierHost(sender dbus.Sender, service string) *dbus.Error {
	busName, objectPath := parseSNIService(sender, service)
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

func (w *WatcherExport) Get(interfaceName, property string) (interface{}, *dbus.Error) {
	if interfaceName != "org.kde.StatusNotifierWatcher" {
		return nil, nil
	}
	switch property {
	case "RegisteredStatusNotifierItems":
		items := make([]string, 0, len(w.Watcher.Items))
		for _, e := range w.Watcher.Items {
			items = append(items, e.BusName+e.ObjectPath)
		}
		return items, nil
	case "IsStatusNotifierHostRegistered":
		return len(w.Watcher.Hosts) > 0, nil
	case "ProtocolVersion":
		return int32(0), nil
	}
	return nil, nil
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
		"IsStatusNotifierHostRegistered": dbus.MakeVariant(len(w.Watcher.Hosts) > 0),
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
			"IsStatusNotifierHostRegistered": dbus.MakeVariant(len(w.Hosts) > 0),
		},
		[]string{},
	)
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
// preRegisteredHostName, if non-empty, is added to the hosts map BEFORE the
// watcher name is claimed.  This ensures IsStatusNotifierHostRegistered is true
// from the very first moment the watcher is visible on the bus.  Electron apps
// subscribe to NameOwnerChanged for org.kde.StatusNotifierWatcher and check
// IsStatusNotifierHostRegistered immediately; without pre-registration they see
// false and permanently fall back to XEmbed.
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

	signals := make(chan *dbus.Signal, 32)
	conn.Signal(signals)

	log.Printf("tray/watcher: started (org.kde.StatusNotifierWatcher)")

	for sig := range signals {
		if sig == nil || sig.Name != "org.freedesktop.DBus.NameOwnerChanged" || len(sig.Body) < 3 {
			continue
		}
		name, _ := sig.Body[0].(string)
		newOwner, _ := sig.Body[2].(string)
		// Only care about unique names (":1.xxx") losing their owner.
		if newOwner == "" && strings.HasPrefix(name, ":") {
			watcher.removeBusName(name)
		}
	}
}
