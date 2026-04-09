package config

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type WorldClock struct {
	Name string `toml:"name"`
	Zone string `toml:"zone"`
}

type LanguageEntry struct {
	Match string `toml:"match"` // substring to match in layout name (case-insensitive)
	Label string `toml:"label"` // displayed label
}

type Modules struct {
	Workspaces    bool `toml:"workspaces"`
	FocusedApp    bool `toml:"focused_app"`
	Music         bool `toml:"music"`
	Mode          bool `toml:"mode"`
	Scratchpad    bool `toml:"scratchpad"`
	DateClock     bool `toml:"date_clock"`
	TimeClock     bool `toml:"time_clock"`
	Notification  bool `toml:"notification"`
	MPD           bool `toml:"mpd"`
	Wallpaper     bool `toml:"wallpaper"`
	Clipboard     bool `toml:"clipboard"`
	Weather       bool `toml:"weather"`
	Pipewire      bool `toml:"pipewire"`
	Network       bool `toml:"network"`
	PowerProfile  bool `toml:"power_profile"`
	CPU           bool `toml:"cpu"`
	Memory        bool `toml:"memory"`
	Temperature   bool `toml:"temperature"`
	KeyboardState bool `toml:"keyboard_state"`
	Language      bool `toml:"language"`
	Battery       bool `toml:"battery"`
	Tray          bool `toml:"tray"`
	Power         bool `toml:"power"`
}

type Config struct {
	Modules       Modules         `toml:"modules"`
	WorldClocks   []WorldClock    `toml:"world_clocks"`
	WallpapersDir string          `toml:"wallpapers_dir"`
	WeatherLat    string          `toml:"weather_lat"`
	WeatherLon    string          `toml:"weather_lon"`
	Languages     []LanguageEntry `toml:"languages"`
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
		WallpapersDir: filepath.Join(mustHomeDir(), "Pictures", "wp"),
		WeatherLat:    "50.0755",
		WeatherLon:    "14.4378",
		Languages:     nil, // nil = use built-in formatLanguage logic
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
	return filepath.Join(mustHomeDir(), ".config", "status-bar", "config.toml")
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

	if _, err := toml.Decode(string(data), &cfg); err != nil {
		log.Printf("config: parse %s: %v", path, err)
	}
	// Expand leading ~ in paths.
	if strings.HasPrefix(cfg.WallpapersDir, "~/") {
		cfg.WallpapersDir = filepath.Join(mustHomeDir(), cfg.WallpapersDir[2:])
	}
	return &cfg
}
