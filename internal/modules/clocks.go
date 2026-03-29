package modules

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func NewDateClock() gtk.Widgetter {
	module := newTextModule("clock-date")
	popover := gtk.NewPopover()
	popover.AddCSSClass("status-popup")
	popover.SetHasArrow(false)
	popover.SetAutohide(true)
	popover.SetParent(module.Box)

	menu := gtk.NewBox(gtk.OrientationVertical, 6)
	menu.SetName("calendar-menu")
	popover.SetChild(menu)

	monthLabel := gtk.NewLabel("")
	monthLabel.SetName("calendar-menu-month")
	monthLabel.SetXAlign(0)
	menu.Append(monthLabel)

	weekdayRow := gtk.NewGrid()
	weekdayRow.SetColumnHomogeneous(true)
	weekdayRow.SetColumnSpacing(4)
	weekdayRow.SetName("calendar-menu-weekdays")
	menu.Append(weekdayRow)

	for index, weekday := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		day := gtk.NewLabel(weekday)
		day.AddCSSClass("calendar-weekday")
		weekdayRow.Attach(day, index, 0, 1, 1)
	}

	calendarGrid := gtk.NewGrid()
	calendarGrid.SetColumnHomogeneous(true)
	calendarGrid.SetRowSpacing(4)
	calendarGrid.SetColumnSpacing(4)
	calendarGrid.SetName("calendar-menu-grid")
	menu.Append(calendarGrid)

	dayCells := make([]*gtk.Label, 0, 42)
	for row := 0; row < 6; row++ {
		for col := 0; col < 7; col++ {
			cell := gtk.NewLabel("")
			cell.AddCSSClass("calendar-day")
			calendarGrid.Attach(cell, col, row, 1, 1)
			dayCells = append(dayCells, cell)
		}
	}

	attachHoverPopover(module.Box, popover, func() { runDetached("gnome-calendar") }, nil)
	startPolling(time.Second, func() {
		now := time.Now()
		display := now.Format("Mon 02, Jan")
		monthText := now.Format("January 2006")
		daysInMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()
		firstWeekday := int(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Weekday())
		startOffset := (firstWeekday + 6) % 7
		currentDay := now.Day()

		ui(func() {
			setTextModule(module, display)
			monthLabel.SetLabel(monthText)

			for _, cell := range dayCells {
				cell.SetLabel("")
				cell.RemoveCSSClass("today")
			}

			for day := 1; day <= daysInMonth; day++ {
				index := startOffset + day - 1
				if index < 0 || index >= len(dayCells) {
					continue
				}
				cell := dayCells[index]
				cell.SetLabel(fmt.Sprintf("%2d", day))
				if day == currentDay {
					cell.AddCSSClass("today")
				}
			}
		})
	})
	return module.Box
}

func NewTimeClock() gtk.Widgetter {
	module := newTextModule("clock-time")
	popover := gtk.NewPopover()
	popover.AddCSSClass("status-popup")
	popover.SetHasArrow(false)
	popover.SetAutohide(true)
	popover.SetParent(module.Box)

	menu := gtk.NewBox(gtk.OrientationVertical, 4)
	menu.SetName("world-clock-menu")
	popover.SetChild(menu)

	title := gtk.NewLabel("World Clocks")
	title.SetName("world-clock-menu-title")
	title.SetXAlign(0)
	menu.Append(title)

	clocks := readWorldClocks()
	rows := make([]*gtk.Label, 0, len(clocks))
	for range clocks {
		row := gtk.NewLabel("")
		row.AddCSSClass("world-clock-row")
		row.SetXAlign(0)
		menu.Append(row)
		rows = append(rows, row)
	}

	attachHoverPopover(module.Box, popover, func() { runDetached("gnome-clocks") }, nil)
	startPolling(time.Second, func() {
		now := time.Now()
		localText := now.Format("15:04")
		lines := make([]string, 0, len(clocks))
		for _, clock := range clocks {
			loc, err := time.LoadLocation(clock.Zone)
			if err != nil {
				lines = append(lines, fmt.Sprintf("%s - invalid zone", clock.Name))
				continue
			}
			lines = append(lines, fmt.Sprintf("%s - %s", clock.Name, now.In(loc).Format("15:04")))
		}

		ui(func() {
			setTextModule(module, localText)
			for index := range rows {
				if index < len(lines) {
					rows[index].SetLabel(lines[index])
				}
			}
		})
	})
	return module.Box
}

func readWorldClocks() []struct {
	Name string
	Zone string
} {
	value := strings.TrimSpace(os.Getenv("STATUSBAR_WORLD_CLOCKS"))
	if value == "" {
		value = "Prague=Europe/Prague,New York=America/New_York,Tel Aviv=Asia/Tel_Aviv,Kyiv=Europe/Kyiv,San Francisco=America/Los_Angeles"
	}

	entries := strings.Split(value, ",")
	clocks := make([]struct {
		Name string
		Zone string
	}, 0, len(entries))

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, zone, ok := strings.Cut(entry, "=")
		if !ok {
			zone = name
		}
		name = strings.TrimSpace(name)
		zone = strings.TrimSpace(zone)
		if name == "" || zone == "" {
			continue
		}
		clocks = append(clocks, struct {
			Name string
			Zone string
		}{Name: name, Zone: zone})
	}

	if len(clocks) == 0 {
		return []struct {
			Name string
			Zone string
		}{
			{Name: "Prague", Zone: "Europe/Prague"},
		}
	}

	return clocks
}
