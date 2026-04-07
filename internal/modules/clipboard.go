package modules

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const clipboardMaxHistory = 50

var clipImageMIMEs = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

type clipKind int

const (
	clipKindText clipKind = iota
	clipKindImage
)

type clipEntry struct {
	Kind    clipKind
	Text    string // clipKindText
	ImgData []byte // clipKindImage
	ImgMime string // clipKindImage
}

type clipboardHistory struct {
	mu      sync.Mutex
	entries []clipEntry
}

func (h *clipboardHistory) push(e clipEntry) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch e.Kind {
	case clipKindText:
		if strings.TrimSpace(e.Text) == "" {
			return false
		}
		for i, existing := range h.entries {
			if existing.Kind == clipKindText && existing.Text == e.Text {
				h.entries = append(h.entries[:i], h.entries[i+1:]...)
				break
			}
		}
	case clipKindImage:
		if len(e.ImgData) == 0 {
			return false
		}
		// Deduplicate: skip if the most-recent entry is the same image
		// (guards against dual text+image watcher firing for same event)
		if len(h.entries) > 0 {
			last := h.entries[0]
			if last.Kind == clipKindImage && len(last.ImgData) == len(e.ImgData) {
				return false
			}
		}
	}

	h.entries = append([]clipEntry{e}, h.entries...)
	if len(h.entries) > clipboardMaxHistory {
		h.entries = h.entries[:clipboardMaxHistory]
	}
	return true
}

func (h *clipboardHistory) clear() {
	h.mu.Lock()
	h.entries = h.entries[:0]
	h.mu.Unlock()
}

func (h *clipboardHistory) snapshot() []clipEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]clipEntry, len(h.entries))
	copy(out, h.entries)
	return out
}

// readCurrentClip reads the current clipboard content (text or image).
// Uses wl-paste --list-types to detect the type, then fetches the data.
func readCurrentClip() (clipEntry, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	typesOut, err := exec.CommandContext(ctx, "wl-paste", "--list-types").Output()
	if err != nil {
		return clipEntry{}, false
	}

	mimes := strings.Fields(string(typesOut))
	for _, want := range clipImageMIMEs {
		for _, have := range mimes {
			if have == want {
				data, err := exec.CommandContext(ctx, "wl-paste", "--type", have).Output()
				if err == nil && len(data) > 0 {
					return clipEntry{Kind: clipKindImage, ImgData: data, ImgMime: have}, true
				}
			}
		}
	}

	data, err := exec.CommandContext(ctx, "wl-paste", "--no-newline").Output()
	if err != nil || len(data) == 0 {
		return clipEntry{}, false
	}
	return clipEntry{Kind: clipKindText, Text: string(data)}, true
}

// subscribeWatcher runs "wl-paste <args>" in a restart loop, calling onNotify
// for each line of output (i.e. each clipboard change event).
func subscribeWatcher(args []string, onNotify func()) {
	for {
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, "wl-paste", args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			cancel()
			time.Sleep(3 * time.Second)
			continue
		}
		if err := cmd.Start(); err != nil {
			cancel()
			time.Sleep(3 * time.Second)
			continue
		}
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			onNotify()
		}
		_ = cmd.Wait()
		cancel()
		time.Sleep(time.Second)
	}
}

// startClipboardWatchers launches background goroutines that watch for text
// and image clipboard changes.
func startClipboardWatchers(history *clipboardHistory) {
	onNotify := func() {
		entry, ok := readCurrentClip()
		if ok {
			history.push(entry)
		}
	}
	// "echo" outputs a single newline per clipboard change — one line = one event.
	go subscribeWatcher([]string{"--watch", "echo"}, onNotify)
	// Separate watcher for image-only clipboard events (e.g. screenshots with no text type).
	go subscribeWatcher([]string{"--type", "image/png", "--watch", "echo"}, onNotify)
}

// clipImageThumbnail decodes imgData into a scaled thumbnail widget.
func clipImageThumbnail(data []byte) gtk.Widgetter {
	loader := gdkpixbuf.NewPixbufLoader()
	if err := loader.Write(data); err != nil {
		_ = loader.Close()
		return gtk.NewLabel("[image]")
	}
	if err := loader.Close(); err != nil {
		return gtk.NewLabel("[image]")
	}
	pixbuf := loader.Pixbuf()
	if pixbuf == nil {
		return gtk.NewLabel("[image]")
	}

	const maxW, maxH = 200, 140
	w, h := pixbuf.Width(), pixbuf.Height()
	tw, th := maxW, maxH
	if w > 0 && h > 0 {
		if w*maxH > h*maxW {
			th = maxW * h / w
		} else {
			tw = maxH * w / h
		}
	}
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	scaled := pixbuf.ScaleSimple(tw, th, gdkpixbuf.InterpBilinear)
	if scaled == nil {
		scaled = pixbuf
	}
	pic := gtk.NewPictureForPixbuf(scaled)
	pic.SetContentFit(gtk.ContentFitContain)
	pic.SetSizeRequest(tw, th)
	return pic
}

func NewClipboard() gtk.Widgetter {
	history := &clipboardHistory{}

	button := gtk.NewButtonWithLabel("󰅇")
	button.SetHasFrame(false)
	button.SetName("custom-clipboard")
	button.AddCSSClass("module")
	button.SetTooltipText("Clipboard history")

	popover := gtk.NewPopover()
	popover.AddCSSClass("status-popup")
	popover.SetHasArrow(false)
	popover.SetAutohide(true)
	popover.SetParent(button)

	outer := gtk.NewBox(gtk.OrientationVertical, 4)
	outer.SetName("clipboard-menu")
	popover.SetChild(outer)

	header := gtk.NewBox(gtk.OrientationHorizontal, 0)
	title := gtk.NewLabel("Clipboard")
	title.SetName("clipboard-menu-title")
	title.SetHExpand(true)
	title.SetXAlign(0)
	header.Append(title)

	clearBtn := gtk.NewButtonWithLabel("󰃢")
	clearBtn.SetHasFrame(false)
	clearBtn.SetTooltipText("Clear history")
	header.Append(clearBtn)
	outer.Append(header)

	scroll := gtk.NewScrolledWindow()
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroll.SetPropagateNaturalHeight(true)
	scroll.SetMaxContentHeight(400)
	scroll.SetSizeRequest(-1, 400)
	outer.Append(scroll)

	listBox := gtk.NewBox(gtk.OrientationVertical, 2)
	listBox.SetName("clipboard-list")
	scroll.SetChild(listBox)

	rebuildList := func() {
		for child := listBox.FirstChild(); child != nil; child = listBox.FirstChild() {
			listBox.Remove(child)
		}
		entries := history.snapshot()
		if len(entries) == 0 {
			empty := gtk.NewLabel("No clipboard history")
			empty.AddCSSClass("clipboard-empty")
			listBox.Append(empty)
			return
		}
		for _, entry := range entries {
			entry := entry
			row := gtk.NewButton()
			row.SetHasFrame(false)
			row.AddCSSClass("clipboard-row")

			switch entry.Kind {
			case clipKindImage:
				row.SetChild(clipImageThumbnail(entry.ImgData))
			case clipKindText:
				preview := strings.ReplaceAll(entry.Text, "\n", " ")
				runes := []rune(preview)
				if len(runes) > 80 {
					preview = string(runes[:80]) + "…"
				}
				lbl := gtk.NewLabel(preview)
				lbl.SetHAlign(gtk.AlignStart)
				lbl.SetMaxWidthChars(42)
				row.SetChild(lbl)
			}

			row.ConnectClicked(func() {
				go func() {
					var cmd *exec.Cmd
					switch entry.Kind {
					case clipKindText:
						cmd = exec.Command("wl-copy")
						cmd.Stdin = bytes.NewReader([]byte(entry.Text))
					case clipKindImage:
						cmd = exec.Command("wl-copy", "--type", entry.ImgMime)
						cmd.Stdin = bytes.NewReader(entry.ImgData)
					}
					_ = cmd.Run()
					ui(func() { popover.Popdown() })
				}()
			})
			listBox.Append(row)
		}
	}

	clearBtn.ConnectClicked(func() {
		history.clear()
		rebuildList()
	})

	attachHoverPopover(button, popover, nil, rebuildList)

	startClipboardWatchers(history)

	return button
}
