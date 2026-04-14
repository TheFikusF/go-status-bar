<img width="1920" height="45" alt="image" src="https://github.com/user-attachments/assets/332c2b77-f927-4ed1-9dc0-f8d4df284c3d" />

# go-status-bar

> [!WARNING]
> It is written completely with AI and suits specifically MY needs, so it may be partly or completely broken for your particular use-case.

A custom GTK4 layer-shell status bar written in Go.

## Run

Run the bar via one of the commands bellow:

```bash
go run .

./statusbar

# add --css to select specific styles file
./statusbar --css ./styles/frutiger-aero.css
```

## Build

```bash
go build .
```

> This will build the bar, place it into `$HOME/.local/bin/statusbar` and run it:
>
> ```bash
> ./build.sh
> ```

## Configuration

The bar reads config from:

```text
$HOME/.config/status-bar/config.yaml
```

You can override that path with:

```bash
STATUSBAR_CONFIG=/path/to/config.yaml ./statusbar
```

Use [config.example.yaml](./config.example.yaml) as the reference file. Notable config behavior:

- Every module can be enabled or disabled under `modules:`.
- Module-specific settings live in nested maps such as `weather:`, `wallpaper:`, `audio:`, `network:`, `bluetooth:`, `clocks:`, and `calendar:`.
- `on_click` commands are configurable for weather, wallpaper, audio, network, clocks, and calendar.
- `audio.show_text`, `network.show_text`, and `bluetooth.show_text` control whether those modules render text next to the icons.
- `world_clocks` controls the popup list shown by the time module.
- `languages` lets you replace the built-in keymap labels with custom names or glyphs.

## CSS loading

CSS is loaded in this order:

1. `--css /path/to/file.css` if provided
2. `$HOME/.config/status-bar/style.css` if it exists
3. the embedded default from `styles/bar.css`

Included themes currently include:

- `styles/bar.css`
- `styles/blocks.css`
- `styles/frutiger-aero.css`

## Runtime integrations

This project is Linux-only and assumes GTK4 plus `gtk4-layer-shell` are installed. A lot of modules shell out to existing desktop tools. Important ones are:

- `hyprctl` and `hyprpaper` for Hyprland-focused workspace, focused-app, input, and wallpaper behavior
- `nmcli` for the network module
- `pactl` and optionally `wpctl` for audio control
- `rfkill` and `bluetoothctl` for Bluetooth power and device management
- `swaync-client` for the notifications module
- `wl-copy` for clipboard restore/copy actions
- `powerprofilesctl` for the power-profile module
- `cava` for media visualization

If one of those tools is missing, the corresponding module may show degraded output or lose interactive actions.

## Notes

- The bar is currently more Hyprland-oriented than Wayland-generic.
- It spawns a separate window for each detected monitor connector.
- Weather data comes from Open-Meteo.
- The battery widget falls back to AC-style behavior on systems without a battery.
