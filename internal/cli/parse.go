package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PocketAlert/cli/internal/api"
)

var levelNames = map[int]string{-2: "silent", -1: "low", 0: "default", 1: "high", 2: "critical"}

var levelAliases = map[string]int{
	"silent": -2, "min": -2,
	"low":     -1,
	"default": 0, "normal": 0,
	"high":     1,
	"critical": 2, "max": 2, "urgent": 2,
}

const levelHelp = "silent, low, default, high, critical (or -2..2)"

func levelName(p int) string {
	if n, ok := levelNames[p]; ok {
		return n
	}
	return strconv.Itoa(p)
}

func levelPtrName(p *int) string {
	if p == nil {
		return ""
	}
	return levelName(*p)
}

// parseLevel validates locally so typos fail before a message is spent
// against the daily quota.
func parseLevel(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if n, ok := levelAliases[s]; ok {
		return n, nil
	}
	if n, err := strconv.Atoi(s); err == nil && n >= -2 && n <= 2 {
		return n, nil
	}
	return 0, fmt.Errorf("invalid level %q: use %s", s, levelHelp)
}

// parseAction turns "type:label:value" into an action button.
//
//	view:Open dashboard:https://status.example.com
//	copy:Copy code:123456
//	http:Restart:https://ci.example.com/restart          (POST)
//	http:Ack:GET https://example.com/ack
//	http:Deploy:{"url":"https://…","method":"PUT"}
//
// Webhook actions are templates: values containing %field.path% are only
// checked after substitution on the server.
func parseAction(s string, template bool) (api.Action, error) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[2]) == "" {
		return api.Action{}, fmt.Errorf("invalid action %q: expected type:label:value", s)
	}
	typ, label, value := strings.ToLower(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
	loose := template && strings.Contains(value, "%")

	switch typ {
	case "view":
		if !loose && !isHTTPURL(value) {
			return api.Action{}, fmt.Errorf("view action %q: value must be an http(s) URL", label)
		}
	case "copy":
	case "http":
		v, err := httpActionValue(value, loose)
		if err != nil {
			return api.Action{}, fmt.Errorf("http action %q: %w", label, err)
		}
		value = v
	default:
		return api.Action{}, fmt.Errorf("invalid action type %q: use view, http or copy", typ)
	}
	return api.Action{Type: typ, Label: label, Value: value}, nil
}

var httpMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true}

func httpActionValue(v string, loose bool) (string, error) {
	if strings.HasPrefix(v, "{") {
		var cfg struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal([]byte(v), &cfg); err != nil || (!loose && !isHTTPURL(cfg.URL)) {
			return "", fmt.Errorf("JSON value needs a valid http(s) \"url\"")
		}
		return v, nil
	}
	method, target := "POST", v
	if m, rest, ok := strings.Cut(v, " "); ok && httpMethods[strings.ToUpper(m)] {
		method, target = strings.ToUpper(m), strings.TrimSpace(rest)
	}
	if !loose && !isHTTPURL(target) {
		return "", fmt.Errorf("value must be [METHOD ]URL or a JSON object")
	}
	b, _ := json.Marshal(map[string]string{"url": target, "method": method})
	return string(b), nil
}

func parseActions(specs []string, template bool) ([]api.Action, error) {
	if len(specs) > 3 {
		return nil, fmt.Errorf("at most 3 actions are allowed, got %d", len(specs))
	}
	out := make([]api.Action, 0, len(specs))
	for _, s := range specs {
		a, err := parseAction(s, template)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

var dayPart = regexp.MustCompile(`^(\d+)d`)

// normalizeDelay accepts Go durations plus a leading day count ("1d",
// "2d12h") and converts days to hours, because the API only understands
// Go durations and bare seconds.
func normalizeDelay(s string) (string, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if _, err := strconv.ParseUint(s, 10, 64); err == nil {
		return s, nil
	}
	var total time.Duration
	if m := dayPart.FindStringSubmatch(s); m != nil {
		days, _ := strconv.Atoi(m[1])
		total = time.Duration(days) * 24 * time.Hour
		s = s[len(m[0]):]
	}
	if s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return "", fmt.Errorf("invalid delay: use 30m, 2h, 1d, 1d12h or seconds")
		}
		total += d
	}
	if total <= 0 {
		return "", fmt.Errorf("delay must be positive")
	}
	return total.String(), nil
}
