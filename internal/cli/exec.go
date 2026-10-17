package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

func (a *app) execCmd() *cobra.Command {
	var (
		o    messageOpts
		on   string
		tail int
	)
	cmd := &cobra.Command{
		Use:   "exec [flags] -- command [args...]",
		Short: "Run a command and get a push when it finishes",
		Long: `Run a command, stream its output as usual, then send a notification with
the exit status, duration and the last lines of output. The command's exit
code is passed through, so exec is safe to use in scripts and CI.`,
		Example: `  pocketalert exec -- make release
  pocketalert exec --on failure -l critical -- ./backup.sh
  pocketalert exec -t "Nightly import" --tail 20 -- python import.py`,
		Args:    cobra.MinimumNArgs(1),
		Aliases: []string{"run"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if on != "always" && on != "success" && on != "failure" {
				return fmt.Errorf("--on must be always, success or failure")
			}
			// Validate before running so a typo doesn't surface only after a
			// two-hour job.
			if o.level != "" {
				if _, err := parseLevel(o.level); err != nil {
					return err
				}
			}
			if _, err := parseActions(o.actions, false); err != nil {
				return err
			}

			lines := newTailBuffer(tail)
			child := exec.Command(args[0], args[1:]...)
			child.Stdin = a.stdin
			child.Stdout = io.MultiWriter(a.stdout, lines)
			child.Stderr = io.MultiWriter(a.stderr, lines)

			start := time.Now()
			runErr := child.Run()
			elapsed := time.Since(start).Round(time.Second)

			code := 0
			var exitErr *exec.ExitError
			switch {
			case runErr == nil:
			case errors.As(runErr, &exitErr):
				code = exitErr.ExitCode()
				if code < 0 {
					code = 1
				}
			default:
				fmt.Fprintln(a.stderr, "Error:", runErr)
				code = 127
			}

			failed := code != 0
			if (on == "success" && failed) || (on == "failure" && !failed) {
				return exitCode(code)
			}

			name := o.title
			if name == "" {
				name = truncate(strings.Join(args, " "), 60)
			}
			if failed {
				o.title = fmt.Sprintf("✗ %s (exit %d)", name, code)
				if o.level == "" {
					o.level = "high"
				}
			} else {
				o.title = "✓ " + name
			}
			host, _ := os.Hostname()
			o.message = fmt.Sprintf("Finished in %s on %s", elapsed, dash(host))
			if out := lines.String(); out != "" {
				o.message += "\n\n" + out
			}

			res, raw, err := a.sendMessage(cmd.Context(), &o)
			if err != nil {
				fmt.Fprintln(a.stderr, "pocketalert: notification not sent:", err)
				return exitCode(code)
			}
			// The confirmation goes to stderr: the wrapped command's stdout may
			// be piped somewhere.
			if !o.quiet {
				stdout := a.stdout
				a.stdout = a.stderr
				a.reportSent(&o, res, raw)
				a.stdout = stdout
			}
			return exitCode(code)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&o.title, "title", "t", "", "Name used in the notification title (default: the command)")
	f.StringVar(&on, "on", "always", "When to notify: always, success, failure")
	f.IntVar(&tail, "tail", 10, "Include the last N lines of output (0 to omit)")
	o.bindDelivery(f)
	f.SetInterspersed(false)
	return cmd
}

func exitCode(code int) error {
	if code == 0 {
		return nil
	}
	return exitError{code}
}

// tailBuffer keeps the last n lines written to it. stdout and stderr write
// concurrently, hence the mutex.
type tailBuffer struct {
	mu      sync.Mutex
	n       int
	lines   []string
	partial bytes.Buffer
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func newTailBuffer(n int) *tailBuffer { return &tailBuffer{n: n} }

func (t *tailBuffer) Write(p []byte) (int, error) {
	if t.n <= 0 {
		return len(p), nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.partial.Write(p)
	for {
		line, err := t.partial.ReadString('\n')
		if err != nil {
			t.partial.Reset()
			t.partial.WriteString(line)
			break
		}
		t.push(ansiEscape.ReplaceAllString(strings.TrimRight(line, "\r\n"), ""))
	}
	return len(p), nil
}

func (t *tailBuffer) push(line string) {
	t.lines = append(t.lines, line)
	if len(t.lines) > t.n {
		t.lines = t.lines[len(t.lines)-t.n:]
	}
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := t.lines
	if rest := strings.TrimSpace(ansiEscape.ReplaceAllString(t.partial.String(), "")); rest != "" {
		lines = append(append([]string{}, lines...), rest)
		if len(lines) > t.n {
			lines = lines[len(lines)-t.n:]
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
