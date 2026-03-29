package modules

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

func NewPower() gtk.Widgetter {
	button := gtk.NewButtonWithLabel("⏻")
	button.SetHasFrame(false)
	button.SetName("custom-power")

	popover := gtk.NewPopover()
	popover.SetHasArrow(false)
	popover.SetAutohide(true)
	popover.SetParent(button)

	list := gtk.NewBox(gtk.OrientationVertical, 4)
	actions := []struct {
		label string
		cmd   []string
	}{
		{label: "Shutdown", cmd: []string{"poweroff"}},
		{label: "Reboot", cmd: []string{"reboot"}},
		{label: "Suspend", cmd: []string{"systemctl", "suspend"}},
		{label: "Hibernate", cmd: []string{"systemctl", "hibernate"}},
	}

	for _, action := range actions {
		action := action
		item := gtk.NewButtonWithLabel(action.label)
		item.SetHasFrame(false)
		item.ConnectClicked(func() {
			popover.Popdown()
			runDetached(action.cmd[0], action.cmd[1:]...)
		})
		list.Append(item)
	}

	popover.SetChild(list)
	attachHoverPopover(button, popover, nil, nil)

	return button
}
