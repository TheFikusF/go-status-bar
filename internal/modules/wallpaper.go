package modules

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func NewWallpaper() gtk.Widgetter {
	button := gtk.NewButtonWithLabel("")
	button.SetHasFrame(false)
	button.SetName("custom-wallpaper")
	button.SetTooltipText("Shuffle wallpaper")

	button.ConnectClicked(func() {
		button.SetSensitive(false)
		button.SetTooltipText("Shuffling wallpaper...")

		go func() {
			wallpaper, err := shuffleWallpaper()
			ui(func() {
				button.SetSensitive(true)
				if err != nil {
					button.SetTooltipText(err.Error())
					return
				}

				button.SetTooltipText("Wallpaper: " + filepath.Base(wallpaper))
			})
		}()
	})

	return button
}

func shuffleWallpaper() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("wallpaper: cannot resolve home directory")
	}

	dir := filepath.Join(home, "Pictures", "wp")
	files, err := listWallpaperFiles(dir)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("wallpaper: no images in %s", dir)
	}

	wallpaper := files[rand.New(rand.NewSource(time.Now().UnixNano())).Intn(len(files))]
	if err := ensureHyprpaper(); err != nil {
		return "", err
	}

	_, _ = runCommand("hyprctl", "hyprpaper", "preload", wallpaper)

	command := ", " + wallpaper + ", cover"
	var applyErr error
	for range 3 {
		if _, applyErr = runCommand("hyprctl", "hyprpaper", "wallpaper", command); applyErr == nil {
			return wallpaper, nil
		}
		time.Sleep(350 * time.Millisecond)
	}

	if _, fallbackErr := runCommand("hyprctl", "hyprpaper", "wallpaper", ", "+wallpaper); fallbackErr == nil {
		return wallpaper, nil
	}

	return "", fmt.Errorf("wallpaper: failed to apply %s", filepath.Base(wallpaper))
}

func listWallpaperFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("wallpaper: cannot read %s", dir)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".png", ".jpg", ".jpeg", ".webp":
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}

	return files, nil
}

func ensureHyprpaper() error {
	if processRunning("hyprpaper") {
		return nil
	}

	runDetached("hyprpaper")

	deadline := time.Now().Add(3 * time.Second)
	for {
		if processRunning("hyprpaper") {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("wallpaper: hyprpaper did not start")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func processRunning(name string) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid := entry.Name()
		if pid == "" || pid[0] < '0' || pid[0] > '9' {
			continue
		}

		comm, err := os.ReadFile(filepath.Join("/proc", pid, "comm"))
		if err != nil {
			continue
		}

		if strings.TrimSpace(string(comm)) == name {
			return true
		}
	}

	return false
}
