package config

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type WorldClock struct {
	Name string `yaml:"name"`
	Zone string `yaml:"zone"`
}

type LanguageEntry struct {
	Match string `yaml:"match"`
	Label string `yaml:"label"`
}

type Modules struct {
	Workspaces    bool `yaml:"workspaces"`
	FocusedApp    bool `yaml:"focused_app"`
	Music         bool `yaml:"music"`
	Mode          bool `yaml:"mode"`
	Scratchpad    bool `yaml:"scratchpad"`
	DateClock     bool `yaml:"date_clock"`
	TimeClock     bool `yaml:"time_clock"`
	Notification  bool `yaml:"notification"`
	MPD           bool `yaml:"mpd"`
	Wallpaper     bool `yaml:"wallpaper"`
	Clipboard     bool `yaml:"clipboard"`
	Weather       bool `yaml:"weather"`
	Pipewire      bool `yaml:"pipewire"`
	Network       bool `yaml:"network"`
	Bluetooth     bool `yaml:"bluetooth"`
	PowerProfile  bool `yaml:"power_profile"`
	CPU           bool `yaml:"cpu"`
	Memory        bool `yaml:"memory"`
	Temperature   bool `yaml:"temperature"`
	KeyboardState bool `yaml:"keyboard_state"`
	Language      bool `yaml:"language"`
	Battery       bool `yaml:"battery"`
	Tray          bool `yaml:"tray"`
	Power         bool `yaml:"power"`
}

type WeatherConfig struct {
	Lat      string `yaml:"lat"`
	Lon      string `yaml:"lon"`
	Location string `yaml:"location"`
	OnClick  string `yaml:"on_click"`
}

type WallpaperConfig struct {
	Dir        string `yaml:"dir"`
	AutoSwitch bool   `yaml:"auto_switch"`
	Interval   int    `yaml:"interval"`
	OnClick    string `yaml:"on_click"`
}

type AudioConfig struct {
	OnClick  string `yaml:"on_click"`
	ShowText bool   `yaml:"show_text"`
}

type NetworkConfig struct {
	OnClick  string `yaml:"on_click"`
	ShowText bool   `yaml:"show_text"`
}

type BluetoothConfig struct {
	ShowText bool `yaml:"show_text"`
}

type ClocksConfig struct {
	OnClick string `yaml:"on_click"`
}

type CalendarConfig struct {
	OnClick string `yaml:"on_click"`
}

type BatteryConfig struct {
	ShowText bool `yaml:"show_text"`
}

type FocusedAppConfig struct {
	ShowEmptyWorkspace bool   `yaml:"show_empty_workspace"`
	EmptyText          string `yaml:"empty_text"`
}

type Config struct {
	Modules     Modules          `yaml:"modules"`
	WorldClocks []WorldClock     `yaml:"world_clocks"`
	Wallpaper   WallpaperConfig  `yaml:"wallpaper"`
	Weather     WeatherConfig    `yaml:"weather"`
	Audio       AudioConfig      `yaml:"audio"`
	Network     NetworkConfig    `yaml:"network"`
	Bluetooth   BluetoothConfig  `yaml:"bluetooth"`
	Clocks      ClocksConfig     `yaml:"clocks"`
	Calendar    CalendarConfig   `yaml:"calendar"`
	Battery     BatteryConfig    `yaml:"battery_config"`
	FocusedApp  FocusedAppConfig `yaml:"focused_app_config"`
	Languages   []LanguageEntry  `yaml:"languages"`
}

func defaultConfig() Config {
	return Config{
		Modules: Modules{Workspaces: false, FocusedApp: true,
			Music:         true,
			Mode:          true,
			Scratchpad:    true,
			DateClock:     true,
			TimeClock:     true,
			Notification:  true,
			MPD:           true,
			Wallpaper:     true,
			Clipboard:     true,
			Weather:       true,
			Pipewire:      true,
			Network:       true,
			Bluetooth:     true,
			PowerProfile:  false,
			CPU:           true,
			Memory:        true,
			Temperature:   true,
			KeyboardState: false,
			Language:      true,
			Battery:       true,
			Tray:          true,
			Power:         true,
		},
		WorldClocks: []WorldClock{
			{Name: "Prague", Zone: "Europe/Prague"},
			{Name: "New York", Zone: "America/New_York"},
			{Name: "Tel Aviv", Zone: "Asia/Tel_Aviv"},
			{Name: "Kyiv", Zone: "Europe/Kyiv"},
			{Name: "San Francisco", Zone: "America/Los_Angeles"},
			{Name: "Maui", Zone: "Pacific/Honolulu"},
		},
		Wallpaper: WallpaperConfig{
			Dir:        filepath.Join(mustHomeDir(), "Pictures", "wp"),
			AutoSwitch: true,
			Interval:   10,
			OnClick:    "some-wallpaper-app",
		},
		Weather: WeatherConfig{
			Lat:      "50.0755",
			Lon:      "14.4378",
			Location: "Prague",
			OnClick:  "gnome-weather",
		},
		Audio: AudioConfig{
			OnClick: "flatpak run com.saivert.pwvucontrol",
		},
		Network: NetworkConfig{
			OnClick: "nm-connection-editor",
		},
		Bluetooth: BluetoothConfig{},
		Clocks: ClocksConfig{
			OnClick: "gnome-clocks",
		},
		Calendar: CalendarConfig{
			OnClick: "gnome-calendar",
		},
		Battery: BatteryConfig{
			ShowText: true,
		},
		FocusedApp: FocusedAppConfig{
			ShowEmptyWorkspace: true,
			EmptyText:          "Desktop",
		},
		Languages: nil, // nil = use built-in formatLanguage logic
	}
}

func mustHomeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return os.Getenv("HOME")
}

func configPath() string {
	if p := os.Getenv("STATUSBAR_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(mustHomeDir(), ".config", "status-bar", "config.yaml")
}

// Load reads the config file if it exists, falling back to defaults for any
// missing fields. It never returns nil.
func Load() *Config {
	cfg := defaultConfig()
	path := configPath()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &cfg
	}
	if err != nil {
		log.Printf("config: read %s: %v", path, err)
		return &cfg
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Printf("config: parse %s: %v", path, err)
	}
	// Expand leading ~ in wallpaper dir path.
	if strings.HasPrefix(cfg.Wallpaper.Dir, "~/") {
		cfg.Wallpaper.Dir = filepath.Join(mustHomeDir(), cfg.Wallpaper.Dir[2:])
	}
	return &cfg
}
