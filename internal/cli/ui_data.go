package cli

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/PocketAlert/cli/internal/api"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	uiPageSize     = 50
	uiPollEvery    = 15 * time.Second
	uiStatusFor    = 4 * time.Second
	uiHistorySize  = 15
	uiMaxFillPages = 20
)

type (
	uiErrMsg     struct{ err error }
	uiAccountMsg struct {
		profile api.Profile
		usage   api.MessageUsage
	}
	uiInboxMsg struct {
		page   api.MessagePage
		offset int
		poll   bool
	}
	uiScheduledMsg []api.ScheduledMessage
	uiAppsMsg      []api.Application
	uiDevicesMsg   []api.Device
	uiHooksMsg     []api.Webhook
	uiHistoryMsg   struct {
		tid     string
		history api.WebhookHistory
	}
	// uiDoneMsg reports a finished mutation; the listed sections are reloaded.
	uiDoneMsg struct {
		text   string
		reload []uiSection
	}
	uiSentMsg struct {
		opts *messageOpts
		res  *api.MessageCreated
		err  error
	}
	uiPollMsg        struct{}
	uiClearStatusMsg struct{ seq int }
)

func (m *uiModel) call(fn func(ctx context.Context) (tea.Msg, error)) tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		msg, err := fn(ctx)
		if err != nil {
			return uiErrMsg{err}
		}
		return msg
	}
}

func (m *uiModel) loadAccount() tea.Cmd {
	return m.call(func(ctx context.Context) (tea.Msg, error) {
		var out uiAccountMsg
		if _, err := m.a.client.Do(ctx, "GET", "/v1/auth/profile", nil, nil, &out.profile); err != nil {
			return nil, err
		}
		if _, err := m.a.client.Do(ctx, "GET", "/v1/messages/usage", nil, nil, &out.usage); err != nil {
			return nil, err
		}
		return out, nil
	})
}

func (m *uiModel) loadInbox(offset int, poll bool) tea.Cmd {
	q := url.Values{
		"size":   {strconv.Itoa(uiPageSize)},
		"offset": {strconv.Itoa(offset)},
		"sort":   {"desc"},
	}
	if m.deviceFilter != "" {
		q.Set("tid", m.deviceFilter)
	}
	if m.appFilterTID != "" {
		q.Set("application", m.appFilterTID)
	}
	return m.call(func(ctx context.Context) (tea.Msg, error) {
		var page api.MessagePage
		_, err := m.a.client.Do(ctx, "GET", "/v2/messages", q, nil, &page)
		for i := range page.Data {
			cleanMessage(&page.Data[i])
		}
		return uiInboxMsg{page: page, offset: offset, poll: poll}, err
	})
}

func (m *uiModel) loadScheduled() tea.Cmd {
	q := url.Values{"status": {m.schedStatus}}
	return m.call(func(ctx context.Context) (tea.Msg, error) {
		var items []api.ScheduledMessage
		_, err := m.a.client.Do(ctx, "GET", "/v1/messages/scheduled", q, nil, &items)
		for i := range items {
			cleanScheduled(&items[i])
		}
		return uiScheduledMsg(items), err
	})
}

func (m *uiModel) loadApps() tea.Cmd {
	return m.call(func(ctx context.Context) (tea.Msg, error) {
		var items []api.Application
		_, err := m.a.client.Do(ctx, "GET", "/v1/applications", nil, nil, &items)
		for i := range items {
			items[i].Name, items[i].Description = clean(items[i].Name), clean(items[i].Description)
		}
		return uiAppsMsg(items), err
	})
}

func (m *uiModel) loadDevices() tea.Cmd {
	return m.call(func(ctx context.Context) (tea.Msg, error) {
		var items []api.Device
		_, err := m.a.client.Do(ctx, "GET", "/v1/devices", nil, nil, &items)
		for i := range items {
			items[i].Name, items[i].SerialNumber = clean(items[i].Name), clean(items[i].SerialNumber)
		}
		return uiDevicesMsg(items), err
	})
}

func (m *uiModel) loadHooks() tea.Cmd {
	return m.call(func(ctx context.Context) (tea.Msg, error) {
		var items []api.Webhook
		_, err := m.a.client.Do(ctx, "GET", "/v1/webhooks", nil, nil, &items)
		for i := range items {
			x := &items[i]
			x.Name, x.URL, x.Message, x.Actions = clean(x.Name), clean(x.URL), clean(x.Message), clean(x.Actions)
		}
		return uiHooksMsg(items), err
	})
}

func (m *uiModel) loadHistory(tid string) tea.Cmd {
	q := url.Values{"per": {strconv.Itoa(uiHistorySize)}}
	return m.call(func(ctx context.Context) (tea.Msg, error) {
		var h api.WebhookHistory
		_, err := m.a.client.Do(ctx, "GET", "/v1/webhooks/"+url.PathEscape(tid)+"/history", q, nil, &h)
		for i := range h.Requests {
			r := &h.Requests[i]
			r.RenderedMessage, r.ErrorCode, r.Method, r.Status = clean(r.RenderedMessage), clean(r.ErrorCode), clean(r.Method), clean(r.Status)
		}
		return uiHistoryMsg{tid: tid, history: h}, err
	})
}

func (m *uiModel) load(s uiSection) tea.Cmd {
	switch s {
	case secInbox:
		return m.loadInbox(0, false)
	case secScheduled:
		return m.loadScheduled()
	case secApps:
		return m.loadApps()
	case secDevices:
		return m.loadDevices()
	case secWebhooks:
		return m.loadHooks()
	}
	return nil
}

// mutate runs a write request and, on success, reports text and reloads the
// given sections.
func (m *uiModel) mutate(method, path string, body any, text string, reload ...uiSection) tea.Cmd {
	return m.call(func(ctx context.Context) (tea.Msg, error) {
		_, err := m.a.client.Do(ctx, method, path, nil, body, nil)
		return uiDoneMsg{text: text, reload: reload}, err
	})
}

func (m *uiModel) send(o *messageOpts) tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		res, _, err := m.a.sendMessage(ctx, o)
		return uiSentMsg{opts: o, res: res, err: err}
	}
}

func uiPoll() tea.Cmd {
	return tea.Tick(uiPollEvery, func(time.Time) tea.Msg { return uiPollMsg{} })
}

// clean drops control characters (escape sequences included) from text that
// came from third parties, so a message cannot retitle the terminal, write
// the clipboard or redraw the screen. Newlines and tabs survive.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return -1
		}
		return r
	}, s)
}

func cleanActions(list []api.Action) {
	for i := range list {
		list[i].Label, list[i].Value = clean(list[i].Label), clean(list[i].Value)
	}
}

func cleanMessage(x *api.Message) {
	x.Title, x.Message, x.Application, x.Device = clean(x.Title), clean(x.Message), clean(x.Application), clean(x.Device)
	cleanActions(x.Actions)
	if x.Attachment != nil {
		x.Attachment.Link = clean(x.Attachment.Link)
	}
}

func cleanScheduled(x *api.ScheduledMessage) {
	x.Title, x.Message, x.Application, x.Device = clean(x.Title), clean(x.Message), clean(x.Application), clean(x.Device)
	cleanActions(x.Actions)
}
