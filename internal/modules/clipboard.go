package modules

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"os"
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
	Text    string   // clipKindText
	ImgPath string   // clipKindImage
	ImgMime string   // clipKindImage
	ImgHash [32]byte // clipKindImage
}

type clipboardRowState struct {
	button   *gtk.Button
	label    *gtk.Label
	picture  *gtk.Picture
	entry    clipEntry
	hasEntry bool
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
		if e.ImgPath == "" {
			e.release()
			return false
		}
		// Deduplicate: skip if the most-recent entry is the same image
		// (guards against dual text+image watcher firing for same event)
		if len(h.entries) > 0 {
			last := h.entries[0]
			if last.Kind == clipKindImage && last.ImgMime == e.ImgMime && last.ImgHash == e.ImgHash {
				e.release()
				return false
			}
		}
	}

	h.entries = append([]clipEntry{e}, h.entries...)
	if len(h.entries) > clipboardMaxHistory {
		for _, stale := range h.entries[clipboardMaxHistory:] {
			stale.release()
		}
		h.entries = h.entries[:clipboardMaxHistory]
	}
	return true
}

func (h *clipboardHistory) clear() {
	h.mu.Lock()
	for _, entry := range h.entries {
		entry.release()
	}
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
					entry, err := newClipboardImageEntry(data, have)
					if err == nil {
						return entry, true
					}
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

func newClipboardImageEntry(data []byte, mime string) (clipEntry, error) {
	hash := sha256.Sum256(data)
	file, err := os.CreateTemp("", "statusbar-clipboard-image-*")
	if err != nil {
		return clipEntry{}, err
	}
	path := file.Name()
	if _, err := file.Write(data); err != nil {
		file.Close()
		_ = os.Remove(path)
		return clipEntry{}, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return clipEntry{}, err
	}
	return clipEntry{Kind: clipKindImage, ImgPath: path, ImgMime: mime, ImgHash: hash}, nil
}

func (e clipEntry) release() {
	if e.Kind == clipKindImage && e.ImgPath != "" {
		_ = os.Remove(e.ImgPath)
	}
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

// clipImageThumbnail loads a scaled thumbnail pixbuf from the stored image file.
func clipImageThumbnail(path string) *gdkpixbuf.Pixbuf {
	pixbuf, err := gdkpixbuf.NewPixbufFromFileAtScale(path, 200, 140, true)
	if err != nil {
		return nil
	}
	return pixbuf
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
	empty := gtk.NewLabel("No clipboard history")
	empty.AddCSSClass("clipboard-empty")
	empty.SetVisible(false)
	listBox.Append(empty)
	rows := make([]*clipboardRowState, 0, clipboardMaxHistory)

	rebuildList := func() {
		entries := history.snapshot()
		if len(entries) == 0 {
			empty.SetVisible(true)
			for _, row := range rows {
				row.reset()
			}
			return
		}
		empty.SetVisible(false)
		for len(rows) < len(entries) {
			row := newClipboardRowState(popover)
			row.button.SetVisible(false)
			listBox.Append(row.button)
			rows = append(rows, row)
		}
		for i, entry := range entries {
			rows[i].update(entry)
			rows[i].button.SetVisible(true)
		}
		for i := len(entries); i < len(rows); i++ {
			rows[i].reset()
		}
	}

	clearBtn.ConnectClicked(func() {
		history.clear()
		rebuildList()
	})

	popup := attachHoverPopover(button, popover, nil, rebuildList)
	popup.SetAfterClose(func() {
		empty.SetVisible(false)
		for _, row := range rows {
			row.button.SetVisible(false)
		}
	})

	startClipboardWatchers(history)

	return button
}

func newClipboardRowState(popover *gtk.Popover) *clipboardRowState {
	row := &clipboardRowState{}
	row.button = gtk.NewButton()
	row.button.SetHasFrame(false)
	row.button.AddCSSClass("clipboard-row")

	row.label = gtk.NewLabel("")
	row.label.SetHAlign(gtk.AlignStart)
	row.label.SetMaxWidthChars(42)

	row.picture = gtk.NewPicture()
	row.picture.SetContentFit(gtk.ContentFitContain)
	row.picture.SetCanShrink(true)
	row.picture.SetSizeRequest(200, 140)

	row.button.ConnectClicked(func() {
		entry := row.entry
		go func() {
			var cmd *exec.Cmd
			var file *os.File
			switch entry.Kind {
			case clipKindText:
				cmd = exec.Command("wl-copy")
				cmd.Stdin = bytes.NewReader([]byte(entry.Text))
			case clipKindImage:
				var err error
				file, err = os.Open(entry.ImgPath)
				if err != nil {
					return
				}
				cmd = exec.Command("wl-copy", "--type", entry.ImgMime)
				cmd.Stdin = file
			}
			_ = cmd.Run()
			if file != nil {
				_ = file.Close()
			}
			ui(func() { popover.Popdown() })
		}()
	})

	return row
}

func (row *clipboardRowState) update(entry clipEntry) {
	if row.hasEntry && sameClipEntry(row.entry, entry) {
		switch entry.Kind {
		case clipKindImage:
			row.button.SetChild(row.picture)
		case clipKindText:
			row.button.SetChild(row.label)
		}
		return
	}

	row.entry = entry
	row.hasEntry = true
	switch entry.Kind {
	case clipKindImage:
		thumb := clipImageThumbnail(entry.ImgPath)
		if thumb != nil {
			row.picture.SetPixbuf(thumb)
		} else {
			row.picture.SetPaintable(nil)
		}
		row.button.SetChild(row.picture)
	case clipKindText:
		preview := strings.ReplaceAll(entry.Text, "\n", " ")
		runes := []rune(preview)
		if len(runes) > 80 {
			preview = string(runes[:80]) + "…"
		}
		row.label.SetLabel(preview)
		row.button.SetChild(row.label)
		row.picture.SetPaintable(nil)
	}
}

func (row *clipboardRowState) reset() {
	row.picture.SetPaintable(nil)
	row.entry = clipEntry{}
	row.hasEntry = false
	row.button.SetVisible(false)
}

func sameClipEntry(left, right clipEntry) bool {
	if left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case clipKindText:
		return left.Text == right.Text
	case clipKindImage:
		return left.ImgMime == right.ImgMime && left.ImgHash == right.ImgHash
	default:
		return false
	}
}
