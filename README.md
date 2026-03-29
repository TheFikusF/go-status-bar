<img width="1920" height="46" alt="image" src="https://github.com/user-attachments/assets/ff2cdee6-297f-4c94-af7e-c2cd32bd7f5e" />

# go-status-bar

> [!WARNING]
> It is written completely with AI and suits specifically MY needs, so use it at your own risk.

A small custom status bar written in Go with GTK4.

## Features

- Waybar-like top bar window using `gtk4-layer-shell`
- GTK CSS styling with left, center, and right module groups
- Live modules for session, host, clock, CPU, memory, and battery
- Linux-friendly system sampling from `/proc` and `/sys`

## Run

```bash
go run .
```

By default the bar uses the CSS embedded from `styles/bar.css`.
If present, it will override that with:

```bash
$HOME/.config/status-bar/style.css
```

## Build

```bash
go build .
```

## Notes

- This version targets Linux desktops with GTK4 and `gtk4-layer-shell` installed.
- The battery module falls back to `AC` on systems without a battery.
- Modules are intentionally simple so you can swap them out for workspace, media, network, or WM-specific integrations.
