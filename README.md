# md

Share markdown files and folders from the terminal through [md.erk.im](https://md.erk.im)
and get a link. Links render in the MDReader UI, can open as slides, and expire
unless you keep them.

```console
$ md share docs/ --ttl 7d
✔ Shared 12 file(s), expires 2026-10-09 14:02 (in 7d)
https://md.erk.im/s/AbC123…
```

## Install

```bash
curl -fsSL https://github.com/eureka-corp/md-cli/releases/latest/download/install.sh | bash
```

Installs to `~/.local/bin/md` (override with `MD_INSTALL_DIR`, pin a version with
`MD_VERSION=v0.1.0`). The script checks the archive against the release checksums.
Later, `md update` upgrades in place. From source: `make install`.

## Setup

1. Sign in on https://md.erk.im/settings and create an API key.
2. Run `md setup` and paste it.

With an API key your shares belong to your account: they appear on the
My shares page and in `md list`, and can be kept forever. If `orion md setup` was
used before, `md` picks up that token and its remembered shares automatically.

| Variable | Purpose |
|---|---|
| `MD_TOKEN` | Upload token, overrides the saved one |
| `MD_URL` | Server, default `https://md.erk.im` |
| `MD_CONFIG_DIR` | Config directory, default `$XDG_CONFIG_HOME/md` or `~/.config/md` |
| `MD_NO_UPDATE_CHECK` | Disable the once-a-day "new version" notice |

## Commands

| Command | What it does |
|---|---|
| `md share <file\|dir\|->... [--ttl 7d\|never] [--title T] [--slides]` | Upload and print the link. Without `--ttl` it asks (1h, 24h, 7d, 30d, never) on a terminal and uses 24h otherwise. |
| `md share <paths>... --update` | Re-upload to the share made earlier for the same paths: same link, new content, same expiry unless `--ttl`. |
| `md share <paths>... --id <link>` | Replace a specific share. |
| `md list [--json]` | Your shares, soonest expiry first. |
| `md extend <link> [7d] [--from-now]` | Push back the expiry (max 30 days from now). |
| `md keep <link>` | Remove the expiry. Personal tokens only. |
| `md rename <link> [title]` | Set the title shown on My shares; no title resets it. |
| `md unshare <link>...` | Delete. |
| `md setup`, `md update`, `md version` | Token, self-update, build info. |
| `md --ai` | Usage guide for AI agents. |

Only the link goes to stdout, so `md share x.md | pbcopy` works. Folders are
walked recursively; hidden folders, `node_modules`, symlinks and non-markdown files
are skipped. Limits: 500 files and 5 MB of markdown per share; images are not
uploaded. Anyone with a link can read it until it expires.

## Development

```bash
make test       # go test -race ./...
make lint       # golangci-lint
make build      # ./md with version info from git
make snapshot   # local GoReleaser build into dist/
```

## Releases

Merging a pull request into `main` tags the next version and publishes a GitHub
release with binaries for Linux and macOS (amd64, arm64), `checksums.txt` and
`install.sh`. The pull request's label picks the bump:

| Label | Bump |
|---|---|
| `major` | `v1.2.3` → `v2.0.0` |
| `minor` | `v1.2.3` → `v1.3.0` |
| none | `v1.2.3` → `v1.2.4` |
| `no-release` | nothing |

Pushing a `v*` tag by hand also releases. CI runs tests, lint and a GoReleaser
dry run on every pull request.
