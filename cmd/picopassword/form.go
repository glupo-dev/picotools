package main

import (
	"strings"

	"picotools/internal/gui"
)

// question is one field of a form, asked on its own line.
type question struct {
	label  string
	value  string // the current value, already written and editable
	secret bool   // shown as one dot per character; tab shows it
	// same, when not 0, is the number (from 1) of an earlier question this answer must
	// equal; if it does not, the form goes back to that question, empty.
	same int
	// skip leaves the question out, looking at the answers given so far.
	skip func(answers []string) bool
	// check says what is wrong with the answer, if anything.
	check func(value string, answers []string) string
}

// form asks its questions one after the other, like the forms of Pico Auth, but the value is
// edited as in a text field: the cursor starts at the end, ←/→ move it, typing inserts,
// backspace deletes one character. When the last one is answered done is called with all
// the answers; if it returns an error message, the form stays open on the last question.
type form struct {
	title   string
	qs      []question
	answers []string
	i       int
	input   []rune
	pos     int  // the cursor, in input
	show    bool // secret answers shown in clear (tab)
	err     string
	done    func(answers []string) string
	cancel  func()
}

func newForm(title string, qs []question, done func([]string) string, cancel func()) *form {
	f := &form{title: title, qs: qs, done: done, cancel: cancel}
	f.start(0, "")
	return f
}

// start moves to question i with an optional error message; after the last it calls done.
func (f *form) start(i int, msg string) {
	for i < len(f.qs) && f.skipped(i, f.answers) {
		f.answers = append(f.answers, "")
		i++
	}
	f.i, f.err = i, msg
	if i == len(f.qs) {
		if msg := f.done(f.answers); msg != "" {
			i := len(f.qs) - 1
			for i > 0 && f.skipped(i, f.answers[:i]) {
				i--
			}
			f.input = []rune(f.answers[i])
			f.pos = len(f.input)
			f.answers = f.answers[:i]
			f.i, f.err = i, msg
		}
		return
	}
	f.input = []rune(f.qs[i].value)
	if msg != "" && f.qs[i].secret {
		f.input = nil // a password asked again starts empty
	}
	f.pos = len(f.input)
}

func (f *form) finished() bool { return f.i == len(f.qs) }

// skipped says whether question i is left out, given the answers before it.
func (f *form) skipped(i int, answers []string) bool {
	return f.qs[i].skip != nil && f.qs[i].skip(answers)
}

func (f *form) key(k string) {
	switch k {
	case gui.KeyEsc:
		f.cancel()
	case gui.KeyBackspace:
		f.err = ""
		if f.pos > 0 {
			f.input = append(f.input[:f.pos-1], f.input[f.pos:]...)
			f.pos--
		}
	case gui.KeyLeft:
		f.pos = max(f.pos-1, 0)
	case gui.KeyRight:
		f.pos = min(f.pos+1, len(f.input))
	case gui.KeyTab:
		if f.qs[f.i].secret {
			f.show = !f.show
		}
	case gui.KeyEnter:
		f.enter()
	}
}

func (f *form) enter() {
	q := f.qs[f.i]
	value := string(f.input)
	if !q.secret {
		value = strings.TrimSpace(value)
	}
	if value == "" {
		f.err = "This one cannot be empty"
		return
	}
	if q.same > 0 && value != f.answers[q.same-1] {
		f.answers = f.answers[:q.same-1]
		f.start(q.same-1, "The two passwords are different")
		return
	}
	if q.check != nil {
		if msg := q.check(value, f.answers); msg != "" {
			f.err = msg
			return
		}
	}
	f.answers = append(f.answers, value)
	f.start(f.i+1, "")
}

func (f *form) rune(r rune) { f.insert([]rune{r}) }

func (f *form) paste(text string) {
	f.insert([]rune(strings.TrimRight(text, "\r\n")))
}

func (f *form) insert(rs []rune) {
	f.err = ""
	f.input = append(f.input[:f.pos], append(rs, f.input[f.pos:]...)...)
	f.pos += len(rs)
}

// render draws the questions answered so far, the current one and its input line.
func (f *form) render(cols int) []gui.Line {
	lines := []gui.Line{{gui.Strong(f.title, nil)}, nil}
	for i := 0; i < f.i; i++ {
		if f.skipped(i, f.answers[:i]) {
			continue
		}
		lines = append(lines, f.answerLine(f.qs[i], f.answers[i], cols))
	}
	if f.i > 0 {
		lines = append(lines, nil)
	}
	if f.finished() {
		return lines
	}
	q := f.qs[f.i]
	lines = append(lines, gui.Line{gui.Plain(q.label + ":")})

	shown := f.input
	if q.secret && !f.show {
		shown = []rune(strings.Repeat("•", len(f.input)))
	}
	shown = []rune(gui.DisplayText(string(shown)))
	pos := min(f.pos, len(shown))
	// Keep the cursor in view on a long value.
	room, first := max(cols-6, 10), 0
	if pos >= room {
		first = pos - room + 1
	}
	last := min(first+room, len(shown))
	under := " "
	if pos < len(shown) {
		under = string(shown[pos])
	}
	input := gui.Line{
		gui.Strong("› ", gui.Accent),
		gui.Plain(string(shown[first:pos])),
		{Text: under, Color: gui.Background, Bg: gui.Accent},
	}
	if pos+1 < last {
		input = append(input, gui.Plain(string(shown[pos+1:last])))
	}
	lines = append(lines, input, nil)
	if f.err != "" {
		lines = append(lines, gui.Line{gui.Strong("✘ "+f.err, gui.Red)}, nil)
	}
	hint := "enter confirm · esc cancel"
	if q.secret {
		hint = "enter confirm · tab show · esc cancel"
		if f.show {
			hint = "enter confirm · tab hide · esc cancel"
		}
	}
	return append(lines, gui.Line{gui.Colored(hint, gui.Muted)})
}

func (f *form) answerLine(q question, answer string, cols int) gui.Line {
	shown := gui.DisplayText(answer)
	if q.secret {
		shown = strings.Repeat("•", len([]rune(answer)))
	}
	return gui.Line{gui.Colored("  "+gui.Truncate(padRight(q.label, 10)+" "+shown, cols-2), gui.Muted)}
}

func padRight(s string, n int) string {
	if r := len([]rune(s)); r < n {
		return s + strings.Repeat(" ", n-r)
	}
	return s
}
