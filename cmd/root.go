// Package cmd implements the md command line.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/eureka-corp/md-cli/internal/buildinfo"
	"github.com/eureka-corp/md-cli/internal/config"
	"github.com/eureka-corp/md-cli/internal/output"
	"github.com/eureka-corp/md-cli/internal/updatecheck"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "md",
		Short: "Share markdown files and folders through md.erk.im",
		Long: `md uploads markdown files and folders to md.erk.im and prints a link.

Links are readable by anyone who has them until they expire. With a personal
token (an API key from https://md.erk.im/settings) shares belong to your account
and can be listed, extended, kept forever, renamed and deleted from here.`,
		Version:       buildinfo.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("md {{.Version}}\n")
	root.PersistentFlags().Bool("ai", false, "Print a usage guide for AI agents and exit")

	root.AddGroup(
		&cobra.Group{ID: "share", Title: "Sharing:"},
		&cobra.Group{ID: "manage", Title: "Managing shares:"},
		&cobra.Group{ID: "cli", Title: "This CLI:"},
	)
	root.AddCommand(
		newShareCmd(), newListCmd(), newExtendCmd(), newKeepCmd(), newRenameCmd(), newUnshareCmd(),
		newSetupCmd(), newUpdateCmd(), newVersionCmd(),
	)
	return root
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	for _, arg := range os.Args[1:] {
		if arg == "--ai" {
			fmt.Print(aiGuide)
			return 0
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	root := newRootCmd()
	err := root.ExecuteContext(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", output.IconError, err)
	}

	cmd, _, _ := root.Find(os.Args[1:])
	if cmd == nil || cmd.Name() != "update" {
		notifyUpdate(ctx, os.Stderr)
	}

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return 130
		}
		return 1
	}
	return 0
}

// notifyUpdate prints a one-line notice on interactive terminals only, so
// scripts reading stdout or stderr are not affected.
func notifyUpdate(ctx context.Context, w io.Writer) {
	if !buildinfo.IsRelease() || os.Getenv("MD_NO_UPDATE_CHECK") != "" || !term.IsTerminal(int(os.Stderr.Fd())) {
		return
	}
	if latest, ok := updatecheck.Newer(ctx, buildinfo.Version, config.Dir(), buildinfo.RepoOwner, buildinfo.RepoName); ok {
		fmt.Fprintf(w, "\n%s md v%s is available %s run 'md update'\n", output.IconRocket, latest, output.IconArrow)
	}
}
