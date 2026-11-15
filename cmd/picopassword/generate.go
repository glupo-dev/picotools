package main

import (
	"crypto/rand"
	"math/big"
	"strings"
)

// Suggested passwords: ↓ in the Password field of New and Edit writes a strong one, ↑ a
// stronger one. How they are made is chosen in Settings › Suggested passwords, on this device.

// genSpec says how a suggested password is made. Letters, upper and lower case, are always in.
type genSpec struct {
	Length  int  `json:"length"`
	Digits  bool `json:"digits"`
	Symbols bool `json:"symbols"`
}

const (
	lowers      = "abcdefghijklmnopqrstuvwxyz"
	uppers      = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits      = "0123456789"
	symbols     = "!@#$%&*?-_=+"
	minGenLen   = 8
	maxGenLen   = 64
	strongLen   = 20
	strongerLen = 30
)

var (
	defaultStrong   = genSpec{Length: strongLen, Digits: true, Symbols: false}
	defaultStronger = genSpec{Length: strongerLen, Digits: true, Symbols: true}
)

// generate makes a random password by spec, with at least one character of every kind it uses.
func generate(s genSpec) string {
	kinds := []string{lowers, uppers}
	if s.Digits {
		kinds = append(kinds, digits)
	}
	if s.Symbols {
		kinds = append(kinds, symbols)
	}
	all := []rune(strings.Join(kinds, ""))
	n := min(max(s.Length, minGenLen), maxGenLen)
	for {
		out := make([]rune, n)
		for i := range out {
			out[i] = all[randInt(len(all))]
		}
		ok := true
		for _, k := range kinds {
			if !strings.ContainsAny(string(out), k) {
				ok = false
			}
		}
		if ok {
			return string(out)
		}
	}
}

func randInt(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(err) // crypto/rand does not fail on the systems Pico Password runs on
	}
	return int(v.Int64())
}

// suggest is a new suggested password, strong or stronger, by the choices of this device.
func (m *model) suggest(stronger bool) string {
	return generate(m.genSpec(stronger))
}

func (m *model) genSpec(stronger bool) genSpec {
	if stronger {
		if m.prefs.Stronger != nil {
			return *m.prefs.Stronger
		}
		return defaultStronger
	}
	if m.prefs.Strong != nil {
		return *m.prefs.Strong
	}
	return defaultStrong
}
