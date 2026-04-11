package modules

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"statusbar/internal/config"

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

func NewWeather(cfg *config.Config) gtk.Widgetter {
	module := newTextModule("custom-weather")
	popover := gtk.NewPopover()
	popover.AddCSSClass("status-popup")
	popover.SetHasArrow(false)
	popover.SetAutohide(true)
	popover.SetParent(module.Box)

	menu := gtk.NewBox(gtk.OrientationVertical, 4)
	menu.SetName("weather-menu")
	popover.SetChild(menu)

	titleText := "Weather"
	if cfg.WeatherLocation != "" {
		titleText = "Weather — " + cfg.WeatherLocation
	}
	title := gtk.NewLabel(titleText)
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
			snapshot := readWeatherSnapshot(cfg.WeatherLat, cfg.WeatherLon)
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

func readWeatherSnapshot(lat, lon string) weatherSnapshot {
	if lat == "" {
		lat = "50.0755"
	}
	if lon == "" {
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
		CurrentText: fmt.Sprintf("%.0f°C %s", payload.Current.Temperature, weatherIconShort(payload.Current.Code)),
	}

	count := minInt(len(payload.Daily.Time), len(payload.Daily.TemperatureMax), len(payload.Daily.WeatherCode), 7)
	for i := 0; i < count; i++ {
		parsedDate, err := time.Parse("2006-01-02", strings.TrimSpace(payload.Daily.Time[i]))
		if err != nil {
			continue
		}
		line := fmt.Sprintf(
			"%s  %.0f°C  %s",
			parsedDate.Format("Mon, Jan 02"),
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
		return "☀️ Clear"
	case code == 1:
		return "🌤️ Mostly clear"
	case code == 2:
		return "⛅ Partly cloudy"
	case code == 3:
		return "☁️ Overcast"
	case code == 45 || code == 48:
		return "🌫️ Fog"
	case code >= 51 && code <= 55:
		return "🌦️ Drizzle"
	case code >= 56 && code <= 57:
		return "🌧️ Freezing drizzle"
	case code >= 61 && code <= 65:
		return "🌧️ Rain"
	case code >= 66 && code <= 67:
		return "🌧️ Freezing rain"
	case code >= 71 && code <= 75:
		return "❄️ Snow"
	case code == 77:
		return "🌨️ Snow grains"
	case code >= 80 && code <= 82:
		return "🌧️ Showers"
	case code >= 85 && code <= 86:
		return "🌨️ Snow showers"
	case code == 95:
		return "⛈️ Thunderstorm"
	case code >= 96 && code <= 99:
		return "⛈️ Thunderstorm + hail"
	default:
		return "🌡️"
	}
}

func weatherIconShort(code int) string {
	switch {
	case code == 0:
		return "☀️"
	case code == 1:
		return "🌤️"
	case code == 2:
		return "⛅"
	case code == 3:
		return "☁️"
	case code == 45 || code == 48:
		return "🌫️"
	case code >= 51 && code <= 57:
		return "🌦️"
	case code >= 61 && code <= 67:
		return "🌧️"
	case code >= 71 && code <= 77:
		return "❄️"
	case code >= 80 && code <= 86:
		return "🌨️"
	case code >= 95:
		return "⛈️"
	default:
		return "🌡️"
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
