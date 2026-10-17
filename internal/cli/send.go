package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/PocketAlert/cli/internal/api"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// messageOpts are the delivery options shared by `send` and `exec`.
type messageOpts struct {
	title, message string
	application    string
	device         string
	level          string
	at, delay      string
	image          string
	actions        []string
	quiet          bool
}

func (o *messageOpts) bindDelivery(f *pflag.FlagSet) {
	f.StringVarP(&o.application, "app", "a", "", "Application TID")
	f.StringVar(&o.application, "application", "", "Application TID")
	_ = f.MarkHidden("application")
	f.StringVarP(&o.device, "device", "d", "", "Device TID, or 'all' (default: every device)")
	f.StringVarP(&o.level, "level", "l", "", "Delivery level: "+levelHelp)
	f.StringArrayVar(&o.actions, "action", nil, "Action button type:label:value, up to 3 (view, http, copy)")
	f.StringVar(&o.image, "image", "", "Attach an image: local file path or http(s) URL")
	f.BoolVarP(&o.quiet, "quiet", "q", false, "Print only the message TID")
}

func (o *messageOpts) bindSchedule(f *pflag.FlagSet) {
	f.StringVar(&o.at, "at", "", `Deliver at a time: "2026-10-20 09:00" (account timezone), RFC 3339 or Unix seconds`)
	f.StringVar(&o.delay, "in", "", "Deliver after a delay: 30m, 2h, 1d, 1d12h")
	f.StringVar(&o.delay, "delay", "", "Alias for --in")
	_ = f.MarkHidden("delay")
}

func (a *app) sendCmd() *cobra.Command {
	var o messageOpts
	cmd := &cobra.Command{
		Use:   "send [title] [message]",
		Short: "Send a push notification",
		Long: `Send a push notification to your devices.

Title and message can be positional arguments or flags. Use "-" as the message,
or pipe text in, to read the message from stdin.`,
		Example: `  pocketalert send "Backup done" "42 GB in 3m"
  pocketalert send -t "Disk" -m "90% full" --level high
  df -h | pocketalert send "Disk report"
  pocketalert send "Standup" "In 10 minutes" --at "2026-10-20 09:50"
  pocketalert send "Reminder" "Rotate keys" --in 1d
  pocketalert send "Deploy failed" "main@4f2c" -l critical \
      --action "view:Open logs:https://ci.example.com/run/1" \
      --action "http:Retry:https://ci.example.com/run/1/retry"`,
		Args:    cobra.MaximumNArgs(2),
		Aliases: []string{"push"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				if o.title != "" {
					return fmt.Errorf("title given both as argument and --title")
				}
				o.title = args[0]
			}
			if len(args) > 1 {
				if o.message != "" {
					return fmt.Errorf("message given both as argument and --message")
				}
				o.message = args[1]
			}
			if o.message == "-" || (o.message == "" && !isTerminal(a.stdin)) {
				b, err := io.ReadAll(io.LimitReader(a.stdin, 1<<20))
				if err != nil {
					return fmt.Errorf("read stdin: %w", err)
				}
				o.message = strings.TrimRight(string(b), "\n")
			}
			if strings.TrimSpace(o.title) == "" {
				return fmt.Errorf("title is required")
			}
			if strings.TrimSpace(o.message) == "" {
				return fmt.Errorf("message is required (argument, --message or stdin)")
			}
			res, raw, err := a.sendMessage(cmd.Context(), &o)
			if err != nil {
				return err
			}
			a.reportSent(&o, res, raw)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVarP(&o.title, "title", "t", "", "Notification title")
	f.StringVarP(&o.message, "message", "m", "", `Notification text ("-" reads stdin)`)
	o.bindDelivery(f)
	o.bindSchedule(f)
	cmd.MarkFlagsMutuallyExclusive("at", "in")
	return cmd
}

func (a *app) sendMessage(ctx context.Context, o *messageOpts) (*api.MessageCreated, []byte, error) {
	req := api.MessageCreate{
		Title:         o.title,
		Message:       o.message,
		ApplicationID: o.application,
		DeviceID:      o.device,
		SendAt:        strings.TrimSpace(o.at),
	}
	if o.level != "" {
		lvl, err := parseLevel(o.level)
		if err != nil {
			return nil, nil, err
		}
		req.Level = json.RawMessage(strconv.Itoa(lvl))
	}
	if o.delay != "" {
		d, err := normalizeDelay(o.delay)
		if err != nil {
			return nil, nil, err
		}
		req.Delay = d
	}
	actions, err := parseActions(o.actions, false)
	if err != nil {
		return nil, nil, err
	}
	req.Actions = actions

	var res api.MessageCreated
	var raw []byte
	if o.image != "" && !isHTTPURL(o.image) && !strings.HasPrefix(o.image, "data:") {
		if _, err := os.Stat(o.image); err != nil {
			return nil, nil, fmt.Errorf("image: %w", err)
		}
		raw, err = a.client.Multipart(ctx, "/v1/messages", multipartFields(req), "image", o.image, &res)
	} else {
		req.Image = o.image
		raw, err = a.client.Do(ctx, "POST", "/v1/messages", nil, req, &res)
	}
	if err != nil {
		return nil, raw, err
	}
	return &res, raw, nil
}

func multipartFields(m api.MessageCreate) map[string]string {
	fields := map[string]string{"title": m.Title, "message": m.Message}
	set := func(k, v string) {
		if v != "" {
			fields[k] = v
		}
	}
	set("application_id", m.ApplicationID)
	set("device_id", m.DeviceID)
	set("level", string(m.Level))
	set("send_at", m.SendAt)
	set("delay", m.Delay)
	if len(m.Actions) > 0 {
		b, _ := json.Marshal(m.Actions)
		fields["actions"] = string(b)
	}
	return fields
}

func (a *app) reportSent(o *messageOpts, res *api.MessageCreated, raw []byte) {
	if a.emitJSON(raw) {
		return
	}
	if o.quiet {
		fmt.Fprintln(a.stdout, res.TID)
		return
	}
	if res.Scheduled {
		a.ok("Scheduled for %s (%s)", res.DeliverAt, res.TZ)
		a.details("  TID", res.TID, "  Cancel", "pocketalert scheduled cancel "+res.TID)
	} else {
		a.ok("Sent")
		a.details("  TID", res.TID, "  Level", levelName(res.Priority), "  Device", res.Device)
	}
	if o.level != "" {
		if want, _ := parseLevel(o.level); want > res.Priority {
			fmt.Fprintf(a.stderr, "note: delivered as %q — %q needs a paid plan\n", levelName(res.Priority), levelName(want))
		}
	}
	if len(o.actions) > 0 && len(res.Actions) == 0 {
		fmt.Fprintln(a.stderr, "note: action buttons were dropped — they need a paid plan")
	}
}
