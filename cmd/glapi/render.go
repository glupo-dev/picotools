package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"gltools/internal/gui"
)

type line = gui.Line

func (m *model) Render(cols, rows int) []line {
	m.cols, m.rows = cols, rows
	lines := []line{
		{gui.Strong("╭────────────────╮", gui.Accent)},
		{gui.Strong("│     GL API     │", gui.Accent)},
		{gui.Strong("╰────────────────╯", gui.Accent)},
		nil,
	}
	switch m.screen {
	case screenProviders:
		lines = append(lines, m.renderProviders()...)
	case screenEndpoints:
		lines = append(lines, m.renderEndpoints()...)
	case screenProviderEdit:
		lines = append(lines, m.renderProviderEdit()...)
	case screenHeaders:
		lines = append(lines, m.renderHeaders()...)
	case screenEndpointEdit:
		lines = append(lines, m.renderEndpointEdit()...)
	case screenForm:
		lines = append(lines, m.form.render(cols)...)
	case screenConfirm:
		lines = append(lines, line{gui.Strong(m.confirm, gui.Amber)}, nil, line{gui.Colored("y yes · any other key no", gui.Muted)})
	case screenBusy:
		lines = append(lines, m.renderBusy()...)
	case screenResponse:
		lines = append(lines, m.renderResponse()...)
	case screenCurl:
		lines = append(lines, m.renderCurl()...)
	}
	return lines
}

// listRoom is how many entries of a list fit: the window less the title (4 lines),
// top margin, the list header, the "new" entry, the status and the hint.
func (m *model) listRoom() int {
	return max(m.rows-11, 1)
}

// entries draws a list with the selected one marked, scrolled to keep it in view.
// Each entry is its label (padded to the same width) and a detail in grey.
func (m *model) entries(labels, details []string, cursor int) []line {
	width := 0
	for _, l := range labels {
		width = max(width, len([]rune(l)))
	}
	room := m.listRoom()
	start := min(max(cursor-room/2, 0), max(len(labels)-room, 0))
	end := min(start+room, len(labels))
	numWidth := len(fmt.Sprint(len(labels)))
	var lines []line
	if start > 0 {
		lines = append(lines, line{gui.Colored(fmt.Sprintf("  ↑ %d more", start), gui.Muted)})
	}
	for i := start; i < end; i++ {
		num := fmt.Sprintf("%*d", numWidth, i+1)
		label := fmt.Sprintf("%-*s", width, labels[i])
		detail := gui.Truncate(details[i], m.cols-width-numWidth-10)
		if i == cursor {
			lines = append(lines, line{gui.Strong("› "+num+"  "+label, gui.Accent), gui.Colored("   "+detail, gui.Muted)})
		} else {
			lines = append(lines, line{gui.Plain("  "), gui.Strong(num, gui.Accent), gui.Plain("  " + label), gui.Colored("   "+detail, gui.Muted)})
		}
	}
	if end < len(labels) {
		lines = append(lines, line{gui.Colored(fmt.Sprintf("  ↓ %d more", len(labels)-end), gui.Muted)})
	}
	return lines
}

// footer is the status, if any, and the hint.
func (m *model) footer(hint string) []line {
	lines := []line{nil}
	if m.status != nil {
		lines = append(lines, m.status, nil)
	}
	return append(lines, line{gui.Colored(hint, gui.Muted)})
}

func (m *model) renderProviders() []line {
	ps := m.store.Providers
	labels := make([]string, len(ps))
	details := make([]string, len(ps))
	for i, p := range ps {
		labels[i] = p.Name
		key := "key"
		if p.KeyIn == keyNone {
			key = "no key"
		}
		details[i] = fmt.Sprintf("%s · %s · %s", host(p.URL), key, count(len(p.Endpoints), "endpoint"))
	}
	var lines []line
	if len(ps) == 0 {
		lines = append(lines, line{gui.Colored("No providers yet: press n to add the first one.", gui.Muted)})
	}
	lines = append(lines, m.entries(labels, details, m.provCursor)...)
	lines = append(lines, nil, line{gui.Plain("  "), gui.Strong("n", gui.Accent), gui.Plain("  new provider")})
	hint := "n new · esc quit"
	if len(ps) > 0 {
		hint = "↑/↓ move · enter open · number quick pick · n new · m edit · d delete · " + gui.FullScreenKey + " full screen · esc quit"
	}
	return append(lines, m.footer(hint)...)
}

func (m *model) renderEndpoints() []line {
	es := m.prov.Endpoints
	lines := []line{{gui.Strong(m.prov.Name, nil), gui.Colored("   "+m.prov.URL, gui.Muted)}, nil}
	if len(es) == 0 {
		lines = append(lines, line{gui.Colored("No endpoints yet: press n to add the first one.", gui.Muted)})
	}
	labels := make([]string, len(es))
	details := make([]string, len(es))
	for i, e := range es {
		labels[i] = e.Name
		details[i] = e.Request // cut with … when longer than the window
	}
	lines = append(lines, m.entries(labels, details, m.epCursor)...)
	lines = append(lines, nil, line{gui.Plain("  "), gui.Strong("n", gui.Accent), gui.Plain("  new endpoint")})
	hint := "n new · esc back"
	if len(es) > 0 {
		hint = "↑/↓ move · enter run · number quick pick · n new · m edit · d delete · esc back"
	}
	return append(lines, m.footer(hint)...)
}

func (m *model) renderProviderEdit() []line {
	p := m.prov
	key := "none"
	switch p.KeyIn {
	case keyHeader:
		key = "header " + p.KeyName
	case keyBearer:
		key = "Authorization: Bearer"
	case keyQuery:
		key = "query parameter " + p.KeyName
	}
	if p.KeyIn != keyNone {
		if k, err := getKey(p.Name); err != nil || k == "" {
			key += " · no key saved"
		} else {
			key += " · ••••••••"
		}
	}
	var headers []string
	for _, h := range p.Headers {
		headers = append(headers, h.Name+": "+h.Value)
	}
	hs := "none"
	if len(headers) > 0 {
		hs = strings.Join(headers, " · ")
	}
	lines := []line{{gui.Strong(p.Name+" › edit", nil)}, nil}
	lines = append(lines, m.entries(
		[]string{"Name", "Base URL", "Key", "Headers ›"},
		[]string{p.Name, p.URL, key, hs}, m.fieldCursor)...)
	return append(lines, m.footer("↑/↓ move · enter change · number quick pick · esc back")...)
}

func (m *model) renderHeaders() []line {
	p := m.prov
	lines := []line{{gui.Strong(p.Name+" › headers", nil), gui.Colored("   sent with every request", gui.Muted)}, nil}
	if len(p.Headers) == 0 {
		lines = append(lines, line{gui.Colored("No fixed headers: press n to add one.", gui.Muted)})
	}
	labels := make([]string, len(p.Headers))
	for i, h := range p.Headers {
		labels[i] = h.Name + ": " + h.Value
	}
	lines = append(lines, m.entries(labels, make([]string, len(labels)), m.headerCursor)...)
	lines = append(lines, nil, line{gui.Plain("  "), gui.Strong("n", gui.Accent), gui.Plain("  new header")})
	hint := "n new · esc back"
	if len(p.Headers) > 0 {
		hint = "↑/↓ move · enter change · n new · d delete · esc back"
	}
	return append(lines, m.footer(hint)...)
}

func (m *model) renderEndpointEdit() []line {
	e := m.ep
	body := "none"
	if e.Body != "" {
		body = gui.DisplayText(e.Body)
	}
	lines := []line{{gui.Strong(m.prov.Name+" › "+e.Name+" › edit", nil)}, nil}
	lines = append(lines, m.entries([]string{"Name", "Request", "Body"}, []string{e.Name, e.Request, body}, m.fieldCursor)...)
	return append(lines, m.footer("↑/↓ move · enter change · number quick pick · esc back")...)
}

// requestHeader is the provider and endpoint names over the request sent.
func (m *model) requestHeader() []line {
	return []line{
		{gui.Strong(m.prov.Name+" › "+m.ep.Name, nil)},
		{gui.Colored(gui.Truncate(m.reqLine, m.cols-2), gui.Muted)},
		nil,
	}
}

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m *model) renderBusy() []line {
	waited := m.now().Sub(m.started)
	frame := spinner[int(waited/(250*time.Millisecond))%len(spinner)]
	return append(m.requestHeader(),
		line{gui.Strong(fmt.Sprintf("%s Waiting for the response… %ds", frame, int(waited.Seconds())), gui.Accent)},
		nil,
		line{gui.Colored("esc cancel", gui.Muted)},
	)
}

// viewRoom is how many rows of the body fit: the window less the title (4 lines), top margin,
// the request header (3), the status line and the blank under it, and the footer.
func (m *model) viewRoom() int {
	room := m.rows - 12
	if m.status != nil {
		room -= 2
	}
	return max(room, 1)
}

func (m *model) renderResponse() []line {
	lines := m.requestHeader()
	if m.err != nil {
		lines = append(lines, line{gui.Strong("✘ "+capitalize(m.err.Error()), gui.Red)})
		return append(lines, m.footer("u copy as curl · esc back")...)
	}
	r := m.resp
	color := gui.Accent
	switch {
	case r.code >= 400:
		color = gui.Red
	case r.code >= 300:
		color = gui.Amber
	}
	ms := r.elapsed.Milliseconds()
	info := fmt.Sprintf("%d ms · %s", ms, size(len(r.body)))
	if m.showHeaders {
		info = fmt.Sprintf("%d response headers", len(r.header))
	}
	lines = append(lines, line{gui.Strong(r.status, color), gui.Colored(" · "+info, gui.Muted)}, nil)

	if !r.isText && !m.showHeaders {
		lines = append(lines, line{gui.Colored("The body is not text ("+r.header.Get("Content-Type")+"): save it with s to open it.", gui.Muted)})
		return append(lines, m.footer("h headers · s save · u copy as curl · esc back")...)
	}
	rows, where := m.viewer.Render(m.cols, m.viewRoom())
	lines = append(lines, rows...)
	hint := "↑/↓ scroll · pgup/pgdn page · h headers · c copy · s save · u copy as curl · esc back · " + where
	if m.showHeaders {
		hint = "↑/↓ scroll · h body · c copy the headers · s save the body · esc back · " + where
	}
	return append(lines, m.footer(hint)...)
}

func (m *model) renderCurl() []line {
	labels := make([]string, len(curlShells))
	details := make([]string, len(curlShells))
	for i, sh := range curlShells {
		labels[i] = sh.name
		details[i] = m.call.curl(sh)
	}
	lines := []line{
		{gui.Strong(m.prov.Name+" › "+m.ep.Name+" › copy as curl", nil)},
		{gui.Colored("The key is left out: whoever runs the command puts it in place of "+keyPlaceholder+".", gui.Muted)},
		nil,
	}
	lines = append(lines, m.entries(labels, details, m.curlCursor)...)
	return append(lines, m.footer("↑/↓ move · enter copy · number quick copy · esc back")...)
}

func host(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Host
}

func count(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}
