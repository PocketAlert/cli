package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type recorded struct {
	Method, Path, Query, Token, ContentType string
	Body                                    []byte
}

type fakeAPI struct {
	mu       sync.Mutex
	requests []recorded
	routes   map[string]func(w http.ResponseWriter, r *http.Request)
}

func newFakeAPI(t *testing.T) (*fakeAPI, *httptest.Server) {
	f := &fakeAPI{routes: map[string]func(http.ResponseWriter, *http.Request){}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.requests = append(f.requests, recorded{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Token"), r.Header.Get("Content-Type"), body})
		h := f.routes[r.Method+" "+r.URL.Path]
		f.mu.Unlock()
		r.Body = io.NopCloser(bytes.NewReader(body))
		if h == nil {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":"not found"}`)
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("POCKETALERT_API_URL", srv.URL)
	t.Setenv("POCKETALERT_API_KEY", "test-key-1234567890")
	t.Setenv("POCKETALERT_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	return f, srv
}

func (f *fakeAPI) on(route string, status int, body string) {
	f.routes[route] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}
}

func (f *fakeAPI) last() recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func run(t *testing.T, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	a := &app{build: BuildInfo{Version: "test"}, stdout: &out, stderr: &errOut, stdin: strings.NewReader(stdin)}
	root := a.rootCmd()
	root.SetArgs(args)
	root.SetOut(&out)
	root.SetErr(&errOut)
	err = root.ExecuteContext(context.Background())
	return out.String(), errOut.String(), err
}

func TestSendJSONBody(t *testing.T) {
	f, _ := newFakeAPI(t)
	f.on("POST /v1/messages", 200, `{"tid":"m1","title":"T","message":"M","priority":1,"actions":[{"type":"view","label":"Open","value":"https://e.com"}],"created_at":"01.01.2026 10:00:00"}`)

	out, _, err := run(t, "", "send", "T", "M", "-l", "high", "-a", "app1", "-d", "all",
		"--in", "1d", "--action", "view:Open:https://e.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "m1") {
		t.Errorf("output lacks tid: %s", out)
	}
	req := f.last()
	if req.Token != "test-key-1234567890" || req.ContentType != "application/json" {
		t.Errorf("headers: %+v", req)
	}
	var body map[string]any
	_ = json.Unmarshal(req.Body, &body)
	want := map[string]any{"title": "T", "message": "M", "level": float64(1), "application_id": "app1", "device_id": "all", "delay": "24h0m0s"}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("body[%s] = %v, want %v", k, body[k], v)
		}
	}
	if acts, _ := body["actions"].([]any); len(acts) != 1 {
		t.Errorf("actions = %v", body["actions"])
	}
}

func TestSendReadsStdinAndQuiet(t *testing.T) {
	f, _ := newFakeAPI(t)
	f.on("POST /v1/messages", 200, `{"tid":"m2","priority":0}`)
	out, _, err := run(t, "line1\nline2\n", "send", "Report", "-q")
	if err != nil {
		t.Fatal(err)
	}
	if out != "m2\n" {
		t.Errorf("quiet output = %q", out)
	}
	var body map[string]any
	_ = json.Unmarshal(f.last().Body, &body)
	if body["message"] != "line1\nline2" {
		t.Errorf("message = %q", body["message"])
	}
}

func TestSendScheduledAndDowngradeNote(t *testing.T) {
	f, _ := newFakeAPI(t)
	f.on("POST /v1/messages", 200, `{"tid":"s1","scheduled":true,"status":"pending","priority":1,"deliver_at":"20.10.2026 09:00:00","tz":"Europe/Berlin"}`)
	out, errOut, err := run(t, "", "send", "T", "M", "--at", "2026-10-20 09:00", "-l", "critical")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Scheduled for 20.10.2026 09:00:00 (Europe/Berlin)") || !strings.Contains(out, "scheduled cancel s1") {
		t.Errorf("output = %s", out)
	}
	if !strings.Contains(errOut, `"critical" needs a paid plan`) {
		t.Errorf("stderr = %s", errOut)
	}
}

func TestSendValidatesLocally(t *testing.T) {
	f, _ := newFakeAPI(t)
	for _, args := range [][]string{
		{"send", "T", "M", "-l", "loud"},
		{"send", "T", "M", "--action", "open:X:https://e.com"},
		{"send", "T", "M", "--at", "x", "--in", "1h"},
		{"send", "T"},
	} {
		if _, _, err := run(t, "", args...); err == nil {
			t.Errorf("%v should fail", args)
		}
	}
	if len(f.requests) != 0 {
		t.Errorf("invalid input reached the API: %d requests", len(f.requests))
	}
}

func TestSendImageMultipart(t *testing.T) {
	f, _ := newFakeAPI(t)
	f.on("POST /v1/messages", 200, `{"tid":"m3","priority":0}`)
	img := filepath.Join(t.TempDir(), "shot.png")
	_ = os.WriteFile(img, []byte("\x89PNG fake"), 0o600)

	if _, _, err := run(t, "", "send", "T", "M", "--image", img, "-l", "low", "--action", "copy:Code:42"); err != nil {
		t.Fatal(err)
	}
	req := f.last()
	if !strings.HasPrefix(req.ContentType, "multipart/form-data") {
		t.Fatalf("content type = %s", req.ContentType)
	}
	for _, s := range []string{`name="title"`, `name="level"`, "-1", `name="actions"`, `"copy"`, `filename="shot.png"`, "PNG fake"} {
		if !bytes.Contains(req.Body, []byte(s)) {
			t.Errorf("multipart body lacks %q", s)
		}
	}
}

func TestDailyLimitError(t *testing.T) {
	f, _ := newFakeAPI(t)
	f.on("POST /v1/messages", 429, `{"error":"daily_limit","limit":50,"used":50,"resets_at":"2026-10-18T00:00:00Z","upgrade_url":"https://pocketalert.app/pricing"}`)
	_, _, err := run(t, "", "send", "T", "M")
	if err == nil || !strings.Contains(err.Error(), "daily message limit reached (50/50)") || !strings.Contains(err.Error(), "pricing") {
		t.Errorf("err = %v", err)
	}
}

func TestExecPropagatesExitCodeAndNotifies(t *testing.T) {
	f, _ := newFakeAPI(t)
	f.on("POST /v1/messages", 200, `{"tid":"e1","priority":1}`)

	out, errOut, err := run(t, "", "exec", "--", "sh", "-c", "echo one; echo two; echo three >&2; exit 3")
	var ee exitError
	if !errors.As(err, &ee) || ee.code != 3 {
		t.Fatalf("err = %v, want exit 3", err)
	}
	if !strings.Contains(out, "one\ntwo") || !strings.Contains(errOut, "three") {
		t.Errorf("output not streamed: out=%q err=%q", out, errOut)
	}
	if strings.Contains(out, "✓ Sent") {
		t.Errorf("confirmation leaked into stdout: %q", out)
	}
	var body map[string]any
	_ = json.Unmarshal(f.last().Body, &body)
	title, _ := body["title"].(string)
	msg, _ := body["message"].(string)
	if !strings.HasPrefix(title, "✗ sh -c") || !strings.Contains(title, "(exit 3)") {
		t.Errorf("title = %q", title)
	}
	if body["level"] != float64(1) {
		t.Errorf("failure level = %v, want high", body["level"])
	}
	for _, line := range []string{"one", "two", "three"} {
		if !strings.Contains(msg, line) {
			t.Errorf("message lacks %q: %q", line, msg)
		}
	}
}

func TestExecOnFailureSkipsSuccess(t *testing.T) {
	f, _ := newFakeAPI(t)
	if _, _, err := run(t, "", "exec", "--on", "failure", "--", "true"); err != nil {
		t.Fatal(err)
	}
	if len(f.requests) != 0 {
		t.Errorf("notified on success")
	}
}

func TestWebhookCreateUsesCurrentFieldNames(t *testing.T) {
	f, _ := newFakeAPI(t)
	f.on("POST /v1/webhooks", 200, `{"tid":"w1","name":"G","url":"abc"}`)
	out, _, err := run(t, "", "webhooks", "create", "-n", "G", "-a", "app1", "-d", "dev1", "-l", "high", "--action", "view:Open:https://e.com/%id%")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "https://p4a.me/wh/abc") {
		t.Errorf("output = %s", out)
	}
	var body map[string]any
	_ = json.Unmarshal(f.last().Body, &body)
	if body["application_id"] != "app1" || body["device_id"] != "dev1" || body["message"] != "*" || body["default_level"] != float64(1) {
		t.Errorf("body = %v", body)
	}
	if s, _ := body["actions"].(string); !strings.Contains(s, "%id%") {
		t.Errorf("actions = %v", body["actions"])
	}
}

func TestUpdateSendsOnlyChangedFields(t *testing.T) {
	f, _ := newFakeAPI(t)
	f.on("PUT /v1/applications/a1", 200, `{"data":"success"}`)
	if _, _, err := run(t, "", "apps", "update", "a1", "--level", "silent"); err != nil {
		t.Fatal(err)
	}
	if got := string(f.last().Body); got != `{"default_level":-2}` {
		t.Errorf("body = %s", got)
	}
	if _, _, err := run(t, "", "apps", "update", "a1"); err == nil {
		t.Error("empty update should fail")
	}
}

func TestMessagesListAndJSON(t *testing.T) {
	f, _ := newFakeAPI(t)
	f.on("GET /v2/messages", 200, `{"data":[{"tid":"m1","title":"Привет мир","priority":2,"was_scheduled":true,"created_at":"x"}],"meta":{"total":5,"has_more":true,"next_offset":1}}`)
	out, errOut, err := run(t, "", "messages", "list", "-n", "1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "critical") || !strings.Contains(out, "⏱ Привет мир") || !strings.Contains(errOut, "--offset 1") {
		t.Errorf("out=%s err=%s", out, errOut)
	}
	if q := f.last().Query; !strings.Contains(q, "size=1") {
		t.Errorf("query = %s", q)
	}
	out, _, _ = run(t, "", "messages", "list", "--json")
	var page map[string]any
	if json.Unmarshal([]byte(out), &page) != nil || page["meta"] == nil {
		t.Errorf("--json output is not the raw response: %s", out)
	}
}

func TestDeleteAllNeedsConfirmation(t *testing.T) {
	f, _ := newFakeAPI(t)
	f.on("DELETE /v1/messages/bulk", 200, `{"message":"ok"}`)
	if _, _, err := run(t, "", "messages", "delete", "--all"); err == nil {
		t.Error("--all without --yes in a script should fail")
	}
	if len(f.requests) != 0 {
		t.Fatal("bulk delete sent without confirmation")
	}
	if _, _, err := run(t, "", "messages", "delete", "--all", "--yes"); err != nil {
		t.Fatal(err)
	}
}

func TestAuthVerifiesBeforeSaving(t *testing.T) {
	f, _ := newFakeAPI(t)
	t.Setenv("POCKETALERT_API_KEY", "")
	f.routes["GET /v1/auth/profile"] = func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") != "good-key-123456" {
			w.WriteHeader(401)
			io.WriteString(w, `{"error":"Unauthorized"}`)
			return
		}
		io.WriteString(w, `{"email":"me@example.com"}`)
	}
	if _, _, err := run(t, "bad-key\n", "auth"); err == nil {
		t.Fatal("bad key accepted")
	}
	if _, err := os.Stat(os.Getenv("POCKETALERT_CONFIG")); err == nil {
		t.Fatal("bad key was saved")
	}
	out, _, err := run(t, "good-key-123456\n", "auth")
	if err != nil || !strings.Contains(out, "me@example.com") {
		t.Fatalf("auth: %v %s", err, out)
	}
	data, _ := os.ReadFile(os.Getenv("POCKETALERT_CONFIG"))
	if !strings.Contains(string(data), "good-key-123456") {
		t.Errorf("config = %s", data)
	}
}

func TestTailBufferKeepsLastLines(t *testing.T) {
	b := newTailBuffer(2)
	io.WriteString(b, "a\nb\n\x1b[31mc\x1b[0m\npart")
	if got := b.String(); got != "c\npart" {
		t.Errorf("tail = %q", got)
	}
}

func TestWatchBatchesLines(t *testing.T) {
	f, _ := newFakeAPI(t)
	f.on("POST /v1/messages", 200, `{"tid":"w1","priority":0}`)
	_, _, err := run(t, "err a\n\nerr b\nerr c\n", "watch", "-t", "errors", "--max-lines", "2", "--batch", "1h", "-q")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.requests) != 1 {
		t.Fatalf("pushes = %d, want 1", len(f.requests))
	}
	var body map[string]any
	_ = json.Unmarshal(f.last().Body, &body)
	if body["title"] != "errors (3 lines)" || body["message"] != "err a\nerr b\n… and 1 more" {
		t.Errorf("body = %v", body)
	}
}
