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

type Config struct {
	Modules             Modules         `yaml:"modules"`
	WorldClocks         []WorldClock    `yaml:"world_clocks"`
	WallpapersDir       string          `yaml:"wallpapers_dir"`
	WallpaperAutoSwitch bool            `yaml:"wallpaper_auto_switch"`
	WallpaperInterval   int             `yaml:"wallpaper_interval"`
	WeatherLat          string          `yaml:"weather_lat"`
	WeatherLon          string          `yaml:"weather_lon"`
	WeatherLocation     string          `yaml:"weather_location"`
	Languages           []LanguageEntry `yaml:"languages"`
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
		WallpapersDir:       filepath.Join(mustHomeDir(), "Pictures", "wp"),
		WallpaperAutoSwitch: true,
		WallpaperInterval:   10,
		WeatherLat:          "50.0755",
		WeatherLon:          "14.4378",
		Languages:           nil, // nil = use built-in formatLanguage logic
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
	// Expand leading ~ in paths.
	if strings.HasPrefix(cfg.WallpapersDir, "~/") {
		cfg.WallpapersDir = filepath.Join(mustHomeDir(), cfg.WallpapersDir[2:])
	}
	return &cfg
}
