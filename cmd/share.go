package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/eureka-corp/md-cli/internal/mdshare"
	"github.com/eureka-corp/md-cli/internal/output"
)

var ttlChoices = []string{"1h", "24h", "7d", "30d", "never"}

type shareOptions struct {
	ttl       string
	update    bool
	id        string
	stdinName string
	title     string
	slides    bool
}

func newShareCmd() *cobra.Command {
	var o shareOptions
	c := &cobra.Command{
		Use:     "share <file|dir|->...",
		GroupID: "share",
		Short:   "Upload markdown files or folders and print a link",
		Long: `Upload markdown files or folders and print a link.

Folders are walked recursively; hidden folders, node_modules, symlinks and
non-markdown files are skipped. Use "-" to read one document from stdin.
Only the link is printed on stdout, so the command can be piped.

--ttl takes a duration (1h, 24h, 7d; at most 30d) or "never". "never" needs a
personal token, because only shares owned by an account can be kept.

With --update, the share last created for the same path(s) is replaced and its
link kept. The expiry is kept too unless --ttl is given. If that share no
longer exists, a new one is created.`,
		Example: `  md share README.md
  md share docs/ --ttl 7d --title "Design docs"
  cat notes.md | md share - --name notes.md
  md share docs/ --update        # same link, new content, same expiry
  md share deck.md --slides      # link opens as a slide deck`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShare(cmd, args, o)
		},
	}
	f := c.Flags()
	f.StringVar(&o.ttl, "ttl", "", "Expiry: a duration such as 1h, 24h, 7d (max 30d), or never")
	f.BoolVarP(&o.update, "update", "u", false, "Replace the share previously created for the same path(s), keeping its link")
	f.StringVar(&o.id, "id", "", "Share link or id to replace (implies --update)")
	f.StringVar(&o.stdinName, "name", "stdin.md", `File name for a document read from stdin ("-")`)
	f.StringVar(&o.title, "title", "", "Title shown on the My shares page")
	f.BoolVar(&o.slides, "slides", false, "Print a link that opens the document as slides")
	return c
}

func runShare(cmd *cobra.Command, args []string, o shareOptions) error {
	stderr := cmd.ErrOrStderr()
	client, err := newClient()
	if err != nil {
		return err
	}

	fromStdin := 0
	for _, a := range args {
		if a == "-" {
			fromStdin++
		}
	}
	if fromStdin > 1 {
		return fmt.Errorf(`"-" can only be given once`)
	}

	files, err := mdshare.Collect(args, cmd.InOrStdin(), o.stdinName)
	if err != nil {
		return err
	}

	var exp mdshare.Expiry
	if o.ttl != "" {
		if exp, err = mdshare.ParseExpiry(o.ttl); err != nil {
			return err
		}
	}

	state := shareState()
	key, err := mdshare.StateKey(args, o.stdinName)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	var share *mdshare.Share
	verb := "Shared"
	if o.update || o.id != "" {
		id := ""
		if o.id != "" {
			if id, err = shareIDArg(o.id); err != nil {
				return err
			}
		} else if rec, ok := state.Lookup(key); ok {
			id = rec.ID
		}
		if id == "" {
			fmt.Fprintf(stderr, "%s No previous share found for this path, creating a new one\n", output.IconInfo)
		} else {
			share, err = client.Update(ctx, id, files, exp)
			switch {
			case mdshare.IsNotFound(err):
				fmt.Fprintf(stderr, "%s Share %s no longer exists, creating a new one\n", output.IconInfo, id)
			case err != nil:
				return err
			default:
				verb = "Updated"
			}
		}
	}

	if share == nil {
		if exp == (mdshare.Expiry{}) {
			exp.TTL = mdshare.DefaultTTL
			if fromStdin == 0 && isTerminal(cmd.InOrStdin()) {
				if exp, err = promptExpiry(cmd.InOrStdin(), stderr); err != nil {
					return err
				}
			}
		}
		if share, err = client.Create(ctx, files, exp); err != nil {
			return err
		}
	}

	if o.title != "" {
		title := o.title
		patched, err := client.Patch(ctx, share.ID, mdshare.Patch{Title: &title})
		if err != nil {
			fmt.Fprintf(stderr, "%s Could not set the title: %v\n", output.IconWarning, err)
		} else {
			share.Title = patched.Title
		}
	}

	if err := state.Remember(key, share); err != nil {
		fmt.Fprintf(stderr, "%s Could not remember this share for --update: %v\n", output.IconWarning, err)
	}

	fmt.Fprintf(stderr, "%s %s %d file(s), expires %s\n",
		output.IconSuccess, verb, share.FileCount, formatExpiry(share.ExpiresAt, time.Now()))
	if share.FileCount != len(files) {
		fmt.Fprintf(stderr, "%s Sent %d file(s) but the server stored %d\n", output.IconWarning, len(files), share.FileCount)
	}
	if exp.TTL > 0 && share.TTL != nil {
		if got := time.Duration(*share.TTL) * time.Second; got != exp.TTL.Truncate(time.Second) {
			fmt.Fprintf(stderr, "%s Expiry adjusted by the server: asked %s, got %s\n",
				output.IconWarning, mdshare.FormatTTL(exp.TTL), mdshare.FormatTTL(got))
		}
	}
	if n := mdshare.LocalImageRefs(files); n > 0 {
		fmt.Fprintf(stderr, "%s %d local image link(s) will not render: only markdown is uploaded\n", output.IconWarning, n)
	}

	link := share.URL
	if o.slides {
		link += "?present"
	}
	fmt.Fprintln(cmd.OutOrStdout(), link)
	return nil
}

func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

func promptExpiry(in io.Reader, out io.Writer) (mdshare.Expiry, error) {
	fmt.Fprintln(out, "Expire after:")
	for i, c := range ttlChoices {
		fmt.Fprintf(out, "  [%d] %s\n", i+1, c)
	}
	fmt.Fprint(out, "Pick [2]: ")

	input, _ := bufio.NewReader(in).ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return mdshare.Expiry{TTL: mdshare.DefaultTTL}, nil
	}
	if idx, err := strconv.Atoi(input); err == nil {
		if idx < 1 || idx > len(ttlChoices) {
			return mdshare.Expiry{}, fmt.Errorf("invalid choice %q", input)
		}
		return mdshare.ParseExpiry(ttlChoices[idx-1])
	}
	// Also accept a typed value such as "3d" or "never".
	return mdshare.ParseExpiry(input)
}
