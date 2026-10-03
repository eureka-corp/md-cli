package cmd

import (
	"fmt"
	"runtime"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/spf13/cobra"

	"github.com/eureka-corp/md-cli/internal/buildinfo"
	"github.com/eureka-corp/md-cli/internal/output"
)

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "update",
		GroupID: "cli",
		Short:   "Update md to the latest release",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// go-selfupdate panics on non-semver versions such as "dev".
			if !buildinfo.IsRelease() {
				return fmt.Errorf("this is a development build; install a release first (see the README)")
			}
			out := cmd.ErrOrStderr()
			fmt.Fprintf(out, "%s Current version %s (%s/%s), checking for updates…\n",
				output.IconInfo, buildinfo.Version, runtime.GOOS, runtime.GOARCH)

			updater, err := selfupdate.NewUpdater(selfupdate.Config{
				Validator: &selfupdate.ChecksumValidator{UniqueFilename: "checksums.txt"},
			})
			if err != nil {
				return err
			}
			slug := selfupdate.NewRepositorySlug(buildinfo.RepoOwner, buildinfo.RepoName)
			latest, found, err := updater.DetectLatest(cmd.Context(), slug)
			if err != nil {
				return fmt.Errorf("detect latest version: %w", err)
			}
			if !found {
				return fmt.Errorf("no release found for %s/%s on %s/%s",
					buildinfo.RepoOwner, buildinfo.RepoName, runtime.GOOS, runtime.GOARCH)
			}
			if !latest.GreaterThan(buildinfo.Version) {
				fmt.Fprintf(out, "%s Already up to date\n", output.IconSuccess)
				return nil
			}
			exe, err := selfupdate.ExecutablePath()
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%s Updating to v%s…\n", output.IconRocket, latest.Version())
			if err := updater.UpdateTo(cmd.Context(), latest, exe); err != nil {
				return fmt.Errorf("update failed: %w", err)
			}
			fmt.Fprintf(out, "%s Updated to v%s\n", output.IconSuccess, latest.Version())
			return nil
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		GroupID: "cli",
		Short:   "Print version information",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			buildinfo.Print(cmd.OutOrStdout())
		},
	}
}
