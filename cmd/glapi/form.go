package main

import (
	"strconv"
	"strings"

	"gltools/internal/gui"
)

// question is one field of a form, asked on its own line.
type question struct {
	label    string
	value    string   // proposed value: enter keeps it, typing replaces it
	secret   bool     // shown as •
	optional bool     // may be left empty
	history  []string // earlier values, the most recent first, reached with ↑/↓
	repeat   bool     // asked again after each answer until an empty one (e.g. headers)
	// skip leaves the question out, looking at the answers given so far.
	skip func(answers []string) bool
	// check says what is wrong with the answer, if anything.
	check func(value string, answers []string) string
}

// form asks its questions one after the other. When the last one is answered done is
// called with all the answers (an empty one for each question left out; the answers of
// a repeated question joined by line breaks). If done returns an error message, the form
// stays open on the last question showing it.
type form struct {
	title   string
	qs      []question
	answers []string
	repeats []string // answers given so far to the current repeated question
	i       int
	input   []rune
	fresh   bool // the input is the proposed value
	hist    int  // position in the history of the current question, -1 = none
	err     string
	done    func(answers []string) string
	cancel  func()
}

func newForm(title string, qs []question, done func([]string) string, cancel func()) *form {
	f := &form{title: title, qs: qs, done: done, cancel: cancel}
	f.start(0)
	return f
}

// start moves to question i, or past the skipped ones; after the last it calls done.
func (f *form) start(i int) {
	for i < len(f.qs) && f.qs[i].skip != nil && f.qs[i].skip(f.answers) {
		f.answers = append(f.answers, "")
		i++
	}
	f.i, f.err, f.hist, f.repeats = i, "", -1, nil
	if i == len(f.qs) {
		if msg := f.done(f.answers); msg != "" {
			// Back to the last question asked, with its answer, to fix it.
			i := len(f.qs) - 1
			for i > 0 && f.qs[i].skip != nil && f.qs[i].skip(f.answers[:i]) {
				i--
			}
			f.input, f.fresh = []rune(f.answers[i]), false
			if f.qs[i].repeat {
				f.input, f.repeats = nil, strings.Split(f.answers[i], "\n")
				if f.answers[i] == "" {
					f.repeats = nil
				}
			}
			f.answers = f.answers[:i]
			f.i, f.err = i, msg
		}
		return
	}
	q := f.qs[i]
	f.input, f.fresh = []rune(q.value), q.value != ""
	if q.value == "" && len(q.history) > 0 {
		f.input, f.fresh, f.hist = []rune(q.history[0]), true, 0
	}
}

func (f *form) finished() bool { return f.i == len(f.qs) }

func (f *form) key(k string) {
	q := f.qs[f.i]
	switch k {
	case gui.KeyEsc:
		f.cancel()
	case gui.KeyBackspace:
		f.err = ""
		if f.fresh {
			f.input, f.fresh = nil, false
		} else if len(f.input) > 0 {
			f.input = f.input[:len(f.input)-1]
		}
	case gui.KeyUp, gui.KeyDown:
		if len(q.history) == 0 {
			return
		}
		step := 1
		if k == gui.KeyDown {
			step = -1
		}
		f.hist = min(max(f.hist+step, -1), len(q.history)-1)
		f.input, f.fresh = nil, false
		if f.hist >= 0 {
			f.input, f.fresh = []rune(q.history[f.hist]), true
		}
	case gui.KeyEnter:
		f.enter()
	}
}

func (f *form) enter() {
	q := f.qs[f.i]
	value := strings.TrimSpace(string(f.input))
	if q.repeat {
		if value == "" {
			f.answers = append(f.answers, strings.Join(f.repeats, "\n"))
			f.start(f.i + 1)
			return
		}
		if q.check != nil {
			if msg := q.check(value, f.answers); msg != "" {
				f.err = msg
				return
			}
		}
		f.repeats = append(f.repeats, value)
		f.input, f.fresh, f.err = nil, false, ""
		return
	}
	if value == "" && !q.optional {
		f.err = "This one cannot be empty"
		return
	}
	if q.check != nil {
		if msg := q.check(value, f.answers); msg != "" {
			f.err = msg
			return
		}
	}
	f.answers = append(f.answers, value)
	f.start(f.i + 1)
}

func (f *form) rune(r rune) {
	f.err = ""
	if f.fresh {
		f.input, f.fresh = nil, false
	}
	f.input = append(f.input, r)
}

func (f *form) paste(text string) {
	f.err = ""
	if f.fresh {
		f.input, f.fresh = nil, false
	}
	f.input = append(f.input, []rune(strings.TrimRight(text, "\r\n"))...)
}

// render draws the questions answered so far, the current one and its input line.
func (f *form) render(cols int) []gui.Line {
	lines := []gui.Line{{gui.Strong(f.title, nil)}, nil}
	answered := 0
	for i := 0; i < f.i; i++ {
		if f.qs[i].skip != nil && f.answers[i] == "" && f.qs[i].skip(f.answers[:i]) {
			continue
		}
		answered++
		lines = append(lines, f.answerLines(f.qs[i], f.answers[i], cols)...)
	}
	if answered > 0 {
		lines = append(lines, nil)
	}
	if f.finished() {
		return lines
	}
	q := f.qs[f.i]
	label := q.label
	if q.optional {
		label += " (optional)"
	}
	lines = append(lines, gui.Line{gui.Plain(label + ":")})
	for _, r := range f.repeats {
		lines = append(lines, gui.Line{gui.Colored("  "+gui.Truncate(r, cols-4), gui.Muted)})
	}

	text := string(f.input)
	if q.secret {
		text = strings.Repeat("•", len(f.input))
	}
	text = gui.DisplayText(text)
	if room := max(cols-10, 10); len([]rune(text)) > room {
		r := []rune(text)
		text = "…" + string(r[len(r)-room+1:])
	}
	shown := gui.Plain(text)
	if f.fresh {
		shown = gui.Colored(text, gui.ValueFg)
	}
	lines = append(lines, gui.Line{gui.Strong("› ", gui.Accent), shown, gui.Colored("█", gui.Accent)})
	lines = append(lines, nil)
	if f.err != "" {
		lines = append(lines, gui.Line{gui.Strong("✘ "+f.err, gui.Red)}, nil)
	}
	hint := "enter confirm · esc cancel · paste with ctrl+v or right click"
	if q.repeat {
		hint = "enter add · enter on empty to finish · esc cancel"
	}
	if f.fresh {
		hint = "enter keep · type to replace · backspace clear · esc cancel"
	}
	if len(q.history) > 0 {
		hint = "↑/↓ earlier values (" + strconv.Itoa(len(q.history)) + ") · " + hint
	}
	return append(lines, gui.Line{gui.Colored(hint, gui.Muted)})
}

func (f *form) answerLines(q question, answer string, cols int) []gui.Line {
	if q.repeat {
		if answer == "" {
			return []gui.Line{{gui.Colored(q.label+": none", gui.Muted)}}
		}
		lines := []gui.Line{{gui.Colored(q.label+":", gui.Muted)}}
		for _, a := range strings.Split(answer, "\n") {
			lines = append(lines, gui.Line{gui.Colored("  "+gui.Truncate(a, cols-4), gui.Muted)})
		}
		return lines
	}
	shown := gui.DisplayText(answer)
	if q.secret && answer != "" {
		shown = "••••••••"
	}
	if answer == "" {
		shown = "—"
	}
	return []gui.Line{{gui.Colored(gui.Truncate(q.label+": "+shown, cols-2), gui.Muted)}}
}
