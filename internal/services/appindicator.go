package services

// This file will contain AppIndicator detection and (future) integration logic.
// For now, it only detects if AppIndicator services are available on the system.

import (
	"context"
	"log"
	"os/exec"
	"strings"

	"github.com/godbus/dbus/v5"
)

// AppIndicatorAvailable returns true if an AppIndicator service is found in PATH or common locations.
func AppIndicatorAvailable() bool {
	return binaryExists("indicator-application-service") ||
		binaryExists("indicator-application") ||
		binaryExists("/usr/lib/x86_64-linux-gnu/indicator-application/indicator-application-service")
}

// AppIndicatorTrayItems fetches tray items from AppIndicator/StatusNotifierItem services on the session bus.
// Returns a list of busName+objectPath strings.
func AppIndicatorTrayItems() []string {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Printf("[appindicator] failed to connect to session bus: %v", err)
		return nil
	}
	defer conn.Close()

	// List all names on the bus
	var names []string
	err = conn.BusObject().CallWithContext(context.Background(), "org.freedesktop.DBus.ListNames", 0).Store(&names)
	if err != nil {
		log.Printf("[appindicator] failed to list D-Bus names: %v", err)
		return nil
	}

	var trayIDs []string
	for _, name := range names {
		if strings.HasPrefix(name, "org.kde.StatusNotifierItem") || strings.HasPrefix(name, "org.ayatana.NotificationItem") {
			// Try the default object path for StatusNotifierItem
			objectPath := "/StatusNotifierItem"
			obj := conn.Object(name, dbus.ObjectPath(objectPath))
			if hasStatusNotifierItemInterface(obj) {
				id := name + objectPath
				trayIDs = append(trayIDs, id)
				log.Printf("[appindicator] Found tray item: %s", id)
			}
		}
	}
	return trayIDs
}

// isAppIndicatorName returns true if the D-Bus name looks like an AppIndicator or StatusNotifierItem.
func isAppIndicatorName(name string) bool {
	return (len(name) > 0 && (name == "org.kde.StatusNotifierItem" ||
		name == "org.kde.StatusNotifierWatcher" ||
		name == "org.ayatana.NotificationItem" ||
		name == "org.ayatana.indicator.application" ||
		name == "org.kde.StatusNotifierHost" ||
		name == "org.kde.StatusNotifierItem-1" ||
		name == "org.kde.StatusNotifierItem-2" ||
		name == "org.kde.StatusNotifierItem-3" ||
		name == "org.kde.StatusNotifierItem-4" ||
		name == "org.kde.StatusNotifierItem-5" ||
		name == "org.kde.StatusNotifierItem-6" ||
		name == "org.kde.StatusNotifierItem-7" ||
		name == "org.kde.StatusNotifierItem-8" ||
		name == "org.kde.StatusNotifierItem-9" ||
		name == "org.kde.StatusNotifierItem-10" ||
		name == "org.kde.StatusNotifierItem-11" ||
		name == "org.kde.StatusNotifierItem-12" ||
		name == "org.kde.StatusNotifierItem-13" ||
		name == "org.kde.StatusNotifierItem-14" ||
		name == "org.kde.StatusNotifierItem-15" ||
		name == "org.kde.StatusNotifierItem-16" ||
		name == "org.kde.StatusNotifierItem-17" ||
		name == "org.kde.StatusNotifierItem-18" ||
		name == "org.kde.StatusNotifierItem-19" ||
		name == "org.kde.StatusNotifierItem-20")) ||
		(len(name) > 0 && (len(name) > 20 && name[:20] == "org.kde.StatusNotifierItem"))
}

// hasStatusNotifierItemInterface checks if the object implements the StatusNotifierItem interface.
func hasStatusNotifierItemInterface(obj dbus.BusObject) bool {
	// Try to introspect the object
	var xml string
	err := obj.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Store(&xml)
	if err != nil {
		return false
	}
	return strings.Contains(xml, "org.kde.StatusNotifierItem") || strings.Contains(xml, "org.ayatana.NotificationItem")
}

// binaryExists checks if a binary exists in PATH or at a given path.
func binaryExists(name string) bool {
	if strings.HasPrefix(name, "/") {
		_, err := exec.LookPath(name)
		return err == nil
	}
	_, err := exec.LookPath(name)
	return err == nil
}
