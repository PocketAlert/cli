package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/PocketAlert/cli/internal/api"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const webhookBaseURL = "https://p4a.me/wh/"

func (a *app) listCmd(short string, run func(cmd *cobra.Command) error) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   short,
		Args:    cobra.NoArgs,
		RunE:    func(cmd *cobra.Command, _ []string) error { return run(cmd) },
	}
}

func (a *app) deleteCmd(noun, prefix string) *cobra.Command {
	return &cobra.Command{
		Use:     "delete <tid...>",
		Aliases: []string{"rm"},
		Short:   "Delete " + noun + "s",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.deleteEach(cmd, prefix, noun, args)
		},
	}
}

// changedLevel adds default_level to body when --level was given.
func changedLevel(f *pflag.FlagSet, level string, body map[string]any) error {
	if !f.Changed("level") {
		return nil
	}
	lvl, err := parseLevel(level)
	if err != nil {
		return err
	}
	body["default_level"] = lvl
	return nil
}

// ---- applications ----

func (a *app) appsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "apps",
		Aliases: []string{"applications", "app"},
		Short:   "Manage applications (notification sources)",
	}
	cmd.AddCommand(
		a.listCmd("List applications", func(cmd *cobra.Command) error {
			var apps []api.Application
			raw, err := a.client.Do(cmd.Context(), "GET", "/v1/applications", nil, nil, &apps)
			if err != nil || a.emitJSON(raw) {
				return err
			}
			if len(apps) == 0 {
				fmt.Fprintln(a.stdout, "No applications. Create one: pocketalert apps create --name \"Production\"")
				return nil
			}
			t := a.table("TID", "NAME", "COLOR", "ACTIVE", "CREATED")
			for _, x := range apps {
				t.row(x.TID, x.Name, dash(x.Color), check(x.IsActive), x.CreatedAt)
			}
			t.flush()
			return nil
		}),
		&cobra.Command{
			Use:   "get <tid>",
			Short: "Show an application",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				var x api.Application
				raw, err := a.client.Do(cmd.Context(), "GET", "/v1/applications/"+url.PathEscape(args[0]), nil, nil, &x)
				if err != nil || a.emitJSON(raw) {
					return err
				}
				a.details("TID", x.TID, "Name", x.Name, "Description", x.Description, "Color", x.Color,
					"Default level", levelPtrName(x.DefaultLevel), "Active", check(x.IsActive), "Created", x.CreatedAt)
				return nil
			},
		},
		a.appWriteCmd(false),
		a.appWriteCmd(true),
		a.deleteCmd("application", "/v1/applications/"),
	)
	return cmd
}

func (a *app) appWriteCmd(update bool) *cobra.Command {
	var name, description, color, level string
	cmd := &cobra.Command{
		Use:     "create",
		Short:   "Create an application",
		Args:    cobra.NoArgs,
		Example: `  pocketalert apps create --name "Production" --color "#EF4444" --level high`,
	}
	if update {
		cmd.Use, cmd.Short, cmd.Args = "update <tid>", "Update an application", cobra.ExactArgs(1)
		cmd.Example = `  pocketalert apps update k2x9… --name "Prod" --level critical`
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		f := cmd.Flags()
		body := map[string]any{}
		for flag, v := range map[string]string{"name": name, "description": description, "color": color} {
			if f.Changed(flag) {
				body[flag] = v
			}
		}
		if err := changedLevel(f, level, body); err != nil {
			return err
		}
		if !update {
			body["color"] = color
			raw, err := a.client.Do(cmd.Context(), "POST", "/v1/applications", nil, body, nil)
			if err != nil || a.emitJSON(raw) {
				return err
			}
			var x api.Application
			_ = json.Unmarshal(raw, &x)
			a.ok("Created application %q", x.Name)
			a.details("  TID", x.TID, "  Send with", "pocketalert send -a "+x.TID+" \"Title\" \"Message\"")
			return nil
		}
		if len(body) == 0 {
			return fmt.Errorf("nothing to update: pass --name, --description, --color or --level")
		}
		raw, err := a.client.Do(cmd.Context(), "PUT", "/v1/applications/"+url.PathEscape(args[0]), nil, body, nil)
		if err != nil || a.emitJSON(raw) {
			return err
		}
		a.ok("Updated application %s", args[0])
		return nil
	}
	f := cmd.Flags()
	f.StringVarP(&name, "name", "n", "", "Application name")
	f.StringVar(&description, "description", "", "Description")
	f.StringVarP(&color, "color", "c", "#3B82F6", "Color as hex")
	f.StringVarP(&level, "level", "l", "", "Default level for messages without one: "+levelHelp)
	if !update {
		_ = cmd.MarkFlagRequired("name")
	}
	return cmd
}

// ---- devices ----

func (a *app) devicesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "devices",
		Aliases: []string{"device"},
		Short:   "Manage devices that receive notifications",
	}
	cmd.AddCommand(
		a.listCmd("List devices", func(cmd *cobra.Command) error {
			var devices []api.Device
			raw, err := a.client.Do(cmd.Context(), "GET", "/v1/devices", nil, nil, &devices)
			if err != nil || a.emitJSON(raw) {
				return err
			}
			if len(devices) == 0 {
				fmt.Fprintln(a.stdout, "No devices. Install the app and sign in: https://pocketalert.app")
				return nil
			}
			t := a.table("TID", "NAME", "ACTIVE", "CREATED")
			for _, d := range devices {
				t.row(d.TID, d.Name, check(d.IsActive), d.CreatedAt)
			}
			t.flush()
			return nil
		}),
		&cobra.Command{
			Use:   "get <tid>",
			Short: "Show a device",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				var d api.Device
				raw, err := a.client.Do(cmd.Context(), "GET", "/v1/devices/tid/"+url.PathEscape(args[0]), nil, nil, &d)
				if err != nil || a.emitJSON(raw) {
					return err
				}
				a.details("TID", d.TID, "Name", d.Name, "Serial number", d.SerialNumber, "Active", check(d.IsActive), "Created", d.CreatedAt)
				return nil
			},
		},
		&cobra.Command{
			Use:   "rename <tid> <name>",
			Short: "Rename a device",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				raw, err := a.client.Do(cmd.Context(), "PUT", "/v1/devices/"+url.PathEscape(args[0]), nil, map[string]string{"name": args[1]}, nil)
				if err != nil || a.emitJSON(raw) {
					return err
				}
				a.ok("Renamed device %s to %q", args[0], args[1])
				return nil
			},
		},
		a.deleteCmd("device", "/v1/devices/"),
	)
	return cmd
}

// ---- webhooks ----

func (a *app) webhooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "webhooks",
		Aliases: []string{"webhook", "wh"},
		Short:   "Manage inbound webhooks that turn HTTP requests into notifications",
	}
	cmd.AddCommand(
		a.listCmd("List webhooks", func(cmd *cobra.Command) error {
			var hooks []api.Webhook
			raw, err := a.client.Do(cmd.Context(), "GET", "/v1/webhooks", nil, nil, &hooks)
			if err != nil || a.emitJSON(raw) {
				return err
			}
			if len(hooks) == 0 {
				fmt.Fprintln(a.stdout, "No webhooks.")
				return nil
			}
			t := a.table("TID", "NAME", "URL", "ACTIVE", "CREATED")
			for _, w := range hooks {
				t.row(w.TID, w.Name, webhookBaseURL+w.URL, check(w.IsActive != nil && *w.IsActive), w.CreatedAt)
			}
			t.flush()
			return nil
		}),
		&cobra.Command{
			Use:   "get <tid>",
			Short: "Show a webhook",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				var w api.Webhook
				raw, err := a.client.Do(cmd.Context(), "GET", "/v1/webhooks/"+url.PathEscape(args[0]), nil, nil, &w)
				if err != nil || a.emitJSON(raw) {
					return err
				}
				a.details("TID", w.TID, "Name", w.Name, "URL", webhookBaseURL+w.URL, "Template", w.Message,
					"Application", w.ApplicationTID, "Device", w.DeviceTID, "Default level", levelPtrName(w.DefaultLevel),
					"Actions", w.Actions, "Active", check(w.IsActive != nil && *w.IsActive), "Created", w.CreatedAt)
				return nil
			},
		},
		a.webhookWriteCmd(false),
		a.webhookWriteCmd(true),
		a.webhookHistoryCmd(),
		a.deleteCmd("webhook", "/v1/webhooks/"),
	)
	return cmd
}

func (a *app) webhookWriteCmd(update bool) *cobra.Command {
	var (
		name, message, slug, application, device, level string
		actions                                         []string
		active                                          bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a webhook",
		Args:  cobra.NoArgs,
		Example: `  pocketalert webhooks create --name Grafana --template "%title%: %message%" -l high
  pocketalert webhooks create --name Raw --template "*" --slug my-alerts`,
	}
	if update {
		cmd.Use, cmd.Short, cmd.Args = "update <tid>", "Update a webhook (only the given flags change)", cobra.ExactArgs(1)
		cmd.Example = `  pocketalert webhooks update k2x9… --template "%alert.name%" --active=false`
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		f := cmd.Flags()
		body := map[string]any{}
		for flag, key := range map[string]string{"name": "name", "template": "message", "slug": "url", "app": "application_id", "device": "device_id"} {
			if f.Changed(flag) {
				v, _ := f.GetString(flag)
				body[key] = v
			}
		}
		if err := changedLevel(f, level, body); err != nil {
			return err
		}
		if f.Changed("action") {
			parsed, err := parseActions(actions, true)
			if err != nil {
				return err
			}
			b, _ := json.Marshal(parsed)
			body["actions"] = string(b)
		}
		if update && f.Changed("active") {
			body["is_active"] = active
		}

		if !update {
			body["message"] = message
			var w api.Webhook
			raw, err := a.client.Do(cmd.Context(), "POST", "/v1/webhooks", nil, body, &w)
			if err != nil || a.emitJSON(raw) {
				return err
			}
			a.ok("Created webhook %q", w.Name)
			a.details("  TID", w.TID, "  URL", webhookBaseURL+w.URL)
			fmt.Fprintf(a.stdout, "\nTry it:\n  curl -X POST %s%s -H 'Content-Type: application/json' -d '{\"title\":\"Hi\",\"message\":\"It works\"}'\n", webhookBaseURL, w.URL)
			return nil
		}
		if len(body) == 0 {
			return fmt.Errorf("nothing to update: pass at least one flag")
		}
		raw, err := a.client.Do(cmd.Context(), "PUT", "/v1/webhooks/"+url.PathEscape(args[0]), nil, body, nil)
		if err != nil || a.emitJSON(raw) {
			return err
		}
		a.ok("Updated webhook %s", args[0])
		return nil
	}
	f := cmd.Flags()
	f.StringVarP(&name, "name", "n", "", "Webhook name")
	f.StringVarP(&message, "template", "m", "*", "Message template: * for the whole payload, %field.path% to extract values")
	f.StringVar(&slug, "slug", "", "Custom URL slug (default: generated)")
	f.StringVarP(&application, "app", "a", "", "Application TID")
	f.StringVarP(&device, "device", "d", "", "Device TID or 'all'")
	f.StringVarP(&level, "level", "l", "", "Default level: "+levelHelp)
	f.StringArrayVar(&actions, "action", nil, "Action button type:label:value, may contain %field.path% (up to 3)")
	if update {
		f.BoolVar(&active, "active", true, "Enable or disable the webhook (--active=false)")
	} else {
		_ = cmd.MarkFlagRequired("name")
	}
	return cmd
}

func (a *app) webhookHistoryCmd() *cobra.Command {
	var page, per int
	var status string
	cmd := &cobra.Command{
		Use:   "history <tid>",
		Short: "Show requests recently received by a webhook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{"page": {strconv.Itoa(page)}, "per": {strconv.Itoa(per)}}
			if status != "" {
				if status != "success" && status != "error" {
					return fmt.Errorf("--status must be success or error")
				}
				q.Set("status", status)
			}
			var h api.WebhookHistory
			raw, err := a.client.Do(cmd.Context(), "GET", "/v1/webhooks/"+url.PathEscape(args[0])+"/history", q, nil, &h)
			if err != nil || a.emitJSON(raw) {
				return err
			}
			if len(h.Requests) == 0 {
				fmt.Fprintf(a.stdout, "No requests in the last %d days.\n", h.RetentionDays)
				return nil
			}
			t := a.table("TIME", "METHOD", "STATUS", "SIZE", "SOURCE", "MESSAGE")
			for _, r := range h.Requests {
				result := r.Status
				if r.ErrorCode != "" {
					result += " (" + r.ErrorCode + ")"
				}
				t.row(r.CreatedAt, r.Method, result, r.Size, dash(r.SourceIP), dash(truncate(r.RenderedMessage, 40)))
			}
			t.flush()
			fmt.Fprintf(a.stderr, "\nPage %d, %d of %d requests (kept %d days).\n", h.Page, len(h.Requests), h.Total, h.RetentionDays)
			return nil
		},
	}
	f := cmd.Flags()
	f.IntVar(&page, "page", 1, "Page number")
	f.IntVar(&per, "per", 20, "Requests per page (max 200)")
	f.StringVar(&status, "status", "", "Filter: success or error")
	return cmd
}

// ---- forms ----

func (a *app) formsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "forms",
		Aliases: []string{"form"},
		Short:   "List and delete public forms (create them in the web console)",
	}
	cmd.AddCommand(
		a.listCmd("List forms", func(cmd *cobra.Command) error {
			var forms []api.Form
			raw, err := a.client.Do(cmd.Context(), "GET", "/v1/forms", nil, nil, &forms)
			if err != nil || a.emitJSON(raw) {
				return err
			}
			if len(forms) == 0 {
				fmt.Fprintln(a.stdout, "No forms. Create one at https://pocketalert.app/forms")
				return nil
			}
			t := a.table("TID", "NAME", "ACTIVE", "SUBMISSIONS", "LAST SUBMISSION", "LINK")
			for _, x := range forms {
				active := check(x.IsActive)
				if x.Closed {
					active = "closed"
				}
				t.row(x.TID, x.Name, active, x.SubmissionsCount, dash(x.LastSubmissionAt), "https://p4a.me/f/"+x.TID)
			}
			t.flush()
			return nil
		}),
		a.deleteCmd("form", "/v1/forms/"),
	)
	return cmd
}

// ---- API keys ----

func (a *app) apikeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "apikeys",
		Aliases: []string{"keys", "apikey"},
		Short:   "Manage API keys",
	}
	var name string
	create := &cobra.Command{
		Use:   "create",
		Short: "Create an API key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var k api.APIKey
			raw, err := a.client.Do(cmd.Context(), "POST", "/v1/apikeys", nil, map[string]string{"name": name}, &k)
			if err != nil || a.emitJSON(raw) {
				return err
			}
			a.ok("Created API key %q", k.Name)
			a.details("  TID", k.TID, "  Token", k.Token)
			fmt.Fprintln(a.stderr, "\nStore the token now — it is a secret.")
			return nil
		},
	}
	create.Flags().StringVarP(&name, "name", "n", "", "Key name")
	_ = create.MarkFlagRequired("name")

	cmd.AddCommand(
		a.listCmd("List API keys", func(cmd *cobra.Command) error {
			var keys []api.APIKey
			raw, err := a.client.Do(cmd.Context(), "GET", "/v1/apikeys", nil, nil, &keys)
			if err != nil || a.emitJSON(raw) {
				return err
			}
			if len(keys) == 0 {
				fmt.Fprintln(a.stdout, "No API keys.")
				return nil
			}
			t := a.table("TID", "NAME", "TOKEN", "ACTIVE", "CREATED")
			for _, k := range keys {
				current := ""
				if k.Token == a.cfg.APIKey {
					current = " (this CLI)"
				}
				t.row(k.TID, k.Name+current, maskToken(k.Token), check(k.IsActive), k.CreatedAt)
			}
			t.flush()
			return nil
		}),
		create,
		a.deleteCmd("API key", "/v1/apikeys/"),
	)
	return cmd
}
