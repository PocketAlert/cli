package cli

import (
	"fmt"
	"net/url"

	"github.com/PocketAlert/cli/internal/api"
	"github.com/spf13/cobra"
)

var scheduledStatuses = map[string]bool{"all": true, "pending": true, "sending": true, "sent": true, "canceled": true, "failed": true}

func (a *app) scheduledCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "scheduled",
		Aliases: []string{"sched"},
		Short:   "List and cancel scheduled messages",
		Long: `Scheduled messages are created with "pocketalert send --at" or "--in"
(paid plans) and delivered later.`,
	}
	cmd.AddCommand(a.scheduledListCmd(), a.scheduledCancelCmd())
	return cmd
}

func (a *app) scheduledListCmd() *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List scheduled messages, soonest first",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !scheduledStatuses[status] {
				return fmt.Errorf("--status must be one of all, pending, sending, sent, canceled, failed")
			}
			var items []api.ScheduledMessage
			raw, err := a.client.Do(cmd.Context(), "GET", "/v1/messages/scheduled", url.Values{"status": {status}}, nil, &items)
			if err != nil || a.emitJSON(raw) {
				return err
			}
			if len(items) == 0 {
				if status == "all" {
					fmt.Fprintln(a.stdout, "No scheduled messages.")
				} else {
					fmt.Fprintf(a.stdout, "No %s scheduled messages.\n", status)
				}
				return nil
			}
			t := a.table("TID", "DELIVER AT", "STATUS", "LEVEL", "TITLE", "DEVICE")
			for _, m := range items {
				t.row(m.TID, m.DeliverAt, m.Status, levelName(m.Priority), truncate(m.Title, 40), dash(m.Device))
			}
			t.flush()
			if len(items) > 0 {
				fmt.Fprintf(a.stderr, "\nTimes are in %s.\n", items[0].TZ)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "pending", "Filter: all, pending, sending, sent, canceled, failed")
	return cmd
}

func (a *app) scheduledCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "cancel <tid...>",
		Aliases: []string{"rm", "delete"},
		Short:   "Cancel pending scheduled messages",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, tid := range args {
				if _, err := a.client.Do(cmd.Context(), "DELETE", "/v1/messages/scheduled/"+url.PathEscape(tid), nil, nil, nil); err != nil {
					return fmt.Errorf("%s: %w", tid, err)
				}
				if !a.json {
					a.ok("Canceled %s", tid)
				}
			}
			return nil
		},
	}
}
