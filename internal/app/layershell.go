package app

/*
#cgo pkg-config: gtk4 gtk4-layer-shell-0
#include <gtk/gtk.h>
#include <gtk4-layer-shell.h>

static void statusbar_init_layer_shell(GtkWindow* win) {
	gtk_layer_init_for_window(win);
	gtk_layer_set_namespace(win, "statusbar");
	gtk_layer_set_layer(win, GTK_LAYER_SHELL_LAYER_TOP);
	gtk_layer_set_anchor(win, GTK_LAYER_SHELL_EDGE_TOP, TRUE);
	gtk_layer_set_anchor(win, GTK_LAYER_SHELL_EDGE_LEFT, TRUE);
	gtk_layer_set_anchor(win, GTK_LAYER_SHELL_EDGE_RIGHT, TRUE);
	gtk_layer_auto_exclusive_zone_enable(win);
	gtk_layer_set_keyboard_mode(win, GTK_LAYER_SHELL_KEYBOARD_MODE_NONE);
}
*/
import "C"

import "unsafe"

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

func initLayerShell(window *gtk.ApplicationWindow) {
	C.statusbar_init_layer_shell((*C.GtkWindow)(unsafe.Pointer(window.Native())))
}
