package cli

import (
	"bufio"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func (a *app) watchCmd() *cobra.Command {
	var (
		o        messageOpts
		window   time.Duration
		maxLines int
	)
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Send a push for every line piped in (batched)",
		Long: `Read lines from stdin and send them as notifications. Lines that arrive
within --batch of each other are grouped into one push, so a burst of log
errors costs one message from your daily quota instead of hundreds.`,
		Example: `  tail -f app.log | grep --line-buffered ERROR | pocketalert watch -t "app errors"
  journalctl -fu nginx | pocketalert watch --batch 1m -l high`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if isTerminal(a.stdin) {
				return fmt.Errorf("pipe something in, e.g. tail -f app.log | pocketalert watch")
			}
			if maxLines < 1 {
				return fmt.Errorf("--max-lines must be at least 1")
			}
			if o.level != "" {
				if _, err := parseLevel(o.level); err != nil {
					return err
				}
			}
			if _, err := parseActions(o.actions, false); err != nil {
				return err
			}
			title := o.title
			if title == "" {
				title = "New output"
			}

			lines := make(chan string)
			go func() {
				defer close(lines)
				sc := bufio.NewScanner(a.stdin)
				sc.Buffer(make([]byte, 64*1024), 1<<20)
				for sc.Scan() {
					if l := strings.TrimSpace(ansiEscape.ReplaceAllString(sc.Text(), "")); l != "" {
						lines <- l
					}
				}
			}()

			var (
				batch   []string
				dropped int
				timer   <-chan time.Time
			)
			flush := func() {
				if len(batch) == 0 {
					return
				}
				msg := strings.Join(batch, "\n")
				if dropped > 0 {
					msg += fmt.Sprintf("\n… and %d more", dropped)
				}
				o.title = title
				if n := len(batch) + dropped; n > 1 {
					o.title = fmt.Sprintf("%s (%d lines)", title, n)
				}
				o.message = msg
				res, raw, err := a.sendMessage(cmd.Context(), &o)
				if err != nil {
					// Keep watching: one failed push (e.g. a network blip)
					// shouldn't end a long-running tail.
					fmt.Fprintln(a.stderr, "pocketalert: push failed:", err)
				} else {
					a.reportSent(&o, res, raw)
				}
				batch, dropped, timer = nil, 0, nil
			}

			for {
				select {
				case <-cmd.Context().Done():
					flush()
					return nil
				case l, ok := <-lines:
					if !ok {
						flush()
						return nil
					}
					if len(batch) < maxLines {
						batch = append(batch, l)
					} else {
						dropped++
					}
					if timer == nil {
						timer = time.After(window)
					}
				case <-timer:
					flush()
				}
			}
		},
	}
	f := cmd.Flags()
	f.StringVarP(&o.title, "title", "t", "", `Notification title (default "New output")`)
	f.DurationVar(&window, "batch", 5*time.Second, "Group lines arriving within this window into one push")
	f.IntVar(&maxLines, "max-lines", 20, "Lines per push; the rest are counted")
	o.bindDelivery(f)
	return cmd
}
