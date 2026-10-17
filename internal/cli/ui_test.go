package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PocketAlert/cli/internal/api"
	"github.com/PocketAlert/cli/internal/config"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	uiTestInbox = `{"data":[
		{"tid":"m1","title":"Deploy failed","message":"main@4f2c broke","application":"Prod","device":"All devices","is_read":false,"priority":2,"created_at":"17.10.2026 10:00:00"},
		{"tid":"m2","title":"Backup done","message":"42 GB","application":"Ops","device":"Phone","is_read":true,"priority":0,"created_at":"16.10.2026 03:00:00"}
	],"meta":{"total":2,"has_more":false,"next_offset":2}}`
	uiTestApps    = `[{"tid":"a1","name":"Prod","color":"#EF4444","is_active":true},{"tid":"a2","name":"Ops","color":"#3B82F6","is_active":true}]`
	uiTestDevices = `[{"tid":"d1","name":"Phone","is_active":true}]`
)

func newUITest(t *testing.T) (*fakeAPI, *uiModel) {
	t.Helper()
	f, _ := newFakeAPI(t)
	f.on("GET /v1/auth/profile", 200, `{"email":"me@example.com"}`)
	f.on("GET /v1/messages/usage", 200, `{"used":3,"limit":50,"resets_at":"tomorrow"}`)
	f.on("GET /v2/messages", 200, uiTestInbox)
	f.on("GET /v1/applications", 200, uiTestApps)
	f.on("GET /v1/devices", 200, uiTestDevices)
	cfg, _, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a := &app{cfg: cfg, client: api.New(cfg.BaseURL, cfg.APIKey, "test"), stdout: io.Discard, stderr: io.Discard}
	m := newUIModel(context.Background(), a)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	pump(m, m.Init())
	return f, m
}

// pump runs commands and feeds their messages back like the Bubble Tea
// runtime would. Timers (status expiry, polling, blinking) never fire within
// the short wait, so they are dropped.
func pump(m *uiModel, cmd tea.Cmd) {
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0 && steps < 200; steps++ {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		ch := make(chan tea.Msg, 1)
		go func() { ch <- c() }()
		select {
		case msg := <-ch:
			switch v := msg.(type) {
			case nil, spinner.TickMsg:
			case tea.BatchMsg:
				queue = append(queue, v...)
			default:
				_, next := m.Update(msg)
				queue = append(queue, next)
			}
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func press(m *uiModel, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "ctrl+s":
			msg = tea.KeyMsg{Type: tea.KeyCtrlS}
		case "ctrl+r":
			msg = tea.KeyMsg{Type: tea.KeyCtrlR}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		_, cmd := m.Update(msg)
		pump(m, cmd)
	}
}

func (f *fakeAPI) find(method, path string) (recorded, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.requests) - 1; i >= 0; i-- {
		if r := f.requests[i]; r.Method == method && r.Path == path {
			return r, true
		}
	}
	return recorded{}, false
}

func TestUIInboxLoads(t *testing.T) {
	_, m := newUITest(t)
	view := m.View()
	for _, want := range []string{"Deploy failed", "Backup done", "Inbox (1)", "3/50 today", "me@example.com", "CRIT"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
}

func TestUIOpenMarksRead(t *testing.T) {
	f, m := newUITest(t)
	press(m, "enter")
	if !m.detail {
		t.Fatal("detail view not open")
	}
	if !strings.Contains(m.View(), "main@4f2c broke") {
		t.Errorf("detail lacks message body:\n%s", m.View())
	}
	req, ok := f.find("PUT", "/v1/messages/m1")
	if !ok || !strings.Contains(string(req.Body), `"is_read":true`) {
		t.Errorf("message not marked read: %+v", req)
	}
	if m.unreadCount() != 0 {
		t.Error("unread count not updated locally")
	}
	press(m, "esc")
	if m.detail {
		t.Error("esc did not close detail")
	}
}

func TestUICompose(t *testing.T) {
	f, m := newUITest(t)
	f.on("POST /v1/messages", 200, `{"tid":"m9","priority":1,"scheduled":true,"deliver_at":"2026-10-17 12:00","tz":"Europe/Moscow"}`)
	f.on("GET /v1/messages/scheduled", 200, `[]`)

	press(m, "n")
	c, ok := m.overlay.(*uiCompose)
	if !ok {
		t.Fatal("compose did not open")
	}
	press(m, "H", "i", "tab", "B", "o", "d", "y", "tab", "l", "tab", "tab", "l", "l", "l", "l", "tab", "2", "h")
	if c.app != 1 || c.level != 4 {
		t.Fatalf("app=%d level=%d", c.app, c.level)
	}
	press(m, "ctrl+s")
	if m.overlay != nil {
		t.Fatalf("compose still open, err=%q", c.err)
	}
	req, ok := f.find("POST", "/v1/messages")
	if !ok {
		t.Fatal("no POST /v1/messages")
	}
	var body map[string]any
	_ = json.Unmarshal(req.Body, &body)
	want := map[string]any{"title": "Hi", "message": "Body", "application_id": "a1", "level": float64(1), "delay": "2h0m0s"}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("body[%s] = %v, want %v", k, body[k], v)
		}
	}
	if !strings.Contains(m.status, "Scheduled for 2026-10-17 12:00") {
		t.Errorf("status = %q", m.status)
	}
	if _, ok := f.find("GET", "/v1/messages/scheduled"); !ok {
		t.Error("scheduled list not reloaded")
	}
}

func TestUIComposeShowsAPIError(t *testing.T) {
	f, m := newUITest(t)
	f.on("POST /v1/messages", 429, `{"error":"limit","limit":50,"used":50,"resets_at":"00:00"}`)
	press(m, "n", "T", "tab", "M", "ctrl+s")
	c, ok := m.overlay.(*uiCompose)
	if !ok || !strings.Contains(c.err, "daily message limit") {
		t.Fatalf("expected limit error in form, got overlay=%T", m.overlay)
	}
}

func TestUIDeleteConfirm(t *testing.T) {
	f, m := newUITest(t)
	f.on("DELETE /v1/messages/m2", 200, `{}`)
	press(m, "down", "d")
	if _, ok := m.overlay.(*uiConfirm); !ok {
		t.Fatal("no confirmation")
	}
	press(m, "y")
	if _, ok := f.find("DELETE", "/v1/messages/m2"); !ok {
		t.Error("DELETE not sent")
	}
	if len(m.messages) != 1 || m.messages[0].TID != "m1" {
		t.Errorf("messages = %+v", m.messages)
	}
}

func TestUIAppFilterAndSearch(t *testing.T) {
	f, m := newUITest(t)
	press(m, "3", "down", "enter")
	if m.sec != secInbox || m.appFilter != "Ops" {
		t.Fatalf("sec=%d filter=%q", m.sec, m.appFilter)
	}
	if req, _ := f.find("GET", "/v2/messages"); !strings.Contains(req.Query, "application=a2") {
		t.Errorf("app filter not sent to the API: %s", req.Query)
	}
	if got := m.filter(m.messages); len(got) != 1 || got[0].TID != "m2" {
		t.Errorf("filtered = %+v", got)
	}
	press(m, "esc")
	press(m, "/", "d", "e", "p", "enter")
	if got := m.filter(m.messages); len(got) != 1 || got[0].TID != "m1" {
		t.Errorf("search = %+v", got)
	}
}

func TestUIPollAddsNew(t *testing.T) {
	f, m := newUITest(t)
	press(m, "down")
	f.on("GET /v2/messages", 200, `{"data":[{"tid":"m3","title":"New one","is_read":false,"created_at":"17.10.2026 11:00:00"},
		{"tid":"m1","title":"Deploy failed","is_read":false}],"meta":{"total":3}}`)
	pump(m, func() tea.Msg { return uiPollMsg{} })
	if len(m.messages) != 3 || m.messages[0].TID != "m3" || !m.fresh["m3"] {
		t.Fatalf("messages = %+v", m.messages)
	}
	if sel, _ := m.selectedMessage(); sel.TID != "m2" {
		t.Errorf("cursor moved off the selected message to %s", sel.TID)
	}
	if !strings.Contains(m.status, "1 new message") {
		t.Errorf("status = %q", m.status)
	}
}

// An API without the application filter returns everything; the UI keeps
// paging until the filtered list fills the screen.
func TestUIFilterFillsPages(t *testing.T) {
	f, m := newUITest(t)
	f.routes["GET /v2/messages"] = func(w http.ResponseWriter, r *http.Request) {
		app := "Other"
		if r.URL.Query().Get("offset") == "4" {
			app = "Ops"
		}
		io.WriteString(w, `{"data":[{"tid":"x`+r.URL.Query().Get("offset")+`","title":"t","application":"`+app+`"},{"tid":"y`+r.URL.Query().Get("offset")+`","title":"t","application":"Other"}],"meta":{"total":100,"has_more":true}}`)
	}
	press(m, "3", "down", "enter")
	if got := m.filter(m.messages); len(got) == 0 || got[0].TID != "x4" {
		t.Fatalf("filtered = %+v (loaded %d)", got, len(m.messages))
	}
	if m.fillPages > uiMaxFillPages {
		t.Errorf("fillPages = %d", m.fillPages)
	}
}

func TestUIStripsEscapes(t *testing.T) {
	f, m := newUITest(t)
	f.on("GET /v2/messages", 200, `{"data":[{"tid":"e1","title":"hi\u001b]0;pwned\u0007","message":"a\u001b]52;c;Zm9v\u0007\nb","is_read":true}],"meta":{"total":1}}`)
	press(m, "ctrl+r")
	x := m.messages[0]
	if strings.ContainsRune(x.Title+x.Message, 0x1b) || strings.ContainsRune(x.Title, 0x07) {
		t.Errorf("escape survived: %q %q", x.Title, x.Message)
	}
	if !strings.Contains(x.Message, "\nb") && !strings.Contains(x.Message, "\n") {
		t.Errorf("newline dropped: %q", x.Message)
	}
}

func TestSplitWhen(t *testing.T) {
	cases := map[string][2]string{
		"":                 {"", ""},
		"now":              {"", ""},
		"30m":              {"", "30m"},
		"1d12h":            {"", "1d12h"},
		"90":               {"", "90"},
		"1760000000":       {"1760000000", ""},
		"2026-10-20 09:00": {"2026-10-20 09:00", ""},
	}
	for in, want := range cases {
		at, delay := splitWhen(in)
		if at != want[0] || delay != want[1] {
			t.Errorf("splitWhen(%q) = %q, %q; want %v", in, at, delay, want)
		}
	}
}
