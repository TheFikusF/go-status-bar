package modules

import (
	"bufio"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type swayncState struct {
	Class string
	Count int
}

func NewNotification() gtk.Widgetter {
	module := newTextModule("custom-notification")
	attachClick(module.Box, func() {
		runDetached("swaync-client", "-t", "-sw")
	}, func() {
		runDetached("swaync-client", "-d", "-sw")
	})

	refresh := func() {
		state, ok := readNotificationState()
		if !ok {
			ui(func() { setTextModule(module, "") })
			return
		}

		text := notificationIcon(state.Class)
		text += strconv.Itoa(state.Count)

		ui(func() {
			setTextModule(module, text)
			switch state.Class {
			case "dnd-notification", "dnd-none", "dnd-inhibited-notification", "dnd-inhibited-none":
				module.Box.AddCSSClass("dnd")
			default:
				module.Box.RemoveCSSClass("dnd")
			}
		})
	}

	refresh()
	go subscribeNotifications(refresh)
	startPolling(30*time.Second, refresh)

	return module.Box
}

func subscribeNotifications(refresh func()) {
	for {
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, "swaync-client", "--skip-wait", "--subscribe-waybar")

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			cancel()
			time.Sleep(3 * time.Second)
			continue
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			cancel()
			time.Sleep(3 * time.Second)
			continue
		}

		if err := cmd.Start(); err != nil {
			cancel()
			time.Sleep(3 * time.Second)
			continue
		}

		refresh()

		done := make(chan struct{}, 2)
		scan := func(scanner *bufio.Scanner) {
			for scanner.Scan() {
				refresh()
			}
			done <- struct{}{}
		}

		go scan(bufio.NewScanner(stdout))
		go scan(bufio.NewScanner(stderr))

		<-done
		cancel()
		_ = cmd.Wait()
		time.Sleep(2 * time.Second)
	}
}

func readNotificationState() (swayncState, bool) {
	count := 0
	if output, err := runCommand("swaync-client", "--skip-wait", "--count"); err == nil {
		count, _ = strconv.Atoi(strings.TrimSpace(string(output)))
	}

	dnd := commandIsTrue("swaync-client", "--skip-wait", "--get-dnd")
	inhibited := commandIsTrue("swaync-client", "--skip-wait", "--get-inhibited")

	class := "none"
	if count > 0 {
		class = "notification"
	}
	if inhibited {
		if count > 0 {
			class = "inhibited-notification"
		} else {
			class = "inhibited-none"
		}
	}
	if dnd {
		if count > 0 {
			class = "dnd-notification"
		} else {
			class = "dnd-none"
		}
		if inhibited {
			if count > 0 {
				class = "dnd-inhibited-notification"
			} else {
				class = "dnd-inhibited-none"
			}
		}
	}

	return swayncState{Class: class, Count: count}, true
}

func commandIsTrue(name string, args ...string) bool {
	output, err := runCommand(name, args...)
	if err != nil {
		return false
	}

	value := strings.TrimSpace(strings.ToLower(string(output)))
	return value == "true" || value == "1" || value == "on" || value == "enabled"
}

func notificationIcon(class string) string {
	switch class {
	case "notification":
		return "󰂚"
	case "none":
		return "󰂚"
	case "dnd-notification":
		return "󰂛"
	case "dnd-none":
		return "󰂛"
	case "inhibited-notification":
		return "󰂚"
	case "inhibited-none":
		return "󰂚"
	case "dnd-inhibited-notification":
		return "󰂛"
	case "dnd-inhibited-none":
		return "󰂛"
	default:
		return "󰂚"
	}
}
