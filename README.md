<img width="1920" height="46" alt="image" src="https://github.com/user-attachments/assets/ff2cdee6-297f-4c94-af7e-c2cd32bd7f5e" />

# go-status-bar

> [!WARNING]
> It is written completely with AI and suits specifically MY needs, so it may be partly or completely broken for your particular use-case.

A custom GTK4 layer-shell status bar written in Go.

## What it does

- Spawns one bar per connected monitor.
- Uses GTK CSS themes, with the default style embedded from `styles/bar.css`.
- Organizes modules into left, center, and right groups.
- Mixes simple system widgets with interactive popups for audio, Wi‑Fi, Bluetooth, weather, wallpaper, clipboard, power, and more.
- Favors direct shell integration over big framework dependencies.

## Current modules

The bar can enable or disable modules through config. The current set is:

- workspaces
- focused app
- music
- mode
- scratchpad
- date clock
- time clock
- notifications
- mpd
- wallpaper
- clipboard
- weather
- pipewire / audio
- network
- bluetooth
- power profile
- cpu
- memory
- temperature
- keyboard state
- language
- battery
- tray
- power menu

Some modules are passive status indicators, but several are interactive:

- `time_clock` opens a world-clock popup.
- `date_clock` opens a calendar popup.
- `weather` shows a 7-day forecast popup.
- `pipewire` shows output/input devices and sliders.
- `network` shows a Wi‑Fi list and networking toggle.
- `bluetooth` shows discovered devices, power state, and connect/disconnect actions.
- `wallpaper` can shuffle wallpapers and manage auto-switching.
- `clipboard`, `language`, `tray`, and `power` all have their own popups.

## Run

Run the bar with the embedded default CSS:

```bash
go run .
```

or run the built binary:

```bash
./statusbar
```

To load a specific CSS file, use the `--css` flag:

```bash
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

## Hyprland-specific behavior

The focused-app module can temporarily show workspace information while the Super key is held, and several left-side modules depend on `hyprctl` data being available.
