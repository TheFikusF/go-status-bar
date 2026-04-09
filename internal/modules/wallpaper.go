package modules

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// wallpapersDir returns the wallpapers directory, honouring the
// STATUSBAR_WALLPAPERS_DIR environment variable as an override.
func wallpapersDir() string {
	if dir := os.Getenv("STATUSBAR_WALLPAPERS_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, "Pictures", "wp")
}

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
	// Track the active wallpaper filename; seed from hyprpaper on startup.
	currentWallpaper := filepath.Base(readCurrentWallpaper())
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
		dir := wallpapersDir()
		files, err := os.ReadDir(dir)
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

			row := gtk.NewButton()
			row.SetHasFrame(false)
			row.AddCSSClass("wallpaper-choice-row")
			if name == currentWallpaper {
				row.AddCSSClass("active")
			}

			inner := gtk.NewBox(gtk.OrientationHorizontal, 8)

			// Preview icon
			img := gtk.NewImageFromFile(filepath.Join(dir, name))
			img.SetPixelSize(48)
			img.SetVAlign(gtk.AlignCenter)
			inner.Append(img)

			// Label
			label := gtk.NewLabel(name)
			label.SetHAlign(gtk.AlignStart)
			label.SetTooltipText(name)
			label.AddCSSClass("wallpaper-choice-label")
			inner.Append(label)

			row.SetChild(inner)
			row.ConnectClicked(func() {
				go func(name string) {
					path := filepath.Join(dir, name)
					err := setWallpaper(path)
					ui(func() {
						if err != nil {
							button.SetTooltipText("Failed: " + err.Error())
						} else {
							currentWallpaper = name
							button.SetTooltipText("Wallpaper: " + name)
						}
						popover.Popdown()
					})
				}(name)
			})
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

				currentWallpaper = filepath.Base(wallpaper)
				button.SetTooltipText("Wallpaper: " + currentWallpaper)
			})
		}()
	})

	return button
}

func shuffleWallpaper() (string, error) {
	dir := wallpapersDir()
	files, err := listWallpaperFiles(dir)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("wallpaper: no images in %s", dir)
	}

	wallpaper := files[rand.Intn(len(files))]
	if err := setWallpaper(wallpaper); err != nil {
		return "", err
	}
	return wallpaper, nil
}

func listWallpaperFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Println(err)
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

func readCurrentWallpaper() string {
	out, err := runCommand("hyprctl", "hyprpaper", "listactive")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if idx := strings.Index(line, " = "); idx >= 0 {
			return strings.TrimSpace(line[idx+3:])
		}
	}
	return ""
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
