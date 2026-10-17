package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/PocketAlert/cli/internal/api"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func boxWidth(width int) int { return min(78, width-4) }

func keyHints(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, stKey.Render(pairs[i])+" "+stDim.Render(pairs[i+1]))
	}
	return strings.Join(parts, stDim.Render(" · "))
}

// ---- confirm ----

type uiConfirm struct {
	text string
	yes  func() tea.Cmd
}

func newConfirm(text string, yes func() tea.Cmd) *uiConfirm { return &uiConfirm{text: text, yes: yes} }

func (c *uiConfirm) update(_ *uiModel, msg tea.Msg) (bool, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return false, nil
	}
	switch strings.ToLower(key.String()) {
	case "y", "enter":
		return true, c.yes()
	case "n", "esc", "q", "ctrl+c":
		return true, nil
	}
	return false, nil
}

func (c *uiConfirm) view(_ *uiModel, width, _ int) string {
	w := boxWidth(width)
	return stBox.Width(w).Render(lipgloss.NewStyle().Width(w-6).Render(stBold.Render(c.text)) + "\n\n" + keyHints("y", "yes", "n", "no"))
}

// ---- single-line prompt ----

type uiPrompt struct {
	label  string
	input  textinput.Model
	submit func(string) tea.Cmd
}

func newPrompt(label, value string, submit func(string) tea.Cmd) *uiPrompt {
	in := textinput.New()
	in.SetValue(value)
	in.CharLimit = 100
	in.Focus()
	return &uiPrompt{label: label, input: in, submit: submit}
}

func (p *uiPrompt) update(_ *uiModel, msg tea.Msg) (bool, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "ctrl+c":
			return true, nil
		case "enter":
			v := strings.TrimSpace(p.input.Value())
			if v == "" {
				return false, nil
			}
			return true, p.submit(v)
		}
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return false, cmd
}

func (p *uiPrompt) view(_ *uiModel, width, _ int) string {
	w := boxWidth(width)
	p.input.Width = w - 10
	return stBox.Width(w).Render(stBold.Render(p.label) + "\n\n" + p.input.View() + "\n\n" + keyHints("enter", "save", "esc", "cancel"))
}

// ---- level picker ----

type uiLevelPicker struct {
	label  string
	level  int
	submit func(int) tea.Cmd
}

func newLevelPicker(label string, level int, submit func(int) tea.Cmd) *uiLevelPicker {
	return &uiLevelPicker{label: label, level: level, submit: submit}
}

func (p *uiLevelPicker) update(_ *uiModel, msg tea.Msg) (bool, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return false, nil
	}
	switch key.String() {
	case "esc", "ctrl+c", "q":
		return true, nil
	case "enter":
		return true, p.submit(p.level)
	case "left", "h", "up", "k":
		p.level = max(-2, p.level-1)
	case "right", "l", "down", "j", "tab":
		p.level = min(2, p.level+1)
	}
	return false, nil
}

func (p *uiLevelPicker) view(_ *uiModel, width, _ int) string {
	var opts []string
	for l := -2; l <= 2; l++ {
		name := " " + levelName(l) + " "
		if l == p.level {
			name = stTabOn.Render(levelName(l))
		} else {
			name = stTab.Render(levelName(l))
		}
		opts = append(opts, name)
	}
	return stBox.Width(boxWidth(width)).Render(stBold.Render(p.label) + "\n\n" + strings.Join(opts, " ") + "\n\n" +
		stDim.Render("critical bypasses Do Not Disturb (paid plans)") + "\n\n" + keyHints("←/→", "choose", "enter", "save", "esc", "cancel"))
}

// ---- help ----

type uiHelp struct{}

func (uiHelp) update(_ *uiModel, msg tea.Msg) (bool, tea.Cmd) {
	_, ok := msg.(tea.KeyMsg)
	return ok, nil
}

var uiHelpSections = []struct {
	title string
	keys  []string
}{
	{"Navigation", []string{
		"tab / shift+tab", "next / previous section",
		"1…5", "jump to a section",
		"↑↓ j k", "move",
		"pgup pgdn g G", "page, top, bottom",
		"enter", "open (apps/devices: their messages)",
		"esc", "back, clear search and filters",
		"J / K", "next / previous item in detail",
		"/", "search",
		"ctrl+r", "refresh",
		"y", "copy text, TID or webhook URL",
		"q", "quit",
	}},
	{"Messages", []string{
		"n", "new message",
		"f", "resend / duplicate selected",
		"r", "toggle read",
		"a", "mark all read",
		"u", "show only unread",
		"d / D", "delete one / delete all",
	}},
	{"Resources", []string{
		"s", "send to this app or device",
		"c", "create application",
		"e", "rename",
		"v", "app default level",
		"t", "pause / resume webhook",
		"d", "delete, remove, cancel",
	}},
}

func (uiHelp) view(_ *uiModel, width, _ int) string {
	var cols []string
	for _, s := range uiHelpSections {
		var b strings.Builder
		b.WriteString(stAccent.Bold(true).Render(s.title) + "\n")
		for i := 0; i+1 < len(s.keys); i += 2 {
			b.WriteString(stKey.Render(fit(s.keys[i], 15, false)) + " " + s.keys[i+1] + "\n")
		}
		cols = append(cols, b.String())
	}
	var body string
	if width >= 120 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, cols[0], "   ", lipgloss.JoinVertical(lipgloss.Left, cols[1], cols[2]))
	} else {
		body = strings.Join(cols, "\n")
	}
	return stBox.Render(body + "\n" + stDim.Render("New messages appear automatically every 15s. Any key closes."))
}

// ---- compose ----

type composeSeed struct {
	title, message string
	appName        string
	appTID         string
	deviceTID      string
	actions        []api.Action
	level          *int
}

const (
	cfTitle = iota
	cfMessage
	cfApp
	cfDevice
	cfLevel
	cfWhen
	cfImage
	cfAction1
	cfAction2
	cfAction3
	cfCount
)

// uiCompose is the new-message form. Its level index 0 means "use the
// application's default"; 1…5 map to -2…2.
type uiCompose struct {
	title   textinput.Model
	body    textarea.Model
	when    textinput.Model
	image   textinput.Model
	actions [3]textinput.Model
	apps    []api.Application
	devices []api.Device
	app     int
	device  int
	level   int
	focus   int
	err     string
	sending bool
}

func newCompose(m *uiModel, seed *composeSeed) *uiCompose {
	input := func(placeholder string, limit int) textinput.Model {
		in := textinput.New()
		in.Placeholder = placeholder
		in.CharLimit = limit
		in.Prompt = "  "
		return in
	}
	c := &uiCompose{
		title:   input("What happened?", 250),
		when:    input("now — or 30m, 2h, 1d, 2026-10-20 09:00", 40),
		image:   input("file path or https:// URL (paid)", 1000),
		apps:    m.apps,
		devices: m.devices,
	}
	for i := range c.actions {
		c.actions[i] = input("view:Open:https://…  http:Retry:https://…  copy:Copy:text", 2000)
	}
	c.body = textarea.New()
	c.body.Placeholder = "Details…"
	c.body.ShowLineNumbers = false
	c.body.Prompt = "  "
	c.body.CharLimit = 4000
	c.body.SetHeight(5)
	c.body.FocusedStyle.CursorLine = lipgloss.NewStyle()

	if seed != nil {
		c.title.SetValue(seed.title)
		c.body.SetValue(seed.message)
		for i, x := range c.apps {
			if (seed.appTID != "" && x.TID == seed.appTID) || (seed.appTID == "" && seed.appName != "" && x.Name == seed.appName) {
				c.app = i + 1
			}
		}
		for i, x := range c.devices {
			if x.TID == seed.deviceTID {
				c.device = i + 1
			}
		}
		if seed.level != nil {
			c.level = *seed.level + 3
		}
		for i, act := range seed.actions {
			if i < len(c.actions) {
				c.actions[i].SetValue(act.Type + ":" + act.Label + ":" + act.Value)
			}
		}
	}
	c.setFocus(cfTitle)
	return c
}

func (c *uiCompose) setFocus(f int) {
	c.focus = (f + cfCount) % cfCount
	c.title.Blur()
	c.body.Blur()
	c.when.Blur()
	c.image.Blur()
	for i := range c.actions {
		c.actions[i].Blur()
	}
	switch {
	case c.focus == cfTitle:
		c.title.Focus()
	case c.focus == cfMessage:
		c.body.Focus()
	case c.focus == cfWhen:
		c.when.Focus()
	case c.focus == cfImage:
		c.image.Focus()
	case c.focus >= cfAction1:
		c.actions[c.focus-cfAction1].Focus()
	}
}

func (c *uiCompose) cycle(delta int) {
	switch c.focus {
	case cfApp:
		c.app = (c.app + delta + len(c.apps) + 1) % (len(c.apps) + 1)
	case cfDevice:
		c.device = (c.device + delta + len(c.devices) + 1) % (len(c.devices) + 1)
	case cfLevel:
		c.level = (c.level + delta + 6) % 6
	}
}

func (c *uiCompose) update(m *uiModel, msg tea.Msg) (bool, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		if c.sending {
			return false, nil
		}
		switch key.String() {
		case "esc", "ctrl+c":
			return true, nil
		case "ctrl+s", "ctrl+enter", "alt+enter":
			return c.submit(m)
		case "tab", "down":
			if key.String() == "down" && c.focus == cfMessage {
				break
			}
			c.setFocus(c.focus + 1)
			return false, nil
		case "shift+tab", "up":
			if key.String() == "up" && c.focus == cfMessage {
				break
			}
			c.setFocus(c.focus - 1)
			return false, nil
		case "enter":
			if c.focus != cfMessage {
				if c.focus == cfAction3 {
					return c.submit(m)
				}
				c.setFocus(c.focus + 1)
				return false, nil
			}
		case "left", "right", " ", "h", "l":
			if c.focus == cfApp || c.focus == cfDevice || c.focus == cfLevel {
				if key.String() == "left" || key.String() == "h" {
					c.cycle(-1)
				} else {
					c.cycle(1)
				}
				return false, nil
			}
		}
	}
	var cmd tea.Cmd
	switch {
	case c.focus == cfTitle:
		c.title, cmd = c.title.Update(msg)
	case c.focus == cfMessage:
		c.body, cmd = c.body.Update(msg)
	case c.focus == cfWhen:
		c.when, cmd = c.when.Update(msg)
	case c.focus == cfImage:
		c.image, cmd = c.image.Update(msg)
	case c.focus >= cfAction1:
		i := c.focus - cfAction1
		c.actions[i], cmd = c.actions[i].Update(msg)
	}
	return false, cmd
}

func (c *uiCompose) submit(m *uiModel) (bool, tea.Cmd) {
	o := &messageOpts{
		title:   strings.TrimSpace(c.title.Value()),
		message: strings.TrimSpace(c.body.Value()),
		image:   strings.TrimSpace(c.image.Value()),
	}
	switch {
	case o.title == "":
		c.err = "Title is required"
		c.setFocus(cfTitle)
		return false, nil
	case o.message == "":
		c.err = "Message is required"
		c.setFocus(cfMessage)
		return false, nil
	}
	if c.app > 0 {
		o.application = c.apps[c.app-1].TID
	}
	if c.device > 0 {
		o.device = c.devices[c.device-1].TID
	}
	if c.level > 0 {
		o.level = strconv.Itoa(c.level - 3)
	}
	o.at, o.delay = splitWhen(c.when.Value())
	for _, in := range c.actions {
		if v := strings.TrimSpace(in.Value()); v != "" {
			o.actions = append(o.actions, v)
		}
	}
	if _, err := parseActions(o.actions, false); err != nil {
		c.err = err.Error()
		return false, nil
	}
	c.err, c.sending = "", true
	return false, m.send(o)
}

// splitWhen tells a delay ("30m", "1d") from an absolute time. Bare numbers
// of 10+ digits are Unix timestamps, shorter ones are seconds of delay.
func splitWhen(s string) (at, delay string) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "now") {
		return "", ""
	}
	if _, err := strconv.ParseUint(s, 10, 64); err == nil && len(s) >= 10 {
		return s, ""
	}
	if _, err := normalizeDelay(s); err == nil {
		return "", s
	}
	return s, ""
}

func (c *uiCompose) view(_ *uiModel, width, height int) string {
	w := boxWidth(width)
	inner := w - 6
	fieldW := inner - 13
	c.title.Width = fieldW
	c.when.Width = fieldW
	c.image.Width = fieldW
	for i := range c.actions {
		c.actions[i].Width = fieldW
	}
	c.body.SetWidth(fieldW)
	c.body.SetHeight(min(6, max(2, height-26)))

	label := func(f int, name string) string {
		if c.focus == f {
			return stAccent.Bold(true).Render(fmt.Sprintf("%-11s", "› "+name))
		}
		return stDim.Render(fmt.Sprintf("%-11s", "  "+name))
	}
	choice := func(f int, value string) string {
		if c.focus == f {
			return stAccent.Render("‹ ") + stBold.Render(value) + stAccent.Render(" ›")
		}
		return "  " + value
	}

	appName := "app default"
	if c.app > 0 {
		appName = c.apps[c.app-1].Name
	} else if len(c.apps) == 0 {
		appName = "—"
	}
	devName := "All devices"
	if c.device > 0 {
		devName = c.devices[c.device-1].Name
	}
	lvl := "app default"
	if c.level > 0 {
		lvl = levelStyle(c.level - 3).Render(levelName(c.level - 3))
	}
	if c.app == 0 && len(c.apps) > 0 {
		appName = "none (default app)"
	}

	rows := []string{
		stBold.Render("New message"),
		"",
		label(cfTitle, "Title") + c.title.View(),
		lipgloss.JoinHorizontal(lipgloss.Top, label(cfMessage, "Message"), c.body.View()),
		label(cfApp, "App") + choice(cfApp, appName),
		label(cfDevice, "Device") + choice(cfDevice, devName),
		label(cfLevel, "Level") + choice(cfLevel, lvl),
		label(cfWhen, "When") + c.when.View(),
		label(cfImage, "Image") + c.image.View(),
	}
	for i := range c.actions {
		rows = append(rows, label(cfAction1+i, "Action "+strconv.Itoa(i+1))+c.actions[i].View())
	}
	rows = append(rows, "")
	switch {
	case c.sending:
		rows = append(rows, stAccent.Render("Sending…"))
	case c.err != "":
		rows = append(rows, lipgloss.NewStyle().Width(inner).Render(stErr.Render(c.err)))
	default:
		rows = append(rows, keyHints("ctrl+s", "send", "tab", "next field", "←/→", "choose", "esc", "cancel"))
	}
	return stBox.Width(w).Render(strings.Join(rows, "\n"))
}
