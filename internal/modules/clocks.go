package modules

import (
	"fmt"
	"time"

	"statusbar/internal/config"

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

	// Create a horizontal box for month navigation
	monthNavBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	monthNavBox.SetHAlign(gtk.AlignFill)
	monthNavBox.SetHExpand(true)
	monthNavBox.SetName("calendar-menu-month-nav")

	// Make children expand appropriately
	prevBtn := gtk.NewButton()
	prevBtn.SetLabel("<")
	prevBtn.SetTooltipText("Previous month")
	prevBtn.SetHAlign(gtk.AlignStart)
	monthNavBox.Append(prevBtn)

	monthLabel := gtk.NewLabel("")
	monthLabel.SetName("calendar-menu-month")
	monthLabel.SetXAlign(0.5) // Center label
	monthLabel.SetHExpand(true)
	monthNavBox.Append(monthLabel)

	nextBtn := gtk.NewButton()
	nextBtn.SetLabel(">")
	nextBtn.SetTooltipText("Next month")
	nextBtn.SetHAlign(gtk.AlignEnd)
	monthNavBox.Append(nextBtn)

	menu.Append(monthNavBox)

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
	// State for displayed month/year
	type monthState struct {
		year  int
		month time.Month
	}
	var shown monthState
	var today time.Time

	updateCalendar := func() {
		now := time.Now()
		display := now.Format("Mon 02, Jan")
		monthText := time.Date(shown.year, shown.month, 1, 0, 0, 0, 0, now.Location()).Format("January 2006")
		daysInMonth := time.Date(shown.year, shown.month+1, 0, 0, 0, 0, 0, now.Location()).Day()
		firstWeekday := int(time.Date(shown.year, shown.month, 1, 0, 0, 0, 0, now.Location()).Weekday())
		startOffset := (firstWeekday + 6) % 7
		currentDay := 0
		if shown.year == today.Year() && shown.month == today.Month() {
			currentDay = today.Day()
		}

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
	}

	// Set up initial state
	today = time.Now()
	shown = monthState{year: today.Year(), month: today.Month()}

	// Button handlers
	prevBtn.ConnectClicked(func() {
		if shown.month == time.January {
			shown.month = time.December
			shown.year--
		} else {
			shown.month--
		}
		updateCalendar()
	})
	nextBtn.ConnectClicked(func() {
		if shown.month == time.December {
			shown.month = time.January
			shown.year++
		} else {
			shown.month++
		}
		updateCalendar()
	})

	// Polling for live update
	startPolling(time.Second, func() {
		now := time.Now()
		today = now
		// If showing current month, update calendar
		if shown.year == now.Year() && shown.month == now.Month() {
			updateCalendar()
		}
	})

	// Initial render
	updateCalendar()
	return module.Box
}

func NewTimeClock(cfg *config.Config) gtk.Widgetter {
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

	clocks := cfg.WorldClocks
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
