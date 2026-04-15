pkill statusbar
go build .
cp statusbar $HOME/.local/bin/statusbar
hyprctl dispatch exec $(which statusbar)