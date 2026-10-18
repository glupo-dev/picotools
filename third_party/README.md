# third_party

`terminal/` is a copy of github.com/fyne-io/terminal
(version v0.0.0-20261015114605-aafe3cca2cb3, BSD-3-Clause license, see terminal/LICENSE),
used by the Shell menu entry. It is modified for Pico Tools:

- color.go: brighter standard colors, readable on our dark background
- scrollback.go + output.go (scrollDown) + escape.go (escapeScrollUp): lines that leave the top of the screen are kept,
  so the shell can be scrolled back with the mouse wheel / trackpad

Search for "Pico Tools" to find the changes.
- escape.go (escapeScrollUp): CSI S no longer moves the cursor (it caused the prompt to
  appear in the middle of long output on Windows)
- term_windows.go (startPTY): the Windows console is created with the real window size
  instead of 80x25 (the later resize did not reach PowerShell)
