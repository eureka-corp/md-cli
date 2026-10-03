package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eureka-corp/md-cli/internal/config"
	"github.com/eureka-corp/md-cli/internal/mdshare"
	"github.com/eureka-corp/md-cli/internal/output"
)

func newSetupCmd() *cobra.Command {
	var token, url string
	c := &cobra.Command{
		Use:     "setup",
		GroupID: "cli",
		Short:   "Save and verify the upload token",
		Long: `Save and verify the upload token.

Create a personal token on https://md.erk.im/shares (sign in first) so your
shares belong to your account. Pass --token to skip the prompt.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("read config: %w", err)
			}
			in := bufio.NewReader(cmd.InOrStdin())
			stderr := cmd.ErrOrStderr()

			if token == "" {
				fallback := cfg.Token
				if fallback == "" {
					fallback = config.OrionToken()
				}
				hint := ""
				if fallback != "" {
					hint = fmt.Sprintf(" [keep …%s]", last4(fallback))
				}
				fmt.Fprintf(stderr, "Upload token%s: ", hint)
				input, _ := in.ReadString('\n')
				if token = strings.TrimSpace(input); token == "" {
					token = fallback
				}
			}
			if token == "" {
				return fmt.Errorf("an upload token is required")
			}

			if url == "" {
				url = baseURL(cfg)
				if !cmd.Flags().Changed("token") {
					fmt.Fprintf(stderr, "Server [%s]: ", url)
					if input, _ := in.ReadString('\n'); strings.TrimSpace(input) != "" {
						url = strings.TrimSpace(input)
					}
				}
			}
			url = strings.TrimRight(url, "/")

			// A rejected token is not saved; an unreachable server only warns.
			verifyErr := (&mdshare.Client{BaseURL: url, Token: token}).Verify(cmd.Context())
			var apiErr *mdshare.APIError
			if errors.As(verifyErr, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				return fmt.Errorf("token rejected by %s", url)
			}

			cfg.Token, cfg.BaseURL = token, url
			if url == mdshare.DefaultBaseURL {
				cfg.BaseURL = ""
			}
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			if verifyErr != nil {
				fmt.Fprintf(stderr, "%s Could not verify the token (%v); saved anyway\n", output.IconWarning, verifyErr)
				return nil
			}
			kind := "shared operator token: shares are not listed under an account"
			if strings.HasPrefix(token, "mdr_") {
				kind = "personal token"
			}
			fmt.Fprintf(stderr, "%s Token verified and saved (%s)\n", output.IconSuccess, kind)
			return nil
		},
	}
	c.Flags().StringVar(&token, "token", "", "Upload token (skips the prompt)")
	c.Flags().StringVar(&url, "url", "", "Server URL (default "+mdshare.DefaultBaseURL+")")
	return c
}

func last4(s string) string {
	if len(s) <= 4 {
		return s
	}
	return s[len(s)-4:]
}
