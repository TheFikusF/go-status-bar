package services

// This file will contain the StatusNotifierWatcher (SNI) implementation extracted from modules/tray.go.
// The watcher provides the org.kde.StatusNotifierWatcher D-Bus service for tray icons.

import (
	"log"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

type SNIEntry struct {
	BusName    string
	ObjectPath string
}

type StatusNotifierWatcher struct {
	Conn  *dbus.Conn
	Items map[string]SNIEntry // key: busName+objectPath
	Hosts map[string]SNIEntry
}

type WatcherExport struct {
	Watcher *StatusNotifierWatcher
}

// --- org.freedesktop.DBus.Properties full support ---
func (w *WatcherExport) GetAll(interfaceName string) (map[string]dbus.Variant, *dbus.Error) {
	log.Printf("SNI: GetAll called for interface %s", interfaceName)
	if interfaceName != "org.kde.StatusNotifierWatcher" {
		return map[string]dbus.Variant{}, nil
	}
	items, _ := w.GetRegisteredStatusNotifierItems()
	host, _ := w.GetIsStatusNotifierHostRegistered()
	return map[string]dbus.Variant{
		"RegisteredStatusNotifierItems":  dbus.MakeVariant(items),
		"IsStatusNotifierHostRegistered": dbus.MakeVariant(host),
	}, nil
}

func (w *WatcherExport) Set(interfaceName, property string, value dbus.Variant) *dbus.Error {
	log.Printf("SNI: Set called for %s.%s = %v (ignored)", interfaceName, property, value)
	// SNI watcher properties are read-only, ignore
	return nil
}

func sniKey(busName, objectPath string) string {
	return busName + "|" + objectPath
}

func (w *WatcherExport) GetRegisteredStatusNotifierItems() ([]string, *dbus.Error) {
	items := make([]string, 0, len(w.Watcher.Items))
	for _, entry := range w.Watcher.Items {
		items = append(items, entry.BusName+entry.ObjectPath)
	}
	return items, nil
}
func (w *WatcherExport) GetIsStatusNotifierHostRegistered() (bool, *dbus.Error) {
	return len(w.Watcher.Hosts) > 0, nil
}

func (w *WatcherExport) RegisterStatusNotifierItem(service string) *dbus.Error {
	busName, objectPath := parseSNIService(service, w.Watcher.Conn)
	key := sniKey(busName, objectPath)
	if _, exists := w.Watcher.Items[key]; exists {
		log.Printf("SNI: Duplicate item registration for %s %s", busName, objectPath)
		return nil
	}
	w.Watcher.Items[key] = SNIEntry{busName, objectPath}
	w.Watcher.emitPropertiesChanged()
	w.Watcher.emitSignal("StatusNotifierItemRegistered", busName+objectPath)
	log.Printf("SNI: Registered item %s %s", busName, objectPath)
	return nil
}
func (w *WatcherExport) RegisterStatusNotifierHost(service string) *dbus.Error {
	busName, objectPath := parseSNIService(service, w.Watcher.Conn)
	key := sniKey(busName, objectPath)
	if _, exists := w.Watcher.Hosts[key]; exists {
		log.Printf("SNI: Duplicate host registration for %s %s", busName, objectPath)
		return nil
	}
	w.Watcher.Hosts[key] = SNIEntry{busName, objectPath}
	w.Watcher.emitPropertiesChanged()
	w.Watcher.emitSignal("StatusNotifierHostRegistered", busName+objectPath)
	log.Printf("SNI: Registered host %s %s", busName, objectPath)
	return nil
}

func (w *WatcherExport) UnregisterStatusNotifierItem(service string) *dbus.Error {
	busName, objectPath := parseSNIService(service, w.Watcher.Conn)
	key := sniKey(busName, objectPath)
	if _, exists := w.Watcher.Items[key]; exists {
		delete(w.Watcher.Items, key)
		w.Watcher.emitPropertiesChanged()
		w.Watcher.emitSignal("StatusNotifierItemUnregistered", busName+objectPath)
		log.Printf("SNI: Unregistered item %s %s", busName, objectPath)
	}
	return nil
}
func (w *WatcherExport) UnregisterStatusNotifierHost(service string) *dbus.Error {
	busName, objectPath := parseSNIService(service, w.Watcher.Conn)
	key := sniKey(busName, objectPath)
	if _, exists := w.Watcher.Hosts[key]; exists {
		delete(w.Watcher.Hosts, key)
		w.Watcher.emitPropertiesChanged()
		w.Watcher.emitSignal("StatusNotifierHostUnregistered", busName+objectPath)
		log.Printf("SNI: Unregistered host %s %s", busName, objectPath)
	}
	return nil
}

func (w *WatcherExport) Get(interfaceName, property string) (interface{}, *dbus.Error) {
	log.Printf("SNI: Get called for %s.%s", interfaceName, property)
	if interfaceName != "org.kde.StatusNotifierWatcher" {
		return nil, nil
	}
	switch property {
	case "RegisteredStatusNotifierItems":
		return w.GetRegisteredStatusNotifierItems()
	case "IsStatusNotifierHostRegistered":
		return w.GetIsStatusNotifierHostRegistered()
	}
	return nil, nil
}

func parseSNIService(service string, conn *dbus.Conn) (string, string) {
	if len(service) > 0 && service[0] == '/' {
		busName := ""
		if conn != nil {
			busName = conn.Names()[0]
		}
		return busName, service
	}
	return service, "/StatusNotifierItem"
}

func (w *StatusNotifierWatcher) emitSignal(signal string, value string) {
	if w.Conn != nil {
		w.Conn.Emit(
			"/StatusNotifierWatcher",
			"org.kde.StatusNotifierWatcher."+signal,
			value,
		)
	}
}
func (w *StatusNotifierWatcher) emitPropertiesChanged() {
	if w.Conn != nil {
		items := make([]string, 0, len(w.Items))
		for _, entry := range w.Items {
			items = append(items, entry.BusName+entry.ObjectPath)
		}
		changed := map[string]dbus.Variant{
			"RegisteredStatusNotifierItems":  dbus.MakeVariant(items),
			"IsStatusNotifierHostRegistered": dbus.MakeVariant(len(w.Hosts) > 0),
		}
		w.Conn.Emit(
			"/StatusNotifierWatcher",
			"org.freedesktop.DBus.Properties.PropertiesChanged",
			"org.kde.StatusNotifierWatcher",
			changed,
			[]string{},
		)
	}
}

// Starts the embedded watcher if none is present
func MaybeStartEmbeddedWatcher() {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Printf("tray: failed to connect to session bus for watcher: %v", err)
		return
	}
	flags := dbus.NameFlagDoNotQueue | dbus.NameFlagAllowReplacement | dbus.NameFlagReplaceExisting
	reply, err := conn.RequestName("org.kde.StatusNotifierWatcher", flags)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		log.Printf("tray: failed to acquire watcher name: %v, reply=%d", err, reply)
		conn.Close()
		return // Another watcher is running
	}
	watcher := &StatusNotifierWatcher{
		Conn:  conn,
		Items: make(map[string]SNIEntry),
		Hosts: make(map[string]SNIEntry),
	}
	export := &WatcherExport{Watcher: watcher}
	// Export methods and properties
	conn.Export(export, "/StatusNotifierWatcher", "org.kde.StatusNotifierWatcher")
	conn.Export(export, "/StatusNotifierWatcher", "org.freedesktop.DBus.Properties")
	// Export custom introspection XML for full SNI compatibility
	node := &introspect.Node{
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
				},
				Signals: []introspect.Signal{
					{Name: "StatusNotifierItemRegistered", Args: []introspect.Arg{{Type: "s"}}},
					{Name: "StatusNotifierItemUnregistered", Args: []introspect.Arg{{Type: "s"}}},
					{Name: "StatusNotifierHostRegistered", Args: []introspect.Arg{{Type: "s"}}},
					{Name: "StatusNotifierHostUnregistered", Args: []introspect.Arg{{Type: "s"}}},
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
	conn.Export(introspect.NewIntrospectable(node), "/StatusNotifierWatcher", "org.freedesktop.DBus.Introspectable")
	log.Printf("tray: embedded StatusNotifierWatcher started on D-Bus with full introspection")
	// Keep connection alive
	select {}
}
