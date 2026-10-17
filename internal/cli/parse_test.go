package cli

import "testing"

func TestParseLevel(t *testing.T) {
	cases := map[string]int{"silent": -2, "MIN": -2, "low": -1, "normal": 0, "high": 1, "urgent": 2, "-2": -2, "2": 2}
	for in, want := range cases {
		got, err := parseLevel(in)
		if err != nil || got != want {
			t.Errorf("parseLevel(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"3", "loud", ""} {
		if _, err := parseLevel(in); err == nil {
			t.Errorf("parseLevel(%q) should fail", in)
		}
	}
}

func TestParseAction(t *testing.T) {
	cases := []struct{ in, typ, label, value string }{
		{"view:Open:https://example.com/a?b=c", "view", "Open", "https://example.com/a?b=c"},
		{"copy:Code:12:34", "copy", "Code", "12:34"},
		{"http:Restart:https://ci.example.com/r", "http", "Restart", `{"method":"POST","url":"https://ci.example.com/r"}`},
		{"http:Ack:get https://e.com", "http", "Ack", `{"method":"GET","url":"https://e.com"}`},
		{`http:Raw:{"url":"https://e.com","method":"PUT"}`, "http", "Raw", `{"url":"https://e.com","method":"PUT"}`},
	}
	for _, c := range cases {
		a, err := parseAction(c.in, false)
		if err != nil {
			t.Fatalf("parseAction(%q): %v", c.in, err)
		}
		if a.Type != c.typ || a.Label != c.label || a.Value != c.value {
			t.Errorf("parseAction(%q) = %+v", c.in, a)
		}
	}
	for _, in := range []string{"view:Open:ftp://x", "open:X:https://e.com", "view::https://e.com", "copy:X", "http:X:not a url"} {
		if _, err := parseAction(in, false); err == nil {
			t.Errorf("parseAction(%q) should fail", in)
		}
	}
}

func TestNormalizeDelay(t *testing.T) {
	cases := map[string]string{"30m": "30m0s", "90": "90", "1d": "24h0m0s", "2d12h": "60h0m0s", "1h30m": "1h30m0s"}
	for in, want := range cases {
		got, err := normalizeDelay(in)
		if err != nil || got != want {
			t.Errorf("normalizeDelay(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "-5m", "soon", "1w"} {
		if _, err := normalizeDelay(in); err == nil {
			t.Errorf("normalizeDelay(%q) should fail", in)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("Привет, мир", 7); got != "Привет…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("a\nb", 10); got != "a b" {
		t.Errorf("truncate = %q", got)
	}
}
