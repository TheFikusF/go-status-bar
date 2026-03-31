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

const (
	WALLPAPERS_DIR = "$HOME/Pictures/wp"
)

// setWallpaper sets the wallpaper to the given path using hyprpaper logic
func setWallpaper(path string) error {
	if err := ensureHyprpaper(); err != nil {
		return err
	}
	_, _ = runCommand("hyprctl", "hyprpaper", "preload", path)
	command := ", " + path + ", cover"
	var applyErr error
	for range 3 {
		if _, applyErr = runCommand("hyprctl", "hyprpaper", "wallpaper", command); applyErr == nil {
			return nil
		}
		time.Sleep(350 * time.Millisecond)
	}
	if _, fallbackErr := runCommand("hyprctl", "hyprpaper", "wallpaper", ", "+path); fallbackErr == nil {
		return nil
	}
	return fmt.Errorf("wallpaper: failed to apply %s", filepath.Base(path))
}

func NewWallpaper() gtk.Widgetter {
	button := gtk.NewButtonWithLabel("")
	button.SetHasFrame(false)
	button.SetName("custom-wallpaper")
	button.SetTooltipText("Shuffle wallpaper")

	// Create popover for wallpaper selection

	popover := gtk.NewPopover()
	popover.AddCSSClass("status-popup")
	popover.SetHasArrow(false)
	popover.SetAutohide(true)
	popover.SetParent(button)

	menu := gtk.NewBox(gtk.OrientationVertical, 4)
	menu.SetName("wallpaper-menu")
	popover.SetChild(menu)

	title := gtk.NewLabel("Wallpapers")
	title.SetName("wallpaper-menu-title")
	title.SetXAlign(0)
	menu.Append(title)

	listBox := gtk.NewBox(gtk.OrientationVertical, 2)
	menu.Append(listBox)

	// Function to update popover content
	updatePopover := func() {
		// Clear previous children
		for child := listBox.FirstChild(); child != nil; child = listBox.FirstChild() {
			listBox.Remove(child)
		}
		// List wallpapers (example: from ~/Pictures/Wallpapers)
		wallpapersDir := os.Getenv("STATUSBAR_WALLPAPERS_DIR")
		if wallpapersDir == "" {
			wallpapersDir = os.ExpandEnv(WALLPAPERS_DIR)
		}
		files, err := os.ReadDir(wallpapersDir)
		if err != nil {
			label := gtk.NewLabel("No wallpapers found")
			label.AddCSSClass("wallpaper-choice")
			listBox.Append(label)
			return
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			name := file.Name()
			row := gtk.NewBox(gtk.OrientationHorizontal, 8)
			row.AddCSSClass("wallpaper-choice-row")

			// Preview icon
			img := gtk.NewImageFromFile(filepath.Join(wallpapersDir, name))
			img.SetPixelSize(48)
			img.SetVAlign(gtk.AlignCenter)
			row.Append(img)

			// Label
			label := gtk.NewLabel(name)
			label.SetHAlign(gtk.AlignStart)
			label.SetTooltipText(name)
			label.AddCSSClass("wallpaper-choice-label")
			row.Append(label)

			// Click to set wallpaper using GestureClick
			click := gtk.NewGestureClick()
			click.ConnectPressed(func(_ int, _, _ float64) {
				go func(name string) {
					path := filepath.Join(wallpapersDir, name)
					err := setWallpaper(path)
					ui(func() {
						if err != nil {
							button.SetTooltipText("Failed: " + err.Error())
						} else {
							button.SetTooltipText("Wallpaper: " + name)
						}
						popover.Popdown()
					})
				}(name)
			})
			row.AddController(click)
			listBox.Append(row)
		}
	}

	attachHoverPopover(button, popover, nil, updatePopover)

	// Keep shuffle on click
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
	files, err := listWallpaperFiles(WALLPAPERS_DIR)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("wallpaper: no images in %s", WALLPAPERS_DIR)
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
