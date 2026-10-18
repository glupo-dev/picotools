package terminal

import (
	"image/color"
	"log"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

var (
	// Pico Tools: brighter standard colors, readable on a dark background.
	basicColors = []color.Color{
		&color.RGBA{0x5C, 0x63, 0x70, 255},
		&color.RGBA{0xE0, 0x6C, 0x75, 255},
		&color.RGBA{0x98, 0xC3, 0x79, 255},
		&color.RGBA{0xE5, 0xC0, 0x7B, 255},
		&color.RGBA{0x61, 0xAF, 0xEF, 255},
		&color.RGBA{0xC6, 0x78, 0xDD, 255},
		&color.RGBA{0x56, 0xB6, 0xC2, 255},
		&color.RGBA{0xAB, 0xB2, 0xBF, 255},
	}
	brightColors = []color.Color{
		&color.RGBA{0x7F, 0x84, 0x8E, 255},
		&color.RGBA{0xFF, 0x7A, 0x85, 255},
		&color.RGBA{0xB5, 0xE8, 0x90, 255},
		&color.RGBA{0xF0, 0xD1, 0x97, 255},
		&color.RGBA{0x8C, 0xC8, 0xFF, 255},
		&color.RGBA{0xDD, 0xA0, 0xF0, 255},
		&color.RGBA{0x7F, 0xD3, 0xDD, 255},
		&color.RGBA{0xFF, 0xFF, 0xFF, 255},
	}
	colourBands = []uint8{
		0x00,
		0x5f,
		0x87,
		0xaf,
		0xd7,
		0xff,
	}
)

func (t *Terminal) handleColorEscape(message string) {
	if message == "" || message == "0" {
		t.currentBG = nil
		t.currentFG = nil
		t.bold = false
		t.italic = false
		t.underline = false
		t.strikethrough = false
		t.blinking = false
		return
	}
	if message[0] == '>' || message[0] == '?' {
		if t.debug {
			log.Println("Strange colour mode", message)
		}
		return
	}
	modes := strings.Split(message, ";")
	for i := 0; i < len(modes); i++ {
		mode := modes[i]
		if mode == "" {
			continue
		}

		if (mode == "38" || mode == "48") && i+1 < len(modes) {
			nextMode := modes[i+1]
			if nextMode == "5" && i+2 < len(modes) {
				t.handleColorModeMap(mode, modes[i+2])
				i += 2
			} else if nextMode == "2" && i+4 < len(modes) {
				t.handleColorModeRGB(mode, modes[i+2], modes[i+3], modes[i+4])
				i += 4
			}
		} else {
			t.handleColorMode(mode)
		}
	}
}

func (t *Terminal) handleColorMode(modeStr string) {
	mode, err := strconv.Atoi(modeStr)
	if err != nil {
		fyne.LogError("Failed to parse color mode: "+modeStr, err)
		return
	}
	switch mode {
	case 0:
		t.currentBG, t.currentFG = nil, nil
		t.bold = false
		t.blinking = false
		t.italic = false
		t.underline = false
		t.strikethrough = false
	case 1:
		t.bold = true
	case 3:
		t.italic = true
	case 4:
		t.underline = true
	case 5:
		t.blinking = true
	case 7: // reverse
		bg, fg := t.currentBG, t.currentFG
		if fg == nil {
			t.currentBG = theme.Color(theme.ColorNameForeground)
		} else {
			t.currentBG = fg
		}
		if bg == nil {
			t.currentFG = theme.Color(theme.ColorNameDisabledButton)
		} else {
			t.currentFG = bg
		}
	case 9:
		t.strikethrough = true
	case 22:
		t.bold = false
	case 23:
		t.italic = false
	case 24:
		t.underline = false
	case 27: // reverse off
		bg, fg := t.currentBG, t.currentFG
		if fg != nil {
			t.currentBG = nil
		} else {
			t.currentBG = fg
		}
		if bg != nil {
			t.currentFG = nil
		} else {
			t.currentFG = bg
		}
	case 30, 31, 32, 33, 34, 35, 36, 37:
		t.currentFG = basicColors[mode-30]
	case 39:
		t.currentFG = nil
	case 40, 41, 42, 43, 44, 45, 46, 47:
		t.currentBG = basicColors[mode-40]
	case 49:
		t.currentBG = nil
	case 90, 91, 92, 93, 94, 95, 96, 97:
		t.currentFG = brightColors[mode-90]
	case 100, 101, 102, 103, 104, 105, 106, 107:
		t.currentBG = brightColors[mode-100]
	default:
		if t.debug {
			log.Println("Unsupported graphics mode", mode)
		}
	}
}

func (t *Terminal) handleColorModeMap(mode, ids string) {
	var c color.Color
	id, err := strconv.Atoi(ids)
	if err != nil {
		if t.debug {
			log.Println("Invalid color map ID", ids)
		}
		return
	}
	if id <= 7 {
		c = basicColors[id]
	} else if id <= 15 {
		c = brightColors[id-8]
	} else if id <= 231 {
		id -= 16
		b := id % 6
		id = (id - b) / 6
		g := id % 6
		r := (id - g) / 6
		c = &color.RGBA{colourBands[r], colourBands[g], colourBands[b], 255}
	} else if id <= 255 {
		id -= 232
		inc := 256 / 24
		y := id * inc
		c = &color.Gray{uint8(y)}
	} else if t.debug {
		log.Println("Invalid colour map ID", id)
	}

	switch mode {
	case "38":
		t.currentFG = c
	case "48":
		t.currentBG = c
	}
}

func (t *Terminal) handleColorModeRGB(mode, rs, gs, bs string) {
	r, _ := strconv.Atoi(rs)
	g, _ := strconv.Atoi(gs)
	b, _ := strconv.Atoi(bs)
	c := &color.RGBA{uint8(r), uint8(g), uint8(b), 255}

	switch mode {
	case "38":
		t.currentFG = c
	case "48":
		t.currentBG = c
	}
}
