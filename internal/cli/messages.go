package cli

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/PocketAlert/cli/internal/api"
	"github.com/spf13/cobra"
)

func (a *app) messagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "messages",
		Aliases: []string{"msg", "message"},
		Short:   "List, read and delete received messages",
	}

	send := a.sendCmd()
	send.Aliases = nil
	send.Short = "Send a push notification (same as `pocketalert send`)"
	cmd.AddCommand(a.messagesListCmd(), a.messagesGetCmd(), send, a.messagesReadCmd(),
		a.messagesDeleteCmd(), a.messagesStatsCmd())
	return cmd
}

func (a *app) messagesListCmd() *cobra.Command {
	var (
		limit, offset int
		sort, device  string
	)
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List messages, newest first",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if sort != "asc" && sort != "desc" {
				return fmt.Errorf("--sort must be asc or desc")
			}
			q := url.Values{
				"size":   {strconv.Itoa(limit)},
				"offset": {strconv.Itoa(offset)},
				"sort":   {sort},
			}
			if device != "" {
				q.Set("tid", device)
			}
			var page api.MessagePage
			raw, err := a.client.Do(cmd.Context(), "GET", "/v2/messages", q, nil, &page)
			if err != nil {
				return err
			}
			if a.emitJSON(raw) {
				return nil
			}
			if len(page.Data) == 0 {
				fmt.Fprintln(a.stdout, "No messages.")
				return nil
			}
			t := a.table("TID", "LEVEL", "TITLE", "APPLICATION", "READ", "CREATED")
			for _, m := range page.Data {
				title := m.Title
				if m.WasScheduled {
					title = "⏱ " + title
				}
				t.row(m.TID, levelName(m.Priority), truncate(title, 40), dash(m.Application), check(m.IsRead), m.CreatedAt)
			}
			t.flush()
			if page.Meta.HasMore {
				fmt.Fprintf(a.stderr, "\nShowing %d–%d of %d. Next page: --offset %d\n",
					offset+1, offset+len(page.Data), page.Meta.Total, page.Meta.NextOffset)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.IntVarP(&limit, "limit", "n", 20, "Number of messages")
	f.IntVar(&offset, "offset", 0, "Skip this many messages")
	f.StringVar(&sort, "sort", "desc", "Sort order: asc or desc")
	f.StringVarP(&device, "device", "d", "", "Only messages sent to this device TID")
	return cmd
}

func (a *app) messagesGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <tid>",
		Short: "Show a message (marks it as read)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var m api.Message
			raw, err := a.client.Do(cmd.Context(), "GET", "/v1/messages/"+url.PathEscape(args[0]), nil, nil, &m)
			if err != nil {
				return err
			}
			if a.emitJSON(raw) {
				return nil
			}
			a.details(
				"TID", m.TID,
				"Title", m.Title,
				"Level", levelName(m.Priority),
				"Application", m.Application,
				"Device", m.Device,
				"Created", m.CreatedAt,
			)
			if m.WasScheduled {
				a.details("Scheduled", "yes")
			}
			if m.Attachment != nil {
				a.details("Attachment", fmt.Sprintf("%s (%s)", m.Attachment.FileName, m.Attachment.Link))
			}
			for i, act := range m.Actions {
				a.details(fmt.Sprintf("Action %d", i+1), fmt.Sprintf("[%s] %s → %s", act.Type, act.Label, act.Value))
			}
			fmt.Fprintln(a.stdout)
			fmt.Fprintln(a.stdout, m.Message)
			return nil
		},
	}
}

func (a *app) messagesReadCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "read [tid...]",
		Short: "Mark messages as read",
		Example: `  pocketalert messages read k2x9…
  pocketalert messages read --all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all == (len(args) > 0) {
				return fmt.Errorf("pass message TIDs or --all")
			}
			if all {
				raw, err := a.client.Do(cmd.Context(), "PUT", "/v1/messages/read-all", nil, nil, nil)
				if err != nil || a.emitJSON(raw) {
					return err
				}
				a.ok("All messages marked as read")
				return nil
			}
			for _, tid := range args {
				if _, err := a.client.Do(cmd.Context(), "PUT", "/v1/messages/"+url.PathEscape(tid), nil, map[string]bool{"is_read": true}, nil); err != nil {
					return fmt.Errorf("%s: %w", tid, err)
				}
				a.ok("Marked %s as read", tid)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Mark every message as read")
	return cmd
}

func (a *app) messagesDeleteCmd() *cobra.Command {
	var all, yes bool
	cmd := &cobra.Command{
		Use:     "delete [tid...]",
		Aliases: []string{"rm"},
		Short:   "Delete messages",
		Example: `  pocketalert messages delete k2x9… p8m1…
  pocketalert messages delete --all --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all == (len(args) > 0) {
				return fmt.Errorf("pass message TIDs or --all")
			}
			if all {
				if !yes {
					ok, err := a.confirm("Delete ALL messages and attachments? This cannot be undone.")
					if err != nil || !ok {
						return err
					}
				}
				raw, err := a.client.Do(cmd.Context(), "DELETE", "/v1/messages/bulk", nil, nil, nil)
				if err != nil || a.emitJSON(raw) {
					return err
				}
				a.ok("All messages deleted")
				return nil
			}
			return a.deleteEach(cmd, "/v1/messages/", "message", args)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Delete every message on the account")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation for --all")
	return cmd
}

func (a *app) messagesStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Messages per day",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var stats []api.MessageStat
			raw, err := a.client.Do(cmd.Context(), "GET", "/v1/messages/stat", nil, nil, &stats)
			if err != nil || a.emitJSON(raw) {
				return err
			}
			if len(stats) == 0 {
				fmt.Fprintln(a.stdout, "No messages yet.")
				return nil
			}
			peak := 1
			for _, s := range stats {
				peak = max(peak, s.Count)
			}
			t := a.table("DATE", "COUNT", "")
			for _, s := range stats {
				t.row(s.Date, s.Count, strings.Repeat("▇", (s.Count*30+peak-1)/peak))
			}
			t.flush()
			return nil
		},
	}
}

func (a *app) usageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "usage",
		Short: "Today's message usage against the daily limit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var u api.MessageUsage
			raw, err := a.client.Do(cmd.Context(), "GET", "/v1/messages/usage", nil, nil, &u)
			if err != nil || a.emitJSON(raw) {
				return err
			}
			limit := "unlimited"
			if !u.Unlimited && u.Limit != nil {
				limit = strconv.Itoa(*u.Limit)
			}
			rejected := ""
			if u.Rejected > 0 {
				rejected = strconv.Itoa(u.Rejected)
			}
			a.details(
				"Sent today", strconv.Itoa(u.Used),
				"Daily limit", limit,
				"Rejected", rejected,
				"Resets at", u.ResetsAt,
			)
			return nil
		},
	}
}

// deleteEach deletes resources one by one and stops at the first failure so
// the user sees exactly which TID was not removed.
func (a *app) deleteEach(cmd *cobra.Command, prefix, noun string, tids []string) error {
	for _, tid := range tids {
		if _, err := a.client.Do(cmd.Context(), "DELETE", prefix+url.PathEscape(tid), nil, nil, nil); err != nil {
			return fmt.Errorf("%s: %w", tid, err)
		}
		if !a.json {
			a.ok("Deleted %s %s", noun, tid)
		}
	}
	return nil
}
