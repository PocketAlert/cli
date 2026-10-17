package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/PocketAlert/cli/internal/api"
	"github.com/PocketAlert/cli/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func maskToken(t string) string { return config.MaskKey(t) }

func (a *app) authCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "auth [api-key]",
		Aliases: []string{"login"},
		Short:   "Save your API key",
		Long: `Verify an API key and save it to ~/.pocketalert/config.json.

Without an argument the key is read from stdin or a hidden prompt, which keeps
it out of your shell history. Create keys at https://pocketalert.app/account/apikeys`,
		Example: `  pocketalert auth
  echo "$KEY" | pocketalert auth`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := ""
			if len(args) == 1 {
				key = args[0]
			} else {
				var err error
				if key, err = a.readSecret("API key: "); err != nil {
					return err
				}
			}
			key = strings.TrimSpace(key)
			if key == "" {
				return fmt.Errorf("API key is empty")
			}

			probe := api.New(a.cfg.BaseURL, key, a.client.UserAgent)
			var p api.Profile
			if _, err := probe.Do(cmd.Context(), "GET", "/v1/auth/profile", nil, nil, &p); err != nil {
				return fmt.Errorf("key rejected: %w", err)
			}

			stored, err := config.LoadFile()
			if err != nil {
				return err
			}
			stored.APIKey = key
			if err := config.Save(stored); err != nil {
				return err
			}
			a.ok("Signed in as %s", p.Email)
			fmt.Fprintf(a.stdout, "  Key saved to %s\n", config.Path())
			if a.source == config.SourceEnv {
				fmt.Fprintf(a.stderr, "note: %s is set and takes precedence over the saved key\n", config.EnvAPIKey)
			}
			return nil
		},
	}
}

func (a *app) readSecret(prompt string) (string, error) {
	if f, ok := a.stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(a.stderr, prompt)
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(a.stderr)
		return string(b), err
	}
	line, err := bufio.NewReader(a.stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return line, nil
}

func (a *app) logoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the saved API key",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			stored, err := config.LoadFile()
			if err != nil {
				return err
			}
			stored.APIKey = ""
			if err := config.Save(stored); err != nil {
				return err
			}
			a.ok("Logged out")
			if a.source == config.SourceEnv {
				fmt.Fprintf(a.stderr, "note: %s is still set in your environment\n", config.EnvAPIKey)
			}
			return nil
		},
	}
}

func (a *app) profileCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "profile",
		Aliases: []string{"whoami"},
		Short:   "Show the account behind the API key",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var p api.Profile
			raw, err := a.client.Do(cmd.Context(), "GET", "/v1/auth/profile", nil, nil, &p)
			if err != nil || a.emitJSON(raw) {
				return err
			}
			plan := "Free"
			if p.SubscriptionStatus != "" {
				plan = p.SubscriptionStatus
			}
			a.details("Username", p.Username, "Email", p.Email, "Email verified", check(p.EmailVerified),
				"Timezone", p.Timezone, "Subscription", plan)
			return nil
		},
	}
}

var configKeys = map[string]string{"api_key": "API key", "base_url": "API base URL"}

func (a *app) configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or change CLI settings",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			source := string(a.source)
			if source == "" {
				source = "not set"
			}
			a.details(
				"Config file", config.Path(),
				"API URL", a.cfg.BaseURL,
				"API key", config.MaskKey(a.cfg.APIKey),
				"Key source", source,
			)
			return nil
		},
	}
	set := &cobra.Command{
		Use:       "set <key> <value>",
		Short:     "Set a value: api_key or base_url",
		Args:      cobra.ExactArgs(2),
		ValidArgs: []string{"api_key", "base_url"},
		RunE: func(_ *cobra.Command, args []string) error {
			stored, err := config.LoadFile()
			if err != nil {
				return err
			}
			switch args[0] {
			case "api_key":
				stored.APIKey = strings.TrimSpace(args[1])
			case "base_url":
				if !isHTTPURL(args[1]) {
					return fmt.Errorf("base_url must be an http(s) URL")
				}
				stored.BaseURL = strings.TrimRight(args[1], "/")
			default:
				return fmt.Errorf("unknown key %q: use api_key or base_url", args[0])
			}
			if err := config.Save(stored); err != nil {
				return err
			}
			a.ok("Saved %s", configKeys[args[0]])
			return nil
		},
	}
	unset := &cobra.Command{
		Use:       "reset <key>",
		Short:     "Restore a value to its default",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"api_key", "base_url"},
		RunE: func(_ *cobra.Command, args []string) error {
			stored, err := config.LoadFile()
			if err != nil {
				return err
			}
			switch args[0] {
			case "api_key":
				stored.APIKey = ""
			case "base_url":
				stored.BaseURL = ""
			default:
				return fmt.Errorf("unknown key %q: use api_key or base_url", args[0])
			}
			if err := config.Save(stored); err != nil {
				return err
			}
			a.ok("Reset %s", configKeys[args[0]])
			return nil
		},
	}
	cmd.AddCommand(set, unset)
	return cmd
}
