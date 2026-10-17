package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"golang.org/x/term"
)

// emitJSON pretty-prints a raw API response. It reports whether --json is on,
// so commands can return right after it.
func (a *app) emitJSON(raw []byte) bool {
	if !a.json {
		return false
	}
	var buf bytes.Buffer
	if json.Indent(&buf, raw, "", "  ") != nil {
		buf.Reset()
		buf.Write(raw)
	}
	fmt.Fprintln(a.stdout, buf.String())
	return true
}

func (a *app) ok(format string, args ...any) {
	fmt.Fprintf(a.stdout, "✓ "+format+"\n", args...)
}

type table struct{ w *tabwriter.Writer }

func (a *app) table(headers ...string) *table {
	t := &table{w: tabwriter.NewWriter(a.stdout, 0, 0, 2, ' ', 0)}
	fmt.Fprintln(t.w, strings.Join(headers, "\t"))
	return t
}

func (t *table) row(cols ...any) {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = strings.ReplaceAll(fmt.Sprint(c), "\t", " ")
	}
	fmt.Fprintln(t.w, strings.Join(parts, "\t"))
}

func (t *table) flush() { t.w.Flush() }

// details prints aligned "Key: value" pairs, skipping empty values.
func (a *app) details(pairs ...string) {
	w := tabwriter.NewWriter(a.stdout, 0, 0, 2, ' ', 0)
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] == "" {
			continue
		}
		fmt.Fprintf(w, "%s:\t%s\n", pairs[i], pairs[i+1])
	}
	w.Flush()
}

func check(b bool) string {
	if b {
		return "✓"
	}
	return "·"
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// truncate shortens by runes so multi-byte titles are never cut mid-character.
func truncate(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

func isTerminal(f any) bool {
	file, ok := f.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

// confirm asks a yes/no question; without a terminal it refuses so scripts
// must pass --yes explicitly.
func (a *app) confirm(question string) (bool, error) {
	if !isTerminal(a.stdin) {
		return false, fmt.Errorf("refusing to continue without confirmation: pass --yes")
	}
	fmt.Fprintf(a.stderr, "%s [y/N]: ", question)
	line, err := bufio.NewReader(a.stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
