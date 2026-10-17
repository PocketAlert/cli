package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/PocketAlert/cli/internal/api"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

type uiSection int

const (
	secInbox uiSection = iota
	secScheduled
	secApps
	secDevices
	secWebhooks
	uiSectionCount
)

var uiSectionNames = [uiSectionCount]string{"Inbox", "Scheduled", "Applications", "Devices", "Webhooks"}

var uiScheduledStatuses = []string{"pending", "all", "sent", "canceled", "failed"}

func (a *app) uiCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "ui",
		Aliases: []string{"tui", "i"},
		Short:   "Interactive mode: browse messages, compose, manage resources",
		Long:    `Full-screen interactive mode. Press ? inside for keyboard shortcuts.`,
		Args:    cobra.NoArgs,
		RunE:    func(cmd *cobra.Command, _ []string) error { return a.runUI(cmd.Context()) },
	}
}

func (a *app) runUI(ctx context.Context) error {
	if a.cfg.APIKey == "" {
		return api.ErrNotAuthenticated
	}
	if !isTerminal(a.stdin) || !isTerminal(a.stdout) {
		return fmt.Errorf("interactive mode needs a terminal")
	}
	m := newUIModel(ctx, a)
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if err == tea.ErrProgramKilled && ctx.Err() != nil {
		return nil
	}
	return err
}

type uiModel struct {
	a   *app
	ctx context.Context

	width, height int
	sec           uiSection
	cursor        [uiSectionCount]int
	loaded        [uiSectionCount]bool
	pending       int

	profile *api.Profile
	usage   *api.MessageUsage

	messages     []api.Message
	total        int
	hasMore      bool
	loadingMore  bool
	unreadOnly   bool
	deviceFilter string
	appFilter    string
	appFilterTID string
	fillPages    int
	filterLabel  string
	fresh        map[string]bool
	scheduled    []api.ScheduledMessage
	schedStatus  string
	apps         []api.Application
	devices      []api.Device
	hooks        []api.Webhook
	history      *api.WebhookHistory
	historyFor   string
	searchInput  textinput.Model
	searching    bool
	query        string
	detail       bool
	viewport     viewport.Model
	overlay      uiOverlay
	status       string
	statusErr    bool
	statusSeq    int
	spin         spinner.Model
}

// uiOverlay is a modal that takes over keyboard input until it closes.
type uiOverlay interface {
	update(m *uiModel, msg tea.Msg) (closed bool, cmd tea.Cmd)
	view(m *uiModel, width, height int) string
}

func newUIModel(ctx context.Context, a *app) *uiModel {
	search := textinput.New()
	search.Prompt = "/ "
	search.Placeholder = "search title, text, app"
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = stAccent
	return &uiModel{
		a:           a,
		ctx:         ctx,
		fresh:       map[string]bool{},
		schedStatus: "pending",
		searchInput: search,
		spin:        sp,
		viewport:    viewport.New(0, 0),
	}
}

func (m *uiModel) Init() tea.Cmd {
	m.pending = 3
	m.loaded[secInbox] = true
	m.loaded[secApps] = true
	m.loaded[secDevices] = true
	return tea.Batch(m.spin.Tick, m.loadAccount(), m.loadInbox(0, false), m.loadApps(), m.loadDevices(), uiPoll())
}

func (m *uiModel) setStatus(text string, isErr bool) tea.Cmd {
	m.status, m.statusErr = text, isErr
	m.statusSeq++
	seq := m.statusSeq
	return tea.Tick(uiStatusFor, func(_ time.Time) tea.Msg { return uiClearStatusMsg{seq} })
}

func (m *uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeViewport()
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case uiClearStatusMsg:
		if msg.seq == m.statusSeq {
			m.status = ""
		}
		return m, nil
	case uiPollMsg:
		cmds := []tea.Cmd{uiPoll()}
		if m.loaded[secInbox] && !m.loadingMore {
			cmds = append(cmds, m.loadInbox(0, true), m.loadAccount())
		}
		return m, tea.Batch(cmds...)
	case uiErrMsg:
		m.done()
		m.loadingMore = false
		return m, m.setStatus(clean(msg.err.Error()), true)
	case uiAccountMsg:
		m.profile, m.usage = &msg.profile, &msg.usage
		return m, nil
	case uiInboxMsg:
		return m, m.applyInbox(msg)
	case uiScheduledMsg:
		m.scheduled = msg
		m.done()
		m.clampCursor(secScheduled)
		m.refreshDetail()
		return m, nil
	case uiAppsMsg:
		m.apps = msg
		m.done()
		m.clampCursor(secApps)
		return m, nil
	case uiDevicesMsg:
		m.devices = msg
		m.done()
		m.clampCursor(secDevices)
		return m, nil
	case uiHooksMsg:
		m.hooks = msg
		m.done()
		m.clampCursor(secWebhooks)
		m.refreshDetail()
		return m, nil
	case uiHistoryMsg:
		if msg.tid == m.historyFor {
			m.history = &msg.history
			m.refreshDetail()
		}
		return m, nil
	case uiDoneMsg:
		cmds := []tea.Cmd{m.setStatus(msg.text, false)}
		for _, s := range msg.reload {
			cmds = append(cmds, m.reload(s))
		}
		return m, tea.Batch(cmds...)
	case uiSentMsg:
		return m, m.applySent(msg)
	}

	if m.overlay != nil {
		closed, cmd := m.overlay.update(m, msg)
		if closed {
			m.overlay = nil
		}
		return m, cmd
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.searching {
		return m, m.updateSearch(key)
	}
	if m.detail {
		return m, m.updateDetail(key)
	}
	return m, m.updateList(key)
}

func (m *uiModel) done() {
	if m.pending > 0 {
		m.pending--
	}
}

func (m *uiModel) reload(s uiSection) tea.Cmd {
	if !m.loaded[s] && s != m.sec {
		return nil
	}
	m.loaded[s] = true
	m.pending++
	return m.load(s)
}

func (m *uiModel) applyInbox(msg uiInboxMsg) tea.Cmd {
	page := msg.page
	if !msg.poll {
		m.done()
	}
	m.total, m.loadingMore = page.Meta.Total, false
	switch {
	case msg.poll:
		known := make(map[string]bool, len(m.messages))
		for _, x := range m.messages {
			known[x.TID] = true
		}
		var added []api.Message
		for _, x := range page.Data {
			if !known[x.TID] {
				added = append(added, x)
				m.fresh[x.TID] = true
			}
		}
		if len(added) == 0 {
			return nil
		}
		if m.sec == secInbox && (m.cursor[secInbox] > 0 || m.detail) {
			m.cursor[secInbox] += len(m.filter(added))
		}
		m.messages = append(added, m.messages...)
		return m.setStatus(fmt.Sprintf("%d new message%s", len(added), plural(len(added))), false)
	case msg.offset == 0:
		m.messages = page.Data
		m.hasMore = page.Meta.HasMore
		m.fillPages = 0
	default:
		m.messages = append(m.messages, page.Data...)
		m.hasMore = page.Meta.HasMore
	}
	m.clampCursor(secInbox)
	m.refreshDetail()
	return m.fill()
}

// fill keeps loading pages while client-side filters (search, unread, or an
// app filter an older API ignores) leave the screen short of rows.
func (m *uiModel) fill() tea.Cmd {
	filtered := m.query != "" || m.unreadOnly || m.appFilter != ""
	if m.sec != secInbox || !filtered || !m.hasMore || m.loadingMore || m.fillPages >= uiMaxFillPages || m.rowCount() >= m.listHeight() {
		return nil
	}
	m.fillPages++
	m.loadingMore = true
	return m.loadInbox(len(m.messages), false)
}

func (m *uiModel) applySent(msg uiSentMsg) tea.Cmd {
	if c, ok := m.overlay.(*uiCompose); ok {
		if msg.err != nil {
			c.sending, c.err = false, msg.err.Error()
			return nil
		}
		m.overlay = nil
	} else if msg.err != nil {
		return m.setStatus(msg.err.Error(), true)
	}
	text := "✓ Sent"
	reload := secInbox
	if msg.res.Scheduled {
		text = fmt.Sprintf("✓ Scheduled for %s (%s)", msg.res.DeliverAt, msg.res.TZ)
		reload = secScheduled
	}
	if want, err := parseLevel(msg.opts.level); msg.opts.level != "" && err == nil && want > msg.res.Priority {
		text += fmt.Sprintf(" · delivered as %s, %s needs a paid plan", levelName(msg.res.Priority), levelName(want))
	}
	if len(msg.opts.actions) > 0 && len(msg.res.Actions) == 0 {
		text += " · actions need a paid plan"
	}
	m.loaded[reload] = true
	m.pending++
	return tea.Batch(m.setStatus(text, false), m.load(reload), m.loadAccount())
}

// ---- key handling ----

func (m *uiModel) updateSearch(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.searching = false
		m.query = ""
		m.searchInput.SetValue("")
		m.searchInput.Blur()
		return nil
	case "enter", "down", "up":
		m.searching = false
		m.searchInput.Blur()
		return nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(key)
	m.query = strings.TrimSpace(m.searchInput.Value())
	m.cursor[m.sec] = 0
	m.fillPages = 0
	return tea.Batch(cmd, m.fill())
}

func (m *uiModel) updateDetail(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc", "backspace", "left", "h":
		m.detail = false
		m.history, m.historyFor = nil, ""
		return nil
	case "q", "ctrl+c":
		return tea.Quit
	case "J", "ctrl+n":
		return m.stepDetail(1)
	case "K", "ctrl+p":
		return m.stepDetail(-1)
	}
	if cmd, handled := m.actionKey(key); handled {
		return cmd
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(key)
	return cmd
}

func (m *uiModel) stepDetail(delta int) tea.Cmd {
	n := m.rowCount()
	next := m.cursor[m.sec] + delta
	if next < 0 || next >= n {
		return nil
	}
	m.cursor[m.sec] = next
	return m.openDetail()
}

func (m *uiModel) updateList(key tea.KeyMsg) tea.Cmd {
	n := m.rowCount()
	c := &m.cursor[m.sec]
	page := max(1, m.listHeight()-1)
	switch key.String() {
	case "ctrl+c", "q":
		return tea.Quit
	case "tab", "L":
		return m.switchTo((m.sec + 1) % uiSectionCount)
	case "shift+tab", "H":
		return m.switchTo((m.sec + uiSectionCount - 1) % uiSectionCount)
	case "1", "2", "3", "4", "5":
		return m.switchTo(uiSection(key.String()[0] - '1'))
	case "down", "j":
		if *c < n-1 {
			*c++
		}
	case "up", "k":
		if *c > 0 {
			*c--
		}
	case "pgdown", "ctrl+d", " ":
		*c = min(n-1, *c+page)
	case "pgup", "ctrl+u":
		*c = max(0, *c-page)
	case "home", "g":
		*c = 0
	case "end", "G":
		*c = max(0, n-1)
	case "enter", "right", "l":
		return m.enter()
	case "/":
		m.searching = true
		m.searchInput.SetValue(m.query)
		m.searchInput.CursorEnd()
		return m.searchInput.Focus()
	case "esc":
		if m.query != "" {
			m.query = ""
			m.searchInput.SetValue("")
		} else if m.filterLabel != "" || m.unreadOnly {
			return m.clearFilters()
		}
	case "?":
		m.overlay = uiHelp{}
	case "ctrl+r", "f5":
		return m.reload(m.sec)
	default:
		cmd, _ := m.actionKey(key)
		return cmd
	}
	return m.maybeLoadMore()
}

func (m *uiModel) switchTo(s uiSection) tea.Cmd {
	if s < 0 || s >= uiSectionCount || s == m.sec {
		return nil
	}
	m.sec = s
	m.detail = false
	m.query, m.searching = "", false
	m.searchInput.SetValue("")
	if !m.loaded[s] {
		return m.reload(s)
	}
	return nil
}

func (m *uiModel) maybeLoadMore() tea.Cmd {
	if m.sec != secInbox || !m.hasMore || m.loadingMore {
		return nil
	}
	if m.cursor[secInbox] < m.rowCount()-5 {
		return nil
	}
	m.loadingMore = true
	return m.loadInbox(len(m.messages), false)
}

func (m *uiModel) clearFilters() tea.Cmd {
	refetch := m.deviceFilter != "" || m.appFilterTID != ""
	m.deviceFilter, m.appFilter, m.appFilterTID, m.filterLabel, m.unreadOnly = "", "", "", "", false
	m.cursor[secInbox] = 0
	if refetch {
		return m.reload(secInbox)
	}
	return nil
}

func (m *uiModel) enter() tea.Cmd {
	switch m.sec {
	case secApps:
		if x, ok := m.selectedApp(); ok {
			m.appFilter, m.appFilterTID, m.deviceFilter, m.filterLabel = x.Name, x.TID, "", "app: "+x.Name
			m.cursor[secInbox] = 0
			m.sec = secInbox
			m.messages = nil
			return m.reload(secInbox)
		}
		return nil
	case secDevices:
		if x, ok := m.selectedDevice(); ok {
			m.deviceFilter, m.appFilter, m.appFilterTID, m.filterLabel = x.TID, "", "", "device: "+x.Name
			m.cursor[secInbox] = 0
			m.sec = secInbox
			m.messages = nil
			return m.reload(secInbox)
		}
		return nil
	}
	return m.openDetail()
}

func (m *uiModel) openDetail() tea.Cmd {
	if m.rowCount() == 0 {
		return nil
	}
	m.detail = true
	m.viewport.GotoTop()
	var cmd tea.Cmd
	switch m.sec {
	case secInbox:
		x, _ := m.selectedMessage()
		delete(m.fresh, x.TID)
		if !x.IsRead {
			m.setRead(x.TID)
			cmd = m.call(func(ctx context.Context) (tea.Msg, error) {
				_, err := m.a.client.Do(ctx, "PUT", "/v1/messages/"+url.PathEscape(x.TID), nil, map[string]bool{"is_read": true}, nil)
				return nil, err
			})
		}
	case secWebhooks:
		x, _ := m.selectedHook()
		m.history, m.historyFor = nil, x.TID
		cmd = m.loadHistory(x.TID)
	}
	m.refreshDetail()
	return cmd
}

func (m *uiModel) setRead(tids ...string) {
	set := map[string]bool{}
	for _, t := range tids {
		set[t] = true
	}
	for i := range m.messages {
		if len(tids) == 0 || set[m.messages[i].TID] {
			m.messages[i].IsRead = true
		}
	}
}

// actionKey handles the per-section shortcuts shared by the list and the
// detail view.
func (m *uiModel) actionKey(key tea.KeyMsg) (tea.Cmd, bool) {
	k := key.String()
	switch k {
	case "n":
		m.overlay = newCompose(m, nil)
		return textinput.Blink, true
	case "y":
		return m.copySelected(), true
	}
	switch m.sec {
	case secInbox:
		return m.inboxKey(k)
	case secScheduled:
		return m.scheduledKey(k)
	case secApps:
		return m.appsKey(k)
	case secDevices:
		return m.devicesKey(k)
	case secWebhooks:
		return m.hooksKey(k)
	}
	return nil, false
}

func (m *uiModel) inboxKey(k string) (tea.Cmd, bool) {
	x, ok := m.selectedMessage()
	switch k {
	case "u":
		m.unreadOnly = !m.unreadOnly
		m.cursor[secInbox] = 0
		m.fillPages = 0
		return m.fill(), true
	case "a":
		m.overlay = newConfirm("Mark all messages as read?", func() tea.Cmd {
			m.setRead()
			return m.mutate("PUT", "/v1/messages/read-all", nil, "✓ All messages marked as read")
		})
		return nil, true
	case "D":
		m.overlay = newConfirm("Delete ALL messages? This cannot be undone.", func() tea.Cmd {
			m.messages, m.detail = nil, false
			return m.mutate("DELETE", "/v1/messages/bulk", nil, "✓ All messages deleted", secInbox)
		})
		return nil, true
	}
	if !ok {
		return nil, false
	}
	switch k {
	case "r", "m":
		read := !x.IsRead
		for i := range m.messages {
			if m.messages[i].TID == x.TID {
				m.messages[i].IsRead = read
			}
		}
		delete(m.fresh, x.TID)
		return m.mutate("PUT", "/v1/messages/"+url.PathEscape(x.TID), map[string]bool{"is_read": read}, ""), true
	case "d", "delete":
		m.overlay = newConfirm(fmt.Sprintf("Delete %q?", truncate(x.Title, 40)), func() tea.Cmd {
			m.removeMessage(x.TID)
			return m.mutate("DELETE", "/v1/messages/"+url.PathEscape(x.TID), nil, "✓ Deleted")
		})
		return nil, true
	case "f":
		m.overlay = newCompose(m, &composeSeed{title: x.Title, message: x.Message, appName: x.Application, actions: x.Actions, level: &x.Priority})
		return textinput.Blink, true
	}
	return nil, false
}

func (m *uiModel) removeMessage(tid string) {
	for i, x := range m.messages {
		if x.TID == tid {
			m.messages = append(m.messages[:i], m.messages[i+1:]...)
			m.total--
			break
		}
	}
	m.clampCursor(secInbox)
	if m.rowCount() == 0 {
		m.detail = false
	}
	m.refreshDetail()
}

func (m *uiModel) scheduledKey(k string) (tea.Cmd, bool) {
	if k == "s" {
		i := indexOf(uiScheduledStatuses, m.schedStatus)
		m.schedStatus = uiScheduledStatuses[(i+1)%len(uiScheduledStatuses)]
		m.cursor[secScheduled] = 0
		return m.reload(secScheduled), true
	}
	x, ok := m.selectedScheduled()
	if !ok {
		return nil, false
	}
	switch k {
	case "d", "c", "delete":
		if x.Status != "pending" {
			return m.setStatus("Only pending messages can be canceled", true), true
		}
		m.overlay = newConfirm(fmt.Sprintf("Cancel %q scheduled for %s?", truncate(x.Title, 30), x.DeliverAt), func() tea.Cmd {
			return m.mutate("DELETE", "/v1/messages/scheduled/"+url.PathEscape(x.TID), nil, "✓ Canceled", secScheduled)
		})
		return nil, true
	case "f":
		m.overlay = newCompose(m, &composeSeed{title: x.Title, message: x.Message, appName: x.Application, actions: x.Actions, level: &x.Priority})
		return textinput.Blink, true
	}
	return nil, false
}

func (m *uiModel) appsKey(k string) (tea.Cmd, bool) {
	if k == "c" {
		m.overlay = newPrompt("New application name", "", func(name string) tea.Cmd {
			return m.mutate("POST", "/v1/applications", map[string]any{"name": name, "color": "#3B82F6"}, "✓ Created "+name, secApps)
		})
		return textinput.Blink, true
	}
	x, ok := m.selectedApp()
	if !ok {
		return nil, false
	}
	path := "/v1/applications/" + url.PathEscape(x.TID)
	switch k {
	case "e":
		m.overlay = newPrompt("Rename application", x.Name, func(name string) tea.Cmd {
			return m.mutate("PUT", path, map[string]any{"name": name}, "✓ Renamed", secApps)
		})
		return textinput.Blink, true
	case "v":
		cur := 0
		if x.DefaultLevel != nil {
			cur = *x.DefaultLevel
		}
		m.overlay = newLevelPicker("Default level for "+x.Name, cur, func(lvl int) tea.Cmd {
			return m.mutate("PUT", path, map[string]any{"default_level": lvl}, "✓ Default level: "+levelName(lvl), secApps)
		})
		return nil, true
	case "s":
		m.overlay = newCompose(m, &composeSeed{appTID: x.TID})
		return textinput.Blink, true
	case "d", "delete":
		m.overlay = newConfirm(fmt.Sprintf("Delete application %q?", x.Name), func() tea.Cmd {
			return m.mutate("DELETE", path, nil, "✓ Deleted "+x.Name, secApps)
		})
		return nil, true
	}
	return nil, false
}

func (m *uiModel) devicesKey(k string) (tea.Cmd, bool) {
	x, ok := m.selectedDevice()
	if !ok {
		return nil, false
	}
	path := "/v1/devices/" + url.PathEscape(x.TID)
	switch k {
	case "e":
		m.overlay = newPrompt("Rename device", x.Name, func(name string) tea.Cmd {
			return m.mutate("PUT", path, map[string]string{"name": name}, "✓ Renamed", secDevices)
		})
		return textinput.Blink, true
	case "s":
		m.overlay = newCompose(m, &composeSeed{deviceTID: x.TID})
		return textinput.Blink, true
	case "d", "delete":
		m.overlay = newConfirm(fmt.Sprintf("Remove device %q? It stops receiving notifications.", x.Name), func() tea.Cmd {
			return m.mutate("DELETE", path, nil, "✓ Removed "+x.Name, secDevices)
		})
		return nil, true
	}
	return nil, false
}

func (m *uiModel) hooksKey(k string) (tea.Cmd, bool) {
	x, ok := m.selectedHook()
	if !ok {
		return nil, false
	}
	path := "/v1/webhooks/" + url.PathEscape(x.TID)
	switch k {
	case "e":
		m.overlay = newPrompt("Rename webhook", x.Name, func(name string) tea.Cmd {
			return m.mutate("PUT", path, map[string]any{"name": name}, "✓ Renamed", secWebhooks)
		})
		return textinput.Blink, true
	case "t", "p":
		active := !hookActive(x)
		text := "✓ Paused " + x.Name
		if active {
			text = "✓ Resumed " + x.Name
		}
		return m.mutate("PUT", path, map[string]any{"is_active": active}, text, secWebhooks), true
	case "d", "delete":
		m.overlay = newConfirm(fmt.Sprintf("Delete webhook %q? Its URL stops working.", x.Name), func() tea.Cmd {
			m.detail = false
			return m.mutate("DELETE", path, nil, "✓ Deleted "+x.Name, secWebhooks)
		})
		return nil, true
	case "ctrl+r":
		if m.detail {
			return m.loadHistory(x.TID), true
		}
	}
	return nil, false
}

func hookActive(w api.Webhook) bool { return w.IsActive == nil || *w.IsActive }

// copySelected puts the selection's most useful value on the clipboard via
// OSC 52, which works over SSH and needs no external tools.
func (m *uiModel) copySelected() tea.Cmd {
	var value, what string
	switch m.sec {
	case secInbox:
		if x, ok := m.selectedMessage(); ok {
			value, what = x.Message, "message text"
		}
	case secScheduled:
		if x, ok := m.selectedScheduled(); ok {
			value, what = x.TID, "TID"
		}
	case secApps:
		if x, ok := m.selectedApp(); ok {
			value, what = x.TID, "application TID"
		}
	case secDevices:
		if x, ok := m.selectedDevice(); ok {
			value, what = x.TID, "device TID"
		}
	case secWebhooks:
		if x, ok := m.selectedHook(); ok {
			value, what = webhookBaseURL+x.URL, "webhook URL"
		}
	}
	if value == "" {
		return nil
	}
	fmt.Fprintf(os.Stderr, "\x1b]52;c;%s\a", base64.StdEncoding.EncodeToString([]byte(value)))
	return m.setStatus("Copied "+what, false)
}

// ---- selection ----

func (m *uiModel) filter(in []api.Message) []api.Message {
	if !m.unreadOnly && m.appFilter == "" && m.query == "" {
		return in
	}
	q := strings.ToLower(m.query)
	out := make([]api.Message, 0, len(in))
	for _, x := range in {
		if m.unreadOnly && x.IsRead && !m.fresh[x.TID] {
			continue
		}
		if m.appFilter != "" && x.Application != m.appFilter {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(x.Title+"\n"+x.Message+"\n"+x.Application), q) {
			continue
		}
		out = append(out, x)
	}
	return out
}

func matches(q string, fields ...string) bool {
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(strings.Join(fields, "\n")), strings.ToLower(q))
}

func (m *uiModel) visibleScheduled() []api.ScheduledMessage {
	var out []api.ScheduledMessage
	for _, x := range m.scheduled {
		if matches(m.query, x.Title, x.Message, x.Application) {
			out = append(out, x)
		}
	}
	return out
}

func (m *uiModel) visibleApps() []api.Application {
	var out []api.Application
	for _, x := range m.apps {
		if matches(m.query, x.Name, x.Description) {
			out = append(out, x)
		}
	}
	return out
}

func (m *uiModel) visibleDevices() []api.Device {
	var out []api.Device
	for _, x := range m.devices {
		if matches(m.query, x.Name, x.SerialNumber) {
			out = append(out, x)
		}
	}
	return out
}

func (m *uiModel) visibleHooks() []api.Webhook {
	var out []api.Webhook
	for _, x := range m.hooks {
		if matches(m.query, x.Name, x.URL, x.Message) {
			out = append(out, x)
		}
	}
	return out
}

func (m *uiModel) rowCount() int {
	switch m.sec {
	case secInbox:
		return len(m.filter(m.messages))
	case secScheduled:
		return len(m.visibleScheduled())
	case secApps:
		return len(m.visibleApps())
	case secDevices:
		return len(m.visibleDevices())
	case secWebhooks:
		return len(m.visibleHooks())
	}
	return 0
}

func (m *uiModel) clampCursor(s uiSection) {
	sec := m.sec
	m.sec = s
	n := m.rowCount()
	m.sec = sec
	if m.cursor[s] >= n {
		m.cursor[s] = max(0, n-1)
	}
}

func pick[T any](items []T, i int) (T, bool) {
	var zero T
	if i < 0 || i >= len(items) {
		return zero, false
	}
	return items[i], true
}

func (m *uiModel) selectedMessage() (api.Message, bool) {
	return pick(m.filter(m.messages), m.cursor[secInbox])
}

func (m *uiModel) selectedScheduled() (api.ScheduledMessage, bool) {
	return pick(m.visibleScheduled(), m.cursor[secScheduled])
}

func (m *uiModel) selectedApp() (api.Application, bool) {
	return pick(m.visibleApps(), m.cursor[secApps])
}

func (m *uiModel) selectedDevice() (api.Device, bool) {
	return pick(m.visibleDevices(), m.cursor[secDevices])
}

func (m *uiModel) selectedHook() (api.Webhook, bool) {
	return pick(m.visibleHooks(), m.cursor[secWebhooks])
}

func indexOf(list []string, v string) int {
	for i, s := range list {
		if s == v {
			return i
		}
	}
	return 0
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
