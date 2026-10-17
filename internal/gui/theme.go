// Package gui holds what the GL apps share: the dark console look, the text area
// with mouse selection and the window that opens maximized.
package gui

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/theme"
)

const TextSize = 13

// Window size in characters when not maximized.
const (
	DefaultCols = 150
	DefaultRows = 20
)

// Smallest window size in characters.
const (
	MinCols = 60
	MinRows = 14
)

var Background = color.NRGBA{0x17, 0x1B, 0x1A, 0xFF}

// Colors shared by the apps.
var (
	Accent  = color.NRGBA{0x5D, 0xCA, 0xA5, 0xFF}
	Amber   = color.NRGBA{0xE3, 0xB3, 0x41, 0xFF}
	Red     = color.NRGBA{0xF8, 0x51, 0x49, 0xFF}
	Muted   = color.NRGBA{0x86, 0x91, 0x8D, 0xFF}
	ValueFg = color.NRGBA{0x79, 0xC0, 0xFF, 0xFF}
)

// ConsoleTheme is a dark theme with our background color and text size.
type ConsoleTheme struct{ fyne.Theme }

func (t ConsoleTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	if n == theme.ColorNameBackground {
		return Background
	}
	return t.Theme.Color(n, theme.VariantDark)
}

func (t ConsoleTheme) Size(n fyne.ThemeSizeName) float32 {
	if n == theme.SizeNameText {
		return TextSize
	}
	return t.Theme.Size(n)
}

func CellSize() fyne.Size {
	return fyne.MeasureText("M", TextSize, fyne.TextStyle{Monospace: true})
}

// NewWindow creates the app and its window with the console theme and no outer margin:
// each screen sets its own, so its background reaches the edges.
func NewWindow(id, title string) (fyne.App, fyne.Window) {
	a := app.NewWithID(id)
	a.Settings().SetTheme(ConsoleTheme{theme.DefaultTheme()})
	w := a.NewWindow(title)
	w.SetPadded(false)
	return a, w
}

// Run shows the window maximized and runs the app until it is closed.
// started, if given, is called once the window is on screen.
func Run(a fyne.App, w fyne.Window, started ...func()) {
	// Size used if the window cannot be maximized.
	cell := CellSize()
	pad := theme.Padding() * 2
	w.Resize(fyne.NewSize(cell.Width*DefaultCols+pad, cell.Height*DefaultRows+pad))
	w.CenterOnScreen()
	a.Lifecycle().SetOnStarted(func() {
		maximize(w, 20)
		for _, f := range started {
			f()
		}
	})
	w.ShowAndRun()
}

// maximize enlarges the window to fill the screen, leaving the menu bar, Dock
// and taskbar visible. The native window may not exist yet right after start,
// so it retries a few times.
func maximize(w fyne.Window, tries int) {
	nw, ok := w.(driver.NativeWindow)
	if !ok {
		return
	}
	done := false
	nw.RunNative(func(ctx any) { done = maximizeNative(ctx) })
	if !done && tries > 0 {
		time.AfterFunc(50*time.Millisecond, func() { fyne.Do(func() { maximize(w, tries-1) }) })
	}
}
