package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/PocketAlert/cli/internal/api"
	"github.com/PocketAlert/cli/internal/config"
	"github.com/spf13/cobra"
)

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// app carries state shared by every command. It is filled in the root
// PersistentPreRunE so flags such as --json are already parsed.
type app struct {
	build  BuildInfo
	cfg    *config.Config
	source config.Source
	client *api.Client
	json   bool
	stdout io.Writer
	stderr io.Writer
	stdin  io.Reader
}

// exitError carries an exit code chosen by the command itself (e.g. `exec`
// propagates the wrapped command's status) without printing anything more.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

func Execute(build BuildInfo) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	a := &app{build: build, stdout: os.Stdout, stderr: os.Stderr, stdin: os.Stdin}
	root := a.rootCmd()
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	var ee exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	var apiErr *api.Error
	if a.json && errors.As(err, &apiErr) && len(apiErr.Raw) > 0 {
		fmt.Fprintln(a.stderr, string(apiErr.Raw))
	} else {
		fmt.Fprintln(a.stderr, "Error:", err)
	}
	return 1
}

func (a *app) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "pocketalert",
		Short: "Send push notifications and manage Pocket Alert from the terminal",
		Long: `Pocket Alert CLI — send push notifications to your phone and manage
applications, devices, webhooks and API keys.

Get started:
  pocketalert auth                 # paste your API key
  pocketalert send "Deploy done" "v1.4.2 is live"

In CI, set POCKETALERT_API_KEY instead of running auth.
Docs: https://info.pocketalert.app/cli`,
		Version:       a.build.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cfg, source, err := config.Load()
			if err != nil {
				return err
			}
			a.cfg, a.source = cfg, source
			a.client = api.New(cfg.BaseURL, cfg.APIKey, "pocketalert-cli/"+a.build.Version)
			return nil
		},
	}
	root.SetVersionTemplate(a.versionString() + "\n")
	root.PersistentFlags().BoolVar(&a.json, "json", false, "Print raw JSON responses (for scripts and jq)")

	root.AddGroup(
		&cobra.Group{ID: "notify", Title: "Notifications:"},
		&cobra.Group{ID: "manage", Title: "Resources:"},
		&cobra.Group{ID: "account", Title: "Account:"},
	)

	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	add("notify", a.sendCmd(), a.execCmd(), a.watchCmd(), a.messagesCmd(), a.scheduledCmd())
	add("manage", a.appsCmd(), a.devicesCmd(), a.webhooksCmd(), a.formsCmd(), a.apikeysCmd())
	add("account", a.authCmd(), a.logoutCmd(), a.profileCmd(), a.usageCmd(), a.configCmd())
	root.AddCommand(a.versionCmd())
	return root
}

func (a *app) versionString() string {
	s := "pocketalert " + a.build.Version
	if a.build.Commit != "" {
		s += " (" + a.build.Commit
		if a.build.Date != "" {
			s += ", " + a.build.Date
		}
		s += ")"
	}
	return s
}

func (a *app) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		Args:  cobra.NoArgs,
		Run: func(*cobra.Command, []string) {
			fmt.Fprintln(a.stdout, a.versionString())
		},
	}
}
