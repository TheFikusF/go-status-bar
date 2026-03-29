package modules

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type weatherResponse struct {
	Current struct {
		Temperature float64 `json:"temperature_2m"`
		Code        int     `json:"weather_code"`
	} `json:"current"`
	Daily struct {
		Time           []string  `json:"time"`
		TemperatureMax []float64 `json:"temperature_2m_max"`
		WeatherCode    []int     `json:"weather_code"`
	} `json:"daily"`
}

type weatherSnapshot struct {
	CurrentText string
	Forecast    []string
}

func NewWeather() gtk.Widgetter {
	module := newTextModule("custom-weather")
	popover := gtk.NewPopover()
	popover.AddCSSClass("status-popup")
	popover.SetHasArrow(false)
	popover.SetAutohide(true)
	popover.SetParent(module.Box)

	menu := gtk.NewBox(gtk.OrientationVertical, 4)
	menu.SetName("weather-menu")
	popover.SetChild(menu)

	title := gtk.NewLabel("This Week")
	title.SetName("weather-menu-title")
	title.SetXAlign(0)
	menu.Append(title)

	forecastRows := make([]*gtk.Label, 0, 7)
	for range 7 {
		row := gtk.NewLabel("")
		row.AddCSSClass("weather-forecast-row")
		row.SetXAlign(0)
		menu.Append(row)
		forecastRows = append(forecastRows, row)
	}

	refreshRequests := make(chan struct{}, 1)
	requestRefresh := func() {
		select {
		case refreshRequests <- struct{}{}:
		default:
		}
	}

	go func() {
		for range refreshRequests {
			snapshot := readWeatherSnapshot()
			ui(func() {
				setTextModule(module, snapshot.CurrentText)
				for i := range forecastRows {
					if i < len(snapshot.Forecast) {
						forecastRows[i].SetLabel(snapshot.Forecast[i])
						forecastRows[i].SetVisible(true)
					} else {
						forecastRows[i].SetLabel("")
						forecastRows[i].SetVisible(false)
					}
				}
			})
		}
	}()

	attachHoverPopover(module.Box, popover, func() { runDetached("gnome-weather") }, nil)
	requestRefresh()
	startPolling(30*time.Minute, func() {
		requestRefresh()
	})
	return module.Box
}

func readWeatherSnapshot() weatherSnapshot {
	lat := os.Getenv("STATUSBAR_LAT")
	lon := os.Getenv("STATUSBAR_LON")
	if lat == "" || lon == "" {
		lat = "50.0755"
		lon = "14.4378"
	}

	endpoint := "https://api.open-meteo.com/v1/forecast?latitude=" + url.QueryEscape(lat) +
		"&longitude=" + url.QueryEscape(lon) +
		"&current=temperature_2m,weather_code" +
		"&daily=weather_code,temperature_2m_max&timezone=auto&forecast_days=7"

	var payload weatherResponse
	if err := fetchJSON(endpoint, &payload); err != nil {
		return weatherSnapshot{}
	}

	result := weatherSnapshot{
		CurrentText: fmt.Sprintf("%.0fC %s", payload.Current.Temperature, weatherIcon(payload.Current.Code)),
	}

	count := minInt(len(payload.Daily.Time), len(payload.Daily.TemperatureMax), len(payload.Daily.WeatherCode), 7)
	for i := 0; i < count; i++ {
		parsedDate, err := time.Parse("2006-01-02", strings.TrimSpace(payload.Daily.Time[i]))
		if err != nil {
			continue
		}
		line := fmt.Sprintf(
			"%s - %.0fC %s",
			parsedDate.Format("Jan, 02"),
			payload.Daily.TemperatureMax[i],
			weatherIcon(payload.Daily.WeatherCode[i]),
		)
		result.Forecast = append(result.Forecast, line)
	}

	return result
}

func weatherIcon(code int) string {
	switch {
	case code == 0:
		return "☀️"
	case code <= 3:
		return "⛅"
	case code < 60:
		return "🌫"
	case code < 70:
		return "🌧"
	case code < 80:
		return "❄️"
	case code < 100:
		return "⛈️"
	default:
		return "🌦️"
	}
}

func minInt(values ...int) int {
	if len(values) == 0 {
		return 0
	}
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}
