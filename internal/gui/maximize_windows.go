package gui

import (
	"syscall"

	"fyne.io/fyne/v2/driver"
)

var showWindow = syscall.NewLazyDLL("user32.dll").NewProc("ShowWindow")

const swMaximize = 3

func maximizeNative(ctx any) bool {
	c, ok := ctx.(driver.WindowsWindowContext)
	if !ok || c.HWND == 0 {
		return false
	}
	showWindow.Call(c.HWND, swMaximize)
	return true
}
