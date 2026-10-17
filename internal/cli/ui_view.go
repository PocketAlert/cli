package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/PocketAlert/cli/internal/api"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	colAccent = lipgloss.AdaptiveColor{Light: "#C2410C", Dark: "#FB923C"}
	colDim    = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8B8F98"}
	colFaint  = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#3F434A"}
	colSelBg  = lipgloss.AdaptiveColor{Light: "#FFEDD5", Dark: "#3B2A1E"}
	colErr    = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	colOK     = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}

	stAccent = lipgloss.NewStyle().Foreground(colAccent)
	stDim    = lipgloss.NewStyle().Foreground(colDim)
	stBold   = lipgloss.NewStyle().Bold(true)
	stErr    = lipgloss.NewStyle().Foreground(colErr)
	stOK     = lipgloss.NewStyle().Foreground(colOK)
	stSel    = lipgloss.NewStyle().Background(colSelBg).Foreground(lipgloss.AdaptiveColor{Light: "#111827", Dark: "#F9FAFB"})
	stTab    = lipgloss.NewStyle().Padding(0, 1).Foreground(colDim)
	stTabOn  = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(colAccent)
	stKey    = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	stBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colAccent).Padding(1, 2)
	stRule   = lipgloss.NewStyle().Foreground(colFaint)

	levelStyles = map[int]lipgloss.Style{
		-2: lipgloss.NewStyle().Foreground(colDim),
		-1: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#2563EB", Dark: "#60A5FA"}),
		0:  lipgloss.NewStyle().Foreground(colDim),
		1:  lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#C2410C", Dark: "#FB923C"}).Bold(true),
		2:  lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#DC2626")).Bold(true),
	}
)

func levelStyle(p int) lipgloss.Style {
	if s, ok := levelStyles[p]; ok {
		return s
	}
	return stDim
}

func levelBadge(p int) string {
	switch p {
	case 2:
		return "CRIT"
	case 1:
		return "HIGH"
	case -1:
		return "low"
	case -2:
		return "mute"
	}
	return ""
}

func (m *uiModel) bodyHeight() int { return max(1, m.height-3) }

// listHeight leaves one row for the column header.
func (m *uiModel) listHeight() int { return max(1, m.bodyHeight()-1) }

func (m *uiModel) previewWidth() int {
	if m.width < 120 || m.sec == secApps || m.sec == secDevices {
		return 0
	}
	return m.width * 2 / 5
}

func (m *uiModel) resizeViewport() {
	m.viewport.Width = max(10, m.width-4)
	m.viewport.Height = m.bodyHeight()
	m.refreshDetail()
}

func (m *uiModel) refreshDetail() {
	if !m.detail {
		return
	}
	if m.rowCount() == 0 {
		m.detail = false
		return
	}
	m.viewport.SetContent(m.detailContent(m.viewport.Width, true))
}

func (m *uiModel) View() string {
	if m.width == 0 {
		return ""
	}
	body := m.viewBody()
	return lipgloss.JoinVertical(lipgloss.Left, m.viewHeader(), m.viewSubheader(), body, m.viewFooter())
}

func (m *uiModel) viewHeader() string {
	logo := stAccent.Bold(true).Render(" ◆ Pocket Alert ")
	var tabs []string
	for i, name := range uiSectionNames {
		label := fmt.Sprintf("%d %s", i+1, name)
		if uiSection(i) == secInbox {
			if n := m.unreadCount(); n > 0 {
				label += fmt.Sprintf(" (%d)", n)
			}
		}
		if uiSection(i) == m.sec {
			tabs = append(tabs, stTabOn.Render(label))
		} else {
			tabs = append(tabs, stTab.Render(label))
		}
	}
	left := logo + strings.Join(tabs, "")

	var right []string
	if u := m.usage; u != nil {
		if u.Unlimited || u.Limit == nil {
			right = append(right, stDim.Render(fmt.Sprintf("%d sent today", u.Used)))
		} else {
			st := stDim
			if u.Used >= *u.Limit {
				st = stErr
			}
			right = append(right, st.Render(fmt.Sprintf("%d/%d today", u.Used, *u.Limit)))
		}
	}
	if p := m.profile; p != nil && p.Email != "" {
		right = append([]string{stDim.Render(p.Email)}, right...)
	}
	spin := ""
	if m.pending > 0 || m.loadingMore {
		spin = m.spin.View() + " "
	}
	// Drop the e-mail first, then usage, when the tabs need the room.
	for i := 0; i <= len(right); i++ {
		r := spin + strings.Join(right[i:], stDim.Render(" · ")) + " "
		if gap := m.width - lipgloss.Width(left) - lipgloss.Width(r); gap >= 1 {
			return left + strings.Repeat(" ", gap) + r
		}
	}
	return ansi.Truncate(left, m.width, "")
}

func (m *uiModel) unreadCount() int {
	n := 0
	for _, x := range m.messages {
		if !x.IsRead {
			n++
		}
	}
	return n
}

func (m *uiModel) viewSubheader() string {
	if m.searching {
		return " " + m.searchInput.View()
	}
	var parts []string
	switch m.sec {
	case secInbox:
		info := fmt.Sprintf("%d messages", m.total)
		if len(m.messages) < m.total {
			info += fmt.Sprintf(", %d loaded", len(m.messages))
		}
		parts = append(parts, info)
		if m.filterLabel != "" {
			parts = append(parts, stAccent.Render("["+m.filterLabel+"]"))
		}
		if m.unreadOnly {
			parts = append(parts, stAccent.Render("[unread]"))
		}
	case secScheduled:
		parts = append(parts, fmt.Sprintf("%d %s", len(m.scheduled), m.schedStatus), stDim.Render("s: status"))
		if len(m.scheduled) > 0 {
			parts = append(parts, stDim.Render("times in "+m.scheduled[0].TZ))
		}
	case secApps:
		parts = append(parts, fmt.Sprintf("%d applications", len(m.apps)))
	case secDevices:
		parts = append(parts, fmt.Sprintf("%d devices", len(m.devices)))
	case secWebhooks:
		parts = append(parts, fmt.Sprintf("%d webhooks", len(m.hooks)))
	}
	if m.query != "" {
		parts = append(parts, stAccent.Render("/"+m.query))
	}
	return stDim.Render(" ") + strings.Join(parts, stDim.Render(" · "))
}

func (m *uiModel) viewFooter() string {
	if m.status != "" {
		st := stOK
		if m.statusErr {
			st = stErr
		}
		return " " + ansi.Truncate(st.Render(m.status), m.width-2, "…")
	}
	var hints [][2]string
	switch {
	case m.overlay != nil:
		return ""
	case m.searching:
		hints = [][2]string{{"enter", "apply"}, {"esc", "clear"}}
	case m.detail:
		hints = [][2]string{{"esc", "back"}, {"J/K", "next/prev"}, {"↑↓", "scroll"}}
		hints = append(hints, m.sectionHints()...)
	default:
		hints = [][2]string{{"tab", "switch"}, {"enter", "open"}, {"n", "new"}}
		hints = append(hints, m.sectionHints()...)
		hints = append(hints, [2]string{"/", "search"}, [2]string{"?", "help"}, [2]string{"q", "quit"})
	}
	var b strings.Builder
	b.WriteString(" ")
	for i, h := range hints {
		if i > 0 {
			b.WriteString(stDim.Render(" · "))
		}
		b.WriteString(stKey.Render(h[0]) + " " + stDim.Render(h[1]))
	}
	return ansi.Truncate(b.String(), m.width, "…")
}

func (m *uiModel) sectionHints() [][2]string {
	switch m.sec {
	case secInbox:
		return [][2]string{{"r", "read"}, {"d", "delete"}, {"f", "resend"}, {"u", "unread"}}
	case secScheduled:
		return [][2]string{{"d", "cancel"}, {"f", "duplicate"}, {"s", "status"}}
	case secApps:
		return [][2]string{{"s", "send"}, {"c", "create"}, {"e", "rename"}, {"v", "level"}, {"d", "delete"}}
	case secDevices:
		return [][2]string{{"s", "send"}, {"e", "rename"}, {"d", "remove"}}
	case secWebhooks:
		return [][2]string{{"t", "pause/resume"}, {"y", "copy URL"}, {"e", "rename"}, {"d", "delete"}}
	}
	return nil
}

func (m *uiModel) viewBody() string {
	h := m.bodyHeight()
	if m.overlay != nil {
		return lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, m.overlay.view(m, m.width, h))
	}
	if m.detail {
		return lipgloss.NewStyle().Padding(0, 2).Height(h).MaxHeight(h).Render(m.viewport.View())
	}
	pw := m.previewWidth()
	list := m.viewList(m.width - pw)
	if pw == 0 {
		return list
	}
	preview := ""
	if m.rowCount() > 0 {
		preview = m.detailContent(pw-4, false)
	}
	pane := lipgloss.NewStyle().
		Width(pw-1).Height(h).MaxHeight(h).
		Padding(0, 1).
		BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(colFaint).
		Render(preview)
	return lipgloss.JoinHorizontal(lipgloss.Top, list, pane)
}

// ---- list ----

type uiCol struct {
	text  string
	width int // 0 = flexible
	style lipgloss.Style
	right bool
}

func fit(s string, w int, right bool) string {
	s = ansi.Truncate(strings.Join(strings.Fields(s), " "), w, "…")
	pad := strings.Repeat(" ", max(0, w-ansi.StringWidth(s)))
	if right {
		return pad + s
	}
	return s + pad
}

func renderRow(cols []uiCol, width int, selected bool) string {
	fixed, flex := 0, 0
	for _, c := range cols {
		fixed += c.width + 1
		if c.width == 0 {
			flex++
		}
	}
	flexW := 0
	if flex > 0 {
		flexW = max(4, (width-2-fixed)/flex)
	}
	var b strings.Builder
	b.WriteString(" ")
	for _, c := range cols {
		w := c.width
		if w == 0 {
			w = flexW
		}
		cell := fit(c.text, w, c.right)
		if !selected {
			cell = c.style.Render(cell)
		}
		b.WriteString(cell + " ")
	}
	line := ansi.Truncate(b.String(), width, "")
	if selected {
		return stSel.Width(width).Render(line)
	}
	return line + strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
}

func (m *uiModel) viewList(width int) string {
	h := m.listHeight()
	header, rows := m.rows()
	cur := m.cursor[m.sec]
	top := 0
	if cur >= h {
		top = cur - h + 1
	}
	lines := []string{stDim.Bold(true).Render(renderRow(header, width, false))}
	for i := top; i < len(rows) && i < top+h; i++ {
		lines = append(lines, renderRow(rows[i], width, i == cur))
	}
	if len(rows) == 0 {
		lines = append(lines, "", stDim.Render(fit(" "+m.emptyText(), width, false)))
	}
	return lipgloss.NewStyle().Width(width).Height(m.bodyHeight()).MaxHeight(m.bodyHeight()).Render(strings.Join(lines, "\n"))
}

func (m *uiModel) emptyText() string {
	if !m.loaded[m.sec] || m.pending > 0 {
		return "Loading…"
	}
	if m.query != "" || m.filterLabel != "" || m.unreadOnly {
		return "Nothing matches. esc clears the filter."
	}
	switch m.sec {
	case secInbox:
		return "No messages yet. Press n to send one."
	case secScheduled:
		return "No " + m.schedStatus + " scheduled messages. Press n and set “When”."
	case secApps:
		return "No applications. Press c to create one."
	case secDevices:
		return "No devices. Install the Pocket Alert app and sign in."
	case secWebhooks:
		return "No webhooks. Create one with: pocketalert webhooks create"
	}
	return ""
}

func (m *uiModel) rows() ([]uiCol, [][]uiCol) {
	var rows [][]uiCol
	switch m.sec {
	case secInbox:
		for _, x := range m.filter(m.messages) {
			mark, title := " ", stDim
			if !x.IsRead {
				mark, title = "●", stBold
			}
			if m.fresh[x.TID] {
				mark = "★"
			}
			t := x.Title
			if x.WasScheduled {
				t = "⏱ " + t
			}
			if len(x.Actions) > 0 || x.Attachment != nil {
				t += stDim.Render(attachmentHint(x))
			}
			rows = append(rows, []uiCol{
				{text: mark, width: 1, style: stAccent},
				{text: levelBadge(x.Priority), width: 4, style: levelStyle(x.Priority)},
				{text: t, style: title},
				{text: x.Application, width: 16, style: stDim},
				{text: shortTime(x.CreatedAt), width: 12, style: stDim, right: true},
			})
		}
		return []uiCol{{text: "", width: 1}, {text: "LVL", width: 4}, {text: "TITLE"}, {text: "APP", width: 16}, {text: "TIME", width: 12, right: true}}, rows
	case secScheduled:
		for _, x := range m.visibleScheduled() {
			rows = append(rows, []uiCol{
				{text: x.DeliverAt, width: 19, style: stAccent},
				{text: x.Status, width: 9, style: statusStyle(x.Status)},
				{text: levelBadge(x.Priority), width: 4, style: levelStyle(x.Priority)},
				{text: x.Title},
				{text: dash(x.Application), width: 16, style: stDim},
			})
		}
		return []uiCol{{text: "DELIVER AT", width: 19}, {text: "STATUS", width: 9}, {text: "LVL", width: 4}, {text: "TITLE"}, {text: "APP", width: 16}}, rows
	case secApps:
		for _, x := range m.visibleApps() {
			rows = append(rows, []uiCol{
				{text: "●", width: 1, style: lipgloss.NewStyle().Foreground(lipgloss.Color(x.Color))},
				{text: x.Name, style: stBold},
				{text: dash(levelPtrName(x.DefaultLevel)), width: 9, style: stDim},
				{text: activeLabel(x.IsActive), width: 8, style: activeStyle(x.IsActive)},
				{text: x.TID, width: 22, style: stDim},
			})
		}
		return []uiCol{{text: "", width: 1}, {text: "NAME"}, {text: "LEVEL", width: 9}, {text: "STATE", width: 8}, {text: "TID", width: 22}}, rows
	case secDevices:
		for _, x := range m.visibleDevices() {
			rows = append(rows, []uiCol{
				{text: x.Name, style: stBold},
				{text: activeLabel(x.IsActive), width: 8, style: activeStyle(x.IsActive)},
				{text: shortTime(x.CreatedAt), width: 12, style: stDim},
				{text: x.TID, width: 22, style: stDim},
			})
		}
		return []uiCol{{text: "NAME"}, {text: "STATE", width: 8}, {text: "ADDED", width: 12}, {text: "TID", width: 22}}, rows
	case secWebhooks:
		for _, x := range m.visibleHooks() {
			rows = append(rows, []uiCol{
				{text: activeLabel(hookActive(x)), width: 8, style: activeStyle(hookActive(x))},
				{text: x.Name, style: stBold},
				{text: "p4a.me/wh/" + x.URL, width: 30, style: stDim},
				{text: dash(levelPtrName(x.DefaultLevel)), width: 9, style: stDim},
			})
		}
		return []uiCol{{text: "STATE", width: 8}, {text: "NAME"}, {text: "URL", width: 30}, {text: "LEVEL", width: 9}}, rows
	}
	return nil, nil
}

func attachmentHint(x api.Message) string {
	s := ""
	if x.Attachment != nil {
		s += " 🖼"
	}
	if len(x.Actions) > 0 {
		s += fmt.Sprintf(" ⚡%d", len(x.Actions))
	}
	return s
}

func activeLabel(b bool) string {
	if b {
		return "active"
	}
	return "paused"
}

func activeStyle(b bool) lipgloss.Style {
	if b {
		return stOK
	}
	return stDim
}

func statusStyle(s string) lipgloss.Style {
	switch s {
	case "pending", "sending":
		return stAccent
	case "failed":
		return stErr
	case "sent":
		return stOK
	}
	return stDim
}

var apiTimeLayouts = []string{"02.01.2006 15:04:05", time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"}

// shortTime turns the API's account-local timestamps into "15:04" for today
// and a date otherwise, so the list stays narrow.
func shortTime(s string) string {
	for _, layout := range apiTimeLayouts {
		t, err := time.ParseInLocation(layout, s, time.Local)
		if err != nil {
			continue
		}
		now := time.Now()
		switch {
		case t.Year() == now.Year() && t.YearDay() == now.YearDay():
			return t.Format("15:04")
		case t.Year() == now.Year():
			return t.Format("Jan 02 15:04")
		default:
			return t.Format("2006-01-02")
		}
	}
	return s
}

// ---- detail ----

func (m *uiModel) detailContent(width int, full bool) string {
	wrap := lipgloss.NewStyle().Width(width)
	var b strings.Builder
	kv := func(k, v string) {
		if v != "" {
			b.WriteString(stDim.Render(fmt.Sprintf("%-12s", k)) + v + "\n")
		}
	}
	actions := func(list []api.Action) {
		for i, act := range list {
			kv(fmt.Sprintf("Action %d", i+1), fmt.Sprintf("%s %s → %s", stAccent.Render("["+act.Type+"]"), act.Label, act.Value))
		}
	}
	switch m.sec {
	case secInbox:
		x, ok := m.selectedMessage()
		if !ok {
			return ""
		}
		b.WriteString(wrap.Render(stBold.Render(x.Title)) + "\n\n")
		kv("Level", levelStyle(x.Priority).Render(levelName(x.Priority)))
		kv("Application", x.Application)
		kv("Device", x.Device)
		kv("Received", x.CreatedAt)
		if x.WasScheduled {
			kv("Scheduled", "yes")
		}
		if x.Attachment != nil {
			kv("Image", x.Attachment.Link)
		}
		actions(x.Actions)
		kv("TID", stDim.Render(x.TID))
		b.WriteString("\n" + wrap.Render(x.Message) + "\n")
	case secScheduled:
		x, ok := m.selectedScheduled()
		if !ok {
			return ""
		}
		b.WriteString(wrap.Render(stBold.Render(x.Title)) + "\n\n")
		kv("Deliver at", stAccent.Render(x.DeliverAt+" ("+x.TZ+")"))
		kv("Status", statusStyle(x.Status).Render(x.Status))
		kv("Level", levelStyle(x.Priority).Render(levelName(x.Priority)))
		kv("Application", x.Application)
		kv("Device", x.Device)
		kv("Created", x.CreatedAt)
		actions(x.Actions)
		kv("TID", stDim.Render(x.TID))
		b.WriteString("\n" + wrap.Render(x.Message) + "\n")
	case secWebhooks:
		x, ok := m.selectedHook()
		if !ok {
			return ""
		}
		b.WriteString(stBold.Render(x.Name) + "  " + activeStyle(hookActive(x)).Render(activeLabel(hookActive(x))) + "\n\n")
		kv("URL", stAccent.Render(webhookBaseURL+x.URL))
		kv("Level", levelPtrName(x.DefaultLevel))
		kv("Application", m.appName(x.ApplicationTID))
		kv("Device", m.deviceName(x.DeviceTID))
		kv("Created", x.CreatedAt)
		kv("TID", stDim.Render(x.TID))
		b.WriteString("\n" + stDim.Render("Message template") + "\n" + wrap.Render(x.Message) + "\n")
		if x.Actions != "" && x.Actions != "[]" {
			b.WriteString("\n" + stDim.Render("Actions") + "\n" + wrap.Render(x.Actions) + "\n")
		}
		if full {
			b.WriteString("\n" + m.historyContent(width))
		} else {
			b.WriteString("\n" + stDim.Render("enter: request history"))
		}
	}
	return b.String()
}

func (m *uiModel) historyContent(width int) string {
	var b strings.Builder
	b.WriteString(stRule.Render(strings.Repeat("─", min(width, 60))) + "\n")
	h := m.history
	if h == nil {
		return b.String() + stDim.Render("Loading history…")
	}
	b.WriteString(stBold.Render(fmt.Sprintf("Recent requests (%d total, kept %d days)", h.Total, h.RetentionDays)) + "\n\n")
	if len(h.Requests) == 0 {
		return b.String() + stDim.Render("No requests yet. curl -X POST "+webhookBaseURL+"… to test.")
	}
	for _, r := range h.Requests {
		st := stOK
		if r.Status != "success" {
			st = stErr
		}
		line := fmt.Sprintf("%s  %s  %-6s %s", stDim.Render(r.CreatedAt), st.Render(fmt.Sprintf("%-7s", r.Status)), r.Method, stDim.Render(fmt.Sprintf("%dms", r.DurationMS)))
		b.WriteString(line + "\n")
		detail := r.RenderedMessage
		if r.ErrorCode != "" {
			detail = stErr.Render(r.ErrorCode)
		}
		if detail != "" {
			b.WriteString("  " + ansi.Truncate(strings.Join(strings.Fields(detail), " "), width-2, "…") + "\n")
		}
	}
	return b.String()
}

func (m *uiModel) appName(tid string) string {
	for _, x := range m.apps {
		if x.TID == tid {
			return x.Name
		}
	}
	return tid
}

func (m *uiModel) deviceName(tid string) string {
	if tid == "" || tid == "all" {
		return "All devices"
	}
	for _, x := range m.devices {
		if x.TID == tid {
			return x.Name
		}
	}
	return tid
}
