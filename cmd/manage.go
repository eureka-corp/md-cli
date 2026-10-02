package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/eureka-corp/md-cli/internal/mdshare"
	"github.com/eureka-corp/md-cli/internal/output"
)

func newListCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		GroupID: "manage",
		Short:   "List the shares owned by your account",
		Long: `List the shares owned by your account, soonest expiry first.

Needs a personal token. A share can take up to a minute to appear after it is
created.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := newClient()
			if err != nil {
				return err
			}
			shares, err := client.List(cmd.Context())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(shares)
			}
			if len(shares) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "No shares. Shares made with the shared operator token are not listed.")
				return nil
			}
			w := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "TITLE\tFILES\tEXPIRES\tLINK")
			now := time.Now()
			for _, s := range shares {
				expires := "never"
				if s.ExpiresAt != nil {
					expires = relative(s.ExpiresAt.Sub(now))
				}
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", truncate(s.Title, 48), s.FileCount, expires, s.URL)
			}
			return w.Flush()
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	return c
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func newExtendCmd() *cobra.Command {
	var fromNow bool
	c := &cobra.Command{
		Use:     "extend <link|id> [duration]",
		GroupID: "manage",
		Short:   "Push back a share's expiry (default 7d)",
		Long: `Push back a share's expiry by a duration (default 7d), up to 30 days from now.

With --from-now the expiry is set to now + duration instead. On a share that
never expires, either form gives it an expiry again.`,
		Example: `  md extend https://md.erk.im/s/abc 30d
  md extend abc 2h --from-now`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := shareIDArg(args[0])
			if err != nil {
				return err
			}
			d := 7 * 24 * time.Hour
			if len(args) == 2 {
				if d, err = mdshare.ParseTTL(args[1]); err != nil {
					return err
				}
			}
			p := mdshare.Patch{Extend: d}
			if fromNow {
				p = mdshare.Patch{TTL: d}
			}
			return patchAndReport(cmd, id, p)
		},
	}
	c.Flags().BoolVar(&fromNow, "from-now", false, "Set the expiry to now + duration")
	return c
}

func newKeepCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "keep <link|id>",
		GroupID: "manage",
		Short:   "Remove a share's expiry so it is kept until deleted",
		Long: `Remove a share's expiry so it is kept until you delete it.

Only shares owned by an account (uploaded with a personal token) can be kept.
Use "md extend" to give it an expiry again.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := shareIDArg(args[0])
			if err != nil {
				return err
			}
			return patchAndReport(cmd, id, mdshare.Patch{Keep: true})
		},
	}
}

func newRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "rename <link|id> [title]",
		GroupID: "manage",
		Short:   "Set the title shown on the My shares page",
		Long:    `Set the title shown on the My shares page. Without a title, it goes back to the file name.`,
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := shareIDArg(args[0])
			if err != nil {
				return err
			}
			title := ""
			if len(args) == 2 {
				title = strings.TrimSpace(args[1])
			}
			return patchAndReport(cmd, id, mdshare.Patch{Title: &title})
		},
	}
}

func patchAndReport(cmd *cobra.Command, id string, p mdshare.Patch) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	share, err := client.Patch(cmd.Context(), id, p)
	if err != nil {
		return err
	}
	if p.Title == nil {
		_ = shareState().SetExpiry(id, share.ExpiresAt)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%s %s — expires %s\n", output.IconSuccess, share.Title, formatExpiry(share.ExpiresAt, time.Now()))
	fmt.Fprintln(cmd.OutOrStdout(), share.URL)
	return nil
}

func newUnshareCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "unshare <link|id>...",
		Aliases: []string{"rm", "delete"},
		GroupID: "manage",
		Short:   "Delete shares before they expire",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClient()
			if err != nil {
				return err
			}
			state := shareState()
			for _, arg := range args {
				id, err := shareIDArg(arg)
				if err != nil {
					return err
				}
				if err := client.Delete(cmd.Context(), id); err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
				_ = state.Forget(id)
				fmt.Fprintf(cmd.ErrOrStderr(), "%s Deleted %s\n", output.IconSuccess, id)
			}
			return nil
		},
	}
}
