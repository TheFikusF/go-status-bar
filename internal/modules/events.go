package modules

import (
	"bufio"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

type hyprEvent struct {
	Name string
	Data string
}

var (
	hyprOnce        sync.Once
	hyprMu          sync.Mutex
	hyprSubscribers []chan hyprEvent

	mprisOnce        sync.Once
	mprisMu          sync.Mutex
	mprisSubscribers []chan struct{}
)

func subscribeHyprEvents() (<-chan hyprEvent, func()) {
	ch := make(chan hyprEvent, 16)

	hyprMu.Lock()
	hyprSubscribers = append(hyprSubscribers, ch)
	hyprMu.Unlock()

	hyprOnce.Do(func() {
		go runHyprEventLoop()
	})

	unsubscribe := func() {
		hyprMu.Lock()
		for i, s := range hyprSubscribers {
			if s == ch {
				hyprSubscribers = append(hyprSubscribers[:i], hyprSubscribers[i+1:]...)
				break
			}
		}
		hyprMu.Unlock()
	}

	return ch, unsubscribe
}

func runHyprEventLoop() {
	for {
		path := hyprSocketPath()
		if path == "" {
			time.Sleep(5 * time.Second)
			continue
		}

		conn, err := net.Dial("unix", path)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			name, data, ok := strings.Cut(line, ">>")
			if !ok {
				continue
			}
			broadcastHyprEvent(hyprEvent{Name: name, Data: data})
		}

		_ = conn.Close()
		time.Sleep(time.Second)
	}
}

func broadcastHyprEvent(event hyprEvent) {
	hyprMu.Lock()
	subscribers := append([]chan hyprEvent(nil), hyprSubscribers...)
	hyprMu.Unlock()

	for _, subscriber := range subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func hyprSocketPath() string {
	signature := strings.TrimSpace(os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"))
	if signature == "" {
		return ""
	}

	runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if runtimeDir == "" {
		runtimeDir = filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
	}

	return filepath.Join(runtimeDir, "hypr", signature, ".socket2.sock")
}

func subscribeMPRISEvents() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 16)

	mprisMu.Lock()
	mprisSubscribers = append(mprisSubscribers, ch)
	mprisMu.Unlock()

	mprisOnce.Do(func() {
		go runMPRISEventLoop()
	})

	unsubscribe := func() {
		mprisMu.Lock()
		for i, s := range mprisSubscribers {
			if s == ch {
				mprisSubscribers = append(mprisSubscribers[:i], mprisSubscribers[i+1:]...)
				break
			}
		}
		mprisMu.Unlock()
	}

	return ch, unsubscribe
}

func runMPRISEventLoop() {
	for {
		conn, err := dbus.ConnectSessionBus()
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		if err := conn.AddMatchSignal(
			dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
			dbus.WithMatchMember("PropertiesChanged"),
			dbus.WithMatchObjectPath("/org/mpris/MediaPlayer2"),
		); err != nil {
			log.Printf("mpris signal match failed: %v", err)
		}

		if err := conn.AddMatchSignal(
			dbus.WithMatchInterface("org.mpris.MediaPlayer2.Player"),
			dbus.WithMatchMember("Seeked"),
		); err != nil {
			log.Printf("mpris seeked match failed: %v", err)
		}

		if err := conn.AddMatchSignal(
			dbus.WithMatchInterface("org.freedesktop.DBus"),
			dbus.WithMatchMember("NameOwnerChanged"),
		); err != nil {
			log.Printf("dbus name-owner match failed: %v", err)
		}

		signals := make(chan *dbus.Signal, 32)
		conn.Signal(signals)
		broadcastMPRISEvent()

		for signal := range signals {
			if signal == nil {
				break
			}

			switch signal.Name {
			case "org.freedesktop.DBus.Properties.PropertiesChanged":
				if len(signal.Body) > 0 {
					if iface, ok := signal.Body[0].(string); ok && iface == "org.mpris.MediaPlayer2.Player" {
						broadcastMPRISEvent()
					}
				}
			case "org.mpris.MediaPlayer2.Player.Seeked":
				broadcastMPRISEvent()
			case "org.freedesktop.DBus.NameOwnerChanged":
				if len(signal.Body) > 0 {
					if name, ok := signal.Body[0].(string); ok && strings.HasPrefix(name, "org.mpris.MediaPlayer2.") {
						broadcastMPRISEvent()
					}
				}
			}
		}

		conn.RemoveSignal(signals)
		_ = conn.Close()
		time.Sleep(time.Second)
	}
}

func broadcastMPRISEvent() {
	mprisMu.Lock()
	subscribers := append([]chan struct{}(nil), mprisSubscribers...)
	mprisMu.Unlock()

	for _, subscriber := range subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}
